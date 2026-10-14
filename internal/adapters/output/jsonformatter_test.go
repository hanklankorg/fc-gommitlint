// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"encoding/json"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func TestJSONFormatter_ValidReport(t *testing.T) {
	t.Parallel()

	commit := domain.Commit{
		Hash:        testCommitHashABC1234,
		Subject:     testCommitSubjectFeatNew,
		Message:     testCommitMessageFeatNew,
		Author:      testTestUserName,
		AuthorEmail: testUserEmail,
		CommitDate:  "2025-06-14T10:00:00Z",
	}

	validationErrors := []domain.ValidationError{
		{
			Rule:       testRuleTestRule,
			Diagnostic: testDiagnostic(testErrorCode),
			Message:    testValidationErrorMessage,
			Context: map[string]string{
				diagnostics.ContextExpected: testContextValueSomething,
				"found":                     testContextValueSomethingElse,
			},
		},
	}

	ruleResults := []domain.RuleReport{
		{
			Name:    testRuleTestRule,
			Status:  domain.StatusFailed,
			Message: testRuleFailedMessage,
			Errors:  validationErrors,
		},
		{
			Name:    testPassingRuleName,
			Status:  domain.StatusPassed,
			Message: testRulePassedMessage,
			Errors:  nil,
		},
	}

	commitReport := domain.CommitReport{
		Commit:      commit,
		Passed:      false,
		RuleResults: ruleResults,
	}

	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(commitReport).
		build()

	// Test JSON formatting
	result := JSON(report, nil)

	// Validate it's valid JSON
	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON")

	check.Equal(t, "2025-06-14T10:00:00Z", jsonData["timestamp"])
	summary, hasSummary := jsonData["summary"].(map[string]any)
	check.True(t, hasSummary)

	allPassed, ok := summary["allPassed"].(bool)
	check.True(t, ok)
	check.False(t, allPassed)
	check.InDelta(t, 1, summary["totalCommits"], 0.01)
	check.InDelta(t, 1, summary["validatedCommits"], 0.01)
	check.InDelta(t, 0, summary["ignoredCommits"], 0.01)
	check.InDelta(t, 0, summary["passedCommits"], 0.01)
	check.InDelta(t, 0, summary["repositoryFailedRules"], 0.01)

	ruleSummary, hasRuleSummary := summary["failedRules"].(map[string]any)
	check.True(t, hasRuleSummary, "should include rule summary")
	check.Len(t, ruleSummary, 1)
	check.InDelta(t, 1, ruleSummary[testRuleTestRule], 0.01)

	commitResults, hasCommitResults := jsonData["commits"].([]any)
	check.True(t, hasCommitResults)
	check.Len(t, commitResults, 1)

	commitData, isCommitMap := commitResults[0].(map[string]any)
	check.True(t, isCommitMap)
	check.Equal(t, testCommitHashABC1234, commitData["hash"])
	check.Equal(t, "commit", commitData["type"])
	check.Equal(t, testCommitSubjectFeatNew, commitData["subject"])
	passed, ok := commitData["passed"].(bool)
	check.True(t, ok)
	check.False(t, passed)
	check.Equal(t, "Test User <test@example.com>", commitData["author"])
	check.Equal(t, "2025-06-14T10:00:00Z", commitData["commitDate"])
	check.InDelta(t, 1, commitData["errorCount"], 0.01)

	jsonRuleResults, hasRuleResults := commitData["rules"].([]any)
	check.True(t, hasRuleResults)
	check.Len(t, jsonRuleResults, 2)

	failedRule, isRuleMap := jsonRuleResults[0].(map[string]any)
	check.True(t, isRuleMap)
	check.Equal(t, testRuleTestRule, failedRule["rule"])
	check.Equal(t, "failed", failedRule["status"])
	check.Equal(t, testRuleFailedMessage, failedRule["message"])

	errors, hasErrors := failedRule["errors"].([]any)
	check.True(t, hasErrors)
	check.Len(t, errors, 1)

	errorData, isErrorMap := errors[0].(map[string]any)
	check.True(t, isErrorMap)
	check.Equal(t, testValidationErrorMessage, errorData["message"])
	_, hasCode := errorData["code"]
	check.False(t, hasCode)

	diagnostic, hasDiagnostic := errorData["diagnostic"].(map[string]any)
	check.True(t, hasDiagnostic)
	check.Equal(t, string(testErrorCode), diagnostic["reason"])

	context, ok := errorData["context"].(map[string]any)
	check.True(t, ok)
	check.Equal(t, testContextValueSomething, context[diagnostics.ContextExpected])
	check.Equal(t, testContextValueSomethingElse, context["found"])
}

