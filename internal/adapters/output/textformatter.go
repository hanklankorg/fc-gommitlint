// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// textSink is the writer interface used internally by the text formatter.
// Satisfied by *strings.Builder (collecting to a string for the Text() API)
// and by *bufio.Writer (streaming to an io.Writer via TextTo). The interior
// helpers call only Write and WriteString, so both backends work unchanged.
type textSink interface {
	io.Writer
	io.StringWriter
}

// TextOptions contains configuration for text formatting.
type TextOptions struct {
	VerboseLevel int // 0=quiet, 1=verbose (-v), 2=extra verbose (-vv)
	ShowHelp     bool
	ShowRuleHelp bool
	ShowRuleName bool
	ShowContext  bool
	// Quiet mirrors --quiet: render only failures and the failure summary.
	// Distinct from VerboseLevel=0 (which still shows passed rules and tips).
	Quiet        bool
	RuleHelpName string
	UseColor     bool
	Theme        string
	Colors       ColorScheme
	Localizer    Localizer
	// ConfigSources are display-ready configuration sources, highest precedence
	// first, prepared by the CLI boundary.
	ConfigSources []string
	// ConfigSourcesUnexpected marks configuration reaching in from outside the
	// repository — shown even when everything passed.
	ConfigSourcesUnexpected bool
}

// writeFailedRulesSummary writes a summary of failed rules.
func writeFailedRulesSummary(builder textSink, summary domain.ReportSummary, colors ColorScheme, localizer Localizer) {
	failedCount := 0
	for _, count := range summary.FailedRules {
		failedCount += count
	}

	if failedCount == 0 {
		return
	}

	localizer = ensureLocalizer(localizer)

	issues := localizer.GetN("%d issue", "%d issues", failedCount, failedCount)
	commits := localizer.GetN("%d commit", "%d commits", summary.ValidatedCommits, summary.ValidatedCommits)
	message := localizer.Get("✗ %s found (checked %s", issues, commits)

	if summary.IgnoredCommits > 0 {
		ignored := localizer.GetN("%d ignored", "%d ignored", summary.IgnoredCommits, summary.IgnoredCommits)
		message += localizer.Get(", %s", ignored)
	}

	message += localizer.Get(")")
	// Writes through textSink can in theory return an error; in practice the
	// CLI path uses a bufio.Writer that defers the syscall to Flush(), and
	// the test path uses strings.Builder which never fails. Discarding is
	// safe — Flush at the end of the report surfaces any real failure.
	_, _ = builder.WriteString(colors.Error(message + "\n"))

	// Add detailed rule breakdown
	ruleNames := make([]string, 0, len(summary.FailedRules))
	for ruleName := range summary.FailedRules {
		ruleNames = append(ruleNames, ruleName)
	}

	slices.Sort(ruleNames)

	for _, ruleName := range ruleNames {
		count := summary.FailedRules[ruleName]
		message := localizer.GetN("- %s: %d failure\n", "- %s: %d failures\n", count, ruleName, count)
		_, _ = builder.WriteString(colors.Muted(message))
	}
}

func writeRuleList(builder textSink, ruleNames []string, colors ColorScheme, localizer Localizer) {
	if len(ruleNames) == 0 {
		return
	}

	localizer = ensureLocalizer(localizer)

	rules := localizer.GetN("%d rule", "%d rules", len(ruleNames), len(ruleNames))
	_, _ = builder.WriteString(colors.Muted(localizer.Get("  %s checked:\n", rules)))

	for _, ruleName := range ruleNames {
		_, _ = builder.WriteString(colors.Muted(fmt.Sprintf("  - %s\n", ruleName)))
	}
}

// Text formats a domain report as plain text with colors.
//
// Wraps writeTextReport with a strings.Builder for callers (tests, library
// embedders) that prefer a string return value. CLI consumers should call
// TextTo instead — it writes directly to the output writer, skipping the
// intermediate buffer.
func Text(report domain.Report, options TextOptions) string {
	var builder strings.Builder

	_ = writeTextReport(&builder, report, options)

	return builder.String()
}

// TextTo streams a text-formatted report to w. Output is byte-identical to
// Text(report, options) — same prepared options, same helper functions.
//
// Why streaming: at 5000+ commits the full rendered text is several MB.
// Materialising it in a strings.Builder before writing doubles peak RSS
// for no payoff. bufio.NewWriter coalesces many small WriteString calls
// into a few syscalls; the writer interface lets the same helpers feed
// either backend.
func TextTo(w io.Writer, report domain.Report, options TextOptions) error {
	bw := bufio.NewWriter(w)
	if err := writeTextReport(bw, report, options); err != nil {
		return err
	}

	return bw.Flush()
}

