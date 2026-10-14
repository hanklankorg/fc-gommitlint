// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"slices"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// ConfigProvenanceEntry describes the source for one effective config value.
type ConfigProvenanceEntry struct {
	Overrides []ConfigProvenanceOverride `json:"overrides,omitempty" yaml:"overrides,omitempty"`
	Path      string                     `json:"path"                yaml:"path"`
	Reason    string                     `json:"reason,omitempty"    yaml:"reason,omitempty"`
	Source    string                     `json:"source"              yaml:"source"`
	Value     string                     `json:"value"               yaml:"value"`
}

// ConfigProvenanceOverride describes an overridden value and source.
type ConfigProvenanceOverride struct {
	Source string `json:"source" yaml:"source"`
	Value  string `json:"value"  yaml:"value"`
}

const (
	provenanceRulePrefix    = "rule."
	provenanceValueDisabled = "disabled"
	provenanceValueEnabled  = "enabled"
)

type provenanceExtractor func(configTypes.Config) (string, bool)

type provenanceField struct {
	path      string
	explicit  bool
	extractor provenanceExtractor
}

// Typed builders below keep getProvenanceFields() readable: each entry is one line.
// The extractor reports (value, true) when the user set the field and ("", false) when
// it remains at its zero/nil default.

func ptrIntField(path string, get func(configTypes.Config) *int) provenanceField {
	return provenanceField{path: path, explicit: true, extractor: func(c configTypes.Config) (string, bool) {
		if v := get(c); v != nil {
			return strconv.Itoa(*v), true
		}

		return "", false
	}}
}

func ptrBoolField(path string, get func(configTypes.Config) *bool) provenanceField {
	return provenanceField{path: path, explicit: true, extractor: func(c configTypes.Config) (string, bool) {
		if v := get(c); v != nil {
			return strconv.FormatBool(*v), true
		}

		return "", false
	}}
}

func ptrStringField(path string, get func(configTypes.Config) *string) provenanceField {
	return provenanceField{path: path, explicit: true, extractor: func(c configTypes.Config) (string, bool) {
		if v := get(c); v != nil {
			return *v, true
		}

		return "", false
	}}
}

func stringField(path string, get func(configTypes.Config) string) provenanceField {
	return provenanceField{path: path, explicit: true, extractor: func(c configTypes.Config) (string, bool) {
		if v := get(c); v != "" {
			return v, true
		}

		return "", false
	}}
}

func stringSliceField(path string, get func(configTypes.Config) []string) provenanceField {
	return provenanceField{path: path, explicit: true, extractor: func(c configTypes.Config) (string, bool) {
		if v := get(c); v != nil {
			return formatStringSlice(v), true
		}

		return "", false
	}}
}

// InspectConfigProvenance reports where selected effective config values came from.
func InspectConfigProvenance(sources []ConfigSource, fs FileSystem) []ConfigProvenanceEntry {
	if len(sources) == 0 {
		return nil
	}

	fields := getProvenanceFields()
	historyByPath := make(map[string][]ConfigProvenanceOverride, len(fields))

	for _, source := range sources {
		cfg, ok := loadProvenanceConfig(source, fs)
		if !ok {
			continue
		}

		for _, field := range fields {
			value, ok := field.extractor(cfg)
			if !ok {
				continue
			}

			historyByPath[field.path] = append(historyByPath[field.path], ConfigProvenanceOverride{Source: source.Path, Value: value})
		}
	}

	entries := make([]ConfigProvenanceEntry, 0, len(historyByPath))
	for _, field := range fields {
		history, ok := historyByPath[field.path]
		if ok {
			entries = append(entries, buildProvenanceEntry(field.path, history))
		}
	}

	entries = append(entries, inspectRuleProvenance(sources, fs)...)

	return entries
}

func inspectRuleProvenance(sources []ConfigSource, fs FileSystem) []ConfigProvenanceEntry {
	// nosemgrep: trailofbits.go.iterate-over-empty-map.iterate-over-empty-map -- ruleHistory is populated by appendDefault/appendConfigured below; the empty case is guarded before iteration.
	ruleHistory := make(map[string][]ConfigProvenanceOverride)

	for _, source := range sources {
		if source.Type == SourceTypeDefaults {
			appendDefaultRuleProvenance(ruleHistory, source)

			continue
		}

		cfg, ok := loadProvenanceConfig(source, fs)
		if !ok {
			continue
		}

		appendConfiguredRuleProvenance(ruleHistory, source, cfg)
	}

	if len(ruleHistory) == 0 {
		return nil
	}

	ruleNames := make([]string, 0, len(ruleHistory))
	for rule := range ruleHistory {
		ruleNames = append(ruleNames, rule)
	}

	slices.Sort(ruleNames)

	entries := make([]ConfigProvenanceEntry, 0, len(ruleNames))
	for _, rule := range ruleNames {
		entries = append(entries, buildProvenanceEntry(provenanceRulePrefix+rule, ruleHistory[rule]))
	}

	return entries
}

