// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"encoding/csv"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

// Test constants for divider formatting.
const (
	dividerPattern     = "··················································································"
	lightOrangeANSI214 = "\033[38;5;214m"
)

var ansiCodePattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripAnsiCodes(text string) string {
	return ansiCodePattern.ReplaceAllString(text, "")
}

func TestTextFormatter_SanitizesCommitSubjectForTerminal(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Commits: []domain.CommitReport{
			{
				Commit: domain.Commit{
					Hash:    testCommitHashABC1234,
					Subject: "feat: \x1b]8;;https://evil.example\x07link\x1b]8;;\x07",
				},
				Passed: true,
			},
		},
		Summary: domain.ReportSummary{AllPassed: true, ValidatedCommits: 1, PassedCommits: 1, TotalCommits: 1},
	}

	result := Text(report, TextOptions{})

	check.NotContains(t, result, "\x1b]", "raw OSC sequence should not be present")
	check.NotContains(t, result, "\x07", "raw BEL control should not be present")
	check.Contains(t, result, "\\x1b]8;;https://evil.example\\x07link\\x1b]8;;\\x07")
}

func TestTextFormatter_ProgressiveDisclosure(t *testing.T) {
	t.Parallel()

	commit := domain.Commit{
		Hash:    testCommitHashABC1234,
		Subject: testCommitSubjectNotConventional,
		Message: testCommitSubjectNotConventional,
	}

	conventionalFormat := domain.ConventionalCommitFormatTemplate
	conventionalHelp := fmt.Sprintf("Use format: %s (e.g., 'feat: add login')", conventionalFormat)
	conventionalHelpShort := "Use format: " + conventionalFormat

	errors := []domain.ValidationError{
		{
			Diagnostic: testDiagnosticWithID(testCheckIDGL201, testErrorCodeInvalidConventional),
			Message:    "Commit message doesn't follow conventional format",
			Context: map[string]string{
				diagnostics.ContextExpected: conventionalFormat,
				"found":                     testCommitSubjectNotConventional,
			},
		},
		{
			Diagnostic: testDiagnosticWithID("GL104", testErrorCodeInvalidCase),
			Message:    "First word 'not' should be in sentence case",
			Context: map[string]string{
				contextFirstWord:    "not",
				contextExpectedWord: "Not",
			},
		},
	}

	ruleResults := []domain.RuleReport{
		{
			Name:    testRuleTestRuleDisplay,
			Status:  domain.StatusFailed,
			Message: testRuleFailedMessage,
			Errors:  errors,
		},
	}

	commitReport := domain.CommitReport{
		Commit:      commit,
		RuleResults: ruleResults,
	}

	report := domain.Report{
		Commits: []domain.CommitReport{commitReport},
		Summary: domain.ReportSummary{
			TotalCommits:     1,
			ValidatedCommits: 1,
			IgnoredCommits:   0,
			PassedCommits:    0,
			AllPassed:        false,
		},
	}

	tests := []struct {
		name            string
		options         TextOptions
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "Basic level - concise summary on same line",
			options: TextOptions{
				ShowHelp: false,
			},
			wantContains: []string{
				"✗ " + testRuleTestRuleDisplay,
				conventionalHelpShort,
				testStartWithNot,
			},
			wantNotContains: []string{
				"[invalid_conventional_format]",
				testOutputHelpHeader,
				testRuleTestRuleDisplay + ":",
				testOutputErrorCodeHeader,
			},
		},
		{
			name: "Verbose level - curated context with help",
			options: TextOptions{
				VerboseLevel: 1,
			},
			wantContains: []string{
				"✗ " + testRuleTestRuleDisplay,
				testCheckIDGL201 + ": ",
				conventionalHelpShort,
				"GL104: ",
				testStartWithNot,
				testOutputHelpHeader, // -v adds remediation help once per rule
			},
			wantNotContains: []string{
				"Current Word:", // full context map is reserved for -vv
			},
		},
		{
			name: "Very verbose level - codes and help text",
			options: TextOptions{
				VerboseLevel: 2,
				ShowHelp:     true,
			},
			wantContains: []string{
				"✗ " + testRuleTestRuleDisplay,
				testCheckIDGL201 + ": ",
				conventionalHelpShort,
				testOutputHelpHeader,
				conventionalHelp,
				"GL104: ",
				testStartWithNot,
				"Check the case of the first character of your commit subject",
			},
			wantNotContains: []string{
				"Subject: not conventional commit", // Subject should not be duplicated
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := Text(report, testCase.options)

			for _, want := range testCase.wantContains {
				check.Contains(t, result, want, "Expected output to contain: %s", want)
			}

			for _, notWant := range testCase.wantNotContains {
				check.NotContains(t, result, notWant, "Expected output to NOT contain: %s", notWant)
			}
		})
	}
}

