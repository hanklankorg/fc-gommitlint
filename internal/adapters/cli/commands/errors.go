// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// ErrOutputWrite is returned when writing to command output fails.
var ErrOutputWrite = errors.New("failed to write command output")

// Common command errors.
var (
	// ErrUnexpectedArgument is returned when a command receives unexpected positional arguments.
	ErrUnexpectedArgument     = errors.New("unexpected argument")
	ErrNonPositiveTimeout     = errors.New("timeout must be a positive duration")
	ErrHookAlreadyExists      = errors.New("gommitlint hook already exists")
	ErrGlobalHookExists       = errors.New("gommitlint global hook already exists")
	ErrHooksPathInUse         = errors.New("git config core.hooksPath already names another directory")
	ErrHooksPathRedirected    = errors.New("repository hooks run from a core.hooksPath directory")
	ErrCustomHookExists       = errors.New("custom hook exists")
	ErrUnsupportedHookType    = errors.New("unsupported hook type")
	ErrNoHomeDirectory        = errors.New("cannot determine home directory")
	ErrManualSourceNotFound   = errors.New("manual source not found")
	errManualSourceNotRegular = errors.New("manual source is not a regular file")
	// ErrValidationReportWritten marks the validation failure returned after a
	// complete report has already been emitted. Only this sentinel is suppressed
	// at the process boundary; pre-report ValidationError values remain visible.
	ErrValidationReportWritten = errors.New("validation report already written")
)

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func notGommitlintHookError(hookType, hookPath string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	suggestions := customHookRecoverySuggestions(hookPath, "gommitlint hook install", loc)

	return newRecoveryErrorWithCause(
		ErrNotGommitlintHook,
		output.Msg("existing %s hook at %s was installed by another tool", hookType, hookPath),
		suggestions...,
	)
}

func hookInstallError(hookType string, cause error, retryCommand string, localizer output.Localizer) error {
	if _, ok := errors.AsType[RecoveryError](cause); ok {
		return cause
	}

	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		commandOperationFailure(cause),
		output.Msg("%s hook installation failed: %s", hookType, cause),
		loc.Get("Check hook status: gommitlint hook status"),
		retryCommand,
	)
}

// RecoveryError represents an error with recovery suggestions.
type RecoveryError struct {
	Msg         output.LocaleMsg
	Suggestions []string
	Cause       error
}

type commandOperationError struct {
	cause error
}

func (e commandOperationError) Error() string {
	return e.cause.Error()
}

func (e commandOperationError) Unwrap() error {
	return e.cause
}

func (commandOperationError) ExitCode() int {
	return int(domain.ExitCodeSoftware)
}

func commandOperationFailure(cause error) error {
	var exitErr interface{ ExitCode() int }
	if errors.As(cause, &exitErr) {
		return cause
	}

	return commandOperationError{cause: cause}
}

// Error formats the recovery error with actionable suggestions for CLI users.
func (e RecoveryError) Error() string {
	msg := e.Msg.String()

	if len(e.Suggestions) == 0 {
		return msg
	}

	var builder strings.Builder

	builder.WriteString(msg)
	builder.WriteString("\n\nNext steps:")

	for _, suggestion := range e.Suggestions {
		builder.WriteString("\n  • ")
		builder.WriteString(suggestion)
	}

	return builder.String()
}

// Format renders the recovery error with localized structural strings for CLI output.
// Use this at the output boundary instead of Error() to get translated "Next steps:" header.
func (e RecoveryError) Format(localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	msg := output.CleanForTerminal(e.Msg.Localize(localizer))

	if len(e.Suggestions) == 0 {
		return msg
	}

	var builder strings.Builder

	builder.WriteString(msg)
	builder.WriteString("\n\n")
	builder.WriteString(output.CleanForTerminal(localizer.Get("Next steps:")))

	for _, suggestion := range e.Suggestions {
		builder.WriteString("\n  • ")
		builder.WriteString(output.CleanForTerminal(suggestion))
	}

	return builder.String()
}

func (e RecoveryError) Unwrap() error {
	return e.Cause
}

func newRecoveryErrorWithCause(cause error, msg output.LocaleMsg, suggestions ...string) error {
	return RecoveryError{
		Msg:         msg,
		Suggestions: suggestions,
		Cause:       cause,
	}
}

// hookAlreadyExistsError creates an error for when a gommitlint hook already exists.
func hookAlreadyExistsError(hookPath string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrHookAlreadyExists,
		output.Msg("gommitlint hook already exists at %s", hookPath),
		loc.Get("View current hook: cat %s", shellQuote(hookPath)),
		loc.Get("Check hook status: gommitlint hook status"),
		loc.Get("Force overwrite: gommitlint hook install --force"),
	)
}

