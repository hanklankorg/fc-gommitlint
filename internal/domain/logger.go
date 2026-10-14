// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

// Logger defines logging interface for domain layer.
// Separates trace (flow) from debug (state) logging for better debugging effectiveness.
type Logger interface {
	// Trace outputs trace-level messages for execution flow.
	Trace(msg string, args ...any)

	// TraceFunc for lazy evaluation in performance-critical trace logging.
	// Function only called if trace level is enabled.
	TraceFunc(fn func() (string, []any))

	// Debug outputs debug-level messages for application state.
	Debug(msg string, args ...any)

	// Info outputs an info-level message with optional arguments.
	Info(msg string, args ...any)

	// Warn outputs a warning-level message with optional arguments.
	Warn(msg string, args ...any)

	// Error outputs an error-level message with optional arguments.
	Error(msg string, args ...any)
}
