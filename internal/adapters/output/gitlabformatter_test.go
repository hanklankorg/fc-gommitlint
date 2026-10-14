// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

func TestGitLab_ValidReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(domain.CommitReport{
			Commit: domain.Commit{
				Hash:        "abc1234567890",
				Subject:     testCommitSubjectFeatNew,
				Message:     testCommitMessageFeatNew,
				Author:      testTestUserName,
				AuthorEmail: testUserEmail,
			},
			Passed: false,
			RuleResults: []domain.RuleReport{
				{
					Name:    testRuleTestRule,
					Status:  domain.StatusFailed,
					Message: testRuleFailedMessage,
					Errors: []domain.ValidationError{
						{
							Rule:       testRuleTestRule,
							Diagnostic: testDiagnostic(testErrorCode),
							Message:    testValidationErrorMessage,
							Context: map[string]string{
								diagnostics.ContextExpected: testContextValueSomething,
								"found":                     testContextValueSomethingElse,
							},
						},
						{
							Rule:       testRuleTestRule,
							Diagnostic: testDiagnostic("another_error"),
							Message:    "Another test error",
							Context:    map[string]string{"line": "1"},
						},
					},
				},
				{
					Name:    testPassingRuleName,
					Status:  domain.StatusPassed,
					Message: testRulePassedMessage,
				},
			},
		}).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, "section_start:", "should contain section start marker")
	check.Contains(t, result, ":gommitlint_summary[collapsed=true]", "should contain summary section")
	check.Contains(t, result, "Gommitlint Summary", "should contain summary title")
	check.Contains(t, result, "section_end:", "should contain section end marker")
	check.Contains(t, result, "Validated 1 commit", "should show commit count")
	check.Contains(t, result, "Passed: 0, Failed: 1", "should show pass/fail counts")

	check.Contains(t, result, ":commit_1\r", "should contain commit section start")
	check.Contains(t, result, "Commit #1: abc1234 - feat: add new feature", "should show commit info")

	check.Contains(t, result, "ERROR:", "should contain error marker")
	check.Contains(t, result, "abc1234 - Test Rule: Test validation error",
		"should contain error with short hash for first error")
	check.Contains(t, result, "abc1234 - Test Rule: Another test error",
		"should contain error with short hash for second error")

	check.Contains(t, result, "1 rule failed", "should show failed rule count")
}

func TestGitLab_PassingReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		at(formatterFixtureTime()).
		passingCommit(testCommitHashDEF5678, testCommitSubjectDocsReadme).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, "Passed: 1, Failed: 0", "should show success counts")
	check.Contains(t, result, "All rules passed", "should show success indicator")
	check.Contains(t, result, "All commit validation rules passed", "should show final success message")

	// Should not contain error messages (but ANSI codes for checkmark are okay)
	check.NotContains(t, result, "ERROR:", "should not contain error messages")
}

func TestGitLab_EmptyReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().at(formatterFixtureTime()).build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, "section_start:",
		"should contain summary section even for empty report")
	check.Contains(t, result, ":gommitlint_summary[collapsed=true]",
		"should contain summary section name")
	check.Contains(t, result, "Validated 0 commits", "should show zero commits")
	check.Contains(t, result, "section_end:", "should close summary section")
	check.Contains(t, result, ":gommitlint_summary\r", "should close summary section by name")
}

func TestGitLab_WithRepositoryResults(t *testing.T) {
	t.Parallel()

	// RepoFailedRules is pinned to 2 to exercise the plural summary text even
	// though only one repository rule is failing here.
	report := newReportBuilder().
		at(formatterFixtureTime()).
		warn(domain.Warning{Code: testValidationWarningCode, Message: "warning one"}).
		repoRule(domain.RuleReport{
			Name:    testBranchRuleName,
			Status:  domain.StatusFailed,
			Message: testBranchRuleFailed,
			Errors: []domain.ValidationError{
				{
					Rule:       testBranchRuleName,
					Diagnostic: testDiagnostic(testValidationBranchCode),
					Message:    testBranchValidationFailed,
					Context: map[string]string{
						testValidationBranchKey: testValidationMainBranch,
					},
				},
			},
		}).
		repoRule(domain.RuleReport{
			Name:    "PassingRepoRule",
			Status:  domain.StatusPassed,
			Message: "Repository rule passed",
		}).
		summary(func(s *domain.ReportSummary) { s.RepoFailedRules = 2 }).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, ":repository[collapsed=true]",
		"should contain repository section start")
	check.Contains(t, result, ":repository\r",
		"should contain repository section end")
	check.Contains(t, result, "Repository Validation", "should contain repository section title")
	check.Contains(t, result, "Repository failures: 2", "should show repository failures in summary")
	check.Contains(t, result, "Configuration Warnings", "should show configuration warnings section")
	check.Contains(t, result, "WARNING:", "should output WARNING prefix")
	check.Contains(t, result, "warning one", "should output warning message")

	check.Contains(t, result, "Branch Rule - Branch validation failed",
		"should contain repository error")
	check.Contains(t, result, "repository rules failed",
		"should mention repository failures")

	check.Contains(t, result, "Passing Repo Rule",
		"should show passing repository rule")
}

