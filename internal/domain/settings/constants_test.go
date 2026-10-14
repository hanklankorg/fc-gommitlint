// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestForbiddenEndings_ContainsStandardPunctuation(t *testing.T) {
	t.Parallel()

	expectedEndings := []string{".", ",", ";", ":", "!", "?"}

	check.Equal(t, expectedEndings, settings.GetDefaultForbiddenEndings())
}

func TestConventionalTypes_ContainsStandardTypes(t *testing.T) {
	t.Parallel()

	// Test that we have the standard conventional commit types
	// According to https://www.conventionalcommits.org/
	standardTypes := map[string]string{
		settingsTestTypeFeat: "new feature",
		settingsTestTypeFix:  "bug fix",
		settingsTestTypeDocs: "documentation",
		"style":              "formatting changes",
		"refactor":           "code refactoring",
		"perf":               "performance improvements",
		"test":               "tests",
		"build":              "build system",
		"ci":                 "continuous integration",
		"chore":              "maintenance",
		"revert":             "revert changes",
	}

	for standardType := range standardTypes {
		check.Contains(t, settings.GetDefaultConventionalTypes(), standardType,
			"Missing standard conventional commit type: %s", standardType)
	}
}

func TestNormalizeRuleName_TrimsUnquotesAndFoldsCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase_string",
			input:    rulecatalog.Subject,
			expected: rulecatalog.Subject,
		},
		{
			name:     "uppercase_string",
			input:    "SUBJECT",
			expected: rulecatalog.Subject,
		},
		{
			name:     "mixed_case_string",
			input:    "SuBjEcT",
			expected: rulecatalog.Subject,
		},
		{
			name:     "trim_whitespace",
			input:    " " + rulecatalog.Subject + " ",
			expected: rulecatalog.Subject,
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "string_with_quotes",
			input:    "'" + rulecatalog.Subject + "'",
			expected: rulecatalog.Subject,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := settings.NormalizeRuleName(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestNormalizeRulesConfig_DeduplicatesPreservingOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		enabled          []string
		disabled         []string
		expectedEnabled  []string
		expectedDisabled []string
	}{
		{
			name:             "empty_rules",
			enabled:          nil,
			disabled:         nil,
			expectedEnabled:  nil,
			expectedDisabled: nil,
		},
		{
			name:             "normalize_and_deduplicate",
			enabled:          []string{rulecatalog.Subject, " " + rulecatalog.Subject + " ", " " + rulecatalog.Conventional + " "},
			disabled:         []string{rulecatalog.Spell, " " + rulecatalog.Spell + " ", ""},
			expectedEnabled:  []string{rulecatalog.Subject, rulecatalog.Conventional},
			expectedDisabled: []string{rulecatalog.Spell},
		},
		{
			name:             "preserve_order",
			enabled:          []string{rulecatalog.Spell, rulecatalog.Subject, rulecatalog.Conventional},
			disabled:         []string{rulecatalog.JiraReference, rulecatalog.CommitBody},
			expectedEnabled:  []string{rulecatalog.Spell, rulecatalog.Subject, rulecatalog.Conventional},
			expectedDisabled: []string{rulecatalog.JiraReference, rulecatalog.CommitBody},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rules := settings.NormalizeRulesConfig(settings.RulesConfig{
				Enabled:  testCase.enabled,
				Disabled: testCase.disabled,
			})

			check.Equal(t, testCase.expectedEnabled, rules.Enabled)
			check.Equal(t, testCase.expectedDisabled, rules.Disabled)
		})
	}
}

func TestNormalizeChecksConfig_UppercasesAndDeduplicates(t *testing.T) {
	t.Parallel()

	checksConfig := settings.NormalizeChecksConfig(settings.ChecksConfig{
		Enabled:  []string{"gl102", " " + testCheckIDGL301 + " ", "gl102"},
		Disabled: []string{"'gl901'", ""},
	})

	check.Equal(t, []string{testCheckIDGL102, testCheckIDGL301}, checksConfig.Enabled)
	check.Equal(t, []string{testCheckIDGL901}, checksConfig.Disabled)
}

func TestRuleNamesForEnabledChecks_MapsChecksToRulesOnce(t *testing.T) {
	t.Parallel()

	rules := settings.RuleNamesForEnabledChecks(settings.ChecksConfig{
		Enabled: []string{testCheckIDGL301, testCheckIDGL102, testCheckIDGL301},
	})

	check.Equal(t, []string{rulecatalog.CommitBody, rulecatalog.Subject}, rules)
}

func TestRulesConfigForEnabled_DerivesDisabledRules(t *testing.T) {
	t.Parallel()

	rules := settings.RulesConfigForEnabled(rulecatalog.Subject, rulecatalog.CommitBody, rulecatalog.Subject)

	check.Equal(t, []string{rulecatalog.Subject, rulecatalog.CommitBody}, rules.Enabled)
	check.NotContains(t, rules.Disabled, rulecatalog.Subject)
	check.NotContains(t, rules.Disabled, rulecatalog.CommitBody)
	check.Contains(t, rules.Disabled, rulecatalog.Conventional)
	check.Contains(t, rules.Disabled, rulecatalog.LinearHistory)
}

func TestResolveEnabledRules_DefaultDisabledCanBeExplicitlyEnabled(t *testing.T) {
	t.Parallel()

	resolved := settings.ResolveEnabledRules(rulecatalog.DefaultEnabled(), settings.RulesConfig{
		Enabled: []string{rulecatalog.CommitBody},
	})

	check.Contains(t, resolved, rulecatalog.CommitBody)
}
