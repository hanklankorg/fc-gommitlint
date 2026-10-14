// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Managed source sync errors.
var (
	ErrManagedSourceNotFound       = errors.New("managed source not found")
	ErrManagedSourceIntegrity      = errors.New("managed source integrity check failed")
	ErrManagedSourceEnvNotSet      = errors.New("managed source environment variable is not set")
	ErrManagedSourceInvalidURL     = errors.New("managed source URL is invalid")
	ErrManagedSourcePath           = errors.New("managed source path is invalid")
	ErrManagedSourceOutOfDate      = errors.New("managed source is out of date")
	ErrManagedSourceUnexpectedHost = errors.New("managed source redirect changed host")
	ErrManagedSourceFetchFailed    = errors.New("managed source fetch failed")
)

const maxManagedSourceBytes = MaxConfigSize

// Managed source sync status values used in sync command output.
const (
	ManagedSourceSyncStatusChecked  = "checked"
	ManagedSourceSyncStatusSynced   = "synced"
	ManagedSourceSyncStatusVerified = "verified"
)

// SyncOptions controls managed source sync behavior.
type SyncOptions struct {
	CheckOnly bool
	DryRun    bool
	Env       EnvLookup
	FS        SyncFileSystem
	HTTP      HTTPDoer
	Now       func() time.Time
	SourceID  string
	// Timeout bounds each managed-source download when HTTP is not supplied.
	// Zero or negative selects DefaultSyncTimeout.
	Timeout time.Duration
}

// DefaultSyncTimeout bounds managed-source downloads when no timeout is given.
const DefaultSyncTimeout = 10 * time.Second

// HTTPDoer sends HTTP requests for managed source syncing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// SyncFileSystem provides the file operations needed by managed source syncing.
type SyncFileSystem interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
}

// ManagedSourceSyncItem describes the outcome for a single managed source.
type ManagedSourceSyncItem struct {
	Destination string
	ID          string
	Status      string
}

// ManagedSourceIntegrityError reports an integrity mismatch for a managed source.
type ManagedSourceIntegrityError struct {
	Actual   string
	Expected string
	ID       string
}

func (e *ManagedSourceIntegrityError) Error() string {
	return fmt.Sprintf("%s: expected %s, got %s", e.ID, e.Expected, e.Actual)
}

func (e *ManagedSourceIntegrityError) Unwrap() error {
	return ErrManagedSourceIntegrity
}

// ManagedSourceEnvError reports a missing environment variable for a managed source.
type ManagedSourceEnvError struct {
	ConfiguredSource string
	ID               string
	Variable         string
}

func (e *ManagedSourceEnvError) Error() string {
	return fmt.Sprintf("%s: %s", e.ID, e.Variable)
}

func (e *ManagedSourceEnvError) Unwrap() error {
	return ErrManagedSourceEnvNotSet
}

// ManagedSourceURLError reports an invalid resolved URL for a managed source.
type ManagedSourceURLError struct {
	ConfiguredSource string
	ID               string
	ResolvedURL      string
}

func (e *ManagedSourceURLError) Error() string {
	return fmt.Sprintf("%s: %s", e.ID, e.ResolvedURL)
}

func (e *ManagedSourceURLError) Unwrap() error {
	return ErrManagedSourceInvalidURL
}

type managedSourceFetchError struct {
	cause  error
	detail string
}

func (e *managedSourceFetchError) Error() string {
	return fmt.Sprintf("%s: %s", ErrManagedSourceFetchFailed, e.detail)
}

func (e *managedSourceFetchError) Unwrap() []error {
	return []error{ErrManagedSourceFetchFailed, e.cause}
}

// ManagedSourceSyncResult contains per-source results.
type ManagedSourceSyncResult struct {
	Items []ManagedSourceSyncItem
}

