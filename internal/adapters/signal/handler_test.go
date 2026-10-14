// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signal

import (
	"context"
	"errors"
	"os"
	ossignal "os/signal"
	"syscall"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Test error constants for err113 compliance.
var (
	ErrSomeError = errors.New("some error")
)

// mockLogger captures log calls for testing.
type mockLogger struct {
	infoCalls []logCall
}

type logCall struct {
	message string
	args    []any
}

func (m *mockLogger) Trace(_ string, _ ...any)           {}
func (m *mockLogger) TraceFunc(_ func() (string, []any)) {}
func (m *mockLogger) Debug(_ string, _ ...any)           {}
func (m *mockLogger) Info(msg string, args ...any) {
	m.infoCalls = append(m.infoCalls, logCall{message: msg, args: args})
}
func (m *mockLogger) Warn(_ string, _ ...any)          {}
func (m *mockLogger) Error(_ string, _ ...any)         {}
func (m *mockLogger) Log(_ string, _ string, _ ...any) {}

func TestNewHandler_ConfiguresSignalsAndLogger(t *testing.T) {
	t.Parallel()

	t.Run("with default signals", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger)

		check.Equal(t, logger, handler.logger)
		check.Len(t, handler.signals, 3)
		check.Contains(t, handler.signals, os.Interrupt)
		check.Contains(t, handler.signals, syscall.SIGTERM)
		check.Contains(t, handler.signals, syscall.SIGQUIT)
	})

	t.Run("with custom signals", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		customSignals := []os.Signal{os.Interrupt, syscall.SIGUSR1}
		handler := NewHandler(logger, customSignals...)

		check.Equal(t, logger, handler.logger)
		check.Equal(t, customSignals, handler.signals)
	})
}

func TestHandler_WithContext(t *testing.T) {
	t.Parallel()

	t.Run("creates cancellable context", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger, os.Interrupt)

		// A cancellable parent, released on cleanup, so the handler's
		// signal goroutine does not outlive the test.
		parent, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)

		ctx := handler.WithContext(parent)

		check.NotNil(t, ctx)
		check.NotEqual(t, parent, ctx)

		// Verify context has cancellation capability
		select {
		case <-ctx.Done():
			t.Fatal("Context should not be cancelled initially")
		default:
		}

		cancel()

		select {
		case <-ctx.Done():
			check.ErrorIs(t, ctx.Err(), context.Canceled)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Context should be cancelled with its parent")
		}
	})

	t.Run("context inherits from parent", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger, os.Interrupt)

		// Create parent context with timeout
		parent, parentCancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer parentCancel()

		ctx := handler.WithContext(parent)

		// Wait for parent timeout
		<-parent.Done()

		// Child context should also be done
		select {
		case <-ctx.Done():
			check.Equal(t, context.DeadlineExceeded, ctx.Err())
		case <-time.After(50 * time.Millisecond):
			t.Fatal("Child context should be cancelled when parent times out")
		}
	})

	t.Run("custom signal cancels context", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger, syscall.SIGUSR1) // Use SIGUSR1 to avoid interfering with test runner
		parent := t.Context()

		ctx := handler.WithContext(parent)

		// Send signal to trigger cancellation
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)

		// Wait for context to be cancelled
		select {
		case <-ctx.Done():
			// Context cancelled successfully
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Context should have been cancelled")
		}
	})
}

