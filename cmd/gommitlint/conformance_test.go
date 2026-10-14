// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signal"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// These conformance tests give the CLI-conventions rulings teeth in the
// default `go test ./...` gate. They complement the binary-level contract
// tests under test/integration (build tag `integration`). All run
// with an English localizer / LC_ALL=C so the Swedish user locale cannot
// break the English string assertions.

// TestBareInvocation_ExitsUsageErrorWithConciseHelpOnStderr pins the
// bare-invocation ruling: `gommitlint` with no subcommand routes the concise
// usage card to stderr (misuse, not an explicit --help request), keeps stdout
// pure, and exits EX_USAGE (64).
func TestBareInvocation_ExitsUsageErrorWithConciseHelpOnStderr(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	configureHelpLocalization(testLocalizer())

	var stdout, stderr bytes.Buffer

	app := createCLIApp(testLocalizer())
	app.Writer = &stdout
	app.ErrWriter = &stderr

	err := app.Run(t.Context(), []string{appName})

	// The Action returns the bare-invocation sentinel.
	check.ErrorIs(t, err, ErrNoCommand, "bare invocation must return ErrNoCommand")

	// The concise help lands on stderr, not stdout (stdout stays pure so a
	// pipeline reading a bare invocation gets nothing spurious).
	check.Empty(t, stdout.String(), "bare invocation must not write to stdout")
	check.Contains(t, stderr.String(), "gommitlint - git commit linter",
		"concise usage card must appear on stderr")
	check.Contains(t, stderr.String(), "gommitlint --help",
		"concise card must point at full --help")

	// And it exits EX_USAGE (64), not 0.
	code := resolveExitCode(t.Context(), err)
	check.Equal(t, 64, code, "bare invocation must exit EX_USAGE (64)")
	check.Equal(t, int(domain.ExitCodeUsageError), code)
}

// TestResolveExitCode_Contract pins the exit-code contract at the CLI
// boundary. The numbers are a documented contract (see
// docs/.../reference/commands.adoc): a regression here should scream.
func TestResolveExitCode_Contract(t *testing.T) { //nolint:tparallel // t.Setenv pins the locale for the whole table, so the top level stays serial.
	t.Setenv("LC_ALL", "C")

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "success", err: nil, want: 0},
		{name: "bare invocation → EX_USAGE", err: ErrNoCommand, want: 64},
		{
			name: "wrapped bare invocation still EX_USAGE",
			err:  fmt.Errorf("wrapper: %w", ErrNoCommand),
			want: 64,
		},
		{
			name: "unknown command → 2 (POSIX)",
			err:  fmt.Errorf("frobnicate: %w", ErrUnknownCommand),
			want: 2,
		},
		{
			name: "context cancellation without signal → 2",
			err:  context.Canceled,
			want: 2,
		},
		{
			name: "unexpected error → EX_SOFTWARE",
			err:  fmt.Errorf("database implementation failed: %w", io.ErrUnexpectedEOF),
			want: 70,
		},
		{
			name: "output write failure → EX_SOFTWARE",
			err: commands.RecoveryError{
				Msg:   output.Msg("failed to write command output"),
				Cause: commands.ErrOutputWrite,
			},
			want: 70,
		},
		{
			name: "wrapped child status is preserved",
			err:  fmt.Errorf("child failed: %w", cli.Exit("child detail", 42)),
			want: 42,
		},
		{
			name: "invalid child status → EX_SOFTWARE",
			err:  cli.Exit("child was signalled", -1),
			want: 70,
		},
		{
			name: "missing message file → EX_NOINPUT",
			err:  domain.New("messagefile", diagnostics.ReasonMessageFileMissing, "message file missing"),
			want: 66,
		},
		{
			name: "invalid repository → EX_UNAVAILABLE",
			err:  domain.New("repository", diagnostics.ReasonInvalidRepo, "not a repository"),
			want: 69,
		},
		{
			name: "invalid configuration → EX_CONFIG",
			err:  domain.New("config", diagnostics.ReasonInvalidConfig, "invalid config"),
			want: 78,
		},
		{
			name: "invalid commit → EX_DATAERR",
			err:  domain.New("commit", diagnostics.ReasonInvalidCommit, "invalid commit"),
			want: 65,
		},
		{
			name: "validation failure → 1",
			err:  domain.New("validation", diagnostics.ReasonValidationFailed, "rule failed"),
			want: 1,
		},
		{
			name: "recovery-wrapped validation error keeps its reason code",
			err: commands.RecoveryError{
				Msg:   output.Msg("invalid config"),
				Cause: domain.New("config", diagnostics.ReasonInvalidConfig, "invalid config"),
			},
			want: 78,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := resolveExitCode(t.Context(), testCase.err)
			check.Equal(t, testCase.want, got,
				"%s: expected exit %d, got %d", testCase.name, testCase.want, got)
		})
	}
}

