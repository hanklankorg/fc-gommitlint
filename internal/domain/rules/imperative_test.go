// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"errors"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

var errStemmerFailed = errors.New("stemmer failed")

// failingStemmer fails for every word, as Snowball does for an unknown language.
type failingStemmer struct{}

func (failingStemmer) Stem(string) (string, error) {
	return "", errStemmerFailed
}

type testStemmer struct{}

func (testStemmer) Stem(word string) (string, error) {
	stems := map[string]string{
		testWordAdded:       testWordAdd,
		"adding":            testWordAdd,
		"adds":              testWordAdd,
		"created":           testWordCreat,
		testWordCreating:    testWordCreat,
		"creates":           testWordCreat,
		testWordFixed:       testWordFix,
		"fixes":             testWordFix,
		testWordImplemented: testWordImplement,
		"implementing":      testWordImplement,
		"implements":        testWordImplement,
		"updated":           testWordUpdat,
		"updating":          testWordUpdat,
		"updates":           testWordUpdat,
	}

	if stem, ok := stems[word]; ok {
		return stem, nil
	}

	return word, nil
}

func newImperativeValidator() *rules.ImperativeValidator {
	return rules.NewImperativeValidator(testStemmer{})
}

// validateImperativeTestResult validates imperative test results.
func validateImperativeTestResult(t *testing.T, errors []domain.ValidationError, expectedErrors int, expectedCode expectedReason, description string) {
	t.Helper()

	check.Len(t, errors, expectedErrors,
		"Expected %d errors but got %d: %s",
		expectedErrors, len(errors), description)

	if expectedErrors > 0 {
		check.Equal(t, expectedCode, reasonOf(errors[0]),
			"Expected error code %s but got %s for test: %s",
			expectedCode, reasonOf(errors[0]), description)

		check.NotNil(t, errors[0].Context, "Error should have context")
		suggestion, hasSuggestion := errors[0].Context[diagnostics.ContextSuggestion]
		check.True(t, hasSuggestion, "Should provide suggestion for %s", expectedCode)
		check.NotEmpty(t, suggestion, "Suggestion should not be empty")
		check.NotEmpty(t, errors[0].Message, "Error should have a message")
	}
}

// runImperativeValidationTests runs imperative validation test scenarios.
func runImperativeValidationTests(t *testing.T, testCases []struct {
	name            string
	text            string
	originalSubject string
	expectedErrors  int
	expectedCode    expectedReason
	description     string
}) {
	t.Helper()

	validator := newImperativeValidator()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, false, nil, "TestRule")
			validateImperativeTestResult(t, errors, testCase.expectedErrors, testCase.expectedCode, testCase.description)
		})
	}
}

func TestImperativeValidator_PastTenseDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "detects past tense - added",
			text:            "added user authentication",
			originalSubject: "added user authentication",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonPastTense,
			description:     "Stemmer should detect 'added' as past tense of 'add'",
		},
		{
			name:            "detects past tense - implemented",
			text:            "implemented new feature",
			originalSubject: "implemented new feature",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonPastTense,
			description:     "Stemmer should detect complex past tense forms",
		},
		{
			name:            "detects past tense - fixed",
			text:            "fixed bug in parser",
			originalSubject: "fixed bug in parser",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonPastTense,
			description:     "Should detect irregular past tense",
		},
		{
			name:            "preserves base forms ending with ed",
			text:            "embed configuration files",
			originalSubject: "embed configuration files",
			expectedErrors:  0,
			description:     "Words naturally ending in 'ed' should pass",
		},
		{
			name:            "preserves embed variations",
			text:            "embed new settings",
			originalSubject: "embed new settings",
			expectedErrors:  0,
			description:     "Base forms ending with 'ed' should not trigger false positives",
		},
	}

	runImperativeValidationTests(t, testCases)
}