// writeTextReport is the body shared by Text and TextTo. Takes any textSink
// (Write + WriteString); the interior helpers are agnostic about whether
// they're feeding a buffer or a streaming writer.
func writeTextReport(builder textSink, report domain.Report, options TextOptions) error { //nolint:gocognit,cyclop // formatting functions are inherently complex
	options = prepareTextOptions(options)
	localizer := options.Localizer

	// Format commits
	formatCommits(builder, report, options)

	writeRepositoryRules(builder, report.Repository.RuleResults, options.Colors, localizer)

	// Config provenance is context for the verdict, so it precedes the final
	// conclusion rather than appearing as an informational tail after it.
	if !options.Quiet {
		writeConfigSources(builder, options, report.Summary.AllPassed)
	}

	// Summary section. Every failed report uses the same final-summary path,
	// including a single failed commit with no repository rules.
	if len(report.Commits) == 1 && len(report.Repository.RuleResults) == 0 && report.Commits[0].Passed { //nolint:nestif // conditional formatting logic
		if !options.Quiet {
			_, _ = builder.WriteString("\n")

			ruleNames := make([]string, 0, len(report.Commits[0].RuleResults))
			for _, ruleResult := range report.Commits[0].RuleResults {
				ruleNames = append(ruleNames, ruleResult.Name)
			}

			if options.VerboseLevel == 0 {
				writeRuleList(builder, ruleNames, options.Colors, localizer)
			}

			_, _ = builder.WriteString(options.Colors.Muted("\n  " + localizer.Get("Tip: to validate multiple commits, use --count, --range, or --base-branch") + "\n"))

			if report.Summary.SkippedRepoRules > 0 {
				skipped := localizer.GetN("%d repository rule skipped", "%d repository rules skipped", report.Summary.SkippedRepoRules, report.Summary.SkippedRepoRules)
				_, _ = builder.WriteString(options.Colors.Muted("  - " + skipped + "\n"))
			}

			_, _ = builder.WriteString(options.Colors.Success(localizer.Get("✓ All checks passed") + "\n"))
		}
	} else {
		ruleCounts := countReportRules(report)
		totalIssues := ruleCounts.failed

		if totalIssues == 0 && !options.Quiet {
			_, _ = builder.WriteString("\n")
			allRuleNames := collectReportRuleNames(report)
			summaryView := buildReportSummaryView(report, localizer)

			ruleSummary := localizer.GetN("%d rule", "%d rules", len(allRuleNames), len(allRuleNames))

			summary := localizer.Get("✓ All checks passed — %s, %s, %s", summaryView.validatedCommits, ruleSummary, summaryView.warningCount)
			if summaryView.ignoredCount > 0 {
				summary += localizer.Get(", %s", summaryView.ignoredLabel)
			}

			if options.VerboseLevel == 0 {
				_, _ = builder.WriteString(options.Colors.Muted(localizer.Get("Tip: use -v for rule details") + "\n"))
			}

			if summaryView.skippedRepoCount > 0 {
				_, _ = builder.WriteString(options.Colors.Muted("  - " + summaryView.skippedRepoRules + "\n"))
			}

			_, _ = builder.WriteString(options.Colors.Success(summary + "\n"))
		} else if totalIssues > 0 {
			_, _ = builder.WriteString("\n")
			// Count individual violations so the total matches the per-check
			// breakdown; rules that fail without error details fall back to
			// the failed-rule count.
			issueTotal := countReportErrors(report)
			if issueTotal == 0 {
				issueTotal = totalIssues
			}

			issues := localizer.GetN("%d issue", "%d issues", issueTotal, issueTotal)
			summaryView := buildReportSummaryView(report, localizer)

			message := localizer.Get("✗ %s found", issues)
			if breakdown := formatCheckBreakdown(report, localizer); breakdown != "" {
				message += " (" + breakdown + ")"
			}

			if failedCommits := countFailedCommits(report); failedCommits > 0 {
				message += localizer.GetN(" in %d commit", " in %d commits", failedCommits, failedCommits)
			}

			if summaryView.skippedRepoCount > 0 {
				message += localizer.Get(" - %s", summaryView.skippedRepoRules)
			}

			_, _ = builder.WriteString(options.Colors.Error(message + "\n"))
		}
	}

	return nil
}

// breakdownMaxIDs bounds how many check IDs the failure summary names.
// One or two dominating IDs tell the reader it is one class of problem;
// past three the line stops being a summary.
const breakdownMaxIDs = 3