func TestTextFormatter_SingleError(t *testing.T) {
	t.Parallel()

	commit := domain.Commit{
		Hash:    testCommitHashABC1234,
		Subject: testCommitSubjectTestCommit,
		Message: testCommitSubjectTestCommit,
	}

	singleError := []domain.ValidationError{
		{
			Diagnostic: testDiagnostic(testErrorCodeMissingSignoff),
			Message:    messageMissingSignoff,
		},
	}

	ruleResults := []domain.RuleReport{
		{
			Name:    rules.SignOffRule{}.Name(),
			Status:  domain.StatusFailed,
			Message: testRuleFailedMessage,
			Errors:  singleError,
		},
	}

	commitReport := domain.CommitReport{
		Commit:      commit,
		RuleResults: ruleResults,
	}

	report := domain.Report{
		Commits: []domain.CommitReport{commitReport},
		Summary: domain.ReportSummary{
			TotalCommits:     1,
			ValidatedCommits: 1,
			IgnoredCommits:   0,
			PassedCommits:    0,
			AllPassed:        false,
		},
	}

	// Basic level should show single error message directly
	basicOptions := TextOptions{ShowHelp: false}
	result := Text(report, basicOptions)

	check.Contains(t, result, "✗ Sign-off")
	check.Contains(t, result, "No Signed-off-by trailer found")
	check.NotContains(t, result, "["+testErrorCodeMissingSignoff+"]")
	check.NotContains(t, result, "Sign-off:")
	check.NotContains(t, result, testOutputErrorCodeHeader)
}

func TestTextFormatter_FailedSingleCommitHasOneFinalConclusionAfterConfig(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		configSources("defaults", ".gommitlint.yaml").
		failingCommit(testCommitHashABC1234, "bad subject", "subject",
			domain.New("subject", diagnostics.ReasonSubjectTooLong, "subject is too long")).
		build()

	result := Text(report, TextOptions{ConfigSources: []string{".gommitlint.yaml"}})
	conclusion := "✗ 1 issue found"

	check.Equal(t, 1, strings.Count(result, conclusion))
	check.Contains(t, result, "Config: .gommitlint.yaml")
	check.True(t,
		strings.Index(result, "Config:") < strings.Index(result, conclusion),
		"config metadata must precede the final conclusion",
	)
	lastLine := result[strings.LastIndex(strings.TrimSuffix(result, "\n"), "\n")+1:]
	check.True(t, strings.HasPrefix(lastLine, conclusion),
		"failure conclusion must be the final output line: %q", result)
}

func TestTextFormatter_VerboseContextReservesRedForActualValue(t *testing.T) {
	t.Parallel()

	style := func(name string) func(...any) string {
		return func(values ...any) string { return "<" + name + ">" + fmt.Sprint(values...) + "</" + name + ">" }
	}
	colors := ColorScheme{
		Error: style("red"),
		Info:  style("info"),
		Muted: style("muted"),
	}
	err := domain.New("subject", diagnostics.ReasonSubjectCase, "wrong case").WithContextMap(diagnostics.Context{
		diagnostics.ContextExpected: "lowercase",
		diagnostics.ContextActual:   "Uppercase",
		diagnostics.ContextLine:     "1",
	})

	var result strings.Builder

	writeErrorContext(&result, err, "  ", colors, TextOptions{VerboseLevel: 2, ShowContext: true}, nil, map[string]bool{})

	check.Contains(t, result.String(), "<info>  Expected: lowercase")
	check.Contains(t, result.String(), "<red>  Actual: Uppercase")
	check.Contains(t, result.String(), "<muted>  Line: 1")
	check.NotContains(t, result.String(), "<red>  Expected:")
}

