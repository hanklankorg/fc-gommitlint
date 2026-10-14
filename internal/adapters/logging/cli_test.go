// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestLogger_MaintainsSameInstanceInContext(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	// Initialize logger with specific settings
	loggedCtx := InitLogger(ctx, "text", LogLevelDebugName)
	check.NotNil(t, loggedCtx, "InitLogger should return valid context")

	// Retrieve logger multiple times - should be consistent
	logger1 := GetLogger(loggedCtx)
	logger2 := GetLogger(loggedCtx)

	check.NotNil(t, logger1, "first GetLogger call should return valid logger")
	check.NotNil(t, logger2, "second GetLogger call should return valid logger")
	check.Same(t, logger1, logger2, "GetLogger should return same logger instance")
}

func TestWriterConfig_ConfiguresOutputAndTypeCorrectly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		config       Config
		expectedType WriterType
	}{
		{
			name:         "JSON format",
			config:       NewConfig().WithFormat(OutputJSON),
			expectedType: WriterTypeStdout,
		},
		{
			name:         "Console format",
			config:       NewConfig().WithFormat(OutputConsole),
			expectedType: WriterTypeConsole,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writerConfig := createWriterConfig(testCase.config)

			check.Equal(t, testCase.expectedType, writerConfig.Type)
			// Logs share the process with the report on stdout: a log line
			// on stdout would corrupt a --format json or sarif document.
			check.Same(t, os.Stderr, writerConfig.Output)

			// For Console format, time format is stored in TimeFormat
			if testCase.config.Format == OutputConsole {
				check.Equal(t, testCase.config.TimeFormat, writerConfig.TimeFormat)
			}
		})
	}
}

func TestZerologger_RespectsCallerAndLevelSettings(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config Config
	}{
		{
			name: "Debug level with caller",
			config: NewConfig().
				WithLevel(LogLevelDebug).
				WithCaller(true),
		},
		{
			name: "Info level without caller",
			config: NewConfig().
				WithLevel(LogLevelInfo).
				WithCaller(false),
		},
		{
			name: "Error level with caller",
			config: NewConfig().
				WithLevel(LogLevelError).
				WithCaller(true),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			logger := createZerologger(&buf, testCase.config)

			// Test that logger works - use appropriate level for the test
			if testCase.config.Level == LogLevelError {
				logger.Error().Msg("test message")
			} else {
				logger.Info().Msg("test message")
			}

			// Verify output was written
			check.Positive(t, buf.Len())

			// Parse and verify log entry
			var logged map[string]any

			err := json.Unmarshal(buf.Bytes(), &logged)
			check.NoError(t, err)

			check.Equal(t, "test message", logged["message"])

			if testCase.config.Level == LogLevelError {
				check.Equal(t, LogLevelErrorName, logged["level"])
			} else {
				check.Equal(t, LogLevelInfoName, logged["level"])
			}

			// Both directions. Asserting only that caller appears when it was
			// asked for leaves the setting half-tested: a logger that always
			// attached caller information would satisfy every row.
			if testCase.config.CallerInfo {
				check.Contains(t, logged, logFieldCaller)
			} else {
				check.NotContains(t, logged, logFieldCaller,
					"caller must be absent when the setting is off")
			}
		})
	}
}

// JSON lines must carry exactly one timestamp, in the configured format: the
// hook that formats it replaces Context.Timestamp(), it does not add to it.
func TestZerologger_JSONWritesOneTimestampInConfiguredFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	logger := createZerologger(&buf, NewConfig().WithFormat(OutputJSON).WithTimeFormat(time.RFC3339))
	logger.Info().Msg("hello")

	var line map[string]any

	check.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	check.Equal(t, 1, strings.Count(buf.String(), `"time":`), "one time field per line")

	stamp, ok := line["time"].(string)
	check.True(t, ok, "time must be a string")
	check.True(t, strings.HasSuffix(stamp, "Z"), "time must be UTC RFC3339: %s", stamp)
}

// Log output is diagnostics on stderr, which is routinely redirected into a
// file or a CI log. The console writer used to colour unconditionally, so a
// redirected stderr collected raw escape sequences and NO_COLOR was ignored.
func TestLogColor_StaysOffUnlessStderrIsAnOptedInTerminal(t *testing.T) {
	sink, err := os.CreateTemp(t.TempDir(), "log")
	check.NoError(t, err)

	t.Cleanup(func() { _ = sink.Close() })

	t.Run("piped output is never coloured", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", "xterm-256color")

		check.False(t, logColorEnabled(sink), "a regular file is not a terminal")
	})

	t.Run("NO_COLOR wins", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("TERM", "xterm-256color")

		check.False(t, logColorEnabled(sink))
	})

	t.Run("TERM=dumb wins", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("TERM", termDumb)

		check.False(t, logColorEnabled(sink))
	})

	t.Run("the console writer config carries the decision", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		check.True(t, createWriterConfig(NewConfig().WithFormat(OutputConsole)).NoColor)
	})
}