// formatCheckBreakdown aggregates failing check IDs across the report into
// a compact "GL305 ×11, GL102 ×2" string, most frequent first. Errors
// without a public check ID are counted in the total but not named here.
func formatCheckBreakdown(report domain.Report, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)
	counts := map[string]int{}

	forEachReportError(report, func(err domain.ValidationError) {
		if id := string(err.Diagnostic.ID); id != "" {
			counts[id]++
		}
	})

	if len(counts) == 0 {
		return ""
	}

	ids := make([]string, 0, len(counts))
	for id := range counts {
		ids = append(ids, id)
	}

	slices.SortFunc(ids, func(a, b string) int {
		if counts[a] != counts[b] {
			return counts[b] - counts[a]
		}

		return strings.Compare(a, b)
	})

	shown := ids
	if len(shown) > breakdownMaxIDs {
		shown = shown[:breakdownMaxIDs]
	}

	parts := make([]string, 0, len(shown)+1)
	for _, id := range shown {
		parts = append(parts, localizer.Get("%s ×%d", id, counts[id]))
	}

	if hidden := len(ids) - len(shown); hidden > 0 {
		parts = append(parts, localizer.Get("+%d more", hidden))
	}

	return strings.Join(parts, ", ")
}

func countReportErrors(report domain.Report) int {
	total := 0

	forEachReportError(report, func(domain.ValidationError) {
		total++
	})

	return total
}

func countFailedCommits(report domain.Report) int {
	failed := 0

	for _, commitReport := range report.Commits {
		if !commitReport.Passed {
			failed++
		}
	}

	return failed
}

func forEachReportError(report domain.Report, visit func(domain.ValidationError)) {
	for _, commitReport := range report.Commits {
		for _, rule := range commitReport.RuleResults {
			for _, err := range rule.Errors {
				visit(err)
			}
		}
	}

	for _, rule := range report.Repository.RuleResults {
		for _, err := range rule.Errors {
			visit(err)
		}
	}
}

// prepareTextOptions sets up the options with defaults.
func prepareTextOptions(options TextOptions) TextOptions {
	// Set Colors if not already set
	if options.Colors.Success == nil {
		options.Colors = GetColorScheme(options.UseColor, options.Theme)
	}

	options.Localizer = ensureLocalizer(options.Localizer)

	// Auto-set options based on verbosity level
	if options.VerboseLevel >= 1 {
		options.ShowRuleName = true
		options.ShowContext = true
		options.ShowHelp = true
	}

	return options
}

// writeConfigSources renders config provenance when it is worth the line.
//
// Which config is in effect is the question a reader asks when a rule fires
// they did not expect — not after a clean run. Printing it unconditionally put
// an absolute path in the output of every passing commit-msg hook, so the line
// is earned by one of: something failed, the user asked for detail, or the
// config came from outside the repository and the reader could not have
// guessed it.
func writeConfigSources(builder textSink, options TextOptions, allPassed bool) {
	if len(options.ConfigSources) == 0 {
		return
	}

	if allPassed && options.VerboseLevel == 0 && !options.ConfigSourcesUnexpected {
		return
	}

	localizer := ensureLocalizer(options.Localizer)
	line := localizer.Get("Config: %s", strings.Join(options.ConfigSources, " + "))
	_, _ = builder.WriteString(options.Colors.Muted(line + "\n"))
}

func writeRepositoryRules(builder textSink, ruleResults []domain.RuleReport, colors ColorScheme, localizer Localizer) {
	if len(ruleResults) == 0 {
		return
	}

	_, _ = builder.WriteString("\n")

	// Use the same ` - <message>` separator for both passed and failed rules.
	// The rule.Message owns its own punctuation; the renderer only owns layout.
	// Previously passed rules wrapped in parens, which combined with messages
	// like "1 ahead of 'main' (max 10)" to produce "(1 ahead of 'main' (max 10))"
	// — two levels of nesting that read like a typo.
	for _, rule := range ruleResults {
		displayName := DisplayName(rule.Name, localizer)

		if rule.Status == domain.StatusPassed {
			line := fmt.Sprintf("%s %s", colors.Success("✓"), displayName)
			if rule.Message != "" {
				line += colors.Muted(" - " + rule.Message)
			}

			_, _ = builder.WriteString(line + "\n")

			continue
		}

		_, _ = fmt.Fprintf(builder, "%s %s", colors.Error("✗"), colors.Error(displayName))

		if rule.Message != "" {
			_, _ = builder.WriteString(colors.Muted(" - " + rule.Message))
		}

		_, _ = builder.WriteString("\n")
	}
}

