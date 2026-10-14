// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	domainrules "codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// mockSpellChecker is a test double for SpellChecker interface.
type mockSpellChecker struct {
	misspellings []domain.Misspelling
}

// CheckText implements the SpellChecker interface for testing.
func (m *mockSpellChecker) CheckText(text string) []domain.Misspelling {
	// If misspellings are configured, use them
	if m.misspellings != nil {
		return m.misspellings
	}

	// Otherwise, simulate finding common test misspellings
	return m.findCommonMisspellings(text)
}

// findCommonMisspellings simulates detecting common misspellings for testing.
func (m *mockSpellChecker) findCommonMisspellings(text string) []domain.Misspelling {
	var found []domain.Misspelling

	// Map of common misspellings to their corrections
	//nolint:misspell // Intentional misspellings for testing
	misspellings := map[string]string{
		"the the":      testWordThe,
		testWordTeh:    testWordThe,
		"recieve":      "receive",
		"occured":      "occurred",
		"seperate":     "separate",
		"acheive":      "achieve",
		"definately":   "definitely",
		"inconsistant": "inconsistent",
		"mispelling":   "misspelling",
	}

	for wrong, correct := range misspellings {
		if idx := strings.Index(text, wrong); idx >= 0 {
			found = append(found, domain.Misspelling{
				Word:       wrong,
				Suggestion: correct,
				Position:   idx,
			})
		}
	}

	return found
}

// minimalSpellCommit creates a minimal commit for spell checking tests.
// Only populates fields needed for spell rule validation.
func minimalSpellCommit(message string) domain.Commit {
	// Parse subject and body for spell checking
	lines := strings.Split(message, "\n")
	subject := lines[0]

	var body string
	if len(lines) > 1 {
		body = strings.Join(lines[1:], "\n")
	}

	return domain.Commit{
		Subject: subject,
		Message: message,
		Body:    body,
	}
}

// TestSpellRule tests the spell checking rule functionality.
func TestSpellRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		subject          string
		body             string
		expectMisspelled bool
		description      string
	}{
		{
			name:        "Valid spelling",
			subject:     testSubjectAddNewFeature,
			body:        "This commit adds a new feature to the application.",
			description: "Should handle correctly spelled words",
		},
		{
			name:             "Common misspelling in subject",
			subject:          "Fix " + testWordTeh + " message validation",
			body:             "Update validation logic",
			expectMisspelled: true,
			description:      "Should check spelling in subject",
		},
		{
			name:             "Common misspelling in body",
			subject:          "Update documentation",
			body:             "This " + testWordRecieve + " path updates the documentation files.",
			expectMisspelled: true,
			description:      "Should check spelling in body",
		},
		{
			name:        "Technical terms",
			subject:     "Add JWT authentication",
			body:        "Implement JWT-based authentication with bcrypt hashing.",
			description: "Should handle technical terms",
		},
		{
			name:        "Code snippets",
			subject:     "Fix variable naming",
			body:        "Update getUserInfo() method to getUser().",
			description: "Should handle code snippets",
		},
		{
			name:        "Empty subject and body",
			subject:     "",
			body:        "",
			description: "Should handle empty content gracefully",
		},
		{
			name:        "Subject only commit",
			subject:     "Quick typo fix",
			body:        "",
			description: "Should handle subject-only commits",
		},
		{
			name:        "Special characters and punctuation",
			subject:     "Fix bug: can't access user data",
			body:        "Resolve the issue where users can't access their data.",
			description: "Should handle contractions and punctuation",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create commit using factory
			message := testCase.subject
			if testCase.body != "" {
				message = testCase.subject + "\n\n" + testCase.body
			}

			commit := minimalSpellCommit(message)

			cfg := settings.Config{}
			checker := &mockSpellChecker{}
			rule := domainrules.NewSpellRule(cfg, checker)

			failures := rule.Validate(commit)

			// The deterministic checker makes each expected outcome exact.
			if testCase.expectMisspelled {
				check.NotEmpty(t, failures, "%s: expected a spelling failure", testCase.description)
			} else {
				check.Empty(t, failures, "%s: expected no spelling failure, got %v", testCase.description, failures)
			}
		})
	}
}

// TestSpellRule_EnhancedContext tests that all context fields are properly populated.
func TestSpellRule_ContextHandling(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Spell: settings.SpellConfig{
			Locale: new("uk"),
		},
	}

	// Use a message with known misspellings
	commit := minimalSpellCommit("Fix teh issue") //nolint:misspell
	checker := &mockSpellChecker{
		misspellings: []domain.Misspelling{
			{Word: testWordTeh, Suggestion: testWordThe, Position: 4},
		},
	}
	rule := domainrules.NewSpellRule(cfg, checker)

	errors := rule.Validate(commit)
	check.NotEmpty(t, errors, "Should find misspellings")

	for _, spellErr := range errors {
		check.Equal(t, testWordTeh, spellErr.Context[diagnostics.ContextWord], "Should have misspelled word")
		check.Equal(t, testWordThe, spellErr.Context[diagnostics.ContextSuggestion], "Should have suggestion")
		check.Equal(t, "uk", spellErr.Context[diagnostics.ContextLocale], "Should have locale")
		check.Contains(t, spellErr.Message, testWordTeh, "Message should include misspelled word")
		check.Contains(t, spellErr.Message, testWordThe, "Message should include suggestion")
	}
}

