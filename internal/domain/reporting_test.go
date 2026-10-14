// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestBuildReport(t *testing.T) {
	t.Parallel()

	commitResults := []ValidationResult{
		{
			Commit: Commit{Hash: testHashABC123, Subject: testSubjectFeatAddFeature, CommitDate: testCommitDateJan1},
			Errors: []ValidationError{},
		},
		{
			Commit: Commit{Hash: "def456", Subject: "invalid subject", CommitDate: testCommitDateJan2},
			Errors: []ValidationError{
				New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Subject too long"),
			},
		},
	}

	repoErrors := []ValidationError{
		New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits ahead"),
	}

	commitRules := []CommitRule{
		createPassingRule(diagnostics.ContextSubject),
		createPassingRule("conventional"),
	}

	repoRules := []RepositoryRule{
		createPassingRepoRule("branchahead"),
	}

	testTime := time.Now()
	rbc := ReportBuildContext{
		CommitRules:   commitRules,
		RepoRules:     repoRules,
		HasRepository: false,
		SkipRepoRules: false,
	}
	report := BuildReport(commitResults, repoErrors, rbc, nil, ReportInput{
		Timestamp:      testTime,
		WarningDetails: []Warning{{Code: testString, Message: testWarningMessage}},
		ConfigSources:  []string{testConfigSourceDefaults},
	})
	check.Equal(t, []Warning{{Code: testString, Message: testWarningMessage}}, report.Metadata.WarningDetails)
	check.Equal(t, []string{testConfigSourceDefaults}, report.Metadata.ConfigSources)

	// Test summary
	check.Equal(t, 2, report.Summary.TotalCommits)
	check.Equal(t, 1, report.Summary.PassedCommits)
	check.Equal(t, 1, report.Summary.FailedCommits)
	check.False(t, report.Summary.AllPassed)
	check.Len(t, report.Summary.FailedRules, 2) // subject + branchahead
	check.Equal(t, 1, report.Summary.FailedRules[diagnostics.ContextSubject])
	check.Equal(t, 1, report.Summary.FailedRules["branchahead"])

	// Commits keep their input order (sorting is the git adapter's job)
	check.Len(t, report.Commits, 2)
	check.Equal(t, testHashABC123, report.Commits[0].Commit.Hash) // 2023-01-01 (older)
	check.Equal(t, "def456", report.Commits[1].Commit.Hash)       // 2023-01-02 (newer)
	check.True(t, report.Commits[0].Passed)
	check.False(t, report.Commits[1].Passed)

	// Test repository report
	check.Len(t, report.Repository.RuleResults, 1)
	check.Equal(t, "branchahead", report.Repository.RuleResults[0].Name)
	check.Equal(t, StatusFailed, report.Repository.RuleResults[0].Status)

	// Test metadata
	check.Equal(t, testTime, report.Metadata.Timestamp)
}