// customHookExistsError creates an error for when a custom (non-gommitlint) hook exists.
func customHookExistsError(hookPath, installCommand string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrCustomHookExists,
		output.Msg("custom hook exists at %s - will not overwrite", hookPath),
		customHookRecoverySuggestions(hookPath, installCommand, loc)...,
	)
}

func customHookRecoverySuggestions(hookPath, installCommand string, localizer output.Localizer) []string {
	quotedPath := shellQuote(hookPath)

	return []string{
		localizer.Get("View current hook: cat %s", quotedPath),
		localizer.Get("Backup custom hook: cp %s %s.backup", quotedPath, quotedPath),
		localizer.Get("Remove custom hook manually: rm %s", quotedPath),
		localizer.Get("Then install gommitlint: %s", installCommand),
		localizer.Get("Note: --force will NOT work for custom hooks (safety feature)"),
	}
}

// globalHookAlreadyExistsError creates an error for when a gommitlint global hook exists.
func globalHookAlreadyExistsError(hookPath string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrGlobalHookExists,
		output.Msg("gommitlint global hook already exists at %s", hookPath),
		loc.Get("View current hook: cat %s", shellQuote(hookPath)),
		loc.Get("Check hook status: gommitlint hook status"),
		loc.Get("Force overwrite: gommitlint hook install --global --force"),
	)
}

// hooksPathInUseError explains that global core.hooksPath already names a
// directory gommitlint does not own, and how to proceed without losing it.
func hooksPathInUseError(currentPath string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrHooksPathInUse,
		output.Msg("git config core.hooksPath already points to %s - will not take it over", currentPath),
		loc.Get("Inspect that directory: ls -la -- %s", shellQuote(currentPath)),
		loc.Get("Keep the current hooks directory and add gommitlint through its hook manager"),
		loc.Get("Replace it (previous value is recorded and restored by 'hook remove --global'): gommitlint hook install --global --force"),
		loc.Get("Or install per repository instead: gommitlint hook install"),
	)
}

// hooksPathRedirectedError explains that a local install would be inert: git
// runs this repository's hooks from redirect (core.hooksPath), not .git/hooks.
func hooksPathRedirectedError(redirect string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	if filepath.Clean(redirect) == filepath.Clean(GlobalHooksPath()) {
		return newRecoveryErrorWithCause(
			ErrHooksPathRedirected,
			output.Msg("this repository's hooks run from gommitlint's global hooks directory %s", redirect),
			loc.Get("Install globally instead: gommitlint hook install --global"),
			loc.Get("Or run per-repository hooks here: git config core.hooksPath .git/hooks"),
		)
	}

	return newRecoveryErrorWithCause(
		ErrHooksPathRedirected,
		output.Msg("this repository's hooks run from %s (core.hooksPath), so a hook in .git/hooks would never run", redirect),
		loc.Get("Add 'gommitlint validate --message-file \"$1\"' to that hook manager's commit-msg hook"),
		loc.Get("Or run per-repository hooks here: git config core.hooksPath .git/hooks"),
	)
}

// invalidRepositoryError creates an error for invalid repository paths.
func invalidRepositoryError(err error, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	// Render just the cause, not the wrapper. The sentinel ErrInvalidRepoPath
	// already contributes the "invalid repository path:" prefix we want — if we
	// also embedded err.Error() (which includes that prefix when wrapped via
	// %w) the user would see it twice.
	detail := strings.TrimPrefix(err.Error(), cliAdapter.ErrInvalidRepoPath.Error()+": ")
	message := "invalid repository path: " + detail
	cause := errors.Join(domain.New("cli", diagnostics.ReasonInvalidRepo, message), err)

	return newRecoveryErrorWithCause(
		cause,
		output.Msg("invalid repository path: %s", detail),
		loc.Get("Change to a git repository: cd /path/to/repo"),
		loc.Get("Initialize git repository: git init"),
		loc.Get("Specify repository path: gommitlint --repo-path /path/to/repo"),
		loc.Get("Check current directory: pwd"),
	)
}

