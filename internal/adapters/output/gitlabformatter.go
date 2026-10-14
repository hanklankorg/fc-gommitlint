// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// ANSI color codes for terminal output.
const (
	ansiClear = "\033[0K" // Clear to end of line
)

// GitLabOptions configures GitLab CI log formatting.
type GitLabOptions struct {
	UseColor  bool
	Theme     string
	Localizer Localizer
}

// gitlabColors handles color formatting respecting NO_COLOR.
type gitlabColors struct {
	useColor bool
	scheme   ColorScheme
}

func newGitlabColors(useColor bool, theme string) gitlabColors {
	return gitlabColors{
		useColor: useColor,
		scheme:   GetColorScheme(useColor, theme),
	}
}

func (c gitlabColors) red(text string) string {
	return c.scheme.Error(text)
}

func (c gitlabColors) green(text string) string {
	return c.scheme.Success(text)
}

func (c gitlabColors) yellow(text string) string {
	return c.scheme.Warning(text)
}

// GitLab formats a domain report for GitLab CI job logs with collapsible sections.
// The useColor parameter controls whether ANSI color codes are included.
// When false (e.g., when NO_COLOR is set), colors are omitted.
//
// Uses GitLab CI section markers:
// - section_start:TIMESTAMP:NAME[collapsed=true]\r\e[0K for section headers
// - section_end:TIMESTAMP:NAME\r\e[0K for section endings
//
// The \e[0K escape sequence clears the line to hide section markers in rendered output.
// The \r carriage return returns cursor to start of line.
//
// Reference: https://docs.gitlab.com/ci/jobs/job_logs.html#custom-collapsible-sections
func GitLab(report domain.Report, opts ...GitLabOptions) string {
	var options GitLabOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	var builder strings.Builder

	localizer := ensureLocalizer(options.Localizer)
	timestamp := time.Now().Unix()
	colors := newGitlabColors(options.UseColor, options.Theme)

	writeGitLabSummary(&builder, report, timestamp, colors, localizer)
	writeGitLabWarnings(&builder, report.Metadata.WarningDetails, timestamp, colors, localizer)
	writeGitLabCommits(&builder, report, timestamp, colors, localizer)
	writeGitLabRepository(&builder, report, timestamp, colors, localizer)
	writeGitLabFinalStatus(&builder, report, colors, localizer)

	return builder.String()
}

func writeGitLabSummary(builder *strings.Builder, report domain.Report, timestamp int64, colors gitlabColors, localizer Localizer) {
	writeSectionStart(builder, timestamp, "gommitlint_summary", localizer.Get("Gommitlint Summary"), true, colors)

	for _, line := range reportSummaryLines(report, localizer) {
		builder.WriteString(line + "\n")
	}

	writeSectionEnd(builder, timestamp, "gommitlint_summary", colors)
}

func writeGitLabWarnings(builder *strings.Builder, warnings []domain.Warning, timestamp int64, colors gitlabColors, localizer Localizer) {
	if len(warnings) == 0 {
		return
	}

	writeSectionStart(builder, timestamp, "config_warnings", localizer.Get("Configuration Warnings"), true, colors)

	for _, warning := range FormatWarnings(warnings, localizer) {
		builder.WriteString(localizer.Get("%s %s\n", colors.yellow(localizer.Get("WARNING:")), warning))
	}

	writeSectionEnd(builder, timestamp, "config_warnings", colors)
}

func writeGitLabCommits(builder *strings.Builder, report domain.Report, timestamp int64, colors gitlabColors, localizer Localizer) {
	for _, commitView := range collectCommitReportViews(report, localizer) {
		sectionName := fmt.Sprintf("commit_%d", commitView.index)
		sectionTitle := localizer.Get("Commit #%d: %s - %s", commitView.index, commitView.shortHash, commitView.subject)

		writeSectionStart(builder, timestamp, sectionName, sectionTitle, commitView.passed, colors)

		writeGitLabRules(builder, commitView, colors, localizer)
		writeSectionEnd(builder, timestamp, sectionName, colors)
	}
}

func writeGitLabRepository(builder *strings.Builder, report domain.Report, timestamp int64, colors gitlabColors, localizer Localizer) {
	if len(report.Repository.RuleResults) == 0 {
		return
	}

	writeSectionStart(builder, timestamp, "repository", localizer.Get("Repository Validation"), true, colors)

	for _, repoResult := range collectRuleMessages(report.Repository.RuleResults, localizer) {
		if repoResult.status == domain.StatusFailed {
			for _, message := range repoResult.messages {
				builder.WriteString(localizer.Get("%s %s - %s\n",
					colors.red(localizer.Get("ERROR:")), repoResult.displayName, message))
			}
		} else {
			builder.WriteString(localizer.Get("%s %s\n", colors.green("✓"), repoResult.displayName))
		}
	}

	writeSectionEnd(builder, timestamp, "repository", colors)
}

func writeGitLabFinalStatus(builder *strings.Builder, report domain.Report, colors gitlabColors, localizer Localizer) {
	builder.WriteString("\n")

	if report.Summary.AllPassed {
		builder.WriteString(localizer.Get("%s\n", colors.green(localizer.Get("✓ All commit validation rules passed"))))

		return
	}

	builder.WriteString(localizer.Get("%s\n", colors.red("✗ "+validationFailedMessage(report, localizer))))
}

// writeSectionStart writes a GitLab CI collapsible section start marker.
//
// The \e[0K (clear-to-EOL) escape is required by GitLab's CI log spec to make
// sections fold — it's a structural marker, not a colouring one — so it must
// be emitted regardless of useColor. Previously this was gated through
// clearLine(), which returned "" when colours were off, breaking collapsible
// sections in NO_COLOR or non-TTY environments where GitLab Runner itself
// still parses the markers.
//
// Spec: https://docs.gitlab.com/ci/jobs/job_logs/#custom-collapsible-sections
func writeSectionStart(builder *strings.Builder, timestamp int64, name, title string, collapsed bool, _ gitlabColors) {
	collapsedStr := ""
	if collapsed {
		collapsedStr = "[collapsed=true]"
	}

	// Format: \e[0Ksection_start:TIMESTAMP:NAME[collapsed=true]\r\e[0KTITLE
	fmt.Fprintf(builder, "%ssection_start:%d:%s%s\r%s%s\n",
		ansiClear, timestamp, name, collapsedStr, ansiClear, title)
}

// writeSectionEnd writes a GitLab CI collapsible section end marker.
//
// See writeSectionStart for why ansiClear is unconditional here.
func writeSectionEnd(builder *strings.Builder, timestamp int64, name string, _ gitlabColors) {
	// Format: \e[0Ksection_end:TIMESTAMP:NAME\r\e[0K
	fmt.Fprintf(builder, "%ssection_end:%d:%s\r%s\n", ansiClear, timestamp, name, ansiClear)
}

func writeGitLabRules(builder *strings.Builder, commitView commitReportView, colors gitlabColors, localizer Localizer) {
	for _, failure := range commitView.failures {
		for _, message := range failure.messages {
			builder.WriteString(localizer.Get("%s %s - %s: %s\n",
				colors.red(localizer.Get("ERROR:")), commitView.shortHash, failure.displayName, message))
		}
	}

	if commitView.passed {
		builder.WriteString(localizer.Get("%s All rules passed\n", colors.green("✓")))
	} else {
		summary := localizer.GetN("%s %d rule failed\n", "%s %d rules failed\n", len(commitView.failures), colors.red("✗"), len(commitView.failures))
		builder.WriteString(summary)
	}
}