func TestJSONFormatter_IncludesCheckDiagnostics(t *testing.T) {
	t.Parallel()

	validationErr := domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Subject too long")
	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(domain.CommitReport{
			Commit: domain.Commit{Subject: "a very long subject"},
			RuleResults: []domain.RuleReport{{
				Name:   rulecatalog.Subject,
				Status: domain.StatusFailed,
				Checks: []string{testCheckIDGL101, testCheckIDGL102},
				Errors: []domain.ValidationError{validationErr},
			}},
		}).
		build()

	result := JSON(report, nil)

	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err)

	commitResults, hasCommitResults := jsonData["commits"].([]any)
	check.True(t, hasCommitResults)

	commitData, isCommitMap := commitResults[0].(map[string]any)
	check.True(t, isCommitMap)

	ruleResults, hasRuleResults := commitData["rules"].([]any)
	check.True(t, hasRuleResults)

	ruleData, isRuleMap := ruleResults[0].(map[string]any)
	check.True(t, isRuleMap)

	checksData, hasChecks := ruleData["checks"].([]any)
	check.True(t, hasChecks)
	check.Equal(t, testCheckIDGL102, checksData[1])

	errors, hasErrors := ruleData["errors"].([]any)
	check.True(t, hasErrors)

	errorData, isErrorMap := errors[0].(map[string]any)
	check.True(t, isErrorMap)

	_, hasRuleName := errorData["ruleName"]
	check.False(t, hasRuleName)

	diagnostic, hasDiagnostic := errorData["diagnostic"].(map[string]any)
	check.True(t, hasDiagnostic)
	check.Equal(t, testCheckIDGL102, diagnostic["id"])
	check.Equal(t, "subject_too_long", diagnostic["reason"])
	check.Equal(t, "message.subject", diagnostic["category"])
	check.Equal(t, "Subject must fit the configured length", diagnostic["summary"])
}

// TestJSONFormatter_SummaryCountsAndHelpAreExact pins the summary fields the
// other tests leave at zero or false. Every count is distinct, so a field
// wired to its neighbour cannot pass, and the whole object is compared so a
// dropped or renamed key is seen. The help text is the registered guidance
// for the reason, with the context values filled in.
func TestJSONFormatter_SummaryCountsAndHelpAreExact(t *testing.T) {
	t.Parallel()

	tooLong := domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Subject too long").
		WithContextMap(diagnostics.Context{
			diagnostics.ContextOver:        "7",
			diagnostics.ContextExpectedMax: "72",
		})

	report := newReportBuilder().
		at(formatterFixtureTime()).
		failingCommit(testCommitHashABC1234, "a very long subject", rulecatalog.Subject, tooLong).
		summary(func(summary *domain.ReportSummary) {
			summary.TotalCommits = 6
			summary.ValidatedCommits = 4
			summary.IgnoredCommits = 2
			summary.PassedCommits = 3
			summary.FailedCommits = 1
			summary.RepoFailedRules = 5
			summary.SkippedRepoRules = 8
			summary.IgnoreRepoRules = true
		}).
		build()

	var jsonData struct {
		Summary map[string]any `json:"summary"`
		Commits []struct {
			Rules []struct {
				Errors []struct {
					Help string `json:"help"`
				} `json:"errors"`
			} `json:"rules"`
		} `json:"commits"`
	}

	check.NoError(t, json.Unmarshal([]byte(JSON(report, nil)), &jsonData))

	check.Equal(t, map[string]any{
		"allPassed":              false,
		"failedRules":            map[string]any{rulecatalog.Subject: float64(1)},
		"repositoryRulesIgnored": true,
		"totalCommits":           float64(6),
		"validatedCommits":       float64(4),
		"ignoredCommits":         float64(2),
		"passedCommits":          float64(3),
		"failedCommits":          float64(1),
		"repositoryFailedRules":  float64(5),
		"skippedRepositoryRules": float64(8),
	}, jsonData.Summary)

	check.Len(t, jsonData.Commits, 1)
	check.Len(t, jsonData.Commits[0].Rules, 1)
	check.Len(t, jsonData.Commits[0].Rules[0].Errors, 1)
	check.Equal(t,
		"Shorten your commit message subject line by 7 characters — aim for under 72 characters",
		jsonData.Commits[0].Rules[0].Errors[0].Help)
}