// SyncManagedSources fetches, verifies, and vendors managed sources for a config file.
//
// Secret-residue minimization: every env var referenced (in source URLs or
// header templates) is collected across all sources, then unset from the
// process environment in a single defer at the end of the sync. Two sources
// that share a token therefore both resolve correctly before the env block
// is scrubbed.
func SyncManagedSources(ctx context.Context, configPath string, options SyncOptions) (ManagedSourceSyncResult, error) {
	options = withSyncOptionDefaults(options)

	content, err := options.FS.ReadFile(configPath)
	if err != nil {
		return ManagedSourceSyncResult{}, fmt.Errorf("read config: %w", err)
	}

	parsed := ParseConfigBytesStrict(configPath, content)
	if parsed.Error != nil {
		return ManagedSourceSyncResult{}, parsed.Error
	}

	return syncManagedSources(ctx, configPath, content, parsed.Config, options)
}

func syncManagedSources(ctx context.Context, configPath string, configContent []byte, cfg configTypes.Config, options SyncOptions) (ManagedSourceSyncResult, error) {
	options = withSyncOptionDefaults(options)

	indices := selectManagedSourceIndices(cfg.ManagedSources, options.SourceID)

	result, err := newManagedSourceSyncResult(indices, options.SourceID)
	if err != nil || len(indices) == 0 {
		return result, err
	}

	var consumedEnvVars []string

	defer func() {
		unsetSecretEnvVars(options.Env, dedupeEnvNames(consumedEnvVars))
	}()

	updatedConfig := cfg
	rootDir := filepath.Dir(filepath.Clean(configPath))
	updatedAny := false
	staged := make([]syncedManagedSource, 0, len(indices))

	for _, index := range indices {
		item, consumed, err := prepareManagedSource(ctx, rootDir, updatedConfig.ManagedSources[index], options)
		consumedEnvVars = append(consumedEnvVars, consumed...)

		if err != nil {
			return ManagedSourceSyncResult{}, err
		}

		if item.updateConfig {
			updatedConfig.ManagedSources[index].Resolved = item.resolved
			updatedAny = true
		}

		result.Items = append(result.Items, item.ManagedSourceSyncItem)
		staged = append(staged, item)
	}

	encodedConfig, err := encodeUpdatedManagedConfig(configPath, configContent, updatedConfig, updatedAny, options)
	if err != nil {
		return ManagedSourceSyncResult{}, err
	}

	for _, item := range staged {
		if !item.writeDestination {
			continue
		}

		if err := options.FS.WriteFileAtomic(item.Destination, item.content, 0o600); err != nil {
			return ManagedSourceSyncResult{}, fmt.Errorf("write managed source %s: %w", item.ID, err)
		}
	}

	if encodedConfig != nil {
		if err := options.FS.WriteFileAtomic(configPath, encodedConfig, 0o600); err != nil {
			return ManagedSourceSyncResult{}, fmt.Errorf("write config: %w", err)
		}
	}

	return result, nil
}

func newManagedSourceSyncResult(indices []int, sourceID string) (ManagedSourceSyncResult, error) {
	if len(indices) == 0 {
		if sourceID != "" {
			return ManagedSourceSyncResult{}, fmt.Errorf("%w: %s", ErrManagedSourceNotFound, sourceID)
		}

		return ManagedSourceSyncResult{}, nil
	}

	return ManagedSourceSyncResult{Items: make([]ManagedSourceSyncItem, 0, len(indices))}, nil
}

func encodeUpdatedManagedConfig(configPath string, configContent []byte, updatedConfig configTypes.Config, updatedAny bool, options SyncOptions) ([]byte, error) {
	if options.CheckOnly || options.DryRun || !updatedAny {
		return nil, nil
	}

	encodedConfig, err := encodeConfigWithManagedSources(configPath, configContent, updatedConfig.ManagedSources)
	if err != nil {
		return nil, fmt.Errorf("encode config: %w", err)
	}

	return encodedConfig, nil
}

func withSyncOptionDefaults(options SyncOptions) SyncOptions {
	if options.FS == nil {
		options.FS = filesystem.NewOSFileSystem()
	}

	if options.Env == nil {
		options.Env = OSEnvironment{}
	}

	if options.Timeout <= 0 {
		options.Timeout = DefaultSyncTimeout
	}

	if options.HTTP == nil {
		options.HTTP = &http.Client{
			Timeout:       options.Timeout,
			CheckRedirect: validateManagedSourceRedirect,
		}
	}

	if options.Now == nil {
		options.Now = time.Now
	}

	return options
}