func TestImperativeValidator_GerundDetection(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "detects gerund - adding",
			text:            "adding new features",
			originalSubject: "adding new features",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonGerund,
			description:     "Should detect present participle forms",
		},
		{
			name:            "detects gerund - creating",
			text:            "creating user interface",
			originalSubject: "creating user interface",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonGerund,
			description:     "Should detect -ing forms that aren't base verbs",
		},
		{
			name:            "detects gerund - updating",
			text:            "updating documentation",
			originalSubject: "updating documentation",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonGerund,
			description:     "Should detect complex gerund forms",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, false, nil, "TestRule")
			validateImperativeTestResult(t, errors, testCase.expectedErrors, testCase.expectedCode, testCase.description)
		})
	}
}

func TestImperativeValidator_ThirdPersonDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "detects third person - adds",
			text:            "adds new functionality",
			originalSubject: "adds new functionality",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonThirdPerson,
			description:     "Should detect third person singular forms",
		},
		{
			name:            "detects third person - fixes",
			text:            "fixes memory leak",
			originalSubject: "fixes memory leak",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonThirdPerson,
			description:     "Should detect third person with irregular verbs",
		},
		{
			name:            "detects third person - updates",
			text:            "updates configuration",
			originalSubject: "updates configuration",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonThirdPerson,
			description:     "Should detect regular third person forms",
		},
		{
			name:            "preserves words ending with s",
			text:            "focus on performance",
			originalSubject: "focus on performance",
			expectedErrors:  0,
			description:     "Words naturally ending in 's' should pass",
		},
		{
			name:            "preserves process command",
			text:            "process user input",
			originalSubject: "process user input",
			expectedErrors:  0,
			description:     "Base form verbs ending in 's' should pass",
		},
	}

	runImperativeValidationTests(t, testCases)
}

func TestImperativeValidator_ValidImperativeForms(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		description     string
	}{
		{
			name:            "accepts imperative - add",
			text:            "add user authentication",
			originalSubject: "add user authentication",
			description:     "Proper imperative form should pass",
		},
		{
			name:            "accepts imperative - fix",
			text:            "fix memory leak",
			originalSubject: "fix memory leak",
			description:     "Simple imperative should pass",
		},
		{
			name:            "accepts imperative - update",
			text:            "update documentation",
			originalSubject: "update documentation",
			description:     "Regular imperative should pass",
		},
		{
			name:            "accepts imperative - implement",
			text:            "implement new feature",
			originalSubject: "implement new feature",
			description:     "Complex imperative should pass",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, false, nil, "TestRule")
			check.Empty(t, errors, "Expected no errors for valid imperative: %s", testCase.description)
		})
	}
}

func TestImperativeValidator_NonImperativeStarters(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "rejects article - the",
			text:            "the new feature",
			originalSubject: "the new feature",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonNonVerb,
			description:     "Should reject articles as non-verbs",
		},
		{
			name:            "rejects pronoun - this",
			text:            "this fixes the bug",
			originalSubject: "this fixes the bug",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonNonVerb,
			description:     "Should reject pronouns as non-verbs",
		},
		{
			name:            "rejects pronoun - we",
			text:            "we added new feature",
			originalSubject: "we added new feature",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonNonVerb,
			description:     "Should reject pronouns as sentence starters",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, false, nil, "TestRule")

			check.Len(t, errors, testCase.expectedErrors,
				"Expected %d errors but got %d: %s",
				testCase.expectedErrors, len(errors), testCase.description)

			if testCase.expectedErrors > 0 {
				check.Equal(t, testCase.expectedCode, reasonOf(errors[0]),
					"Expected error code %s but got %s for test: %s",
					testCase.expectedCode, reasonOf(errors[0]), testCase.description)
				check.NotEmpty(t, errors[0].Message, "Error should have a message")
			}
		})
	}
}

