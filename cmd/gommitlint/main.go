// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package main provides the gommitlint command-line tool for validating commit messages.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/i18n"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/security"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signal"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/urfave/cli/v3"
)

// Application errors.
var (
	ErrUnknownCommand = errors.New(unknownCommandMessage)

	// ErrNoCommand marks a bare invocation (no subcommand). The root Action
	// writes the concise usage card to stderr and returns this sentinel so
	// run() maps it to EX_USAGE (64) per the CLI-conventions bare-invocation
	// ruling, without logging a redundant error line.
	ErrNoCommand = errors.New("no command specified")
)

// Command names and CLI tokens. Flag names live in the flags package, color
// modes in the cli adapter, and log-level names in the logging adapter; this
// package imports those owners instead of re-declaring the literals.
const (
	appName               = "gommitlint"
	unknownCommandMessage = "unknown command"

	commandConfig   = "config"
	commandDoctor   = "doctor"
	commandHelp     = "help"
	commandHook     = "hook"
	commandMan      = "man"
	commandTest     = "test"
	commandValidate = "validate"

	flagAllLong     = "--all"
	flagColorLong   = "--" + flags.Color
	flagFormatLong  = "--" + flags.Format
	flagHelpLong    = "--help"
	flagRuleLong    = "--" + flags.Rule
	flagStdoutLong  = "--stdout"
	flagThemeLong   = "--" + flags.Theme
	flagVersionLong = "--version"

	unexpectedInternalErrorMessage = "gommitlint: unexpected internal error; use --log-level=debug for diagnostic details"
	incorrectUsageFormat           = "Incorrect Usage: %s\n\n"
	unknownFlagErrorPrefix         = "flag provided but not defined: -"

	fishValidateCondition   = "__fish_seen_subcommand_from validate"
	fishConfigShowCondition = "__fish_seen_subcommand_from config; and __fish_seen_subcommand_from show"
)

// cliUsageError marks errors raised by urfave's parsing and usage-validation
// boundary. Runtime errors must never be wrapped as this type.
type cliUsageError struct {
	cause error
}

func (e cliUsageError) Error() string {
	return e.cause.Error()
}

func (e cliUsageError) Unwrap() error {
	return e.cause
}

// These variables are set by the build process.
var (
	version = "dev"
	//lint:ignore U1000 set by the linker as release provenance
	commit = "none"    //nolint:gochecknoglobals // Linker-injected release provenance
	date   = "unknown" //nolint:gochecknoglobals // Build metadata injection
)

// formatBuildDate renders the linker-injected build stamp for display.
//
// The build injects a Unix epoch -- .goreleaser.yaml uses {{ .CommitTimestamp }},
// release.yml and scripts/build/build-cross-platform.sh use `git log %ct` -- so the
// stamp carries no timezone or locale and stays byte-identical across rebuilds of
// the same commit. Formatting here rather than at build time preserves that: the
// same epoch always renders the same UTC string, so reproducibility is unaffected.
// Stamps that are not an epoch, such as the "unknown" default of a plain
// `go build`, are passed through unchanged.
func formatBuildDate(stamp string) string {
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return stamp
	}

	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

func isDebugOrTraceLogLevel(logLevel string) bool {
	logLevel = strings.ToLower(strings.TrimSpace(logLevel))

	return logLevel == logadapter.LogLevelDebugName || logLevel == logadapter.LogLevelTraceName
}

func effectiveLogLevel(configured string, quiet bool) string {
	if quiet {
		return logadapter.LogLevelErrorName
	}

	return configured
}

func applyUsageErrorHandler(cmd *cli.Command) {
	cmd.OnUsageError = handleCLIUsageError

	for _, subcommand := range cmd.Commands {
		applyUsageErrorHandler(subcommand)
	}
}

// applyPostSetupUsageErrorHandlers runs from urfave's completion-command
// configuration hook: root defaults (including the built-in help command) are
// present, but recursive command-graph setup has not started. Cloning that
// exact built-in prevents urfave from adding unclassified nested help commands.
func applyPostSetupUsageErrorHandlers(root *cli.Command) {
	helpTemplate := root.Command(commandHelp)
	if helpTemplate == nil {
		applyUsageErrorHandler(root)

		return
	}

	applyUsageErrorHandlerWithHelp(root, helpTemplate, false)
}