func TestTextFormatter_PlainFormat(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Commits: []domain.CommitReport{
			{
				Commit: domain.Commit{
					Hash:    "abc1234567890",
					Subject: "feat: add, api",
					Message: "feat: add, api\n\nBody too short: 5\nneeds 10",
				},
				RuleResults: []domain.RuleReport{
					{
						Name:   rules.SubjectRule{}.Name(),
						Status: domain.StatusFailed,
						Errors: []domain.ValidationError{
							{
								Diagnostic: testDiagnostic("body_too_short"),
								Message:    "Body too short: 5\nneeds 10",
							},
						},
					},
				},
			},
		},
		Repository: domain.RepositoryReport{
			RuleResults: []domain.RuleReport{
				{
					Name:   rules.LinearHistoryRule{}.Name(),
					Status: domain.StatusPassed,
				},
			},
		},
		Summary: domain.ReportSummary{
			TotalCommits:     1,
			ValidatedCommits: 1,
			IgnoredCommits:   0,
			PassedCommits:    0,
			AllPassed:        false,
		},
	}

	result := TextPlain(report)
	reader := csv.NewReader(strings.NewReader(result))
	reader.FieldsPerRecord = -1 // record types use different arities; we assert each shape below

	records, err := reader.ReadAll()
	check.NoError(t, err)
	check.Len(t, records, 3)

	// Layout: every row leads with its type column so a single csv reader handles them all.
	check.Equal(t, []string{
		"COMMIT",
		testCommitHashABC1234,
		"feat: add, api",
		rulecatalog.Subject,
		textPlainStatusFail,
		"Body too short: 5\\nneeds 10",
	}, records[0])

	check.Equal(t, []string{
		"REPO", "", "",
		rulecatalog.LinearHistory,
		textPlainStatusPass,
		"",
	}, records[1])

	check.Equal(t, []string{
		"SUMMARY", textPlainStatusFail, "1", "2", "1", "0",
	}, records[2])
}

func TestTextFormatter_IgnoredCommitsSummary(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Summary: domain.ReportSummary{
			TotalCommits:     3,
			ValidatedCommits: 2,
			IgnoredCommits:   1,
			PassedCommits:    2,
			FailedCommits:    0,
			AllPassed:        true,
		},
		Metadata: domain.ReportMetadata{
			WarningDetails: []domain.Warning{{Code: testValidationWarningCode, Message: "warning"}},
		},

		Commits: []domain.CommitReport{
			{
				Commit: domain.Commit{Hash: testCommitHashABC1234, Subject: testCommitSubjectFeatOK},
				Passed: true,
			},
			{
				Commit: domain.Commit{Hash: testCommitHashDEF5678, Subject: "fix: ok"},
				Passed: true,
			},
		},
	}

	result := Text(report, TextOptions{})

	check.Contains(t, result, "2 commit")
	check.Contains(t, result, "1 ignored")
	check.Contains(t, result, "1 warning")
	// Warning detail rendering was moved out of the text report (stdout)
	// into the CLI's stderr emission (see validate_setup.go's
	// showConfigWarnings). The summary line still includes the warning
	// count; the per-warning text now lives on stderr only — confirmed by
	// stream_contract_integration_test.go.
	check.NotContains(t, result, "Warnings:",
		"warning details must not appear in the text report (stdout); they belong on stderr")
}