func TestExitCodeForSignal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		err      error
		expected int
	}{
		{
			name:     "context cancelled",
			err:      context.Canceled,
			expected: 2, // Generic cancellation is a system error
		},
		{
			name:     "SIGINT signal error",
			err:      CancellationError{Signal: os.Interrupt},
			expected: 130,
		},
		{
			name:     "SIGQUIT signal error",
			err:      CancellationError{Signal: syscall.SIGQUIT},
			expected: 131,
		},
		{
			name:     "SIGTERM signal error",
			err:      CancellationError{Signal: syscall.SIGTERM},
			expected: 143,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			expected: 2,
		},
		{
			name:     "generic error",
			err:      ErrSomeError,
			expected: 2,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: 2,
		},
		{
			name:     "validation error",
			err:      domain.New("test", diagnostics.ReasonInvalidFormat, "test validation error"),
			expected: 1,
		},
		{
			name:     "configuration error",
			err:      domain.New("test", diagnostics.ReasonInvalidConfig, "test config error"),
			expected: 78,
		},
		{
			name:     "validation failed error",
			err:      domain.New("test", diagnostics.ReasonValidationFailed, "test validation failed"),
			expected: 1,
		},
		{
			name:     "spell check failed - internal software error",
			err:      domain.New("test", diagnostics.ReasonSpellCheckFailed, "spell check system failed"),
			expected: 70, // EX_SOFTWARE
		},
		{
			name:     "too many commits - malformed input",
			err:      domain.New("test", diagnostics.ReasonTooManyCommits, "too many commits ahead"),
			expected: 65, // EX_DATAERR
		},
		{
			name:     "operation cancelled - system error",
			err:      domain.New("test", diagnostics.ReasonCancelled, "operation was cancelled"),
			expected: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			exitCode := ExitCodeForSignal(testCase.err)
			check.Equal(t, testCase.expected, exitCode)
		})
	}
}

// TestExitCodeForSignal_CoversEveryExplicitlyMappedReason fills in the rest of
// the sysexits contract.
//
// GetExitCodeType maps about twenty reasons explicitly and lets the other
// sixty-odd fall through to ExitCodeValidation — deliberately, per the reason
// on its lint exemption, because most reasons ARE rule failures. So exhaustiveness
// over the whole Reason enum would be the wrong assertion. What matters is the
// EXPLICIT set: those are the reasons whose exit code is a deliberate signal to
// CI, and the table above reached only six of them.
//
// The key-related group is the one worth having most: six reasons all meant to
// report EX_NOPERM, none of them tested, and any one of them drifting to the
// default would silently turn a signing-trust failure into an ordinary rule
// failure — which a pipeline gating on exit 1 versus 77 would read as "the
// commit is bad" rather than "the key material is wrong".
func TestExitCodeForSignal_CoversEveryExplicitlyMappedReason(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		reason   diagnostics.Reason
		expected int
	}{
		// EX_CONFIG
		{diagnostics.ReasonInvalidConfig, 78},
		{diagnostics.ReasonRepoRuleNoRepo, 78},

		// EX_NOINPUT: a named input does not exist
		{diagnostics.ReasonCommitNotFound, 66},
		{diagnostics.ReasonRangeNotFound, 66},
		{diagnostics.ReasonMissingReference, 66},
		{diagnostics.ReasonConfigNotFound, 66},
		{diagnostics.ReasonMessageFileMissing, 66},

		// EX_DATAERR: malformed input
		{diagnostics.ReasonInvalidCommit, 65},
		{diagnostics.ReasonInvalidReference, 65},
		{diagnostics.ReasonInvalidCommitRange, 65},
		{diagnostics.ReasonTooManyCommits, 65},

		// EX_UNAVAILABLE: the git environment is missing or broken
		{diagnostics.ReasonInvalidRepo, 69},
		{diagnostics.ReasonGitOperationFailed, 69},

		// EX_SOFTWARE
		{diagnostics.ReasonSpellCheckFailed, 70},

		// EX_NOPERM: signing-key permission and trust failures
		{diagnostics.ReasonNoKeyDir, 77},
		{diagnostics.ReasonInvalidKeyDir, 77},
		{diagnostics.ReasonMissingKeyDirectory, 77},
		{diagnostics.ReasonInvalidKeyDirectory, 77},
		{diagnostics.ReasonNoTrustedKeys, 77},
		{diagnostics.ReasonKeyLoadFailed, 77},

		// Non-signal cancellation
		{diagnostics.ReasonCancelled, 2},
		{diagnostics.ReasonContextCancelled, 2},
	} {
		t.Run(string(testCase.reason), func(t *testing.T) {
			t.Parallel()

			err := domain.New("test", testCase.reason, "test error")
			check.Equal(t, testCase.expected, ExitCodeForSignal(err))
		})
	}

	// The default arm, asserted as the deliberate behaviour it is rather than
	// left implicit: an unmapped reason is a rule failure, not a system error.
	t.Run("an unmapped reason is a rule failure", func(t *testing.T) {
		t.Parallel()

		err := domain.New("test", diagnostics.ReasonInvalidFormat, "test error")
		check.Equal(t, 1, ExitCodeForSignal(err))
	})
}

