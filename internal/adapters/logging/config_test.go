// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/rs/zerolog"
)

const (
	logTestNameDebug = "debug level"
	logTestNameError = "error level"
	logTestNameInfo  = "info level"
	logTestNameTrace = "trace level"
	logTestNameWarn  = "warn level"
)

func TestLogLevel_ParsesAllValidStrings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected LogLevel
	}{
		{logTestNameError, LogLevelErrorName, LogLevelError},
		{logTestNameWarn, LogLevelWarnName, LogLevelWarn},
		{logTestNameInfo, LogLevelInfoName, LogLevelInfo},
		{logTestNameDebug, LogLevelDebugName, LogLevelDebug},
		{logTestNameTrace, LogLevelTraceName, LogLevelTrace},
		{"invalid level", "invalid", LogLevelInfo},
		{"empty string", "", LogLevelInfo},
		{"upper case is accepted", "ERROR", LogLevelError},
		{"surrounding whitespace is ignored", " Debug ", LogLevelDebug},
		// The aliases IsValidLogLevel lets through the CLI. "information" is
		// also the fallback value, so only "warning" can observe the alias.
		{"warning alias", "warning", LogLevelWarn},
		{"information alias", "information", LogLevelInfo},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := parseLogLevel(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestLogLevel_MapsToExpectedZerologLevels(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    LogLevel
		expected zerolog.Level
	}{
		{logTestNameError, LogLevelError, zerolog.ErrorLevel},
		{logTestNameWarn, LogLevelWarn, zerolog.WarnLevel},
		{logTestNameInfo, LogLevelInfo, zerolog.InfoLevel},
		{logTestNameDebug, LogLevelDebug, zerolog.DebugLevel},
		{logTestNameTrace, LogLevelTrace, zerolog.TraceLevel},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testCase.input.toZerologLevel()
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestFormatLevel_RendersLevelPrefixes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    any
		expected string
	}{
		{logTestNameTrace, LogLevelTraceName, colorCyan + "TRACE:" + colorReset},
		{logTestNameDebug, LogLevelDebugName, colorGray + "DEBUG:" + colorReset},
		{logTestNameInfo, LogLevelInfoName, ""},
		{"warn level", LogLevelWarnName, colorYellow + "warning:" + colorReset},
		{logTestNameError, LogLevelErrorName, colorRed + "error:" + colorReset},
		{"unknown string level", "unknown", colorWhite + "unknown" + colorReset},
		{"non-string input", 123, "%!s(int=123)"},
		{"nil input", nil, "%!s(<nil>)"},
		{"empty string", "", colorWhite + "" + colorReset},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := formatLevel(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// plainFormatLevel is what a redirected stderr gets. The console writer's
// NoColor setting cannot strip this token, because the token is produced by
// the formatter itself, so the formatter has to be chosen for the sink.
func TestPlainFormatLevel_RendersLevelPrefixesWithoutEscapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    any
		expected string
	}{
		{logTestNameTrace, LogLevelTraceName, "TRACE:"},
		{logTestNameDebug, LogLevelDebugName, "DEBUG:"},
		{logTestNameInfo, LogLevelInfoName, ""},
		{"warn level", LogLevelWarnName, "warning:"},
		{logTestNameError, LogLevelErrorName, "error:"},
		{"unknown string level", "unknown", "unknown"},
		{"non-string input", 123, "%!s(int=123)"},
		{"nil input", nil, "%!s(<nil>)"},
		{"empty string", "", ""},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := plainFormatLevel(testCase.input)

			check.Equal(t, testCase.expected, result)
			check.NotContains(t, result, "\x1b")
		})
	}
}
