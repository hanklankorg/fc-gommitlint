// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

import (
	"fmt"
	"net/url"
	"slices"
)

const redactedConfigValue = "[REDACTED]"

// DerefBool safely dereferences a bool pointer, returning defaultVal if nil.
func DerefBool(p *bool, defaultVal bool) bool {
	if p == nil {
		return defaultVal
	}

	return *p
}

// DerefInt safely dereferences an int pointer, returning defaultVal if nil.
func DerefInt(p *int, defaultVal int) int {
	if p == nil {
		return defaultVal
	}

	return *p
}

// DerefPositiveInt dereferences an int pointer and enforces a positive value;
// returns defaultVal when the pointer is nil or the stored value is <= 0.
func DerefPositiveInt(p *int, defaultVal int) int {
	v := DerefInt(p, defaultVal)
	if v <= 0 {
		return defaultVal
	}

	return v
}

// DerefString safely dereferences a string pointer, returning defaultVal if nil.
func DerefString(p *string, defaultVal string) string {
	if p == nil {
		return defaultVal
	}

	return *p
}

// Config is the complete gommitlint configuration as loaded from a config file.
type Config struct {
	// Allow fixup/squash commits ("fixup! …", "squash! …") to pass linting silently.
	// Default: false (these commits fail with a clear error).
	AllowFixupCommits *bool                 `json:"allow_fixup_commits" toml:"allow_fixup_commits" yaml:"allow_fixup_commits"`
	Checks            ChecksConfig          `json:"checks"              toml:"checks"              yaml:"checks"`
	Conventional      ConventionalConfig    `json:"conventional"        toml:"conventional"        yaml:"conventional"`
	CryptoSignature   CryptoSignatureConfig `json:"crypto_signature"    toml:"crypto_signature"    yaml:"crypto_signature"`
	// Inherit settings from other config files or built-in presets. Each entry
	// is a local file path or a `preset:<name>` reference, resolved relative to
	// this file; later entries override earlier ones. A project config stays
	// confined to its own directory — only a trusted global or --config
	// config may use an absolute path. Schema validates the list shape only —
	// `gommitlint config check` verifies the merged result.
	Extends  []string       `json:"extends"  toml:"extends"  yaml:"extends"`
	Identity IdentityConfig `json:"identity" toml:"identity" yaml:"identity"`
	// Skip validation for specific commits, identified by a 7-64 character
	// SHA-1 or SHA-256 object ID prefix. Example: ["a1b2c3d4e5…"]. Useful
	// for one-off legacy commits that predate the current rules.
	IgnoreCommits []string   `json:"ignore_commits" toml:"ignore_commits" yaml:"ignore_commits"`
	Jira          JiraConfig `json:"jira"           toml:"jira"           yaml:"jira"`
	// Remote-managed configuration sources fetched and vendored by `gommitlint
	// config sync`. Schema validates each entry's literal fields (URL shape,
	// integrity); `gommitlint config check` verifies the resolved file is
	// well-formed.
	ManagedSources []ManagedSource `json:"managed_sources" toml:"managed_sources" yaml:"managed_sources"`
	Message        MessageConfig   `json:"message"         toml:"message"         yaml:"message"`
	// Default output format. Run `gommitlint config show` to see supported values.
	Output string `json:"output" toml:"output" yaml:"output"`
	// Default output theme. Run `gommitlint config show` to see supported values.
	Theme  string       `json:"theme"  toml:"theme"  yaml:"theme"`
	Repo   RepoConfig   `json:"repo"   toml:"repo"   yaml:"repo"`
	Rules  RulesConfig  `json:"rules"  toml:"rules"  yaml:"rules"`
	Spell  SpellConfig  `json:"spell"  toml:"spell"  yaml:"spell"`
	Limits LimitsConfig `json:"limits" toml:"limits" yaml:"limits"`
}

