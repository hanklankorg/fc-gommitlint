// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"fmt"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/rs/zerolog"
)

// Logger implements domain.Logger interface using zerolog.
// Uses value semantics - Logger struct contains the zerolog.Logger by value.
type Logger struct {
	logger zerolog.Logger
}

// Compile-time interface compliance check.
var _ domain.Logger = (*Logger)(nil)

// New creates a new domain logger from zerolog.
func New(logger zerolog.Logger) Logger {
	return Logger{logger: logger}
}

// Trace outputs a trace-level message with optional key-value arguments.
// Focuses on execution flow: method entry/exit, execution path, timing, parameter flow.
func (l Logger) Trace(msg string, args ...any) {
	if !l.isTraceEnabled() {
		return
	}

	l.logWithArgs(l.logger.Trace(), msg, args...)
}

// Debug outputs a debug-level message with optional key-value arguments.
// Focuses on application state: variable values, business logic outcomes, data transformations.
func (l Logger) Debug(msg string, args ...any) {
	if !l.isDebugEnabled() {
		return
	}

	l.logWithArgs(l.logger.Debug(), msg, args...)
}

// Info outputs an info-level message with optional key-value arguments.
func (l Logger) Info(msg string, args ...any) {
	l.logWithArgs(l.logger.Info(), msg, args...)
}

// Warn outputs a warning-level message with optional key-value arguments.
func (l Logger) Warn(msg string, args ...any) {
	l.logWithArgs(l.logger.Warn(), msg, args...)
}

// Error outputs an error-level message with optional key-value arguments.
func (l Logger) Error(msg string, args ...any) {
	l.logWithArgs(l.logger.Error(), msg, args...)
}

// TraceFunc provides lazy evaluation for trace logging with function call.
// Use when trace arguments are expensive to compute.
//
//	Example: logger.TraceFunc(func() (string, []any) {
//	    return "expensive calculation", []any{"result", expensiveCalc()}
//	})
func (l Logger) TraceFunc(fn func() (string, []any)) {
	if !l.isTraceEnabled() {
		return
	}

	msg, args := fn()
	l.logWithArgs(l.logger.Trace(), msg, args...)
}

// logWithArgs adds key-value arguments to log event and dispatches message.
func (l Logger) logWithArgs(event *zerolog.Event, msg string, args ...any) {
	if len(args) > 0 {
		event = l.addFields(event, args...)
	}

	event.Msg(msg)
}

// addFields adds key-value pairs to zerolog event with validation.
// Handles odd-length args by padding with empty string.
func (l Logger) addFields(event *zerolog.Event, args ...any) *zerolog.Event {
	if len(args)%2 != 0 {
		// Full slice expression so the pad cannot be written into the
		// caller's array. Plain append reuses spare capacity, so an odd-length
		// args that is a prefix of a longer slice — logger.Info(msg, full[:3]...)
		// — would have the caller's next element silently overwritten.
		args = append(slices.Clip(args), "") // Pad odd args consistently
	}

	for i := range len(args) / 2 {
		idx := i * 2

		key := l.validateAndFormatKey(args[idx])
		if key == "" {
			continue // Skip invalid keys
		}

		event = event.Interface(key, args[idx+1])
	}

	return event
}

// validateAndFormatKey ensures log keys are valid strings and properly formatted.
// Invalid keys are filtered out to maintain log integrity.
func (l Logger) validateAndFormatKey(keyArg any) string {
	if keyArg == nil {
		return "" // Skip nil keys
	}

	key := fmt.Sprint(keyArg)
	key = strings.TrimSpace(key)

	// Reject empty or whitespace-only keys
	if key == "" {
		return ""
	}

	// Reject keys that could break structured logging
	if strings.ContainsAny(key, "\n\r\t") {
		return "" // Skip keys with control characters
	}

	switch key {
	case logFieldLevel, logFieldTime, logFieldMessage, logFieldMsg, logFieldCaller:
		return key + "_user" // Prefix to avoid conflicts
	default:
		return key
	}
}

// isTraceEnabled reports whether trace level logging is enabled.
func (l Logger) isTraceEnabled() bool {
	return l.logger.GetLevel() <= zerolog.TraceLevel
}

// isDebugEnabled reports whether debug level logging is enabled.
func (l Logger) isDebugEnabled() bool {
	return l.logger.GetLevel() <= zerolog.DebugLevel
}