func TestBuildSummary(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name                string
		commitResults       []ValidationResult
		repoErrors          []ValidationError
		wantPassed          int
		wantFailed          int
		wantRepoFailedRules int
		wantAllPass         bool
		wantRuleFailures    map[string]int
	}{
		{
			name: "All commits pass, no repo errors",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
				{Errors: []ValidationError{}},
			},
			repoErrors:          []ValidationError{},
			wantPassed:          2,
			wantFailed:          0,
			wantRepoFailedRules: 0,
			wantAllPass:         true,
			wantRuleFailures:    map[string]int{},
		},
		{
			name: "Some commits fail",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
				{Errors: []ValidationError{
					New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Too long"),
					New("conventional", diagnostics.ReasonInvalidFormat, "Invalid"),
				}},
			},
			repoErrors:          []ValidationError{},
			wantPassed:          1,
			wantFailed:          1,
			wantRepoFailedRules: 0,
			wantAllPass:         false,
			wantRuleFailures: map[string]int{
				diagnostics.ContextSubject: 1,
				"conventional":             1,
			},
		},
		{
			name: "No errors",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
			},
			repoErrors:          []ValidationError{},
			wantPassed:          1,
			wantFailed:          0,
			wantRepoFailedRules: 0,
			wantAllPass:         true,
			wantRuleFailures:    map[string]int{},
		},
		{
			name: "Repository errors",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
			},
			repoErrors: []ValidationError{
				New("branchahead", diagnostics.ReasonTooManyCommits, "Too many"),
			},
			wantPassed:          1,
			wantFailed:          0,
			wantRepoFailedRules: 1,
			wantAllPass:         false,
			wantRuleFailures: map[string]int{
				"branchahead": 1,
			},
		},
		{
			name: "Two errors from one repository rule count the rule once",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
			},
			repoErrors: []ValidationError{
				New("branchahead", diagnostics.ReasonTooManyCommits, "Too many"),
				New("branchahead", diagnostics.ReasonInvalidRepo, "Invalid repo"),
			},
			wantPassed:          1,
			wantFailed:          0,
			wantRepoFailedRules: 1,
			wantAllPass:         false,
			wantRuleFailures: map[string]int{
				"branchahead": 2,
			},
		},
		{
			name: "Errors from two repository rules count both rules",
			commitResults: []ValidationResult{
				{Errors: []ValidationError{}},
			},
			repoErrors: []ValidationError{
				New("branchahead", diagnostics.ReasonTooManyCommits, "Too many"),
				New("linearhistory", diagnostics.ReasonMergeCommitFound, "Merge"),
				New("branchahead", diagnostics.ReasonInvalidRepo, "Invalid repo"),
			},
			wantPassed:          1,
			wantFailed:          0,
			wantRepoFailedRules: 2,
			wantAllPass:         false,
			wantRuleFailures: map[string]int{
				"branchahead":   2,
				"linearhistory": 1,
			},
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ignoreInfo := CommitIgnoreInfo{
				TotalCommits:   len(testCase.commitResults),
				ValidatedCount: len(testCase.commitResults),
				IgnoredCount:   0,
			}
			summary := buildSummary(testCase.commitResults, testCase.repoErrors, ignoreInfo, 0, false)

			check.Equal(t, len(testCase.commitResults), summary.TotalCommits)
			check.Equal(t, testCase.wantPassed, summary.PassedCommits)
			check.Equal(t, testCase.wantFailed, summary.FailedCommits)
			check.Equal(t, testCase.wantRepoFailedRules, summary.RepoFailedRules)
			check.Equal(t, testCase.wantAllPass, summary.AllPassed)
			check.Equal(t, testCase.wantRuleFailures, summary.FailedRules)
		})
	}
}

func TestBuildCommitReports(t *testing.T) {
	t.Parallel()

	// Input is pre-sorted oldest-first (sorting is done by git adapter)
	commitResults := []ValidationResult{
		{
			Commit: Commit{Hash: "older", CommitDate: testCommitDateJan1},
			Errors: []ValidationError{},
		},
		{
			Commit: Commit{Hash: "newer", CommitDate: testCommitDateJan2},
			Errors: []ValidationError{
				New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Too long"),
			},
		},
	}

	commitRules := []CommitRule{
		createPassingRule(diagnostics.ContextSubject),
		createPassingRule("conventional"),
	}

	reports := buildCommitReports(commitResults, commitRules)

	check.Len(t, reports, 2)

	// Order should be preserved (oldest first)
	check.Equal(t, "older", reports[0].Commit.Hash)
	check.Equal(t, "newer", reports[1].Commit.Hash)

	check.True(t, reports[0].Passed)
	check.Len(t, reports[0].RuleResults, 2)
	check.Equal(t, StatusPassed, reports[0].RuleResults[0].Status)
	check.Equal(t, StatusPassed, reports[0].RuleResults[1].Status)

	check.False(t, reports[1].Passed)
	check.Len(t, reports[1].RuleResults, 2)

	// Find the failed rule result
	var subjectResult, conventionalResult *RuleReport

	for i := range reports[1].RuleResults {
		switch reports[1].RuleResults[i].Name {
		case diagnostics.ContextSubject:
			subjectResult = &reports[1].RuleResults[i]
		case "conventional":
			conventionalResult = &reports[1].RuleResults[i]
		}
	}

	check.NotNil(t, subjectResult)
	check.NotNil(t, conventionalResult)
	check.Equal(t, StatusFailed, subjectResult.Status)
	check.Equal(t, "Too long", subjectResult.Message)
	check.Equal(t, StatusPassed, conventionalResult.Status)
}

