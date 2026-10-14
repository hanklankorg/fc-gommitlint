// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// ErrConfigValidation is returned when config validation fails.
var ErrConfigValidation = errors.New("config validation failed")

// Config loading errors.
var (
	ErrConfigExtendsCycle         = errors.New("config extends cycle detected")
	ErrConfigExtendsDepthExceeded = errors.New("config extends depth exceeded")
	ErrConfigEmptyExtendPath      = errors.New("config extends contains empty path")
	// ErrConfigExtendsEscape fires when an extends: entry resolves outside the
	// parent config file's directory subtree. The configuration loader treats
	// a config file's directory as a security boundary: anything outside is
	// off-limits, so a malicious .gommitlint.yaml committed to a repo cannot
	// `extends: ../../../etc/passwd` its way into reading user-private files
	// or applying attacker-controlled rules from outside the repo.
	ErrConfigExtendsEscape = errors.New("config extends path resolves outside the parent config directory")
)

const (
	configSourceDefaults  = string(SourceTypeDefaults)
	maxConfigExtendsDepth = 10
)

// LoadResult holds the loaded configuration and information about its sources.
type LoadResult struct {
	Config             configTypes.Config
	ManagedSourcesPath string
	SourceGraph        []ConfigSourceNode
	Sources            []ConfigSource
	Warnings           []domain.Warning // Non-fatal issues encountered during loading
}

// SourcesSummary returns a human-readable summary of config sources.
// Shows highest priority first (e.g., ".gommitlint.yaml + ~/.config/gommitlint/settings.yaml").
func (r LoadResult) SourcesSummary() string {
	if len(r.Sources) == 0 {
		return configSourceDefaults
	}

	// Show in reverse order (highest priority first) for user clarity
	var parts []string
	for _, v := range slices.Backward(r.Sources) {
		parts = append(parts, v.Path)
	}

	return strings.Join(parts, " + ")
}