func TestImperativeValidator_EdgeCasesAndErrorHandling(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "handles empty text",
			text:            "",
			originalSubject: "",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonNoFirstWord,
			description:     "Should handle empty input gracefully",
		},
		{
			name:            "handles whitespace only",
			text:            "   \t\n   ",
			originalSubject: "   \t\n   ",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonNoFirstWord,
			description:     "Should handle whitespace-only input",
		},
		{
			name:            "handles numeric start",
			text:            "123 add feature",
			originalSubject: "123 add feature",
			expectedErrors:  0,
			description:     "The first word is '123': not a non-verb starter and without a verb suffix, so it passes",
		},
		{
			name:            "handles unknown words gracefully",
			text:            "xyz123 unknown word",
			originalSubject: "xyz123 unknown word",
			expectedErrors:  0,
			description:     "Should handle stemming failures gracefully",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, false, nil, "TestRule")

			check.Len(t, errors, testCase.expectedErrors,
				"Expected %d errors but got %d: %s",
				testCase.expectedErrors, len(errors), testCase.description)

			if testCase.expectedErrors > 0 {
				check.Equal(t, testCase.expectedCode, reasonOf(errors[0]),
					"Expected error code %s but got %s for test: %s",
					testCase.expectedCode, reasonOf(errors[0]), testCase.description)
				check.NotEmpty(t, errors[0].Message, "Error should have a message")
			}
		})
	}
}

func TestImperativeValidator_ConventionalCommitValidation(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name            string
		text            string
		originalSubject string
		expectedErrors  int
		expectedCode    expectedReason
		description     string
	}{
		{
			name:            "conventional commit description validation",
			text:            "feat: added user auth",
			originalSubject: "feat: added user auth",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonPastTense,
			description:     "Should validate description part of conventional commits",
		},
		{
			name:            "conventional commit with scope",
			text:            "feat(auth): implemented login",
			originalSubject: "feat(auth): implemented login",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonPastTense,
			description:     "Should validate description in scoped conventional commits",
		},
		{
			name:            "conventional commit valid imperative",
			text:            "feat: add user authentication",
			originalSubject: "feat: add user authentication",
			expectedErrors:  0,
			description:     "Valid conventional commit with imperative should pass",
		},
		{
			name:            "conventional commit with breaking change",
			text:            "feat!: add breaking change",
			originalSubject: "feat!: add breaking change",
			expectedErrors:  0,
			description:     "Breaking change conventional commits should work",
		},
		{
			name:            "invalid conventional format",
			text:            "feat add user auth",
			originalSubject: "feat add user auth",
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonInvalidConventionalFormat,
			description:     "Should catch malformed conventional commits",
		},
		{
			name:            "missing conventional description",
			text:            testConventionalFeatColon,
			originalSubject: testConventionalFeatColon,
			expectedErrors:  1,
			expectedCode:    diagnostics.ReasonMissingConventionalSubject,
			description:     "Should catch missing description in conventional commits",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.originalSubject, true, nil, "TestRule")

			check.Len(t, errors, testCase.expectedErrors,
				"Expected %d errors but got %d: %s",
				testCase.expectedErrors, len(errors), testCase.description)

			if testCase.expectedErrors > 0 {
				check.Equal(t, testCase.expectedCode, reasonOf(errors[0]),
					"Expected error code %s but got %s for test: %s",
					testCase.expectedCode, reasonOf(errors[0]), testCase.description)
				check.NotEmpty(t, errors[0].Message, "Error should have a message")
			}
		})
	}
}

