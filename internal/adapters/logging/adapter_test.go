// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/rs/zerolog"
)

const (
	logFieldKey2   = "key2"
	logFieldValue1 = "value1"
	logFieldValue2 = "value2"
)

// testLogMethod tests logging methods with various argument patterns.
func testLogMethod(t *testing.T, methodName string, logLevel zerolog.Level, logFunc func(*Logger, string, ...any)) {
	t.Helper()

	testCases := []struct {
		name     string
		msg      string
		args     []any
		expected map[string]any
	}{
		{
			name:     methodName + " without args",
			msg:      methodName + " message",
			args:     nil,
			expected: map[string]any{logFieldLevel: methodName, logFieldMessage: methodName + " message"},
		},
		{
			name: methodName + " with key-value args",
			msg:  methodName + " with data",
			args: []any{logFieldKey1, logFieldValue1, logFieldKey2, 42},
			expected: map[string]any{
				logFieldLevel:   methodName,
				logFieldMessage: methodName + " with data",
				logFieldKey1:    logFieldValue1,
				logFieldKey2:    float64(42), // JSON unmarshals numbers as float64
			},
		},
		{
			name: methodName + " with odd args",
			msg:  methodName + " odd args",
			args: []any{logFieldKey1, logFieldValue1, "orphaned"},
			expected: map[string]any{
				logFieldLevel:   methodName,
				logFieldMessage: methodName + " odd args",
				logFieldKey1:    logFieldValue1,
				"orphaned":      "", // Padded with empty string
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			zerologLogger := zerolog.New(&buf).Level(logLevel)
			logger := &Logger{logger: zerologLogger}

			logFunc(logger, testCase.msg, testCase.args...)

			var logged map[string]any

			err := json.Unmarshal(buf.Bytes(), &logged)
			check.NoError(t, err)

			for key, expectedValue := range testCase.expected {
				check.Equal(t, expectedValue, logged[key])
			}
		})
	}
}

func TestLogger_Trace(t *testing.T) {
	t.Parallel()

	testLogMethod(t, "trace", zerolog.TraceLevel, func(l *Logger, msg string, args ...any) {
		l.Trace(msg, args...)
	})
}

func TestLogger_Debug(t *testing.T) {
	t.Parallel()

	testLogMethod(t, "debug", zerolog.DebugLevel, func(l *Logger, msg string, args ...any) {
		l.Debug(msg, args...)
	})
}

func TestLogger_Warn(t *testing.T) {
	t.Parallel()

	testLogMethod(t, "warn", zerolog.WarnLevel, func(l *Logger, msg string, args ...any) {
		l.Warn(msg, args...)
	})
}

func TestLogger_Info(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		msg      string
		args     []any
		expected map[string]any
	}{
		{
			name:     "Info without args",
			msg:      "info message",
			args:     nil,
			expected: map[string]any{logFieldLevel: logLevelInfo, logFieldMessage: "info message"},
		},
		{
			name: "Info with key-value args",
			msg:  "info with data",
			args: []any{"user", "john", logFieldCount, 5},
			expected: map[string]any{
				logFieldLevel:   logLevelInfo,
				logFieldMessage: "info with data",
				"user":          "john",
				logFieldCount:   float64(5),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			zerologLogger := zerolog.New(&buf)
			logger := New(zerologLogger)

			logger.Info(testCase.msg, testCase.args...)

			var logged map[string]any

			err := json.Unmarshal(buf.Bytes(), &logged)
			check.NoError(t, err)

			for key, expectedValue := range testCase.expected {
				check.Equal(t, expectedValue, logged[key])
			}
		})
	}
}

func TestLogger_Error(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		msg      string
		args     []any
		expected map[string]any
	}{
		{
			name:     "Error without args",
			msg:      "error occurred",
			args:     nil,
			expected: map[string]any{logFieldLevel: logLevelError, logFieldMessage: "error occurred"},
		},
		{
			name: "Error with context",
			msg:  "validation failed",
			args: []any{"field", "email", "reason", "invalid format"},
			expected: map[string]any{
				logFieldLevel:   logLevelError,
				logFieldMessage: "validation failed",
				"field":         "email",
				"reason":        "invalid format",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			zerologLogger := zerolog.New(&buf)
			logger := New(zerologLogger)

			logger.Error(testCase.msg, testCase.args...)

			var logged map[string]any

			err := json.Unmarshal(buf.Bytes(), &logged)
			check.NoError(t, err)

			for key, expectedValue := range testCase.expected {
				check.Equal(t, expectedValue, logged[key])
			}
		})
	}
}