// ManagedSource is one remote configuration file synced by `gommitlint config sync` — schema validates the literal fields below, `gommitlint config check` reports problems with the resolved file.
type ManagedSource struct {
	// Local path where the synced file is written, relative to this config file.
	Destination string `json:"destination" toml:"destination" yaml:"destination"`
	// HTTP headers sent with the fetch request. Values support ${ENV_VAR}
	// substitution so secrets stay out of the file. When set, the source URL
	// must be https://. Literal values remain configuration; environment-expanded
	// values are not persisted or displayed, and config show redacts literals.
	Headers map[string]string `json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty"`
	// Stable identifier used by `gommitlint config sync --source <id>`.
	ID        string                 `json:"id"        toml:"id"        yaml:"id"`
	Integrity ManagedSourceIntegrity `json:"integrity" toml:"integrity" yaml:"integrity"`
	// Tool-managed sync metadata. Do not edit by hand — `gommitlint config
	// sync` updates this block when content or resolved source metadata changes.
	Resolved ManagedSourceResolved `json:"resolved" toml:"resolved" yaml:"resolved"`
	// HTTPS URL or ${ENV_VAR} resolving to one.
	Source string `json:"source" toml:"source" yaml:"source"`
}

// ManagedSourceIntegrity pins the expected content of a synced source.
// `gommitlint config sync` refuses to write a file whose hash does not match.
type ManagedSourceIntegrity struct {
	// Lowercase hex-encoded SHA-256 of the fetched file. Required.
	SHA256 string `json:"sha256" toml:"sha256" yaml:"sha256"`
}

// ManagedSourceResolved holds tool-managed metadata written by `gommitlint
// config sync`. Do not edit by hand.
type ManagedSourceResolved struct {
	// RFC 3339 timestamp of the last persisted sync update.
	FetchedAt string `json:"fetched_at" toml:"fetched_at" yaml:"fetched_at"`
	// Final URL after redirects.
	FinalURL string `json:"final_url" toml:"final_url" yaml:"final_url"`
	// Upstream revision (commit SHA, tag, etc.) if the source advertises one.
	Revision string `json:"revision" toml:"revision" yaml:"revision"`
}

