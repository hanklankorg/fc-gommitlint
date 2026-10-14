// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const mergeTestSubjectRule = rulecatalog.Subject

type silentLogger struct{}

func (silentLogger) Log(_ string, _ string, _ ...any)   {}
func (silentLogger) Trace(_ string, _ ...any)           {}
func (silentLogger) TraceFunc(_ func() (string, []any)) {}
func (silentLogger) Debug(_ string, _ ...any)           {}
func (silentLogger) Info(_ string, _ ...any)            {}
func (silentLogger) Warn(_ string, _ ...any)            {}
func (silentLogger) Error(_ string, _ ...any)           {}

type stubRule struct {
	name string
	errs []domain.ValidationError
}

func (r stubRule) Name() string { return r.name }
func (r stubRule) Validate(_ domain.Commit) []domain.ValidationError {
	return r.errs
}

// TestValidateCommit_MergeCommitSkippedWhenLinearHistoryDisabled verifies the
// dispatcher skips merge commits entirely (no rules invoked) when the
// LinearHistory rule is not in the active set.
func TestValidateCommit_MergeCommitSkippedWhenLinearHistoryDisabled(t *testing.T) {
	t.Parallel()

	merge := domain.Commit{
		Hash:          "abc1234",
		Subject:       "Merge pull request #42 from foo/bar",
		IsMergeCommit: true,
	}

	failing := stubRule{
		name: mergeTestSubjectRule,
		errs: []domain.ValidationError{domain.New(mergeTestSubjectRule, "x", "should not run")},
	}

	result := domain.ValidateCommit(merge, []domain.CommitRule{failing}, settings.Config{}, silentLogger{})

	check.False(t, result.HasFailures(), "merge commits must be skipped when LinearHistory is disabled")
	check.Empty(t, result.Errors)
}

// TestValidateCommit_MergeCommitValidatedWhenLinearHistoryEnabled verifies that
// when LinearHistory is enabled, merge commits ARE validated (and the rule
// produces its own error). Other rules still run.
func TestValidateCommit_MergeCommitValidatedWhenLinearHistoryEnabled(t *testing.T) {
	t.Parallel()

	merge := domain.Commit{
		Hash:          "abc1234",
		Subject:       "Merge pull request #42 from foo/bar",
		IsMergeCommit: true,
	}

	linearHistory := stubRule{
		name: rulecatalog.LinearHistory,
		errs: []domain.ValidationError{domain.New(rulecatalog.LinearHistory, "merge_commit_found", "merge commit")},
	}
	other := stubRule{
		name: mergeTestSubjectRule,
		errs: []domain.ValidationError{domain.New(mergeTestSubjectRule, "case", "wrong case")},
	}

	result := domain.ValidateCommit(merge, []domain.CommitRule{linearHistory, other}, settings.Config{}, silentLogger{})

	check.True(t, result.HasFailures())
	check.Len(t, result.Errors, 2, "with LinearHistory enabled, merge commits run the full pipeline")
}

// TestValidateCommit_NonMergeCommitAlwaysValidated verifies non-merge commits
// run rules regardless of LinearHistory state.
func TestValidateCommit_NonMergeCommitAlwaysValidated(t *testing.T) {
	t.Parallel()

	commit := domain.Commit{Hash: "def", Subject: "feat: x", IsMergeCommit: false}
	rule := stubRule{
		name: rulecatalog.Subject,
		errs: []domain.ValidationError{domain.New(rulecatalog.Subject, "x", "fail")},
	}

	result := domain.ValidateCommit(commit, []domain.CommitRule{rule}, settings.Config{}, silentLogger{})

	check.True(t, result.HasFailures(), "non-merge commits must always be validated")
}

// TestValidateCommit_MergeSkipDependsOnFlagNotSubject is a boundary test: the
// merge-skip must branch on the trusted IsMergeCommit boolean (set by the git
// adapter from parent count), not on user-controllable subject content.
// A crafted subject that LOOKS like a merge must still be validated when
// IsMergeCommit is false.
func TestValidateCommit_MergeSkipDependsOnFlagNotSubject(t *testing.T) {
	t.Parallel()

	fakeMerge := domain.Commit{
		Hash:          "aaa1111",
		Subject:       "Merge pull request #1 from attacker/bypass",
		IsMergeCommit: false, // attacker can't flip this — it comes from parent count
	}

	rule := stubRule{
		name: rulecatalog.Subject,
		errs: []domain.ValidationError{domain.New(rulecatalog.Subject, "case", "must fire")},
	}

	result := domain.ValidateCommit(fakeMerge, []domain.CommitRule{rule}, settings.Config{}, silentLogger{})

	check.True(t, result.HasFailures(),
		"commits with merge-like subjects but IsMergeCommit=false must be validated; skip must not be content-based")
}