func formatCommits(builder textSink, report domain.Report, options TextOptions) {
	if len(report.Commits) == 0 {
		return
	}

	_, _ = builder.WriteString("\n")

	previousWasExpanded := false
	localizer := ensureLocalizer(options.Localizer)

	for index, commitReport := range report.Commits {
		// Quiet mode: pass-through commits contribute nothing — neither the compact
		// "✓ sha subject" line nor the expanded header. The user opted into a
		// failures-only stream.
		if options.Quiet && commitReport.Passed {
			continue
		}

		// Passing commits stay one-liners below -vv: at -v the user wants
		// failure detail, not fifty identical expanded pass blocks.
		showCompact := commitReport.Passed && options.VerboseLevel < 2

		if showCompact { //nolint:nestif // formatting logic needs nested conditions
			// Add blank line if previous was expanded
			if previousWasExpanded {
				_, _ = builder.WriteString("\n")
			}

			shortSHA := truncateSHA(commitReport.Commit.Hash)
			subject := truncateSubjectForHeader(commitReport.Commit.Subject)
			// For single commit, don't show #1
			if len(report.Commits) == 1 {
				_, _ = fmt.Fprintf(builder, "%s %s %s\n",
					options.Colors.Success("✓"),
					shortSHA,
					subject)
			} else {
				commitNum := localizer.Get("#%d", index+1)
				_, _ = fmt.Fprintf(builder, "%s %s %s %s\n",
					commitNum,
					options.Colors.Success("✓"),
					shortSHA,
					subject)
			}

			previousWasExpanded = false
		} else {
			// Add blank line before expanded (except first)
			if index > 0 {
				_, _ = builder.WriteString("\n")
			}

			// Expanded format for failed or verbose
			writeCommitHeader(builder, commitReport, index, len(report.Commits), options.Colors, localizer)
			writeCommitRules(builder, commitReport, options.Colors, options, nil)

			previousWasExpanded = true
		}
	}
}

// writeCommitHeader writes the commit header to the builder.
func writeCommitHeader(builder textSink, commitReport domain.CommitReport, index, totalCommits int, _ ColorScheme, localizer Localizer) {
	shortSHA := truncateSHA(commitReport.Commit.Hash)
	subject := truncateSubjectForHeader(commitReport.Commit.Subject)

	localizer = ensureLocalizer(localizer)

	switch {
	case totalCommits > 1:
		_, _ = builder.WriteString(localizer.Get("COMMIT #%d [%s] %s", index+1, shortSHA, subject) + "\n")
	case shortSHA == "":
		// No SHA when input is stdin or --message-file: don't render an empty [] marker.
		_, _ = builder.WriteString(localizer.Get("COMMIT %s", subject) + "\n")
	default:
		_, _ = builder.WriteString(localizer.Get("COMMIT [%s] %s", shortSHA, subject) + "\n")
	}
}

// commitHeaderSubjectMax bounds the subject preview rendered in the COMMIT
// header. Per-rule details still report the actual subject length.
const commitHeaderSubjectMax = 80

// truncateSubjectForHeader trims the COMMIT header preview to a sane width,
// preserving full-rune boundaries (UTF-8 safe) and adding an ellipsis when cut.
func truncateSubjectForHeader(subject string) string {
	subject = cleanForTerminal(subject)
	if utf8.RuneCountInString(subject) <= commitHeaderSubjectMax {
		return subject
	}

	// Reserve three runes for the ellipsis so the visible width matches the budget.
	return truncateToRunes(subject, commitHeaderSubjectMax-3) + "..."
}

// writeCommitRules writes the commit rules to the builder.
//
// Quiet mode hides passed rules entirely — the user opted into a failures-only
// view and we honor it.
func writeCommitRules(builder textSink, commitReport domain.CommitReport, colors ColorScheme, options TextOptions, _ []domain.RuleReport) {
	rulesToShow := filterCommitRules(commitReport.RuleResults, options)
	rulesToShow = sortRulesAlphabetically(rulesToShow)

	for _, rule := range rulesToShow {
		if rule.Status == domain.StatusPassed {
			if options.Quiet {
				continue
			}

			writePassedRule(builder, rule, colors, options)
		} else {
			writeFailedRule(builder, rule, colors, options)
		}
	}
}

