// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestColorScheme_WrapsTextInThemeANSIWhenColorsEnabled is the positive half of
// the colour contract: with colours on, each function must wrap its argument in
// that role's ANSI prefix and the reset sequence.
func TestColorScheme_WrapsTextInThemeANSIWhenColorsEnabled(t *testing.T) {
	t.Parallel()

	colors := GetColorScheme(true, "")
	palette := themePalettes[DefaultTheme]

	testCases := []struct {
		name    string
		colorFn func(a ...any) string
		input   string
		prefix  string
	}{
		{"Success color function", colors.Success, testInputTestMessage, palette.success},
		{"Warning color function", colors.Warning, testInputWarningText, palette.warning},
		{"Error color function", colors.Error, testInputErrorText, palette.error},
		{"Header color function", colors.Header, testInputHeaderText, boldANSI + palette.header},
		{"Bold color function", colors.Bold, testInputBoldText, boldANSI},
		{"Muted color function", colors.Muted, testInputMutedText, palette.muted},
		{"BoldError color function", colors.BoldError, testInputErrorText, boldANSI + palette.error},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.colorFn(testCase.input)

			check.Equal(t, testCase.prefix+testCase.input+resetANSI, result)
		})
	}
}

func TestColorScheme_SkipsColorsWhenDisabled(t *testing.T) {
	t.Parallel()

	colors := GetColorScheme(false, "")

	testCases := []struct {
		name    string
		colorFn func(a ...any) string
		input   string
	}{
		{"Success no-color", colors.Success, testInputTestMessage},
		{"Warning no-color", colors.Warning, testInputWarningText},
		{"Error no-color", colors.Error, testInputErrorText},
		{"Header no-color", colors.Header, testInputHeaderText},
		{"Bold no-color", colors.Bold, testInputBoldText},
		{"Muted no-color", colors.Muted, testInputMutedText},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.colorFn(testCase.input)

			// Without colors, output should be exactly the input
			check.Equal(t, testCase.input, result)
		})
	}
}

func TestColorScheme_ConcatenatesMultipleArguments(t *testing.T) {
	t.Parallel()

	result := GetColorScheme(true, "").Success("Hello", " ", "World")
	check.Equal(t, themePalettes[DefaultTheme].success+"Hello World"+resetANSI, result)

	resultNoColor := GetColorScheme(false, "").Success("Hello", " ", "World")
	check.Equal(t, "Hello World", resultNoColor)
}

// Empty input stays empty rather than becoming a bare prefix-and-reset pair,
// which would print an escape sequence around nothing.
func TestColorScheme_ReturnsEmptyForEmptyInput(t *testing.T) {
	t.Parallel()

	check.Empty(t, GetColorScheme(true, "").Success(""))
	check.Empty(t, GetColorScheme(false, "").Success(""))
}