// TestSpellRule_IgnoreWordsSuppressesOnlyTheListedWord covers Spell.IgnoreWords,
// which filterIgnoredWords applies by matching the reported word.
func TestSpellRule_IgnoreWordsSuppressesOnlyTheListedWord(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		ignoreWords []string
		expectWords []string
		description string
	}{
		{
			name:        "no_ignore_list_reports_both",
			ignoreWords: nil,
			expectWords: []string{testWordTeh, testWordRecieve},
			description: "baseline: without the feature both misspellings surface",
		},
		{
			name:        "listed_word_is_suppressed",
			ignoreWords: []string{testWordTeh},
			expectWords: []string{testWordRecieve},
			description: "the ignored word goes, the unlisted one stays",
		},
		{
			// Covers the lowercasing of the configured word only. The other
			// half of that comparison cannot be reached here: this checker
			// reports the literal lowercase keys of its own map, so no input
			// produces a capitalised Word to match against.
			name:        "configured_word_case_does_not_matter",
			ignoreWords: []string{strings.ToUpper(testWordTeh)},
			expectWords: []string{testWordRecieve},
			description: "a word listed in config in any case still suppresses its misspelling",
		},
		{
			name:        "listing_every_word_clears_the_failure",
			ignoreWords: []string{testWordTeh, testWordRecieve},
			expectWords: nil,
			description: "a fully ignored commit produces no failures at all",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Spell: settings.SpellConfig{
					IgnoreWords: testCase.ignoreWords,
					Locale:      new("us"),
				},
			}

			commit := minimalSpellCommit("Fix " + testWordTeh + " so we " + testWordRecieve + " it")
			rule := domainrules.NewSpellRule(cfg, &mockSpellChecker{})

			failures := rule.Validate(commit)

			got := make([]string, 0, len(failures))
			for _, failure := range failures {
				got = append(got, failure.Context[diagnostics.ContextWord])
			}

			slices.Sort(got)

			// Built the same way as got, so the fully-ignored case compares
			// two empty slices rather than empty against nil.
			want := make([]string, 0, len(testCase.expectWords))
			want = append(want, testCase.expectWords...)
			slices.Sort(want)

			check.Equal(t, want, got, testCase.description)
		})
	}
}

// TestSpellRule_HelpTextTruncation tests the help text truncation for many errors.
func TestSpellRule_MultipleErrors(t *testing.T) {
	t.Parallel()

	// Verify that multiple misspellings produce one error per misspelling,
	// each with the right code and context map fields.
	misspellings := []domain.Misspelling{
		{Word: testWordTeh, Suggestion: testWordThe, Position: 0},
		{Word: "recieve", Suggestion: "receive", Position: 1},       //nolint:misspell
		{Word: "seperate", Suggestion: "separate", Position: 2},     //nolint:misspell
		{Word: "acheive", Suggestion: "achieve", Position: 3},       //nolint:misspell
		{Word: "occured", Suggestion: "occurred", Position: 4},      //nolint:misspell
		{Word: "definately", Suggestion: "definitely", Position: 5}, //nolint:misspell
	}

	checker := &mockSpellChecker{misspellings: misspellings}
	rule := domainrules.NewSpellRule(settings.Config{}, checker)
	commit := minimalSpellCommit("This is teh recieve seperate acheive occured definately") //nolint:misspell

	errors := rule.Validate(commit)

	check.Len(t, errors, len(misspellings), "Should produce one error per misspelling")

	for idx, err := range errors {
		check.Equal(t, diagnostics.ReasonMisspelledWord, reasonOf(err))
		check.Equal(t, misspellings[idx].Word, err.Context[diagnostics.ContextWord])
		check.Equal(t, misspellings[idx].Suggestion, err.Context[diagnostics.ContextSuggestion])
		check.Equal(t, settings.DefaultSpellLocale, err.Context[diagnostics.ContextLocale])
		check.Contains(t, err.Message, misspellings[idx].Word)
		check.Contains(t, err.Message, misspellings[idx].Suggestion)
	}
}

// TestSpellRule_ErrorCodes tests that proper error codes are used.
func TestSpellRule_ErrorCodes(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{}
	commit := minimalSpellCommit("Fix teh issue") //nolint:misspell
	checker := &mockSpellChecker{
		misspellings: []domain.Misspelling{
			{Word: testWordTeh, Suggestion: testWordThe, Position: 4},
		},
	}
	rule := domainrules.NewSpellRule(cfg, checker)

	errors := rule.Validate(commit)
	check.NotEmpty(t, errors, "Should find misspellings")

	for _, spellErr := range errors {
		check.Equal(t, diagnostics.ReasonMisspelledWord, reasonOf(spellErr),
			"Should use correct error code")
		check.Equal(t, rulecatalog.Spell, spellErr.Rule, "Should have correct rule name")
	}
}