type syncedManagedSource struct {
	ManagedSourceSyncItem

	content          []byte
	resolved         configTypes.ManagedSourceResolved
	updateConfig     bool
	writeDestination bool
}

func prepareManagedSource(ctx context.Context, rootDir string, source configTypes.ManagedSource, options SyncOptions) (syncedManagedSource, []string, error) {
	content, finalURL, revision, consumedEnv, err := fetchAndValidateManagedSource(ctx, source, options.HTTP, options.Env)
	if err != nil {
		return syncedManagedSource{}, consumedEnv, err
	}

	item, err := newSyncedManagedSource(rootDir, source)
	if err != nil {
		return syncedManagedSource{}, consumedEnv, err
	}

	currentMatches, err := verifyManagedSourceCurrentState(source.ID, item.Destination, content, options)
	if err != nil {
		return syncedManagedSource{}, consumedEnv, err
	}

	if options.CheckOnly || options.DryRun {
		item.Status = ManagedSourceSyncStatusChecked

		return item, consumedEnv, nil
	}

	resolved := configTypes.ManagedSourceResolved{
		FetchedAt: options.Now().UTC().Format(time.RFC3339),
		FinalURL:  finalURL,
		Revision:  revision,
	}

	if currentMatches && managedSourceResolvedMatches(source.Resolved, resolved) {
		return item, consumedEnv, nil
	}

	item.Status = ManagedSourceSyncStatusSynced
	item.content = content
	item.resolved = resolved
	item.updateConfig = true
	item.writeDestination = !currentMatches

	return item, consumedEnv, nil
}

func fetchAndValidateManagedSource(ctx context.Context, source configTypes.ManagedSource, client HTTPDoer, env EnvLookup) ([]byte, string, string, []string, error) {
	resolvedURL, urlEnvVars, err := resolveManagedSourceURL(source, env)
	if err != nil {
		return nil, "", "", urlEnvVars, err
	}

	resolvedHeaders, headerEnvVars, err := resolveManagedSourceHeaders(source, env)
	// slices.Concat instead of append(a, b...) because the caller's urlEnvVars
	// must not be aliased — append may reuse its backing array if cap permits.
	consumedEnv := slices.Concat(urlEnvVars, headerEnvVars)

	if err != nil {
		return nil, "", "", consumedEnv, err
	}

	// Belt-and-suspenders: createManagedSourceRequest zeroes each []byte
	// after applying it to the http.Header, but error paths before that
	// (URL parse, scheme check) skip the loop. Defer guarantees the
	// resolver-owned buffers are zeroed before returning regardless of
	// which path we exit through.
	defer clearByteSecrets(resolvedHeaders)

	content, finalURL, revision, err := fetchManagedSource(ctx, client, resolvedURL, resolvedHeaders)
	if err != nil {
		return nil, "", "", consumedEnv, err
	}

	if err := verifyManagedSourceIntegrity(source, content); err != nil {
		return nil, "", "", consumedEnv, err
	}

	if err := validateManagedSourceContent(source, content); err != nil {
		return nil, "", "", consumedEnv, err
	}

	return content, finalURL, revision, consumedEnv, nil
}

// resolveManagedSourceHeaders expands ${ENV_VAR} placeholders in header
// values to their environment values. Returns an error if any referenced
// variable is missing — silently sending a request with an empty auth
// header would produce a 401 the user can't debug without seeing the cause.
//
// Returns nil on a source with no Headers; callers handle the empty case
// without an extra nil check.
//
// Returns []byte rather than string so the resolver-owned copy of each
// secret can be zeroed deterministically after the HTTP request applies
// it — see clearByteSecrets and createManagedSourceRequest. The second
// return value is the deduped list of env var names consumed during
// expansion, so the caller can scrub them from the process env block.
func resolveManagedSourceHeaders(source configTypes.ManagedSource, env EnvLookup) (map[string][]byte, []string, error) {
	if len(source.Headers) == 0 {
		return nil, nil, nil
	}

	resolved := make(map[string][]byte, len(source.Headers))

	var consumed []string

	names := make([]string, 0, len(source.Headers))
	for name := range source.Headers {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		value := source.Headers[name]
		expanded, refs, err := expandManagedSourceTemplate(value, env, source.ID, name)
		consumed = append(consumed, refs...)

		if err != nil {
			// Scrub the partial map before bailing — anything already
			// resolved is a secret we own and must not leak via GC delay.
			clearByteSecrets(resolved)

			return nil, consumed, err
		}

		resolved[name] = expanded
	}

	return resolved, consumed, nil
}