func TestCLIUsageErrors_AreTypedAndExitUsage(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	configureHelpLocalization(testLocalizer())

	tests := []struct {
		name                   string
		args                   []string
		wantError              string
		wantSuggestion         string
		wantHelp               bool
		wantNoSuggestion       bool
		wantNoTerminalControls bool
	}{
		{
			name:           "unknown_root_flag",
			args:           []string{appName, "--hlep"},
			wantError:      "Incorrect Usage: flag provided but not defined: -hlep",
			wantSuggestion: `Did you mean "--help"?`,
			wantHelp:       true,
		},
		{
			name:           "unknown_subcommand_flag",
			args:           []string{appName, commandValidate, "--cont"},
			wantError:      "Incorrect Usage: flag provided but not defined: -cont",
			wantSuggestion: `Did you mean "--count"?`,
			wantHelp:       true,
		},
		{
			name:           "unknown_inherited_global_flag",
			args:           []string{appName, commandValidate, "--repo-pth", "."}, // typos:disable-line
			wantError:      "Incorrect Usage: flag provided but not defined: -repo-pth",
			wantSuggestion: `Did you mean "--repo-path"?`,
			wantHelp:       true,
		},
		{
			name:             "unrelated_flag_has_no_suggestion",
			args:             []string{appName, commandValidate, "--qwerty"},
			wantError:        "Incorrect Usage: flag provided but not defined: -qwerty",
			wantHelp:         true,
			wantNoSuggestion: true,
		},
		{
			name:                   "unknown_flag_escapes_terminal_controls",
			args:                   []string{appName, commandValidate, "--bad\x1b[2Jfake"},
			wantError:              `Incorrect Usage: flag provided but not defined: -bad\x1b[2Jfake`,
			wantHelp:               true,
			wantNoTerminalControls: true,
		},
		{
			name:      "invalid_flag_value",
			args:      []string{appName, commandValidate, "--count", "nope"},
			wantError: `Incorrect Usage: invalid value "nope" for flag -count:`,
			wantHelp:  true,
		},
		{
			name:      "root_builtin_help_unknown_flag",
			args:      []string{appName, commandHelp, "--bad-flag"},
			wantError: "Incorrect Usage: flag provided but not defined: -bad-flag",
		},
		{
			name:      "nested_builtin_help_unknown_flag",
			args:      []string{appName, commandConfig, commandHelp, "--bad-flag"},
			wantError: "Incorrect Usage: flag provided but not defined: -bad-flag",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			app := createCLIApp(testLocalizer())
			app.Writer = &stdout
			app.ErrWriter = &stderr

			err := app.Run(t.Context(), testCase.args)

			var usageErr cliUsageError
			check.ErrorAs(t, err, &usageErr)
			check.Equal(t, int(domain.ExitCodeUsage), resolveExitCode(t.Context(), err))
			check.Contains(t, stderr.String(), testCase.wantError)
			check.NotContains(t, stderr.String(), unexpectedInternalErrorMessage)
			check.NotContains(t, stdout.String(), unexpectedInternalErrorMessage)

			if testCase.wantHelp {
				check.Contains(t, stdout.String(), "USAGE")
			} else {
				check.Empty(t, stdout.String(), "hidden built-in help must retain urfave's output behavior")
			}

			if testCase.wantSuggestion != "" {
				check.Contains(t, stderr.String(), testCase.wantSuggestion)
			}

			if testCase.wantNoSuggestion {
				check.NotContains(t, stderr.String(), "Did you mean")
			}

			if testCase.wantNoTerminalControls {
				check.NotContains(t, stderr.String(), "\x1b")
			}

			var topLevelStderr bytes.Buffer

			unexpected, writeErr := writeTopLevelError(&topLevelStderr, testLocalizer(), err)
			check.NoError(t, writeErr)
			check.False(t, unexpected)
			check.Empty(t, topLevelStderr.String(), "usage was already rendered by the parser boundary")
		})
	}
}

