// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

func TestTable_SummaryAndRows(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Summary: domain.ReportSummary{
			TotalCommits:     1,
			ValidatedCommits: 1,
			IgnoredCommits:   0,
			PassedCommits:    1,
			FailedCommits:    0,
			AllPassed:        true,
		},
		Commits: []domain.CommitReport{
			{
				Commit: domain.Commit{
					Hash:    testCommitHashABC1234,
					Subject: "feat: add table output test",
				},
				Passed: true,
				RuleResults: []domain.RuleReport{
					{
						Name:    rules.SubjectRule{}.Name(),
						Status:  domain.StatusPassed,
						Message: "Passed",
					},
				},
			},
		},
		Repository: domain.RepositoryReport{
			RuleResults: []domain.RuleReport{
				{
					Name:    rules.BranchAheadRule{}.Name(),
					Status:  domain.StatusPassed,
					Message: "Passed",
				},
			},
		},
	}

	result := Table(report, TableOptions{Colors: GetColorScheme(false, "")})

	check.Contains(t, result, "SUMMARY")
	check.Contains(t, result, "Rules: Total 2")
	check.Contains(t, result, "Commits: Validated 1")
	check.Contains(t, result, testRuleSubjectName)
	check.Contains(t, result, "Branch Ahead")
	check.Contains(t, result, "PASS")
}

// TestTable_TitleFollowsTheOutcome pins the title decision the formatter makes
// from the aggregate: a PASSED header over a failing run is the bug its comment
// warns about, and the passing test above can only ever see the PASSED branch.
func TestTable_TitleFollowsTheOutcome(t *testing.T) {
	t.Parallel()

	failing := newReportBuilder().
		failingCommit(testCommitHashABC1234, "bad subject", rules.SubjectRule{}.Name(),
			domain.ValidationError{Message: "Subject too long"}).
		build()
	passing := newReportBuilder().passingCommit(testCommitHashABC1234, "feat: good subject").build()

	for _, testCase := range []struct {
		name      string
		report    domain.Report
		wantTitle string
		notTitle  string
	}{
		{name: "a failed rule gives the FAILED title", report: failing, wantTitle: "✗ VALIDATION FAILED", notTitle: "VALIDATION PASSED"},
		{name: "the control: no failed rule gives the PASSED title", report: passing, wantTitle: "✓ VALIDATION PASSED", notTitle: "VALIDATION FAILED"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := Table(testCase.report, TableOptions{Colors: GetColorScheme(false, "")})

			check.Contains(t, result, testCase.wantTitle)
			check.NotContains(t, result, testCase.notTitle)
		})
	}
}