// expandManagedSourceTemplate substitutes ${VAR} placeholders in value with
// the corresponding environment values. Unknown variables produce a typed
// error naming the offending header so debugging doesn't require trial-
// and-error.
//
// Returns []byte so the resolver-owned buffer can be zeroed once the value
// has been applied to the HTTP request; the second return is the list of
// env var names referenced during this expansion.
//
// The substitution is intentionally minimal — no nested expressions, no
// defaults, no string functions. Same template grammar everywhere keeps
// the security review surface small.
func expandManagedSourceTemplate(value string, env EnvLookup, sourceID, headerName string) ([]byte, []string, error) {
	if !strings.Contains(value, "${") {
		return []byte(value), nil, nil
	}

	var (
		result   = make([]byte, 0, len(value))
		consumed []string
		index    int
	)

	for index < len(value) {
		open := strings.Index(value[index:], "${")
		if open == -1 {
			result = append(result, value[index:]...)

			break
		}

		result = append(result, value[index:index+open]...)
		index += open

		closeIdx := strings.Index(value[index:], "}")
		if closeIdx == -1 {
			// Unterminated placeholder — treat the remainder as literal so we
			// don't swallow content silently. Callers can spot this via the
			// `${` in the resolved value if they look.
			result = append(result, value[index:]...)

			break
		}

		varName := value[index+2 : index+closeIdx]
		if !configTypes.IsValidManagedSourceEnvName(varName) {
			clear(result)

			return nil, consumed, &ManagedSourceEnvError{
				ConfiguredSource: "headers." + headerName,
				ID:               sourceID,
				Variable:         varName,
			}
		}

		expanded := strings.TrimSpace(env.GetEnv(varName))
		if expanded == "" {
			clear(result)

			return nil, consumed, &ManagedSourceEnvError{
				ConfiguredSource: "headers." + headerName,
				ID:               sourceID,
				Variable:         varName,
			}
		}

		result = append(result, expanded...)
		consumed = append(consumed, varName)

		index += closeIdx + 1
	}

	return result, consumed, nil
}

func resolveManagedSourceURL(source configTypes.ManagedSource, env EnvLookup) (string, []string, error) {
	if variable, ok := configTypes.ParseManagedSourceEnvReference(source.Source); ok {
		resolved := strings.TrimSpace(env.GetEnv(variable))
		if resolved == "" {
			return "", nil, &ManagedSourceEnvError{ConfiguredSource: source.Source, ID: source.ID, Variable: variable}
		}

		if err := validateManagedSourceResolvedURL(source, resolved); err != nil {
			return "", []string{variable}, err
		}

		return resolved, []string{variable}, nil
	}

	if err := validateManagedSourceResolvedURL(source, source.Source); err != nil {
		return "", nil, err
	}

	return source.Source, nil, nil
}

func validateManagedSourceResolvedURL(source configTypes.ManagedSource, rawURL string) error {
	if !configTypes.IsValidManagedSourceURL(rawURL) {
		configuredSource := source.Source
		if _, placeholder := configTypes.ParseManagedSourceEnvReference(configuredSource); !placeholder {
			configuredSource = configTypes.SanitizeManagedSourceURL(configuredSource)
		}

		return &ManagedSourceURLError{
			ConfiguredSource: configuredSource,
			ID:               source.ID,
			ResolvedURL:      configTypes.SanitizeManagedSourceURL(rawURL),
		}
	}

	return nil
}

func newSyncedManagedSource(rootDir string, source configTypes.ManagedSource) (syncedManagedSource, error) {
	destinationPath, err := resolveManagedSourceDestination(rootDir, source.Destination)
	if err != nil {
		return syncedManagedSource{}, err
	}

	return syncedManagedSource{
		Destination: destinationPath,
		ID:          source.ID,
		Status:      ManagedSourceSyncStatusVerified,
	}, nil
}

