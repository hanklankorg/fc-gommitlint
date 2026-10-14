// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package spell

import (
	"errors"
	"fmt"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"github.com/golangci/misspell"
)

// ErrInvalidLocale indicates an unsupported spell check locale was specified.
var ErrInvalidLocale = errors.New("invalid spell check locale")

// MisspellAdapter implements the rules.SpellChecker interface using the misspell library.
// The interface is defined in the rules package (consumer side), following Go idioms.
// The misspell.Replacer is thread-safe after compilation, so no mutex is needed.
type MisspellAdapter struct {
	replacer *misspell.Replacer
}

// Compile-time interface compliance check.
var _ rules.SpellChecker = (*MisspellAdapter)(nil)

// NewMisspellAdapter creates a new spell checker adapter configured for the specified locale.
//
// Supported locales (case-insensitive):
//
//   - "both" — accept both US and UK spellings. Recommended default for global
//     teams: catches genuine misspellings without flagging cross-dialect words
//     ("color" and "colour" both pass).
//   - "US" — American English. Flags British dialect words.
//   - "UK" / "GB" — British English. Flags American dialect words.
//   - "" — backward-compatible alias for "both".
//
// Returns ErrInvalidLocale for unsupported locales.
func NewMisspellAdapter(locale string) (*MisspellAdapter, error) {
	replacer := misspell.New()

	switch strings.ToLower(locale) {
	case "us":
		// American English - convert British spellings to American
		replacer.AddRuleList(misspell.DictAmerican)
	case "uk", "gb":
		// British English - convert American spellings to British
		replacer.AddRuleList(misspell.DictBritish)
	case "", "both":
		// Neutral mode: the misspell library's default rules flag words that are
		// misspelled in BOTH variants. Dialect-specific variants pass.
	default:
		return nil, fmt.Errorf("%w: %q (supported: both, US, UK, GB)", ErrInvalidLocale, locale)
	}

	replacer.Compile()

	return &MisspellAdapter{
		replacer: replacer,
	}, nil
}

// CheckText implements the rules.SpellChecker interface by checking text for misspellings.
func (a *MisspellAdapter) CheckText(text string) []domain.Misspelling {
	// No mutex needed - misspell.Replacer is immutable after Compile()
	_, diffs := a.replacer.Replace(text)

	misspellings := make([]domain.Misspelling, 0, len(diffs))

	for _, diff := range diffs {
		misspellings = append(misspellings, domain.Misspelling{
			Word:       diff.Original,
			Position:   diff.Column,
			Suggestion: diff.Corrected,
		})
	}

	return misspellings
}
