// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

import (
	"slices"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// ConfigFormatYAML is the YAML config file format identifier.
const ConfigFormatYAML = "yaml"

// GetValidRuleNames returns the supported rule names for configuration.
func GetValidRuleNames() []string {
	return rulecatalog.IDs()
}

// GetValidRuleNamesSorted returns the supported rule names sorted alphabetically.
func GetValidRuleNamesSorted() []string {
	names := GetValidRuleNames()
	slices.Sort(names)

	return names
}

// IsValidRuleName reports whether a rule name is valid for configuration.
func IsValidRuleName(ruleName string) bool {
	return rulecatalog.IsValid(ruleName)
}

// NormalizeRuleName standardizes a rule name for comparisons.
func NormalizeRuleName(ruleName string) string {
	return rulecatalog.Normalize(ruleName)
}

// NormalizeRulesConfig standardizes rule names and removes empties while preserving order.
func NormalizeRulesConfig(rules RulesConfig) RulesConfig {
	rules.Enabled = normalizeRuleList(rules.Enabled)
	rules.Disabled = normalizeRuleList(rules.Disabled)

	return rules
}

// RuleResolution describes resolved rule configuration.
type RuleResolution struct {
	DefaultEnabled  []string
	DefaultDisabled []string
	ConfigEnabled   []string
	ConfigDisabled  []string
	FinalEnabled    []string
}

// ResolveEnabledRules applies rule enable/disable configuration to defaults.
// Disabled always takes precedence: a rule in both Enabled and Disabled is disabled.
func ResolveEnabledRules(defaultEnabled []string, rulesConfig RulesConfig) []string {
	result := make([]string, 0, len(defaultEnabled)+len(rulesConfig.Enabled))
	seen := make(map[string]struct{}, len(defaultEnabled)+len(rulesConfig.Enabled))
	disabledSet := make(map[string]struct{}, len(rulesConfig.Disabled))

	for _, disabled := range rulesConfig.Disabled {
		normalized := NormalizeRuleName(disabled)
		if normalized != "" {
			disabledSet[normalized] = struct{}{}
		}
	}

	addRule := func(rule string) {
		if rule == "" {
			return
		}

		if _, ok := seen[rule]; ok {
			return
		}

		// Disabled takes precedence over all sources
		if _, disabled := disabledSet[rule]; disabled {
			return
		}

		seen[rule] = struct{}{}
		result = append(result, rule)
	}

	for _, name := range rulesConfig.Enabled {
		addRule(NormalizeRuleName(name))
	}

	for _, rule := range defaultEnabled {
		addRule(NormalizeRuleName(rule))
	}

	return result
}

// ResolveRuleConfiguration normalizes and resolves rule configuration for logging.
func ResolveRuleConfiguration(defaultEnabled []string, rulesConfig RulesConfig) RuleResolution {
	normalizedConfig := NormalizeRulesConfig(rulesConfig)
	normalizedDefault := normalizeRuleList(defaultEnabled)

	return RuleResolution{
		DefaultEnabled:  normalizedDefault,
		DefaultDisabled: GetDefaultDisabledRules(),
		ConfigEnabled:   normalizedConfig.Enabled,
		ConfigDisabled:  normalizedConfig.Disabled,
		FinalEnabled:    ResolveEnabledRules(normalizedDefault, normalizedConfig),
	}
}

// RulesConfigForEnabled returns a rules config that enables exactly the supplied rules.
func RulesConfigForEnabled(enabled ...string) RulesConfig {
	normalizedEnabled := normalizeRuleList(enabled)

	enabledSet := make(map[string]struct{}, len(normalizedEnabled))
	for _, rule := range normalizedEnabled {
		enabledSet[rule] = struct{}{}
	}

	disabled := make([]string, 0, len(rulecatalog.Rules())-len(enabledSet))
	for _, rule := range rulecatalog.Rules() {
		if _, ok := enabledSet[rule.ID]; ok {
			continue
		}

		disabled = append(disabled, rule.ID)
	}

	return RulesConfig{Enabled: normalizedEnabled, Disabled: disabled}
}

func normalizeRuleList(rules []string) []string {
	if len(rules) == 0 {
		return rules
	}

	normalized := make([]string, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))

	for _, rule := range rules {
		cleaned := NormalizeRuleName(rule)
		if cleaned == "" {
			continue
		}

		if _, ok := seen[cleaned]; ok {
			continue
		}

		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	return normalized
}

// GetValidConfigShowFormats returns the supported formats for config show output.
func GetValidConfigShowFormats() []string {
	result := []string{"text", "json", ConfigFormatYAML}
	slices.Sort(result)

	return result
}

// GetValidSpellLocales returns the supported locale values for spell checking.
//
// "both" is the recommended default: it accepts US and UK spellings without
// flagging cross-variant words ("color" and "colour" both pass). "US" and "UK"
// (alias "GB") flag the opposite dialect. Empty string is accepted for
// backward compatibility and maps to "both".
func GetValidSpellLocales() []string {
	return []string{"both", "US", "UK", "GB"}
}

// GetDefaultConventionalTypes returns the standard conventional commit types.
func GetDefaultConventionalTypes() []string {
	return []string{
		"build", "chore", "ci", "docs", "feat", "fix",
		"perf", "refactor", "revert", "style", "test",
	}
}

// GetDefaultDisabledRules returns rules disabled by default.
func GetDefaultDisabledRules() []string {
	return rulecatalog.DefaultDisabled()
}

// GetDefaultForbiddenEndings returns the default punctuation marks that should not end commit subjects.
func GetDefaultForbiddenEndings() []string {
	return []string{".", ",", ";", ":", "!", "?"}
}

// Configuration defaults as constants.
const (
	// DefaultSubjectMaxLength allows room for conventional commit format:
	// type(scope): description. With type (2-8 chars), scope (0-20 chars),
	// and separator (2-4 chars), this leaves ~72 chars for the description.
	DefaultSubjectMaxLength = 100
	// DefaultSubjectCase signals context-aware case selection.
	// When "auto", the SubjectRule resolves to: "lower" for conventional commits,
	// "sentence" for non-conventional commits.
	DefaultSubjectCase = CaseAuto
	// DefaultConventionalMaxDescLength aligns with conventional commit best practices
	// ensuring descriptions remain concise while fitting terminal displays.
	DefaultConventionalMaxDescLength = 72
	// DefaultBodyMaxLineLength follows the Git convention of wrapping body
	// lines at 72 so `git log` output fits an 80-column terminal.
	DefaultBodyMaxLineLength = 72
	DefaultSpellLocale       = "both"
	DefaultMaxCommitCount    = 2000
	DefaultMaxCommitsAhead   = 10
	DefaultMaxKeyFiles       = 200

	// DefaultMinSignoffCount is the minimum sign-offs required when the signoff rule is enabled.
	DefaultMinSignoffCount = 1
	// MinSignoffCount is the lowest configured sign-off requirement.
	MinSignoffCount = 0
	// MaxSignoffCount is the highest configured sign-off requirement.
	MaxSignoffCount = 10
	// DefaultMinRSABits is the minimum RSA key length (NIST recommended minimum).
	// Configured values may range from 1-8192; values below 2048 are considered weak.
	DefaultMinRSABits = 2048
	// DefaultMinECBits is the minimum elliptic curve key length (P-256 standard).
	// Common values: 256 (P-256), 384 (P-384), 521 (P-521).
	DefaultMinECBits = 256
)