// Sanitized returns a copy of the config with sensitive fields masked.
// This is used for display purposes to prevent accidental exposure of secrets.
func (c Config) Sanitized() Config {
	sanitized := c
	sanitized.AllowFixupCommits = clonePointer(c.AllowFixupCommits)
	sanitized.Checks.Disabled = slices.Clone(c.Checks.Disabled)
	sanitized.Checks.Enabled = slices.Clone(c.Checks.Enabled)
	sanitized.Conventional.AllowMultiScope = clonePointer(c.Conventional.AllowMultiScope)
	sanitized.Conventional.DisallowBreaking = clonePointer(c.Conventional.DisallowBreaking)
	sanitized.Conventional.MaxDescriptionLength = clonePointer(c.Conventional.MaxDescriptionLength)
	sanitized.Conventional.RequireScope = clonePointer(c.Conventional.RequireScope)
	sanitized.Conventional.Scopes = slices.Clone(c.Conventional.Scopes)
	sanitized.Conventional.Types = slices.Clone(c.Conventional.Types)
	sanitized.CryptoSignature.AllowedSigners = slices.Clone(c.CryptoSignature.AllowedSigners)
	sanitized.CryptoSignature.MinECBits = clonePointer(c.CryptoSignature.MinECBits)
	sanitized.CryptoSignature.MinRSABits = clonePointer(c.CryptoSignature.MinRSABits)
	sanitized.CryptoSignature.Required = clonePointer(c.CryptoSignature.Required)
	sanitized.CryptoSignature.RequireGPG = clonePointer(c.CryptoSignature.RequireGPG)
	sanitized.CryptoSignature.RequireSSH = clonePointer(c.CryptoSignature.RequireSSH)
	sanitized.CryptoSignature.VerifyFormat = clonePointer(c.CryptoSignature.VerifyFormat)
	sanitized.Extends = slices.Clone(c.Extends)
	sanitized.Identity.AllowedAuthors = slices.Clone(c.Identity.AllowedAuthors)
	sanitized.IgnoreCommits = slices.Clone(c.IgnoreCommits)
	sanitized.Jira.IgnoreTicketPatterns = slices.Clone(c.Jira.IgnoreTicketPatterns)
	sanitized.Jira.ProjectPrefixes = slices.Clone(c.Jira.ProjectPrefixes)
	sanitized.Jira.RequireInBody = clonePointer(c.Jira.RequireInBody)
	sanitized.Jira.RequireInSubject = clonePointer(c.Jira.RequireInSubject)
	sanitized.Limits.MaxCommitCount = clonePointer(c.Limits.MaxCommitCount)
	sanitized.Limits.MaxKeyFiles = clonePointer(c.Limits.MaxKeyFiles)
	sanitized.Message.Body.AllowSignoffOnly = clonePointer(c.Message.Body.AllowSignoffOnly)
	sanitized.Message.Body.MaxLineLength = clonePointer(c.Message.Body.MaxLineLength)
	sanitized.Message.Body.MinLength = clonePointer(c.Message.Body.MinLength)
	sanitized.Message.Body.MinSignoffCount = clonePointer(c.Message.Body.MinSignoffCount)
	sanitized.Message.Body.Required = clonePointer(c.Message.Body.Required)
	sanitized.Message.Subject.Case = clonePointer(c.Message.Subject.Case)
	sanitized.Message.Subject.ForbidEndings = slices.Clone(c.Message.Subject.ForbidEndings)
	sanitized.Message.Subject.MaxLength = clonePointer(c.Message.Subject.MaxLength)
	sanitized.Message.Subject.RequireImperative = clonePointer(c.Message.Subject.RequireImperative)
	sanitized.Repo.MaxCommitsAhead = clonePointer(c.Repo.MaxCommitsAhead)
	sanitized.Rules.Disabled = slices.Clone(c.Rules.Disabled)
	sanitized.Rules.Enabled = slices.Clone(c.Rules.Enabled)
	sanitized.Spell.IgnoreWords = slices.Clone(c.Spell.IgnoreWords)
	sanitized.Spell.Locale = clonePointer(c.Spell.Locale)
	sanitized.ManagedSources = make([]ManagedSource, len(c.ManagedSources))

	for index, source := range c.ManagedSources {
		sanitized.ManagedSources[index] = source
		if _, placeholder := ParseManagedSourceEnvReference(source.Source); !placeholder {
			sanitized.ManagedSources[index].Source = SanitizeManagedSourceURL(source.Source)
		}

		sanitized.ManagedSources[index].Resolved.FinalURL = SanitizeManagedSourceURL(source.Resolved.FinalURL)
		sanitized.ManagedSources[index].Headers = make(map[string]string, len(source.Headers))

		for name, value := range source.Headers {
			if _, placeholder := ParseManagedSourceEnvReference(value); placeholder {
				sanitized.ManagedSources[index].Headers[name] = value
			} else {
				sanitized.ManagedSources[index].Headers[name] = redactedConfigValue
			}
		}
	}

	// Mask any overly long ignore commits (could be sensitive SHAs)
	if len(sanitized.IgnoreCommits) > 10 {
		masked := make([]string, 0, 11)
		masked = append(masked, sanitized.IgnoreCommits[:5]...)
		masked = append(masked, fmt.Sprintf("... %d more ...", len(sanitized.IgnoreCommits)-10))
		masked = append(masked, sanitized.IgnoreCommits[len(sanitized.IgnoreCommits)-5:]...)
		sanitized.IgnoreCommits = masked
	}

	return sanitized
}

// SanitizeManagedSourceURL removes URL metadata that may contain secrets.
// Values other than absolute HTTPS URLs with a host are replaced with the
// stable config redaction marker.
func SanitizeManagedSourceURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" {
		return redactedConfigValue
	}

	return (&url.URL{
		Scheme:  parsed.Scheme,
		Host:    parsed.Host,
		Path:    parsed.Path,
		RawPath: parsed.RawPath,
	}).String()
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