func TestCLIUsageError_PropagatesWriteFailure(t *testing.T) {
	t.Parallel()

	parseErr := io.ErrUnexpectedEOF
	cmd := &cli.Command{
		Name:      appName,
		HideHelp:  true,
		ErrWriter: failingWriter{err: io.ErrClosedPipe},
	}

	err := handleCLIUsageError(t.Context(), cmd, parseErr, false)

	var usageErr cliUsageError

	check.ErrorAs(t, err, &usageErr)
	check.ErrorIs(t, err, parseErr)
	check.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestWriteTopLevelError_UsesTypedAndSafePublicMessages(t *testing.T) {
	t.Parallel()

	t.Run("recovery_error_is_actionable", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		err := commands.RecoveryError{
			Msg:         output.Msg("invalid input"),
			Suggestions: []string{"fix the input"},
			Cause:       fmt.Errorf("private cause: %w", io.ErrClosedPipe),
		}

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Contains(t, stderr.String(), "invalid input")
		check.Contains(t, stderr.String(), "fix the input")
		check.NotContains(t, stderr.String(), "private cause")
	})

	t.Run("recovery_error_escapes_terminal_controls", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		err := commands.RecoveryError{
			Msg:         output.Msg("invalid path: %s", "bad\x1b[2Jpath"),
			Suggestions: []string{"retry\x1b[2Jnow"},
		}

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Contains(t, stderr.String(), `invalid path: bad\x1b[2Jpath`)
		check.Contains(t, stderr.String(), "\n\nNext steps:\n  • retry\\x1b[2Jnow\n")
		check.NotContains(t, stderr.String(), "\x1b")
	})

	t.Run("unexpected_error_hides_internal_details", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		unexpected, writeErr := writeTopLevelError(
			&stderr,
			testLocalizer(),
			fmt.Errorf("secret token and stack implementation detail: %w", io.ErrUnexpectedEOF),
		)

		check.NoError(t, writeErr)
		check.True(t, unexpected)
		check.Equal(t, unexpectedInternalErrorMessage+"\n", stderr.String())
		check.NotContains(t, stderr.String(), "secret token")
		check.NotContains(t, stderr.String(), "stack")
	})

	t.Run("ordinary_cancellation_is_visible", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), context.Canceled)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Equal(t, "gommitlint: operation canceled\n", stderr.String())
	})

	t.Run("deadline_is_visible", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), context.DeadlineExceeded)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Equal(t, "gommitlint: operation timed out\n", stderr.String())
	})

	t.Run("pre_report_validation_error_is_rendered", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		err := domain.New("validate", "invalid_commit", "commit input is invalid")

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Contains(t, stderr.String(), "commit input is invalid")
	})

	t.Run("post_report_validation_sentinel_is_suppressed", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		err := fmt.Errorf("%w: %w", commands.ErrValidationReportWritten,
			domain.New("validate", "validation_failed", "duplicate conclusion"))

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Empty(t, stderr.String())
	})

	t.Run("exit_aware_error_preserves_status_but_hides_child_detail", func(t *testing.T) {
		t.Parallel()

		var stderr bytes.Buffer

		err := fmt.Errorf("wrapper detail: %w", cli.Exit("private child stderr", 42))

		unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)

		check.NoError(t, writeErr)
		check.False(t, unexpected)
		check.Equal(t, 42, resolveExitCode(t.Context(), err))
		check.Contains(t, stderr.String(), "exit status 42")
		check.NotContains(t, stderr.String(), "private child stderr")
		check.NotContains(t, stderr.String(), "wrapper detail")
	})

	t.Run("write_failure_is_returned", func(t *testing.T) {
		t.Parallel()

		unexpected, writeErr := writeTopLevelError(
			failingWriter{err: io.ErrClosedPipe},
			testLocalizer(),
			io.ErrUnexpectedEOF,
		)

		check.True(t, unexpected)
		check.ErrorIs(t, writeErr, io.ErrClosedPipe)
	})
}

