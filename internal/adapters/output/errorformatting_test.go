// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestErrorFormatting_ErrorSummary_ReturnsUnknownErrorForEmptyList(t *testing.T) {
	t.Parallel()

	result := createErrorSummary([]domain.ValidationError{})
	check.Equal(t, messageUnknownError, result)
}

func TestErrorFormatting_ErrorSummary_UsesSingleErrorMessage(t *testing.T) {
	t.Parallel()

	errors := []domain.ValidationError{
		{Message: testSubjectIsTooLongMessage},
	}

	result := createErrorSummary(errors)
	check.Equal(t, testSubjectIsTooLongMessage, result)
}

func TestErrorFormatting_ErrorSummary_CombinesMultipleErrors(t *testing.T) {
	t.Parallel()

	errors := []domain.ValidationError{
		{Message: testSubjectIsTooLongMessage},
		{Message: "Missing conventional format"},
		{Message: "Wrong case detected"},
	}

	result := createErrorSummary(errors)
	expected := testSubjectIsTooLongMessage + ", Missing conventional format, Wrong case detected"
	check.Equal(t, expected, result)
}

// Two errors carrying the same reason are still listed separately and in the
// order they arrived -- the summary joins, it does not group or dedupe.
func TestErrorFormatting_ErrorSummary_KeepsSameReasonErrorsInOrder(t *testing.T) {
	t.Parallel()

	result := createErrorSummary([]domain.ValidationError{
		{Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeMisspelledWord)}, Message: "Misspelled: word1"},
		{Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeMisspelledWord)}, Message: "Misspelled: word2"},
	})

	check.Equal(t, "Misspelled: word1, Misspelled: word2", result)
}

func TestErrorFormatting_ExtractErrorType_KnownCodes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		code     string
		message  string
		expected string
	}{
		{
			name:     testErrorCodeSubjectTooLong,
			code:     testErrorCodeSubjectTooLong,
			message:  testSubjectIsTooLongMessage,
			expected: messageSubjectIs,
		},
		{
			name:     testErrorCodeInvalidCase,
			code:     testErrorCodeInvalidCase,
			message:  "Wrong case detected",
			expected: messageWrongCase,
		},
		{
			name:     testErrorCodeMisspelledWord,
			code:     testErrorCodeMisspelledWord,
			message:  "Word is misspelled",
			expected: "Misspelled",
		},
		{
			name:     testErrorCodeMissingSignoff,
			code:     testErrorCodeMissingSignoff,
			message:  "Commit missing sign-off",
			expected: messageMissingSignoff,
		},
		{
			name:     testErrorCodeInvalidConventional,
			code:     testErrorCodeInvalidConventional,
			message:  "Not conventional format",
			expected: "Format issue",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testCase.code)},
				Message:    testCase.message,
			}

			result := extractErrorType(err)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestErrorFormatting_ExtractErrorType_UnknownCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "Two words message",
			message:  "Custom validation error",
			expected: "Custom validation",
		},
		{
			name:     "Single word message",
			message:  testMessageError,
			expected: testMessageError,
		},
		{
			name:     testNameEmptyMessage,
			message:  "",
			expected: string(diagnostics.ReasonUnknown),
		},
		{
			name:     "Long message",
			message:  "This is a very long error message with many words",
			expected: "This is",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonUnknown},
				Message:    testCase.message,
			}

			result := extractErrorType(err)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestErrorFormatting_WriteFormattedError_VerbosityLevels(t *testing.T) {
	t.Parallel()

	err := domain.ValidationError{
		Diagnostic: diagnostics.Diagnostic{Reason: testErrorCode},
		Message:    testTestErrorMessage,
		Context: map[string]string{
			diagnostics.ContextExpected: "expected_value",
			"found":                     "found_value",
		},
	}

	colors := GetColorScheme(false, "") // No colors for easier testing

	testCases := []struct {
		name            string
		options         TextOptions
		showHelp        bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "Verbose level 0 - basic",
			options: TextOptions{
				VerboseLevel: 0,
			},
			showHelp: false,
			wantContains: []string{
				"    " + testTestErrorMessage,
			},
			wantNotContains: []string{
				testOutputErrorCodeHeader,
				"Error Message:",
				"Expected:",
			},
		},
		{
			name: "Verbose level 1 - structured friendly",
			options: TextOptions{
				VerboseLevel: 1,
			},
			showHelp: false,
			wantContains: []string{
				testTestErrorMessage,
				"→ Fix the test error",
			},
			wantNotContains: []string{
				testOutputErrorCodeHeader,
			},
		},
		{
			name: "Verbose level 2 - full technical detail",
			options: TextOptions{
				VerboseLevel: 2,
			},
			showHelp: true,
			wantContains: []string{
				"    Error Message: " + testTestErrorMessage,
				"    Expected: expected_value",
				"    Found: found_value",
			},
			wantNotContains: []string{
				testOutputHelpHeader, // test_error has no registered help text in LookupHelp
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			writeFormattedError(&builder, err, testCase.options, testCase.showHelp, colors)
			result := builder.String()

			for _, want := range testCase.wantContains {
				check.Contains(t, result, want, "Expected output to contain: %s", want)
			}

			for _, notWant := range testCase.wantNotContains {
				check.NotContains(t, result, notWant, "Expected output to NOT contain: %s", notWant)
			}
		})
	}
}

