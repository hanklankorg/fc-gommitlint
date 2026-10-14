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
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

func TestGitHubFormatter_FormatsValidationErrors(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(domain.CommitReport{
			Commit: domain.Commit{
				Hash:        "abc1234def5678",
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

	result := GitHub(report)

	check.Contains(t, result, "::group::Gommitlint Summary", "should contain summary group")
	check.Contains(t, result, "::endgroup::", "should close groups")
	check.Contains(t, result, "Validated 1 commit", "should show commit count")
	check.Contains(t, result, "Passed: 0, Failed: 1", "should show pass/fail counts")

	check.Contains(t, result, "::group::Commit #1: abc1234", "should contain commit group with short hash")
	check.Contains(t, result, testCommitSubjectFeatNew, "should show commit subject")

	check.Contains(t, result, "::error title=Test Rule::Test validation error [abc1234]",
		"should contain error annotation with rule name and short hash")
	check.Contains(t, result, "::error title=Test Rule::Another test error [abc1234]",
		"should contain error annotation for second error")

	check.Contains(t, result, "✗ 1 rule failed", "should show failed rule count")

	check.Contains(t, result, "::error title=Gommitlint::Validation failed",
		"should contain final error status")

	check.NotContains(t, result, "::set-output", "should use annotation commands instead of step-output commands")
}

func TestGitHubFormatter_FormatsPassingReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		at(formatterFixtureTime()).
		passingCommit(testCommitHashDEF5678, testCommitSubjectDocsReadme).
		build()

	result := GitHub(report)

	check.Contains(t, result, "Passed: 1, Failed: 0", "should show success counts")
	check.Contains(t, result, "✓ All rules passed", "should show success indicator")

	// Should not contain error annotations for rules
	check.NotContains(t, result, "::error title=Test Rule", "should not contain error annotations")

	check.Contains(t, result, "::notice title=Gommitlint::All commit validation rules passed",
		"should contain success notice")

	check.NotContains(t, result, "::set-output", "should use annotation commands instead of step-output commands")
}

func TestGitHubFormatter_ShowsPassedForEmptyReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().at(formatterFixtureTime()).build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Gommitlint Summary", "should contain summary even for empty report")
	check.Contains(t, result, "Validated 0 commits", "should show zero commits")
	check.Contains(t, result, "::notice title=Gommitlint::All commit validation rules passed",
		"should pass with no commits")
}

func TestGitHubFormatter_IncludesRepositoryResults(t *testing.T) {
	t.Parallel()

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
			Name:    testRepoRuleName,
			Status:  domain.StatusFailed,
			Message: "Repository rule failed",
			Errors: []domain.ValidationError{
				{
					Rule:       testRepoRuleName,
					Diagnostic: testDiagnostic("repo_error"),
					Message:    "Repository check failed",
				},
			},
		}).
		build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Repository Validation", "should contain repository group")
	check.Contains(t, result, "Repository failures: 2", "should show repository failures in summary")
	check.Contains(t, result, "::group::Configuration Warnings", "should show configuration warnings group")
	check.Contains(t, result, "::warning::warning one", "should output warning annotation with message")

	check.Contains(t, result, "::error title=Branch Rule::Branch validation failed",
		"should contain repository error annotation")
	check.Contains(t, result, "::error title=Repo Rule::Repository check failed",
		"should contain second repository error annotation")

	check.Contains(t, result, "::error title=Gommitlint::Validation failed",
		"should fail with repository errors")
	check.Contains(t, result, "repository rules failed",
		"should mention repository failures")
}

func TestGitHubFormatter_FormatsMultipleCommits(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectFeatFeature).
		failingCommit(testCommitHashDEF5678, "fix: broken test", testRuleTestRule,
			domain.ValidationError{Rule: testRuleTestRule, Message: "Test failed"}).
		build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Commit #1: abc1234", "should contain first commit")
	check.Contains(t, result, "::group::Commit #2: def5678", "should contain second commit")

	check.Contains(t, result, "✓ All rules passed", "should show success for passing commit")
	check.Contains(t, result, "✗ 1 rule failed", "should show failure for failing commit")

	check.Contains(t, result, "Passed: 1, Failed: 1", "should show mixed results")
}

func TestGitHubFormatter_SkipsCommitsWithEmptyHash(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit("", testCommitSubjectTest).
		passingCommit(testCommitHashABC1234, testCommitSubjectValid).
		build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Commit #2: abc1234", "should show valid commit")
	check.NotContains(t, result, "::group::Commit #1:", "should not show first commit group with empty hash")

	commitGroups := 0

	lines := strings.SplitSeq(result, "\n")
	for line := range lines {
		if strings.Contains(line, "::group::Commit #") {
			commitGroups++
		}
	}

	check.Equal(t, 1, commitGroups, "should only have one commit group")
}

func TestGitHubFormatter_GroupsByCommit(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectTestCommit).
		repoRule(domain.RuleReport{Name: testRepoRuleName, Status: domain.StatusPassed}).
		build()

	result := GitHub(report)

	groupCount := strings.Count(result, "::group::")
	endGroupCount := strings.Count(result, "::endgroup::")

	check.Equal(t, groupCount, endGroupCount, "all groups should be properly closed")
	check.GreaterOrEqual(t, groupCount, 2, "should have at least summary and commit groups")
}

