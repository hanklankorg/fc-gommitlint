// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var managedSourceSHA256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var (
	managedSourceEnvPattern     = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)
	managedSourceEnvNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ConfigError is a problem found in the user's configuration file.
//
// Named for the config rather than for "validation": in gommitlint's language
// validation means commit validation, and domain.ValidationError is the rule
// violation that produces. Two types called ValidationError, both in the domain
// layer, meant the central term named two unrelated things.
type ConfigError string

// Error returns the configuration problem's message.
func (e ConfigError) Error() string {
	return string(e)
}

// RegexCompileError describes an invalid configured regular expression.
type RegexCompileError struct {
	Pattern string
	Err     error
}

// ValidateConfigErrors checks configuration for errors and returns typed errors.
func ValidateConfigErrors(cfg Config) []ConfigError {
	messages := ValidateConfig(cfg)
	if len(messages) == 0 {
		return nil
	}

	errors := make([]ConfigError, len(messages))
	for i, message := range messages {
		errors[i] = ConfigError(message)
	}

	return errors
}

// ConfigErrorMessages returns the string messages for configuration problems.
func ConfigErrorMessages(errors []ConfigError) []string {
	messages := make([]string, len(errors))
	for i, err := range errors {
		messages[i] = err.Error()
	}

	return messages
}

// ValidateConfig checks configuration for errors.
func ValidateConfig(cfg Config) []string {
	errors := make([]string, 0, 10)

	errors = append(errors, ValidateSubjectConfig(cfg.Message.Subject)...)
	errors = append(errors, ValidateBodyConfig(cfg.Message.Body)...)
	errors = append(errors, ValidateConventionalConfig(cfg.Conventional)...)
	errors = append(errors, ValidateCryptoSignatureConfig(cfg.CryptoSignature)...)
	errors = append(errors, ValidateSpellConfig(cfg.Spell)...)
	errors = append(errors, ValidateRulesConfig(cfg.Rules)...)
	errors = append(errors, ValidateChecksConfig(cfg.Checks)...)
	errors = append(errors, ValidateJiraConfig(cfg.Jira)...)
	errors = append(errors, ValidateIdentityConfig(cfg.Identity)...)
	errors = append(errors, ValidateRepoConfig(cfg.Repo)...)
	errors = append(errors, ValidateLimitsConfig(cfg.Limits)...)
	errors = append(errors, ValidateManagedSources(cfg.ManagedSources)...)

	return errors
}

// ValidateManagedSources checks remote managed source configuration.
func ValidateManagedSources(sources []ManagedSource) []string {
	if len(sources) == 0 {
		return nil
	}

	errors := make([]string, 0, len(sources)*4)
	seenIDs := make(map[string]bool, len(sources))

	for _, source := range sources {
		errors = append(errors, validateManagedSourceID(source, seenIDs)...)
		errors = append(errors, validateManagedSourceURL(source)...)
		errors = append(errors, validateManagedSourceDestination(source)...)
		errors = append(errors, validateManagedSourceIntegrity(source)...)
		errors = append(errors, validateManagedSourceHeaders(source)...)
	}

	return errors
}

// forbiddenManagedSourceHeaders are header names users may not configure.
// Each is set automatically by Go's http client or controls protocol-level
// semantics — overriding them risks request smuggling or breaks the request.
//
// Package-level var because Go has no immutable map literal; entries are
// read-only by convention. Keeping as map (vs slice) for O(1) lookup in the
// per-header validation loop.
//
//nolint:gochecknoglobals // canonical denylist, read-only after init.
var forbiddenManagedSourceHeaders = map[string]struct{}{
	"host":              {},
	"content-length":    {},
	"transfer-encoding": {},
	"connection":        {},
}

func validateManagedSourceHeaders(source ManagedSource) []string {
	if len(source.Headers) == 0 {
		return nil
	}

	var errors []string

	for name := range source.Headers {
		trimmed := strings.TrimSpace(name)
		switch {
		case trimmed == "":
			errors = append(errors, fmt.Sprintf("managed_sources %q has an empty header name", source.ID))
		case containsCRLF(trimmed):
			// CR/LF in a header name is HTTP-smuggling territory. Reject hard.
			errors = append(errors, fmt.Sprintf("managed_sources %q header name %q contains CR or LF", source.ID, trimmed))
		default:
			if _, forbidden := forbiddenManagedSourceHeaders[strings.ToLower(trimmed)]; forbidden {
				errors = append(errors, fmt.Sprintf("managed_sources %q header %q is set automatically by the HTTP client and cannot be overridden", source.ID, trimmed))
			}
		}
	}

	// HTTPS-only when headers are set. URLs use ${ENV_VAR} indirection or a
	// literal URL — only the literal form is checkable here; resolved URLs
	// from env are checked at fetch time.
	if !strings.HasPrefix(source.Source, "${") && source.Source != "" {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(source.Source)), "https://") {
			errors = append(errors, fmt.Sprintf("managed_sources %q has headers configured but source is not https:// (refusing to send headers over plaintext)", source.ID))
		}
	}

	return errors
}