func TestErrorFormatting_WriteFormattedError_ContextOrdering(t *testing.T) {
	t.Parallel()

	err := domain.ValidationError{
		Diagnostic: diagnostics.Diagnostic{Reason: testErrorCode},
		Message:    testTestErrorShort,
		Context: map[string]string{
			diagnostics.ContextSuggestion: "use this instead",
			contextFirstWord:              "incorrect",
			contextExpectedWord:           "correct",
			contextCharPosition:           "5",
			"random_key":                  "random_value",
			"actual_length":               "50",
		},
	}

	var builder strings.Builder

	colors := GetColorScheme(false, "")
	options := TextOptions{VerboseLevel: 2}

	writeFormattedError(&builder, err, options, false, colors)
	result := builder.String()

	firstWordPos := strings.Index(result, "First Word:")
	expectedWordPos := strings.Index(result, "Expected:")
	actualLengthPos := strings.Index(result, "Actual Length:")
	charPositionPos := strings.Index(result, "Position:")
	suggestionPos := strings.Index(result, "Suggestion:")
	randomKeyPos := strings.Index(result, "Random Key:")

	// Ordered keys should appear first
	check.Less(t, firstWordPos, randomKeyPos, "Ordered keys should appear before unordered")
	check.Less(t, expectedWordPos, randomKeyPos, "Ordered keys should appear before unordered")
	check.Less(t, actualLengthPos, randomKeyPos, "Ordered keys should appear before unordered")
	check.Less(t, charPositionPos, randomKeyPos, "Ordered keys should appear before unordered")
	check.Less(t, suggestionPos, randomKeyPos, "Ordered keys should appear before unordered")
}

func TestErrorFormatting_WriteContextPair(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "expected",
			key:      diagnostics.ContextExpected,
			expected: testExpectedLabel,
		},

		{
			name:     "found key",
			key:      "found",
			value:    contextActualValue,
			expected: "    Found: " + contextActualValue,
		},
		{
			name:     "custom key",
			key:      "custom_key",
			value:    "custom_value",
			expected: "    Custom Key: custom_value",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			colors := GetColorScheme(false, "")

			writeContextPair(&builder, testCase.key, testCase.value, colors, nil)
			result := builder.String()

			check.Contains(t, result, testCase.expected)
		})
	}
}

func TestErrorFormatting_WriteHelpSection_FormattingStyles(t *testing.T) {
	t.Parallel()

	helpText := `This is a section header:
✓ This is a success example
• This is a bullet point with very long text that should wrap properly across multiple lines
Regular text line
Another regular line`

	var builder strings.Builder

	colors := GetColorScheme(false, "")

	writeHelpSection(&builder, helpText, colors)
	result := builder.String()

	check.Contains(t, result, "    This is a section header:")
	check.Contains(t, result, "    ✓ This is a success example")
	check.Contains(t, result, "    • This is a bullet point")
	check.Contains(t, result, "    Regular text line")

	lines := strings.Split(result, "\n")
	bulletLines := make([]string, 0, len(lines))

	for _, line := range lines {
		if strings.Contains(line, "bullet point") {
			bulletLines = append(bulletLines, line)

			break
		}
	}

	check.NotEmpty(t, bulletLines, "Should find bullet point line")
}