func applyUsageErrorHandlerWithHelp(cmd, helpTemplate *cli.Command, helpHidden bool) {
	cmd.OnUsageError = handleCLIUsageError
	helpHidden = helpHidden || cmd.HideHelp

	if !helpHidden && !cmd.HideHelpCommand && cmd.Command(commandHelp) == nil {
		helpCommand := *helpTemplate
		helpCommand.OnUsageError = handleCLIUsageError
		cmd.Commands = append(cmd.Commands, &helpCommand)
	}

	for _, subcommand := range cmd.Commands {
		applyUsageErrorHandlerWithHelp(subcommand, helpTemplate, helpHidden)
	}
}

// handleCLIUsageError preserves urfave's default usage output while attaching
// a typed classification that is safe to distinguish from runtime failures.
func handleCLIUsageError(_ context.Context, cmd *cli.Command, err error, isSubcommand bool) error {
	writer := cmd.Root().ErrWriter

	var renderErr error

	if _, writeErr := fmt.Fprintf(writer, incorrectUsageFormat, output.CleanForTerminal(err.Error())); writeErr != nil {
		renderErr = errors.Join(renderErr, fmt.Errorf("write usage error: %w", writeErr))
	}

	hideHelp := false
	for _, ancestor := range cmd.Lineage() {
		hideHelp = hideHelp || ancestor.HideHelp
	}

	renderErr = errors.Join(renderErr, writeFlagSuggestion(writer, cmd, err, hideHelp))
	renderErr = errors.Join(renderErr, showUsageHelp(cmd, isSubcommand, hideHelp))

	usageErr := cliUsageError{cause: err}
	if renderErr != nil {
		return errors.Join(usageErr, renderErr)
	}

	return usageErr
}

func writeFlagSuggestion(writer io.Writer, cmd *cli.Command, usageErr error, hideHelp bool) error {
	if !cmd.Suggest {
		return nil
	}

	flagName, ok := strings.CutPrefix(usageErr.Error(), unknownFlagErrorPrefix)
	if !ok {
		return nil
	}

	visibleFlags := append(cmd.VisibleFlags(), cmd.VisiblePersistentFlags()...)

	suggestion := cli.SuggestFlag(visibleFlags, flagName, hideHelp)
	if !flagSuggestionIsRelevant(flagName, suggestion) {
		return nil
	}

	if _, err := fmt.Fprintf(writer, cli.SuggestDidYouMeanTemplate+"\n\n", suggestion); err != nil {
		return fmt.Errorf("write flag suggestion: %w", err)
	}

	return nil
}

func showUsageHelp(cmd *cli.Command, isSubcommand bool, hideHelp bool) error {
	if hideHelp {
		return nil
	}

	if isSubcommand {
		if err := cli.ShowSubcommandHelp(cmd); err != nil {
			return fmt.Errorf("show subcommand help: %w", err)
		}

		return nil
	}

	if err := cli.ShowRootCommandHelp(cmd); err != nil {
		return fmt.Errorf("show root help: %w", err)
	}

	return nil
}

// createCLIApp builds the main command with subcommands and global flags.
func createCLIApp(localizer output.Localizer) *cli.Command {
	return createCLIAppWithCommandNotFoundState(localizer, newCommandNotFoundState(localizer))
}