// filterCommitRules filters rules based on help options.
func filterCommitRules(commitRules []domain.RuleReport, options TextOptions) []domain.RuleReport {
	if options.ShowRuleHelp && options.RuleHelpName != "" {
		for _, ruleReport := range commitRules {
			if ruleReport.Name == options.RuleHelpName {
				return []domain.RuleReport{ruleReport}
			}
		}

		return []domain.RuleReport{}
	}

	return commitRules
}

// sortRulesAlphabetically sorts rules by name alphabetically.
func sortRulesAlphabetically(rules []domain.RuleReport) []domain.RuleReport {
	sorted := make([]domain.RuleReport, len(rules))
	copy(sorted, rules)

	slices.SortFunc(sorted, func(first, second domain.RuleReport) int {
		if first.Name < second.Name {
			return -1
		}

		if first.Name > second.Name {
			return 1
		}

		return 0
	})

	return sorted
}

// writePassedRule writes a passed rule to the builder.
func writePassedRule(builder textSink, rule domain.RuleReport, colors ColorScheme, options TextOptions) {
	displayName := DisplayName(rule.Name, options.Localizer)

	line := fmt.Sprintf("  %s %s", colors.Success("✓"), displayName)
	if message := passedRuleMessage(rule, options); message != "" {
		line += colors.Muted(fmt.Sprintf(" (%s)", message))
	}

	_, _ = builder.WriteString(line + "\n")
}

func passedRuleMessage(rule domain.RuleReport, options TextOptions) string {
	if rule.Message == "" {
		return ""
	}

	// The active-check catalog repeats identically for every commit, so it is
	// reference material, not per-commit signal: reserve it for -vv.
	if options.VerboseLevel < 2 && isActiveCheckSummary(rule) {
		return ""
	}

	return rule.Message
}

func isActiveCheckSummary(rule domain.RuleReport) bool {
	if len(rule.Checks) == 0 {
		return false
	}

	return rule.Message == strings.Join(rule.Checks, ", ")
}

// writeFailedRule writes a failed rule to the builder.
func writeFailedRule(builder textSink, rule domain.RuleReport, colors ColorScheme, options TextOptions) {
	// Early return for rules without errors
	if len(rule.Errors) == 0 {
		writeRuleWithoutErrors(builder, rule, colors, options.Localizer)

		return
	}

	// Write the rule header with first error
	writeRuleHeader(builder, rule, colors, options.Localizer)

	// Handle verbose vs non-verbose modes
	if options.VerboseLevel > 0 {
		writeVerboseErrors(builder, rule.Errors, colors, options, options.Localizer)
	} else {
		writeAdditionalErrorMessages(builder, rule.Errors, colors, options.Localizer)
	}

	writeRuleHelpHint(builder, rule, colors, options.Localizer)
}

// writeRuleHelpHint writes a one-line pointer to detailed rule docs.
//
// Rationale: --rule-help carries the canonical examples for each rule, but users
// only discover it after stumbling on it in --help. Surfacing the exact command
// next to a failure closes that gap. Shown at every verbosity level: default
// output is what CI shows, and one muted line is where the discovery matters most.
func writeRuleHelpHint(builder textSink, rule domain.RuleReport, colors ColorScheme, localizer Localizer) {
	if rule.Name == "" {
		return
	}

	localizer = ensureLocalizer(localizer)

	const indent = "      "

	hint := localizer.Get("See examples: gommitlint validate --rule-help %s", strings.ToLower(rule.Name))
	_, _ = builder.WriteString(colors.Muted(indent + hint + "\n"))
}

// writeRuleWithoutErrors writes a rule that has no errors array but may have a message.
func writeRuleWithoutErrors(builder textSink, rule domain.RuleReport, colors ColorScheme, localizer Localizer) {
	displayName := DisplayName(rule.Name, localizer)
	_, _ = fmt.Fprintf(builder, "  %s %s", colors.Error("✗"), colors.Error(displayName))

	if rule.Message != "" {
		_, _ = builder.WriteString(colors.Muted(" - " + rule.Message))
	}

	_, _ = builder.WriteString("\n")
}

// failureLine renders one validation error as a single display line,
// prefixed with its public GLxxx check ID when the error carries one.
// The ID is the stable reference users grep for in docs and CI logs, so
// it belongs on the failure itself, at every verbosity level.
func failureLine(err domain.ValidationError, localizer Localizer) string {
	message := cleanForTerminal(convertToUserFriendlyMessage(err, localizer))
	if id := string(err.Diagnostic.ID); id != "" {
		return id + ": " + message
	}

	return message
}

