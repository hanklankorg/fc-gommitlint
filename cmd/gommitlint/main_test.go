// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/i18n"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func testLocalizer() output.Localizer {
	return output.EnsureLocalizer(i18n.NewLocalizer("en"))
}

func testSwedishLocalizer(t *testing.T) output.Localizer {
	t.Helper()

	localeDir, err := filepath.Abs("../../locale")
	check.NoError(t, err)

	return output.EnsureLocalizer(i18n.NewLocalizerFromDir("sv", localeDir))
}

func TestDebugOrTraceLogLevel_NormalizesConfiguredLevel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		level    string
		expected bool
	}{
		{name: "debug", level: logadapter.LogLevelDebugName, expected: true},
		{name: "normalized trace", level: " TRACE ", expected: true},
		{name: "info", level: logadapter.LogLevelInfoName, expected: false},
		{name: "empty", level: "", expected: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := isDebugOrTraceLogLevel(testCase.level)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestEffectiveLogLevel_QuietSuppressesNonErrorLogs(t *testing.T) {
	t.Parallel()

	check.Equal(t, logadapter.LogLevelErrorName, effectiveLogLevel(logadapter.LogLevelTraceName, true))
	check.Equal(t, logadapter.LogLevelTraceName, effectiveLogLevel(logadapter.LogLevelTraceName, false))
}

func TestHandleVersionFlag_ReportsWriteFailure(t *testing.T) {
	t.Parallel()

	handled, err := handleVersionFlag(
		[]string{appName, flagVersionLong},
		failingWriter{err: io.ErrClosedPipe},
	)

	check.True(t, handled, "--version must remain recognized when its output fails")
	check.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestRunWithIO_VersionUsesInjectedStreams(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	exitCode := runWithIO(
		[]string{appName, flagVersionLong},
		strings.NewReader("unused"),
		&stdout,
		&stderr,
	)

	check.Equal(t, 0, exitCode)
	check.Contains(t, stdout.String(), "gommitlint version")
	check.Empty(t, stderr.String())
}

func TestRunWithIO_VersionWriteFailureIsSoftwareError(t *testing.T) {
	t.Setenv("LC_ALL", "C")

	var stderr bytes.Buffer

	exitCode := runWithIO(
		[]string{appName, flagVersionLong},
		strings.NewReader("unused"),
		failingWriter{err: io.ErrClosedPipe},
		&stderr,
	)

	check.Equal(t, int(domain.ExitCodeSoftware), exitCode)
	check.Equal(t, unexpectedInternalErrorMessage+"\n", stderr.String())
}

func TestRunWithIO_HelpFlagUnknownTopicMatchesHelpCommand(t *testing.T) {
	t.Setenv("LC_ALL", "C")

	runHelp := func(args []string) (int, string, string) {
		var stdout, stderr bytes.Buffer

		exitCode := runWithIO(args, strings.NewReader(""), &stdout, &stderr)

		return exitCode, stdout.String(), stderr.String()
	}

	wantExitCode, wantStdout, wantStderr := runHelp([]string{appName, commandHelp, "nonsense"})
	gotExitCode, gotStdout, gotStderr := runHelp([]string{appName, flagHelpLong, "nonsense"})

	check.Equal(t, 2, wantExitCode)
	check.Equal(t, wantExitCode, gotExitCode)
	check.Equal(t, wantStdout, gotStdout)
	check.Equal(t, wantStderr, gotStderr)
	check.Contains(t, gotStderr, `Unknown command "nonsense"`)
}

func TestRunWithIO_CompletionStdoutWriteFailureExitsSoftware(t *testing.T) {
	t.Setenv("LC_ALL", "C")

	var stderr bytes.Buffer

	exitCode := runWithIO(
		[]string{appName, completionCommandName, flagStdoutLong, shellBash},
		strings.NewReader(""),
		failingWriter{err: io.ErrClosedPipe},
		&stderr,
	)

	check.Equal(t, 70, exitCode)
	check.Contains(t, stderr.String(), "failed to write completion output")
	check.NotContains(t, stderr.String(), unexpectedInternalErrorMessage)
}

// No t.Parallel(): configureHelpLocalization mutates urfave/cli globals.
func TestValidateHelp_EndsWithSeeAlsoDocsLinks(t *testing.T) {
	configureHelpLocalization(testLocalizer())

	var buf strings.Builder

	app := createCLIApp(testLocalizer())
	app.Writer = &buf

	err := app.Run(t.Context(), []string{appName, commandValidate, "--" + commandHelp})
	check.NoError(t, err)

	helpOutput := buf.String()
	check.Contains(t, helpOutput, "SEE ALSO")
	check.Contains(t, helpOutput, "--rule-help")
	check.Contains(t, helpOutput, "docs/modules/ROOT/pages/reference/commands.adoc#user-content-_gommitlint_validate")
	check.Contains(t, helpOutput, "docs/modules/ROOT/pages/reference/rules.adoc")
}

func TestValidateHelp_LocalizesExamplesSection(t *testing.T) {
	localizer := testSwedishLocalizer(t)
	configureHelpLocalization(localizer)

	var outputBuffer strings.Builder

	app := createCLIApp(localizer)
	app.Writer = &outputBuffer

	err := app.Run(t.Context(), []string{appName, commandValidate, flagHelpLong})

	check.NoError(t, err)
	check.Contains(t, outputBuffer.String(), "\nEXEMPEL:\n")
}

func TestRootHelp_LocalizesExamplesWhenCatalogRetainsSourceMarker(t *testing.T) {
	localizer := testSwedishLocalizer(t)
	configureHelpLocalization(localizer)

	var outputBuffer strings.Builder

	app := createCLIApp(localizer)
	app.Writer = &outputBuffer

	err := app.Run(t.Context(), []string{appName, flagHelpLong})

	check.NoError(t, err)
	check.Contains(t, outputBuffer.String(), "\nEXEMPEL:\n")
	check.NotContains(t, outputBuffer.String(), "\n   Examples:\n")
}

func TestHelpOutputState_RecordsWriteFailure(t *testing.T) {
	state := configureHelpLocalization(testLocalizer())
	app := createCLIApp(testLocalizer())
	app.Writer = failingWriter{err: io.ErrClosedPipe}
	app.ErrWriter = io.Discard

	err := app.Run(t.Context(), []string{appName, flagHelpLong})

	check.NoError(t, err)
	check.ErrorIs(t, state.Err(), io.ErrClosedPipe)
}

// TestCLIApp_ExposesTheDocumentedCommandsAndHandlers drives the real
// createCLIApp -- not the replica the integration harness builds -- and pins
// the surface a user meets: the command set, the version string, the global
// flags, and the handlers wired onto the tree.
func TestCLIApp_ExposesTheDocumentedCommandsAndHandlers(t *testing.T) {
	t.Parallel()

	t.Run("creates valid cli application", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		check.Equal(t, appName, app.Name)
		check.Contains(t, app.Usage, "git commit linter")
		check.NotEmpty(t, app.Description)
		check.True(t, app.EnableShellCompletion)
		check.True(t, app.HideVersion) // We hide default version to remove -v alias
		check.True(t, app.Suggest)
		check.True(t, app.UseShortOptionHandling)

		// The exact set, not a count plus a few names. Len(6) with five
		// Contains left "translations" unpinned: swapping it for another
		// command satisfied both halves.
		commandNames := make([]string, 0, len(app.Commands))
		for _, cmd := range app.Commands {
			commandNames = append(commandNames, cmd.Name)
		}

		slices.Sort(commandNames)

		want := []string{commandConfig, commandDoctor, commandHook, commandMan, "translations", commandValidate}
		slices.Sort(want)

		check.Equal(t, want, commandNames, "the top-level command set")
	})

	t.Run("version information", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		// Test that version string is properly formatted
		check.Contains(t, app.Version, version)
		check.Contains(t, app.Version, formatBuildDate(date))
		check.Contains(t, app.Version, "Build date:")
	})

	t.Run("global flags structure", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		// Test that required global flags are present
		check.NotEmpty(t, app.Flags)

		flagNames := make(map[string]bool)
		for _, flag := range app.Flags {
			flagNames[flag.Names()[0]] = true
		}

		// Test key flags are present - check for expected flags
		expectedFlags := []string{
			flags.Config,
			flags.LogLevel,
			flags.Quiet,
			flags.Color,
		}

		for _, expected := range expectedFlags {
			check.True(t, flagNames[expected], "should have %s flag", expected)
		}
	})

	t.Run("command not found handler reaches subcommands", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		check.NotNil(t, app.CommandNotFound, "the root needs a not-found handler")

		for _, cmd := range app.Commands {
			if len(cmd.Commands) == 0 {
				continue
			}

			check.NotNil(t, cmd.CommandNotFound,
				"%s has subcommands, so it needs its own not-found handler", cmd.Name)
		}
	})

	t.Run("before handler initializes logging", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		// Test that the Before handler is set
		check.NotNil(t, app.Before)

		// Test the Before handler by calling it directly
		ctx := t.Context()

		// Create a command with the exact same flags as the app
		cmd := &cli.Command{
			Flags: app.Flags,
		}

		// Call the Before handler
		newCtx, err := app.Before(ctx, cmd)
		check.NoError(t, err)
		check.NotNil(t, newCtx)

		// Test that context was modified (logging was initialized)
		check.NotEqual(t, ctx, newCtx)
	})

	t.Run("per-command handlers and subcommands are wired", func(t *testing.T) {
		t.Parallel()

		app := createCLIApp(testLocalizer())

		commandMap := make(map[string]*cli.Command)
		for _, cmd := range app.Commands {
			commandMap[cmd.Name] = cmd
		}

		validateCmd, exists := commandMap[commandValidate]
		check.True(t, exists)
		check.NotNil(t, validateCmd.Action)
		check.NotEmpty(t, validateCmd.Flags)

		manCmd, exists := commandMap[commandMan]
		check.True(t, exists)
		check.NotNil(t, manCmd.Action)

		doctorCmd, exists := commandMap[commandDoctor]
		check.True(t, exists)
		check.NotNil(t, doctorCmd.Action)

		configCmd, exists := commandMap[commandConfig]
		check.True(t, exists)
		check.NotEmpty(t, configCmd.Commands) // config has subcommands

		hookCmd, exists := commandMap[commandHook]
		check.True(t, exists)
		check.NotEmpty(t, hookCmd.Commands) // hook has subcommands
	})
}