// containsCRLF reports whether s contains an ASCII carriage return or line
// feed. Used to defend against HTTP-smuggling attempts via header names.
func containsCRLF(s string) bool {
	return strings.ContainsAny(s, "\r\n")
}

func validateManagedSourceID(source ManagedSource, seenIDs map[string]bool) []string {
	trimmedID := strings.TrimSpace(source.ID)
	switch {
	case trimmedID == "":
		return []string{"managed_sources contains entry with empty id"}
	case seenIDs[trimmedID]:
		return []string{fmt.Sprintf("managed_sources contains duplicate id %q", source.ID)}
	default:
		seenIDs[trimmedID] = true

		return nil
	}
}

func validateManagedSourceURL(source ManagedSource) []string {
	if _, ok := ParseManagedSourceEnvReference(source.Source); ok {
		return nil
	}

	if !IsValidManagedSourceURL(source.Source) {
		return []string{fmt.Sprintf("managed_sources %q source must be a valid https URL", source.ID)}
	}

	return nil
}

// IsValidManagedSourceURL reports whether rawURL is an HTTPS URL with a host.
func IsValidManagedSourceURL(rawURL string) bool {
	parsedURL, err := url.Parse(strings.TrimSpace(rawURL))

	return err == nil && parsedURL != nil && parsedURL.Scheme == "https" && parsedURL.Host != ""
}

// ParseManagedSourceEnvReference returns the referenced env var name for a ${VAR} source.
func ParseManagedSourceEnvReference(source string) (string, bool) {
	matches := managedSourceEnvPattern.FindStringSubmatch(strings.TrimSpace(source))
	if len(matches) != 2 {
		return "", false
	}

	return matches[1], true
}

// IsValidManagedSourceEnvName reports whether name matches the env-var
// identifier shape we accept inside ${...} placeholders. Same grammar as
// the standalone source-URL form so authors learn one rule, not two.
func IsValidManagedSourceEnvName(name string) bool {
	return managedSourceEnvNamePattern.MatchString(name)
}

func validateManagedSourceDestination(source ManagedSource) []string {
	destination := strings.TrimSpace(source.Destination)
	switch {
	case destination == "":
		return []string{fmt.Sprintf("managed_sources %q destination is required", source.ID)}
	case !IsLocalManagedSourceDestination(destination):
		return []string{fmt.Sprintf("managed_sources %q destination must be a relative local path", source.ID)}
	default:
		return nil
	}
}

// IsLocalManagedSourceDestination reports whether destination is a non-empty relative local path.
func IsLocalManagedSourceDestination(destination string) bool {
	trimmed := strings.TrimSpace(destination)
	if trimmed == "" {
		return false
	}

	return !filepath.IsAbs(trimmed) && !strings.HasPrefix(filepath.Clean(trimmed), "..")
}

func validateManagedSourceIntegrity(source ManagedSource) []string {
	sha := strings.TrimSpace(source.Integrity.SHA256)
	switch {
	case sha == "":
		return []string{fmt.Sprintf("managed_sources %q integrity.sha256 is required", source.ID)}
	case !managedSourceSHA256Pattern.MatchString(sha):
		return []string{fmt.Sprintf("managed_sources %q integrity.sha256 must be a 64-character hex string", source.ID)}
	default:
		return nil
	}
}

// ValidateRulesConfig checks rule enable/disable configuration.
func ValidateRulesConfig(rules RulesConfig) []string {
	errors := make([]string, 0, len(rules.Enabled)+len(rules.Disabled))

	errors = append(errors, validateRuleList("enabled", rules.Enabled)...)
	errors = append(errors, validateRuleList("disabled", rules.Disabled)...)

	return errors
}

func validateRuleList(listName string, rules []string) []string {
	if len(rules) == 0 {
		return nil
	}

	validNames := GetValidRuleNames()
	validList := strings.Join(validNames, ", ")
	errors := make([]string, 0, len(rules))

	for _, rule := range rules {
		cleaned := NormalizeRuleName(rule)
		if cleaned == "" {
			errors = append(errors, fmt.Sprintf("rules %s contains empty rule name", listName))

			continue
		}

		if !IsValidRuleName(cleaned) {
			errors = append(errors, fmt.Sprintf("rules %s contains unknown rule %q (valid: %s)", listName, rule, validList))
		}
	}

	return errors
}