func TestBuildRuleReports(t *testing.T) {
	t.Parallel()

	result := ValidationResult{
		Errors: []ValidationError{
			New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Too long"),
			New(diagnostics.ContextSubject, diagnostics.ReasonSubjectCase, "Wrong case"),
			New("conventional", diagnostics.ReasonInvalidFormat, "Invalid format"),
		},
	}

	commitRules := []CommitRule{
		createPassingRule(diagnostics.ContextSubject),
		createPassingRule("conventional"),
		createPassingRule("spell"),
	}

	reports := buildRuleReports(result, commitRules)

	check.Len(t, reports, 3)

	// Find reports by name
	ruleMap := make(map[string]RuleReport)
	for _, report := range reports {
		ruleMap[report.Name] = report
	}

	// Subject rule should have failed with multiple errors
	subjectReport := ruleMap[diagnostics.ContextSubject]
	check.Equal(t, StatusFailed, subjectReport.Status)
	check.Len(t, subjectReport.Errors, 2)
	check.Contains(t, subjectReport.Message, "Too long")
	check.Contains(t, subjectReport.Message, "Wrong case")

	// Conventional rule should have failed with one error
	conventionalReport := ruleMap["conventional"]
	check.Equal(t, StatusFailed, conventionalReport.Status)
	check.Len(t, conventionalReport.Errors, 1)
	check.Equal(t, "Invalid format", conventionalReport.Message)

	// Spell rule should have passed (no errors)
	spellReport := ruleMap["spell"]
	check.Equal(t, StatusPassed, spellReport.Status)
	check.Empty(t, spellReport.Errors)
	check.Empty(t, spellReport.Message)
}

func TestBuildRepositoryReport(t *testing.T) {
	t.Parallel()

	repoErrors := []ValidationError{
		New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits"),
	}

	repoRules := []RepositoryRule{
		createPassingRepoRule("branchahead"),
	}

	report := buildRepositoryReport(repoErrors, repoRules, nil)

	check.Len(t, report.RuleResults, 1)
	check.Equal(t, "branchahead", report.RuleResults[0].Name)
	check.Equal(t, StatusFailed, report.RuleResults[0].Status)
	check.Equal(t, "Too many commits", report.RuleResults[0].Message)
	check.Len(t, report.RuleResults[0].Errors, 1)
}

func TestBuildRepositoryRuleReports(t *testing.T) {
	t.Parallel()

	repoErrors := []ValidationError{
		New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits"),
		New("branchahead", diagnostics.ReasonInvalidRepo, "Invalid repo"),
	}

	repoRules := []RepositoryRule{
		createPassingRepoRule("branchahead"),
		createPassingRepoRule("passing_rule"),
	}

	reports := buildRepositoryRuleReports(repoErrors, repoRules, nil)

	check.Len(t, reports, 2)

	// Find reports by name
	ruleMap := make(map[string]RuleReport)
	for _, report := range reports {
		ruleMap[report.Name] = report
	}

	// Branchahead rule should have failed with multiple errors
	branchReport := ruleMap["branchahead"]
	check.Equal(t, StatusFailed, branchReport.Status)
	check.Len(t, branchReport.Errors, 2)
	check.Contains(t, branchReport.Message, "Too many commits")
	check.Contains(t, branchReport.Message, "Invalid repo")

	// Passing rule should have passed
	passingReport := ruleMap["passing_rule"]
	check.Equal(t, StatusPassed, passingReport.Status)
	check.Empty(t, passingReport.Errors)
	check.Empty(t, passingReport.Message)
}

func TestBuildRepositoryRuleReports_WithInfo(t *testing.T) {
	t.Parallel()

	repoRules := []RepositoryRule{
		createPassingRepoRule("branchahead"),
	}

	repoRuleInfo := map[string]string{
		"branchahead": "3/5",
	}

	reports := buildRepositoryRuleReports(nil, repoRules, repoRuleInfo)

	check.Len(t, reports, 1)
	check.Equal(t, StatusPassed, reports[0].Status)
	check.Equal(t, "3/5", reports[0].Message)
}

func TestBuildMetadata(t *testing.T) {
	t.Parallel()

	testTime := time.Now()
	metadata := buildMetadata(ReportInput{
		Timestamp:      testTime,
		WarningDetails: []Warning{{Code: "test", Message: "warn1"}},
		ConfigSources:  []string{testConfigSourceDefaults},
	})

	check.Equal(t, []Warning{{Code: "test", Message: "warn1"}}, metadata.WarningDetails)
	check.Equal(t, testTime, metadata.Timestamp)
}

func TestCommitReports_PreservesInputOrder(t *testing.T) {
	t.Parallel()

	// Input order must survive even when it disagrees with date order —
	// sorting is the git adapter's job, so the dates here deliberately
	// do not ascend.
	commitResults := []ValidationResult{
		{Commit: Commit{Hash: "first", CommitDate: "2023-01-03T00:00:00Z"}},
		{Commit: Commit{Hash: "second", CommitDate: testCommitDateJan1}},
		{Commit: Commit{Hash: "third", CommitDate: testCommitDateJan2}},
	}

	commitRules := []CommitRule{}

	reports := buildCommitReports(commitResults, commitRules)

	check.Len(t, reports, 3)
	check.Equal(t, "first", reports[0].Commit.Hash)
	check.Equal(t, "second", reports[1].Commit.Hash)
	check.Equal(t, "third", reports[2].Commit.Hash)
}