func TestExitCodeForOSSignal(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		signal   os.Signal
		expected int
	}{
		{
			name:     "SIGINT",
			signal:   os.Interrupt,
			expected: 130, // 128 + 2
		},
		{
			name:     "SIGQUIT",
			signal:   syscall.SIGQUIT,
			expected: 131, // 128 + 3
		},
		{
			name:     "SIGTERM",
			signal:   syscall.SIGTERM,
			expected: 143, // 128 + 15
		},
		{
			name:     "unknown signal returns -1",
			signal:   syscall.SIGUSR1,
			expected: -1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := ExitCodeForOSSignal(testCase.signal)
			check.Equal(t, testCase.expected, actual)
		})
	}
}

func TestExitCodeForSignalWithContext(t *testing.T) {
	t.Parallel()

	t.Run("signal error in context cause", func(t *testing.T) {
		t.Parallel()

		// Create a context with cancel cause
		ctx, cancel := context.WithCancelCause(t.Context())
		sigErr := CancellationError{Signal: syscall.SIGTERM}
		cancel(sigErr)

		// The error returned is context.Canceled, but the cause is our signal error
		exitCode := ExitCodeForSignalWithContext(ctx, context.Canceled)
		check.Equal(t, 143, exitCode) // SIGTERM = 128 + 15
	})

	t.Run("SIGINT in context cause", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(CancellationError{Signal: os.Interrupt})

		exitCode := ExitCodeForSignalWithContext(ctx, context.Canceled)
		check.Equal(t, 130, exitCode) // SIGINT = 128 + 2
	})

	t.Run("SIGQUIT in context cause", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(CancellationError{Signal: syscall.SIGQUIT})

		exitCode := ExitCodeForSignalWithContext(ctx, context.Canceled)
		check.Equal(t, 131, exitCode) // SIGQUIT = 128 + 3
	})

	t.Run("no cause returns system error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		exitCode := ExitCodeForSignalWithContext(ctx, context.Canceled)
		check.Equal(t, 2, exitCode) // Generic cancellation is system error
	})

	t.Run("validation error is unaffected", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := domain.New("test", diagnostics.ReasonValidationFailed, "test")

		exitCode := ExitCodeForSignalWithContext(ctx, err)
		check.Equal(t, 1, exitCode)
	})

	t.Run("unknown signal with validation error falls through", func(t *testing.T) {
		t.Parallel()

		// Create a context with an unknown signal
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(CancellationError{Signal: syscall.SIGUSR1}) // Unknown signal

		// Pass a validation error
		err := domain.New("test", diagnostics.ReasonValidationFailed, "test")

		// Should return validation error code, not signal code
		exitCode := ExitCodeForSignalWithContext(ctx, err)
		check.Equal(t, 1, exitCode, "unknown signal should be ignored, validation error should take precedence")
	})
}