// TestMainLogic_RunsHelpThroughTheRealApp verifies that help completes even
// under an already-cancelled context because it does not start command work.
func TestMainLogic_RunsHelpThroughTheRealApp(t *testing.T) {
	app := createCLIApp(testLocalizer())
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	check.NoError(t, app.Run(t.Context(), []string{appName, flagHelpLong}))

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	app = createCLIApp(testLocalizer())
	app.Writer = io.Discard
	app.ErrWriter = io.Discard
	check.NoError(t, app.Run(cancelled, []string{appName, flagHelpLong}),
		"help should complete even with a cancelled context")
}

func TestFormatBuildDate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		stamp string
		want  string
	}{
		{
			name:  "release epoch renders as UTC RFC3339",
			stamp: "1780978337",
			want:  "2026-06-09T04:12:17Z",
		},
		{
			name:  "epoch zero renders as the Unix origin",
			stamp: "0",
			want:  "1970-01-01T00:00:00Z",
		},
		{
			name:  "unset default passes through unchanged",
			stamp: "unknown",
			want:  "unknown",
		},
		{
			name:  "already formatted stamp passes through unchanged",
			stamp: "2026-06-09T04:12:17Z",
			want:  "2026-06-09T04:12:17Z",
		},
		{
			name:  "empty stamp passes through unchanged",
			stamp: "",
			want:  "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, formatBuildDate(testCase.stamp), "rendered build stamp")
		})
	}
}

// TestFormatBuildDateIgnoresLocalZone guards the reproducible-build contract: the
// stamp must render in UTC so a given epoch produces the same string on every
// machine. Rebinding time.Local makes the assertion fail if the renderer ever
// switches to local time, which a TZ environment variable cannot do because Go
// caches the local zone on first use. Not parallel: it mutates package state.
func TestFormatBuildDateIgnoresLocalZone(t *testing.T) {
	original := time.Local

	t.Cleanup(func() { time.Local = original })

	time.Local = time.FixedZone("TEST", 7*60*60)

	check.Equal(t, "2026-06-09T04:12:17Z", formatBuildDate("1780978337"), "stamp rendered in UTC")
}