func TestBuildCommitRuleReports_WithActiveChecks(t *testing.T) {
	t.Parallel()

	commitRules := []CommitRule{
		createPassingRuleWithChecks(diagnostics.ContextSubject, []string{testCheckLength, testCheckCase, "suffix"}),
		createPassingRule("spell"),
		createPassingRuleWithChecks("signature", []string{testCheckPresence, "format"}),
	}

	// No errors — all rules pass
	errorsByRule := map[string][]ValidationError{}

	reports := buildCommitRuleReports(commitRules, errorsByRule)

	check.Len(t, reports, 3)

	// Subject: should have checks in Message
	check.Equal(t, diagnostics.ContextSubject, reports[0].Name)
	check.Equal(t, StatusPassed, reports[0].Status)
	check.Equal(t, "length, case, suffix", reports[0].Message)

	// Spell: no CheckDescriber, empty message
	check.Equal(t, "spell", reports[1].Name)
	check.Equal(t, StatusPassed, reports[1].Status)
	check.Empty(t, reports[1].Message)

	// Signature: should have checks in Message
	check.Equal(t, "signature", reports[2].Name)
	check.Equal(t, StatusPassed, reports[2].Status)
	check.Equal(t, "presence, format", reports[2].Message)
}

func TestBuildCommitRuleReports_FailedRuleIgnoresChecks(t *testing.T) {
	t.Parallel()

	commitRules := []CommitRule{
		createPassingRuleWithChecks(diagnostics.ContextSubject, []string{testCheckLength, testCheckCase}),
	}

	// Subject has errors — should show error message, not checks
	errorsByRule := map[string][]ValidationError{
		diagnostics.ContextSubject: {New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Subject too long")},
	}

	reports := buildCommitRuleReports(commitRules, errorsByRule)

	check.Len(t, reports, 1)
	check.Equal(t, StatusFailed, reports[0].Status)
	check.Equal(t, "Subject too long", reports[0].Message)
}

func TestReport_InputNotModified(t *testing.T) {
	t.Parallel()

	// Test that building reports doesn't modify input data
	originalResults := []ValidationResult{
		{
			Commit: Commit{Hash: testHashABC123},
			Errors: []ValidationError{
				New(diagnostics.ContextSubject, diagnostics.ReasonSubjectTooLong, "Too long"),
			},
		},
	}

	originalErrors := []ValidationError{
		New("branchahead", diagnostics.ReasonTooManyCommits, "Too many"),
	}

	// Copy for comparison
	resultsCopy := make([]ValidationResult, len(originalResults))
	copy(resultsCopy, originalResults)

	errorsCopy := make([]ValidationError, len(originalErrors))
	copy(errorsCopy, originalErrors)

	testTime := time.Now()
	rbc := ReportBuildContext{
		CommitRules:   []CommitRule{},
		RepoRules:     []RepositoryRule{},
		HasRepository: false,
		SkipRepoRules: false,
	}
	BuildReport(originalResults, originalErrors, rbc, nil, ReportInput{Timestamp: testTime})

	// Original data should be unchanged
	check.Equal(t, resultsCopy, originalResults)
	check.Equal(t, errorsCopy, originalErrors)
}

func TestBuildReport_SkippedRepoRulesCountsEveryRepoRuleThatCouldNotRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		hasRepository bool
		skipRepoRules bool
		wantSkipped   int
		wantIgnored   bool
	}{
		{name: "no repository skips every repo rule", hasRepository: false, skipRepoRules: false, wantSkipped: 2, wantIgnored: false},
		{name: "explicit skip counts every repo rule as ignored", hasRepository: true, skipRepoRules: true, wantSkipped: 2, wantIgnored: true},
		{name: "repository present and not skipped runs them all", hasRepository: true, skipRepoRules: false, wantSkipped: 0, wantIgnored: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rbc := ReportBuildContext{
				RepoRules:     []RepositoryRule{createPassingRepoRule("branchahead"), createPassingRepoRule("linearhistory")},
				HasRepository: testCase.hasRepository,
				SkipRepoRules: testCase.skipRepoRules,
			}
			report := BuildReport(nil, nil, rbc, nil, ReportInput{})

			check.Equal(t, testCase.wantSkipped, report.Summary.SkippedRepoRules)
			check.Equal(t, testCase.wantIgnored, report.Summary.IgnoreRepoRules)
		})
	}
}