func createCLIAppWithCommandNotFoundState(localizer output.Localizer, commandNotFound *commandNotFoundState) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	startDescription := strings.Join([]string{
		localizer.Get("gommitlint validates Git commit messages against configurable rules to ensure consistency and quality. It supports conventional commit format validation, subject line checks, body requirements, JIRA reference validation, spell checking, and more. The tool can validate individual commits, ranges of commits, or files containing commit messages."),
		localizer.Get("By default, gommitlint validates the HEAD commit. Use --base-branch to validate all commits in a feature branch, or --count to validate multiple recent commits."),
		localizer.Get("Gommitlint includes built-in defaults. Discovery mode merges user configuration from $XDG_CONFIG_HOME/gommitlint/settings.{yaml,yml,toml} and project configuration from .gommitlint.{yaml,yml,toml}. --config selects defaults plus one explicit file and its imports without discovered files; --ignore-config uses defaults only. Dedicated environment variables affect discovery and runtime flags, not a general configuration overlay."),
		localizer.Get(`Examples:
   # Validate the HEAD commit
   gommitlint validate

   # Validate the last 5 commits
   gommitlint validate --count 5

   # Validate every commit on a feature branch
   gommitlint validate --base-branch main

   # Read a commit message from stdin (commit-msg hook)
   cat .git/COMMIT_EDITMSG | gommitlint validate --message-file -

   # Machine-readable output for CI pipelines
   gommitlint validate --format json`),
	}, "\n\n")

	subcommands := []*cli.Command{
		commands.NewValidateCommand(localizer),
		commands.NewDoctorCommand(localizer),
		commands.NewConfigCommand(localizer),
		commands.NewHookCommand(localizer),
		commands.NewManCommand(localizer),
		commands.NewTranslationsCommand(localizer),
	}

	for _, subcommand := range subcommands {
		applyShellCompletion(subcommand)
		applyCommandNotFound(subcommand, commandNotFound.handle)
		applySuggest(subcommand)
	}

	app := &cli.Command{
		Name:        appName,
		Usage:       localizer.Get("git commit linter"),
		UsageText:   localizer.Get("gommitlint [OPTION...] COMMAND [args...]"),
		Description: startDescription,
		// Not localized: --version is a machine-readable probe. The conventions
		// doc calls it out as how scripts verify compatibility, so translating
		// the labels would make `--version | grep 'Build date'` pass or fail
		// depending on the caller's locale.
		Version: fmt.Sprintf("%s (Build date: %s)", version, formatBuildDate(date)),

		// Enable shell completion for all supported shells
		EnableShellCompletion: true,
		ShellComplete:         shellCompleteWithValues,
		ConfigureShellCompletionCommand: func(completion *cli.Command) {
			configureCompletionCommand(completion, localizer)
			applyPostSetupUsageErrorHandlers(completion.Root())
		},

		// Hide the default version flag (which includes -v alias)
		HideVersion: true,

		// Handle command typos with helpful suggestions
		CommandNotFound: commandNotFound.handle,
		After:           commandNotFound.after,

		// Enable built-in command and flag suggestions
		Suggest:                true,
		UseShortOptionHandling: true,

		// Global flags (available to all subcommands)
		Flags: cliadapter.NewGlobalFlags(localizer, ""),

		// Before hook for global setup (logging initialization)
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			logLevel := cmd.String(flags.LogLevel)

			// Validate --log-level here, before InitLogger silently defaults
			// invalid values to "info". Surfaces the typo loudly so users
			// don't wonder why their --log-level=traec produced info-only logs.
			if err := commands.ValidateLogLevel(logLevel, localizer); err != nil {
				return ctx, err
			}

			logLevel = effectiveLogLevel(logLevel, cmd.Bool(flags.Quiet))

			// Coredump suppression is best-effort. Capture the error here and
			// log it after logger initialization so debug-mode users can see
			// why the defense is unavailable on locked-down systems.
			//
			// --allow-coredumps (also bound to GOMMITLINT_ALLOW_COREDUMPS)
			// skips suppression for developers diagnosing gommitlint itself.
			var coredumpErr error

			if !cmd.Bool(flags.AllowCoredumps) {
				coredumpErr = security.DisableCoredumps()
			}

			ctx = logadapter.InitLogger(ctx, output.FormatText, logLevel)

			if coredumpErr != nil {
				logadapter.GetDomainLogger(ctx).Debug(
					"coredump suppression unavailable; crashes may dump memory to disk",
					"error", coredumpErr,
				)
			}

			return ctx, nil
		},

		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() > 0 {
				return handleCommandNotFound(ctx, cmd, cmd.Args().Get(0), localizer)
			}
			// Bare invocation: a subcommand is required. Per the CLI-conventions
			// two-tier error-help rule, the concise usage card goes to stderr
			// (this is misuse, not an explicit --help request) and run() exits
			// EX_USAGE (64) via the ErrNoCommand sentinel.
			writer := cmd.Root().ErrWriter
			useColor := cliadapter.NewOutputOptions(writer).
				WithColor(cmd.String(flags.Color)).
				ShouldUseColor()
			showConciseHelp(writer, localizer, useColor)

			return ErrNoCommand
		},

		Commands: subcommands,
	}

	applyUsageErrorHandler(app)

	return app
}

// handleVersionFlag checks for --version as the only argument and prints version info.
// It reports whether the argument was handled separately from any output error.
// This ensures --version only works at top level, not in subcommands.
func handleVersionFlag(args []string, writer io.Writer) (bool, error) {
	if len(args) == 2 && args[1] == flagVersionLong {
		// Deliberately unlocalized -- see the Version field in createCLIApp.
		_, err := fmt.Fprintf(writer, "gommitlint version %s (Build date: %s)\n", version, formatBuildDate(date))

		return true, err
	}

	return false, nil
}