func TestGitLab_MultipleCommits(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectFeatFeature).
		failingCommit("def567890123456", "fix: broken test", testRuleTestRule,
			domain.ValidationError{Rule: testRuleTestRule, Message: "Test failed"}).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, ":commit_1[collapsed=true]",
		"should contain first commit section (collapsed because passing)")
	check.Regexp(t, `section_start:\d+:commit_2\r`, result,
		"should open the second commit section without the collapsed flag (failing)")
	check.NotContains(t, result, ":commit_2[collapsed=true]",
		"a failing commit section must not be collapsed")
	check.Contains(t, result, "Commit #1: abc1234", "should contain first commit")
	check.Contains(t, result, "Commit #2: def5678", "should contain second commit with truncated hash")

	check.Contains(t, result, "All rules passed", "should show success for passing commit")
	check.Contains(t, result, "1 rule failed", "should show failure for failing commit")

	check.Contains(t, result, "def5678 - Test Rule: Test failed",
		"should show error with short hash")

	check.Contains(t, result, "Passed: 1, Failed: 1", "should show mixed results")
}

func TestGitLab_EmptyCommitHash(t *testing.T) {
	t.Parallel()

	// Commits with an empty hash are filtered out by the formatter.
	report := newReportBuilder().
		passingCommit("", testCommitSubjectTest).
		passingCommit(testCommitHashABC1234, testCommitSubjectValid).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	// Should only show the valid commit (numbered as #2 since it's at index 1)
	check.Contains(t, result, ":commit_2[collapsed=true]",
		"should show valid commit section")
	check.Contains(t, result, "Commit #2: abc1234", "should show valid commit")
	check.NotContains(t, result, ":commit_1[collapsed",
		"should not show first commit section with empty hash")

	// Should not contain commits with empty hash
	commitSections := 0

	lines := strings.SplitSeq(result, "\n")
	for line := range lines {
		if strings.Contains(line, ":commit_") && strings.Contains(line, "section_start:") {
			commitSections++
		}
	}

	check.Equal(t, 1, commitSections, "should only have one commit section")
}

func TestGitLab_SectionComponents(t *testing.T) {
	t.Parallel()

	// Test that all sections are properly opened and closed
	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectTestCommit).
		repoRule(domain.RuleReport{Name: testRepoRuleName, Status: domain.StatusPassed}).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	// Count sections starts and ends
	sectionStartCount := strings.Count(result, "section_start:")
	sectionEndCount := strings.Count(result, "section_end:")

	check.Equal(t, sectionStartCount, sectionEndCount, "all sections should be properly closed")
	check.GreaterOrEqual(t, sectionStartCount, 2, "should have at least summary and commit sections")
}

func TestGitLab_ShortHashGeneration(t *testing.T) {
	t.Parallel()

	// Test that long hashes are properly shortened to 7 characters
	testCases := []struct {
		name         string
		inputHash    string
		expectedHash string
	}{
		{
			name:         "long hash",
			inputHash:    "abcdef1234567890",
			expectedHash: testCommitHashABCDEF1,
		},
		{
			name:         "exactly 7 characters",
			inputHash:    testCommitHashABCDEF1,
			expectedHash: testCommitHashABCDEF1,
		},
		{
			name:         "short hash",
			inputHash:    testCommitHashABC123,
			expectedHash: testCommitHashABC123,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report := newReportBuilder().
				failingCommit(testCase.inputHash, testCommitSubjectTestCommit, testRuleTestRule,
					domain.ValidationError{Rule: testRuleTestRule, Message: testTestErrorShort}).
				build()

			result := GitLab(report, GitLabOptions{UseColor: true})
			expectedError := testCase.expectedHash + " - Test Rule: Test error"
			check.Contains(t, result, expectedError,
				"should show error with correct hash length")
		})
	}
}