func TestErrorFormatting_WriteRuleReports_PassedCount(t *testing.T) {
	t.Parallel()

	rules := []domain.RuleReport{
		{
			Name:   "Passing Rule1",
			Status: domain.StatusPassed,
			Errors: nil,
		},
		{
			Name:   "Failing Rule",
			Status: domain.StatusFailed,
			Errors: []domain.ValidationError{{Diagnostic: diagnostics.Diagnostic{Reason: testErrorCode}, Message: testMessageError}},
		},
		{
			Name:   "Passing Rule2",
			Status: domain.StatusPassed,
			Errors: nil,
		},
	}

	var builder strings.Builder

	colors := GetColorScheme(false, "")
	options := TextOptions{VerboseLevel: 0}

	passedCount := writeRuleReports(&builder, rules, colors, options)

	check.Equal(t, 2, passedCount, "Should count 2 passed rules")

	result := builder.String()
	check.Contains(t, result, "✓ Passing Rule1")
	check.Contains(t, result, "✓ Passing Rule2")
	check.Contains(t, result, testOutputFailingRule)
}

func TestErrorFormatting_WriteRuleReports_VerbosityLevels(t *testing.T) {
	t.Parallel()

	rules := []domain.RuleReport{
		{
			Name:   "Failing Rule",
			Status: domain.StatusFailed,
			Errors: []domain.ValidationError{
				{
					Diagnostic: diagnostics.Diagnostic{Reason: testErrorCode},
					Message:    testTestErrorMessage,
				},
			},
		},
	}

	testCases := []struct {
		name            string
		verboseLevel    int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "Basic level",
			verboseLevel: 0,
			wantContains: []string{
				testOutputFailingRule,
				testTestErrorMessage,
			},
			wantNotContains: []string{
				testOutputErrorCodeHeader,
				"Failing Rule:",
				"→ Fix the test error", // suggestion not shown at basic level
			},
		},
		{
			name:         "Verbose level",
			verboseLevel: 1,
			wantContains: []string{
				testOutputFailingRule,
				testTestErrorMessage,
			},
			wantNotContains: []string{
				testOutputHelpHeader, // help not shown at level 1
			},
		},
		{
			name:         "Very verbose level",
			verboseLevel: 2,
			wantContains: []string{
				testOutputFailingRule,
				testTestErrorMessage,
			},
			wantNotContains: []string{
				testOutputHelpHeader, // test_error has no registered help text
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			colors := GetColorScheme(false, "")
			options := TextOptions{VerboseLevel: testCase.verboseLevel}

			writeRuleReports(&builder, rules, colors, options)
			result := builder.String()

			for _, want := range testCase.wantContains {
				check.Contains(t, result, want)
			}

			for _, notWant := range testCase.wantNotContains {
				check.NotContains(t, result, notWant)
			}
		})
	}
}

func TestErrorFormatting_WriteRuleReports_EmptyRules(t *testing.T) {
	t.Parallel()

	var builder strings.Builder

	colors := GetColorScheme(false, "")
	options := TextOptions{VerboseLevel: 1}

	passedCount := writeRuleReports(&builder, []domain.RuleReport{}, colors, options)

	check.Equal(t, 0, passedCount)
	check.Empty(t, builder.String())
}

func TestErrorFormatting_MultipleErrors_SummaryAndDetails(t *testing.T) {
	t.Parallel()

	maxLength := settings.DefaultSubjectMaxLength
	actualLength := maxLength + 25
	maxLengthStr := strconv.Itoa(maxLength)
	actualLengthStr := strconv.Itoa(actualLength)

	// Test error formatting with summary creation and individual error details
	errors := []domain.ValidationError{
		{
			Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeSubjectTooLong)},
			Message:    fmt.Sprintf("Subject line exceeds %d characters", maxLength),
			Context: map[string]string{
				"actual_length":               actualLengthStr,
				"expected_length":             maxLengthStr,
				diagnostics.ContextSuggestion: "Shorten the subject line",
			},
		},
		{
			Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeInvalidCase)},
			Message:    "First word should be capitalized",
			Context: map[string]string{
				contextFirstWord:    testWordFixLower,
				contextExpectedWord: testExpectedWordFix,
			},
		},
	}

	// Test error summary
	summary := createErrorSummary(errors)
	expected := fmt.Sprintf("Subject line exceeds %d characters, First word should be capitalized", maxLength)
	check.Equal(t, expected, summary)

	// Test individual error formatting
	for _, formatErr := range errors {
		var builder strings.Builder

		colors := GetColorScheme(false, "")
		options := TextOptions{VerboseLevel: 2}

		writeFormattedError(&builder, formatErr, options, true, colors)
		result := builder.String()

		check.Contains(t, result, formatErr.Message)
	}
}