// TestText_MultipleCommitsSummary tests summary formatting for multiple commits.
func TestTextFormatter_MultipleCommitsSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		report        domain.Report
		options       TextOptions
		expectedLines []string
		notExpected   []string
		description   string
	}{
		{
			name: "repository rules all passed",
			report: domain.Report{
				Commits: []domain.CommitReport{},
				Repository: domain.RepositoryReport{
					RuleResults: []domain.RuleReport{
						{
							Name:   rulecatalog.BranchAhead,
							Status: domain.StatusPassed,
							Errors: nil,
						},
						{
							Name:   rulecatalog.Identity,
							Status: domain.StatusPassed,
							Errors: nil,
						},
					},
				},
				Summary: domain.ReportSummary{AllPassed: true},
			},
			options: TextOptions{UseColor: false},
			expectedLines: []string{
				"✓ Branch Ahead",
				"✓ Identity",
				"✓ All checks passed",
			},
			description: "should format passing repository rules",
		},
		{
			name: "repository rules with failures",
			report: domain.Report{
				Commits: []domain.CommitReport{},
				Repository: domain.RepositoryReport{
					RuleResults: []domain.RuleReport{
						{
							Name:   rulecatalog.BranchAhead,
							Status: domain.StatusFailed,
							Errors: []domain.ValidationError{
								{
									Diagnostic: testDiagnostic("behind_main"),
									Message:    "Branch is 2 commits behind main",
								},
							},
						},
						{
							Name:   rulecatalog.Identity,
							Status: domain.StatusPassed,
							Errors: nil,
						},
					},
				},
				Summary: domain.ReportSummary{AllPassed: false},
			},
			options: TextOptions{UseColor: false},
			expectedLines: []string{
				"✗ Branch Ahead",
				"✓ Identity",
				"✗ 1 issue found",
			},
			description: "repository rules with failures",
		},
		{
			name: "repository rules with pass info",
			report: domain.Report{
				Commits: []domain.CommitReport{},
				Repository: domain.RepositoryReport{
					RuleResults: []domain.RuleReport{
						{
							Name: rulecatalog.BranchAhead,

							Status:  domain.StatusPassed,
							Errors:  nil,
							Message: "3/5",
						},
					},
				},
				Summary: domain.ReportSummary{AllPassed: true},
			},
			options: TextOptions{UseColor: false},
			expectedLines: []string{
				"✓ Branch Ahead - 3/5",
				"✓ All checks passed",
			},
			description: "should append pass info for repository rules",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := testCase.options
			options.Colors = GetColorScheme(options.UseColor, options.Theme)

			result := Text(testCase.report, options)

			for _, expectedLine := range testCase.expectedLines {
				check.Contains(t, result, expectedLine, testCase.description)
			}

			for _, notExpectedLine := range testCase.notExpected {
				check.NotContains(t, result, notExpectedLine, testCase.description)
			}
		})
	}
}

// TestWriteFailedRulesSummary tests the failed rules summary formatting.
func TestTextFormatter_WriteFailedRulesSummary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		summary       domain.ReportSummary
		expectedLines []string
		notExpected   []string
		description   string
	}{
		{
			name: "multiple failed rules",
			summary: domain.ReportSummary{
				FailedRules: map[string]int{
					"conventional":             3,
					diagnostics.ContextSubject: 1,
					"body":                     2,
				},
			},
			expectedLines: []string{
				"- body: 2 failures",
				"- conventional: 3 failures",
				"- subject: 1 failure",
			},
			description: "should format failed rules summary sorted alphabetically",
		},
		{
			name: "single failed rule",
			summary: domain.ReportSummary{
				FailedRules: map[string]int{
					"signoff": 1,
				},
			},
			expectedLines: []string{
				"- signoff: 1 failure",
			},
			description: "should format single failed rule",
		},
		{
			name: "no failed rules",
			summary: domain.ReportSummary{
				FailedRules: map[string]int{},
			},
			expectedLines: []string{},
			description:   "should produce no output for empty failed rules",
		},
		{
			name: "nil failed rules map",
			summary: domain.ReportSummary{
				FailedRules: nil,
			},
			expectedLines: []string{},
			description:   "should handle nil failed rules map",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var builder strings.Builder

			colors := GetColorScheme(false, "") // No colors for easier testing

			writeFailedRulesSummary(&builder, testCase.summary, colors, nil)
			result := builder.String()

			if len(testCase.expectedLines) == 0 {
				check.Empty(t, result, testCase.description)
			} else {
				for _, expectedLine := range testCase.expectedLines {
					check.Contains(t, result, expectedLine, testCase.description)
				}
			}
		})
	}
}