// Sorted returns a copy of the config with all list fields sorted alphabetically.
// This is useful for consistent output in config files and displays.
func (c Config) Sorted() Config {
	sorted := c

	// Sort all string slice fields alphabetically
	sorted.Message.Subject.ForbidEndings = sortedCopy(c.Message.Subject.ForbidEndings)
	sorted.Conventional.Types = sortedCopy(c.Conventional.Types)
	sorted.Conventional.Scopes = sortedCopy(c.Conventional.Scopes)
	sorted.CryptoSignature.AllowedSigners = sortedCopy(c.CryptoSignature.AllowedSigners)
	sorted.Identity.AllowedAuthors = sortedCopy(c.Identity.AllowedAuthors)
	sorted.Jira.ProjectPrefixes = sortedCopy(c.Jira.ProjectPrefixes)
	sorted.Jira.IgnoreTicketPatterns = sortedCopy(c.Jira.IgnoreTicketPatterns)
	sorted.Spell.IgnoreWords = sortedCopy(c.Spell.IgnoreWords)
	sorted.Checks.Enabled = sortedCopy(c.Checks.Enabled)
	sorted.Checks.Disabled = sortedCopy(c.Checks.Disabled)
	sorted.Rules.Enabled = sortedCopy(c.Rules.Enabled)
	sorted.Rules.Disabled = sortedCopy(c.Rules.Disabled)
	sorted.IgnoreCommits = sortedCopy(c.IgnoreCommits)

	return sorted
}

// sortedCopy returns a sorted copy of the input slice.
// Returns nil for empty or nil input to avoid returning shared references.
func sortedCopy(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	result := make([]string, len(input))
	copy(result, input)
	slices.Sort(result)

	return result
}

// ChecksConfig contains configuration for public GLxxx check activation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type ChecksConfig struct {
	// Disabled specifies GLxxx checks to deactivate.
	// Takes precedence over Enabled list: a check in both lists is disabled.
	Disabled []string `json:"disabled" toml:"disabled" yaml:"disabled"`
	// Enabled specifies GLxxx checks to activate beyond their parent rule defaults.
	// Enabling a check also activates its parent rule unless rules.disabled vetoes it.
	Enabled []string `json:"enabled" toml:"enabled" yaml:"enabled"`
}

// MessageConfig contains configuration for commit message validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type MessageConfig struct {
	Body    BodyConfig    `json:"body"    toml:"body"    yaml:"body"`
	Subject SubjectConfig `json:"subject" toml:"subject" yaml:"subject"`
}

// SubjectConfig contains configuration options for commit subject validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type SubjectConfig struct {
	// Case selects subject capitalization validation.
	//
	//   - "lower", "upper", "sentence" enforce that specific case.
	//   - "auto" (the product default) enforces lower case for conventional
	//     descriptions and sentence case for non-conventional subjects.
	//   - "any" disables the check, equivalent to listing GL104 in
	//     checks.disabled.
	Case *string `json:"case" toml:"case" yaml:"case"`
	// ForbidEndings prevents specified suffixes in subject lines.
	// Each entry contributes all characters to the forbidden set.
	// Default: [".", ",", ";", ":", "!", "?"] (from GetDefaultForbiddenEndings).
	// Example: [".", "!?", "?"] forbids '.', '!', and '?' endings.
	ForbidEndings []string `json:"forbid_endings" toml:"forbid_endings" yaml:"forbid_endings"`
	// Positive character limit for commit subject lines (default: 100).
	MaxLength *int `json:"max_length" toml:"max_length" yaml:"max_length"`
	// RequireImperative enforces imperative mood in subject.
	// The product default is true. Example: "Add feature" rather than "Added feature".
	RequireImperative *bool `json:"require_imperative" toml:"require_imperative" yaml:"require_imperative"`
}

// BodyConfig contains configuration options for commit body validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type BodyConfig struct {
	// AllowSignoffOnly permits body containing only sign-off trailers.
	// Default: false (requires descriptive content before sign-offs when body is required).
	// Example: "Signed-off-by: Author <email>"
	AllowSignoffOnly *bool `json:"allow_signoff_only" toml:"allow_signoff_only" yaml:"allow_signoff_only"`
	// MaxLineLength enforces maximum line width in body.
	// Range: 0-200, Default: 72. Set 0 to disable.
	// Git convention: 72 chars (80 terminal - 4 indent - 4 padding)
	// Lines holding a single unbreakable token (for example a URL) and
	// identity trailers such as "Signed-off-by: Name <address>" are exempt.
	MaxLineLength *int `json:"max_line_length" toml:"max_line_length" yaml:"max_line_length"`
	// Minimum character count for commit body (0-1000, default: 0 = disabled)
	MinLength *int `json:"min_length" toml:"min_length" yaml:"min_length"`
	// MinSignoffCount requires minimum number of sign-off trailers.
	// Range: 0-10, Default: 1, For multi-reviewer workflows
	MinSignoffCount *int `json:"min_signoff_count" jsonschema:"minimum=0,maximum=10" toml:"min_signoff_count" yaml:"min_signoff_count"`
	// Required mandates presence of commit body beyond subject.
	// Default: false, Useful for detailed change documentation
	Required *bool `json:"required" toml:"required" yaml:"required"`
}