func TestLogger_AddFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		args     []any
		expected map[string]any
	}{
		{
			name:     "Even number of args",
			args:     []any{logFieldKey1, logFieldValue1, logFieldKey2, logFieldValue2},
			expected: map[string]any{logFieldKey1: logFieldValue1, logFieldKey2: logFieldValue2},
		},
		{
			name:     "Odd number of args - padded with empty string",
			args:     []any{logFieldKey1, logFieldValue1, logFieldKey2},
			expected: map[string]any{logFieldKey1: logFieldValue1, logFieldKey2: ""},
		},
		{
			name:     "Empty args",
			args:     []any{},
			expected: map[string]any{},
		},
		{
			name:     "Single arg - key without value",
			args:     []any{"lonely_key"},
			expected: map[string]any{"lonely_key": ""},
		},
		{
			name:     "Mixed types",
			args:     []any{"string_key", "string_val", "int_key", 42, "bool_key", true},
			expected: map[string]any{"string_key": "string_val", "int_key": float64(42), "bool_key": true},
		},
		{
			name:     "Nil values",
			args:     []any{"nil_key", nil, "normal_key", "normal_value"},
			expected: map[string]any{"nil_key": nil, "normal_key": "normal_value"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			zerologLogger := zerolog.New(&buf)
			logger := New(zerologLogger)

			// Use Info to test addFields functionality
			logger.Info("test message", testCase.args...)

			var logged map[string]any

			err := json.Unmarshal(buf.Bytes(), &logged)
			check.NoError(t, err)

			check.Equal(t, "test message", logged[logFieldMessage])

			for key, expectedValue := range testCase.expected {
				check.Equal(t, expectedValue, logged[key])
			}
		})
	}
}

func TestLogger_IndependentState(t *testing.T) {
	t.Parallel()

	// addFields pads an odd-length argument list with append, and append
	// writes in place whenever the slice has spare capacity — so the caller's
	// array is what is at stake, not the slice header they handed over.
	//
	// The aliasing is the whole point of this fixture, and the previous form
	// could not see it. Building the input with make([]any, n) gives capacity
	// equal to length, which forces append to reallocate; the caller's memory
	// then cannot be touched no matter what addFields does, and the test
	// passes unconditionally. It could not fail for the reason it exists.
	//
	// Here the odd-length args are a PREFIX of a longer slice, which is the
	// shape a caller reaches by writing logger.Info(msg, full[:3]...). The
	// element just past the end is the one an in-place pad would clobber.
	full := []any{"key", "value", logFieldCount, "caller data past the end"}
	originalArgs := append([]any(nil), full...)
	args := full[:3]

	var buf bytes.Buffer

	New(zerolog.New(&buf)).Info("test", args...)

	check.Equal(t, originalArgs, full,
		"logging must not write its padding into the caller's array")
}

// fieldsOf returns the log record's key set, so a test can say what a record
// contains AND what it does not. The tables above only ever assert that the
// keys they expect are present with the right values, which cannot notice a
// field nobody asked for — and an unasked-for field in a log line is exactly
// how secrets escape.
func fieldsOf(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var logged map[string]any

	check.NoError(t, json.Unmarshal(raw, &logged))

	return logged
}

