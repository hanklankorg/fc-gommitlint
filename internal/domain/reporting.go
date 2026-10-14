// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// ReportBuildContext holds the rule context needed to build a report.
// It carries only the four fields BuildReport actually requires, avoiding
// a hard dependency on the full ValidationContext.
type ReportBuildContext struct {
	CommitRules   []CommitRule
	RepoRules     []RepositoryRule
	HasRepository bool // true when a live repository is available
	SkipRepoRules bool // true when repo rules were explicitly bypassed
}

// ReportInput bundles the metadata passed to BuildReport.
type ReportInput struct {
	Timestamp      time.Time
	WarningDetails []Warning
	ConfigSources  []string
	RepoRuleInfo   map[string]string
	// MessageFile is the commit message file path when validating with
	// --message-file, empty otherwise. Lets location-aware formatters
	// (SARIF) anchor findings to a real file.
	MessageFile string
}

// Misspelling a detected spelling error.
type Misspelling struct {
	Word       string
	Position   int
	Suggestion string
}

// Report a validation report.
type Report struct {
	Summary    ReportSummary
	Commits    []CommitReport
	Repository RepositoryReport
	Metadata   ReportMetadata
}

// ReportSummary contains high-level validation statistics.
type ReportSummary struct {
	TotalCommits     int
	ValidatedCommits int
	IgnoredCommits   int
	PassedCommits    int
	FailedCommits    int
	RepoFailedRules  int
	AllPassed        bool
	FailedRules      map[string]int // Rule name -> failure count
	SkippedRepoRules int            // Number of repository rules skipped (no repo or ignored)
	IgnoreRepoRules  bool           // Whether repo rules were explicitly ignored via flag
}

// CommitReport contains formatted information about a single commit validation.
type CommitReport struct {
	Commit      Commit
	RuleResults []RuleReport
	Passed      bool
}

// RuleReport contains formatted rule validation information.
type RuleReport struct {
	Name    string
	Status  ValidationStatus
	Checks  []string
	Errors  []ValidationError
	Message string // Formatted message for display
}

// RepositoryReport contains repository-level validation results.
type RepositoryReport struct {
	RuleResults []RuleReport
}

// ReportMetadata contains report generation context.
type ReportMetadata struct {
	Timestamp      time.Time
	WarningDetails []Warning
	ConfigSources  []string
	// MessageFile is the commit message file path when validating with
	// --message-file, empty otherwise.
	MessageFile string
}

// BuildReport creates a report showing all executed rules (both passed and failed).
// Pass nil for ignoreInfo if no commits were ignored.
// Pure function: Timestamp inside input is used as-is for testability.
func BuildReport(commitResults []ValidationResult, repoErrors []ValidationError,
	rbc ReportBuildContext, ignoreInfo *CommitIgnoreInfo, input ReportInput,
) Report {
	// Use default ignore info if not provided
	info := CommitIgnoreInfo{
		TotalCommits:   len(commitResults),
		ValidatedCount: len(commitResults),
		IgnoredCount:   0,
	}

	if ignoreInfo != nil {
		info = *ignoreInfo
	}

	// Calculate skipped repo rules
	skippedRepoRules := 0
	if !rbc.HasRepository || rbc.SkipRepoRules {
		skippedRepoRules = len(rbc.RepoRules)
	}

	return Report{
		Summary:    buildSummary(commitResults, repoErrors, info, skippedRepoRules, rbc.SkipRepoRules),
		Commits:    buildCommitReports(commitResults, rbc.CommitRules),
		Repository: buildRepositoryReport(repoErrors, rbc.RepoRules, input.RepoRuleInfo),
		Metadata:   buildMetadata(input),
	}
}

// buildSummary creates report summary from validation results.
func buildSummary(commitResults []ValidationResult, repoErrors []ValidationError, ignoreInfo CommitIgnoreInfo, skippedRepoRules int, ignoreRepoRules bool) ReportSummary {
	validatedCommits := len(commitResults)
	passedCommits := 0
	failedRules := make(map[string]int)

	// Count passed commits and collect failed rules
	for _, result := range commitResults {
		if !result.HasFailures() {
			passedCommits++
		}

		// Count rule failures
		for _, validationErr := range result.Errors {
			failedRules[validationErr.Rule]++
		}
	}

	// Count repository rule failures
	repoFailedRules := 0

	if len(repoErrors) > 0 {
		repoRuleSet := make(map[string]struct{}, len(repoErrors))
		for _, repoErr := range repoErrors {
			failedRules[repoErr.Rule]++
			if _, exists := repoRuleSet[repoErr.Rule]; !exists {
				repoRuleSet[repoErr.Rule] = struct{}{}
				repoFailedRules++
			}
		}
	}

	failedCommits := validatedCommits - passedCommits
	allPassed := failedCommits == 0 && len(repoErrors) == 0

	return ReportSummary{
		TotalCommits:     ignoreInfo.TotalCommits,
		ValidatedCommits: ignoreInfo.ValidatedCount,
		IgnoredCommits:   ignoreInfo.IgnoredCount,
		PassedCommits:    passedCommits,
		FailedCommits:    failedCommits,
		RepoFailedRules:  repoFailedRules,
		AllPassed:        allPassed,
		FailedRules:      failedRules,
		SkippedRepoRules: skippedRepoRules,
		IgnoreRepoRules:  ignoreRepoRules,
	}
}

