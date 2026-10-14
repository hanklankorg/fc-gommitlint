// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

// TestLinearHistoryRule_NameMatchesDomainConstant pins the cross-layer
// contract: the dispatcher in internal/domain/validation.go decides whether
// to skip merge commits by matching Rule.Name() against rulecatalog.LinearHistory.
// If the rule's reported name ever drifts from the domain constant, the
// dispatcher silently starts running content rules on merges again.
func TestLinearHistoryRule_NameMatchesDomainConstant(t *testing.T) {
	t.Parallel()

	check.Equal(t, rulecatalog.LinearHistory, rules.NewLinearHistoryRule().Name(),
		"rule name must equal rulecatalog.LinearHistory — dispatcher uses this for merge-skip decisions")
}

func TestLinearHistoryRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commit      domain.Commit
		wantErr     bool
		wantErrCode expectedReason
	}{
		{
			name:    "non_merge_commit_passes",
			commit:  domain.Commit{Hash: testCommitHashABC1234, Subject: testConventionalFeatAdd, IsMergeCommit: false},
			wantErr: false,
		},
		{
			name:        "merge_commit_fails",
			commit:      domain.Commit{Hash: testCommitHashABC1234, Subject: "Merge branch 'feature'", IsMergeCommit: true},
			wantErr:     true,
			wantErrCode: diagnostics.ReasonMergeCommitFound,
		},
		{
			name:        "merge_pull_request_fails",
			commit:      domain.Commit{Hash: testCommitHashDEF5678, Subject: "Merge pull request #123", IsMergeCommit: true},
			wantErr:     true,
			wantErrCode: diagnostics.ReasonMergeCommitFound,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Rule enablement is handled by the factory - the rule itself always validates
			rule := rules.NewLinearHistoryRule()
			errors := rule.Validate(testCase.commit)

			if testCase.wantErr {
				check.Len(t, errors, 1, "expected one error")
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
				check.Equal(t, rulecatalog.LinearHistory, errors[0].Rule)
				check.NotEmpty(t, errors[0].Message)
				check.Contains(t, errors[0].Context, "commit_hash")
				check.Contains(t, errors[0].Context, diagnostics.ContextSubject)
			} else {
				check.Empty(t, errors, "expected no errors")
			}
		})
	}
}

func TestLinearHistoryRule_ErrorDetails(t *testing.T) {
	t.Parallel()

	commit := domain.Commit{
		Hash:          "abc1234567890",
		Subject:       "Merge branch 'feature/awesome' into main",
		IsMergeCommit: true,
	}

	rule := rules.NewLinearHistoryRule()
	errors := rule.Validate(commit)

	check.Len(t, errors, 1)

	err := errors[0]
	check.Equal(t, rulecatalog.LinearHistory, err.Rule)
	check.Equal(t, diagnostics.ReasonMergeCommitFound, reasonOf(err))
	check.Contains(t, err.Message, testCommitHashABC1234) // Short hash
	check.Contains(t, err.Message, "linear history")
	check.Equal(t, "abc1234567890", err.Context["commit_hash"])
	check.Equal(t, "Merge branch 'feature/awesome' into main", err.Context[diagnostics.ContextSubject])
	check.Equal(t, "merge commit", err.Context[diagnostics.ContextActual])
	check.Equal(t, "non-merge commit (linear history)", err.Context[diagnostics.ContextExpected])
}

func TestLinearHistoryRule_ShortHashHandling(t *testing.T) {
	t.Parallel()

	// The whole message is compared: a substring check would accept an
	// untruncated hash, since the short form is a prefix of the long one.
	tests := []struct {
		name        string
		hash        string
		wantMessage string
	}{
		{
			name:        "full_40_char_hash",
			hash:        "abc1234567890abcdef1234567890abcdef1234",
			wantMessage: "Merge commit abc1234 violates linear history requirement",
		},
		{
			name:        "short_7_char_hash",
			hash:        testCommitHashABC1234,
			wantMessage: "Merge commit abc1234 violates linear history requirement",
		},
		{
			name:        "very_short_hash",
			hash:        "abc",
			wantMessage: "Merge commit abc violates linear history requirement",
		},
		{
			name:        "empty_hash",
			hash:        "",
			wantMessage: "Merge commit  violates linear history requirement",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := domain.Commit{Hash: testCase.hash, Subject: "Merge commit", IsMergeCommit: true}

			rule := rules.NewLinearHistoryRule()
			errors := rule.Validate(commit)

			check.Len(t, errors, 1)
			check.Equal(t, testCase.wantMessage, errors[0].Message)
		})
	}
}