func keysOf(logged map[string]any) []string {
	keys := make([]string, 0, len(logged))
	for key := range logged {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

// TestAddFields_RejectsUnsafeKeysAndRenamesReservedOnes covers
// validateAndFormatKey, which had no test at all despite being the only thing
// standing between caller-supplied keys and the structured log format.
//
// Two separate jobs, both untested until now. Keys that could forge log
// structure — nil, blank, or carrying a newline, carriage return or tab — are
// dropped along with their values, because a key containing a newline lets a
// caller write what looks like a second log entry. Keys that collide with
// zerolog's own fields are renamed with a _user suffix rather than dropped, so
// a caller cannot overwrite the real level, timestamp, message or caller while
// still getting their data recorded.
//
// Each row asserts the exact key set, not just the presence of what it expects.
// A "the bad key is absent" assertion alone would be satisfied by a logger that
// dropped every field, so every row also names what must survive.
func TestAddFields_RejectsUnsafeKeysAndRenamesReservedOnes(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name      string
		args      []any
		wantKeys  []string
		wantPairs map[string]any
	}{
		{
			name:      "a nil key drops the pair",
			args:      []any{nil, "dropped", "kept", "value"},
			wantKeys:  []string{"kept", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{"kept": "value"},
		},
		{
			name:      "a blank key drops the pair",
			args:      []any{"   ", "dropped", "kept", "value"},
			wantKeys:  []string{"kept", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{"kept": "value"},
		},
		{
			name:      "a newline in a key drops the pair",
			args:      []any{"forged\nlevel", "fatal", "kept", "value"},
			wantKeys:  []string{"kept", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{"kept": "value"},
		},
		{
			name:      "a carriage return in a key drops the pair",
			args:      []any{"forged\rlevel", "fatal", "kept", "value"},
			wantKeys:  []string{"kept", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{"kept": "value"},
		},
		{
			name:      "a tab in a key drops the pair",
			args:      []any{"forged\tlevel", "fatal", "kept", "value"},
			wantKeys:  []string{"kept", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{"kept": "value"},
		},
		{
			name:     "level cannot be overwritten, only shadowed",
			args:     []any{logFieldLevel, "fatal"},
			wantKeys: []string{logFieldLevel, logFieldLevel + "_user", logFieldMessage},
			wantPairs: map[string]any{
				logFieldLevel:           logLevelInfo,
				logFieldLevel + "_user": "fatal",
			},
		},
		{
			name:     "message cannot be overwritten, only shadowed",
			args:     []any{logFieldMessage, "forged"},
			wantKeys: []string{logFieldLevel, logFieldMessage, logFieldMessage + "_user"},
			wantPairs: map[string]any{
				logFieldMessage:           "real message",
				logFieldMessage + "_user": "forged",
			},
		},
		{
			name:      "msg is reserved too",
			args:      []any{logFieldMsg, "forged"},
			wantKeys:  []string{logFieldLevel, logFieldMessage, logFieldMsg + "_user"},
			wantPairs: map[string]any{logFieldMsg + "_user": "forged"},
		},
		{
			name:      "time is reserved too",
			args:      []any{logFieldTime, "forged"},
			wantKeys:  []string{logFieldLevel, logFieldMessage, logFieldTime + "_user"},
			wantPairs: map[string]any{logFieldTime + "_user": "forged"},
		},
		{
			name:      "caller is reserved too",
			args:      []any{logFieldCaller, "forged"},
			wantKeys:  []string{logFieldCaller + "_user", logFieldLevel, logFieldMessage},
			wantPairs: map[string]any{logFieldCaller + "_user": "forged"},
		},
		{
			// The control for every row above: an ordinary key is untouched,
			// so the rejections are attributable to the key being unsafe and
			// not to addFields dropping things in general.
			name:      "an ordinary key is passed through unchanged",
			args:      []any{"ordinary", "value"},
			wantKeys:  []string{logFieldLevel, logFieldMessage, "ordinary"},
			wantPairs: map[string]any{"ordinary": "value"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			New(zerolog.New(&buf)).Info("real message", testCase.args...)

			logged := fieldsOf(t, buf.Bytes())
			check.Equal(t, testCase.wantKeys, keysOf(logged), "exact field set")

			for key, want := range testCase.wantPairs {
				check.Equal(t, want, logged[key])
			}
		})
	}
}

// TestTraceFunc_DoesNotEvaluateItsArgumentWhenSuppressed is the reason
// TraceFunc exists — it takes a closure so expensive arguments are not computed
// when trace is off. Suppressing the OUTPUT is not the same claim as skipping
// the WORK, and only the output half was observable before.
func TestTraceFunc_DoesNotEvaluateItsArgumentWhenSuppressed(t *testing.T) {
	t.Parallel()

	var (
		buf     bytes.Buffer
		invoked bool
	)

	build := func() (string, []any) {
		invoked = true

		return "expensive", nil
	}

	New(zerolog.New(&buf).Level(zerolog.InfoLevel)).TraceFunc(build)
	check.False(t, invoked, "the closure must not run when trace is disabled")
	check.Empty(t, buf.String())

	New(zerolog.New(&buf).Level(zerolog.TraceLevel)).TraceFunc(build)
	check.True(t, invoked, "the control: the closure does run when trace is enabled")
	check.NotEmpty(t, buf.String())
}