func appendDefaultRuleProvenance(ruleHistory map[string][]ConfigProvenanceOverride, source ConfigSource) {
	for _, rule := range rulecatalog.Rules() {
		value := provenanceValueDisabled
		if rule.DefaultEnabled {
			value = provenanceValueEnabled
		}

		appendRuleProvenance(ruleHistory, rule.ID, source.Path, value)
	}
}

func appendConfiguredRuleProvenance(ruleHistory map[string][]ConfigProvenanceOverride, source ConfigSource, cfg configTypes.Config) {
	for _, rule := range cfg.Rules.Enabled {
		appendRuleProvenance(ruleHistory, rule, source.Path, provenanceValueEnabled)
	}

	for _, rule := range cfg.Rules.Disabled {
		appendRuleProvenance(ruleHistory, rule, source.Path, provenanceValueDisabled)
	}
}

func appendRuleProvenance(ruleHistory map[string][]ConfigProvenanceOverride, rule, source, value string) {
	ruleHistory[rule] = append(ruleHistory[rule], ConfigProvenanceOverride{Source: source, Value: value})
}

func buildProvenanceEntry(path string, history []ConfigProvenanceOverride) ConfigProvenanceEntry {
	current := history[len(history)-1]
	entry := ConfigProvenanceEntry{
		Path:   path,
		Reason: provenanceReason(path, current.Value, current.Source),
		Source: current.Source,
		Value:  current.Value,
	}

	if len(history) > 1 {
		overrides := make([]ConfigProvenanceOverride, 0, len(history)-1)
		for index := len(history) - 2; index >= 0; index-- {
			overrides = append(overrides, history[index])
		}

		entry.Overrides = overrides
	}

	return entry
}

func provenanceReason(path, value, source string) string {
	if strings.HasPrefix(path, provenanceRulePrefix) {
		return provenanceRuleReason(value, source)
	}

	if isExplicitConfigPath(path) {
		return "explicitly set in config"
	}

	return ""
}

func provenanceRuleReason(value, source string) string {
	if source == configSourceDefaults {
		switch value {
		case provenanceValueEnabled:
			return "enabled by default"
		case provenanceValueDisabled:
			return "disabled by default"
		default:
			return ""
		}
	}

	switch value {
	case provenanceValueEnabled:
		return "explicitly enabled in rules.enabled"
	case provenanceValueDisabled:
		return "explicitly disabled in rules.disabled"
	default:
		return ""
	}
}

func isExplicitConfigPath(path string) bool {
	for _, field := range getProvenanceFields() {
		if field.path == path {
			return field.explicit
		}
	}

	return false
}

func loadProvenanceConfig(source ConfigSource, fs FileSystem) (configTypes.Config, bool) {
	if source.Type == SourceTypeDefaults {
		return NewConfigWithDefaults(), true
	}

	if source.Type == SourceTypePreset {
		presetName, ok := ParsePresetReference(source.Path)
		if !ok {
			return configTypes.Config{}, false
		}

		cfg, err := ResolvePreset(presetName)
		if err != nil {
			return configTypes.Config{}, false
		}

		return cfg, true
	}

	result := parseConfigFileStrict(source.Path, fs)
	if result.Error != nil {
		return configTypes.Config{}, false
	}

	return result.Config, true
}