// buildCommitReports creates commit reports showing all executed rules.
// Commits are expected to be pre-sorted in chronological order (oldest first) by the git adapter.
func buildCommitReports(commitResults []ValidationResult, commitRules []CommitRule) []CommitReport {
	reports := make([]CommitReport, len(commitResults))

	for i, result := range commitResults {
		reports[i] = CommitReport{
			Commit:      result.Commit,
			RuleResults: buildRuleReports(result, commitRules),
			Passed:      !result.HasFailures(),
		}
	}

	return reports
}

// buildRepositoryReport creates repository report showing all executed rules.
func buildRepositoryReport(repoErrors []ValidationError, repoRules []RepositoryRule, repoRuleInfo map[string]string) RepositoryReport {
	return RepositoryReport{
		RuleResults: buildRepositoryRuleReports(repoErrors, repoRules, repoRuleInfo),
	}
}

// buildRuleReports creates rule reports showing all executed commit rules.
func buildRuleReports(result ValidationResult, commitRules []CommitRule) []RuleReport {
	errorsByRule := groupErrorsByRule(result.Errors)
	suppressConventionalOverlap(errorsByRule)

	ruleNames := extractRuleNames(commitRules)
	onlyPseudoErrors := hasOnlyPseudoRuleErrors(errorsByRule, ruleNames)

	reports := make([]RuleReport, 0, len(commitRules)+len(errorsByRule))

	// First, add errors from pseudo-rules (like FixupCommit) that aren't in commitRules
	reports = append(reports, buildPseudoRuleReports(errorsByRule, ruleNames)...)

	if onlyPseudoErrors {
		return reports
	}

	// Add reports for real rules
	reports = append(reports, buildCommitRuleReports(commitRules, errorsByRule)...)

	return reports
}

// groupErrorsByRule creates a map of rule name to its errors.
func groupErrorsByRule(errors []ValidationError) map[string][]ValidationError {
	errorsByRule := make(map[string][]ValidationError)
	for _, validationErr := range errors {
		errorsByRule[validationErr.Rule] = append(errorsByRule[validationErr.Rule], validationErr)
	}

	return errorsByRule
}

// suppressConventionalOverlap removes conventional-format errors that the Subject
// rule emits when it's configured as conventional-aware — those overlap with the
// ConventionalCommit rule's own errors and surface the same root cause twice in
// the report. Keep ConventionalCommit's verdict authoritative; drop Subject's
// duplicate row for the same Code.
//
// Mutates the map in place: removes the suppressed errors from Subject's slice,
// and removes Subject's entry entirely if no errors remain after suppression.
func suppressConventionalOverlap(errorsByRule map[string][]ValidationError) {
	conventional, hasConventional := errorsByRule[rulecatalog.Conventional]

	subject, hasSubject := errorsByRule[rulecatalog.Subject]
	if !hasConventional || !hasSubject {
		return
	}

	conventionalReasons := make(map[diagnostics.Reason]bool, len(conventional))
	for _, validationErr := range conventional {
		switch validationErr.Diagnostic.Reason { //nolint:exhaustive // only Conventional* reasons drive de-duplication; others are intentionally ignored.
		case diagnostics.ReasonInvalidConventionalFormat,
			diagnostics.ReasonInvalidConventionalType,
			diagnostics.ReasonInvalidConventionalScope,
			diagnostics.ReasonMissingConventionalSubject,
			diagnostics.ReasonEmptyConventionalDesc,
			diagnostics.ReasonConventionalDescTooLong:
			conventionalReasons[validationErr.Diagnostic.Reason] = true
		}
	}

	if len(conventionalReasons) == 0 {
		return
	}

	filtered := subject[:0]
	for _, validationErr := range subject {
		if conventionalReasons[validationErr.Diagnostic.Reason] {
			continue
		}

		filtered = append(filtered, validationErr)
	}

	if len(filtered) == 0 {
		delete(errorsByRule, rulecatalog.Subject)

		return
	}

	errorsByRule[rulecatalog.Subject] = filtered
}

