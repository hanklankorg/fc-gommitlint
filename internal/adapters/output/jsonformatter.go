// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// jsonMarshalFailedCode is the error code used when JSON marshaling fails.
const jsonMarshalFailedCode = "json_marshal_failed"

const (
	jsonFieldMessage = "message"
	jsonFieldType    = "type"
	jsonTypeCommit   = "commit"
	jsonTypeMessage  = jsonFieldMessage
)

type jsonReport struct {
	Timestamp  string         `json:"timestamp"`
	Summary    jsonSummary    `json:"summary"`
	Metadata   jsonMetadata   `json:"metadata"`
	Warnings   []jsonWarning  `json:"warnings"`
	Commits    []jsonCommit   `json:"commits"`
	Repository jsonRepository `json:"repository"`
}

type jsonSummary struct {
	FailedRules            map[string]int `json:"failedRules"`
	AllPassed              bool           `json:"allPassed"`
	RepositoryRulesIgnored bool           `json:"repositoryRulesIgnored"`
	TotalCommits           int            `json:"totalCommits"`
	ValidatedCommits       int            `json:"validatedCommits"`
	IgnoredCommits         int            `json:"ignoredCommits"`
	PassedCommits          int            `json:"passedCommits"`
	FailedCommits          int            `json:"failedCommits"`
	RepositoryFailedRules  int            `json:"repositoryFailedRules"`
	SkippedRepositoryRules int            `json:"skippedRepositoryRules"`
}

type jsonMetadata struct {
	ConfigSources []string `json:"configSources"`
}

type jsonWarning struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Context map[string]string `json:"context"`
}

type jsonCommit struct {
	Hash       string     `json:"hash,omitempty"`
	Type       string     `json:"type"`
	Subject    string     `json:"subject"`
	Author     string     `json:"author"`
	CommitDate string     `json:"commitDate,omitempty"`
	Rules      []jsonRule `json:"rules"`
	Passed     bool       `json:"passed"`
	ErrorCount int        `json:"errorCount"`
}

type jsonRepository struct {
	Rules []jsonRule `json:"rules"`
}

type jsonRule struct {
	Rule    string      `json:"rule"`
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Checks  []string    `json:"checks"`
	Errors  []jsonError `json:"errors"`
}

type jsonError struct {
	Diagnostic jsonDiagnostic      `json:"diagnostic"`
	Message    string              `json:"message"`
	Context    diagnostics.Context `json:"context"`
	Help       string              `json:"help,omitempty"`
}