// getProvenanceFields lists every leaf Config field tracked by provenance. Adding
// a new Config field requires a matching entry here; the provenance coverage test
// (provenance_coverage_test.go) fails until that entry exists.
func getProvenanceFields() []provenanceField {
	return []provenanceField{
		ptrBoolField("allow_fixup_commits", func(c configTypes.Config) *bool { return c.AllowFixupCommits }),
		stringSliceField("checks.disabled", func(c configTypes.Config) []string { return c.Checks.Disabled }),
		stringSliceField("checks.enabled", func(c configTypes.Config) []string { return c.Checks.Enabled }),
		ptrBoolField("conventional.allow_multi_scope", func(c configTypes.Config) *bool { return c.Conventional.AllowMultiScope }),
		ptrBoolField("conventional.disallow_breaking", func(c configTypes.Config) *bool { return c.Conventional.DisallowBreaking }),
		ptrIntField("conventional.max_description_length", func(c configTypes.Config) *int { return c.Conventional.MaxDescriptionLength }),
		ptrBoolField("conventional.require_scope", func(c configTypes.Config) *bool { return c.Conventional.RequireScope }),
		stringSliceField("conventional.scopes", func(c configTypes.Config) []string { return c.Conventional.Scopes }),
		stringSliceField("conventional.types", func(c configTypes.Config) []string { return c.Conventional.Types }),
		stringSliceField("crypto_signature.allowed_signers", func(c configTypes.Config) []string { return c.CryptoSignature.AllowedSigners }),
		stringField("crypto_signature.key_directory", func(c configTypes.Config) string { return c.CryptoSignature.KeyDirectory }),
		ptrIntField("crypto_signature.min_ec_bits", func(c configTypes.Config) *int { return c.CryptoSignature.MinECBits }),
		ptrIntField("crypto_signature.min_rsa_bits", func(c configTypes.Config) *int { return c.CryptoSignature.MinRSABits }),
		ptrBoolField("crypto_signature.required", func(c configTypes.Config) *bool { return c.CryptoSignature.Required }),
		ptrBoolField("crypto_signature.require_gpg", func(c configTypes.Config) *bool { return c.CryptoSignature.RequireGPG }),
		ptrBoolField("crypto_signature.require_ssh", func(c configTypes.Config) *bool { return c.CryptoSignature.RequireSSH }),
		ptrBoolField("crypto_signature.verify_format", func(c configTypes.Config) *bool { return c.CryptoSignature.VerifyFormat }),
		stringSliceField("extends", func(c configTypes.Config) []string { return c.Extends }),
		stringSliceField("identity.allowed_authors", func(c configTypes.Config) []string { return c.Identity.AllowedAuthors }),
		stringSliceField("ignore_commits", func(c configTypes.Config) []string { return c.IgnoreCommits }),
		stringSliceField("jira.ignore_ticket_patterns", func(c configTypes.Config) []string { return c.Jira.IgnoreTicketPatterns }),
		stringSliceField("jira.project_prefixes", func(c configTypes.Config) []string { return c.Jira.ProjectPrefixes }),
		ptrBoolField("jira.require_in_body", func(c configTypes.Config) *bool { return c.Jira.RequireInBody }),
		ptrBoolField("jira.require_in_subject", func(c configTypes.Config) *bool { return c.Jira.RequireInSubject }),
		ptrIntField("limits.max_commit_count", func(c configTypes.Config) *int { return c.Limits.MaxCommitCount }),
		ptrIntField("limits.max_key_files", func(c configTypes.Config) *int { return c.Limits.MaxKeyFiles }),
		ptrBoolField("message.body.allow_signoff_only", func(c configTypes.Config) *bool { return c.Message.Body.AllowSignoffOnly }),
		ptrIntField("message.body.max_line_length", func(c configTypes.Config) *int { return c.Message.Body.MaxLineLength }),
		ptrIntField("message.body.min_length", func(c configTypes.Config) *int { return c.Message.Body.MinLength }),
		ptrIntField("message.body.min_signoff_count", func(c configTypes.Config) *int { return c.Message.Body.MinSignoffCount }),
		ptrBoolField("message.body.required", func(c configTypes.Config) *bool { return c.Message.Body.Required }),
		ptrStringField("message.subject.case", func(c configTypes.Config) *string { return c.Message.Subject.Case }),
		stringSliceField("message.subject.forbid_endings", func(c configTypes.Config) []string { return c.Message.Subject.ForbidEndings }),
		ptrIntField("message.subject.max_length", func(c configTypes.Config) *int { return c.Message.Subject.MaxLength }),
		ptrBoolField("message.subject.require_imperative", func(c configTypes.Config) *bool { return c.Message.Subject.RequireImperative }),
		stringField("output", func(c configTypes.Config) string { return c.Output }),
		ptrIntField("repo.max_commits_ahead", func(c configTypes.Config) *int { return c.Repo.MaxCommitsAhead }),
		stringSliceField("rules.disabled", func(c configTypes.Config) []string { return c.Rules.Disabled }),
		stringSliceField("rules.enabled", func(c configTypes.Config) []string { return c.Rules.Enabled }),
		stringSliceField("spell.ignore_words", func(c configTypes.Config) []string { return c.Spell.IgnoreWords }),
		ptrStringField("spell.locale", func(c configTypes.Config) *string { return c.Spell.Locale }),
		stringField("theme", func(c configTypes.Config) string { return c.Theme }),
	}
}

// provenanceUntracked is the set of Config paths intentionally excluded from
// provenance tracking. Keep this list short; prefer to add fields to the
// tracked table above.
//
//   - managed_sources is a slice of structured records, tracked via the dedicated
//     ManagedSources status pipeline rather than per-field provenance.
//
//nolint:gochecknoglobals // Constant exclusion list; declaring it inline would duplicate it across coverage checks.
var provenanceUntracked = map[string]struct{}{
	"managed_sources": {},
}

func formatStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}

	sorted := slices.Clone(values)
	slices.Sort(sorted)

	return "[" + strings.Join(sorted, ", ") + "]"
}
