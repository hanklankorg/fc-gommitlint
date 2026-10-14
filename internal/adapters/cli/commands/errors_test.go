// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Test constants.
const (
	commitMsgHookType = "commit-msg"
)

// Test error constants for err113 compliance.
var (
	ErrNotGitRepository   = errors.New("not a git repository")
	ErrInvalidYAMLSyntax  = errors.New("invalid YAML syntax")
	errMissingRef         = errors.New("missing-ref")
	errCommitCountMaximum = errors.New("maximum is 100")
)

func TestRecoveryError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		message      string
		suggestions  []string
		wantContains []string
	}{
		{
			name:        "error without suggestions",
			message:     "simple error",
			suggestions: nil,
			wantContains: []string{
				"simple error",
			},
		},
		{
			name:        "error with single suggestion",
			message:     "something went wrong",
			suggestions: []string{"try this command"},
			wantContains: []string{
				"something went wrong",
				testOutputNextSteps,
				"• try this command",
			},
		},
		{
			name:    "error with multiple suggestions",
			message: "validation failed",
			suggestions: []string{
				"check your config",
				"run with --verbose",
				"see documentation",
			},
			wantContains: []string{
				"validation failed",
				testOutputNextSteps,
				"• check your config",
				"• run with --verbose",
				"• see documentation",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := RecoveryError{Msg: output.Msg(testCase.message), Suggestions: testCase.suggestions}
			errorStr := err.Error()

			for _, expected := range testCase.wantContains {
				check.Contains(t, errorStr, expected)
			}
		})
	}
}

func TestHookAlreadyExistsError(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	hookPath := "/test/.git/hooks/commit-msg"
	err := hookAlreadyExistsError(hookPath, nil)

	errorStr := err.Error()
	check.Contains(t, errorStr, "hook already exists at /test/.git/hooks/commit-msg")
	check.Contains(t, errorStr, "Next steps:")
	check.Contains(t, errorStr, "View current hook: cat '/test/.git/hooks/commit-msg'")
	check.Contains(t, errorStr, "Check hook status: gommitlint hook status")
	check.Contains(t, errorStr, "Force overwrite: gommitlint hook install --force")
}

func TestInvalidRepositoryError(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	originalErr := ErrNotGitRepository
	err := invalidRepositoryError(originalErr, nil)

	errorStr := err.Error()
	check.Contains(t, errorStr, "invalid repository path: not a git repository")
	check.Contains(t, errorStr, "Next steps:")
	check.Contains(t, errorStr, "Change to a git repository: cd /path/to/repo")
	check.Contains(t, errorStr, "Initialize git repository: git init")
	check.Contains(t, errorStr, "Specify repository path: gommitlint --repo-path /path/to/repo")
	check.Contains(t, errorStr, "Check current directory: pwd")

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonInvalidRepo, validationErr.Diagnostic.Reason)
}

func TestConfigLoadError(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	originalErr := ErrInvalidYAMLSyntax
	err := configLoadError(originalErr, nil)
	check.ErrorIs(t, err, originalErr)

	errorStr := err.Error()
	check.Contains(t, errorStr, "failed to load configuration: invalid YAML syntax")
	check.Contains(t, errorStr, "Next steps:")
	check.Contains(t, errorStr, "Check config syntax: gommitlint config show")
	check.Contains(t, errorStr, "Create default config: gommitlint config init")
	check.Contains(t, errorStr, "Use default config: gommitlint validate --ignore-config")
	check.Contains(t, errorStr, "Specify config file: gommitlint --config /path/to/settings.yaml")
	check.Contains(t, errorStr, "View config docs: https://codefloe.com/itiquette/gommitlint#configuration")

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonInvalidConfig, validationErr.Diagnostic.Reason)
}

func TestConfigLoadError_ConfigFileNotFound(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	err := configLoadError(ErrConfigFileNotFound, nil)
	check.ErrorIs(t, err, ErrConfigFileNotFound)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ID("GL904"), validationErr.Diagnostic.ID)
	check.Equal(t, diagnostics.ReasonConfigNotFound, validationErr.Diagnostic.Reason)
}