// TestSignalCause_RealSignalResolvesToPosixExitCode joins the two halves the
// rest of this file only covers separately: that a real OS signal reaches the
// installed handler, and that the cancellation cause the handler stores is
// what ExitCodeForSignalWithContext reads to produce the process exit code.
// This is the path cmd/gommitlint/main.go takes — app.Run returns
// context.Canceled and the signal survives only as the context's cause.
//
// Deliberately not parallel: it delivers real termination signals to this test
// process. signal.Notify suppresses the default die-on-signal disposition only
// while a handler is installed, and Go holds every t.Parallel test until the
// sequential phase completes, so no other test in this binary observes them.
func TestSignalCause_RealSignalResolvesToPosixExitCode(t *testing.T) {
	testCases := []struct {
		name   string
		signal syscall.Signal
		want   int
	}{
		{name: "SIGINT resolves to 130", signal: syscall.SIGINT, want: 130},
		{name: "SIGQUIT resolves to 131", signal: syscall.SIGQUIT, want: 131},
		{name: "SIGTERM resolves to 143", signal: syscall.SIGTERM, want: 143},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			handler := NewHandler(&mockLogger{}, testCase.signal)
			ctx := handler.WithContext(t.Context())

			check.NoError(t, syscall.Kill(syscall.Getpid(), testCase.signal))

			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatalf("context was not cancelled after delivering %s", testCase.signal)
			}

			var cause CancellationError
			check.ErrorAs(t, context.Cause(ctx), &cause, "cancellation cause must carry the signal")
			check.Equal(t, testCase.signal, cause.Signal, "cause must carry the signal that was delivered")

			check.Equal(t, testCase.want, ExitCodeForSignalWithContext(ctx, ctx.Err()))
		})
	}
}

func TestHandler_WithContext_ContextsAreIndependent(t *testing.T) {
	t.Parallel()

	logger := &mockLogger{}
	handler := NewHandler(logger, os.Interrupt)

	parent1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()

	parent2 := t.Context()

	ctx1 := handler.WithContext(parent1)
	ctx2 := handler.WithContext(parent2)

	// Cancelling one context must not cancel the other.
	cancel1()

	select {
	case <-ctx1.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context 1 should be cancelled with its parent")
	}

	select {
	case <-ctx2.Done():
		t.Fatal("Context 2 must not be cancelled by context 1's parent")
	default:
	}
}

func TestHandler_RapidContextCancellation(t *testing.T) {
	t.Parallel()

	t.Run("handles immediate parent cancellation without leak", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger, os.Interrupt)

		// Create parent context that's already cancelled
		parent, cancel := context.WithCancel(t.Context())
		cancel() // Cancel immediately before WithContext

		ctx := handler.WithContext(parent)

		// Child context should also be done
		select {
		case <-ctx.Done():
			// Expected - parent was already cancelled
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Child context should be cancelled when parent is already cancelled")
		}
	})

	t.Run("handles rapid repeated cancellation", func(t *testing.T) {
		t.Parallel()

		logger := &mockLogger{}
		handler := NewHandler(logger, os.Interrupt)

		// Run multiple rapid cancellation cycles to test for resource leaks
		for range 10 {
			parent, cancel := context.WithCancel(t.Context())
			ctx := handler.WithContext(parent)

			// Immediately cancel
			cancel()

			// Ensure child context is cancelled
			select {
			case <-ctx.Done():
				// Expected
			case <-time.After(50 * time.Millisecond):
				t.Fatal("Context should be cancelled")
			}
		}
	})
}

// TestHandler_WithContext_SignalDuringTeardownDoesNotPanic delivers a stream of
// configured signals while handler contexts are created and cancelled, so some
// arrive while a WithContext goroutine tears down. The handler must unregister
// its channel from os/signal before anything could close it; otherwise the
// runtime sends the signal on a closed channel and panics the process instead
// of exiting with the signal's code.
//
// Deliberately not parallel: it delivers SIGUSR2 to this test process. The
// guard subscription keeps SIGUSR2 from taking its default die-on-signal action
// between handler registrations. It is never stopped: that would restore the
// default action while a spammed signal could still be in flight.
func TestHandler_WithContext_SignalDuringTeardownDoesNotPanic(t *testing.T) {
	guard := make(chan os.Signal, 1)
	ossignal.Notify(guard, syscall.SIGUSR2)

	stop := make(chan struct{})
	spammerDone := make(chan struct{})

	go func() {
		defer close(spammerDone)

		for {
			select {
			case <-stop:
				return
			default:
				_ = syscall.Kill(syscall.Getpid(), syscall.SIGUSR2)
			}
		}
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		parent, cancel := context.WithCancel(t.Context())
		ctx := NewHandler(&mockLogger{}, syscall.SIGUSR2).WithContext(parent)

		cancel()
		<-ctx.Done()
	}

	close(stop)
	<-spammerDone
}
