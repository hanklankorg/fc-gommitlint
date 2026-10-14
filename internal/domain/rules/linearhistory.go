// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// LinearHistoryRule validates that commits are not merge commits.
// This enforces a rebase workflow where feature branches are rebased onto
// the target branch rather than merged.
// Implements domain.CommitRule interface.
type LinearHistoryRule struct{}

// Compile-time interface compliance check.
var _ domain.CommitRule = LinearHistoryRule{}

// NewLinearHistoryRule creates a new rule for enforcing linear history.
// The rule is only created by the factory when enabled via configuration,
// so no internal enabled check is needed.
func NewLinearHistoryRule() LinearHistoryRule {
	return LinearHistoryRule{}
}

// Name implements CommitRule interface.
func (r LinearHistoryRule) Name() string {
	return rulecatalog.LinearHistory
}

// Validate implements CommitRule interface.
// Returns a validation error if the commit is a merge commit.
func (r LinearHistoryRule) Validate(commit domain.Commit) []domain.ValidationError {
	if !commit.IsMergeCommit {
		return nil
	}

	return []domain.ValidationError{r.createError(commit)}
}

// createError creates a validation error for a merge commit.
func (r LinearHistoryRule) createError(commit domain.Commit) domain.ValidationError {
	shortHash := commit.Hash
	if len(shortHash) > domain.ShortHashLength {
		shortHash = shortHash[:domain.ShortHashLength]
	}

	message := fmt.Sprintf("Merge commit %s violates linear history requirement", shortHash)

	return domain.New(r.Name(), diagnostics.ReasonMergeCommitFound, message).
		WithContextMap(diagnostics.Context{
			diagnostics.ContextCommitHash: commit.Hash,
			diagnostics.ContextSubject:    commit.Subject,
			diagnostics.ContextActual:     "merge commit",
			diagnostics.ContextExpected:   "non-merge commit (linear history)",
		})
}