func main() {
	os.Exit(runWithPanicBoundary(run, os.Stderr))
}

// run supplies process-global arguments and streams to the testable lifecycle.
func run() int {
	return runWithIO(os.Args, os.Stdin, os.Stdout, os.Stderr)
}

// resolveExitCode maps a top-level run error to a process exit code. It is the
// single source of truth for the exit-code contract at the CLI boundary: a
// bare invocation (no subcommand) is EX_USAGE (64) per the bare-invocation
// ruling; signals and validation/environment errors defer to the signal-aware
// classifier. Kept separate from run() so the contract is unit-testable.
func resolveExitCode(ctx context.Context, err error) int {
	if err == nil {
		return 0
	}

	if errors.Is(err, ErrNoCommand) {
		return int(domain.ExitCodeUsageError)
	}

	if _, ok := errors.AsType[cliUsageError](err); ok {
		return int(domain.ExitCodeUsage)
	}

	var cancellationErr signal.CancellationError
	if errors.As(err, &cancellationErr) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return signal.ExitCodeForSignalWithContext(ctx, err)
	}

	if validationErr, ok := errors.AsType[domain.ValidationError](err); ok {
		return int(domain.GetExitCodeType(validationErr.Diagnostic.Reason))
	}

	if errors.Is(err, commands.ErrOutputWrite) {
		return int(domain.ExitCodeSoftware)
	}

	if exitCode, ok := errorExitCode(err); ok {
		return exitCode
	}

	var recovery commands.RecoveryError
	if errors.As(err, &recovery) || errors.Is(err, ErrUnknownCommand) {
		return int(domain.ExitCodeUsage)
	}

	return int(domain.ExitCodeSoftware)
}

// errorExitCode preserves a valid status supplied by a child process or other
// exit-aware error. Unix process statuses are one byte; invalid and successful
// statuses attached to an error fall back to EX_SOFTWARE.
func errorExitCode(err error) (int, bool) {
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) {
		return 0, false
	}

	exitCode := exitErr.ExitCode()

	return exitCode, exitCode > 0 && exitCode <= 255
}

// observedSignal reports whether this invocation was cancelled by a known
// signal. Child processes may legitimately return any one-byte status,
// including 128-255, so an exit code alone is never evidence that gommitlint
// itself received a signal.
func observedSignal(ctx context.Context, err error) bool {
	var cancellationErr signal.CancellationError
	if errors.As(err, &cancellationErr) {
		return signal.ExitCodeForOSSignal(cancellationErr.Signal) >= 128
	}

	if !errors.Is(err, context.Canceled) {
		return false
	}

	return errors.As(context.Cause(ctx), &cancellationErr) &&
		signal.ExitCodeForOSSignal(cancellationErr.Signal) >= 128
}

// writeTopLevelError renders expected errors as their public typed form and
// substitutes a stable message for unexpected errors. It reports whether the
// error was unexpected so callers can restrict internal details to debug logs.
func writeTopLevelError(writer io.Writer, localizer output.Localizer, err error) (bool, error) {
	if errors.Is(err, commands.ErrValidationReportWritten) {
		return false, nil
	}

	localizer = output.EnsureLocalizer(localizer)

	if recovery, ok := errors.AsType[commands.RecoveryError](err); ok {
		_, writeErr := fmt.Fprintln(writer, recovery.Format(localizer))

		return false, writeErr
	}

	var (
		validationErr   domain.ValidationError
		cancellationErr signal.CancellationError
		usageErr        cliUsageError
	)

	if errors.As(err, &validationErr) {
		_, writeErr := fmt.Fprintln(writer, output.PublicValidationMessage(validationErr, localizer))

		return false, writeErr
	}

	if errors.Is(err, context.DeadlineExceeded) {
		_, writeErr := fmt.Fprintln(writer, localizer.Get("gommitlint: operation timed out"))

		return false, writeErr
	}

	if errors.As(err, &cancellationErr) || errors.Is(err, context.Canceled) {
		_, writeErr := fmt.Fprintln(writer, localizer.Get("gommitlint: operation canceled"))

		return false, writeErr
	}

	if errors.As(err, &usageErr) ||
		errors.Is(err, ErrUnknownCommand) {
		return false, nil
	}

	if exitCode, ok := errorExitCode(err); ok {
		_, writeErr := fmt.Fprintln(writer, localizer.Get("gommitlint: child process failed with exit status %d", exitCode))

		return false, writeErr
	}

	_, writeErr := fmt.Fprintln(writer, localizer.Get(unexpectedInternalErrorMessage))

	return true, writeErr
}

