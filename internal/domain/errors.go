// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"fmt"
	"maps"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// ExitCodeType classifies validation errors into process exit codes.
// See docs/modules/ROOT/pages/reference/commands.adoc ("Exit Codes") for the full contract.
type ExitCodeType int

// Exit codes follow BSD sysexits.h for environmental failures; 0/1/2 retain
// their POSIX meanings. See docs/modules/ROOT/pages/reference/commands.adoc for the full table.
const (
	ExitCodeValidation    ExitCodeType = 1  // rule failure (POSIX)
	ExitCodeUsage         ExitCodeType = 2  // CLI misuse / non-signal cancellation (POSIX)
	ExitCodeUsageError    ExitCodeType = 64 // EX_USAGE: no subcommand supplied (bare invocation)
	ExitCodeDataErr       ExitCodeType = 65 // EX_DATAERR: malformed input
	ExitCodeNoInput       ExitCodeType = 66 // EX_NOINPUT: named input does not exist
	ExitCodeUnavailable   ExitCodeType = 69 // EX_UNAVAILABLE: git environment missing/failed
	ExitCodeSoftware      ExitCodeType = 70 // EX_SOFTWARE: internal subsystem failure
	ExitCodeNoPerm        ExitCodeType = 77 // EX_NOPERM: signing-key permission/trust failure
	ExitCodeConfiguration ExitCodeType = 78 // EX_CONFIG: bad config file
)

// GetExitCodeType classifies a validation reason. New reasons default to
// ExitCodeValidation; add an explicit case only for environmental, input,
// permission, or internal-software failures.
func GetExitCodeType(reason diagnostics.Reason) ExitCodeType {
	switch reason {
	case diagnostics.ReasonInvalidConfig, diagnostics.ReasonRepoRuleNoRepo:
		return ExitCodeConfiguration

	case diagnostics.ReasonCommitNotFound, diagnostics.ReasonRangeNotFound, diagnostics.ReasonMissingReference,
		diagnostics.ReasonConfigNotFound, diagnostics.ReasonMessageFileMissing:
		return ExitCodeNoInput

	case diagnostics.ReasonInvalidCommit, diagnostics.ReasonInvalidReference, diagnostics.ReasonInvalidCommitRange, diagnostics.ReasonTooManyCommits:
		return ExitCodeDataErr

	case diagnostics.ReasonInvalidRepo, diagnostics.ReasonGitOperationFailed:
		return ExitCodeUnavailable

	case diagnostics.ReasonSpellCheckFailed:
		return ExitCodeSoftware

	case diagnostics.ReasonNoKeyDir, diagnostics.ReasonInvalidKeyDir, diagnostics.ReasonMissingKeyDirectory, diagnostics.ReasonInvalidKeyDirectory,
		diagnostics.ReasonNoTrustedKeys, diagnostics.ReasonKeyLoadFailed:
		return ExitCodeNoPerm

	case diagnostics.ReasonCancelled, diagnostics.ReasonContextCancelled:
		return ExitCodeUsage

	default:
		return ExitCodeValidation
	}
}

// ValidationError detected during validation.
type ValidationError struct {
	// Rule is the name of the rule that produced this error.
	Rule string

	// Diagnostic is the structured public diagnostic and internal reason.
	Diagnostic diagnostics.Diagnostic

	// Message is a human-readable error message.
	Message string

	// Context contains additional information about the error.
	Context diagnostics.Context
}

func (e ValidationError) Error() string {
	return e.Message
}

// WithContextMap adds context values to a ValidationError.
func (e ValidationError) WithContextMap(ctx diagnostics.Context) ValidationError {
	result := e

	// Copy existing context plus new values
	newContext := make(diagnostics.Context, len(result.Context)+len(ctx))
	maps.Copy(newContext, result.Context)

	maps.Copy(newContext, ctx)

	result.Context = newContext

	return result
}

// WithActualExpected adds the most common Actual/Expected context pair. Use this
// shorthand when no additional context keys are needed; otherwise compose with
// WithContextMap.
func (e ValidationError) WithActualExpected(actual, expected string) ValidationError {
	return e.WithContextMap(diagnostics.Context{
		diagnostics.ContextActual:   actual,
		diagnostics.ContextExpected: expected,
	})
}

// WithUserMessage updates the error message with a user-friendly version.
// Clear, actionable messages while preserving technical details.
func (e ValidationError) WithUserMessage(format string, args ...any) ValidationError {
	result := e
	result.Message = fmt.Sprintf(format, args...)

	return result
}

// New creates a new ValidationError.
func New(rule string, reason diagnostics.Reason, message string) ValidationError {
	diagnostic := diagnostics.Diagnostic{Reason: reason}
	if definition, ok := diagnostics.ByRuleAndReason(rule, reason); ok {
		diagnostic = definition.Diagnostic(reason)
	} else if definition, ok := diagnostics.BySourceAndReason(rule, reason); ok {
		diagnostic = definition.Diagnostic(reason)
	}

	err := ValidationError{
		Rule:       rule,
		Diagnostic: diagnostic,
		Message:    message,
		Context:    make(diagnostics.Context),
	}

	return err
}