func TestImperativeValidator_Stemming(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	// The stem itself is the stub stemmer's, so only the verdict it leads to is asserted.
	testCases := []struct {
		word        string
		shouldError bool
		errorType   expectedReason
		description string
	}{
		// Past tense stemming
		{testWordAdded, true, diagnostics.ReasonPastTense, "Simple past tense"},
		{testWordImplemented, true, diagnostics.ReasonPastTense, "Complex past tense"},
		{"created", true, diagnostics.ReasonPastTense, "Regular -ed past tense"},
		{testWordFixed, true, diagnostics.ReasonPastTense, "Irregular past tense"},

		// Gerund stemming
		{"adding", true, diagnostics.ReasonGerund, "Simple gerund"},
		{testWordCreating, true, diagnostics.ReasonGerund, "Regular gerund"},
		{"implementing", true, diagnostics.ReasonGerund, "Complex gerund"},

		// Third person stemming
		{"adds", true, diagnostics.ReasonThirdPerson, "Simple third person"},
		{"creates", true, diagnostics.ReasonThirdPerson, "Regular third person"},
		{"implements", true, diagnostics.ReasonThirdPerson, "Complex third person"},

		// Base forms that should pass
		{testWordAdd, false, "", "Base form verb"},
		{testWordFix, false, "", "Base form irregular"},
		{testWordImplement, false, "", "Base form complex"},
		{"create", false, "", "Base form regular"},

		// Words ending with ed/ing/s that are base forms
		{"embed", false, "", testBaseFormEndingED},
		{"shed", false, "", testBaseFormEndingED},
		{"feed", false, "", testBaseFormEndingED},
		{"focus", false, "", testBaseFormEndingS},
		{"process", false, "", testBaseFormEndingS},
		{"pass", false, "", testBaseFormEndingS},
	}

	for _, testCase := range testCases {
		t.Run(testCase.word, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.word, testCase.word, false, nil, "TestRule")

			if testCase.shouldError {
				check.NotEmpty(t, errors, "Expected error for '%s': %s", testCase.word, testCase.description)
				check.Equal(t, testCase.errorType, reasonOf(errors[0]),
					"Expected error type %s for '%s' but got %s",
					testCase.errorType, testCase.word, reasonOf(errors[0]))

				// Verify suggestion quality for verb forms
				if suggestion, hasSuggestion := errors[0].Context[diagnostics.ContextSuggestion]; hasSuggestion {
					check.NotEmpty(t, suggestion, "Should provide suggestion for '%s'", testCase.word)
					// For well-known cases, verify the suggestion makes sense
					switch testCase.word {
					// Suggestions match the original word's first-letter
					// case so they don't fight a configured lowercase
					// subject-case rule.
					case "added":
						check.Equal(t, "add", suggestion, "Should suggest 'add' for 'added'")
					case "fixed":
						check.Equal(t, "fix", suggestion, "Should suggest 'fix' for 'fixed'")
					case testWordCreating:
						check.Equal(t, "create", suggestion, "Should suggest 'create' for 'creating'")
					}
				}
			} else {
				check.Empty(t, errors, "Expected no error for '%s': %s", testCase.word, testCase.description)
			}
		})
	}
}

func TestImperativeValidator_ConventionalCommitParsing(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	testCases := []struct {
		name           string
		subject        string
		expectedErrors int
		expectedCode   expectedReason
		description    string
	}{
		{
			name:           "valid conventional with imperative",
			subject:        "feat: add new user system",
			expectedErrors: 0,
			description:    "Should parse and validate conventional commit description",
		},
		{
			name:           "conventional with past tense in description",
			subject:        "feat: added new user system",
			expectedErrors: 1,
			expectedCode:   diagnostics.ReasonPastTense,
			description:    "Should detect past tense in conventional commit description",
		},
		{
			name:           "conventional with scope and past tense",
			subject:        "feat(auth): implemented login system",
			expectedErrors: 1,
			expectedCode:   diagnostics.ReasonPastTense,
			description:    "Should validate description in scoped conventional commits",
		},
		{
			name:           "conventional with gerund",
			subject:        "fix(api): updating user endpoints",
			expectedErrors: 1,
			expectedCode:   diagnostics.ReasonGerund,
			description:    "Should detect gerunds in conventional commit descriptions",
		},
		{
			name:           "conventional with breaking change",
			subject:        "feat!: remove deprecated API",
			expectedErrors: 0,
			description:    "Should handle breaking change indicators",
		},
		{
			name:           "conventional without description",
			subject:        testConventionalFeatColon,
			expectedErrors: 1,
			expectedCode:   diagnostics.ReasonMissingConventionalSubject,
			description:    "Should catch missing description",
		},
		{
			name:           "conventional-like but invalid",
			subject:        "feat add new feature",
			expectedErrors: 1,
			expectedCode:   diagnostics.ReasonInvalidConventionalFormat,
			description:    "Should catch malformed conventional commits",
		},
		{
			name:           "conventional with multiple scopes",
			subject:        "feat(ui,api): add user management",
			expectedErrors: 0,
			description:    "Should handle multi-scope conventional commits",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.subject, testCase.subject, true, nil, "TestRule")

			check.Len(t, errors, testCase.expectedErrors,
				"Expected %d errors but got %d for '%s': %s",
				testCase.expectedErrors, len(errors), testCase.subject, testCase.description)

			if testCase.expectedErrors > 0 {
				check.Equal(t, testCase.expectedCode, reasonOf(errors[0]),
					"Expected error code %s but got %s for '%s'",
					testCase.expectedCode, reasonOf(errors[0]), testCase.subject)
			}
		})
	}
}

