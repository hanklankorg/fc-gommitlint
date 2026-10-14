// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// TableOptions contains the configuration for table formatting.
// The table formatter only uses color and localisation; the full TextOptions
// set (verbose levels, help flags, rule-help name, etc.) is irrelevant here.
type TableOptions struct {
	Colors    ColorScheme
	Localizer Localizer
}

// tableColumns holds the column widths for the table formatter.
type tableColumns struct {
	typeWidth    int
	targetWidth  int
	ruleWidth    int
	statusWidth  int
	detailsWidth int
}

// Table creates a perfectly aligned table where all borders match exactly.
func Table(report domain.Report, options TableOptions) string {
	var builder strings.Builder

	colors := options.Colors
	localizer := ensureLocalizer(options.Localizer)

	// Define column widths
	cols := tableColumns{
		typeWidth:    ColTypeWidth,
		targetWidth:  ColTargetWidth,
		ruleWidth:    ColRuleWidth,
		statusWidth:  ColStatusWidth,
		detailsWidth: ColDetailsWidth,
	}

	// Build borders with exact widths
	topBorder := "┌" + strings.Repeat(TableBorderChar, TableInternalWidth) + "┐"
	simpleLine := "├" + strings.Repeat(TableBorderChar, TableInternalWidth) + "┤"
	bottomBorder := "└" + strings.Repeat(TableBorderChar, TableInternalWidth) + "┘"

	// Column widths, separators, and padding must total exactly TableInternalWidth.
	headerStart := "├" +
		strings.Repeat("─", ColTypeWidth+2) + "┬" +
		strings.Repeat("─", ColTargetWidth+2) + "┬" +
		strings.Repeat("─", ColRuleWidth+2) + "┬" +
		strings.Repeat("─", ColStatusWidth+2) + "┬" +
		strings.Repeat("─", ColDetailsWidth+2) + "┤"
	// Total: 1 + 10 + 1 + 16 + 1 + 20 + 1 + 8 + 1 + 40 + 1 = 100

	// Data line - column intersections (must also total 100)
	dataLine := "├" +
		strings.Repeat("─", ColTypeWidth+2) + "┼" +
		strings.Repeat("─", ColTargetWidth+2) + "┼" +
		strings.Repeat("─", ColRuleWidth+2) + "┼" +
		strings.Repeat("─", ColStatusWidth+2) + "┼" +
		strings.Repeat("─", ColDetailsWidth+2) + "┤"
	// Total: 1 + 10 + 1 + 16 + 1 + 20 + 1 + 8 + 1 + 40 + 1 = 100

	// Helper to pad string to exact width (rune-aware)
	pad := func(s string, width int) string { //nolint:varnamelen // s is conventional for string parameter
		runes := []rune(s)
		runeLen := len(runes)

		if runeLen > width {
			if width > 3 {
				return string(runes[:width-3]) + "..."
			}

			return string(runes[:width])
		}

		return s + strings.Repeat(" ", width-runeLen)
	}

	totalCommits := len(report.Commits)
	ruleCounts := countReportRules(report)

	// Start building the table
	builder.WriteString(colors.Info(topBorder) + "\n")

	// Title (centered). Computed from the aggregate, not pre-rendered — a "PASSED"
	// header on a 5-failure run is the worst kind of UX bug.
	var (
		title      string
		titleColor = colors.Success
	)
	if ruleCounts.failed == 0 {
		title = localizer.Get("✓ VALIDATION PASSED")
	} else {
		title = localizer.Get("✗ VALIDATION FAILED")
		titleColor = colors.Error
	}

	titlePadding := (TableInternalWidth - utf8.RuneCountInString(title)) / 2
	titleLine := strings.Repeat(" ", titlePadding) + title
	builder.WriteString(colors.Info("│") + titleColor(pad(titleLine, TableInternalWidth)) + colors.Info("│") + "\n")
	builder.WriteString(colors.Info(simpleLine) + "\n")

	// Commit info section
	if len(report.Commits) == 1 {
		commit := report.Commits[0].Commit
		sha := truncateSHA(commit.Hash)

		builder.WriteString(colors.Info("│") + " " + pad(localizer.Get("SHA: %s", sha), 96) + " " + colors.Info("│") + "\n")
		builder.WriteString(colors.Info("│") + " " + pad(localizer.Get("Subject: %s", cleanForTerminal(commit.Subject)), 96) + " " + colors.Info("│") + "\n")
	} else {
		builder.WriteString(colors.Info("│") + " " + pad(localizer.Get("Commits: %d", totalCommits), 96) + " " + colors.Info("│") + "\n")
	}

	// Transition to table header
	builder.WriteString(colors.Info(headerStart) + "\n")

	// Table header row
	builder.WriteString(colors.Info("│") + " ")
	builder.WriteString(pad(localizer.Get("TYPE"), ColTypeWidth) + " " + colors.Info("│") + " ")
	builder.WriteString(pad(localizer.Get("TARGET"), ColTargetWidth) + " " + colors.Info("│") + " ")
	builder.WriteString(pad(localizer.Get("RULE"), ColRuleWidth) + " " + colors.Info("│") + " ")
	builder.WriteString(pad(localizer.Get("STATUS"), ColStatusWidth) + " " + colors.Info("│") + " ")
	builder.WriteString(pad(localizer.Get("DETAILS"), ColDetailsWidth) + " " + colors.Info("│") + "\n")
	builder.WriteString(colors.Info(dataLine) + "\n")

	// Process commit rules.
	//
	// Previously this branch only fired for single-commit runs, so multi-commit
	// validations (--range, --count > 1) showed an empty table between the
	// header and the summary even when commits had failures. Iterate every
	// commit's rules and emit a row per (commit, rule) so the table is
	// actually load-bearing.
	for commitIdx, commitReport := range report.Commits {
		sha := truncateSHA(commitReport.Commit.Hash)
		if sha == "" {
			sha = localizer.Get("stdin")
		}

		rules := slices.Clone(commitReport.RuleResults)
		slices.SortFunc(rules, func(a, b domain.RuleReport) int {
			return cmp.Compare(a.Name, b.Name)
		})

		for _, rule := range rules {
			writeRuleRow(&builder, "commit", sha, rule, cols, colors, localizer)
		}

		// Separator between commit blocks, between the last commit and repo rules,
		// or before the summary block.
		isLastCommit := commitIdx == len(report.Commits)-1

		hasRepoRules := len(report.Repository.RuleResults) > 0
		switch {
		case !isLastCommit, hasRepoRules:
			builder.WriteString(colors.Info(dataLine) + "\n")
		default:
			builder.WriteString(colors.Info(simpleLine) + "\n")
		}
	}

	// Repository rules
	if len(report.Repository.RuleResults) > 0 {
		for _, rule := range report.Repository.RuleResults {
			writeRuleRow(&builder, "repo", "repository", rule, cols, colors, localizer)
		}

		builder.WriteString(colors.Info(simpleLine) + "\n")
	}

	// Summary section
	builder.WriteString(colors.Info("│") + " " + pad(localizer.Get("SUMMARY"), 96) + " " + colors.Info("│") + "\n")
	builder.WriteString(colors.Info(simpleLine) + "\n")

	rulesSummary := localizer.Get("Rules: Total %d   Passed %d   Failed %d", ruleCounts.total, ruleCounts.total-ruleCounts.failed, ruleCounts.failed)
	builder.WriteString(colors.Info("│") + " " + pad(rulesSummary, 96) + " " + colors.Info("│") + "\n")

	commitsSummary := localizer.Get("Commits: Validated %d   Ignored %d", report.Summary.ValidatedCommits, report.Summary.IgnoredCommits)
	builder.WriteString(colors.Info("│") + " " + pad(commitsSummary, 96) + " " + colors.Info("│") + "\n")

	// Final status
	builder.WriteString(colors.Info("│") + " ")

	if ruleCounts.failed == 0 {
		statusMsg := pad(localizer.Get("✓ ALL CHECKS PASSED SUCCESSFULLY"), 96)
		builder.WriteString(colors.Success(statusMsg))
	} else {
		statusMsg := pad(localizer.Get("✗ VALIDATION FAILED: %d ISSUES", ruleCounts.failed), 96)
		builder.WriteString(colors.Error(statusMsg))
	}

	builder.WriteString(" " + colors.Info("│") + "\n")

	builder.WriteString(colors.Info(bottomBorder) + "\n")

	return builder.String()
}

