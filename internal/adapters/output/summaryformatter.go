// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// SummaryOptions configures job summary output formatting.
type SummaryOptions struct {
	// Title is the custom title for the summary (default: "Gommitlint")
	Title string
	// Localizer provides translations for summary output.
	Localizer Localizer
	// ConfigSources are display-ready configuration sources prepared by the CLI
	// boundary. Empty falls back to the raw paths in the report metadata, so
	// callers that build a summary straight from a report still get provenance.
	//
	// Unlike the text report this is always rendered: a CI job summary is an
	// artifact someone reads once, cold, not terminal scrollback.
	ConfigSources []string
}

// configSources returns the display sources, falling back to the report's raw
// paths when the caller supplied none.
func (o SummaryOptions) configSources(report domain.Report) []string {
	if len(o.ConfigSources) > 0 {
		return o.ConfigSources
	}

	return report.Metadata.ConfigSources
}

// getTitle returns the title with fallback to default.
func (o SummaryOptions) getTitle() string {
	if o.Title != "" {
		return o.Title
	}

	return "Gommitlint"
}

// JobSummary generates a markdown summary suitable for GitHub Actions Job Summary.
// This can be written to $GITHUB_STEP_SUMMARY for display in the workflow summary page.
//
// Reference: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/adding-a-job-summary
func JobSummary(report domain.Report, opts ...SummaryOptions) string {
	var options SummaryOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	var builder strings.Builder

	localizer := ensureLocalizer(options.Localizer)
	title := options.getTitle()
	summaryView := buildReportSummaryView(report, localizer)

	// Header with status
	switch {
	case report.Summary.AllPassed:
		builder.WriteString(localizer.Get("## ✅ %s: All Checks Passed\n\n", title))
	case report.Summary.RepoFailedRules > 0:
		builder.WriteString(localizer.Get("## ❌ %s: Validation Failed\n\n", title))
	default:
		builder.WriteString(localizer.GetN("## ❌ %s: %d Commit Failed\n\n", "## ❌ %s: %d Commits Failed\n\n",
			report.Summary.FailedCommits, title, report.Summary.FailedCommits))
	}

	// Summary statistics
	summaryLine := localizer.Get("**Validated:** %s | **Ignored:** %d | **Passed:** %d | **Failed:** %d",
		summaryView.validatedCommits, summaryView.ignoredCount,
		summaryView.passedCount, summaryView.failedCount)
	if summaryView.repoFailedRuleCount > 0 {
		summaryLine = localizer.Get("%s | **Repo Failed:** %d", summaryLine, summaryView.repoFailedRuleCount)
	}

	builder.WriteString(summaryLine + "\n\n")

	if len(report.Metadata.WarningDetails) > 0 {
		builder.WriteString(localizer.Get("**Config Warnings:**") + "\n")

		for _, warning := range FormatWarnings(report.Metadata.WarningDetails, localizer) {
			fmt.Fprintf(&builder, "- %s\n", warning)
		}

		builder.WriteString("\n")
	}

	if sources := options.configSources(report); len(sources) > 0 {
		builder.WriteString(localizer.Get("**Config Sources:**") + "\n")

		for _, source := range sources {
			fmt.Fprintf(&builder, "- %s\n", source)
		}

		builder.WriteString("\n")
	}

	// Write commit table
	writeCommitTable(&builder, report.Commits, localizer)

	// Write details for failed commits
	if !report.Summary.AllPassed {
		writeFailedCommitDetails(&builder, report.Commits, localizer)
	}

	// Write repository-level failures
	writeRepositoryFailures(&builder, report.Repository.RuleResults, localizer)

	return builder.String()
}

// writeCommitTable writes the commit summary table to the builder.
func writeCommitTable(builder *strings.Builder, commits []domain.CommitReport, localizer Localizer) {
	if len(commits) == 0 {
		return
	}

	localizer = ensureLocalizer(localizer)

	builder.WriteString(localizer.Get("### Commits") + "\n\n")
	builder.WriteString(localizer.Get("| Commit | Subject | Status |") + "\n")
	builder.WriteString(localizer.Get("|--------|---------|--------|") + "\n")

	for _, commitReport := range commits {
		if commitReport.Commit.Hash == "" {
			continue
		}

		status := localizer.Get("✅ Passed")
		if !commitReport.Passed {
			status = localizer.Get("❌ Failed")
		}

		// Truncate subject for table readability. Runes, not bytes: cutting a
		// multibyte character in half would put invalid UTF-8 in the summary.
		subject := cleanForTerminal(commitReport.Commit.Subject)
		if utf8.RuneCountInString(subject) > 50 {
			subject = truncateToRunes(subject, 47) + "..."
		}

		subject = escapeMarkdown(subject)

		fmt.Fprintf(builder, "| `%s` | %s | %s |\n",
			truncateSHA(commitReport.Commit.Hash), subject, status)
	}

	builder.WriteString("\n")
}

// writeFailedCommitDetails writes detailed error information for failed commits.
func writeFailedCommitDetails(builder *strings.Builder, commits []domain.CommitReport, localizer Localizer) {
	localizer = ensureLocalizer(localizer)
	builder.WriteString(localizer.Get("### Failed Commits Details") + "\n\n")

	for _, commitReport := range commits {
		if commitReport.Passed || commitReport.Commit.Hash == "" {
			continue
		}

		builder.WriteString(localizer.Get("#### `%s` - %s\n\n",
			truncateSHA(commitReport.Commit.Hash), escapeMarkdown(cleanForTerminal(commitReport.Commit.Subject))))

		for _, failure := range collectRuleFailures(commitReport.RuleResults, localizer) {
			for _, message := range failure.messages {
				builder.WriteString(localizer.Get("- **%s**: %s\n", failure.displayName, escapeMarkdown(message)))
			}
		}

		builder.WriteString("\n")
	}
}

// writeRepositoryFailures writes repository-level validation failures.
func writeRepositoryFailures(builder *strings.Builder, ruleResults []domain.RuleReport, localizer Localizer) {
	failures := collectRuleFailures(ruleResults, localizer)
	if len(failures) == 0 {
		return
	}

	localizer = ensureLocalizer(localizer)
	builder.WriteString(localizer.Get("### Repository Validation") + "\n\n")

	for _, failure := range failures {
		for _, message := range failure.messages {
			builder.WriteString(localizer.Get("- **%s**: %s\n", failure.displayName, escapeMarkdown(message)))
		}
	}

	builder.WriteString("\n")
}

// escapeMarkdown escapes special markdown characters in text.
func escapeMarkdown(text string) string {
	// Escape pipe characters which would break tables
	text = strings.ReplaceAll(text, "|", "\\|")
	// Escape backticks
	text = strings.ReplaceAll(text, "`", "\\`")

	return text
}