type jsonDiagnostic struct {
	ID       string `json:"id"`
	Reason   string `json:"reason"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

type jsonMarshalErrorOutput struct {
	Error     string `json:"error"`
	ErrorCode string `json:"errorCode"`
	Timestamp string `json:"timestamp"`
	Details   string `json:"details"`
}

// JSON formats a domain report as JSON.
//
// Backed by buildJSONPayload; callers that have a writer in hand can use
// JSONTo to avoid converting the encoded document to a string.
func JSON(report domain.Report, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	payload := buildJSONPayload(report, localizer)

	jsonBytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return jsonMarshalErrorFallback(err)
	}

	return string(jsonBytes)
}

// JSONTo writes a domain report as JSON to w.
//
// encoding/json buffers each encoded value internally before writing it, so
// this is a writer-based API rather than incremental document streaming.
// Output is byte-identical to JSON(report) modulo the trailing newline that
// Encoder adds (which is JSON-spec-compliant).
func JSONTo(writer io.Writer, report domain.Report, localizer Localizer) error {
	localizer = ensureLocalizer(localizer)

	payload := buildJSONPayload(report, localizer)

	enc := json.NewEncoder(writer)
	enc.SetIndent("", "  ")

	if err := enc.Encode(payload); err != nil {
		// Mirror the fallback shape from JSON(): emit a small, well-formed
		// JSON error object so machine consumers can still parse the stream.
		fallback := jsonMarshalErrorFallback(err)
		_, _ = io.WriteString(writer, fallback)

		return err
	}

	return nil
}

// buildJSONPayload constructs the JSON DTO once; shared by the string-returning
// and writer-backed paths so the schema doesn't drift between them.
func buildJSONPayload(report domain.Report, localizer Localizer) jsonReport {
	return jsonReport{
		Timestamp: report.Metadata.Timestamp.Format(time.RFC3339),
		Summary: jsonSummary{
			FailedRules:            report.Summary.FailedRules,
			AllPassed:              report.Summary.AllPassed,
			RepositoryRulesIgnored: report.Summary.IgnoreRepoRules,
			TotalCommits:           report.Summary.TotalCommits,
			ValidatedCommits:       report.Summary.ValidatedCommits,
			IgnoredCommits:         report.Summary.IgnoredCommits,
			PassedCommits:          report.Summary.PassedCommits,
			FailedCommits:          report.Summary.FailedCommits,
			RepositoryFailedRules:  report.Summary.RepoFailedRules,
			SkippedRepositoryRules: report.Summary.SkippedRepoRules,
		},
		Metadata: jsonMetadata{
			ConfigSources: nonNilStrings(report.Metadata.ConfigSources),
		},
		Warnings:   convertWarningsToJSON(report.Metadata.WarningDetails, localizer),
		Commits:    convertCommitsToJSON(report.Commits, report.Metadata.Timestamp, localizer),
		Repository: jsonRepository{Rules: convertRulesToJSON(report.Repository.RuleResults)},
	}
}

// jsonMarshalErrorFallback renders a small, well-formed JSON error object
// when the primary encode fails. Same shape as the previous JSON() inline
// fallback so existing consumers stay compatible.
func jsonMarshalErrorFallback(cause error) string {
	errorOutput := jsonMarshalErrorOutput{
		Error:     "failed to marshal JSON",
		ErrorCode: jsonMarshalFailedCode,
		Timestamp: time.Now().Format(time.RFC3339),
		Details:   cause.Error(),
	}
	if errorBytes, marshalErr := json.MarshalIndent(errorOutput, "", "  "); marshalErr == nil {
		return string(errorBytes)
	}

	return `{"error": "failed to marshal JSON", "details": "unknown error"}`
}

func convertWarningsToJSON(warnings []domain.Warning, localizer Localizer) []jsonWarning {
	if len(warnings) == 0 {
		return []jsonWarning{}
	}

	results := make([]jsonWarning, len(warnings))
	for i, warning := range warnings {
		results[i] = jsonWarning{
			Code:    string(warning.Code),
			Message: FormatWarning(warning, localizer),
			Context: maps.Clone(warning.Context),
		}
	}

	return results
}

// convertCommitsToJSON transforms commit validation reports into JSON DTOs.
func convertCommitsToJSON(commits []domain.CommitReport, fallbackTimestamp time.Time, localizer Localizer) []jsonCommit {
	results := make([]jsonCommit, 0, len(commits))

	for _, commitReport := range commits {
		commit := jsonCommit{
			Type:       jsonTypeMessage,
			Subject:    commitReport.Commit.Subject,
			Passed:     commitReport.Passed,
			Rules:      convertRulesToJSON(commitReport.RuleResults),
			ErrorCount: countErrors(commitReport.RuleResults),
		}

		if commitReport.Commit.Hash != "" {
			commit.Hash = commitReport.Commit.Hash
			commit.Type = jsonTypeCommit
		}

		if commitReport.Commit.CommitDate != "" {
			commit.CommitDate = commitReport.Commit.CommitDate
		} else if !fallbackTimestamp.IsZero() {
			commit.CommitDate = fallbackTimestamp.Format(time.RFC3339)
		}

		if commitReport.Commit.Author != "" {
			if commitReport.Commit.AuthorEmail != "" {
				commit.Author = fmt.Sprintf("%s <%s>", commitReport.Commit.Author, commitReport.Commit.AuthorEmail)
			} else {
				commit.Author = commitReport.Commit.Author
			}
		} else {
			commit.Author = localizer.Get("Unknown")
		}

		results = append(results, commit)
	}

	return results
}

// convertRulesToJSON transforms rule validation reports into JSON DTOs.
func convertRulesToJSON(rules []domain.RuleReport) []jsonRule {
	if len(rules) == 0 {
		return []jsonRule{}
	}

	results := make([]jsonRule, len(rules))

	for i, ruleReport := range rules {
		results[i] = convertRuleReportToJSON(ruleReport)
	}

	return results
}

func convertRuleReportToJSON(ruleReport domain.RuleReport) jsonRule {
	return jsonRule{
		Rule:    ruleReport.Name,
		Status:  string(ruleReport.Status),
		Message: ruleReport.Message,
		Checks:  nonNilStrings(ruleReport.Checks),
		Errors:  convertErrorsToJSON(ruleReport.Errors),
	}
}

func convertErrorsToJSON(validationErrors []domain.ValidationError) []jsonError {
	if len(validationErrors) == 0 {
		return []jsonError{}
	}

	results := make([]jsonError, len(validationErrors))
	for idx, err := range validationErrors {
		result := jsonError{
			Diagnostic: convertDiagnosticToJSON(err.Diagnostic),
			Message:    err.Message,
			Context:    maps.Clone(err.Context),
		}

		if helpMsg, ok := LookupHelpMessage(err.Diagnostic.Reason, err.Context); ok {
			result.Help = helpMsg.String()
		}

		results[idx] = result
	}

	return results
}

func convertDiagnosticToJSON(diagnostic diagnostics.Diagnostic) jsonDiagnostic {
	return jsonDiagnostic{
		ID:       string(diagnostic.ID),
		Reason:   string(diagnostic.Reason),
		Category: string(diagnostic.Category),
		Summary:  diagnostic.Summary,
	}
}

func nonNilStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	result := make([]string, len(values))
	copy(result, values)

	return result
}

func countErrors(rules []domain.RuleReport) int {
	total := 0

	for _, rule := range rules {
		if rule.Status == domain.StatusFailed {
			total += len(rule.Errors)
		}
	}

	return total
}