func TestConfigLoadError_WorldWritablePermissions(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	// World-writable is the only remaining fatal permission state — group-writable is
	// downgraded to a runtime warning (see WarnConfigInsecurePerms).
	pathErr := newConfigPathValidationError(testConfigFileName, configuration.ErrConfigPathWorldWritable)
	err := configLoadError(pathErr, nil)
	check.ErrorIs(t, err, configuration.ErrConfigPathWorldWritable)

	errorStr := err.Error()
	check.Contains(t, errorStr, "failed to load configuration: invalid config path settings.yaml: insecure permissions (world-writable)")
	check.Contains(t, errorStr, "Fix config file permissions: chmod o-w -- 'settings.yaml'")
	check.Contains(t, errorStr, "Ensure the config file is owned by your user")
	check.Contains(t, errorStr, "Use default config: gommitlint validate --ignore-config")
}

func TestValidationTargetError(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	originalErr := domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonInvalidCommitRange, "invalid commit range")
	err := validationTargetError(originalErr, nil)

	errorStr := err.Error()
	check.Contains(t, errorStr, "failed to create validation target: invalid commit range")
	check.Contains(t, errorStr, "Next steps:")
	check.Contains(t, errorStr, "Validate HEAD commit: gommitlint validate")
	check.Contains(t, errorStr, "Validate from file: gommitlint validate -f commit-msg.txt")
	check.Contains(t, errorStr, "Validate from stdin: echo \"feat: message\" | gommitlint validate -f -")
	check.Contains(t, errorStr, "Validate branch: gommitlint validate --base-branch main")
	check.Contains(t, errorStr, "Check git log: git log --oneline -10")

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonInvalidCommitRange, validationErr.Diagnostic.Reason)
}

func TestValidationTargetError_CommitCountOverflowIsDataError(t *testing.T) {
	t.Parallel()

	originalErr := errors.Join(cliAdapter.ErrCommitCountExceedsMax, errCommitCountMaximum)
	err := validationTargetError(originalErr, nil)

	check.ErrorIs(t, err, cliAdapter.ErrCommitCountExceedsMax)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonTooManyCommits, validationErr.Diagnostic.Reason)
	check.Equal(t, domain.ExitCodeDataErr, domain.GetExitCodeType(validationErr.Diagnostic.Reason))
}

func TestMapValidationServiceError_ReferenceNotFoundUsesTargetDiagnostic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		target appValidation.Target
		wantID diagnostics.ID
		want   diagnostics.Reason
	}{
		{
			name:   "commit_target",
			target: appValidation.CommitTarget("missing-ref"),
			wantID: "GL907",
			want:   diagnostics.ReasonCommitNotFound,
		},
		{
			name:   "range_target",
			target: appValidation.RangeTarget("main", "missing-ref"),
			wantID: "GL908",
			want:   diagnostics.ReasonRangeNotFound,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			setup := ValidationSetup{Target: testCase.target}
			originalErr := errors.Join(domain.ErrReferenceNotFound, errMissingRef)
			err := mapValidationServiceError(originalErr, setup, setup.OutputOptions, nil)

			check.ErrorIs(t, err, domain.ErrReferenceNotFound)

			var validationErr domain.ValidationError
			check.ErrorAs(t, err, &validationErr)
			check.Equal(t, testCase.wantID, validationErr.Diagnostic.ID)
			check.Equal(t, testCase.want, validationErr.Diagnostic.Reason)
		})
	}
}

func TestMapValidationServiceError_RangeNotFoundValidationErrorAddsRecovery(t *testing.T) {
	t.Parallel()

	originalErr := domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonRangeNotFound, "base branch not found: missing1, missing2")
	setup := ValidationSetup{}
	err := mapValidationServiceError(originalErr, setup, setup.OutputOptions, nil)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ID("GL908"), validationErr.Diagnostic.ID)
	check.Equal(t, diagnostics.ReasonRangeNotFound, validationErr.Diagnostic.Reason)

	errorStr := err.Error()
	check.Contains(t, errorStr, "base branch not found: missing1, missing2")
	check.Contains(t, errorStr, "List available branches: git branch -a")
	check.Contains(t, errorStr, "Use an existing base branch: gommitlint validate --base-branch main")
}

func TestMapValidationServiceError_EmptyMessageUsesInputDiagnostic(t *testing.T) {
	t.Parallel()

	setup := ValidationSetup{Target: appValidation.MessageTarget("commit-msg.txt")}
	err := mapValidationServiceError(appValidation.ErrEmptyCommitMessage, setup, setup.OutputOptions, nil)

	check.ErrorIs(t, err, appValidation.ErrEmptyCommitMessage)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ID("GL903"), validationErr.Diagnostic.ID)
	check.Equal(t, diagnostics.ReasonInvalidCommit, validationErr.Diagnostic.Reason)
}