// HasWarnings returns true if any warnings were collected during loading.
func (r LoadResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// LoadConfigFromPathWithFS loads configuration from a specific path using a custom filesystem.
// Parsing is strict: any read or parse error is returned.
func LoadConfigFromPathWithFS(configPath string, fs FileSystem) (configTypes.Config, error) {
	result, err := LoadConfigFromPathDetailedWithFS(configPath, fs)
	if err != nil {
		return configTypes.Config{}, err
	}

	return result.Config, nil
}

// LoadConfigFromPathDetailed loads configuration from an explicit path and returns source metadata.
func LoadConfigFromPathDetailed(configPath string) (LoadResult, error) {
	return loadConfigFromPathDetailedWithFS(configPath, nil, SourceTypeCLI)
}

// LoadConfigFromPathDetailedWithFS loads configuration from an explicit path using a custom filesystem.
func LoadConfigFromPathDetailedWithFS(configPath string, fs FileSystem) (LoadResult, error) {
	return loadConfigFromPathDetailedWithFS(configPath, fs, SourceTypeCLI)
}

// LoadConfigHierarchy loads and merges all config files in precedence order.
// Order: defaults → global (~/.config/gommitlint/) → project (.gommitlint.yaml)
// Later sources override earlier ones (like git config).
func LoadConfigHierarchy(repoPath, workingDir string) (LoadResult, error) {
	return loadConfigHierarchyWithEnv(repoPath, workingDir, OSEnvironment{}, nil)
}

// LoadConfigHierarchyWithEnvAndFS loads config hierarchy using custom environment and filesystem.
// This is primarily useful for testing where both environment variables and filesystem need to be mocked.
func LoadConfigHierarchyWithEnvAndFS(repoPath, workingDir string, env EnvLookup, fs FileSystem) (LoadResult, error) {
	return loadConfigHierarchyWithEnv(repoPath, workingDir, env, fs)
}

// loadConfigHierarchyWithEnv is the internal implementation for hierarchical config loading.
// Parse errors in discovered configs are collected as warnings (non-fatal) rather than errors,
// allowing graceful degradation when global or project configs are malformed.
func loadConfigHierarchyWithEnv(repoPath, workingDir string, env EnvLookup, fs FileSystem) (LoadResult, error) {
	result := LoadResult{
		Config:   NewConfigWithDefaults(),
		Sources:  []ConfigSource{{Path: configSourceDefaults, Type: SourceTypeDefaults}},
		Warnings: []domain.Warning{},
	}

	// Find all existing config files
	var found FoundConfigs
	if fs != nil {
		found = FindAllExistingConfigFilesWithEnvAndFS(repoPath, workingDir, env, fs)
	} else {
		found = FindAllExistingConfigFilesWithEnv(repoPath, workingDir, env)
	}

	fsForValidation := fs
	if fsForValidation == nil {
		fsForValidation = filesystem.NewOSFileSystem()
	}

	for _, source := range found.Sources() {
		loadConfigSource(&result, source, fs, fsForValidation)
	}

	// Validate the final merged config
	result.Warnings = append(result.Warnings, collectIgnoreCommitWarnings(result.Config.IgnoreCommits)...)
	result.Warnings = append(result.Warnings, collectRuleOverlapWarnings(result.Config.Rules)...)

	if validationErrors := configTypes.ValidateConfigErrors(result.Config); len(validationErrors) > 0 {
		messages := configTypes.ConfigErrorMessages(validationErrors)

		return LoadResult{}, fmt.Errorf("%w: %s", ErrConfigValidation, strings.Join(messages, "; "))
	}

	return result, nil
}

func collectIgnoreCommitWarnings(ignoreCommits []string) []domain.Warning {
	warnings := make([]domain.Warning, 0)

	for _, sha := range ignoreCommits {
		if domain.ParseSHA(sha) == "" {
			warnings = append(warnings, domain.Warning{
				Code:    domain.WarnInvalidIgnoreCommit,
				Message: "Invalid SHA in ignore_commits: " + sha,
				Context: map[string]string{domain.WarningContextSHA: sha},
			})
		}
	}

	return warnings
}

// collectRuleOverlapWarnings detects rules that appear in both rules.enabled
// and rules.disabled. The resolver chooses disabled-wins silently — the user
// who wrote both probably has the wrong mental model of precedence. Warning
// surfaces the contradiction so they can clean their config.
//
// Names are compared after normalisation so casing or whitespace differences
// don't mask the overlap.
func collectRuleOverlapWarnings(rules configTypes.RulesConfig) []domain.Warning {
	if len(rules.Enabled) == 0 || len(rules.Disabled) == 0 {
		return nil
	}

	enabledSet := make(map[string]bool, len(rules.Enabled))
	for _, name := range rules.Enabled {
		enabledSet[configTypes.NormalizeRuleName(name)] = true
	}

	var overlap []string

	for _, name := range rules.Disabled {
		if enabledSet[configTypes.NormalizeRuleName(name)] {
			overlap = append(overlap, name)
		}
	}

	if len(overlap) == 0 {
		return nil
	}

	return []domain.Warning{{
		Code:    domain.WarnConfigRulesOverlap,
		Message: "rules listed in both enabled and disabled (disabled wins): " + strings.Join(overlap, ", "),
		Context: map[string]string{
			domain.WarningContextRules: strings.Join(overlap, ","),
		},
	}}
}

func loadConfigSource(result *LoadResult, source ConfigSource, fs FileSystem, fsForValidation FileSystem) {
	if source.Path == "" {
		return
	}

	loaded, err := loadConfigTree(source.Path, source.Type, fs, fsForValidation, map[string]bool{}, 0, isTrustedConfigSource(source.Type), true)
	if err != nil {
		// Carry the underlying error text in WarningContextCause so the formatter
		// can render it inline. "Could not parse X" without the *why* (YAML
		// line/column, type mismatch, ...) leaves users guessing — the cause
		// is exactly the actionable signal they need.
		result.Warnings = append(result.Warnings, domain.Warning{
			Code:    domain.WarnConfigParse,
			Message: fmt.Sprintf("%s config %s: %v (skipped)", source.Type, source.Path, err),
			Context: map[string]string{
				domain.WarningContextPath:   source.Path,
				domain.WarningContextSource: string(source.Type),
				domain.WarningContextCause:  err.Error(),
			},
		})

		return
	}

	result.Config = MergeConfig(result.Config, loaded.Config)
	if loaded.ManagedSourcesPath != "" {
		result.ManagedSourcesPath = loaded.ManagedSourcesPath
	}

	result.SourceGraph = append(result.SourceGraph, loaded.SourceGraph...)
	result.Sources = append(result.Sources, loaded.Sources...)
	result.Warnings = append(result.Warnings, loaded.Warnings...)
}

func parseConfigFileStrict(path string, fs FileSystem) ParseResult {
	if fs != nil {
		return ParseConfigFileStrictWithFS(path, fs)
	}

	return ParseConfigFileStrict(path)
}

// isEmptyFile returns true when the file at path is zero bytes long.
// Used to suppress the group-writable warning on stub config files — the
// security check fires immediately when content lands.
func isEmptyFile(path string, fs FileSystem) bool {
	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	info, err := fs.Stat(path)
	if err != nil {
		return false
	}

	return info.Size() == 0
}

func loadConfigFromPathDetailedWithFS(configPath string, fs FileSystem, sourceType ConfigSourceType) (LoadResult, error) {
	loaded, err := loadConfigTree(configPath, sourceType, fs, fs, map[string]bool{}, 0, isTrustedConfigSource(sourceType), true)
	if err != nil {
		return LoadResult{}, err
	}

	config, err := MergeAndValidateConfigs(NewConfigWithDefaults(), loaded.Config)
	if err != nil {
		return LoadResult{}, err
	}

	result := LoadResult{
		Config:             config,
		ManagedSourcesPath: loaded.ManagedSourcesPath,
		SourceGraph:        loaded.SourceGraph,
		Sources:            append([]ConfigSource{{Path: configSourceDefaults, Type: SourceTypeDefaults}}, loaded.Sources...),
		Warnings:           append(loaded.Warnings, collectIgnoreCommitWarnings(config.IgnoreCommits)...),
	}
	result.Warnings = append(result.Warnings, collectRuleOverlapWarnings(config.Rules)...)

	return result, nil
}

func loadConfigTree(path string, sourceType ConfigSourceType, fs FileSystem, fsForValidation FileSystem, stack map[string]bool, depth int, trusted, ownsManagedSources bool) (LoadResult, error) {
	if depth > maxConfigExtendsDepth {
		return LoadResult{}, fmt.Errorf("%w: %s", ErrConfigExtendsDepthExceeded, path)
	}

	if presetName, ok := ParsePresetReference(path); ok {
		return loadPresetConfigTree(path, presetName, stack, depth)
	}

	return loadFileConfigTree(path, sourceType, fs, fsForValidation, stack, depth, trusted, ownsManagedSources)
}

func loadFileConfigTree(path string, sourceType ConfigSourceType, fs FileSystem, fsForValidation FileSystem, stack map[string]bool, depth int, trusted, ownsManagedSources bool) (LoadResult, error) {
	resolvedPath := filepath.Clean(path)
	if stack[resolvedPath] {
		return LoadResult{}, fmt.Errorf("%w: %s", ErrConfigExtendsCycle, resolvedPath)
	}

	if fsForValidation == nil {
		fsForValidation = fs
	}

	permAdvisory, err := auditConfigFilePermissions(resolvedPath, fsForValidation)
	if err != nil {
		return LoadResult{}, err
	}

	parseResult := parseConfigFileStrict(resolvedPath, fs)
	if parseResult.Error != nil {
		return LoadResult{}, parseResult.Error
	}

	if !ownsManagedSources && parseResult.Config.ManagedSources != nil {
		return LoadResult{}, fmt.Errorf("%w: managed_sources may only be declared by a global, project, or explicit config: %s", ErrConfigValidation, resolvedPath)
	}

	stack[resolvedPath] = true
	defer delete(stack, resolvedPath)

	result := LoadResult{Warnings: parseResult.Warnings}

	if ownsManagedSources && parseResult.Config.ManagedSources != nil {
		result.ManagedSourcesPath = resolvedPath
	}

	if ownsManagedSources {
		if err := applyManagedSources(&result, resolvedPath, parseResult.Config, fs, fsForValidation, stack, depth, trusted); err != nil {
			return LoadResult{}, err
		}
	}

	if err := applyExtends(&result, resolvedPath, parseResult.Config.Extends, fs, fsForValidation, stack, depth, trusted); err != nil {
		return LoadResult{}, err
	}

	result.Config = MergeConfig(result.Config, parseResult.Config)
	result.SourceGraph = []ConfigSourceNode{{
		Children: result.SourceGraph,
		Path:     resolvedPath,
		Type:     sourceType,
	}}
	result.Sources = append(result.Sources, ConfigSource{Path: resolvedPath, Type: sourceType})

	if permAdvisory != nil {
		result.Warnings = append(result.Warnings, *permAdvisory)
	}

	return result, nil
}

// mergeLoadResult folds a loaded child config (an extends target, preset, or
// managed source) into the accumulating parent result.
func mergeLoadResult(into *LoadResult, child LoadResult) {
	into.Config = MergeConfig(into.Config, child.Config)
	into.SourceGraph = append(into.SourceGraph, child.SourceGraph...)
	into.Sources = append(into.Sources, child.Sources...)
	into.Warnings = append(into.Warnings, child.Warnings...)
}

// applyManagedSources merges each vendored managed-source `destination` into
// result, like an implicit extends, so `managed_sources` applies itself with no
// separate `extends` entry. Managed sources merge before extends — they are the
// imported base that local extends and the file's own fields refine. A
// destination also named in `extends` is skipped here (the extends pass loads
// it; this avoids a double-load the cycle guard would reject). A declared but
// unvendored source warns rather than failing the load — the fix is to run
// `gommitlint config sync`, not to edit the config.
func applyManagedSources(result *LoadResult, parentPath string, cfg configTypes.Config, fs FileSystem, fsForValidation FileSystem, stack map[string]bool, depth int, trusted bool) error {
	if len(cfg.ManagedSources) == 0 {
		return nil
	}

	extendsPaths := make(map[string]bool, len(cfg.Extends))
	for _, extendPath := range cfg.Extends {
		if resolved, err := resolveExtendedConfigPath(parentPath, extendPath, trusted, fsForValidation); err == nil {
			extendsPaths[resolved] = true
		}
	}

	for _, managed := range cfg.ManagedSources {
		if strings.TrimSpace(managed.Destination) == "" {
			continue
		}

		resolvedDest, err := resolveManagedSourceDestination(filepath.Dir(parentPath), managed.Destination)
		if err != nil {
			return err
		}

		if extendsPaths[resolvedDest] {
			continue // also in extends; the extends pass loads it
		}

		applied, err := loadConfigTree(resolvedDest, SourceTypeExtended, fs, fsForValidation, stack, depth+1, trusted, false)
		if err != nil {
			result.Warnings = append(result.Warnings, domain.Warning{
				Code:    domain.WarnConfigParse,
				Message: fmt.Sprintf("managed source %q not applied: %v (run 'gommitlint config sync')", managed.ID, err),
				Context: map[string]string{domain.WarningContextPath: resolvedDest, domain.WarningContextSource: managed.ID},
			})

			continue
		}

		mergeLoadResult(result, applied)
	}

	return nil
}

// applyExtends merges each `extends` entry (a local file or `preset:<name>`)
// into result, in list order so later entries override earlier ones.
func applyExtends(result *LoadResult, parentPath string, extends []string, fs FileSystem, fsForValidation FileSystem, stack map[string]bool, depth int, trusted bool) error {
	for _, extendPath := range extends {
		resolved, err := resolveExtendedConfigPath(parentPath, extendPath, trusted, fsForValidation)
		if err != nil {
			return err
		}

		extended, err := loadConfigTree(resolved, SourceTypeExtended, fs, fsForValidation, stack, depth+1, trusted, false)
		if err != nil {
			return err
		}

		mergeLoadResult(result, extended)
	}

	return nil
}

func loadPresetConfigTree(ref, presetName string, stack map[string]bool, depth int) (LoadResult, error) {
	if depth > maxConfigExtendsDepth {
		return LoadResult{}, fmt.Errorf("%w: %s", ErrConfigExtendsDepthExceeded, ref)
	}

	if stack[ref] {
		return LoadResult{}, fmt.Errorf("%w: %s", ErrConfigExtendsCycle, ref)
	}

	cfg, err := ResolvePreset(presetName)
	if err != nil {
		return LoadResult{}, err
	}

	stack[ref] = true
	defer delete(stack, ref)

	return LoadResult{
		Config: cfg,
		SourceGraph: []ConfigSourceNode{{
			Path: ref,
			Type: SourceTypePreset,
		}},
		Sources: []ConfigSource{{
			Path: ref,
			Type: SourceTypePreset,
		}},
	}, nil
}

// auditConfigFilePermissions runs the file-permission policy check and
// translates its result into either a fatal error or an advisory warning.
// Extracted from loadFileConfigTree so the orchestrator's branching stays
// linear; the policy decisions (what counts as fatal vs. advisory) are
// localised here.
//
// Returns:
//   - (nil, nil) when the file is OK or doesn't exist (parser will surface
//     the not-found via parseResult.Error).
//   - (*Warning, nil) for group-writable-only — advisory, loaded anyway.
//   - (nil, error) for fatal policy violations (world-writable, symlink,
//     executable bits — the real tampering vectors).
func auditConfigFilePermissions(resolvedPath string, fsForValidation FileSystem) (*domain.Warning, error) {
	err := ValidateConfigPathSecurity(resolvedPath, fsForValidation)
	if err == nil {
		return nil, nil //nolint:nilnil // (nil, nil) means "file passed security check"; no advisory needed.
	}

	switch {
	case errors.Is(err, os.ErrNotExist):
		// Treated below by parseConfigFileStrict — propagate the not-found
		// error from there so the caller sees a single, consistent error.
		return nil, nil //nolint:nilnil // file-not-found is handled downstream by the parser.
	case errors.Is(err, ErrConfigPathGroupWritable):
		// Group-writable is advisory: matches normal repo umasks (0002 -> 0664).
		// World-writable, symlinks, and executable bits remain fatal — those are
		// the real tampering vectors.
		//
		// Empty files get no warning. There is nothing to load and nothing for
		// an attacker to exploit until content lands; the non-empty path fires
		// on the next run the moment that changes. Stub .gommitlint.yaml files
		// shouldn't spam the warning on every invocation.
		if isEmptyFile(resolvedPath, fsForValidation) {
			return nil, nil //nolint:nilnil // empty + group-writable is silent until content lands.
		}

		return &domain.Warning{
			Code:    domain.WarnConfigInsecurePerms,
			Message: fmt.Sprintf("%s: %v", resolvedPath, err),
			Context: map[string]string{domain.WarningContextPath: resolvedPath, domain.WarningContextCause: "group-writable"},
		}, nil
	default:
		return nil, err
	}
}

// resolveExtendedConfigPath resolves an `extends` entry. Untrusted project
// configs may only import non-symlink paths below their own directory; trusted
// global and explicit configs may import external paths. Preset references pass
// through to the preset registry.
func resolveExtendedConfigPath(parentPath, extendPath string, trusted bool, fsForValidation FileSystem) (string, error) {
	trimmed := strings.TrimSpace(extendPath)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s", ErrConfigEmptyExtendPath, parentPath)
	}

	if IsPresetReference(trimmed) {
		return trimmed, nil
	}

	parentDir := filepath.Dir(parentPath)

	var resolved string
	if filepath.IsAbs(trimmed) {
		resolved = filepath.Clean(trimmed)
	} else {
		resolved = filepath.Clean(filepath.Join(parentDir, trimmed))
	}

	// Confinement applies only to untrusted (project) configs — a hostile
	// `.gommitlint.yaml` in a cloned repo must not reach outside its subtree.
	// Trusted configs (the user's own global ~/.config file, or an explicit
	// --config path) may extend absolute/external paths: that's how an
	// operator points every repo at one central policy file on the host.
	if !trusted {
		if err := ensureExtendPathConfined(parentDir, resolved, extendPath); err != nil {
			return "", err
		}

		if err := ensureExtendPathHasNoSymlinks(parentDir, resolved, extendPath, fsForValidation); err != nil {
			return "", err
		}
	}

	return resolved, nil
}