func TestImperativeValidator_ErrorFallback(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	// Test scenarios where stemming might fail or encounter edge cases.
	// Each case should either pass validation (starts with valid imperative)
	// or return appropriate errors (not just crash).
	testCases := []struct {
		name         string
		text         string
		expectErrors bool
		description  string
	}{
		{
			name:         "handles special characters",
			text:         "add @special #characters",
			expectErrors: false, // "add" is valid imperative
			description:  "Should handle text with special characters",
		},
		{
			name:         "handles unicode",
			text:         "добавить новую функцию", // Russian text
			expectErrors: true,                     // Non-English text is not recognized as imperative
			description:  "Should handle non-English unicode text gracefully",
		},
		{
			name:         "handles mixed content",
			text:         "add URLs: https://example.com",
			expectErrors: false, // "add" is valid imperative
			description:  "Should handle mixed text and URLs",
		},
		{
			name:         "handles numbers and letters",
			text:         "add v2.0 support",
			expectErrors: false, // "add" is valid imperative
			description:  "Should handle version numbers",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.text, testCase.text, false, nil, "TestRule")

			if testCase.expectErrors {
				check.NotEmpty(t, errors, "%s: expected errors but got none", testCase.description)
			} else {
				check.Empty(t, errors, "%s: expected no errors but got: %v", testCase.description, errors)
			}
		})
	}
}

func TestImperativeValidator_StemmerFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("nil stemmer uses the built-in suffix stemmer", func(t *testing.T) {
		t.Parallel()

		failures := rules.NewImperativeValidator(nil).ValidateImperative("added feature", "added feature", false, nil, "TestRule")

		check.Len(t, failures, 1)
		check.Equal(t, diagnostics.ReasonPastTense, reasonOf(failures[0]))
	})

	t.Run("a stemmer failure leaves the word unjudged", func(t *testing.T) {
		t.Parallel()

		failures := rules.NewImperativeValidator(failingStemmer{}).ValidateImperative("added feature", "added feature", false, nil, "TestRule")

		check.Empty(t, failures)
	})
}

func TestImperativeValidator_SuggestionQuality(t *testing.T) {
	t.Parallel()

	validator := newImperativeValidator()

	// Test that suggestions are helpful and properly formatted
	testCases := []struct {
		input              string
		expectedSuggestion string
		description        string
	}{
		// Suggestions match the input word's first-letter case so a
		// lowercase commit subject keeps a lowercase suggestion.
		{"added", "add", "Should match lowercase input"},
		{"Added", "Add", "Should match uppercase input"},
		{"implemented", "implement", "Should handle complex past tense"},
		{testWordCreating, "create", "Should handle gerunds"},
		{"Creating", "Create", "Should match uppercase input for gerunds"},
		{"fixes", "fix", "Should handle third person"},
		{"updated", "update", "Should handle regular past tense"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.input, func(t *testing.T) {
			t.Parallel()

			errors := validator.ValidateImperative(testCase.input, testCase.input, false, nil, "TestRule")
			check.NotEmpty(t, errors, "Expected error for '%s'", testCase.input)

			suggestion, hasSuggestion := errors[0].Context[diagnostics.ContextSuggestion]
			check.True(t, hasSuggestion, "Should provide suggestion")
			check.Equal(t, testCase.expectedSuggestion, suggestion, testCase.description)
		})
	}
}