// writeRuleHeader writes the rule name and first error message.
func writeRuleHeader(builder textSink, rule domain.RuleReport, colors ColorScheme, localizer Localizer) {
	localizer = ensureLocalizer(localizer)
	displayName := DisplayName(rule.Name, localizer)
	_, _ = fmt.Fprintf(builder, "  %s %s", colors.Error("✗"), colors.Error(displayName))
	_, _ = builder.WriteString(colors.Muted(" - " + failureLine(rule.Errors[0], localizer) + "\n"))
}

// defaultMaxViolations bounds how many violations one rule renders at the
// default verbosity level. A rule with dozens of hits (e.g. every body line
// too long) would otherwise drown the rest of the report; the collapsed tail
// still reports the hidden count.
const defaultMaxViolations = 5

// writeAdditionalErrorMessages writes the remaining error lines in non-verbose
// mode. Receives all errors including the first: the first is already rendered
// in the rule header, but must seed the dedup set so a later error resolving to
// the same text is not repeated.
func writeAdditionalErrorMessages(builder textSink, errors []domain.ValidationError, colors ColorScheme, localizer Localizer) {
	const indent = "      "

	localizer = ensureLocalizer(localizer)
	seen := make(map[string]bool, len(errors))
	shown := 0
	hidden := 0

	for errIdx, err := range errors {
		line := failureLine(err, localizer)
		// Avoid printing the same user-facing line twice. Rules can emit multiple
		// errors that resolve to identical text; display dedup is the least-invasive fix.
		if seen[line] {
			continue
		}

		seen[line] = true
		shown++

		if errIdx == 0 {
			continue // already rendered in the rule header
		}

		if shown > defaultMaxViolations {
			hidden++

			continue
		}

		_, _ = builder.WriteString(colors.Muted(fmt.Sprintf("%s%s\n", indent, line)))
	}

	if hidden > 0 {
		more := localizer.GetN("… and %d more", "… and %d more", hidden, hidden)
		_, _ = builder.WriteString(colors.Muted(indent + more + "\n"))
	}
}

// writeVerboseErrors writes detailed error information in verbose mode.
//
// Layout per violation: one failure line (check ID + message), then context.
// The ladder: -v keeps context curated (only Expected/Actual values the
// finding does not already state) and adds remediation help, -vv renders the
// complete context map. Help appears once per rule after the violations —
// repeating identical guidance under every violation buried the findings.
func writeVerboseErrors(builder textSink, errors []domain.ValidationError, colors ColorScheme, options TextOptions, localizer Localizer) {
	const indent = "      " // 6 spaces for alignment under the error message

	localizer = ensureLocalizer(localizer)
	seenContext := make(map[string]bool)

	for i, ruleErr := range errors {
		// For additional errors after the first, show the failure line
		if i > 0 {
			_, _ = builder.WriteString(colors.Muted(fmt.Sprintf("%s%s\n", indent, failureLine(ruleErr, localizer))))
		}

		writeErrorContext(builder, ruleErr, indent, colors, options, localizer, seenContext)
	}

	writeErrorsHelp(builder, errors, indent, colors, options, localizer)
}

// writeErrorContext writes error context fields (Expected, Actual, etc).
//
// At -v, a context row whose value the rendered finding already states is
// suppressed: "Line 1 is 74 characters (max 72)" followed by
// "Expected: max 72" and "Actual: 74" repeats every number the reader just
// saw. At -vv nothing is suppressed — that level is the full diagnostic dump.
func writeErrorContext(builder textSink, err domain.ValidationError, indent string, colors ColorScheme, options TextOptions, localizer Localizer, seen map[string]bool) {
	if len(err.Context) == 0 || !options.ShowContext {
		return
	}

	localizer = ensureLocalizer(localizer)

	if options.VerboseLevel >= 2 {
		writeFullErrorContext(builder, err, indent, colors, localizer, seen)

		return
	}

	finding := convertToUserFriendlyMessage(err, localizer)

	// Show Expected first if it exists
	if val, ok := err.Context[diagnostics.ContextExpected]; ok && !findingEmbedsValue(finding, val) {
		_, _ = builder.WriteString(colors.Info(fmt.Sprintf("%s%s\n", indent, localizer.Get("Expected: %s", cleanForTerminal(val)))))
	}

	// Then show Actual if it exists
	if val, ok := err.Context[diagnostics.ContextActual]; ok && !findingEmbedsValue(finding, val) {
		_, _ = builder.WriteString(colors.Error(fmt.Sprintf("%s%s\n", indent, localizer.Get("Actual: %s", cleanForTerminal(val)))))
	}
}

