// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"fmt"
	"io"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// ConfigCheckIssue describes a single configuration health issue.
type ConfigCheckIssue struct {
	Code    string `json:"code"    yaml:"code"`
	Hint    string `json:"hint"    yaml:"hint"`
	Message string `json:"message" yaml:"message"`
	Source  string `json:"source"  yaml:"source"`
}

// ConfigCheckReport summarizes configuration health for display and automation.
type ConfigCheckReport struct {
	ConfigSource           string                              `json:"config_source"                      yaml:"config_source"`
	ConfigSources          []configuration.ConfigSource        `json:"config_sources"                     yaml:"config_sources"`
	Issues                 []ConfigCheckIssue                  `json:"issues"                             yaml:"issues"`
	ManagedSources         []configuration.ManagedSourceStatus `json:"managed_sources"                    yaml:"managed_sources"`
	RecommendedNextCommand string                              `json:"recommended_next_command,omitempty" yaml:"recommended_next_command,omitempty"`
	Summary                ManagedSourceSummaryReport          `json:"summary"                            yaml:"summary"`
	OK                     bool                                `json:"ok"                                 yaml:"ok"`
	Warnings               []string                            `json:"warnings"                           yaml:"warnings"`
}

// ManagedSourceSummary describes one managed source status summary entry.
type ManagedSourceSummary struct {
	Destination string `json:"destination" yaml:"destination"`
	ID          string `json:"id"          yaml:"id"`
	NextStep    string `json:"next_step"   yaml:"next_step"`
	Status      string `json:"status"      yaml:"status"`
}

// ManagedSourceSummaryReport aggregates managed source statuses for output.
type ManagedSourceSummaryReport struct {
	Counts map[string]int         `json:"counts" yaml:"counts"`
	Items  []ManagedSourceSummary `json:"items"  yaml:"items"`
}

// ExecuteConfigCheck validates the active configuration and vendored managed sources.
func ExecuteConfigCheck(_ context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()

	if err := validateConfigSubcommandArgs(cmd, localizer, commandNameCheck); err != nil {
		return err
	}

	cfgResult, format, view, err := resolveConfigPresentation(cmd, localizer)
	if err != nil {
		return err
	}

	statuses := configuration.InspectManagedSources(syncConfigPath(cfgResult), cfgResult.Config, nil)
	report := BuildConfigCheckReport(cfgResult, statuses, localizer)

	if err := printConfigCheckReport(cmd.Writer, report, format, view, localizer); err != nil {
		return err
	}

	if report.OK {
		return nil
	}

	return newRecoveryErrorWithCause(
		ErrConfigCheckFailed,
		output.Msg("configuration issues found"),
		localizer.Get("Review the reported config warnings and managed source statuses"),
		helpSuggestion(localizer, "gommitlint config show --format=yaml"),
	)
}

// BuildConfigCheckReport creates a structured config health report.
func BuildConfigCheckReport(cfgResult ConfigResult, statuses []configuration.ManagedSourceStatus, localizer output.Localizer) ConfigCheckReport {
	issues := collectConfigCheckIssues(cfgResult.Warnings, statuses, localizer)

	return ConfigCheckReport{
		ConfigSource:           cfgResult.DisplaySource(),
		ConfigSources:          cfgResult.Sources,
		Issues:                 issues,
		ManagedSources:         statuses,
		RecommendedNextCommand: recommendNextConfigCommand(statuses, issues),
		Summary:                buildManagedSourceSummary(statuses),
		OK:                     len(issues) == 0,
		Warnings:               output.FormatWarnings(configWarningDetails(cfgResult.Warnings), localizer),
	}
}

func recommendNextConfigCommand(statuses []configuration.ManagedSourceStatus, issues []ConfigCheckIssue) string {
	for _, status := range statuses {
		if managedSourceNeedsAction(status) {
			return status.NextStep
		}
	}

	for _, issue := range issues {
		if issue.Code == configCheckCodeWarning {
			return "gommitlint config show --format=yaml"
		}
	}

	if len(issues) > 0 {
		return "gommitlint config sync --check"
	}

	return ""
}

func managedSourceNeedsAction(status configuration.ManagedSourceStatus) bool {
	return status.IntegrityStatus == configuration.ManagedSourceIntegrityMismatch ||
		status.Status == configuration.ManagedSourceStatusMissing ||
		status.Status == configuration.ManagedSourceStatusInvalid
}

func buildManagedSourceSummary(statuses []configuration.ManagedSourceStatus) ManagedSourceSummaryReport {
	report := ManagedSourceSummaryReport{Counts: make(map[string]int), Items: make([]ManagedSourceSummary, 0, len(statuses))}

	for _, status := range statuses {
		summaryStatus := managedSourceSummaryStatus(status)
		report.Counts[summaryStatus]++
		report.Items = append(report.Items, ManagedSourceSummary{
			Destination: status.Destination,
			ID:          status.ID,
			NextStep:    status.NextStep,
			Status:      summaryStatus,
		})
	}

	return report
}