func verifyManagedSourceCurrentState(sourceID, destinationPath string, content []byte, options SyncOptions) (bool, error) {
	if !options.CheckOnly {
		if _, statErr := options.FS.Stat(destinationPath); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return false, nil
			}

			return false, statErr
		}

		currentContent, readErr := options.FS.ReadFile(destinationPath)
		if readErr != nil {
			return false, readErr
		}

		return bytes.Equal(currentContent, content), nil
	}

	currentContent, err := options.FS.ReadFile(destinationPath)
	if err != nil || !bytes.Equal(currentContent, content) {
		return false, fmt.Errorf("%w: %s", ErrManagedSourceOutOfDate, sourceID)
	}

	return true, nil
}

func managedSourceResolvedMatches(current, expected configTypes.ManagedSourceResolved) bool {
	return current.FinalURL == expected.FinalURL && current.Revision == expected.Revision && current.FetchedAt != ""
}

func selectManagedSourceIndices(sources []configTypes.ManagedSource, sourceID string) []int {
	indices := make([]int, 0, len(sources))
	for index, source := range sources {
		if sourceID == "" || source.ID == sourceID {
			indices = append(indices, index)
		}
	}

	return indices
}

func fetchManagedSource(ctx context.Context, client HTTPDoer, rawURL string, headers map[string][]byte) ([]byte, string, string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", "", &managedSourceFetchError{
			cause:  err,
			detail: "invalid URL " + configTypes.SanitizeManagedSourceURL(rawURL),
		}
	}

	// Managed-source URL validation requires HTTPS. Keep this additional check
	// here so direct callers also cannot transmit configured headers over HTTP.
	if len(headers) > 0 && !strings.EqualFold(parsedURL.Scheme, "https") {
		return nil, "", "", fmt.Errorf("%w: headers require https:// (got %s://)", ErrManagedSourceFetchFailed, parsedURL.Scheme)
	}

	requestURL, err := createManagedSourceRequest(ctx, rawURL, headers)
	if err != nil {
		return nil, "", "", err
	}

	resp, err := client.Do(requestURL)
	if err != nil {
		return nil, "", "", &managedSourceFetchError{
			cause:  err,
			detail: "request to " + configTypes.SanitizeManagedSourceURL(rawURL) + " failed",
		}
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	return readManagedSourceResponse(resp, parsedURL, rawURL)
}

func createManagedSourceRequest(ctx context.Context, rawURL string, headers map[string][]byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &managedSourceFetchError{
			cause:  err,
			detail: "invalid URL " + configTypes.SanitizeManagedSourceURL(rawURL),
		}
	}

	for name, value := range headers {
		// Validation up-front guarantees name passes through Header.Set safely;
		// this loop is the single place we apply user-supplied header values.
		// string(value) makes a heap copy that net/http retains — we can't
		// zero that copy, only our buffer. clear() reduces the resident
		// window for the resolver-owned bytes to "until first Set call."
		req.Header.Set(name, string(value))
		clear(value)
	}

	return req, nil
}

func readManagedSourceResponse(resp *http.Response, parsedURL *url.URL, rawURL string) ([]byte, string, string, error) {
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", "", fmt.Errorf("%w: unexpected status %s", ErrManagedSourceFetchFailed, resp.Status)
	}

	if resp.Request != nil && !strings.EqualFold(resp.Request.URL.Host, parsedURL.Host) {
		return nil, "", "", fmt.Errorf("%w: %s -> %s", ErrManagedSourceUnexpectedHost, parsedURL.Host, resp.Request.URL.Host)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManagedSourceBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("%w: %w", ErrManagedSourceFetchFailed, err)
	}

	if int64(len(body)) > maxManagedSourceBytes {
		return nil, "", "", fmt.Errorf("%w: response exceeds %d bytes", ErrManagedSourceFetchFailed, maxManagedSourceBytes)
	}

	// Keep the full URL on the HTTP request, but omit secret-bearing metadata
	// before the resolved URL crosses the persistence and display boundary.
	finalURL := configTypes.SanitizeManagedSourceURL(rawURL)
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = configTypes.SanitizeManagedSourceURL(resp.Request.URL.String())
	}

	return body, finalURL, extractRevisionFromURL(finalURL), nil
}