// TestFailureLogMessage_FailingRunAlwaysDiagnosesOnStderr pins the other half
// of the stdout/stderr split: writeTopLevelError deliberately prints nothing
// once a validation report has been written, so the process boundary must log
// the conclusion instead. Otherwise `gommitlint validate --format github` (or
// json, gitlab, sarif, table) exits 1 with the report on a redirected stdout
// and not one byte on stderr.
func TestFailureLogMessage_FailingRunAlwaysDiagnosesOnStderr(t *testing.T) {
	t.Parallel()

	reportWritten := fmt.Errorf("%w: %w", commands.ErrValidationReportWritten,
		domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonValidationFailed,
			"validation failed: one or more commits have violations"))

	t.Run("report_written_is_diagnosed_at_every_log_level", func(t *testing.T) {
		t.Parallel()

		for _, logLevel := range []string{"", "error", "warn", "info", "debug", "trace"} {
			message, ok := failureLogMessage(reportWritten, testLocalizer(), false, logLevel)

			check.True(t, ok, "log level "+logLevel+" must still diagnose the failure")
			check.Contains(t, message, commandFailureLogPrefix)
			check.Contains(t, message, "Validation failed")
			// The sentinel is an internal marker, not a user-facing cause.
			check.NotContains(t, message, "validation report already written")
		}
	})

	t.Run("unexpected_internal_detail_stays_behind_debug", func(t *testing.T) {
		t.Parallel()

		internal := fmt.Errorf("secret token: %w", io.ErrUnexpectedEOF)

		_, ok := failureLogMessage(internal, testLocalizer(), true, "info")
		check.False(t, ok, "an arbitrary wrapped cause must not surface at info")

		message, ok := failureLogMessage(internal, testLocalizer(), true, "debug")
		check.True(t, ok)
		check.Contains(t, message, "secret token")
	})

	t.Run("expected_errors_are_not_logged_twice", func(t *testing.T) {
		t.Parallel()

		// writeTopLevelError already printed these, so the boundary stays quiet.
		_, ok := failureLogMessage(context.Canceled, testLocalizer(), false, "debug")
		check.False(t, ok)
	})
}

func TestChildSignalRangeStatusesRemainVisible(t *testing.T) {
	t.Parallel()

	for _, status := range []int{128, 130, 255} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			t.Parallel()

			err := cli.Exit("private child detail", status)

			var stderr bytes.Buffer

			check.Equal(t, status, resolveExitCode(t.Context(), err))
			check.False(t, observedSignal(t.Context(), err))

			unexpected, writeErr := writeTopLevelError(&stderr, testLocalizer(), err)
			check.NoError(t, writeErr)
			check.False(t, unexpected)
			check.Equal(t,
				fmt.Sprintf("gommitlint: child process failed with exit status %d\n", status),
				stderr.String(),
			)
			check.NotContains(t, stderr.String(), "private child detail")
		})
	}
}

func TestObservedSignalRequiresSignalCause(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(signal.CancellationError{Signal: os.Interrupt})

	check.True(t, observedSignal(ctx, context.Canceled))
	check.Equal(t, 130, resolveExitCode(ctx, context.Canceled))
	check.True(t, observedSignal(t.Context(), signal.CancellationError{Signal: os.Interrupt}))
	check.False(t, observedSignal(ctx, cli.Exit("child status", 130)))
}