// writeFullErrorContext renders the complete context map for one violation:
// Expected and Actual first, then the remaining keys sorted. Two filters
// only: the machine-readable reason tag (the reason already lives in the
// diagnostic), and rows identical to one already printed for this rule —
// static values like the subject otherwise repeat under every violation.
func writeFullErrorContext(builder textSink, err domain.ValidationError, indent string, colors ColorScheme, localizer Localizer, seen map[string]bool) {
	emit := func(line string, style func(...any) string) {
		line = cleanForTerminal(line)
		if seen[line] {
			return
		}

		seen[line] = true

		_, _ = builder.WriteString(style(indent + line + "\n"))
	}

	if val, ok := err.Context[diagnostics.ContextExpected]; ok {
		emit(localizer.Get("Expected: %s", val), colors.Info)
	}

	if val, ok := err.Context[diagnostics.ContextActual]; ok {
		emit(localizer.Get("Actual: %s", val), colors.Error)
	}

	reason := string(err.Diagnostic.Reason)

	keys := make([]string, 0, len(err.Context))
	for key, val := range err.Context {
		if key == diagnostics.ContextExpected || key == diagnostics.ContextActual || val == reason {
			continue
		}

		keys = append(keys, key)
	}

	slices.Sort(keys)

	for _, key := range keys {
		emit(formatTechnicalContextKey(key, localizer)+": "+err.Context[key], colors.Muted)
	}
}

// findingEmbedsValue reports whether the finding text already states the
// context value. Matches on word boundaries so a value like "lower" is not
// considered embedded in "lowercase".
func findingEmbedsValue(finding, value string) bool {
	if value == "" {
		return false
	}

	for start := 0; ; start++ {
		idx := strings.Index(finding[start:], value)
		if idx < 0 {
			return false
		}

		start += idx
		if boundaryBefore(finding[:start]) && boundaryAfter(finding[start+len(value):]) {
			return true
		}
	}
}

// boundaryBefore reports whether the text preceding a match ends at a word
// boundary: empty, or a non-alphanumeric rune right before the match.
func boundaryBefore(prefix string) bool {
	if prefix == "" {
		return true
	}

	last, _ := utf8.DecodeLastRuneInString(prefix)

	return !isWordRune(last)
}

// boundaryAfter reports whether the text following a match starts at a word
// boundary: empty, or a non-alphanumeric rune right after the match.
func boundaryAfter(suffix string) bool {
	if suffix == "" {
		return true
	}

	first, _ := utf8.DecodeRuneInString(suffix)

	return !isWordRune(first)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// writeErrorsHelp writes help text once per rule in verbose mode,
// deduplicated across violations that share the same guidance.
func writeErrorsHelp(builder textSink, errors []domain.ValidationError, indent string, colors ColorScheme, options TextOptions, localizer Localizer) {
	if options.VerboseLevel < 1 || !options.ShowHelp {
		return
	}

	localizer = ensureLocalizer(localizer)
	seen := make(map[string]bool, 1)

	for _, err := range errors {
		helpMsg, ok := LookupHelpMessage(err.Diagnostic.Reason, cleanContextValues(err.Context))
		if !ok {
			continue
		}

		text := cleanHelpLines(helpMsg.Localize(localizer))
		if seen[text] {
			continue
		}

		if len(seen) == 0 {
			_, _ = builder.WriteString(colors.Warning(indent + localizer.Get(outputHelpHeader) + "\n"))
		}

		seen[text] = true

		_, _ = builder.WriteString(colors.Warning(fmt.Sprintf("%s  %s\n", indent, text)))
	}
}

// cleanContextValues returns a copy of an error context with every value made
// terminal-safe, so values interpolated into help cannot add lines or controls.
func cleanContextValues(context map[string]string) map[string]string {
	cleaned := make(map[string]string, len(context))
	for key, value := range context {
		cleaned[key] = cleanForTerminal(value)
	}

	return cleaned
}

// cleanHelpLines makes each line of a help message terminal-safe while keeping
// the message's own line breaks and continuation-line indentation.
func cleanHelpLines(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		lines[index] = indent + cleanForTerminal(line)
	}

	return strings.Join(lines, "\n")
}

func writeRuleReports(builder textSink, rules []domain.RuleReport, colors ColorScheme, options TextOptions) int {
	// Auto-set options based on verbosity level if not explicitly set
	options = setAutoOptionsFromVerbosity(options)

	passedCount := 0

	for _, rule := range rules {
		if rule.Status == domain.StatusPassed {
			passedCount++

			writePassedRule(builder, rule, colors, options)
		} else {
			writeFailedRule(builder, rule, colors, options)
		}
	}

	return passedCount
}

