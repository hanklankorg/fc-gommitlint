// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"slices"
)

// Constants for formatting consistency and maintainability.
const (
	resetANSI    = "\033[0m"
	boldANSI     = "\033[1m"
	faintANSI    = "\033[2m"
	themeDracula = "dracula"
)

type themePalette struct {
	success string
	warning string
	error   string
	header  string
	info    string
	muted   string
}

//nolint:gochecknoglobals // Palette registry is immutable runtime data.
var themePalettes = map[string]themePalette{
	"default": {
		success: ansiStandard(32),
		warning: ansiStandard(33),
		error:   ansiStandard(31),
		header:  ansiStandard(36),
		info:    ansiStandard(36),
		muted:   faintANSI,
	},
	"catppuccin-latte": {
		success: ansiRGB(64, 160, 43),
		warning: ansiRGB(223, 142, 29),
		error:   ansiRGB(210, 15, 57),
		header:  ansiRGB(30, 102, 245),
		info:    ansiRGB(30, 102, 245),
		muted:   faintANSI + ansiRGB(156, 160, 176),
	},
	"catppuccin-mocha": {
		success: ansiRGB(166, 227, 161),
		warning: ansiRGB(249, 226, 175),
		error:   ansiRGB(243, 139, 168),
		header:  ansiRGB(137, 180, 250),
		info:    ansiRGB(137, 180, 250),
		muted:   faintANSI + ansiRGB(108, 112, 134),
	},
	themeDracula: {
		success: ansiRGB(80, 250, 123),
		warning: ansiRGB(241, 250, 140),
		error:   ansiRGB(255, 85, 85),
		header:  ansiRGB(139, 233, 253),
		info:    ansiRGB(189, 147, 249),
		muted:   faintANSI + ansiRGB(98, 114, 164),
	},
	"everforest-dark": {
		success: ansiRGB(167, 192, 128),
		warning: ansiRGB(219, 188, 127),
		error:   ansiRGB(230, 126, 128),
		header:  ansiRGB(127, 187, 179),
		info:    ansiRGB(127, 187, 179),
		muted:   faintANSI + ansiRGB(133, 146, 137),
	},
	"everforest-light": {
		success: ansiRGB(141, 161, 1),
		warning: ansiRGB(223, 160, 0),
		error:   ansiRGB(248, 85, 82),
		header:  ansiRGB(58, 148, 197),
		info:    ansiRGB(58, 148, 197),
		muted:   faintANSI + ansiRGB(147, 159, 145),
	},
	"gruvbox-dark": {
		success: ansiRGB(184, 187, 38),
		warning: ansiRGB(250, 189, 47),
		error:   ansiRGB(251, 73, 52),
		header:  ansiRGB(131, 165, 152),
		info:    ansiRGB(131, 165, 152),
		muted:   faintANSI + ansiRGB(146, 131, 116),
	},
	"gruvbox-light": {
		success: ansiRGB(121, 116, 14),
		warning: ansiRGB(181, 118, 20),
		error:   ansiRGB(157, 0, 6),
		header:  ansiRGB(7, 102, 120),
		info:    ansiRGB(7, 102, 120),
		muted:   faintANSI + ansiRGB(124, 111, 100),
	},
	"nord": {
		success: ansiRGB(163, 190, 140),
		warning: ansiRGB(235, 203, 139),
		error:   ansiRGB(191, 97, 106),
		header:  ansiRGB(129, 161, 193),
		info:    ansiRGB(129, 161, 193),
		muted:   faintANSI + ansiRGB(76, 86, 106),
	},
	"onedark": {
		success: ansiRGB(152, 195, 121),
		warning: ansiRGB(229, 192, 123),
		error:   ansiRGB(232, 102, 113),
		header:  ansiRGB(97, 175, 239),
		info:    ansiRGB(97, 175, 239),
		muted:   faintANSI + ansiRGB(92, 99, 112),
	},
	"solarized-dark": {
		success: ansiRGB(133, 153, 0),
		warning: ansiRGB(181, 137, 0),
		error:   ansiRGB(220, 50, 47),
		header:  ansiRGB(38, 139, 210),
		info:    ansiRGB(38, 139, 210),
		muted:   faintANSI + ansiRGB(88, 110, 117),
	},
	"solarized-light": {
		success: ansiRGB(133, 153, 0),
		warning: ansiRGB(181, 137, 0),
		error:   ansiRGB(220, 50, 47),
		header:  ansiRGB(38, 139, 210),
		info:    ansiRGB(38, 139, 210),
		muted:   faintANSI + ansiRGB(147, 161, 161),
	},
	"tokyonight": {
		success: ansiRGB(158, 206, 106),
		warning: ansiRGB(224, 175, 104),
		error:   ansiRGB(247, 118, 142),
		header:  ansiRGB(122, 162, 247),
		info:    ansiRGB(122, 162, 247),
		muted:   faintANSI + ansiRGB(86, 95, 137),
	},
	"tokyonight-day": {
		success: ansiRGB(88, 117, 57),
		warning: ansiRGB(140, 108, 62),
		error:   ansiRGB(245, 42, 101),
		header:  ansiRGB(46, 125, 233),
		info:    ansiRGB(46, 125, 233),
		muted:   faintANSI + ansiRGB(132, 140, 181),
	},
}