// TestExitCodeConstants_MatchDocumentedContract pins the numeric values of the
// documented exit-code vocabulary so the constants block stays in lockstep
// with docs/.../reference/commands.adoc.
func TestExitCodeConstants_MatchDocumentedContract(t *testing.T) {
	t.Parallel()

	check.Equal(t, 1, int(domain.ExitCodeValidation), "validation failure")
	check.Equal(t, 2, int(domain.ExitCodeUsage), "POSIX misuse")
	check.Equal(t, 64, int(domain.ExitCodeUsageError), "EX_USAGE (bare invocation)")
	check.Equal(t, 65, int(domain.ExitCodeDataErr), "EX_DATAERR")
	check.Equal(t, 66, int(domain.ExitCodeNoInput), "EX_NOINPUT")
	check.Equal(t, 69, int(domain.ExitCodeUnavailable), "EX_UNAVAILABLE")
	check.Equal(t, 70, int(domain.ExitCodeSoftware), "EX_SOFTWARE")
	check.Equal(t, 77, int(domain.ExitCodeNoPerm), "EX_NOPERM")
	check.Equal(t, 78, int(domain.ExitCodeConfiguration), "EX_CONFIG")
}

// escByte is the ANSI escape introducer (0x1b). Its absence in redirected
// output is the whole no-colour-when-piped guarantee.
const escByte = "\x1b"

// TestHelp_NoANSIWhenNotTerminal is the in-process half of the
// no-ANSI-when-piped conformance test: help written to a plain buffer (never
// a terminal) must contain zero 0x1b bytes on both streams. The binary-level
// half (a real redirected validate run) lives in the integration suite.
func TestHelp_NoANSIWhenNotTerminal(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	configureHelpLocalization(testLocalizer())

	cases := []struct {
		name string
		args []string
	}{
		{name: "full --help", args: []string{appName, flagHelpLong}},
		{name: "bare invocation", args: []string{appName}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			app := createCLIApp(testLocalizer())
			app.Writer = &stdout
			app.ErrWriter = &stderr

			_ = app.Run(t.Context(), testCase.args)

			check.NotContains(t, stdout.String(), escByte,
				"%s: stdout must not contain ANSI escape when redirected", testCase.name)
			check.NotContains(t, stderr.String(), escByte,
				"%s: stderr must not contain ANSI escape when redirected", testCase.name)
		})
	}
}

func TestConciseHelp_RespectsColorPolicy(t *testing.T) {
	configureHelpLocalization(testLocalizer())

	testCases := []struct {
		name     string
		color    string
		noColor  string
		term     string
		wantANSI bool
	}{
		{name: "always", color: cliadapter.ColorAlways, term: "xterm", wantANSI: true},
		{name: "never", color: cliadapter.ColorNever, term: "xterm"},
		{name: "no_color", color: cliadapter.ColorAlways, noColor: "1", term: "xterm"},
		{name: "dumb_terminal", color: cliadapter.ColorAlways, term: "dumb"},
		{name: "auto_non_terminal", color: cliadapter.ColorAuto, term: "xterm"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", testCase.noColor)
			t.Setenv("TERM", testCase.term)

			var stdout, stderr bytes.Buffer

			app := createCLIApp(testLocalizer())
			app.Writer = &stdout
			app.ErrWriter = &stderr

			err := app.Run(t.Context(), []string{appName, flagColorLong, testCase.color})

			check.ErrorIs(t, err, ErrNoCommand)
			check.Equal(t, testCase.wantANSI, strings.Contains(stderr.String(), escByte))
		})
	}
}

var (
	// helpFlagRef matches any long flag mentioned in help text.
	helpFlagRef = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	// helpExampleCmd captures the subcommand of an `$ gommitlint <cmd>`
	// example invocation in the concise card.
	helpExampleCmd = regexp.MustCompile(`\$ gommitlint ([a-z][a-z0-9-]+)`)
)