// TestConvertToUserFriendlyMessage tests the new user-friendly message conversion.
func TestErrorFormatting_ConvertToUserFriendlyMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      domain.ValidationError
		expected string
	}{
		{
			name: "Conventional commit format error",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeInvalidConventional)},
				Message:    "Invalid format",
				Context:    map[string]string{diagnostics.ContextActual: testContextValueWrong},
			},
			expected: "'wrong' does not follow conventional format",
		},
		{
			name: testNameSubjectTooLong + " with context",
			err: func() domain.ValidationError {
				maxLength := settings.DefaultSubjectMaxLength
				actualLength := maxLength + 15

				return domain.ValidationError{
					Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeSubjectTooLong)},
					Message:    testSubjectIsTooLongMessage,
					Context: map[string]string{
						diagnostics.ContextActual:      strconv.Itoa(actualLength),
						diagnostics.ContextExpectedMax: strconv.Itoa(maxLength),
					},
				}
			}(),
			expected: "Your commit message is 15 characters too long",
		},
		{
			name: "Missing JIRA reference",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeMissingJira)},
				Message:    "Missing JIRA reference",
			},
			expected: "No JIRA reference found",
		},
		{
			name: messageMissingSignoff,
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeMissingSignoff)},
				Message:    messageMissingSignoff,
			},
			expected: "No Signed-off-by trailer found",
		},
		{
			name: "Misspelled word with context",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.Reason(testErrorCodeMisspelledWord)},
				Message:    "Misspelled word",
				Context:    map[string]string{diagnostics.ContextWord: "myspelling"},
			},
			expected: "'myspelling' appears to be misspelled",
		},
		{
			name: "Unknown error code fallback",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonUnknown},
				Message:    "ErrUnknownThing",
			},
			expected: "ErrUnknownThing",
		},
		// Body errors
		{
			name: "Body too short",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonBodyTooShort},
				Message:    "Too short",
			},
			expected: "Body is shorter than the configured minimum",
		},
		{
			name: "Invalid body",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidBody},
				Message:    "Body contains only sign-off lines",
			},
			expected: "Body has no descriptive content",
		},
		{
			name: "Invalid structure",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidStructure},
				Message:    "Invalid commit message structure",
			},
			expected: "Message is not structured as subject, blank line, body",
		},
		{
			name: "Misplaced signoff",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonMisplacedSignoff},
				Message:    "Content found after sign-off lines",
			},
			expected: "Content appears after sign-off lines",
		},
		{
			name: "Insufficient signoffs",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInsufficientSignoffs},
				Message:    "Duplicate signers found",
			},
			expected: "Too few distinct sign-offs",
		},
		// Imperative mood errors
		{
			name: testNameNoFirstWord,
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonNoFirstWord},
				Message:    "Cannot extract first word",
			},
			expected: "Subject does not start with a word",
		},
		{
			name: "Non-verb with context",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonNonVerb},
				Message:    "'this' is not a verb",
				Context:    map[string]string{diagnostics.ContextActual: "this"},
			},
			expected: "'this' is not a verb",
		},
		{
			name: "Past tense with suggestion",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonPastTense},
				Message:    "'added' is past tense",
				Context: map[string]string{
					diagnostics.ContextActual:     "added",
					diagnostics.ContextSuggestion: testWordAdd,
				},
			},
			expected: "'added' is past tense (use 'add')",
		},
		{
			name: "Past tense without context",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonPastTense},
				Message:    "Past tense detected",
			},
			expected: "First word is past tense",
		},
		{
			name: "Gerund with suggestion",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonGerund},
				Message:    "'adding' is a gerund",
				Context: map[string]string{
					diagnostics.ContextActual:     "adding",
					diagnostics.ContextSuggestion: testWordAdd,
				},
			},
			expected: "'adding' is a gerund (use 'add')",
		},
		{
			name: "Third person with suggestion",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonThirdPerson},
				Message:    "'adds' is third person",
				Context: map[string]string{
					diagnostics.ContextActual:     "adds",
					diagnostics.ContextSuggestion: testWordAdd,
				},
			},
			expected: "'adds' is third person (use 'add')",
		},
		// Conventional commit errors
		{
			name: "Invalid spacing",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidSpacing},
				Message:    "Missing space after colon",
			},
			expected: "Missing single space after the type colon",
		},
		{
			name: "Breaking change not allowed",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonBreakingChangeNotAllowed},
				Message:    "Breaking change indicator (!) is not allowed",
			},
			expected: "Breaking change marker (!) is not allowed",
		},
		{
			name: testNameInvalidMultiScope,
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidMultiScope},
				Message:    "Invalid multi-scope format",
			},
			expected: "Scope list is malformed (expected comma-separated without spaces)",
		},
		{
			name: "Multi scope not allowed",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonMultiScopeNotAllowed},
				Message:    "Multiple scopes are not allowed",
			},
			expected: "Multiple scopes are not allowed",
		},
		{
			name: "Invalid conventional scope with context",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidConventionalScope},
				Message:    "Invalid scope 'wrong'",
				Context:    map[string]string{diagnostics.ContextActual: testContextValueWrong},
			},
			expected: "'wrong' is not a valid scope",
		},
		// Signature errors
		{
			name: testNameIncompleteGPGSignature,
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonIncompleteGPGSig},
				Message:    testNameIncompleteGPGSignature,
			},
			expected: "GPG signature is incomplete",
		},
		{
			name: testNameIncompleteSSHSignature,
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonIncompleteSSHSig},
				Message:    testNameIncompleteSSHSignature,
			},
			expected: "SSH signature is incomplete",
		},
		{
			name: "Invalid key directory",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidKeyDirectory},
				Message:    "Key directory validation failed",
			},
			expected: "Signing key directory is not usable",
		},
		// JIRA format
		{
			name: "Invalid key format",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonInvalidKeyFormat},
				Message:    "Invalid JIRA key format",
			},
			expected: "JIRA key in Refs: line is not uppercase",
		},
		// Branch/repo errors
		{
			name: "Too many commits with over context",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonTooManyCommits},
				Message:    "Too many commits",
				Context:    map[string]string{"over": "5"},
			},
			expected: "Branch has 5 commits over the limit",
		},
		{
			name: "Missing reference with ref",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonMissingReference},
				Message:    "Reference branch not found",
				Context:    map[string]string{"ref": "main"},
			},
			expected: "Reference branch 'main' not found",
		},
		{
			name: "Missing reference without ref",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonMissingReference},
				Message:    "Reference branch is required",
			},
			expected: "Specify a base branch with --base-branch",
		},
		{
			name: "Git operation failed",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonGitOperationFailed},
				Message:    "Failed to check commits ahead",
			},
			expected: "Git operation failed — check repository state",
		},
		{
			name: "Config not found",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonConfigNotFound},
				Message:    "failed to load configuration",
			},
			expected: "Check your configuration file path",
		},
		{
			name: "Repo rule no repo",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonRepoRuleNoRepo},
				Message:    "repository rules configured but not in a git repository",
			},
			expected: "Run this command inside a Git repository or use --ignore-repo-rules",
		},
		{
			name: "Validation failed",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonValidationFailed},
				Message:    "1 commit failed validation",
			},
			expected: "Validation failed",
		},
		{
			// The identity rule reports the address as author_email, not actual.
			name: "Identity not allowed names the author",
			err: domain.ValidationError{
				Diagnostic: diagnostics.Diagnostic{Reason: diagnostics.ReasonIdentityNotAllowed},
				Message:    "Author email not authorized",
				Context:    diagnostics.Context{diagnostics.ContextAuthorEmail: "mallory@example.com"},
			},
			expected: "Author 'mallory@example.com' is not in the allowed list",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := convertToUserFriendlyMessage(testCase.err, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// TestGenerateUserSuggestion tests the new user suggestion generation.
func TestErrorFormatting_GenerateUserSuggestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      domain.ValidationError
		expected string
	}{
		{
			name: "Conventional commit format with expected types",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeInvalidConventional),
				Context:    map[string]string{diagnostics.ContextExpected: "fix,feat,docs"},
			},
			expected: "Try: fix: your message",
		},
		{
			name: testNameSubjectTooLong + " with actual subject",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeSubjectTooLong),
				Context:    map[string]string{diagnostics.ContextSubject: "This is a very long commit message that exceeds the limit"},
			},
			expected: "Try: \"This is a very long commit message that excee...\"",
		},
		{
			name: "Invalid case with context",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeInvalidCase),
				Context: map[string]string{
					diagnostics.ContextSubject: "Fix the bug",
					contextFirstWord:           testExpectedWordFix,
					contextExpectedWord:        testWordFixLower,
				},
			},
			expected: "Try: \"fix the bug\"",
		},
		{
			name: "Missing JIRA with subject",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeMissingJira),
				Context:    map[string]string{diagnostics.ContextSubject: "fix user login"},
			},
			expected: "Try: PROJ-123: fix user login",
		},
		{
			name: messageMissingSignoff,
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeMissingSignoff),
			},
			expected: "Add: git commit --signoff",
		},
		{
			name: "Misspelled word with suggestion",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(testErrorCodeMisspelledWord),
				Context:    map[string]string{diagnostics.ContextSuggestion: "receive"},
			},
			expected: "Try: \"receive\"",
		},
		{
			name: "No suggestion available",
			err: domain.ValidationError{
				Diagnostic: testDiagnostic(diagnostics.ReasonUnknown),
			},
			expected: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := generateUserSuggestion(testCase.err, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// TestProgressiveVerbositySystem tests the complete verbosity system.
func TestErrorFormatting_VerbosityLevelsSummary(t *testing.T) {
	t.Parallel()

	conventionalHelpShort := "Use format: " + domain.ConventionalCommitFormatTemplate

	err := domain.ValidationError{
		Rule:       testRuleTestRuleDisplay,
		Diagnostic: diagnostics.Diagnostic{ID: testCheckIDGL201, Reason: testErrorCodeInvalidConventional},
		Message:    "Invalid conventional format",
		Context:    map[string]string{diagnostics.ContextActual: testContextValueWrong, diagnostics.ContextExpected: "fix,feat,docs"},
	}

	tests := []struct {
		name            string
		verboseLevel    int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "Level 0 - User friendly only",
			verboseLevel: 0,
			wantContains: []string{
				"'wrong' does not follow conventional format",
				"→ Try: fix: your message",
			},
			wantNotContains: []string{
				testOutputErrorCodeHeader,
				testRuleTestRuleDisplay + ":",
				testErrorCodeInvalidConventional,
				testOutputHelpHeader,
			},
		},
		{
			name:         "Level 1 - Technical details (no help)",
			verboseLevel: 1,
			wantContains: []string{
				testOutputCheckIDGL201,
				"Error Message: Invalid conventional format",
				"Expected: fix,feat,docs",
				"Actual: wrong",
			},
			wantNotContains: []string{
				testOutputHelpHeader, // Level 1 shows technical details but no help text
			},
		},
		{
			name:         "Level 2 - Full technical detail",
			verboseLevel: 2,
			wantContains: []string{
				testOutputCheckIDGL201,
				"Error Message: Invalid conventional format",
				"Expected: fix,feat,docs",
				"Actual: wrong",
				testOutputHelpHeader,
				conventionalHelpShort,
			},
			wantNotContains: []string{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			colors := GetColorScheme(false, "")
			options := TextOptions{VerboseLevel: testCase.verboseLevel, ShowHelp: testCase.verboseLevel >= 2}

			writeFormattedError(&builder, err, options, options.ShowHelp, colors)

			result := builder.String()
			for _, want := range testCase.wantContains {
				check.Contains(t, result, want, "Expected output to contain: %s", want)
			}

			for _, notWant := range testCase.wantNotContains {
				check.NotContains(t, result, notWant, "Expected output to NOT contain: %s", notWant)
			}
		})
	}
}

// TestErrorFormatting_SpellErrorVerbosity tests spell error output at each verbosity level.
func TestErrorFormatting_SpellErrorVerbosity(t *testing.T) {
	t.Parallel()

	err := domain.ValidationError{
		Rule:       rulecatalog.Spell,
		Diagnostic: diagnostics.Diagnostic{ID: "GL601", Reason: diagnostics.ReasonMisspelledWord},
		Message:    "Misspelled word: 'teh' (suggestion: 'the')",
		Context: map[string]string{
			diagnostics.ContextWord:       "teh", //nolint:misspell
			diagnostics.ContextSuggestion: "the",
			diagnostics.ContextLocale:     "UK",
		},
	}

	tests := []struct {
		name            string
		verboseLevel    int
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "Level 0 - Shows misspelled word and suggestion",
			verboseLevel: 0,
			wantContains: []string{
				testMisspelledTehMessage,
				testTryTheSuggestion,
			},
			wantNotContains: []string{
				"Fix spelling in commit message",
				testOutputErrorCodeHeader,
				"Locale:",
			},
		},
		{
			name:         "Level 1 - Technical details with locale",
			verboseLevel: 1,
			wantContains: []string{
				"'teh' appears to be misspelled",
				testTryTheSuggestion,
				"Check ID: GL601",
				"Misspelled: 'teh'",
				"Suggestion: 'the'",
				"Locale: UK",
			},
		},
		{
			name:         "Level 2 - Full detail with locale",
			verboseLevel: 2,
			wantContains: []string{
				"'teh' appears to be misspelled",
				testTryTheSuggestion,
				"Check ID: GL601",
				"Misspelled: 'teh'",
				"Suggestion: 'the'",
				"Locale: UK",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			colors := GetColorScheme(false, "")
			options := TextOptions{VerboseLevel: testCase.verboseLevel, ShowHelp: testCase.verboseLevel >= 2}

			writeFormattedError(&builder, err, options, options.ShowHelp, colors)

			result := builder.String()
			for _, want := range testCase.wantContains {
				check.Contains(t, result, want, "Expected output to contain: %s", want)
			}

			for _, notWant := range testCase.wantNotContains {
				check.NotContains(t, result, notWant, "Expected output to NOT contain: %s", notWant)
			}
		})
	}
}