func TestGitHubFormatter_EscapesSpecialCharacters(t *testing.T) {
	t.Parallel()

	report := specialCharactersReport()
	result := GitHub(report)

	check.Contains(t, result, "feat: add \"quotes\" and <brackets>",
		"should preserve special characters in subject")
	check.Contains(t, result, "Error with \"quotes\" and <brackets> & symbols",
		"should preserve special characters in error messages")

	check.Contains(t, result, "::group::", "should maintain valid GitHub Actions format")
	check.Contains(t, result, "::endgroup::", "should maintain valid GitHub Actions format")
}

func TestGitHubFormatter_EscapesWorkflowCommandInjection(t *testing.T) {
	t.Parallel()

	const (
		maliciousTitle   = "lint%,:\r\n::error::title-injected"
		maliciousSubject = "subject%\r\n::warning::subject-injected"
		maliciousMessage = "failure%\r\n::notice::message-injected"
		maliciousPath    = "bad%,:\r\n::error::file-injected.yml"
	)

	report := newReportBuilder().
		warn(domain.Warning{
			Code: domain.WarnConfigParse,
			Context: map[string]string{
				domain.WarningContextPath:   maliciousPath,
				domain.WarningContextSource: "project",
			},
		}).
		failingCommit(testCommitHashABC1234, maliciousSubject, "rule%,:",
			domain.ValidationError{Message: maliciousMessage}).
		build()

	result := GitHub(report, GitHubOptions{AnnotationTitle: maliciousTitle})

	check.Contains(t, result,
		"::group::Commit #1: abc1234 - subject%25  ::warning::subject-injected",
		"commit text line breaks become spaces and percent stays escaped")
	check.Contains(t, result,
		"::error title=rule%25%2C%3A::failure%25  ::notice::message-injected [abc1234]",
		"annotation properties and data should use their respective escaping rules")
	check.Contains(t, result,
		"::warning::Could not parse project config file bad%25,:%0D%0A::error::file-injected.yml, using defaults instead",
		"untrusted file names should remain within warning command data")
	check.Contains(t, result,
		"::error title=lint%25%2C%3A%0D%0A%3A%3Aerror%3A%3Atitle-injected::Validation failed",
		"annotation titles should escape every property delimiter")

	check.NotContains(t, result, "\r", "workflow commands must not contain raw carriage returns")
	check.NotContains(t, result, "\n::warning::subject-injected", "commit subjects must not inject commands")
	check.NotContains(t, result, "\n::notice::message-injected", "diagnostics must not inject commands")
	check.NotContains(t, result, "\n::error::file-injected.yml", "file names must not inject commands")
}

func TestGitHubFormatter_CommandEscapingRules(t *testing.T) {
	t.Parallel()

	const value = "%\r\n:,"

	check.Equal(t, "%25%0D%0A:,", escapeGitHubCommandData(value))
	check.Equal(t, "%25%0D%0A%3A%2C", escapeGitHubCommandProperty(value))
}

func TestGitHubFormatter_TruncatesLongHash(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit("abc1234567890abcdef1234567890abcdef1234", testCommitSubjectTestCommit).
		build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Commit #1: abc1234 -",
		"should truncate long hash to 7 characters")
	check.NotContains(t, result, "abc1234567890",
		"should not contain full hash in group header")
}

func TestGitHubFormatter_ShowsPassingRepoRules(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		repoRule(domain.RuleReport{Name: rules.BranchAheadRule{}.Name(), Status: domain.StatusPassed}).
		build()

	result := GitHub(report)

	check.Contains(t, result, "::group::Repository Validation", "should show repository section")
	check.Contains(t, result, "✓ Branch Ahead", "should show passing repo rule")
}

func TestGitHubFormatter_WithCustomAnnotationTitle(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectTestCommit).
		build()

	// Test with explicit options (used when GITHUB_JOB env var is set)
	opts := GitHubOptions{
		AnnotationTitle: "validate-commits",
	}

	result := GitHub(report, opts)

	check.Contains(t, result, "::group::validate-commits Summary", "should use provided title in summary group")
	check.Contains(t, result, "::notice title=validate-commits::All commit validation rules passed",
		"should use provided title in final notice")
}

func TestGitHubFormatter_WithCustomAnnotationTitleOnFailure(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, "bad commit", testRuleTestRule,
			domain.ValidationError{Message: testTestErrorShort}).
		build()

	opts := GitHubOptions{
		AnnotationTitle: "lint",
	}

	result := GitHub(report, opts)

	check.Contains(t, result, "::group::lint Summary", "should use provided title in summary group")
	check.Contains(t, result, "::error title=lint::Validation failed: 1 commit has violations",
		"should use provided title in final error")
}

func TestGitHubFormatter_ShowsSkippedRepoRules(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectTestCommit).
		summary(func(s *domain.ReportSummary) {
			s.SkippedRepoRules = 2
			s.IgnoreRepoRules = true
		}).
		build()

	result := GitHub(report)

	check.Contains(t, result, "Repository rules skipped: 2", "should show skipped repo rules in summary")
}