// configLoadError creates an error for configuration loading failures.
//
// Repo-path errors arrive on the same channel because LoadConfigFromCommand
// performs both config and repo validation. Detect those up front and route to
// repository-shaped recovery hints — emitting "Check config syntax" for an
// invalid --repo-path is the worst-of-both-worlds suggestion.
func configLoadError(err error, localizer output.Localizer) error {
	if errors.Is(err, cliAdapter.ErrInvalidRepoPath) {
		return invalidRepositoryError(err, localizer)
	}

	loc := output.EnsureLocalizer(localizer)
	message := fmt.Sprintf("failed to load configuration: %s", err)

	var cause error = domain.New("cli", diagnostics.ReasonInvalidConfig, message)

	if errors.Is(err, ErrConfigFileNotFound) ||
		errors.Is(err, configuration.ErrConfigUnreadable) {
		cause = domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonConfigNotFound, message)
	}

	if errors.Is(err, configuration.ErrUnknownPreset) {
		return newRecoveryErrorWithCause(
			errors.Join(cause, err),
			output.Msg("failed to load configuration: %s", err),
			loc.Get("Use one of: preset:minimal, preset:conventional, preset:ambitious, preset:secure"),
			loc.Get("Inspect the effective config: gommitlint config show --format=yaml"),
		)
	}

	if _, ok := errors.AsType[domain.ValidationError](err); ok {
		cause = err
	} else {
		cause = errors.Join(cause, err)
	}

	suggestions := []string{
		loc.Get("Check config syntax: gommitlint config show"),
		loc.Get("Create default config: gommitlint config init"),
		loc.Get("Use default config: gommitlint validate --ignore-config"),
		loc.Get("Specify config file: gommitlint --config /path/to/settings.yaml"),
		loc.Get("View config docs: https://codefloe.com/itiquette/gommitlint#configuration"),
	}

	var pathErr configPathValidationError
	if errors.As(err, &pathErr) && errors.Is(err, configuration.ErrConfigPathWorldWritable) {
		suggestions = []string{
			loc.Get("Fix config file permissions: chmod o-w -- %s", shellQuote(pathErr.Path)),
			loc.Get("Ensure the config file is owned by your user"),
			loc.Get("Use default config: gommitlint validate --ignore-config"),
			loc.Get("Specify config file: gommitlint --config /path/to/settings.yaml"),
			loc.Get("View config docs: https://codefloe.com/itiquette/gommitlint#configuration"),
		}
	}

	return newRecoveryErrorWithCause(cause, output.Msg("failed to load configuration: %s", err), suggestions...)
}

// validationTargetError creates an error for validation target issues.
//
// The --count cap (limits.max_commit_count) hit is a frequent case; surface
// the config knob inline so users don't have to grep docs to learn how to
// validate more commits than the default permits.
func validationTargetError(err error, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	// A named file that is not there has one obvious next step, and it is not
	// "here are five ways to invoke validate". Answer the failure the caller
	// actually hit, and skip the "failed to create validation target" prefix:
	// the caller passed a path, not a validation target.
	var missingFile domain.ValidationError
	if errors.As(err, &missingFile) && missingFile.Diagnostic.Reason == diagnostics.ReasonMessageFileMissing {
		return newRecoveryErrorWithCause(
			err,
			output.Msg("%s", missingFile.Message),
			loc.Get("Check the path and re-run"),
			loc.Get("Read the commit message from stdin instead: echo \"feat: message\" | gommitlint validate -f -"),
			loc.Get("Validate the HEAD commit instead: gommitlint validate"),
		)
	}

	suggestions := []string{
		loc.Get("Validate HEAD commit: gommitlint validate"),
		loc.Get("Validate from file: gommitlint validate -f commit-msg.txt"),
		loc.Get("Validate from stdin: echo \"feat: message\" | gommitlint validate -f -"),
		loc.Get("Validate branch: gommitlint validate --base-branch main"),
		loc.Get("Check git log: git log --oneline -10"),
	}

	// Specific to commit-count overflow: surface the config knob that
	// actually raises the cap. The generic suggestions don't reveal it.
	cause := err
	if errors.Is(err, cliAdapter.ErrCommitCountExceedsMax) {
		suggestions = append(suggestions,
			loc.Get("To validate more commits, raise limits.max_commit_count in .gommitlint.yaml"),
		)
		cause = errors.Join(
			domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonTooManyCommits, err.Error()),
			err,
		)
	}

	return newRecoveryErrorWithCause(
		cause,
		output.Msg("failed to create validation target: %s", err),
		suggestions...,
	)
}

func validationRequiresRepositoryError(targetType string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	message := fmt.Sprintf("validation target %q requires a git repository", targetType)
	cause := domain.New("cli", diagnostics.ReasonInvalidRepo, message)

	return newRecoveryErrorWithCause(
		cause,
		output.Msg("validation target %q requires a git repository", targetType),
		loc.Get("Run inside a git repository"),
		loc.Get("Specify repository path: gommitlint validate --repo-path /path/to/repo"),
		loc.Get("Validate from file: gommitlint validate -f commit-msg.txt"),
		loc.Get("Validate from stdin: echo \"feat: message\" | gommitlint validate -f -"),
	)
}

// writeCommandOutput writes output to the command writer with proper error handling.
// Returns an error if writing fails, which typically indicates a broken pipe or I/O issue.
func writeCommandOutput(w io.Writer, text string, localizer output.Localizer) error {
	_, err := fmt.Fprint(w, text)
	if err != nil {
		cause := errors.Join(ErrOutputWrite, err)
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			cause,
			output.Msg("failed to write command output"),
			loc.Get("Check output destination"),
		)
	}

	return nil
}
