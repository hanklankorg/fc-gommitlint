// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"strconv"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// snapshotBodyLineError builds a GL305 violation like the commitbody rule emits.
func snapshotBodyLineError(line, actual string) domain.ValidationError {
	err := domain.New(rulecatalog.CommitBody, diagnostics.ReasonBodyLineTooLong,
		fmt.Sprintf("Line %s exceeds 72 characters (%s found)", line, actual))

	return err.WithContextMap(diagnostics.Context{
		diagnostics.ContextLine:     line,
		diagnostics.ContextActual:   actual,
		diagnostics.ContextExpected: "max 72",
		diagnostics.ContextMax:      "72",
	})
}

func snapshotReport(violations int) domain.Report {
	errors := make([]domain.ValidationError, 0, violations)
	for i := range violations {
		errors = append(errors, snapshotBodyLineError(strconv.Itoa(i+1), strconv.Itoa(74+i)))
	}

	return domain.Report{
		Summary: domain.ReportSummary{TotalCommits: 1, ValidatedCommits: 1},
		Commits: []domain.CommitReport{{
			Commit: domain.Commit{Hash: "abc1234def", Subject: "feat: add thing"},
			RuleResults: []domain.RuleReport{
				{
					Name:   rulecatalog.CommitBody,
					Status: domain.StatusFailed,
					Errors: errors,
				},
				{
					Name:    rulecatalog.Subject,
					Status:  domain.StatusPassed,
					Checks:  []string{"GL101", "GL102"},
					Message: "GL101, GL102",
				},
			},
		}},
	}
}

// TestTextFormatter_GroupedFailureSummary pins the multi-commit summary
// line: total issues grouped by check ID, plus the failing-commit count.
func TestTextFormatter_GroupedFailureSummary(t *testing.T) {
	t.Parallel()

	failing := snapshotReport(2).Commits[0]
	passing := domain.CommitReport{
		Commit: domain.Commit{Hash: "def5678abc", Subject: "fix: adjust"},
		Passed: true,
		RuleResults: []domain.RuleReport{
			{Name: rulecatalog.Subject, Status: domain.StatusPassed},
		},
	}
	report := domain.Report{
		Summary: domain.ReportSummary{TotalCommits: 2, ValidatedCommits: 2},
		Commits: []domain.CommitReport{failing, passing},
	}

	got := Text(report, TextOptions{UseColor: false})

	check.Contains(t, got, "✗ 2 issues found (GL305 ×2) in 1 commit")
}

// TestTextFormatter_VerbosityLadderSnapshots pins the rendering contract at
// every verbosity level. These are deliberate golden strings: a diff here
// means the user-facing output contract changed and the change should be
// intentional, reviewed, and reflected in docs/testing/cli-black-box.adoc.
func TestTextFormatter_VerbosityLadderSnapshots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		violations int
		options    TextOptions
		want       string
	}{
		{
			name:       "default level",
			violations: 2,
			options:    TextOptions{UseColor: false},
			want: `
COMMIT [abc1234] feat: add thing
  ✗ Commit body - GL305: Line 1 is 74 characters (max 72)
      GL305: Line 2 is 75 characters (max 72)
      See examples: gommitlint validate --rule-help commitbody
  ✓ Subject

✗ 2 issues found (GL305 ×2) in 1 commit
`,
		},
		{
			name:       "default level collapses beyond five violations",
			violations: 8,
			options:    TextOptions{UseColor: false},
			want: `
COMMIT [abc1234] feat: add thing
  ✗ Commit body - GL305: Line 1 is 74 characters (max 72)
      GL305: Line 2 is 75 characters (max 72)
      GL305: Line 3 is 76 characters (max 72)
      GL305: Line 4 is 77 characters (max 72)
      GL305: Line 5 is 78 characters (max 72)
      … and 3 more
      See examples: gommitlint validate --rule-help commitbody
  ✓ Subject

✗ 8 issues found (GL305 ×8) in 1 commit
`,
		},
		{
			// Context rows are suppressed here because the finding text already
			// embeds every context value; findings that don't embed them keep
			// their Expected/Actual rows. -v adds remediation help once per rule.
			name:       "verbose level",
			violations: 2,
			options:    TextOptions{VerboseLevel: 1, UseColor: false},
			want: `
COMMIT [abc1234] feat: add thing
  ✗ Commit body - GL305: Line 1 is 74 characters (max 72)
      GL305: Line 2 is 75 characters (max 72)
      Help:
        Wrap body lines at 72 characters for better readability in git log
      See examples: gommitlint validate --rule-help commitbody
  ✓ Subject

✗ 2 issues found (GL305 ×2) in 1 commit
`,
		},
		{
			// -vv renders the complete context map per violation, deduplicated
			// per rule: static rows (Expected: max 72, Max: 72) print once,
			// per-violation rows (Actual, Line) print for every violation.
			name:       "very verbose level",
			violations: 2,
			options:    TextOptions{VerboseLevel: 2, UseColor: false},
			want: `
COMMIT [abc1234] feat: add thing
  ✗ Commit body - GL305: Line 1 is 74 characters (max 72)
      Expected: max 72
      Actual: 74
      Line: 1
      Max: 72
      GL305: Line 2 is 75 characters (max 72)
      Actual: 75
      Line: 2
      Help:
        Wrap body lines at 72 characters for better readability in git log
      See examples: gommitlint validate --rule-help commitbody
  ✓ Subject (GL101, GL102)

✗ 2 issues found (GL305 ×2) in 1 commit
`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := Text(snapshotReport(testCase.violations), testCase.options)
			check.Equal(t, testCase.want, got)
		})
	}
}