// A commit or repository section with no rule results at all still renders as
// passed rather than vanishing or crashing.
func TestTextFormatter_ShowsPassedForNilRuleResults(t *testing.T) {
	t.Parallel()

	t.Run("commit with nil rule results", func(t *testing.T) {
		t.Parallel()

		report := domain.Report{
			Commits: []domain.CommitReport{{
				Commit:      domain.Commit{Hash: testCommitHashABC123, Subject: testCommitSubjectTestCommit},
				Passed:      true,
				RuleResults: nil,
			}},
			Summary: domain.ReportSummary{
				TotalCommits: 1, ValidatedCommits: 1, PassedCommits: 1, AllPassed: true,
			},
		}

		result := Text(report, TextOptions{UseColor: false})

		check.Contains(t, result, testCommitSubjectTestCommit)
		check.Contains(t, result, "✓ All checks passed")
		check.NotContains(t, result, "✗")
	})

	t.Run("repository with nil rule results", func(t *testing.T) {
		t.Parallel()

		report := domain.Report{
			Commits:    []domain.CommitReport{},
			Repository: domain.RepositoryReport{RuleResults: nil},
			Summary:    domain.ReportSummary{AllPassed: true},
		}

		result := Text(report, TextOptions{UseColor: false})

		check.Contains(t, result, "✓ All checks passed")
		check.NotContains(t, result, "✗")
	})

	t.Run("TextPlain renders a passing commit as a PASS record", func(t *testing.T) {
		t.Parallel()

		report := domain.Report{
			Commits: []domain.CommitReport{{
				Commit: domain.Commit{Hash: testCommitHashABC123, Subject: testCommitSubjectTest},
				Passed: true,
				RuleResults: []domain.RuleReport{
					{Name: rulecatalog.Subject, Status: domain.StatusPassed},
				},
			}},
			Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1, PassedCommits: 1, AllPassed: true},
		}

		records, err := csv.NewReader(strings.NewReader(TextPlain(report))).ReadAll()
		check.NoError(t, err)
		check.Len(t, records, 2)
		check.Equal(t, []string{
			"COMMIT", testCommitHashABC123, testCommitSubjectTest,
			rulecatalog.Subject, textPlainStatusPass, "",
		}, records[0])
	})
}

// - Proper spacing and visual hierarchy throughout.
func TestTextFormatter_VerbosityFormatting(t *testing.T) {
	t.Parallel()

	testReport := createTestReportForStructure()

	tests := []struct {
		name            string
		options         TextOptions
		expectHelp      bool
		expectDividers  int
		expectColorCode bool
	}{
		{
			name:            "verbose mode structure",
			options:         TextOptions{VerboseLevel: 1, UseColor: false},
			expectHelp:      false,
			expectDividers:  0, // No dividers now
			expectColorCode: false,
		},
		{
			name:            "extra verbose mode structure",
			options:         TextOptions{VerboseLevel: 2, UseColor: false},
			expectHelp:      true,
			expectDividers:  0, // No dividers now
			expectColorCode: false,
		},
		{
			name:            "colored dividers",
			options:         TextOptions{VerboseLevel: 1, UseColor: true},
			expectHelp:      false, // Level 1 shows technical details but not help text
			expectDividers:  0,     // No dividers now
			expectColorCode: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := Text(testReport, testCase.options)

			// Verify core structural requirements
			assertStructuralIntegrity(t, result, testCase.expectHelp, testCase.expectDividers, testCase.expectColorCode)
		})
	}
}