// TestMapValidationServiceError_MissingRepositoryIsUnavailable covers the
// conversion that keeps "you are not in a repository" out of the usage-error
// bucket: both sentinels the CLI can receive must become ReasonInvalidRepo,
// which the exit-code table maps to EX_UNAVAILABLE (69).
func TestMapValidationServiceError_MissingRepositoryIsUnavailable(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name     string
		sentinel error
	}{
		{name: "fileops sentinel", sentinel: fileops.ErrNotGitRepository},
		{name: "domain sentinel", sentinel: domain.ErrNotGitRepository},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			setup := ValidationSetup{}
			err := mapValidationServiceError(fmt.Errorf("create repository: %w", testCase.sentinel), setup, setup.OutputOptions, nil)

			check.Contains(t, err.Error(), "failed to create repository")

			var validationErr domain.ValidationError
			check.ErrorAs(t, err, &validationErr)
			check.Equal(t, diagnostics.ReasonInvalidRepo, validationErr.Diagnostic.Reason)
			check.Equal(t, domain.ExitCodeUnavailable, domain.GetExitCodeType(validationErr.Diagnostic.Reason))
		})
	}
}

func TestMapValidationServiceError_ContextCancelledUsesDispatcherDiagnostic(t *testing.T) {
	t.Parallel()

	setup := ValidationSetup{}
	err := mapValidationServiceError(context.Canceled, setup, setup.OutputOptions, nil)

	check.ErrorIs(t, err, context.Canceled)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ID("GL906"), validationErr.Diagnostic.ID)
	check.Equal(t, diagnostics.ReasonContextCancelled, validationErr.Diagnostic.Reason)
}

func TestMapValidationServiceError_DeadlineExceededPreservesContext(t *testing.T) {
	t.Parallel()

	setup := ValidationSetup{}
	err := mapValidationServiceError(context.DeadlineExceeded, setup, setup.OutputOptions, nil)

	check.ErrorIs(t, err, context.DeadlineExceeded)
	check.Contains(t, err.Error(), "validation timed out")
	check.NotContains(t, err.Error(), "--help")
}

func TestRecoveryErrorFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		message     string
		suggestions []string
		wantFormat  func(string) bool
	}{
		{
			name:        "no suggestions",
			message:     "simple message",
			suggestions: nil,
			wantFormat: func(s string) bool {
				return s == "simple message" && !strings.Contains(s, "Next steps:")
			},
		},
		{
			name:        "with suggestions",
			message:     "error occurred",
			suggestions: []string{"fix it", "try again"},
			wantFormat: func(s string) bool {
				lines := strings.Split(s, "\n")

				return len(lines) == 5 &&
					lines[0] == "error occurred" &&
					lines[1] == "" &&
					lines[2] == "Next steps:" &&
					lines[3] == "  • fix it" &&
					lines[4] == "  • try again"
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := RecoveryError{
				Msg:         output.Msg(testCase.message),
				Suggestions: testCase.suggestions,
			}

			result := err.Error()
			check.True(t, testCase.wantFormat(result), "Unexpected format: %q", result)
		})
	}
}

// Test error constants for writeCommandOutput tests.
var errMockWrite = errors.New("mock write error")

// failingWriter is a mock writer that always fails.
type failingWriter struct {
	err error
}

func (f failingWriter) Write(p []byte) (int, error) {
	return 0, f.err
}

func TestWriteCommandOutput_Success(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := writeCommandOutput(&buf, "test output", nil)

	check.NoError(t, err)
	check.Equal(t, "test output", buf.String())
}

func TestWriteCommandOutput_FailingWriter(t *testing.T) {
	t.Parallel()

	writer := failingWriter{err: errMockWrite}

	err := writeCommandOutput(writer, "test output", nil)

	check.Error(t, err)
	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.NotContains(t, strings.Join(recoveryErr.Suggestions, "\n"), "--report-file")
}

func TestWriteCommandOutput_EmptyString(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	err := writeCommandOutput(&buf, "", nil)

	check.NoError(t, err)
	check.Empty(t, buf.String())
}