// commandFailureLogPrefix labels the process-boundary failure log. Log messages
// are English throughout the codebase; only the conclusion appended to it is
// localised.
const commandFailureLogPrefix = "Command execution failed: "

// failureLogMessage decides whether a failing run still owes stderr a
// diagnostic, and what that diagnostic may safely say. Two cases qualify:
//
//   - A validation report has already been written. writeTopLevelError stays
//     silent there so the conclusion is not printed twice, but the report goes
//     to stdout, which callers routinely redirect to a file for the machine
//     formats (json, github, gitlab, sarif, table). Without this line stderr
//     carries nothing at all and the run fails silently, whatever the format.
//     Only the public conclusion is logged, never the sentinel chain.
//   - An unexpected internal error, where stderr received just the stable
//     internal-error message. Its wrapped cause is arbitrary text, so it stays
//     behind debug or trace.
func failureLogMessage(err error, localizer output.Localizer, unexpected bool, logLevel string) (string, bool) {
	if errors.Is(err, commands.ErrValidationReportWritten) {
		return commandFailureLogPrefix + reportedFailureConclusion(err, localizer), true
	}

	if unexpected && isDebugOrTraceLogLevel(logLevel) {
		return commandFailureLogPrefix + err.Error(), true
	}

	return "", false
}

// reportedFailureConclusion returns the public one-line conclusion that the
// validation path joins onto ErrValidationReportWritten. The fallback keeps the
// boundary safe should that pairing ever change.
func reportedFailureConclusion(err error, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	if validationErr, ok := errors.AsType[domain.ValidationError](err); ok {
		return output.PublicValidationMessage(validationErr, localizer)
	}

	return localizer.Get("validation failed")
}

// runWithIO owns the CLI lifecycle and returns the process exit code. Keeping
// os.Exit in main lets recoverPanic stay deferred without an early process exit
// bypassing the panic boundary (gocritic exitAfterDefer).
func runWithIO(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	// Handle --version at top level only (before subcommand parsing)
	if handled, err := handleVersionFlag(args, stdout); handled {
		if err == nil {
			return 0
		}

		err = fmt.Errorf("write version: %w", err)
		_, _ = writeTopLevelError(stderr, i18n.NewLocalizerFromEnv(), err)

		return resolveExitCode(context.Background(), err)
	}

	localizer := i18n.NewLocalizerFromEnv()
	helpOutput := configureHelpLocalization(localizer)

	ctx := context.Background()
	invocationCtx := ctx
	commandNotFound := newCommandNotFoundState(localizer)
	app := createCLIAppWithCommandNotFoundState(localizer, commandNotFound)
	app.Reader = stdin
	app.Writer = stdout
	app.ErrWriter = stderr
	originalBefore := app.Before
	app.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		configuredCtx, err := originalBefore(ctx, cmd)
		if err != nil {
			return configuredCtx, err
		}

		configuredCtx = signal.NewHandler(logadapter.GetDomainLogger(configuredCtx)).WithContext(configuredCtx)
		// The CLI invokes Before once; retain its logger and signal cause for final error handling.
		//nolint:fatcontext
		invocationCtx = configuredCtx

		return configuredCtx, nil
	}

	err := app.Run(ctx, args)

	err = errors.Join(err, commandNotFound.take())
	if helpOutput.Err() != nil {
		err = errors.Join(err, fmt.Errorf("render help: %w", helpOutput.Err()))
	}

	if err == nil {
		return 0
	}

	exitCode := resolveExitCode(invocationCtx, err)

	// Bare invocation already wrote the concise usage card to stderr; the help
	// is the message, so skip error logging and return EX_USAGE directly.
	if errors.Is(err, ErrNoCommand) {
		return exitCode
	}

	if observedSignal(invocationCtx, err) {
		return exitCode
	}

	unexpected, writeErr := writeTopLevelError(app.ErrWriter, localizer, err)
	if writeErr != nil {
		err = errors.Join(err, fmt.Errorf("write top-level diagnostic: %w", writeErr))
	}

	logLevel := app.String(flags.LogLevel)
	if message, ok := failureLogMessage(err, localizer, unexpected, logLevel); ok {
		logCtx := logadapter.InitLogger(invocationCtx, output.FormatText, logLevel)
		logadapter.GetDomainLogger(logCtx).Error(message)
	}

	return exitCode
}