func TestGitLab_SpecialCharacters(t *testing.T) {
	t.Parallel()

	report := specialCharactersReport()
	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, "feat: add \"quotes\" and <brackets>",
		"should preserve special characters in subject")
	check.Contains(t, result, "Error with \"quotes\" and <brackets> & symbols",
		"should preserve special characters in error messages")

	check.Contains(t, result, "section_start:", "should maintain valid GitLab CI format")
	check.Contains(t, result, "section_end:", "should maintain valid GitLab CI format")
}

func TestGitLab_ANSIEscapeSequences(t *testing.T) {
	t.Parallel()

	// Test that ANSI escape sequences are present for proper GitLab CI rendering
	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, testCommitSubjectTest, testRuleTestRule,
			domain.ValidationError{Rule: testRuleTestRule, Message: "error"}).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	// Check for ANSI escape sequences used to hide section markers
	check.Contains(t, result, "\033[0K", "should contain ANSI clear-to-end-of-line escape")
	check.Contains(t, result, "\r", "should contain carriage return for section markers")

	// Check for color codes
	check.Contains(t, result, "\033[31m", "should contain red color code for errors")
	check.Contains(t, result, "\033[0m", "should contain reset color code")
}

func TestGitLab_TimestampFormat(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	// Should contain a real Unix timestamp, not literal $(date +%s)
	check.NotContains(t, result, "$(date +%s)",
		"should not contain shell command literal")

	// The timestamp should be a number (between section_start: and :gommitlint_summary)
	check.Regexp(t, `section_start:\d+:gommitlint_summary`, result,
		"should contain numeric timestamp in section start")
	check.Regexp(t, `section_end:\d+:gommitlint_summary`, result,
		"should contain numeric timestamp in section end")
}

func TestGitLab_ShowsSkippedRepoRules(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectTestCommit).
		summary(func(s *domain.ReportSummary) {
			s.SkippedRepoRules = 2
			s.IgnoreRepoRules = true
		}).
		build()

	result := GitLab(report, GitLabOptions{UseColor: true})

	check.Contains(t, result, "Repository rules skipped: 2", "should show skipped repo rules in summary")
}

func TestGitLab_UseColorTogglesANSICodes(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, "feat: test commit", testRuleTestRule,
			domain.ValidationError{Message: testTestErrorMessage}).
		warn(domain.Warning{Code: testValidationWarningCode, Message: "Test warning"}).
		build()

	t.Run("color_enabled_has_ansi_codes", func(t *testing.T) {
		t.Parallel()

		result := GitLab(report, GitLabOptions{UseColor: true})

		// Should contain ANSI color codes when colors enabled
		check.Contains(t, result, "\033[31m", "should contain red ANSI code for errors")
		check.Contains(t, result, "\033[33m", "should contain yellow ANSI code for warnings")
		check.Contains(t, result, "\033[0m", "should contain reset ANSI code")
	})

	t.Run("color_disabled_no_ansi_color_codes", func(t *testing.T) {
		t.Parallel()

		result := GitLab(report, GitLabOptions{UseColor: false})

		// Should NOT contain color ANSI codes when colors disabled
		// Note: We're checking for color codes, not all ANSI codes
		check.NotContains(t, result, "\033[31m", "should not contain red ANSI code")
		check.NotContains(t, result, "\033[32m", "should not contain green ANSI code")
		check.NotContains(t, result, "\033[33m", "should not contain yellow ANSI code")
		check.NotContains(t, result, "\033[0m", "should not contain reset ANSI code")

		// But should still have content
		check.Contains(t, result, "ERROR:", "should still have error label")
		check.Contains(t, result, "WARNING:", "should still have warning label")
		check.Contains(t, result, testTestErrorMessage, "should still have error message")
	})

	t.Run("no_color_preserves_section_structure", func(t *testing.T) {
		t.Parallel()

		result := GitLab(report, GitLabOptions{UseColor: false})

		// Section markers should still be present (they use \r not colors)
		check.Contains(t, result, "section_start:", "should still have section markers")
		check.Contains(t, result, "section_end:", "should still have section end markers")
		check.Contains(t, result, "gommitlint_summary", "should still have summary section")
	})
}