// ConventionalConfig contains configuration options for conventional commit format validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type ConventionalConfig struct {
	// AllowMultiScope permits multiple scopes in conventional commits.
	// Default: false (multi-scope not allowed), Example: "feat(ui,api): add feature"
	AllowMultiScope *bool `json:"allow_multi_scope" toml:"allow_multi_scope" yaml:"allow_multi_scope"`
	// DisallowBreaking rejects breaking change indicator (!) in commits.
	// Default: false (breaking changes allowed), Example: "feat!: breaking change"
	DisallowBreaking *bool `json:"disallow_breaking" toml:"disallow_breaking" yaml:"disallow_breaking"`
	// MaxDescriptionLength limits description part of conventional commits.
	// Range: 1-200; 0 uses the default of 72. Applied after type(scope): prefix.
	MaxDescriptionLength *int `json:"max_description_length" toml:"max_description_length" yaml:"max_description_length"`
	// RequireScope mandates scope presence in conventional commits.
	// Default: false, Example: "feat(api): add endpoint" vs "feat: add endpoint"
	RequireScope *bool `json:"require_scope" toml:"require_scope" yaml:"require_scope"`
	// Scopes defines valid scope values for conventional commits.
	// Empty list allows any scope, Case-sensitive matching
	// Example: ["api", "ui", "docs"] requires "feat(api): ..."
	Scopes []string `json:"scopes" toml:"scopes" yaml:"scopes"`
	// Types restricts allowed conventional commit types.
	// Default: ["build", "chore", "ci", "docs", "feat", "fix", "perf", "refactor", "revert", "style", "test"]
	// Custom types must contain only lowercase ASCII letters.
	Types []string `json:"types" toml:"types" yaml:"types"`
}

// CryptoSignatureConfig contains signature metadata and key-inventory policy.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type CryptoSignatureConfig struct {
	// AllowedSigners restricts eligible key metadata in KeyDirectory.
	// Empty list allows any eligible key. Matches exact key fingerprints, emails, or user IDs.
	// This does not identify or authorize the signer of a commit.
	AllowedSigners []string `json:"allowed_signers" toml:"allowed_signers" yaml:"allowed_signers"`
	// KeyDirectory specifies public key files for independent inventory policy.
	// Default: "". If empty, key policy is skipped but Required and VerifyFormat still apply.
	KeyDirectory string `json:"key_directory" toml:"key_directory" yaml:"key_directory"`
	// MinECBits sets minimum elliptic curve key length requirement.
	// Range: 1-521; 0 uses the default of 256. Common values: 256, 384, 521.
	MinECBits *int `json:"min_ec_bits" toml:"min_ec_bits" yaml:"min_ec_bits"`
	// MinRSABits sets minimum RSA key length requirement.
	// Range: 1-8192; 0 uses the default of 2048. Values below 2048 are weak.
	MinRSABits *int `json:"min_rsa_bits" toml:"min_rsa_bits" yaml:"min_rsa_bits"`
	// Required mandates a GPG or SSH signature block on all commits.
	// Default: false. This checks presence, not cryptographic validity.
	Required *bool `json:"required" toml:"required" yaml:"required"`
	// RequireGPG mandates GPG signatures (excludes SSH signatures).
	// Default: false, Mutually exclusive with RequireSSH
	RequireGPG *bool `json:"require_gpg" toml:"require_gpg" yaml:"require_gpg"`
	// RequireSSH mandates SSH signatures (excludes GPG signatures).
	// Default: false, Mutually exclusive with RequireGPG
	RequireSSH *bool `json:"require_ssh" toml:"require_ssh" yaml:"require_ssh"`
	// VerifyFormat validates signature format structure.
	// Default: false, Checks signature has proper BEGIN/END markers
	VerifyFormat *bool `json:"verify_format" toml:"verify_format" yaml:"verify_format"`
}

