// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"fmt"
	"regexp"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Pre-compiled regex pattern for removing multiple consecutive spaces.
var multipleSpacesPattern = regexp.MustCompile(`\s+`)

// SpellRule validates spelling in commit messages.
type SpellRule struct {
	checker     SpellChecker
	ignoreWords []string
	locale      string
}

// NewSpellRule creates a new SpellRule with the provided checker.
func NewSpellRule(cfg settings.Config, checker SpellChecker) SpellRule {
	return SpellRule{
		checker:     checker,
		ignoreWords: cfg.Spell.IgnoreWords,
		locale:      settings.DerefString(cfg.Spell.Locale, settings.DefaultSpellLocale),
	}
}

// Name implements Rule interface.
func (r SpellRule) Name() string {
	return rulecatalog.Spell
}

// Validate checks commit message text for spelling errors using configured dictionary.
func (r SpellRule) Validate(commit domain.Commit) []domain.ValidationError {
	textToCheck := preprocessText(commit.Subject + " " + commit.Body)

	// Skip spell check if text is empty after preprocessing
	if strings.TrimSpace(textToCheck) == "" {
		return nil
	}

	// Extract misspellings using interface
	misspellings := r.checker.CheckText(textToCheck)
	filteredMisspellings := filterIgnoredWords(misspellings, r.ignoreWords)

	return buildSpellErrors(filteredMisspellings, r.Name(), r.locale)
}

// preprocessText prepares text for spell checking by cleaning up special characters.
func preprocessText(text string) string {
	// Remove markdown-style headers
	lines := strings.Split(text, "\n")
	processedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		// Skip lines that are just headers
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		processedLines = append(processedLines, line)
	}

	text = strings.Join(processedLines, " ")

	// Replace various separators with spaces
	replacer := strings.NewReplacer(
		"[", " ",
		"]", " ",
		"(", " ",
		")", " ",
		":", " ",
		"/", " ",
		"-", " ",
		"_", " ",
	)
	text = replacer.Replace(text)

	// Normalize spacing for accurate spell checking
	text = multipleSpacesPattern.ReplaceAllString(text, " ")

	return strings.TrimSpace(text)
}

// filterIgnoredWords excludes configured words from spell checking.
func filterIgnoredWords(misspellings []domain.Misspelling, ignoreWords []string) []domain.Misspelling {
	if len(ignoreWords) == 0 {
		return misspellings
	}

	// Create ignore map for efficient lookup
	ignoreMap := make(map[string]bool, len(ignoreWords))
	for _, word := range ignoreWords {
		ignoreMap[strings.ToLower(word)] = true
	}

	filtered := make([]domain.Misspelling, 0, len(misspellings))

	for _, misspelling := range misspellings {
		if !ignoreMap[strings.ToLower(misspelling.Word)] {
			filtered = append(filtered, misspelling)
		}
	}

	return filtered
}

// buildSpellErrors converts misspellings to domain validation errors with rich context.
func buildSpellErrors(misspellings []domain.Misspelling, ruleName string, locale string) []domain.ValidationError {
	if len(misspellings) == 0 {
		return nil
	}

	errors := make([]domain.ValidationError, 0, len(misspellings))

	for _, misspelling := range misspellings {
		err := domain.New(ruleName, diagnostics.ReasonMisspelledWord,
			fmt.Sprintf("Misspelled word: '%s' (suggestion: '%s')", misspelling.Word, misspelling.Suggestion)).
			WithContextMap(diagnostics.Context{
				diagnostics.ContextWord:       misspelling.Word,
				diagnostics.ContextSuggestion: misspelling.Suggestion,
				diagnostics.ContextLocale:     locale,
			})

		errors = append(errors, err)
	}

	return errors
}
