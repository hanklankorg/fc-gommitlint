// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func TestTextHelpers_FormatContextKey_SnakeCase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "snake_case",
			input:    contextFirstWord,
			expected: "First Word",
		},
		{
			name:     "multiple_underscores",
			input:    "very_long_context_key_name",
			expected: "Very Long Context Key Name",
		},
		{
			name:     "single_word",
			input:    "word",
			expected: "Word",
		},
		{
			name:     "empty_string",
			input:    "",
			expected: "",
		},
		{
			name:     "already_capitalized",
			input:    "Already_Capitalized",
			expected: "Already Capitalized",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatContextKey(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestFormatContextKey_KebabCase(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "kebab-case",
			input:    "first-word",
			expected: "First Word",
		},
		{
			name:     "multiple-dashes",
			input:    "very-long-context-key",
			expected: "Very Long Context Key",
		},
		{
			name:     "mixed_separators",
			input:    "mixed-and_separated",
			expected: "Mixed And Separated",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatContextKey(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestFormatContextKey_EdgeCases(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "leading_underscore",
			input:    "_leading",
			expected: "Leading",
		},
		{
			name:     "trailing_underscore",
			input:    "trailing_",
			expected: "Trailing",
		},
		{
			name:     "multiple_consecutive_underscores",
			input:    "multiple__underscores",
			expected: "Multiple Underscores",
		},
		{
			name:     "only_separators",
			input:    "___",
			expected: "",
		},
		{
			name:     "leading_dashes",
			input:    "--leading",
			expected: "Leading",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatContextKey(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func specialCharactersReport() domain.Report {
	commit := domain.Commit{
		Hash:    testCommitHashABC1234,
		Subject: "feat: add \"quotes\" and <brackets>",
		Message: "feat: add \"quotes\" and <brackets>\n\nWith special chars: & % $ #",
	}

	validationError := domain.ValidationError{
		Rule:       testRuleTestRule,
		Diagnostic: testDiagnostic("special_chars"),
		Message:    "Error with \"quotes\" and <brackets> & symbols",
	}

	commitReport := domain.CommitReport{
		Commit: commit,
		Passed: false,
		RuleResults: []domain.RuleReport{
			{
				Name:   testRuleTestRule,
				Status: domain.StatusFailed,
				Errors: []domain.ValidationError{validationError},
			},
		},
	}

	return domain.Report{
		Summary: domain.ReportSummary{
			AllPassed:        false,
			TotalCommits:     1,
			ValidatedCommits: 1,
			IgnoredCommits:   0,
		},
		Commits: []domain.CommitReport{commitReport},
	}
}

func TestFormatTechnicalContextKey_KnownKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     contextCharPosition,
			input:    contextCharPosition,
			expected: "Position",
		},
		{
			name:     "line_number",
			input:    "line_number",
			expected: "Line",
		},
		{
			name:     contextColumnNumber,
			input:    contextColumnNumber,
			expected: "Column",
		},
		{
			name:     contextExpectedPattern,
			input:    contextExpectedPattern,
			expected: testExpectedLabel,
		},
		{
			name:     contextActualValue,
			input:    contextActualValue,
			expected: "Found",
		},
		{
			name:     "suggestion",
			input:    "suggestion",
			expected: "Suggestion",
		},
		{
			name:     "expected",
			input:    diagnostics.ContextExpected,
			expected: testExpectedLabel,
		},
		{
			name:     "found",
			input:    "found",
			expected: "Found",
		},
		{
			name:     contextExpectedWord,
			input:    contextExpectedWord,
			expected: "Expected word",
		},
		{
			name:     contextFirstWord,
			input:    contextFirstWord,
			expected: "Current Word",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatTechnicalContextKey(testCase.input, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestFormatTechnicalContextKey_UnknownKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unknown_key",
			input:    "unknown_key",
			expected: "Unknown Key",
		},
		{
			name:     "custom_context",
			input:    "custom_context_field",
			expected: "Custom Context Field",
		},
		{
			name:     "fallback_to_generic",
			input:    "some_other_field",
			expected: "Some Other Field",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatTechnicalContextKey(testCase.input, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}
