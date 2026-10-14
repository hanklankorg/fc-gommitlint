// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"slices"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSupportedFormats_ReturnsSortedClosedSet(t *testing.T) {
	t.Parallel()

	formats := SupportedFormats()

	// Should return all supported formats
	check.Len(t, formats, 8)

	// Should already be sorted
	expected := []string{
		FormatForgejo,
		FormatGitHub,
		FormatGitLab,
		FormatJSON,
		FormatPlain,
		FormatSARIF,
		FormatTable,
		FormatText,
	}
	check.Equal(t, expected, formats)

	// Verify it's actually sorted
	sortedCopy := slices.Clone(formats)
	slices.Sort(sortedCopy)
	check.Equal(t, sortedCopy, formats, "SupportedFormats should return sorted list")
}

func TestIsValidFormat_IsCaseSensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format string
		valid  bool
	}{
		{FormatText, true},
		{FormatPlain, true},
		{FormatJSON, true},
		{FormatGitHub, true},
		{FormatGitLab, true},
		{FormatForgejo, true},
		{"xml", false},
		{"yaml", false},

		{"", false},
		{"TEXT", false}, // case sensitive
		{"Json", false}, // case sensitive
	}

	for _, testCase := range tests {
		t.Run(testCase.format, func(t *testing.T) {
			t.Parallel()

			result := IsValidFormat(testCase.format)
			check.Equal(t, testCase.valid, result)
		})
	}
}

// TestIsGitHubCompatibleFormat_IsTrueForGitHubAndForgejoOnly is the truth table
// validate.go branches on to decide whether workflow commands may be emitted.
// It walks every supported format so a format added without a classification
// lands in the false rows, plus the unknown and wrong-case spellings.
func TestIsGitHubCompatibleFormat_IsTrueForGitHubAndForgejoOnly(t *testing.T) {
	t.Parallel()

	compatible := map[string]bool{FormatGitHub: true, FormatForgejo: true}

	for _, format := range append(SupportedFormats(), "", "GitHub", "xml") {
		t.Run("format_"+format, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, compatible[format], IsGitHubCompatibleFormat(format))
		})
	}
}

func TestFormatPlain_IsMachineReadable(t *testing.T) {
	t.Parallel()

	check.True(t, IsMachineReadableFormat(FormatPlain))
	check.False(t, IsHumanReadableFormat(FormatPlain))
}

func TestNormalizeThemeName_LowercasesTrimsAndUnquotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "lowercase", input: themeDracula, expected: themeDracula},
		{name: "uppercase", input: "DRACULA", expected: themeDracula},
		{name: "trim_whitespace", input: "  nord  ", expected: "nord"},
		{name: "quoted", input: "'tokyonight'", expected: "tokyonight"},
		{name: "empty", input: "", expected: ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := NormalizeThemeName(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestSupportedFormats_ConsistentOutput(t *testing.T) {
	t.Parallel()

	// SupportedFormats should produce consistent output
	formats1 := SupportedFormats()
	formats2 := SupportedFormats()
	check.Equal(t, formats1, formats2)

	// Modifying returned slice should not affect subsequent calls
	formats1[0] = "modified"
	formats3 := SupportedFormats()
	check.NotEqual(t, formats1, formats3)
	check.Equal(t, formats2, formats3)
}
