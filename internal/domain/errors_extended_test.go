// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// TestGetExitCodeType_MapsEachListedReasonToItsCode pins the reasons that get a
// specific exit code, which is what a CI step branches on.
//
// The switch names the exceptions and sends everything else to
// ExitCodeValidation, so the rows below are every listed exception plus a
// sample of the default, not the whole vocabulary. That asymmetry is
// deliberate rather than a gap -- a validation failure is the ordinary outcome,
// and enumerating every reason here would pin the default in dozens of rows
// that say nothing about it.
func TestGetExitCodeType_MapsEachListedReasonToItsCode(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		reason   diagnostics.Reason
		wantCode ExitCodeType
	}{
		// Configuration error should return exit code 78
		{
			name:     "invalid config error",
			reason:   diagnostics.ReasonInvalidConfig,
			wantCode: ExitCodeConfiguration,
		},
		{
			name:     "repository rule without repository",
			reason:   diagnostics.ReasonRepoRuleNoRepo,
			wantCode: ExitCodeConfiguration,
		},
		// Unavailable git environment (exit code 69)
		{
			name:     "invalid repository error",
			reason:   diagnostics.ReasonInvalidRepo,
			wantCode: ExitCodeUnavailable,
		},
		{
			name:     "git operation failed",
			reason:   diagnostics.ReasonGitOperationFailed,
			wantCode: ExitCodeUnavailable,
		},
		// Missing input (exit code 66)
		{
			name:     "config not found",
			reason:   diagnostics.ReasonConfigNotFound,
			wantCode: ExitCodeNoInput,
		},
		{
			name:     "commit not found",
			reason:   diagnostics.ReasonCommitNotFound,
			wantCode: ExitCodeNoInput,
		},
		{
			name:     "range not found",
			reason:   diagnostics.ReasonRangeNotFound,
			wantCode: ExitCodeNoInput,
		},
		{
			name:     "missing reference",
			reason:   diagnostics.ReasonMissingReference,
			wantCode: ExitCodeNoInput,
		},
		{
			name:     "message file missing",
			reason:   diagnostics.ReasonMessageFileMissing,
			wantCode: ExitCodeNoInput,
		},
		// Malformed user input (exit code 65)
		{
			name:     "invalid commit input",
			reason:   diagnostics.ReasonInvalidCommit,
			wantCode: ExitCodeDataErr,
		},
		{
			name:     "invalid reference",
			reason:   diagnostics.ReasonInvalidReference,
			wantCode: ExitCodeDataErr,
		},
		{
			name:     "invalid commit range",
			reason:   diagnostics.ReasonInvalidCommitRange,
			wantCode: ExitCodeDataErr,
		},
		{
			name:     "too many commits",
			reason:   diagnostics.ReasonTooManyCommits,
			wantCode: ExitCodeDataErr,
		},
		// Permission / trust failures (exit code 77)
		{
			name:     "no key directory",
			reason:   diagnostics.ReasonNoKeyDir,
			wantCode: ExitCodeNoPerm,
		},
		{
			name:     "invalid key directory",
			reason:   diagnostics.ReasonInvalidKeyDir,
			wantCode: ExitCodeNoPerm,
		},
		{
			name:     "missing key directory",
			reason:   diagnostics.ReasonMissingKeyDirectory,
			wantCode: ExitCodeNoPerm,
		},
		{
			name:     "invalid key directory duplicate",
			reason:   diagnostics.ReasonInvalidKeyDirectory,
			wantCode: ExitCodeNoPerm,
		},
		{
			name:     "no trusted keys",
			reason:   diagnostics.ReasonNoTrustedKeys,
			wantCode: ExitCodeNoPerm,
		},
		{
			name:     "key load failed",
			reason:   diagnostics.ReasonKeyLoadFailed,
			wantCode: ExitCodeNoPerm,
		},
		// Internal software error (exit code 70)
		{
			name:     "spell check failed",
			reason:   diagnostics.ReasonSpellCheckFailed,
			wantCode: ExitCodeSoftware,
		},
		// Cancellation (exit code 2 when not signal-driven)
		{
			name:     "cancelled",
			reason:   diagnostics.ReasonCancelled,
			wantCode: ExitCodeUsage,
		},
		{
			name:     "context cancelled",
			reason:   diagnostics.ReasonContextCancelled,
			wantCode: ExitCodeUsage,
		},
		// Validation errors should return exit code 1
		{
			name:     "subject too long",
			reason:   diagnostics.ReasonSubjectTooLong,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "subject case error",
			reason:   diagnostics.ReasonSubjectCase,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "invalid format",
			reason:   diagnostics.ReasonInvalidFormat,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "missing sign-off",
			reason:   diagnostics.ReasonMissingSignoff,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "missing jira reference",
			reason:   diagnostics.ReasonMissingJira,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "invalid type",
			reason:   diagnostics.ReasonInvalidConventionalFormat,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "body too short",
			reason:   diagnostics.ReasonBodyTooShort,
			wantCode: ExitCodeValidation,
		},
		{
			name:     "validation failed",
			reason:   diagnostics.ReasonValidationFailed,
			wantCode: ExitCodeValidation,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			exitCode := GetExitCodeType(testCase.reason)
			check.Equal(t, testCase.wantCode, exitCode)
		})
	}
}

// TestValidationError_ErrorInterface tests that ValidationError implements error interface properly.
func TestValidationError_ErrorInterface(t *testing.T) {
	t.Parallel()

	err := New("test", diagnostics.ReasonValidationFailed, "test error message")

	// Should implement error interface
	var _ error = err

	// Error() method should return the message
	check.Equal(t, "test error message", err.Error())
}

// TestCombineValidationErrors_BehaviorInPractice tests how error combination works in real scenarios.
func TestCombineValidationErrors_BehaviorInPractice(t *testing.T) {
	t.Parallel()

	t.Run("combining errors from multiple rules", func(t *testing.T) {
		t.Parallel()

		// Simulate errors from different validation rules
		subjectErrors := []ValidationError{
			New("subject", diagnostics.ReasonSubjectTooLong, "Subject too long"),
			New("subject", diagnostics.ReasonSubjectCase, "Subject must start with lowercase"),
		}

		bodyErrors := []ValidationError{
			New("body", diagnostics.ReasonBodyTooShort, "Body must be at least 20 characters"),
		}

		conventionalErrors := []ValidationError{
			New("conventional", diagnostics.ReasonInvalidConventionalFormat, "Invalid commit type"),
		}

		// Combine all errors
		allErrors := CombineValidationErrors(subjectErrors, bodyErrors, conventionalErrors)

		// Should preserve all errors in order
		check.Len(t, allErrors, 4)
		check.Equal(t, "subject", allErrors[0].Rule)
		check.Equal(t, "subject", allErrors[1].Rule)
		check.Equal(t, "body", allErrors[2].Rule)
		check.Equal(t, "conventional", allErrors[3].Rule)
	})

	t.Run("handling empty and nil slices", func(t *testing.T) {
		t.Parallel()

		validErrors := []ValidationError{
			New("test", diagnostics.ReasonValidationFailed, "test error"),
		}

		// Should handle nil and empty slices gracefully
		combined := CombineValidationErrors(nil, validErrors, []ValidationError{}, nil)
		check.Len(t, combined, 1)
		check.Equal(t, "test error", combined[0].Message)
	})
}
