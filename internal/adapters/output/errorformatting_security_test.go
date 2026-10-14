// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"regexp"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// Test that terminal control sequences are properly escaped in error message formatting.
// Note: Content filtering is NOT the responsibility of the output formatter.
// Security escaping should be done at system boundaries (shell, HTML, etc.)
func TestErrorFormatting_TerminalSafety(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		validationError domain.ValidationError
		shouldEscape    string
		shouldRemove    string
	}{
		{
			name: "ANSI escape sequence in error message",
			validationError: domain.ValidationError{
				Rule:       testRuleTestRule,
				Diagnostic: testDiagnostic(testErrorCode),
				Message:    "Error: \x1b[31mRed\x1b[0m text",
			},
			shouldEscape: "\x1b[", // Should be escaped to \\x1b[
		},
		{
			name: "null byte in message",
			validationError: domain.ValidationError{
				Rule:       testRuleTestRule,
				Diagnostic: testDiagnostic("invalid_format"),
				Message:    "Invalid\x00format",
			},
			shouldRemove: "\x00", // Null bytes should be removed
		},
		{
			name: "normal message with special chars",
			validationError: domain.ValidationError{
				Rule:       testRuleTestRule,
				Diagnostic: testDiagnostic(testErrorCode),
				Message:    "Error: ; rm -rf / ; echo 'gotcha'",
			},
			// Shell commands are not filtered - output layer responsibility
		},
		{
			name: "HTML in message",
			validationError: domain.ValidationError{
				Rule:       testRuleTestRule,
				Diagnostic: testDiagnostic(testErrorCode),
				Message:    "Invalid <script>alert('xss')</script>",
			},
			// HTML escaping is renderer's responsibility
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			summary := createErrorSummary([]domain.ValidationError{testCase.validationError})

			if testCase.shouldEscape != "" {
				check.NotContains(t, summary, testCase.shouldEscape, "Raw escape sequence should not be present")
				check.Contains(t, summary, "\\x1b[", "ANSI escape should be escaped")
			}

			if testCase.shouldRemove != "" {
				check.NotContains(t, summary, testCase.shouldRemove, "Null bytes should be removed")
			}

			// Pass-through is the documented behaviour for content this layer
			// does not filter, so assert the message arrives intact. A bare
			// NotEmpty here passed even if the text had been dropped entirely.
			if testCase.shouldEscape == "" && testCase.shouldRemove == "" {
				check.Equal(t, testCase.validationError.Message, summary)
			}
		})
	}
}

func TestCleanForTerminal_EscapesControlCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "escape", input: "a\x1b[31mb", want: `a\x1b[31mb`},
		{name: "bell", input: "a\x07b", want: `a\x07b`},
		{name: "c1_csi", input: "a\u009b2Jb", want: `a\x9b2Jb`},
		{name: "c1_upper_bound", input: "a\u009fb", want: `a\x9fb`},
		{name: "delete", input: "a\x7fb", want: `a\x7fb`},
		{name: "nul_removed", input: "a\x00b", want: "ab"},
		{name: "line_breaks_and_tab_become_spaces", input: "a\r\n\tb", want: "a   b"},
		{name: "printable_unicode_kept", input: "åäö  ✓", want: "åäö  ✓"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, cleanForTerminal(testCase.input))
		})
	}
}

// Multi-line help is trusted layout: the remediation block must keep its
// separate Config File and CLI Flag lines at the verbosity that shows help.
func TestText_HelpKeepsMultiLineRemediationLayout(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, "Merge branch 'feature'", "linearhistory", domain.ValidationError{
			Rule:       "linearhistory",
			Diagnostic: testDiagnostic(diagnostics.ReasonMergeCommitFound),
			Message:    "merge commit found",
		}).
		build()

	for _, level := range []int{1, 2} {
		output := Text(report, TextOptions{VerboseLevel: level})

		check.Contains(t, output,
			"Disable rule:\n  Config File: rules.disabled: [\"linearhistory\"]\n  CLI Flag: --exclude-rule linearhistory\n",
			"verbosity %d must keep the help's Config File and CLI Flag lines", level)
	}
}

