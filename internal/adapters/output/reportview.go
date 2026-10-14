// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import "codefloe.com/itiquette/gommitlint/internal/domain"

type ruleFailureMessages struct {
	displayName string
	messages    []string
}

type ruleMessages struct {
	displayName string
	status      domain.ValidationStatus
	messages    []string
}

type ruleCounts struct {
	total  int
	passed int
	failed int
}

type reportSummaryView struct {
	validatedCommits    string
	ignoredCommits      string
	ignoredLabel        string
	warningCount        string
	skippedRepoRules    string
	ignoredCount        int
	passedCount         int
	failedCount         int
	repoFailedRuleCount int
	skippedRepoCount    int
}

type commitReportView struct {
	index     int
	shortHash string
	subject   string
	passed    bool
	failures  []ruleFailureMessages
}

func buildReportSummaryView(report domain.Report, localizer Localizer) reportSummaryView {
	localizer = ensureLocalizer(localizer)

	return reportSummaryView{
		validatedCommits:    localizer.GetN("%d commit", "%d commits", report.Summary.ValidatedCommits, report.Summary.ValidatedCommits),
		ignoredCommits:      localizer.GetN("%d commit", "%d commits", report.Summary.IgnoredCommits, report.Summary.IgnoredCommits),
		ignoredLabel:        localizer.GetN("%d ignored", "%d ignored", report.Summary.IgnoredCommits, report.Summary.IgnoredCommits),
		warningCount:        localizer.GetN("%d warning", "%d warnings", len(report.Metadata.WarningDetails), len(report.Metadata.WarningDetails)),
		skippedRepoRules:    localizer.GetN("%d repository rule skipped", "%d repository rules skipped", report.Summary.SkippedRepoRules, report.Summary.SkippedRepoRules),
		ignoredCount:        report.Summary.IgnoredCommits,
		passedCount:         report.Summary.PassedCommits,
		failedCount:         report.Summary.FailedCommits,
		repoFailedRuleCount: report.Summary.RepoFailedRules,
		skippedRepoCount:    report.Summary.SkippedRepoRules,
	}
}

func reportSummaryLines(report domain.Report, localizer Localizer) []string {
	localizer = ensureLocalizer(localizer)
	view := buildReportSummaryView(report, localizer)
	lines := []string{
		localizer.Get("Validated %s", view.validatedCommits),
	}

	if view.ignoredCount > 0 {
		lines = append(lines, localizer.Get("Ignored: %s", view.ignoredCommits))
	}

	lines = append(lines, localizer.Get("Passed: %d, Failed: %d", view.passedCount, view.failedCount))

	if view.repoFailedRuleCount > 0 {
		lines = append(lines, localizer.Get("Repository failures: %d", view.repoFailedRuleCount))
	}

	if view.skippedRepoCount > 0 {
		lines = append(lines, localizer.Get("Repository rules skipped: %d", view.skippedRepoCount))
	}

	return lines
}

func validationFailedMessage(report domain.Report, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)
	message := localizer.GetN("Validation failed: %d commit has violations", "Validation failed: %d commits have violations",
		report.Summary.FailedCommits, report.Summary.FailedCommits)

	if report.Summary.RepoFailedRules > 0 {
		message = localizer.GetN("%s, %d repository rule failed", "%s, %d repository rules failed",
			report.Summary.RepoFailedRules, message, report.Summary.RepoFailedRules)
	}

	return message
}

func collectCommitReportViews(report domain.Report, localizer Localizer) []commitReportView {
	localizer = ensureLocalizer(localizer)
	views := make([]commitReportView, 0, len(report.Commits))

	for index, commitReport := range report.Commits {
		if commitReport.Commit.Hash == "" {
			continue
		}

		views = append(views, commitReportView{
			index:     index + 1,
			shortHash: truncateSHA(commitReport.Commit.Hash),
			subject:   cleanForTerminal(commitReport.Commit.Subject),
			passed:    commitReport.Passed,
			failures:  collectRuleFailures(commitReport.RuleResults, localizer),
		})
	}

	return views
}

func collectRuleFailures(ruleResults []domain.RuleReport, localizer Localizer) []ruleFailureMessages {
	localizer = ensureLocalizer(localizer)
	failures := make([]ruleFailureMessages, 0, len(ruleResults))

	for _, ruleReport := range ruleResults {
		if ruleReport.Status != domain.StatusFailed {
			continue
		}

		failure := ruleFailureMessages{
			displayName: DisplayName(ruleReport.Name, localizer),
			messages:    make([]string, 0, len(ruleReport.Errors)),
		}

		for _, ruleErr := range ruleReport.Errors {
			failure.messages = append(failure.messages, diagnosticMessage(ruleErr, localizer))
		}

		failures = append(failures, failure)
	}

	return failures
}

func collectRuleMessages(ruleResults []domain.RuleReport, localizer Localizer) []ruleMessages {
	localizer = ensureLocalizer(localizer)
	messages := make([]ruleMessages, 0, len(ruleResults))

	for _, ruleReport := range ruleResults {
		ruleMessage := ruleMessages{
			displayName: DisplayName(ruleReport.Name, localizer),
			status:      ruleReport.Status,
			messages:    make([]string, 0, len(ruleReport.Errors)),
		}

		for _, ruleErr := range ruleReport.Errors {
			ruleMessage.messages = append(ruleMessage.messages, diagnosticMessage(ruleErr, localizer))
		}

		messages = append(messages, ruleMessage)
	}

	return messages
}

func countReportRules(report domain.Report) ruleCounts {
	counts := ruleCounts{}

	for _, commit := range report.Commits {
		counts.add(commit.RuleResults)
	}

	counts.add(report.Repository.RuleResults)

	return counts
}

func (c *ruleCounts) add(ruleResults []domain.RuleReport) {
	for _, rule := range ruleResults {
		c.total++

		switch rule.Status {
		case domain.StatusPassed:
			c.passed++
		case domain.StatusFailed:
			c.failed++
		case domain.StatusSkipped:
		}
	}
}

func collectReportRuleNames(report domain.Report) []string {
	ruleNames := make([]string, 0, reportRuleCapacity(report))

	if len(report.Commits) > 0 {
		for _, ruleResult := range report.Commits[0].RuleResults {
			ruleNames = append(ruleNames, ruleResult.Name)
		}
	}

	for _, ruleResult := range report.Repository.RuleResults {
		ruleNames = append(ruleNames, ruleResult.Name)
	}

	return ruleNames
}

func reportRuleCapacity(report domain.Report) int {
	capacity := len(report.Repository.RuleResults)
	if len(report.Commits) > 0 {
		capacity += len(report.Commits[0].RuleResults)
	}

	return capacity
}
