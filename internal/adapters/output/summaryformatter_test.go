// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"
	"testing"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

func TestJobSummary_PassingReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectFeatFeature).
		passingCommit(testCommitHashDEF5678, "fix: bug fix").
		build()

	result := JobSummary(report)

	check.Contains(t, result, "## ✅ Gommitlint: All Checks Passed", "should show success header")
	check.Contains(t, result, "**Validated:** 2 commits", "should show commit count")
	check.Contains(t, result, "**Ignored:** 0", "should show ignored count")
	check.Contains(t, result, "**Passed:** 2", "should show passed count")
	check.Contains(t, result, "**Failed:** 0", "should show failed count")
	check.Contains(t, result, "| `abc1234` |", "should contain first commit hash")
	check.Contains(t, result, "| `def5678` |", "should contain second commit hash")
	check.Contains(t, result, "✅ Passed", "should show passed status")
	check.NotContains(t, result, "### Failed Commits Details", "should not show failure details")
}

func TestJobSummary_FailingReport(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, "feat: good commit").
		failingCommit(testCommitHashDEF5678, "bad commit message", rules.ConventionalCommitRule{}.Name(),
			domain.ValidationError{Message: "Must follow conventional commit format"}).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "## ❌ Gommitlint: 1 Commit Failed", "should show failure header")
	check.Contains(t, result, "### Failed Commits Details", "should show failure details section")
	check.Contains(t, result, "#### `def5678` - bad commit message", "should show failed commit details")
	check.Contains(t, result, "**Commit type**: Must follow conventional commit format",
		"should show rule error")
}

func TestJobSummary_RepositoryFailureHeader(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectFeatOK).
		repoRule(domain.RuleReport{
			Name:   rules.BranchAheadRule{}.Name(),
			Status: domain.StatusFailed,
			Errors: []domain.ValidationError{{Message: "Too many commits"}},
		}).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "## ❌ Gommitlint: Validation Failed", "should show repo failure header")
}

func TestJobSummary_ShowsRepositoryFailureCount(t *testing.T) {
	t.Parallel()

	// RepoFailedRules is pinned without adding repository rules to check the
	// summary count text alone.
	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, testCommitSubjectFeatOK).
		summary(func(s *domain.ReportSummary) {
			s.RepoFailedRules = 2
			s.AllPassed = false
		}).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "**Repo Failed:** 2", "should show repository failures")
}

func TestJobSummary_CustomTitle(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().build()

	opts := SummaryOptions{
		Title: "PR Validation",
	}

	result := JobSummary(report, opts)

	check.Contains(t, result, "## ✅ PR Validation: All Checks Passed", "should use custom title")
	check.NotContains(t, result, "Gommitlint", "should not contain default title")
}

// A CI job summary is an artifact read once, cold, so provenance is listed on
// every run — but it uses the same shortened rendering as the text report.
func TestJobSummary_PrefersPreparedConfigSources(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		configSources("defaults", "/home/dev/project/.gommitlint.yaml").
		build()

	result := JobSummary(report, SummaryOptions{ConfigSources: []string{".gommitlint.yaml"}})

	check.Contains(t, result, "- .gommitlint.yaml")
	check.NotContains(t, result, "/home/dev/project/.gommitlint.yaml")
}

// Callers that build a summary straight from a report — with no CLI boundary to
// prepare display strings — still get provenance from the raw metadata.
func TestJobSummary_FallsBackToReportConfigSources(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		configSources("defaults", "/home/dev/project/.gommitlint.yaml").
		build()

	result := JobSummary(report, SummaryOptions{})

	check.Contains(t, result, "- /home/dev/project/.gommitlint.yaml")
}

func TestJobSummary_TruncatesLongSubject(t *testing.T) {
	t.Parallel()

	longSubject := "feat: this is a very long commit subject that should be truncated in the table display for better readability"

	report := newReportBuilder().
		passingCommit(testCommitHashABC1234, longSubject).
		build()

	result := JobSummary(report)

	check.NotContains(t, result, longSubject, "should not contain full long subject")
	check.Contains(t, result, "...", "should contain truncation indicator")
}

func TestJobSummary_EscapesMarkdownCharacters(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, "feat: add | pipe character", testRuleTestRule,
			domain.ValidationError{Message: "Error with | pipe and `backticks`"}).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "\\|", "should escape pipe characters")
	check.Contains(t, result, "\\`", "should escape backticks")
}

func TestJobSummary_SkipsEmptyHashCommits(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		passingCommit("", "empty hash commit").
		passingCommit(testCommitHashABC1234, testCommitSubjectValid).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "| `abc1234` |", "should contain valid commit")
	check.NotContains(t, result, "empty hash commit", "should not contain empty hash commit")
}

func TestJobSummary_IncludesRepositoryFailures(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		repoRule(domain.RuleReport{
			Name:   rules.BranchAheadRule{}.Name(),
			Status: domain.StatusFailed,
			Errors: []domain.ValidationError{{Message: "Branch is 5 commits ahead of main"}},
		}).
		build()

	result := JobSummary(report)

	check.Contains(t, result, "### Repository Validation", "should show repository section")
	check.Contains(t, result, "**Branch Ahead**: Branch is 5 commits ahead of main",
		"should show repository error")
}

func TestJobSummary_OmitsRepositorySectionWhenAllPass(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		repoRule(domain.RuleReport{Name: rules.BranchAheadRule{}.Name(), Status: domain.StatusPassed}).
		build()

	result := JobSummary(report)

	check.NotContains(t, result, "### Repository Validation",
		"should not show repository section when all pass")
}

func TestJobSummary_WithWarnings(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		warn(
			domain.Warning{Code: "invalid_ignore_commit", Message: "Invalid SHA in ignore_commits: bad", Context: map[string]string{"sha": "bad"}},
			domain.Warning{Code: "unknown_config", Message: "Unknown config key: foo"},
		).
		configSources("defaults", ".gommitlint.yaml").
		build()

	result := JobSummary(report)

	check.Contains(t, result, "**Config Warnings:**", "should show warnings section")
	check.Contains(t, result, "- 'bad' is not a valid commit SHA in ignore_commits, skipping it", "should show first warning")
	check.Contains(t, result, "- Unknown config key: foo", "should show second warning")
	check.Contains(t, result, "**Config Sources:**", "should show config sources section")
	check.Contains(t, result, "- defaults", "should show defaults source")
	check.Contains(t, result, "- .gommitlint.yaml", "should show project source")
}

// The table truncates long subjects by character, never mid-rune: a subject
// whose 47th byte sits inside a multibyte character must not yield invalid UTF-8.
func TestJobSummary_TruncatesSubjectByRune(t *testing.T) {
	t.Parallel()

	subject := strings.Repeat("a", 46) + "ä" + strings.Repeat("b", 10)
	report := newReportBuilder().passingCommit(testCommitHashABC1234, subject).build()

	result := JobSummary(report)

	check.True(t, utf8.ValidString(result), "summary must be valid UTF-8")
	check.Contains(t, result, strings.Repeat("a", 46)+"ä... |", "should keep the whole character before the ellipsis")
}