func TestTextFormatter_VerboseShowsCheckDiagnostics(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1},
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{Subject: "long subject"},
			RuleResults: []domain.RuleReport{{
				Name:   rulecatalog.Subject,
				Status: domain.StatusFailed,
				Errors: []domain.ValidationError{
					domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Subject too long"),
				},
			}},
		}},
	}

	result := Text(report, TextOptions{VerboseLevel: 1, UseColor: false})

	check.Contains(t, result, "GL102: ")
	check.NotContains(t, result, testOutputErrorCodeHeader)
	check.NotContains(t, result, "Category: message.subject")
}

func TestTextFormatter_PassedCheckSummariesRequireVerbose(t *testing.T) {
	t.Parallel()

	report := domain.Report{
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1},
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{Subject: "feat: add output"},
			RuleResults: []domain.RuleReport{
				{
					Name:    rulecatalog.Subject,
					Status:  domain.StatusPassed,
					Checks:  []string{testCheckIDGL101, testCheckIDGL102},
					Message: "GL101, GL102",
				},
				{
					Name:   rulecatalog.CryptoSignature,
					Status: domain.StatusFailed,
					Errors: []domain.ValidationError{
						domain.New(rulecatalog.CryptoSignature, diagnostics.ReasonMissingSignature, "missing signature"),
					},
				},
			},
		}},
	}

	concise := Text(report, TextOptions{UseColor: false})
	verbose := Text(report, TextOptions{VerboseLevel: 1, UseColor: false})
	veryVerbose := Text(report, TextOptions{VerboseLevel: 2, UseColor: false})

	check.Contains(t, concise, "✓ Subject")
	check.NotContains(t, concise, "✓ Subject (GL101, GL102)")
	check.NotContains(t, verbose, "✓ Subject (GL101, GL102)")
	check.Contains(t, veryVerbose, "✓ Subject (GL101, GL102)")
}

// TestTextFormatter_DifferentReportTypes validates structure for single vs multiple error reports.
func TestTextFormatter_DifferentReportTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		report          domain.Report
		expectDividers  int
		expectRuleCount int
	}{
		{
			name:            "single error",
			report:          createSingleErrorReport(),
			expectDividers:  0, // No dividers without WriteDivider calls
			expectRuleCount: 1,
		},
		{
			name:            "multiple rules",
			report:          createMultipleRulesReport(),
			expectDividers:  0, // No dividers without WriteDivider calls
			expectRuleCount: 2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := TextOptions{VerboseLevel: 1, UseColor: false}
			result := Text(testCase.report, options)

			// Count dividers and rules
			dividerCount := strings.Count(result, dividerPattern)
			check.Equal(t, testCase.expectDividers, dividerCount,
				"Must have exactly %d dividers", testCase.expectDividers)

			// Count failed rules - look for "✗ " followed by rule name (capital letter or word)
			// This avoids counting the summary line "✗ X issues found"
			ruleMatches := 0

			lines := strings.SplitSeq(result, "\n")
			for line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "✗ ") && !strings.Contains(trimmed, "issue") {
					ruleMatches++
				}
			}

			// Debug: print output if test fails
			if ruleMatches != testCase.expectRuleCount {
				t.Logf("Output:\n%s", result)
			}

			check.Equal(t, testCase.expectRuleCount, ruleMatches,
				"Must have exactly %d failed rules", testCase.expectRuleCount)
		})
	}
}

