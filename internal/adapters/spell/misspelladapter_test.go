// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package spell

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

const (
	misspelledSuggestion = "misspelled"
	misspelledWord       = "mis" + "pelled"
)

func TestMisspellAdapter_AcceptsUSAndUKLocales(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		locale      string
		description string
	}{
		{
			name:        "US locale",
			locale:      "US",
			description: "should create adapter for US English",
		},
		{
			name:        "US locale lowercase",
			locale:      "us",
			description: "should create adapter for US English (case-insensitive)",
		},
		{
			name:        "UK locale",
			locale:      "UK",
			description: "should create adapter for UK English",
		},
		{
			name:        "UK locale lowercase",
			locale:      "uk",
			description: "should create adapter for UK English (case-insensitive)",
		},
		{
			name:        "GB locale",
			locale:      "GB",
			description: "should create adapter for GB English",
		},
		{
			name:        "GB locale lowercase",
			locale:      "gb",
			description: "should create adapter for GB English (case-insensitive)",
		},
		{
			name:        "empty locale uses neutral English",
			locale:      "",
			description: "should use neutral English for empty locale",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			adapter, err := NewMisspellAdapter(testCase.locale)

			check.NoError(t, err, testCase.description)
			check.NotNil(t, adapter, testCase.description)
			check.NotNil(t, adapter.replacer, "replacer should be initialized")
		})
	}
}

func TestMisspellAdapter_RejectsInvalidLocales(t *testing.T) {
	t.Parallel()

	invalidLocales := []string{"EN", "AU", "CA", "NZ", "invalid", "123", "  "}

	for _, locale := range invalidLocales {
		t.Run(locale, func(t *testing.T) {
			t.Parallel()

			adapter, err := NewMisspellAdapter(locale)

			check.Error(t, err)
			check.ErrorIs(t, err, ErrInvalidLocale)
			check.Nil(t, adapter)
		})
	}
}

func TestMisspellAdapter_CheckText(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		locale               string
		text                 string
		expectedMisspellings []domain.Misspelling
		description          string
	}{
		{
			name:                 "empty text",
			locale:               "US",
			text:                 "",
			expectedMisspellings: []domain.Misspelling{},
			description:          "should return no misspellings for empty text",
		},
		{
			name:                 "correct text",
			locale:               "US",
			text:                 "This is a correct sentence.",
			expectedMisspellings: []domain.Misspelling{},
			description:          "should return no misspellings for correct text",
		},
		{
			name:   "single misspelling",
			locale: "US",
			text:   "This is a mispelled word.", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       misspelledWord,
					Position:   10,
					Suggestion: misspelledSuggestion,
				},
			},
			description: "should detect single misspelling",
		},
		{
			name:   "multiple misspellings",
			locale: "US",
			text:   "This is mispelled and recieve text.", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       misspelledWord,
					Position:   8,
					Suggestion: misspelledSuggestion,
				},
				{
					Word:       "recieve", //nolint: misspell
					Position:   22,
					Suggestion: "receive",
				},
			},
			description: "should detect multiple misspellings",
		},
		{
			name:   "common programming terms",
			locale: "US",
			text:   "refacor the funtion to imporve performence", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       "funtion", //nolint: misspell
					Position:   12,
					Suggestion: "function",
				},
				{
					Word:       "imporve", //nolint: misspell
					Position:   23,
					Suggestion: "improve",
				},
				{
					Word:       "performence", //nolint: misspell
					Position:   31,
					Suggestion: "performance",
				},
			},
			description: "should detect misspelled programming terms",
		},
		{
			name:                 "technical terms that are correct",
			locale:               "US",
			text:                 "implement authentication with JWT tokens",
			expectedMisspellings: []domain.Misspelling{
				// JWT, authentication are technical terms but "implement" should be fine
			},
			description: "should not flag common technical terms",
		},
		{
			name:                 "US vs UK spelling differences",
			locale:               "US",
			text:                 "The color of the program is gray.",
			expectedMisspellings: []domain.Misspelling{
				// US spelling should be fine
			},
			description: "should accept US spelling in US locale",
		},
		{
			name:   "mixed case text",
			locale: "US",
			text:   "MISPELLED word in UPPERCASE and lowercase", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       "MISPELLED", //nolint: misspell
					Position:   0,
					Suggestion: "MISSPELLED",
				},
			},
			description: "should handle mixed case text correctly",
		},
		{
			name:                 "whitespace only text",
			locale:               "US",
			text:                 "   \t\n  ",
			expectedMisspellings: []domain.Misspelling{},
			description:          "should handle whitespace only text",
		},
		{
			name:   "text with punctuation",
			locale: "US",
			text:   "This is mispelled, isn't it?", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       "mispelled", //nolint: misspell
					Position:   8,
					Suggestion: "misspelled",
				},
			},
			description: "should handle text with punctuation correctly",
		},
		{
			name:   "text with numbers",
			locale: "US",
			text:   "Version 1.0 has a mispelling in line 42", //nolint: misspell
			expectedMisspellings: []domain.Misspelling{
				{
					Word:       "mispelling", //nolint: misspell
					Position:   18,
					Suggestion: "misspelling",
				},
			},
			description: "should handle text with numbers correctly",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			adapter, err := NewMisspellAdapter(testCase.locale)
			check.NoError(t, err)

			misspellings := adapter.CheckText(testCase.text)

			check.Equal(t, testCase.expectedMisspellings, misspellings, testCase.description)
		})
	}
}