func TestJSONFormatter_IgnoredCommits(t *testing.T) {
	t.Parallel()

	// Ignored-commit counts don't follow from added commit objects, so they are
	// set explicitly.
	report := newReportBuilder().
		at(formatterFixtureTime()).
		summary(func(s *domain.ReportSummary) {
			s.TotalCommits = 3
			s.ValidatedCommits = 2
			s.IgnoredCommits = 1
			s.PassedCommits = 2
		}).
		build()

	result := JSON(report, nil)

	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON")

	summary, hasSummary := jsonData["summary"].(map[string]any)
	check.True(t, hasSummary)
	check.InDelta(t, 3, summary["totalCommits"], 0.01)
	check.InDelta(t, 2, summary["validatedCommits"], 0.01)
	check.InDelta(t, 1, summary["ignoredCommits"], 0.01)
}

func TestJSONFormatter_EmptyReport(t *testing.T) {
	t.Parallel()

	// Test with empty report
	report := newReportBuilder().
		at(formatterFixtureTime()).
		build()

	result := JSON(report, nil)

	// Validate it's valid JSON
	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON for empty report")

	summary, hasSummary := jsonData["summary"].(map[string]any)
	check.True(t, hasSummary)

	allPassed, ok := summary["allPassed"].(bool)
	check.True(t, ok)
	check.True(t, allPassed)
	check.InDelta(t, 0, summary["totalCommits"], 0.01)
	check.InDelta(t, 0, summary["validatedCommits"], 0.01)
	check.InDelta(t, 0, summary["ignoredCommits"], 0.01)
	check.InDelta(t, 0, summary["passedCommits"], 0.01)
	check.InDelta(t, 0, summary["repositoryFailedRules"], 0.01)
}

func TestJSONFormatter_WithRepositoryResults(t *testing.T) {
	t.Parallel()

	// Test with repository-level validation results
	report := newReportBuilder().
		at(formatterFixtureTime()).
		repoRule(domain.RuleReport{
			Name:    testBranchRuleName,
			Status:  domain.StatusFailed,
			Message: testBranchRuleFailed,
			Errors: []domain.ValidationError{
				{
					Rule:       testBranchRuleName,
					Diagnostic: testDiagnostic(testValidationBranchCode),
					Message:    testBranchValidationFailed,
					Context: map[string]string{
						testValidationBranchKey: testValidationMainBranch,
					},
				},
			},
		}).
		build()

	result := JSON(report, nil)

	// Validate it's valid JSON
	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON")

	repository, exists := jsonData["repository"].(map[string]any)
	check.True(t, exists, "should include repository")

	summary, hasSummary := jsonData["summary"].(map[string]any)
	check.True(t, hasSummary)
	check.InDelta(t, 1, summary["repositoryFailedRules"], 0.01)

	repoArray, isRepoArray := repository["rules"].([]any)
	check.True(t, isRepoArray)
	check.Len(t, repoArray, 1)

	repoRule, isRepoRuleMap := repoArray[0].(map[string]any)
	check.True(t, isRepoRuleMap)
	check.Equal(t, testBranchRuleName, repoRule["rule"])
	check.Equal(t, "failed", repoRule["status"])

	// Verify errors array structure
	errors, hasErrors := repoRule["errors"].([]any)
	check.True(t, hasErrors, "should include errors array")
	check.Len(t, errors, 1)

	errorData, isErrorMap := errors[0].(map[string]any)
	check.True(t, isErrorMap)
	check.Equal(t, testBranchValidationFailed, errorData["message"])
	diagnostic, hasDiagnostic := errorData["diagnostic"].(map[string]any)
	check.True(t, hasDiagnostic)
	check.Equal(t, testValidationBranchCode, diagnostic["reason"])

	context, hasContext := errorData["context"].(map[string]any)
	check.True(t, hasContext, "should include context")
	check.Equal(t, testValidationMainBranch, context[testValidationBranchKey])
}