// assertStructuralIntegrity validates the core formatting structure.
func assertStructuralIntegrity(t *testing.T, result string, expectHelp bool, expectDividers int, expectColorCode bool) {
	t.Helper()

	searchResult := result
	if expectColorCode {
		searchResult = stripAnsiCodes(result)
	}

	check.Contains(t, searchResult, "✗ "+testRuleTestRuleDisplay, "Must show friendly rule name")
	check.Contains(t, searchResult, "First error message", "Must include error message")
	check.Contains(t, searchResult, "Second error message", "Must include second error")
	check.Contains(t, searchResult, "Third error message", "Must include third error")

	if expectHelp {
		check.Contains(t, searchResult, "      Expected: expected1", "Must include expected values in verbose mode")
		check.Contains(t, searchResult, "      Actual: value1", "Must include actual values in verbose mode")
	}

	check.NotContains(t, searchResult, testOutputErrorCodeHeader, "Must not expose internal reason in text output")

	// At extra verbose level, help section only appears when the error code has registered help.
	// The test errors use unregistered codes, so help is not expected here.
	// Help text behavior is tested in TestErrorFormatting_VerbosityLevelsSummary.
	check.NotContains(t, result, testOutputHelpHeader, "Help section should not appear for unregistered error codes")

	// Divider validation
	if expectColorCode {
		coloredDividers := strings.Count(result, lightOrangeANSI214+dividerPattern+resetANSI)
		check.Equal(t, expectDividers, coloredDividers, "Must have colored dividers when colors enabled")
	} else {
		plainDividers := strings.Count(result, dividerPattern)
		colorCodes := strings.Count(result, lightOrangeANSI214)

		check.Equal(t, expectDividers, plainDividers, "Must have plain dividers when colors disabled")
		check.Equal(t, 0, colorCodes, "Must have no color codes when colors disabled")
	}
}

// Test data factories

func createTestReportForStructure() domain.Report {
	return domain.Report{
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{
				Hash:    testCommitHashABC1234,
				Subject: testTestCommitSubject,
				Message: testTestCommitSubject,
			},
			RuleResults: []domain.RuleReport{{
				Name:   testRuleTestRuleDisplay,
				Status: domain.StatusFailed,
				Errors: []domain.ValidationError{
					{
						Diagnostic: testDiagnostic("first_error"),
						Message:    "First error message",
						Context:    map[string]string{diagnostics.ContextActual: "value1", diagnostics.ContextExpected: "expected1"},
					},
					{
						Diagnostic: testDiagnostic("second_error"),
						Message:    "Second error message",
						Context:    map[string]string{contextFirstWord: "actual2", contextExpectedWord: "expected2"},
					},
					{
						Diagnostic: testDiagnostic("third_error"),
						Message:    "Third error message",
						Context:    map[string]string{diagnostics.ContextActual: "value3", diagnostics.ContextExpected: "expected3"},
					},
				},
			}},
		}},
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1, IgnoredCommits: 0, AllPassed: false},
	}
}

func createSingleErrorReport() domain.Report {
	return domain.Report{
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{Hash: testCommitHashABC1234, Subject: testTestCommitSubject},
			RuleResults: []domain.RuleReport{{
				Name:   "SingleRule",
				Status: domain.StatusFailed,
				Errors: []domain.ValidationError{{
					Diagnostic: testDiagnostic("single_error"),
					Message:    "Single error message",
					Context:    map[string]string{diagnostics.ContextActual: testContextValueWrong, diagnostics.ContextExpected: "right"},
				}},
			}},
		}},
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1, IgnoredCommits: 0, AllPassed: false},
	}
}

func createMultipleRulesReport() domain.Report {
	return domain.Report{
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{Hash: testCommitHashABC1234, Subject: testTestCommitSubject},
			RuleResults: []domain.RuleReport{
				{
					Name:   "FirstRule",
					Status: domain.StatusFailed,
					Errors: []domain.ValidationError{
						{Diagnostic: testDiagnostic("error_a"), Message: "Error A"},
						{Diagnostic: testDiagnostic("error_b"), Message: "Error B"},
					},
				},
				{
					Name:   "SecondRule",
					Status: domain.StatusFailed,
					Errors: []domain.ValidationError{
						{Diagnostic: testDiagnostic("error_c"), Message: "Error C"},
					},
				},
			},
		}},
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1, IgnoredCommits: 0, AllPassed: false},
	}
}