// A missing named message file maps to EX_NOINPUT, not misuse.
func TestValidationTargetError_MissingMessageFileIsNoInput(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	cause := domain.New(
		diagnostics.SourceIDValidation,
		diagnostics.ReasonMessageFileMissing,
		"message file does not exist: /nonexistent",
	)

	err := validationTargetError(cause, nil)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonMessageFileMissing, validationErr.Diagnostic.Reason)
	check.Equal(t, domain.ExitCodeNoInput, domain.GetExitCodeType(validationErr.Diagnostic.Reason))

	errorStr := err.Error()

	// The path the caller passed must appear; "which file?" is the first
	// question a missing-file message has to answer.
	check.Contains(t, errorStr, "/nonexistent")

	// Answer the failure, not the command surface: the five-way "how to invoke
	// validate" menu belongs on target-selection failures, not this one.
	check.Contains(t, errorStr, "Check the path and re-run")
	check.False(t, strings.Contains(errorStr, "Validate branch: gommitlint validate --base-branch main"),
		"the generic target menu must not answer a missing-file failure")
	check.False(t, strings.Contains(errorStr, "failed to create validation target"),
		"the caller passed a path, not a validation target")
}

type hostileRecoveryLocalizer struct{}

func (hostileRecoveryLocalizer) Get(message string, args ...any) string {
	if message == "Next steps:" {
		return "Next\x1b[31m\nforged:"
	}

	return output.EnsureLocalizer(nil).Get(message, args...)
}

func (hostileRecoveryLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return output.EnsureLocalizer(nil).GetN(singular, plural, count, args...)
}

func TestRecoveryError_Format_SanitizesLocalizedHeader(t *testing.T) {
	t.Parallel()

	err := RecoveryError{Msg: output.Msg("failed"), Suggestions: []string{"retry"}}
	formatted := err.Format(hostileRecoveryLocalizer{})

	check.Contains(t, formatted, "Next\\x1b[31m forged:")
	check.NotContains(t, formatted, "\x1b")
}

func TestHookRecoverySuggestions_AreAccurateAndShellSafe(t *testing.T) {
	t.Parallel()

	hookPath := "/tmp/hook $(touch marker)"

	var customRecovery RecoveryError
	check.ErrorAs(t, customHookExistsError(hookPath, "gommitlint hook install", nil), &customRecovery)
	customSuggestions := strings.Join(customRecovery.Suggestions, "\n")
	check.Contains(t, customSuggestions, "cat '/tmp/hook $(touch marker)'")
	check.Contains(t, customSuggestions, "cp '/tmp/hook $(touch marker)' '/tmp/hook $(touch marker)'.backup")
	check.Contains(t, customSuggestions, "rm '/tmp/hook $(touch marker)'")

	var globalRecovery RecoveryError
	check.ErrorAs(t, customHookExistsError(hookPath, "gommitlint hook install --global", nil), &globalRecovery)
	check.Contains(t, strings.Join(globalRecovery.Suggestions, "\n"), "Then install gommitlint: gommitlint hook install --global")

	var removalRecovery RecoveryError
	check.ErrorAs(t, notGommitlintHookError(commitMsgHookType, hookPath, nil), &removalRecovery)
	check.NotContains(t, strings.Join(removalRecovery.Suggestions, "\n"), "install --force")

	var hooksPathRecovery RecoveryError
	check.ErrorAs(t, hooksPathInUseError("hooks $(touch marker)", nil), &hooksPathRecovery)
	hooksPathSuggestions := strings.Join(hooksPathRecovery.Suggestions, "\n")
	check.Contains(t, hooksPathSuggestions, "ls -la -- 'hooks $(touch marker)'")
	check.NotContains(t, hooksPathSuggestions, "commit-msg.d/50-gommitlint.sh")

	var redirectRecovery RecoveryError
	check.ErrorAs(t, hooksPathRedirectedError("custom-hooks", nil), &redirectRecovery)
	check.NotContains(t, strings.Join(redirectRecovery.Suggestions, "\n"), "global dispatcher")
}

func TestShellQuote_QuotesPOSIXShellValues(t *testing.T) {
	t.Parallel()

	check.Equal(t, "''", shellQuote(""))
	check.Equal(t, "'/tmp/hook $(touch marker)'", shellQuote("/tmp/hook $(touch marker)"))
	check.Equal(t, "'hook'\"'\"'name'", shellQuote("hook'name"))
}