// TestHelpTruth_ConciseCardReferencesOnlyRealCommandsAndFlags is the
// help-truth conformance test: every command and flag the concise usage card
// advertises must actually exist in the live CLI tree, so help can never
// teach a CLI that does not exist. It is a regex-guard against drift.
func TestHelpTruth_ConciseCardReferencesOnlyRealCommandsAndFlags(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	configureHelpLocalization(testLocalizer())

	app := createCLIApp(testLocalizer())
	commandVocab, flagVocab := collectAppVocabulary(app)

	// --version and --help are urfave-provided pseudo-flags; `help` is the
	// git-like help command. They are real but not carried in cmd.Flags.
	flagVocab["version"] = true
	flagVocab["help"] = true
	commandVocab["help"] = true

	var card bytes.Buffer
	showConciseHelp(&card, testLocalizer(), false)

	help := card.String()

	// Every long flag the card names must be a real flag somewhere in the tree.
	for _, match := range helpFlagRef.FindAllString(help, -1) {
		name := strings.TrimPrefix(match, "--")
		check.True(t, flagVocab[name],
			"concise help references unknown flag --%s (drift: help teaches a flag that does not exist)", name)
	}

	// Every `$ gommitlint <cmd>` example must invoke a real command.
	exampleMatches := helpExampleCmd.FindAllStringSubmatch(help, -1)
	check.NotEmpty(t, exampleMatches, "concise help should contain runnable example invocations")

	for _, match := range exampleMatches {
		cmdName := match[1]
		check.True(t, commandVocab[cmdName],
			"concise help example invokes unknown command %q", cmdName)
	}

	// Every entry in the COMMANDS list must be a real command (bare token) or a
	// real pseudo-flag (e.g. --version).
	for _, token := range commandsListTokens(help, testLocalizer().Get("COMMANDS")) {
		if after, ok := strings.CutPrefix(token, "--"); ok {
			check.True(t, flagVocab[after],
				"COMMANDS list references unknown flag %s", token)

			continue
		}

		check.True(t, commandVocab[token],
			"COMMANDS list references unknown command %q", token)
	}
}

// TestConciseHelpFooter_HasUniformDocLines pins the CLI-conventions uniform
// help footer: the concise/bare screen must carry exactly the three doc lines,
// in order — Docs (rendered site), Issues, Manual (man command). Guards
// against footer drift (e.g. a Support: line or a repo-README Docs URL
// creeping back).
func TestConciseHelpFooter_HasUniformDocLines(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	configureHelpLocalization(testLocalizer())

	var card bytes.Buffer
	showConciseHelp(&card, testLocalizer(), false)

	help := card.String()

	check.Contains(t, help, "Docs: https://docs.itiquette.org/gommitlint/",
		"footer must link the rendered docs site")
	check.Contains(t, help, "Issues: https://codefloe.com/itiquette/gommitlint/issues",
		"footer must link the issues tracker")
	check.Contains(t, help, "Manual: man gommitlint",
		"footer must point at the man page")

	// Order: Docs before Issues before Manual.
	docsIdx := strings.Index(help, "Docs: https://docs.itiquette.org/gommitlint/")
	issuesIdx := strings.Index(help, "Issues: https://codefloe.com/itiquette/gommitlint/issues")
	manualIdx := strings.Index(help, "Manual: man gommitlint")
	check.True(t, docsIdx < issuesIdx && issuesIdx < manualIdx,
		"footer lines must appear in Docs, Issues, Manual order")

	// The old non-uniform lines must be gone.
	check.False(t, strings.Contains(help, "Support:"),
		"footer must not carry the old Support: line")
	check.False(t, strings.Contains(help, "gommitlint#readme"),
		"footer must not link the repo README as Docs")
}

// collectAppVocabulary walks the live CLI tree and returns the set of visible
// command names and the set of all flag names (including aliases).
func collectAppVocabulary(app *cli.Command) (map[string]bool, map[string]bool) {
	commandVocab := map[string]bool{}
	flagVocab := map[string]bool{}

	walkCommands(app, app.Name, func(cmd *cli.Command, _ string) {
		for _, sub := range cmd.Commands {
			if !sub.Hidden {
				commandVocab[sub.Name] = true
			}
		}

		for _, flag := range cmd.Flags {
			if vf, ok := flag.(cli.VisibleFlag); ok && !vf.IsVisible() {
				continue
			}

			for _, name := range flag.Names() {
				flagVocab[name] = true
			}
		}
	})

	return commandVocab, flagVocab
}

// commandsListTokens returns the first token of each entry in the concise
// card's COMMANDS section (the block between the localized header and the next
// blank line).
func commandsListTokens(help, header string) []string {
	idx := strings.Index(help, header)
	if idx < 0 {
		return nil
	}

	lines := strings.Split(help[idx:], "\n")

	var tokens []string

	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			break
		}

		fields := strings.Fields(line)
		if len(fields) > 0 {
			tokens = append(tokens, fields[0])
		}
	}

	return tokens
}
