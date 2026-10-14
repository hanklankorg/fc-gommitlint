// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// CancellationError represents a cancellation due to a signal.
type CancellationError struct {
	Signal os.Signal
}

// Error implements the error interface.
func (e CancellationError) Error() string {
	return fmt.Sprintf("received signal: %v", e.Signal)
}

// Handler routes OS termination signals to context cancellation.
type Handler struct {
	signals []os.Signal
	logger  domain.Logger
}

// NewHandler creates a new signal handler with the specified signals and logger.
func NewHandler(logger domain.Logger, signals ...os.Signal) Handler {
	if len(signals) == 0 {
		// Only handle the three standard termination signals we know how to map
		signals = []os.Signal{
			os.Interrupt,    // SIGINT (Ctrl+C) -> 130
			syscall.SIGTERM, // SIGTERM -> 143
			syscall.SIGQUIT, // SIGQUIT -> 131
		}
	}

	return Handler{
		signals: signals,
		logger:  logger,
	}
}

// WithContext returns a context that will be cancelled when any of the configured signals is received.
func (h Handler) WithContext(parent context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(parent)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, h.signals...)

	go func() {
		// Only unregister: os/signal may still be sending to sigChan until Stop
		// returns, so closing it would panic on a signal arriving during teardown.
		defer signal.Stop(sigChan)

		select {
		case sig := <-sigChan:
			h.logger.Info("Received signal, initiating graceful shutdown", "signal", sig.String())
			cancel(CancellationError{Signal: sig})
		case <-ctx.Done():
			// Parent context cancelled, clean exit without leak
			return
		}
	}()

	return ctx
}

// ExitCodeForSignal returns the exit code following Unix conventions.
// For signals: 128 + signal_number (SIGINT=2, SIGQUIT=3, SIGTERM=15).
// Unknown signals are ignored and not treated as errors.
func ExitCodeForSignal(err error) int {
	// Check if this is a signal error directly
	if sigErr, ok := errors.AsType[CancellationError](err); ok {
		exitCode := ExitCodeForOSSignal(sigErr.Signal)
		// Only return exit code for known signals, ignore unknown ones
		if exitCode != unknownSignalExit {
			return exitCode
		}
		// Unknown signal - don't treat as error, continue checking other error types
	}

	// Check if this is a context cancellation (non-signal)
	if errors.Is(err, context.Canceled) {
		// Generic cancellation without specific signal is treated as cancelled operation
		return int(domain.ExitCodeUsage)
	}

	if validationErr, ok := errors.AsType[domain.ValidationError](err); ok {
		exitType := domain.GetExitCodeType(validationErr.Diagnostic.Reason)

		return int(exitType)
	}

	// Unknown error type — fall back to generic CLI misuse.
	return int(domain.ExitCodeUsage)
}

// handleContextCancellation extracts exit code from context cancellation cause.
func handleContextCancellation(ctx context.Context) int {
	cause := context.Cause(ctx)
	if cause == nil {
		return int(domain.ExitCodeUsage) // No specific cause - treat as cancelled operation
	}

	var causeSignalErr CancellationError
	if !errors.As(cause, &causeSignalErr) {
		return int(domain.ExitCodeUsage) // Cause is not a signal
	}

	exitCode := ExitCodeForOSSignal(causeSignalErr.Signal)
	if exitCode != -1 {
		return exitCode // Known signal
	}

	// Unknown signal - ignore it, treat as normal cancellation
	return int(domain.ExitCodeUsage)
}

// ExitCodeForSignalWithContext returns the appropriate exit code, checking context for signal cause.
func ExitCodeForSignalWithContext(ctx context.Context, err error) int {
	// Check if this is a signal error directly
	if sigErr, ok := errors.AsType[CancellationError](err); ok {
		exitCode := ExitCodeForOSSignal(sigErr.Signal)
		if exitCode != unknownSignalExit {
			return exitCode // Known signal
		}
		// Unknown signal - don't return here, check other error types
	}

	// Check if this is a context cancellation
	if errors.Is(err, context.Canceled) {
		return handleContextCancellation(ctx)
	}

	// Fall back to regular exit code logic
	return ExitCodeForSignal(err)
}

// signalExitBase is the POSIX convention offset added to a signal's number to
// derive its process exit code.
const signalExitBase = 128

// unknownSignalExit is returned by ExitCodeForOSSignal when the caller hands
// in a signal the handler was not configured to recognise; the caller then
// decides whether to fall back to a generic cancellation code.
const unknownSignalExit = -1

// ExitCodeForOSSignal returns the Unix exit code for a given signal per POSIX
// convention (signalExitBase + signal_number). Returns unknownSignalExit for
// signals the handler wasn't configured to recognise.
func ExitCodeForOSSignal(sig os.Signal) int {
	switch sig {
	case os.Interrupt: // SIGINT == 2
		return signalExitBase + int(syscall.SIGINT)
	case syscall.SIGQUIT: // 3
		return signalExitBase + int(syscall.SIGQUIT)
	case syscall.SIGTERM: // 15
		return signalExitBase + int(syscall.SIGTERM)
	default:
		return unknownSignalExit
	}
}