// TestMisspellAdapter_LocaleControlsPreferredSpelling pins that the locale
// decides which regional spelling is flagged: each locale corrects the other
// region's spelling and accepts its own.
func TestMisspellAdapter_LocaleControlsPreferredSpelling(t *testing.T) {
	t.Parallel()

	usAdapter, err := NewMisspellAdapter("US")
	check.NoError(t, err)

	check.Equal(t,
		[]domain.Misspelling{{Word: "colour", Position: 4, Suggestion: "color"}},
		usAdapter.CheckText("The colour is nice."),
		"US locale should correct UK spelling")
	check.Equal(t, []domain.Misspelling{},
		usAdapter.CheckText("The color is nice."),
		"US locale should accept US spelling")

	// UK and GB name the same dialect; both must prefer British spelling.
	for _, locale := range []string{"UK", "GB"} {
		britishAdapter, err := NewMisspellAdapter(locale)
		check.NoError(t, err)

		check.Equal(t,
			[]domain.Misspelling{{Word: "color", Position: 4, Suggestion: "colour"}},
			britishAdapter.CheckText("The color is nice."),
			"%s locale should correct US spelling", locale)
		check.Equal(t, []domain.Misspelling{},
			britishAdapter.CheckText("The colour is nice."),
			"%s locale should accept UK spelling", locale)
	}
}

func TestMisspellAdapter_VariousTextFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		text        string
		description string
	}{
		{
			name:        "very long text",
			text:        "This is a very long text with many words that should be processed correctly without any performance issues or memory problems " + strings.Repeat("repeated ", 100) + "mispelled word at the end", //nolint: misspell
			description: "should handle very long text efficiently",
		},
		{
			name:        "text with special characters",
			text:        "Special chars: @#$%^&*()_+-=[]{}|;':\",./<>? and mispelled word", //nolint: misspell
			description: "should handle text with special characters",
		},
		{
			name:        "text with unicode characters",
			text:        "Unicode: résumé café naïve and mispelled word", //nolint: misspell
			description: "should handle unicode characters",
		},
		{
			name:        "text with line breaks",
			text:        "First line\nSecond line with mispelled word\nThird line", //nolint: misspell
			description: "should handle text with line breaks",
		},
		{
			name:        "text with tabs",
			text:        "Text\twith\ttabs\tand\tmispelled\tword", //nolint: misspell
			description: "should handle text with tabs",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			adapter, err := NewMisspellAdapter("US")
			check.NoError(t, err)

			misspellings := adapter.CheckText(testCase.text)

			// Every row embeds the same incorrect token; the formats around it
			// must not stop the checker from finding and correcting it.
			found := false

			for _, m := range misspellings {
				if m.Word == "mispelled" { //nolint: misspell
					found = true

					check.Equal(t, "misspelled", m.Suggestion, "should suggest correct spelling")

					break
				}
			}

			check.True(t, found, "%s: expected 'mispelled' to be detected in %q", testCase.description, testCase.text)
		})
	}
}

