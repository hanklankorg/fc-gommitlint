// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// GitHubOptions configures GitHub Actions output formatting.
type GitHubOptions struct {
	// AnnotationTitle is the custom title for annotations (default: "Gommitlint")
	AnnotationTitle string
	// Localizer provides translations for output text.
	Localizer Localizer
}

// getAnnotationTitle returns the annotation title with fallback to default.
func (o GitHubOptions) getAnnotationTitle() string {
	if o.AnnotationTitle != "" {
		return o.AnnotationTitle
	}

	return "Gommitlint"
}

// GitHub formats a domain report for GitHub Actions workflow commands.
// This format is also compatible with Forgejo Actions and Gitea Actions.
//
// Uses GitHub Actions workflow commands:
// - ::group:: / ::endgroup:: for collapsible sections
// - ::error:: for error annotations
// - ::warning:: for warning annotations
// - ::notice:: for notice annotations
//
// Reference: https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/workflow-commands-for-github-actions
func GitHub(report domain.Report, opts ...GitHubOptions) string {
	var options GitHubOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	var builder strings.Builder

	localizer := ensureLocalizer(options.Localizer)
	title := options.getAnnotationTitle()

	writeGitHubSummary(&builder, report, title, localizer)
	writeGitHubWarnings(&builder, report.Metadata.WarningDetails, localizer)
	writeGitHubCommits(&builder, report, localizer)
	writeGitHubRepository(&builder, report, localizer)
	writeGitHubFinalStatus(&builder, report, title, localizer)

	return builder.String()
}

func writeGitHubSummary(builder *strings.Builder, report domain.Report, title string, localizer Localizer) {
	builder.WriteString(localizer.Get("::group::%s Summary\n", escapeGitHubCommandData(title)))

	for _, line := range reportSummaryLines(report, localizer) {
		builder.WriteString(line + "\n")
	}

	builder.WriteString("::endgroup::\n")
}

func writeGitHubWarnings(builder *strings.Builder, warnings []domain.Warning, localizer Localizer) {
	if len(warnings) == 0 {
		return
	}

	builder.WriteString(localizer.Get("::group::Configuration Warnings\n"))

	for _, warning := range FormatWarnings(warnings, localizer) {
		builder.WriteString(localizer.Get("::warning::%s\n", escapeGitHubCommandData(warning)))
	}

	builder.WriteString("::endgroup::\n")
}

func writeGitHubCommits(builder *strings.Builder, report domain.Report, localizer Localizer) {
	for _, commitView := range collectCommitReportViews(report, localizer) {
		builder.WriteString(localizer.Get("::group::Commit #%d: %s - %s\n",
			commitView.index,
			escapeGitHubCommandData(commitView.shortHash),
			escapeGitHubCommandData(commitView.subject),
		))

		writeGitHubRules(builder, commitView, localizer)
		builder.WriteString("::endgroup::\n")
	}
}

func writeGitHubRepository(builder *strings.Builder, report domain.Report, localizer Localizer) {
	if len(report.Repository.RuleResults) == 0 {
		return
	}

	builder.WriteString(localizer.Get("::group::Repository Validation\n"))

	for _, repoResult := range collectRuleMessages(report.Repository.RuleResults, localizer) {
		if repoResult.status == domain.StatusFailed {
			for _, message := range repoResult.messages {
				builder.WriteString(localizer.Get("::error title=%s::%s\n",
					escapeGitHubCommandProperty(repoResult.displayName), escapeGitHubCommandData(message)))
			}
		} else {
			builder.WriteString(localizer.Get("✓ %s\n", repoResult.displayName))
		}
	}

	builder.WriteString("::endgroup::\n")
}

func writeGitHubFinalStatus(builder *strings.Builder, report domain.Report, title string, localizer Localizer) {
	if report.Summary.AllPassed {
		builder.WriteString(localizer.Get("::notice title=%s::All commit validation rules passed\n",
			escapeGitHubCommandProperty(title)))

		return
	}

	builder.WriteString(localizer.Get("::error title=%s::%s\n",
		escapeGitHubCommandProperty(title), escapeGitHubCommandData(validationFailedMessage(report, localizer))))
}

func writeGitHubRules(builder *strings.Builder, commitView commitReportView, localizer Localizer) {
	for _, failure := range commitView.failures {
		for _, message := range failure.messages {
			builder.WriteString(localizer.Get("::error title=%s::%s [%s]\n",
				escapeGitHubCommandProperty(failure.displayName),
				escapeGitHubCommandData(message),
				escapeGitHubCommandData(commitView.shortHash),
			))
		}
	}

	if commitView.passed {
		builder.WriteString(localizer.Get("✓ All rules passed\n"))
	} else {
		builder.WriteString(localizer.GetN("✗ %d rule failed\n", "✗ %d rules failed\n", len(commitView.failures), len(commitView.failures)))
	}
}

// GitHub workflow commands use different escaping rules for command data and
// property values. Percent must be escaped first so the escapes added for the
// remaining delimiters are not encoded a second time.
func escapeGitHubCommandData(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	value = strings.ReplaceAll(value, "\r", "%0D")

	return strings.ReplaceAll(value, "\n", "%0A")
}

func escapeGitHubCommandProperty(value string) string {
	value = escapeGitHubCommandData(value)
	value = strings.ReplaceAll(value, ":", "%3A")

	return strings.ReplaceAll(value, ",", "%2C")
}