// IdentityConfig contains configuration options for commit author identity validation.
type IdentityConfig struct {
	// AllowedAuthors restricts commits to specific author email addresses.
	// Empty list allows any author. Matching is case-insensitive.
	// Supports domain wildcards: "*@company.com" matches any email from that domain.
	// Example: ["john@company.com", "*@company.com"]
	AllowedAuthors []string `json:"allowed_authors" toml:"allowed_authors" yaml:"allowed_authors"`
}

// RepoConfig contains configuration options for repository-level validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type RepoConfig struct {
	// MaxCommitsAhead limits how many commits the current branch can be ahead.
	// Range: 1-1000, Default: 10, Prevents large feature branches
	// To disable this rule, add "branchahead" to rules.disabled list
	MaxCommitsAhead *int `json:"max_commits_ahead" toml:"max_commits_ahead" yaml:"max_commits_ahead"`
}

// JiraConfig contains configuration options for JIRA reference validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type JiraConfig struct {
	// IgnoreTicketPatterns excludes JIRA tickets matching regex patterns from validation.
	// Matched tickets are filtered out before checking requirements.
	// Example: ["^TEMP-.*", "^TEST-.*"] ignores TEMP-123, TEST-456
	IgnoreTicketPatterns []string `json:"ignore_ticket_patterns" toml:"ignore_ticket_patterns" yaml:"ignore_ticket_patterns"`
	// ProjectPrefixes defines valid JIRA project prefixes for ticket references.
	// Example: ["PROJ", "DEV"] matches "PROJ-123", "DEV-456"
	// Case-sensitive matching, typically uppercase. When set, only a key with
	// a listed prefix counts as a reference.
	ProjectPrefixes []string `json:"project_prefixes" toml:"project_prefixes" yaml:"project_prefixes"`
	// RequireInBody mandates JIRA ticket reference in commit body.
	// Default: false, Useful for detailed change tracking
	RequireInBody *bool `json:"require_in_body" toml:"require_in_body" yaml:"require_in_body"`
	// RequireInSubject mandates JIRA ticket reference in commit subject.
	// Default: false, Example: "feat: add feature PROJ-123"
	RequireInSubject *bool `json:"require_in_subject" toml:"require_in_subject" yaml:"require_in_subject"`
}

// SpellConfig contains configuration options for spell checking.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type SpellConfig struct {
	// IgnoreWords defines custom words to skip in spell checking.
	// Case-insensitive matching, Common: ["api", "url", "json"]
	IgnoreWords []string `json:"ignore_words" toml:"ignore_words" yaml:"ignore_words"`
	// Locale specifies language/region for spell checking dictionary.
	// Valid values (case-insensitive): "both", "US", "UK", "GB", or empty for "both".
	// Default: "both" — accept US and UK spellings without flagging cross-variant words.
	Locale *string `json:"locale" toml:"locale" yaml:"locale"`
}

// LimitsConfig contains global guardrails for validation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type LimitsConfig struct {
	// MaxCommitCount limits how many commits can be validated in one run.
	// Default: 2000
	MaxCommitCount *int `json:"max_commit_count" toml:"max_commit_count" yaml:"max_commit_count"`
	// MaxKeyFiles limits how many key files can be loaded from a directory.
	// Default: 200
	MaxKeyFiles *int `json:"max_key_files" toml:"max_key_files" yaml:"max_key_files"`
}

// RulesConfig contains configuration for rule activation.
// Fields are ordered alphabetically for consistent YAML/JSON output.
type RulesConfig struct {
	// Disabled specifies rules to deactivate.
	// Takes precedence over Enabled list: a rule in both lists is disabled.
	Disabled []string `json:"disabled" toml:"disabled" yaml:"disabled"`
	// Enabled specifies rules to activate beyond defaults.
	// Rule names: "subject", "conventional", "cryptosignature", etc.
	Enabled []string `json:"enabled" toml:"enabled" yaml:"enabled"`
}