// ValidateSubjectConfig checks subject configuration.
func ValidateSubjectConfig(subject SubjectConfig) []string {
	var errors []string

	maxLength := DerefInt(subject.MaxLength, DefaultSubjectMaxLength)
	if maxLength <= 0 {
		errors = append(errors, "subject max_length must be positive")
	}

	// Case is an enum (see GetValidSubjectCases). Without this check, unknown
	// values (e.g. "ROBOT_MODE") were silently accepted and the rule then fell
	// through to a default that the user couldn't predict. Matches the
	// spell-locale validation pattern.
	caseValue := DerefString(subject.Case, DefaultSubjectCase)
	if caseValue != "" && !IsValidSubjectCase(caseValue) {
		validCases := GetValidSubjectCases()
		errors = append(errors, "subject case must be one of: "+strings.Join(validCases, ", "))
	}

	return errors
}

// ValidateBodyConfig checks body configuration.
func ValidateBodyConfig(body BodyConfig) []string {
	var errors []string

	maxLineLength := DerefInt(body.MaxLineLength, 0)
	if maxLineLength < 0 {
		errors = append(errors, "body max_line_length cannot be negative")
	}

	if maxLineLength > 200 {
		errors = append(errors, "body max_line_length cannot exceed 200")
	}

	minLength := DerefInt(body.MinLength, 0)
	if minLength < 0 {
		errors = append(errors, "body min_length cannot be negative")
	}

	if minLength > 1000 {
		errors = append(errors, "body min_length cannot exceed 1000")
	}

	minSignoffCount := DerefInt(body.MinSignoffCount, DefaultMinSignoffCount)
	if minSignoffCount < MinSignoffCount {
		errors = append(errors, "body min_signoff_count cannot be negative")
	}

	if minSignoffCount > MaxSignoffCount {
		errors = append(errors, "body min_signoff_count cannot exceed 10")
	}

	return errors
}

// ValidateConventionalConfig checks conventional commit configuration.
func ValidateConventionalConfig(conventional ConventionalConfig) []string {
	var errors []string

	for _, commitType := range conventional.Types {
		if !isValidConventionalType(commitType) {
			errors = append(errors, fmt.Sprintf("conventional types contains invalid type %q (must contain only lowercase ASCII letters)", commitType))
		}
	}

	maxDescLength := DerefInt(conventional.MaxDescriptionLength, DefaultConventionalMaxDescLength)
	if maxDescLength < 0 {
		errors = append(errors, "conventional max_description_length cannot be negative")
	}

	if maxDescLength > 200 {
		errors = append(errors, "conventional max_description_length cannot exceed 200")
	}

	return errors
}

func isValidConventionalType(commitType string) bool {
	if commitType == "" {
		return false
	}

	for index := range len(commitType) {
		if commitType[index] < 'a' || commitType[index] > 'z' {
			return false
		}
	}

	return true
}

// ValidateCryptoSignatureConfig checks cryptographic signature configuration.
func ValidateCryptoSignatureConfig(crypto CryptoSignatureConfig) []string {
	var errors []string

	minRSABits := DerefInt(crypto.MinRSABits, DefaultMinRSABits)
	if minRSABits < 0 {
		errors = append(errors, "cryptosignature min_rsa_bits cannot be negative")
	}

	if minRSABits > 8192 {
		errors = append(errors, "cryptosignature min_rsa_bits cannot exceed 8192")
	}

	minECBits := DerefInt(crypto.MinECBits, DefaultMinECBits)
	if minECBits < 0 {
		errors = append(errors, "cryptosignature min_ec_bits cannot be negative")
	}

	if minECBits > 521 {
		errors = append(errors, "cryptosignature min_ec_bits cannot exceed 521")
	}

	if HasConflictingSignatureRequirements(DerefBool(crypto.RequireGPG, false), DerefBool(crypto.RequireSSH, false)) {
		errors = append(errors, "cryptosignature require_gpg and require_ssh are mutually exclusive")
	}

	return errors
}

// HasConflictingSignatureRequirements reports whether GPG and SSH requirements are both enabled.
func HasConflictingSignatureRequirements(requireGPG, requireSSH bool) bool {
	return requireGPG && requireSSH
}

// ValidateSpellConfig checks spell configuration for valid locale values.
func ValidateSpellConfig(spell SpellConfig) []string {
	var errors []string

	locale := DerefString(spell.Locale, DefaultSpellLocale)
	if locale != "" && !IsValidSpellLocale(locale) {
		validLocales := GetValidSpellLocales()
		errors = append(errors, "spell locale must be one of: "+strings.Join(validLocales, ", "))
	}

	return errors
}