func managedSourceSummaryStatus(status configuration.ManagedSourceStatus) string {
	if status.IntegrityStatus == configuration.ManagedSourceIntegrityMismatch {
		return configuration.ManagedSourceIntegrityMismatch
	}

	return status.Status
}

func collectConfigCheckIssues(warnings []domain.Warning, statuses []configuration.ManagedSourceStatus, localizer output.Localizer) []ConfigCheckIssue {
	issues := make([]ConfigCheckIssue, 0, len(warnings)+len(statuses))
	loc := output.EnsureLocalizer(localizer)

	domainWarnings := configWarningDetails(warnings)
	for _, warning := range domainWarnings {
		issues = append(issues, ConfigCheckIssue{Code: configCheckCodeWarning, Hint: loc.Get("Review the warning and update your config if needed"), Message: output.FormatWarning(warning, loc), Source: configCheckSourceWarning})
	}

	for _, status := range statuses {
		switch status.Status {
		case configuration.ManagedSourceStatusMissing:
			issues = append(issues, ConfigCheckIssue{
				Code:    "managed_source_missing",
				Hint:    loc.Get("Run 'gommitlint config sync --source %s'", status.ID),
				Message: loc.Get("managed source %s is missing vendored file %s", status.ID, status.Destination),
				Source:  status.ID,
			})
		case configuration.ManagedSourceStatusInvalid:
			issues = append(issues, ConfigCheckIssue{
				Code:    "managed_source_invalid",
				Hint:    loc.Get("Re-sync '%s' or inspect the vendored file manually", status.ID),
				Message: loc.Get("managed source %s has invalid vendored file %s", status.ID, status.Destination),
				Source:  status.ID,
			})
		}

		if status.IntegrityStatus == configuration.ManagedSourceIntegrityMismatch {
			issues = append(issues, ConfigCheckIssue{
				Code:    "managed_source_integrity_mismatch",
				Hint:    loc.Get("Run 'gommitlint config sync --source %s' or update integrity.sha256 if the change is intentional", status.ID),
				Message: loc.Get("managed source %s vendored file does not match integrity.sha256", status.ID),
				Source:  status.ID,
			})
		}
	}

	return issues
}

func printConfigCheckReport(outputWriter io.Writer, report ConfigCheckReport, format string, view configTextView, localizer output.Localizer) error {
	switch format {
	case output.FormatJSON:
		return printConfigCheckJSON(outputWriter, report)
	case configTypes.ConfigFormatYAML:
		return printConfigCheckYAML(outputWriter, report)
	default:
		return printConfigCheckText(outputWriter, report, view, localizer)
	}
}

func printConfigCheckJSON(outputWriter io.Writer, report ConfigCheckReport) error {
	return writeIndentedJSON(outputWriter, report, output.Msg("failed to encode config check as JSON"))
}

func printConfigCheckYAML(outputWriter io.Writer, report ConfigCheckReport) (err error) {
	encoder := newIndentedYAMLEncoder(outputWriter)

	defer func() {
		if closeErr := encoder.Close(); closeErr != nil && err == nil {
			err = configOutputError(closeErr, output.Msg("failed to encode config check as YAML"))
		}
	}()

	if err := encoder.Encode(report); err != nil {
		return configOutputError(err, output.Msg("failed to encode config check as YAML"))
	}

	return nil
}

func printConfigCheckText(outputWriter io.Writer, report ConfigCheckReport, view configTextView, localizer output.Localizer) error {
	if err := writeConfigCheckSummary(outputWriter, report, localizer); err != nil {
		return err
	}

	// Happy-path collapse: when nothing is present, the legacy expansion
	// repeated "- none" four times. The summary line already carries the
	// status, warning count, and managed-source count — anything more is
	// noise on the most common path.
	happyPath := report.OK &&
		len(report.ManagedSources) == 0 &&
		len(report.Warnings) == 0 &&
		len(report.Issues) == 0

	if happyPath {
		return nil
	}

	if err := writeManagedSourceSummary(outputWriter, report.Summary, view, localizer); err != nil {
		return err
	}

	if view == configViewCompact {
		return nil
	}

	if err := writeConfigCheckWarnings(outputWriter, report.Warnings, localizer); err != nil {
		return err
	}

	return writeConfigCheckIssues(outputWriter, report.Issues, localizer)
}

func writeConfigCheckSummary(outputWriter io.Writer, report ConfigCheckReport, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	statusLine := loc.Get("Status: OK")
	if !report.OK {
		statusLine = loc.GetN("Status: FAIL (%d issue)", "Status: FAIL (%d issues)", len(report.Issues), len(report.Issues))
	}

	lines := []string{
		loc.Get("Gommitlint Configuration Check"),
		"==============================",
		loc.Get("Configuration Source: %s", output.CleanForTerminal(report.ConfigSource)),
		loc.Get("Managed Sources: %d", len(report.ManagedSources)),
		loc.Get("Summary: %s", formatManagedSourceCounts(report.Summary.Counts, loc)),
		loc.Get("Warnings: %d", len(report.Warnings)),
		statusLine,
	}

	if report.RecommendedNextCommand != "" {
		lines = append(lines, loc.Get("Recommended next command: %s", output.CleanForTerminal(report.RecommendedNextCommand)))
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(outputWriter, line); err != nil {
			return configOutputError(err, output.Msg("failed to write config check output"))
		}
	}

	return nil
}