// ColorScheme provides consistent color functions for output formatting.
type ColorScheme struct {
	Success   func(a ...any) string
	Warning   func(a ...any) string
	Error     func(a ...any) string
	BoldError func(a ...any) string
	Header    func(a ...any) string
	Bold      func(a ...any) string
	Muted     func(a ...any) string
	Info      func(a ...any) string
	Reset     func(a ...any) string
}

// GetColorScheme creates appropriate color functions based on terminal capability.
func GetColorScheme(useColor bool, theme string) ColorScheme {
	if !useColor {
		return ColorScheme{
			Success:   fmt.Sprint,
			Warning:   fmt.Sprint,
			Error:     fmt.Sprint,
			BoldError: fmt.Sprint,
			Header:    fmt.Sprint,
			Bold:      fmt.Sprint,
			Muted:     fmt.Sprint,
			Info:      fmt.Sprint,
			Reset:     fmt.Sprint,
		}
	}

	palette := getThemePalette(theme)
	wrap := func(prefix string) func(a ...any) string {
		return func(a ...any) string {
			value := fmt.Sprint(a...)
			if value == "" {
				return ""
			}

			return prefix + value + resetANSI
		}
	}

	return ColorScheme{
		Success:   wrap(palette.success),
		Warning:   wrap(palette.warning),
		Error:     wrap(palette.error),
		BoldError: wrap(boldANSI + palette.error),
		Header:    wrap(boldANSI + palette.header),
		Bold:      wrap(boldANSI),
		Muted:     wrap(palette.muted),
		Info:      wrap(palette.info),
		Reset:     func(_ ...any) string { return resetANSI },
	}
}

func getThemePalette(theme string) themePalette {
	normalized := NormalizeThemeName(theme)
	if normalized == "" {
		normalized = DefaultTheme
	}

	palette, ok := themePalettes[normalized]
	if ok {
		return palette
	}

	return themePalettes[DefaultTheme]
}

// SupportedThemes returns the sorted list of valid output theme names.
func SupportedThemes() []string {
	names := make([]string, 0, len(themePalettes))
	for name := range themePalettes {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// IsValidTheme reports whether the given theme name is supported.
func IsValidTheme(theme string) bool {
	normalized := NormalizeThemeName(theme)
	_, ok := themePalettes[normalized]

	return ok
}

func ansiRGB(red, green, blue int) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm", red, green, blue)
}

func ansiStandard(code int) string {
	return fmt.Sprintf("\033[%dm", code)
}