// ValidateJiraConfig checks JIRA configuration for invalid values.
func ValidateJiraConfig(jira JiraConfig) []string {
	errors := make([]string, 0, len(jira.IgnoreTicketPatterns)+len(jira.ProjectPrefixes))

	_, patternErrors := CompileJiraIgnorePatterns(jira.IgnoreTicketPatterns)
	for _, patternErr := range patternErrors {
		errors = append(errors, fmt.Sprintf("jira ignore_ticket_patterns contains invalid regex %q: %s", patternErr.Pattern, patternErr.Err.Error()))
	}

	for _, prefix := range jira.ProjectPrefixes {
		trimmed := strings.TrimSpace(prefix)
		if trimmed == "" {
			errors = append(errors, "jira project_prefixes contains empty prefix")

			continue
		}

		if !isValidJiraProjectPrefix(trimmed) {
			errors = append(errors, fmt.Sprintf("jira project_prefixes contains invalid prefix %q", prefix))
		}
	}

	return errors
}

// CompileJiraIgnorePatterns compiles JIRA ignore-ticket regex patterns.
func CompileJiraIgnorePatterns(patterns []string) ([]*regexp.Regexp, []RegexCompileError) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	errors := make([]RegexCompileError, 0)

	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			errors = append(errors, RegexCompileError{Pattern: pattern, Err: err})

			continue
		}

		compiled = append(compiled, regex)
	}

	return compiled, errors
}

// IdentityAuthorError represents invalid allowed author entries.
type IdentityAuthorError struct {
	Message      string
	InvalidEntry string
	ValidFormats string
}

// ValidateIdentityAuthorEntry validates allowed author entries and normalizes them.
func ValidateIdentityAuthorEntry(author string) (string, *IdentityAuthorError) {
	trimmed := strings.TrimSpace(author)
	if trimmed == "" {
		return "", nil
	}

	const validFormats = "email@domain.com or *@domain.com"

	if strings.Contains(trimmed, "<") || strings.Contains(trimmed, ">") {
		return "", &IdentityAuthorError{
			Message:      "Invalid allowed_authors format: use email only, not 'Name <email>'",
			InvalidEntry: author,
			ValidFormats: validFormats,
		}
	}

	if !strings.Contains(trimmed, "@") {
		return "", &IdentityAuthorError{
			Message:      "Invalid allowed_authors entry: must be an email address",
			InvalidEntry: author,
			ValidFormats: validFormats,
		}
	}

	return strings.ToLower(trimmed), nil
}

// ValidateIdentityConfig checks identity configuration for invalid allowed authors.
func ValidateIdentityConfig(identity IdentityConfig) []string {
	errors := make([]string, 0, len(identity.AllowedAuthors))

	for _, author := range identity.AllowedAuthors {
		_, err := ValidateIdentityAuthorEntry(author)
		if err != nil {
			errors = append(errors, fmt.Sprintf("identity allowed_authors contains invalid entry %q: %s", err.InvalidEntry, err.Message))
		}
	}

	return errors
}

// ValidateRepoConfig checks repository configuration values.
func ValidateRepoConfig(repo RepoConfig) []string {
	var errors []string

	if repo.MaxCommitsAhead != nil {
		if *repo.MaxCommitsAhead <= 0 {
			errors = append(errors, "repo max_commits_ahead must be positive")
		} else if *repo.MaxCommitsAhead > 1000 {
			errors = append(errors, "repo max_commits_ahead cannot exceed 1000")
		}
	}

	return errors
}

func isValidJiraProjectPrefix(prefix string) bool {
	matched, err := regexp.MatchString("^[A-Z]+$", prefix)
	if err != nil {
		return false
	}

	return matched
}

// ValidateLimitsConfig checks global validation limits.
func ValidateLimitsConfig(limits LimitsConfig) []string {
	var errors []string

	if limits.MaxCommitCount != nil && *limits.MaxCommitCount <= 0 {
		errors = append(errors, "limits max_commit_count must be positive")
	}

	if limits.MaxKeyFiles != nil && *limits.MaxKeyFiles <= 0 {
		errors = append(errors, "limits max_key_files must be positive")
	}

	return errors
}

// IsValidSpellLocale checks if the locale is valid for spell checking.
func IsValidSpellLocale(locale string) bool {
	validLocales := GetValidSpellLocales()
	normalizedLocale := strings.ToLower(locale)

	for _, valid := range validLocales {
		if normalizedLocale == strings.ToLower(valid) {
			return true
		}
	}

	return false
}