// extractRuleNames creates a set of rule names for quick lookup.
func extractRuleNames(commitRules []CommitRule) map[string]bool {
	names := make(map[string]bool, len(commitRules))
	for _, rule := range commitRules {
		names[rule.Name()] = true
	}

	return names
}

// hasOnlyPseudoRuleErrors returns true when only pseudo-rule errors exist (early guard failed),
// meaning real rule results should not be included in the report.
func hasOnlyPseudoRuleErrors(errorsByRule map[string][]ValidationError, ruleNames map[string]bool) bool {
	hasPseudoRuleErrors := false
	hasRealRuleErrors := false

	for ruleName := range errorsByRule {
		if ruleNames[ruleName] {
			hasRealRuleErrors = true
		} else {
			hasPseudoRuleErrors = true
		}
	}

	return hasPseudoRuleErrors && !hasRealRuleErrors
}

// buildPseudoRuleReports creates reports for pseudo-rules (early guards like FixupCommit).
func buildPseudoRuleReports(errorsByRule map[string][]ValidationError, ruleNames map[string]bool) []RuleReport {
	var reports []RuleReport

	for ruleName, errs := range errorsByRule {
		if !ruleNames[ruleName] {
			reports = append(reports, RuleReport{
				Name:    ruleName,
				Status:  StatusFailed,
				Errors:  errs,
				Message: joinErrorMessages(errs),
			})
		}
	}

	return reports
}

// newRuleReport creates a RuleReport for a rule.
// When errs is non-empty the report is marked failed; otherwise passed with info (may be empty).
func newRuleReport(name string, errs []ValidationError, info string) RuleReport {
	if len(errs) > 0 {
		return RuleReport{
			Name:    name,
			Status:  StatusFailed,
			Errors:  errs,
			Message: joinErrorMessages(errs),
		}
	}

	return RuleReport{
		Name:    name,
		Status:  StatusPassed,
		Message: info,
	}
}

// buildCommitRuleReports creates reports for commit rules.
func buildCommitRuleReports(commitRules []CommitRule, errorsByRule map[string][]ValidationError) []RuleReport {
	reports := make([]RuleReport, 0, len(commitRules))

	for _, rule := range commitRules {
		ruleName := rule.Name()
		activeChecks := activeChecks(rule)
		info := strings.Join(activeChecks, ", ")
		report := newRuleReport(ruleName, errorsByRule[ruleName], info)
		report.Checks = activeChecks
		reports = append(reports, report)
	}

	return reports
}

// activeChecks returns active sub-check labels if the rule implements CheckDescriber.
func activeChecks(rule CommitRule) []string {
	describer, ok := rule.(CheckDescriber)
	if !ok {
		return nil
	}

	return describer.ActiveChecks()
}

// joinErrorMessages concatenates error messages with semicolons.
func joinErrorMessages(errs []ValidationError) string {
	var messageBuilder strings.Builder

	for i, err := range errs {
		if i > 0 {
			messageBuilder.WriteString("; ")
		}

		messageBuilder.WriteString(err.Message)
	}

	return messageBuilder.String()
}

// buildRepositoryRuleReports creates rule reports showing all executed repository rules.
func buildRepositoryRuleReports(repoErrors []ValidationError, repoRules []RepositoryRule, repoRuleInfo map[string]string) []RuleReport {
	errorsByRule := groupErrorsByRule(repoErrors)

	reports := make([]RuleReport, 0, len(repoRules))

	for _, rule := range repoRules {
		reports = append(reports, buildRepositoryRuleReport(rule, errorsByRule, repoRuleInfo))
	}

	return reports
}

func buildRepositoryRuleReport(rule RepositoryRule, errorsByRule map[string][]ValidationError, repoRuleInfo map[string]string) RuleReport {
	ruleName := rule.Name()
	info := ""

	if ruleInfo, ok := repoRuleInfo[ruleName]; ok {
		info = ruleInfo
	}

	report := newRuleReport(ruleName, errorsByRule[ruleName], info)
	report.Checks = activeRepositoryChecks(rule)

	return report
}

func activeRepositoryChecks(rule RepositoryRule) []string {
	describer, ok := rule.(CheckDescriber)
	if !ok {
		return nil
	}

	return describer.ActiveChecks()
}

// buildMetadata creates report metadata from a ReportInput.
func buildMetadata(input ReportInput) ReportMetadata {
	return ReportMetadata{
		Timestamp:      input.Timestamp,
		WarningDetails: input.WarningDetails,
		ConfigSources:  input.ConfigSources,
		MessageFile:    input.MessageFile,
	}
}