// The subject rule re-reports a conventional parse failure it hit while looking
// for the first letter; the conventional rule's own verdict is authoritative and
// the subject row must not repeat it.
func TestBuildRuleReports_SuppressesSubjectRowsTheConventionalRuleAlreadyReports(t *testing.T) {
	t.Parallel()

	conventionalFormat := func(rule string) ValidationError {
		return New(rule, diagnostics.ReasonInvalidConventionalFormat, rule+": invalid format")
	}
	tooLong := New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Too long")

	tests := []struct {
		name               string
		errors             []ValidationError
		wantSubjectStatus  ValidationStatus
		wantSubjectReasons []diagnostics.Reason
	}{
		{
			name:               "shared reason is dropped from subject and the rest kept",
			errors:             []ValidationError{conventionalFormat(rulecatalog.Conventional), conventionalFormat(rulecatalog.Subject), tooLong},
			wantSubjectStatus:  StatusFailed,
			wantSubjectReasons: []diagnostics.Reason{diagnostics.ReasonSubjectTooLong},
		},
		{
			name:               "subject with only shared reasons passes",
			errors:             []ValidationError{conventionalFormat(rulecatalog.Conventional), conventionalFormat(rulecatalog.Subject)},
			wantSubjectStatus:  StatusPassed,
			wantSubjectReasons: nil,
		},
		{
			name:               "a reason the conventional rule did not report is kept",
			errors:             []ValidationError{New(rulecatalog.Conventional, diagnostics.ReasonInvalidSpacing, "spacing"), conventionalFormat(rulecatalog.Subject)},
			wantSubjectStatus:  StatusFailed,
			wantSubjectReasons: []diagnostics.Reason{diagnostics.ReasonInvalidConventionalFormat},
		},
		{
			name:               "no conventional row keeps the subject row",
			errors:             []ValidationError{conventionalFormat(rulecatalog.Subject)},
			wantSubjectStatus:  StatusFailed,
			wantSubjectReasons: []diagnostics.Reason{diagnostics.ReasonInvalidConventionalFormat},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commitRules := []CommitRule{createPassingRule(rulecatalog.Subject), createPassingRule(rulecatalog.Conventional)}
			reports := buildRuleReports(ValidationResult{Errors: testCase.errors}, commitRules)
			check.Len(t, reports, 2)

			byName := make(map[string]RuleReport, len(reports))
			for _, report := range reports {
				byName[report.Name] = report
			}

			var subjectReasons []diagnostics.Reason
			for _, validationErr := range byName[rulecatalog.Subject].Errors {
				subjectReasons = append(subjectReasons, validationErr.Diagnostic.Reason)
			}

			check.Equal(t, testCase.wantSubjectStatus, byName[rulecatalog.Subject].Status)
			check.Equal(t, testCase.wantSubjectReasons, subjectReasons)

			// The conventional rule's own row is never touched by the suppression.
			wantConventional := 0

			for _, validationErr := range testCase.errors {
				if validationErr.Rule == rulecatalog.Conventional {
					wantConventional++
				}
			}

			check.Len(t, byName[rulecatalog.Conventional].Errors, wantConventional)
		})
	}
}

// A pseudo-rule error (the fixup guard) means the real rules never ran, so the
// report lists only the pseudo-rule; once a real rule has reported, every real
// rule is listed after it.
func TestBuildRuleReports_OnlyPseudoRuleErrorsOmitTheRealRules(t *testing.T) {
	t.Parallel()

	fixup := New(FixupCommitRuleName, diagnostics.ReasonFixupCommitFound, "Fixup commit")
	commitRules := []CommitRule{createPassingRule(rulecatalog.Subject), createPassingRule(rulecatalog.Conventional)}

	only := buildRuleReports(ValidationResult{Errors: []ValidationError{fixup}}, commitRules)
	check.Len(t, only, 1)
	check.Equal(t, FixupCommitRuleName, only[0].Name)
	check.Equal(t, StatusFailed, only[0].Status)
	check.Equal(t, "Fixup commit", only[0].Message)

	tooLong := New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Too long")
	mixed := buildRuleReports(ValidationResult{Errors: []ValidationError{fixup, tooLong}}, commitRules)
	check.Len(t, mixed, 3)
	check.Equal(t, FixupCommitRuleName, mixed[0].Name)
	check.Equal(t, rulecatalog.Subject, mixed[1].Name)
	check.Equal(t, StatusFailed, mixed[1].Status)
	check.Equal(t, rulecatalog.Conventional, mixed[2].Name)
	check.Equal(t, StatusPassed, mixed[2].Status)
}