func writeRuleRow(builder *strings.Builder, rowType, target string, rule domain.RuleReport, cols tableColumns, colors ColorScheme, localizer Localizer) { //nolint:gocognit,cyclop // formatting functions are inherently complex
	// Helper to pad strings
	pad := func(s string, width int) string { //nolint:varnamelen // s is conventional for string parameter
		runes := []rune(s)
		runeLen := len(runes)

		if runeLen > width {
			if width > 3 {
				return string(runes[:width-3]) + "..."
			}

			return string(runes[:width])
		}

		return s + strings.Repeat(" ", width-runeLen)
	}

	localizer = ensureLocalizer(localizer)

	// Determine status and message
	isFailed := rule.Status == domain.StatusFailed
	status := localizer.Get(textPlainStatusPass)
	message := ""
	statusColor := colors.Success

	if isFailed {
		status = localizer.Get(textPlainStatusFail)
		statusColor = colors.Error
	}

	if len(rule.Errors) > 0 {
		message = diagnosticMessage(rule.Errors[0], localizer)
	} else if rule.Message != "" {
		message = rule.Message
	}

	rowTypeLabel := rowType
	switch rowType {
	case "commit":
		rowTypeLabel = localizer.Get("commit")
	case "repo":
		rowTypeLabel = localizer.Get("repo")
	}

	targetLabel := target
	if target == "repository" {
		targetLabel = localizer.Get("repository")
	}

	// Build the row
	builder.WriteString(colors.Info("│") + " ")
	builder.WriteString(pad(rowTypeLabel, cols.typeWidth) + " " + colors.Info("│") + " ")
	builder.WriteString(pad(targetLabel, cols.targetWidth) + " " + colors.Info("│") + " ")

	displayName := DisplayName(rule.Name, localizer)
	builder.WriteString(pad(displayName, cols.ruleWidth) + " " + colors.Info("│") + " ")

	// Status with color applied AFTER padding
	statusPadded := pad(status, cols.statusWidth)
	builder.WriteString(statusColor(statusPadded) + " " + colors.Info("│") + " ")

	// Details/message handling
	switch {
	case message != "" && isFailed:
		// Wrap long messages
		words := strings.Fields(message)
		lines := []string{}
		currentLine := ""

		for _, word := range words {
			testLine := currentLine
			if testLine != "" {
				testLine += " "
			}

			testLine += word

			if utf8.RuneCountInString(testLine) <= cols.detailsWidth {
				currentLine = testLine
			} else {
				if currentLine != "" {
					lines = append(lines, currentLine)
				}

				currentLine = word
			}
		}

		if currentLine != "" {
			lines = append(lines, currentLine)
		}

		// Write first line
		if len(lines) > 0 {
			builder.WriteString(colors.Warning(pad(lines[0], cols.detailsWidth)))
		} else {
			builder.WriteString(pad("", cols.detailsWidth))
		}

		builder.WriteString(" " + colors.Info("│") + "\n")

		// Write wrapped lines
		for i := 1; i < len(lines); i++ { //nolint:varnamelen // i is conventional for loop index
			builder.WriteString(colors.Info("│") + " ")
			builder.WriteString(pad("", cols.typeWidth) + " " + colors.Info("│") + " ")
			builder.WriteString(pad("", cols.targetWidth) + " " + colors.Info("│") + " ")
			builder.WriteString(pad("", cols.ruleWidth) + " " + colors.Info("│") + " ")
			builder.WriteString(pad("", cols.statusWidth) + " " + colors.Info("│") + " ")
			builder.WriteString(colors.Warning(pad(lines[i], cols.detailsWidth)))
			builder.WriteString(" " + colors.Info("│") + "\n")
		}
	case message != "":
		// PASS with info (e.g., active sub-checks)
		builder.WriteString(colors.Muted(pad(message, cols.detailsWidth)))
		builder.WriteString(" " + colors.Info("│") + "\n")
	default:
		// No message
		builder.WriteString(pad("", cols.detailsWidth) + " " + colors.Info("│") + "\n")
	}
}
