// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// BranchAheadRule validates that the current branch is not too many commits ahead
// of the reference branch.
type BranchAheadRule struct {
	maxCommitsAhead int
	reference       string
}

// NewBranchAheadRule creates a new rule for checking commits ahead of a reference branch.
func NewBranchAheadRule(cfg settings.Config, reference string) BranchAheadRule {
	maxCommitsAhead := settings.DerefPositiveInt(cfg.Repo.MaxCommitsAhead, settings.DefaultMaxCommitsAhead)

	return BranchAheadRule{
		maxCommitsAhead: maxCommitsAhead,
		reference:       reference,
	}
}

// Name implements Rule interface.
func (r BranchAheadRule) Name() string {
	return rulecatalog.BranchAhead
}

// Validate implements RepositoryRule interface.
// Returns a "n/max" info string on pass so the report can display commit counts.
func (r BranchAheadRule) Validate(ctx context.Context, repo domain.BranchAnalyzer) (string, []domain.ValidationError) {
	if configErrors := r.validateConfig(); len(configErrors) > 0 {
		return "", configErrors
	}

	// Skip if no repository is provided
	if repo == nil {
		return "", nil
	}

	if r.reference == "" {
		return "", []domain.ValidationError{
			r.createMissingReferenceError(),
		}
	}

	actualReference := r.reference

	commitsAhead, err := repo.GetCommitsAheadCount(ctx, actualReference)
	if err != nil {
		if errors.Is(err, domain.ErrReferenceNotFound) {
			// For explicitly configured branch that doesn't exist
			if r.reference != "" {
				return "", []domain.ValidationError{
					r.createReferenceNotFoundError(actualReference),
				}
			}

			return "", nil
		}

		// Other errors are actual problems
		return "", r.handleRepositoryError(err)
	}

	// Validate against the maximum
	if commitsAhead > r.maxCommitsAhead {
		return "", []domain.ValidationError{
			r.createValidationError(commitsAhead, actualReference),
		}
	}

	return fmt.Sprintf("%d ahead of '%s' (max %d)", commitsAhead, actualReference, r.maxCommitsAhead), nil
}

// validateConfig validates the rule configuration for potential issues.
func (r BranchAheadRule) validateConfig() []domain.ValidationError {
	var errors []domain.ValidationError

	// Validate maxCommitsAhead is positive
	if r.maxCommitsAhead <= 0 {
		errors = append(errors,
			domain.New(r.Name(), diagnostics.ReasonInvalidConfig,
				fmt.Sprintf("maxCommitsAhead must be positive: %d", r.maxCommitsAhead)).
				WithActualExpected(strconv.Itoa(r.maxCommitsAhead), "positive integer (1 or more)"))
	}

	return errors
}

// handleRepositoryError provides error handling for repository access issues.
func (r BranchAheadRule) handleRepositoryError(err error) []domain.ValidationError {
	if errors.Is(err, domain.ErrRepositoryAccess) {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidRepo,
				"Cannot access repository for branch comparison").
				WithActualExpected("access denied", "accessible repository"),
		}
	}

	// Other git operation errors
	return []domain.ValidationError{
		domain.New(r.Name(), diagnostics.ReasonGitOperationFailed,
			fmt.Sprintf("Failed to check commits ahead of '%s'", r.reference)).
			WithActualExpected("operation failed", "successful git operation"),
	}
}

func (r BranchAheadRule) createMissingReferenceError() domain.ValidationError {
	return domain.New(r.Name(), diagnostics.ReasonMissingReference,
		"Reference branch is required for BranchAhead").
		WithActualExpected("missing reference", "base branch reference")
}

// createReferenceNotFoundError creates an error when explicitly configured reference doesn't exist.
func (r BranchAheadRule) createReferenceNotFoundError(reference string) domain.ValidationError {
	return domain.New(r.Name(), diagnostics.ReasonMissingReference,
		fmt.Sprintf("Reference branch '%s' not found", reference)).
		WithContextMap(diagnostics.Context{
			diagnostics.ContextActual:   "branch not found",
			diagnostics.ContextExpected: fmt.Sprintf("branch '%s' to exist", reference),
			diagnostics.ContextRef:      reference,
		})
}

// createValidationError creates a validation error for excessive commits.
func (r BranchAheadRule) createValidationError(commitsAhead int, referenceBranch string) domain.ValidationError {
	excess := commitsAhead - r.maxCommitsAhead

	message := fmt.Sprintf("Branch is %d commits ahead of '%s' (max allowed: %d)",
		commitsAhead, referenceBranch, r.maxCommitsAhead)

	return domain.New(r.Name(), diagnostics.ReasonTooManyCommits, message).
		WithContextMap(diagnostics.Context{
			diagnostics.ContextActual:   strconv.Itoa(commitsAhead),
			diagnostics.ContextExpected: fmt.Sprintf("max %d", r.maxCommitsAhead),
			diagnostics.ContextOver:     strconv.Itoa(excess),
			diagnostics.ContextRef:      referenceBranch,
		})
}