func writeConfigCheckWarnings(outputWriter io.Writer, warnings []string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	if _, err := fmt.Fprintln(outputWriter, loc.Get("Warnings:")); err != nil {
		return configOutputError(err, output.Msg("failed to write config check output"))
	}

	if len(warnings) == 0 {
		if _, err := fmt.Fprintln(outputWriter, loc.Get("- none")); err != nil {
			return configOutputError(err, output.Msg("failed to write config check output"))
		}

		return nil
	}

	for _, warning := range warnings {
		if _, err := fmt.Fprintf(outputWriter, "- %s\n", output.CleanForTerminal(warning)); err != nil {
			return configOutputError(err, output.Msg("failed to write config check output"))
		}
	}

	return nil
}

func writeManagedSourceSummary(outputWriter io.Writer, summary ManagedSourceSummaryReport, view configTextView, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	if err := writeConfigCheckLine(outputWriter, loc.Get("Managed Source Summary:")); err != nil {
		return err
	}

	if len(summary.Items) == 0 {
		return writeConfigCheckLine(outputWriter, loc.Get("- none"))
	}

	for _, item := range summary.Items {
		if err := writeManagedSourceSummaryItem(outputWriter, item, view, loc); err != nil {
			return err
		}
	}

	return nil
}

func writeManagedSourceSummaryItem(outputWriter io.Writer, item ManagedSourceSummary, view configTextView, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	lines := []string{
		"- " + output.CleanForTerminal(item.ID),
		"  " + loc.Get("status: %s", output.CleanForTerminal(item.Status)),
	}

	if view == configViewVerbose {
		lines = append(lines, "  "+loc.Get("destination: %s", output.CleanForTerminal(item.Destination)))
	}

	if item.NextStep != "" && item.Status != configuration.ManagedSourceStatusVendored {
		lines = append(lines, "  "+loc.Get("next step: %s", output.CleanForTerminal(item.NextStep)))
	}

	lines = append(lines, "")

	for _, line := range lines {
		if err := writeConfigCheckLine(outputWriter, line); err != nil {
			return err
		}
	}

	return nil
}

func writeConfigCheckIssues(outputWriter io.Writer, issues []ConfigCheckIssue, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	if _, err := fmt.Fprintln(outputWriter, loc.Get("Configuration Issues:")); err != nil {
		return configOutputError(err, output.Msg("failed to write config check output"))
	}

	if len(issues) == 0 {
		return writeConfigCheckLine(outputWriter, loc.Get("- none"))
	}

	for _, issue := range issues {
		if err := writeConfigCheckIssue(outputWriter, issue, loc); err != nil {
			return err
		}
	}

	return nil
}

func formatManagedSourceCounts(counts map[string]int, localizer output.Localizer) string {
	loc := output.EnsureLocalizer(localizer)
	orderedStatuses := []string{
		configuration.ManagedSourceIntegrityMismatch,
		configuration.ManagedSourceStatusVendored,
		configuration.ManagedSourceStatusMissing,
		configuration.ManagedSourceStatusInvalid,
		configuration.ManagedSourceStatusConfigured,
		configuration.ManagedSourceSyncStatusVerified,
		configuration.ManagedSourceSyncStatusChecked,
		configuration.ManagedSourceSyncStatusSynced,
	}

	parts := make([]string, 0, len(counts))
	for _, status := range orderedStatuses {
		count, ok := counts[status]
		if ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, status))
		}
	}

	if len(parts) == 0 {
		return loc.Get("none")
	}

	return strings.Join(parts, ", ")
}

func writeConfigCheckIssue(outputWriter io.Writer, issue ConfigCheckIssue, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	lines := []string{
		"- " + output.CleanForTerminal(issue.Source),
		"  " + loc.Get("problem: %s", output.CleanForTerminal(issue.Message)),
		"  " + loc.Get("code: %s", output.CleanForTerminal(issue.Code)),
	}

	if issue.Hint != "" {
		lines = append(lines, "  "+loc.Get("hint: %s", output.CleanForTerminal(issue.Hint)))
	}

	lines = append(lines, "")

	for _, line := range lines {
		if err := writeConfigCheckLine(outputWriter, line); err != nil {
			return err
		}
	}

	return nil
}

func writeConfigCheckLine(outputWriter io.Writer, line string) error {
	if _, err := fmt.Fprintln(outputWriter, line); err != nil {
		return configOutputError(err, output.Msg("failed to write config check output"))
	}

	return nil
}