func TestJSONFormatter_WithWarnings(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		at(formatterFixtureTime()).
		warn(
			domain.Warning{Code: "invalid_ignore_commit", Message: "Invalid SHA in ignore_commits: bad", Context: map[string]string{"sha": "bad"}},
			domain.Warning{Code: "unknown_config", Message: "Unknown config key: foo"},
		).
		configSources("defaults").
		build()

	result := JSON(report, nil)

	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON")

	warnings, hasWarnings := jsonData["warnings"].([]any)
	check.True(t, hasWarnings, "should include warnings array")
	check.Len(t, warnings, 2)

	warningData, isWarningData := warnings[0].(map[string]any)
	check.True(t, isWarningData)
	check.Equal(t, "invalid_ignore_commit", warningData["code"])
	check.Equal(t, "'bad' is not a valid commit SHA in ignore_commits, skipping it", warningData["message"])

	secondWarning, isSecondWarning := warnings[1].(map[string]any)
	check.True(t, isSecondWarning)
	check.Equal(t, "Unknown config key: foo", secondWarning["message"])

	metadata, hasMetadata := jsonData["metadata"].(map[string]any)
	check.True(t, hasMetadata)

	configSources, hasConfigSources := metadata["configSources"].([]any)
	check.True(t, hasConfigSources, "should include configSources array")
	check.Len(t, configSources, 1)
	check.Equal(t, "defaults", configSources[0])
}

func TestJSONFormatter_CommitWithMissingFields(t *testing.T) {
	t.Parallel()

	// Test with commit that has missing optional fields
	commit := domain.Commit{
		Hash:    testCommitHashABC1234,
		Subject: testCommitSubjectTestCommit,
		Message: testCommitSubjectTestCommit,
		// Missing Author, AuthorEmail, CommitDate
	}

	commitReport := domain.CommitReport{
		Commit:      commit,
		Passed:      true,
		RuleResults: nil,
	}

	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(commitReport).
		build()

	result := JSON(report, nil)

	// Validate it's valid JSON
	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err, "should produce valid JSON")

	commitResults, hasCommitResults := jsonData["commits"].([]any)
	commitData, isCommitData := commitResults[0].(map[string]any)

	check.True(t, hasCommitResults)
	check.True(t, isCommitData)

	check.Equal(t, "Unknown", commitData["author"])
	check.Equal(t, "2025-06-14T10:00:00Z", commitData["commitDate"])
}

func TestJSONFormatter_EmptyCommitHash(t *testing.T) {
	t.Parallel()

	// Commits with an empty hash are reported as message-type by the formatter.
	report := newReportBuilder().
		at(formatterFixtureTime()).
		commit(domain.CommitReport{
			Commit: domain.Commit{Hash: "", Subject: testCommitSubjectTest, Message: testCommitSubjectTest},
			Passed: true,
		}).
		commit(domain.CommitReport{
			Commit: domain.Commit{Hash: testCommitHashABC1234, Subject: testCommitSubjectValid, Message: testCommitSubjectValid},
			Passed: true,
		}).
		build()

	result := JSON(report, nil)

	var jsonData map[string]any

	err := json.Unmarshal([]byte(result), &jsonData)
	check.NoError(t, err)

	commitResults, hasCommitResults := jsonData["commits"].([]any)

	check.True(t, hasCommitResults)
	check.Len(t, commitResults, 2, "should include commits with and without hash")

	messageCommit, isMessageCommit := commitResults[0].(map[string]any)
	check.True(t, isMessageCommit)
	check.Equal(t, "message", messageCommit["type"], "should mark as message type")
	check.Equal(t, testCommitSubjectTest, messageCommit["subject"])
	_, hasHash := messageCommit["hash"]
	check.False(t, hasHash, "message validation should not have hash")

	regularCommit, isRegularCommit := commitResults[1].(map[string]any)
	check.True(t, isRegularCommit)
	check.Equal(t, testCommitHashABC1234, regularCommit["hash"])
	check.Equal(t, "valid commit", regularCommit["subject"])
}
