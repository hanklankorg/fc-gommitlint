// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestNormalizeSubjectCase_CanonicalizesSpellings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "canonical_value_unchanged", input: settings.CaseLower, expected: settings.CaseLower},
		{name: "uppercase_string", input: "UPPER", expected: settings.CaseUpper},
		{name: "mixed_case_string", input: "Sentence", expected: settings.CaseSentence},
		{name: "trim_whitespace", input: "  auto  ", expected: settings.CaseAuto},
		{name: "empty_string", input: "", expected: ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.expected, settings.NormalizeSubjectCase(testCase.input))
		})
	}
}

// TestNormalizeMessageConfig covers the seam that keeps config validation and
// rule construction agreeing: validation accepts "Upper" case-insensitively,
// so the value reaching SubjectRule must already be canonical or the rule
// silently falls back to its context-aware default.
func TestNormalizeMessageConfig_CanonicalizesWithoutMutating(t *testing.T) {
	t.Parallel()

	t.Run("canonicalizes case value", func(t *testing.T) {
		t.Parallel()

		config := settings.MessageConfig{}
		config.Subject.Case = new(" Upper ")

		result := settings.NormalizeMessageConfig(config)
		check.Equal(t, settings.CaseUpper, settings.DerefString(result.Subject.Case, ""))
	})

	t.Run("unset case stays unset", func(t *testing.T) {
		t.Parallel()

		result := settings.NormalizeMessageConfig(settings.MessageConfig{})
		check.Nil(t, result.Subject.Case)
	})

	t.Run("leaves the input untouched", func(t *testing.T) {
		t.Parallel()

		config := settings.MessageConfig{}
		config.Subject.Case = new("Upper")

		_ = settings.NormalizeMessageConfig(config)
		check.Equal(t, "Upper", settings.DerefString(config.Subject.Case, ""))
	})
}

func TestSubjectCaseEnum_AcceptsDeclaredAndNonCanonicalValues(t *testing.T) {
	t.Parallel()

	t.Run("valid values are the declared constants", func(t *testing.T) {
		t.Parallel()

		expected := []string{
			settings.CaseAuto,
			settings.CaseAny,
			settings.CaseLower,
			settings.CaseUpper,
			settings.CaseSentence,
		}
		check.Equal(t, expected, settings.GetValidSubjectCases())
	})

	t.Run("accepts non-canonical spellings", func(t *testing.T) {
		t.Parallel()

		check.True(t, settings.IsValidSubjectCase("Upper"))
		check.True(t, settings.IsValidSubjectCase(" sentence "))
	})

	t.Run("rejects unknown values", func(t *testing.T) {
		t.Parallel()

		check.False(t, settings.IsValidSubjectCase("ROBOT_MODE"))
		check.False(t, settings.IsValidSubjectCase(""))
	})
}