func verifyManagedSourceIntegrity(source configTypes.ManagedSource, content []byte) error {
	actual, matches := managedSourceIntegrityMatches(source.Integrity.SHA256, content)
	if !matches {
		return &ManagedSourceIntegrityError{
			Actual:   actual,
			Expected: source.Integrity.SHA256,
			ID:       source.ID,
		}
	}

	return nil
}

func validateManagedSourceContent(source configTypes.ManagedSource, content []byte) error {
	parseResult := ParseConfigBytesStrict(source.Destination, content)
	if parseResult.Error != nil {
		return parseResult.Error
	}

	if parseResult.Config.ManagedSources != nil {
		return fmt.Errorf("%w: managed source %s may not declare managed_sources", ErrConfigValidation, source.ID)
	}

	if _, err := MergeAndValidateConfigs(NewConfigWithDefaults(), parseResult.Config); err != nil {
		return err
	}

	return nil
}

func resolveManagedSourceDestination(rootDir, destination string) (string, error) {
	trimmed := strings.TrimSpace(destination)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty destination", ErrManagedSourcePath)
	}

	if !configTypes.IsLocalManagedSourceDestination(trimmed) {
		return "", fmt.Errorf("%w: destination must be relative", ErrManagedSourcePath)
	}

	resolved := filepath.Clean(filepath.Join(rootDir, trimmed))

	if err := domain.DefaultSecurityPolicy().ValidatePathWithinBase(rootDir, resolved); err != nil {
		return "", fmt.Errorf("%w: destination escapes config root", ErrManagedSourcePath)
	}

	return resolved, nil
}

func extractRevisionFromURL(rawURL string) string {
	parts := strings.Split(strings.Trim(rawURL, "/"), "/")
	for index := range len(parts) - 1 {
		if parts[index] == "commit" {
			return parts[index+1]
		}
	}

	return ""
}

// validateManagedSourceRedirect rejects unsafe redirects before net/http sends
// the next request. URL.Host is the managed-source host identity contract, so
// explicit ports remain significant while host casing does not.
func validateManagedSourceRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}

	if len(via) >= 10 {
		return fmt.Errorf("%w: stopped after 10 redirects", ErrManagedSourceFetchFailed)
	}

	if req.URL == nil || !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("%w: redirect target must use https", ErrManagedSourceInvalidURL)
	}

	origin := via[0].URL
	if origin == nil || !strings.EqualFold(origin.Host, req.URL.Host) {
		originHost := ""
		if origin != nil {
			originHost = origin.Host
		}

		return fmt.Errorf("%w: %s -> %s", ErrManagedSourceUnexpectedHost, originHost, req.URL.Host)
	}

	return nil
}

// clearByteSecrets zeroes every []byte value in the given headers map.
// Idempotent: clear() on an already-zero slice is a no-op, so callers can
// invoke this both eagerly (in createManagedSourceRequest after each Set)
// and as a defer-guarded backstop without worry.
//
// The map keys (header names) and the map itself are not secrets and are
// left untouched. The map values are the resolver-owned secret bytes.
func clearByteSecrets(headers map[string][]byte) {
	for _, value := range headers {
		clear(value)
	}
}

// unsetSecretEnvVars removes the named variables from the process env block
// if the EnvLookup implements EnvUnsetter. Test Environments that don't
// implement it (testEnvMap, testEnv) are treated as no-ops so test runs
// never mutate the real process environment.
func unsetSecretEnvVars(env EnvLookup, names []string) {
	unsetter, ok := env.(EnvUnsetter)
	if !ok {
		return
	}

	for _, name := range names {
		unsetter.UnsetEnv(name)
	}
}

// dedupeEnvNames returns the input slice with duplicates removed, preserving
// first-occurrence order. Used so a single env var referenced by multiple
// managed sources is only passed to UnsetEnv once.
func dedupeEnvNames(names []string) []string {
	if len(names) < 2 {
		return names
	}

	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))

	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}

		seen[name] = struct{}{}
		out = append(out, name)
	}

	return out
}
