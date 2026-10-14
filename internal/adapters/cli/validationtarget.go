// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// Validation target errors.
var (
	ErrCommitCountExceedsMax = errors.New("commit count exceeds maximum allowed value")
)

// ValidationTarget describes what should be validated.
type ValidationTarget = appValidation.Target

// NewValidationTarget creates a ValidationTarget from CLI parameters.
// It uses precedence-based logic to determine validation target.
func NewValidationTarget(messageFile, gitReference, commitRange, baseBranch string, commitCount int, maxCommitCount int) (ValidationTarget, error) {
	switch {
	case messageFile != "":
		if err := validateMessageFile(messageFile); err != nil {
			return ValidationTarget{}, err
		}

		return createMessageTarget(messageFile), nil
	case baseBranch != "":
		if err := validateBaseBranch(baseBranch); err != nil {
			return ValidationTarget{}, err
		}

		return appValidation.BaseBranchTarget(ParseBaseBranchCandidates(baseBranch)[0]), nil
	case commitRange != "":
		parsedRange, err := validateCommitRange(commitRange)
		if err != nil {
			return ValidationTarget{}, err
		}

		return appValidation.RangeTarget(parsedRange.From, parsedRange.To), nil
	case gitReference != "":
		if err := validateGitReference(gitReference); err != nil {
			return ValidationTarget{}, err
		}

		return appValidation.CommitTarget(gitReference), nil
	default:
		if err := validateCommitCount(commitCount, maxCommitCount); err != nil {
			return ValidationTarget{}, fmt.Errorf("invalid commit count: %w", err)
		}

		if commitCount > 1 {
			return appValidation.CountTarget(commitCount), nil
		}

		return appValidation.DefaultTarget(), nil
	}
}

// createMessageTarget creates a ValidationTarget for message files.
func createMessageTarget(messageFile string) ValidationTarget {
	// Don't use filepath.Clean for stdin marker "-"
	// filepath.Clean would convert "-" to "." on some systems
	source := messageFile
	if messageFile != "-" {
		source = filepath.Clean(messageFile)
	}

	return appValidation.MessageTarget(source)
}

// validateMessageFile validates the message file parameter.
//
// A file the caller named but that does not exist is EX_NOINPUT, not misuse.
// The classified diagnostic includes the unusable path.
func validateMessageFile(messageFile string) error {
	if messageFile == "" || messageFile == "-" {
		return nil
	}

	_, err := ValidateMessageFilePath(messageFile)
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrMessageFileNotExist) {
		return domain.New(
			diagnostics.SourceIDValidation,
			diagnostics.ReasonMessageFileMissing,
			"message file does not exist: "+messageFile,
		)
	}

	return fmt.Errorf("invalid message file %s: %w", messageFile, err)
}

// validateRef validates a git reference string and returns a structured error when invalid.
// label is prefixed to the value in the error message (e.g. "Invalid git reference: ").
func validateRef(ref, label string) error {
	if ref == "" {
		return nil
	}

	if err := ValidateGitReference(ref); err != nil {
		return domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonInvalidReference, label+ref)
	}

	return nil
}

// validateGitReference validates the git reference parameter.
func validateGitReference(gitReference string) error {
	return validateRef(gitReference, "Invalid git reference: ")
}

// validateBaseBranch validates the base branch parameter.
// Supports comma-separated lists of branch candidates.
func validateBaseBranch(baseBranch string) error {
	candidates := ParseBaseBranchCandidates(baseBranch)
	if baseBranch != "" && len(candidates) == 0 {
		return domain.New(
			diagnostics.SourceIDValidation,
			diagnostics.ReasonInvalidReference,
			"Invalid base branch: "+baseBranch,
		)
	}

	for _, candidate := range candidates {
		if err := validateRef(candidate, "Invalid base branch: "); err != nil {
			return err
		}
	}

	return nil
}

// validateCommitRange validates the commit range parameter.
func validateCommitRange(commitRange string) (domain.CommitRange, error) {
	if commitRange != "" {
		parsed, err := ParseSecureCommitRange(commitRange)
		if err != nil {
			return domain.CommitRange{}, domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonInvalidCommitRange,
				"Invalid commit range: "+commitRange)
		}

		return parsed, nil
	}

	return domain.CommitRange{}, nil
}

// ParseBaseBranchCandidates splits a comma-separated base branch string
// into trimmed, non-empty candidates preserving the original order.
func ParseBaseBranchCandidates(baseBranch string) []string {
	parts := strings.Split(baseBranch, ",")
	candidates := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	}

	return candidates
}

// validateCommitCount checks if a commit count is reasonable.
func validateCommitCount(count int, maxCommitCount int) error {
	if err := appValidation.ValidateCommitCount(count); err != nil {
		return err
	}

	if maxCommitCount <= 0 {
		return fmt.Errorf("%w (%d)", ErrCommitCountExceedsMax, maxCommitCount)
	}

	if count > maxCommitCount {
		return fmt.Errorf("%w (%d)", ErrCommitCountExceedsMax, maxCommitCount)
	}

	return nil
}