// TestUserFriendlyErrorCategories tests all category handlers.
func TestErrorFormatting_FriendlyMessages_Table(t *testing.T) {
	t.Parallel()

	conventionalHelpShort := "Use format: " + domain.ConventionalCommitFormatTemplate
	conventionalTypeList := strings.Join(settings.GetDefaultConventionalTypes(), ", ")

	tests := []struct {
		name     string
		reason   diagnostics.Reason
		expected string
	}{
		// Conventional commit errors
		{"Conventional format", testErrorCodeInvalidConventional, conventionalHelpShort},
		{testNameInvalidType, diagnostics.ReasonInvalidConventionalType, "Use a valid commit type like: " + conventionalTypeList},
		{"Missing scope", testErrorCodeMissingConventionalScope, "Type is missing a required scope"},

		// Subject line errors
		{testNameSubjectTooLong, testErrorCodeSubjectTooLong, "Your commit message is too long"},
		{messageWrongCase, testErrorCodeWrongCaseUpper, "First word is not capitalized"},
		{testNameMissingSubject, testErrorCodeMissingSubject, "Commit message has no subject"},

		// Body structure errors
		{"Missing body", "missing_body", "Body is required but missing"},
		{"Missing blank line", "missing_blank_line", "No blank line between subject and body"},

		// JIRA errors
		{"Missing JIRA", testErrorCodeMissingJira, "No JIRA reference found"},
		{"Invalid JIRA format", "invalid_jira_format", "JIRA reference is not in the PROJ-123 format"},

		// Signature errors
		{"Missing signoff", testErrorCodeMissingSignoff, "No Signed-off-by trailer found"},
		{testNameWeakKey, diagnostics.ReasonWeakKey, "Signing key is weaker than the required minimum"},

		// Language errors
		{"Misspelled word", testErrorCodeMisspelledWord, "Fix spelling in the commit message"},
		{"Non imperative", testErrorCodeNonImperative, "Subject is not in imperative mood"},

		// System errors
		{"Invalid repo", "invalid_repo", "Check that you are in a valid Git repository"},
		{"Operation cancelled", "operation_cancelled", "Operation cancelled"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := domain.ValidationError{Diagnostic: testDiagnostic(testCase.reason), Message: "Original message"}
			result := convertToUserFriendlyMessage(err, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}