// isTrustedConfigSource reports whether a config source may extend paths outside
// its own directory subtree. The user's global config and an explicitly passed
// config (--config) are authored or chosen by the operator; a project
// config could come from a cloned repo and therefore stays confined.
func isTrustedConfigSource(sourceType ConfigSourceType) bool {
	return sourceType == SourceTypeGlobal || sourceType == SourceTypeCLI
}

// ensureExtendPathConfined checks that resolved is lexically inside parentDir.
func ensureExtendPathConfined(parentDir, resolved, extendPath string) error {
	parentAbs, err := filepath.Abs(parentDir)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrConfigExtendsEscape, extendPath)
	}

	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrConfigExtendsEscape, extendPath)
	}

	rel, err := filepath.Rel(parentAbs, resolvedAbs)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrConfigExtendsEscape, extendPath)
	}

	if rel == "." || rel == "" || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("%w: %q", ErrConfigExtendsEscape, extendPath)
	}

	return nil
}

// ensureExtendPathHasNoSymlinks prevents a lexically confined import from
// escaping through a symlinked path component. Filesystems that cannot model
// symlinks return ordinary file information from Lstat and preserve their test
// double behavior.
func ensureExtendPathHasNoSymlinks(parentDir, resolved, extendPath string, fs FileSystem) error {
	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	parentAbs, err := filepath.Abs(parentDir)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrConfigExtendsEscape, extendPath)
	}

	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrConfigExtendsEscape, extendPath)
	}

	rel, err := filepath.Rel(parentAbs, resolvedAbs)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrConfigExtendsEscape, extendPath)
	}

	current := parentAbs
	for component := range strings.SplitSeq(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)

		info, lstatErr := fs.Lstat(current)
		if lstatErr != nil {
			if errors.Is(lstatErr, os.ErrNotExist) {
				return nil
			}

			return fmt.Errorf("%w: %q: %w", ErrConfigExtendsEscape, extendPath, lstatErr)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %q traverses symlink %s", ErrConfigExtendsEscape, extendPath, current)
		}
	}

	return nil
}