// setAutoOptionsFromVerbosity automatically sets options based on verbosity level.
func setAutoOptionsFromVerbosity(options TextOptions) TextOptions {
	// Rule names should always be shown (default behavior)
	options.ShowRuleName = true

	if options.VerboseLevel >= 1 {
		options.ShowContext = true
		options.ShowHelp = true
	}

	return options
}

// writePassedRule writes a passed rule to the builder.

// formatCommitRules formats commit validation results for plain text output.
func formatCommitRules(builder textSink, commits []domain.CommitReport) {
	for _, commitReport := range commits {
		commitSHA := truncateSHA(commitReport.Commit.Hash)
		subject := commitReport.Commit.Subject

		for _, result := range commitReport.RuleResults {
			formatRuleResult(builder, "COMMIT", commitSHA, subject, result)
		}
	}
}

// formatRepositoryRules formats repository validation results for plain text output.
func formatRepositoryRules(builder textSink, results []domain.RuleReport) {
	for _, result := range results {
		status, message := getRuleStatus(result)
		// Pad to the commit-row column count so every row in the file shares a column shape.
		writePlainRecord(builder, "REPO", "", "", result.Name, status, message)
	}
}

// writePlainRecord emits one RFC-4180 CSV row. Previously prefixed with "TYPE:"
// outside the CSV scope, breaking any standard csv.Reader; now the row type is
// the first column, so the whole stream is uniform and tooling-friendly.
//
// Column order (stable): type, sha, subject, rule, status, message.
// Repo-scoped rules use empty strings for sha and subject so every line has the
// same arity.
func writePlainRecord(builder textSink, fields ...string) {
	writer := csv.NewWriter(builder)

	normalized := make([]string, 0, len(fields))
	for _, field := range fields {
		normalized = append(normalized, normalizePlainField(field))
	}

	if err := writer.Write(normalized); err != nil {
		writePlainRecordFallback(builder, normalized)

		return
	}

	writer.Flush()

	if err := writer.Error(); err != nil {
		writePlainRecordFallback(builder, normalized)
	}
}

func writePlainRecordFallback(builder textSink, fields []string) {
	_, _ = builder.WriteString(strings.Join(fields, ","))
	_, _ = builder.WriteString("\n")
}

func normalizePlainField(field string) string {
	field = strings.ReplaceAll(field, "\r\n", "\n")
	field = strings.ReplaceAll(field, "\r", "\n")

	return strings.ReplaceAll(field, "\n", "\\n")
}

// formatRuleResult formats a single rule result for output.
func formatRuleResult(builder textSink, recordType, sha, subject string, result domain.RuleReport) {
	status, message := getRuleStatus(result)
	writePlainRecord(builder, recordType, sha, subject, result.Name, status, message)
}

// getRuleStatus extracts status and message from a rule result.
func getRuleStatus(result domain.RuleReport) (string, string) {
	if len(result.Errors) > 0 {
		err := result.Errors[0]
		if err.Diagnostic.ID != "" {
			return textPlainStatusFail, plainDiagnosticMessage(err, nil)
		}

		return textPlainStatusFail, err.Message
	}

	return textPlainStatusPass, ""
}

// TextPlain formats a report for automated parsing by CI/CD systems.
//
// Output is RFC-4180 CSV, one record per line, with the record type as the
// first column so a single csv.NewReader can process the whole stream.
//
// Record schemas:
//
//	COMMIT,sha,subject,rule,status,message
//	REPO,,,rule,status,message                 (sha and subject blank for layout uniformity)
//	SUMMARY,status,passed,total,validated,ignored
//
// Newlines in any field are escaped as the literal sequence "\n" so each
// record stays on a single line — callers that want the original newline can
// reverse the substitution.
func TextPlain(report domain.Report) string {
	var builder strings.Builder

	formatCommitRules(&builder, report.Commits)

	formatRepositoryRules(&builder, report.Repository.RuleResults)

	ruleCounts := countReportRules(report)

	status := textPlainStatusPass
	if !report.Summary.AllPassed {
		status = textPlainStatusFail
	}

	writePlainRecord(
		&builder,
		"SUMMARY",
		status,
		strconv.Itoa(ruleCounts.passed),
		strconv.Itoa(ruleCounts.total),
		strconv.Itoa(report.Summary.ValidatedCommits),
		strconv.Itoa(report.Summary.IgnoredCommits),
	)

	return builder.String()
}