// Commit subjects and messages are untrusted: every human-facing format must
// render them without raw control characters, keeping only its own markers.
func TestFormatters_EscapeUntrustedControlCharacters(t *testing.T) {
	t.Parallel()

	const hostile = "fe\x1b]8;;https://evil.example\x07at: add\x1b[2Jthing\u009b31m\x00\r\n" +
		"section_start:1:fake[collapsed=true]\r\x1b[0Kforged"

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, hostile, "subject", domain.ValidationError{
			Rule:       "subject",
			Diagnostic: testDiagnosticWithID("GL201", testErrorCode),
			Message:    "Invalid subject: " + hostile,
			Context: map[string]string{
				diagnostics.ContextActual:  "fe\x1b]0;pwn\x07",
				diagnostics.ContextSubject: hostile,
			},
		}, domain.ValidationError{
			Rule:       "subject",
			Diagnostic: testDiagnosticWithID("GL302", diagnostics.ReasonPastTense),
			Context: map[string]string{
				diagnostics.ContextSuggestion: "add\x1b[2J\u009b0m",
			},
		}, domain.ValidationError{
			Rule:       "subject",
			Diagnostic: testDiagnostic(diagnostics.ReasonTooManyCommits),
			Message:    "Invalid subject: too many commits",
			Context: map[string]string{
				diagnostics.ContextRef:  "main\r\ninjected-line\x1b[2J",
				diagnostics.ContextOver: "2",
			},
		}).
		repoRule(domain.RuleReport{
			Name:   "branchahead",
			Status: domain.StatusFailed,
			Errors: []domain.ValidationError{{
				Rule:       "branchahead",
				Diagnostic: testDiagnostic(testErrorCode),
				Message:    "Invalid subject: " + hostile,
			}},
		}).
		build()

	gitlabMarker := regexp.MustCompile(`\x1b\[0Ksection_(?:start|end):\d+:[a-z0-9_]+(?:\[collapsed=true\])?\r\x1b\[0K`)
	noMarkers := func(output string) string { return output }

	tests := []struct {
		name         string
		render       func() string
		stripMarkers func(string) string
	}{
		{name: "text", render: func() string { return Text(report, TextOptions{}) }, stripMarkers: noMarkers},
		{name: "text_verbose", render: func() string { return Text(report, TextOptions{VerboseLevel: 1}) }, stripMarkers: noMarkers},
		{name: "text_extra_verbose", render: func() string { return Text(report, TextOptions{VerboseLevel: 2}) }, stripMarkers: noMarkers},
		{name: "gitlab", render: func() string { return GitLab(report) }, stripMarkers: func(output string) string {
			return gitlabMarker.ReplaceAllString(output, "")
		}},
		{name: "github", render: func() string { return GitHub(report) }, stripMarkers: noMarkers},
		{name: "table", render: func() string { return Table(report, TableOptions{Colors: GetColorScheme(false, "")}) }, stripMarkers: noMarkers},
		{name: "job_summary", render: func() string { return JobSummary(report) }, stripMarkers: noMarkers},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			output := testCase.render()

			check.Contains(t, output, "at: add", "the visible subject text must survive")
			check.Contains(t, output, "Invalid subject", "the visible diagnostic text must survive")

			for _, r := range testCase.stripMarkers(output) {
				if (r < 0x20 && r != '\n') || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
					t.Fatalf("raw control character %U in %s output:\n%q", r, testCase.name, output)
				}
			}

			for line := range strings.SplitSeq(output, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "injected-line") {
					t.Fatalf("commit text started a new line in %s output:\n%q", testCase.name, output)
				}
			}
		})
	}

	t.Run("text_help_keeps_trusted_line_break", func(t *testing.T) {
		t.Parallel()

		output := Text(report, TextOptions{VerboseLevel: 1})

		check.Contains(t, output, "\nDisable: rules.disabled: [\"branchahead\"]\n",
			"the help's own line break survives while the interpolated ref is escaped")
	})

	t.Run("gitlab_section_lines", func(t *testing.T) {
		t.Parallel()

		starts := 0

		for line := range strings.SplitSeq(GitLab(report), "\n") {
			if strings.HasPrefix(strings.TrimPrefix(line, "\x1b[0K"), "section_start:") {
				starts++
			}
		}

		check.Equal(t, 3, starts, "only the summary, commit and repository sections may start a line")
	})
}