// The spell rule consumes the adapter through rules.SpellChecker.
var _ rules.SpellChecker = &MisspellAdapter{}

func TestMisspellAdapter_ConsistentResultsConcurrently(t *testing.T) {
	t.Parallel()

	adapter, err := NewMisspellAdapter("US")
	check.NoError(t, err)

	text := "This contains mispelled words" //nolint: misspell

	// Anchor what "consistent" is allowed to mean before comparing runs
	// against each other. Agreement on its own is not evidence: a CheckText
	// that returned nil every time, or that lost its dictionary entirely,
	// would satisfy ten identical results perfectly. The comparison below is
	// only worth making once we know each run is finding the real answer.
	expected := []domain.Misspelling{
		{Word: misspelledWord, Position: 14, Suggestion: misspelledSuggestion},
	}
	check.Equal(t, expected, adapter.CheckText(text), "the sequential result the concurrent runs must agree WITH")

	// Test concurrent access
	results := make(chan []domain.Misspelling, 10)

	for range 10 {
		go func() {
			results <- adapter.CheckText(text)
		}()
	}

	// Collect results
	allResults := make([][]domain.Misspelling, 0, 10)
	for range 10 {
		allResults = append(allResults, <-results)
	}

	// Identical means equal, not merely the same length: a race that swapped
	// one finding for another would keep the count and change the content.
	for i := range allResults {
		check.Equal(t, expected, allResults[i], "concurrent result %d should match the sequential result", i)
	}
}

// TestCheckText_PositionIsAByteColumnWithinItsLine pins what Position actually
// means, because the field name does not say and the answer is surprising.
//
// The adapter sets Position from misspell's diff.Column and discards diff.Line.
// Column is a BYTE offset counted from the start of ITS OWN LINE, so on
// multi-line input the line number is simply lost: the two findings below both
// report Position 0 and become indistinguishable, and any caller treating
// Position as an offset into the whole text points at the wrong word.
//
// This is not a live bug. The only consumer, rules.SpellRule, runs its text
// through preprocessText first, which joins every line with a space — so what
// reaches CheckText is single-line by construction and Column is a whole-text
// offset by accident. I checked that before writing this rather than reporting
// a vulnerability that cannot be reached.
//
// It is a trap worth a test, though, precisely because the safety is accidental
// and lives in a different package. Joining a commit body with spaces destroys
// its structure, so "preserve the line breaks" is a plausible future change to
// preprocessText — and it would silently invalidate every reported position
// with nothing here to notice. This test makes that coupling fail loudly.
func TestCheckText_PositionIsAByteColumnWithinItsLine(t *testing.T) {
	t.Parallel()

	adapter, err := NewMisspellAdapter("US")
	check.NoError(t, err)

	t.Run("the line number is not encoded in Position", func(t *testing.T) {
		t.Parallel()

		check.Equal(t,
			[]domain.Misspelling{
				{Word: misspelledWord, Position: 0, Suggestion: misspelledSuggestion},
				{Word: misspelledWord, Position: 0, Suggestion: misspelledSuggestion},
			},
			adapter.CheckText("mispelled line one\nmispelled line two"), //nolint: misspell
			"both lines start at column 0, so the findings collapse onto the same Position")
	})

	t.Run("Position counts bytes, not runes", func(t *testing.T) {
		t.Parallel()

		// "resume cafe naive " with accents is 22 bytes but only 18 runes.
		const prefix = "résumé café naïve "

		check.Equal(t, 22, len(prefix), "guard the fixture: the byte length is the point of this row")
		check.Equal(t, 18, len([]rune(prefix)), "guard the fixture: the rune length must differ from the byte length")

		check.Equal(t,
			[]domain.Misspelling{
				{Word: misspelledWord, Position: 22, Suggestion: misspelledSuggestion},
			},
			adapter.CheckText(prefix+misspelledWord),
			"a caller slicing by rune index would cut the word in the wrong place")
	})
}
