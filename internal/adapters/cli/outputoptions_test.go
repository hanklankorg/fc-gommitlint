// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const (
	testRuleHelpDisplaySubject = "Subject"
	testRuleHelpUpperSubject   = "SUBJECT"
)

func TestOutputOptions_ValidateRuleHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ruleHelp    string
		expectError bool
	}{
		{
			name:        "empty rule help is valid",
			ruleHelp:    "",
			expectError: false,
		},
		{
			name:        "valid factory key - subject",
			ruleHelp:    rulecatalog.Subject,
			expectError: false,
		},
		{
			name:        "valid factory key - conventional",
			ruleHelp:    rulecatalog.Conventional,
			expectError: false,
		},
		{
			// Case is folded, like the surrounding whitespace in the row at
			// the bottom of this table: rulecatalog.Normalize lower-cases, so
			// the spelling a user reaches for resolves to the canonical ID.
			name:        "display name differing only by case is accepted",
			ruleHelp:    testRuleHelpDisplaySubject,
			expectError: false,
		},
		{
			name:        "uppercase rule id is accepted",
			ruleHelp:    testRuleHelpUpperSubject,
			expectError: false,
		},
		{
			// Folding case does not turn display names into aliases, which is
			// what the "Subject" row above used to be testing before that
			// string became a mere case variant of its own ID. A multi-word
			// display name still differs from the ID after folding
			// ("commit body" is not "commitbody"), so it keeps that claim
			// under test.
			name:        "multi-word display name is still not an alias",
			ruleHelp:    "Commit body",
			expectError: true,
		},
		{
			name:        "display-name alias is invalid",
			ruleHelp:    "SignedIdentity",
			expectError: true,
		},
		{
			name:        "invalid rule name",
			ruleHelp:    "invalidrule",
			expectError: true,
		},
		{
			name:        "invalid rule shows sorted valid rules in error",
			ruleHelp:    cliTestNonexistentRef,
			expectError: true,
		},
		{
			name:        "rule with whitespace",
			ruleHelp:    "  subject  ",
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			options := NewOutputOptions(buf).WithRuleHelp(testCase.ruleHelp)

			err := options.ValidateRuleHelp()

			if testCase.expectError {
				check.Error(t, err)
				check.ErrorIs(t, err, ErrUnknownRule)

				var unknownErr UnknownRuleError
				check.ErrorAs(t, err, &unknownErr)
				check.Equal(t, testCase.ruleHelp, unknownErr.Rule)
				check.Equal(t, getValidRuleNames(), unknownErr.ValidRules)
			} else {
				check.NoError(t, err)
			}
		})
	}
}

func TestOutputOptions_ValidateRuleHelp_LocalizesUnknownRule(t *testing.T) {
	t.Parallel()

	const unknownRule = "does-not-exist"

	options := NewOutputOptions(&bytes.Buffer{}).
		WithRuleHelp(unknownRule).
		WithLocalizer(unknownRuleLocalizer{})

	err := options.ValidateRuleHelp()
	check.Error(t, err)
	check.Equal(t,
		fmt.Sprintf("translated unknown rule %q; translated valid rules: %s", unknownRule, strings.Join(getValidRuleNames(), ", ")),
		err.Error(),
	)
}

type unknownRuleLocalizer struct{}

func (unknownRuleLocalizer) Get(message string, args ...any) string {
	if message == "unknown rule %q. Valid rules: %s" {
		message = "translated unknown rule %q; translated valid rules: %s"
	}

	return fmt.Sprintf(message, args...)
}

func (localizer unknownRuleLocalizer) GetN(singular, plural string, count int, args ...any) string {
	if count == 1 {
		return localizer.Get(singular, args...)
	}

	return localizer.Get(plural, args...)
}

func TestOutputOptions_GetNormalizedRuleHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ruleHelp     string
		expectedRule string
	}{
		{
			name:         "empty rule help",
			ruleHelp:     "",
			expectedRule: "",
		},
		{
			name:         "factory key - subject",
			ruleHelp:     rulecatalog.Subject,
			expectedRule: rulecatalog.Subject,
		},
		{
			name:         "factory key - conventional",
			ruleHelp:     rulecatalog.Conventional,
			expectedRule: rulecatalog.Conventional,
		},
		{
			name:         "display name differing only by case folds to the ID",
			ruleHelp:     testRuleHelpDisplaySubject,
			expectedRule: rulecatalog.Subject,
		},
		{
			name:         "uppercase rule id folds to the canonical ID",
			ruleHelp:     testRuleHelpUpperSubject,
			expectedRule: rulecatalog.Subject,
		},
		{
			// Still passed through untouched when it matches nothing, so the
			// error message can quote back what the user actually typed.
			name:         "multi-word display name is preserved as unknown rule",
			ruleHelp:     "Commit body",
			expectedRule: "commit body",
		},
		{
			name:         "unknown rule",
			ruleHelp:     cliTestUnknown,
			expectedRule: cliTestUnknown,
		},
		{
			name:         "rule with whitespace",
			ruleHelp:     "  subject  ",
			expectedRule: rulecatalog.Subject,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			options := NewOutputOptions(buf).WithRuleHelp(testCase.ruleHelp)

			result := options.GetNormalizedRuleHelp()

			check.Equal(t, testCase.expectedRule, result)
		})
	}
}

func TestOutputOptions_WithMethodsSetPropertiesCorrectly(t *testing.T) {
	t.Parallel()

	t.Run("default options", func(t *testing.T) {
		t.Parallel()

		buf := &bytes.Buffer{}
		options := NewOutputOptions(buf)
		check.False(t, options.ShowHelp)
		check.Empty(t, options.RuleHelp)
		check.Equal(t, "default", options.Theme)
	})

	t.Run("with rule help set", func(t *testing.T) {
		t.Parallel()

		buf := &bytes.Buffer{}
		options := NewOutputOptions(buf).WithRuleHelp("subject")
		check.False(t, options.ShowHelp)
		check.Equal(t, "subject", options.RuleHelp)
	})

	t.Run("with theme set", func(t *testing.T) {
		t.Parallel()

		buf := &bytes.Buffer{}
		options := NewOutputOptions(buf).WithTheme("dracula")
		check.Equal(t, "dracula", options.Theme)
	})
}

func TestOutputOptions_ShouldUseColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		colorSetting     string
		writer           func() *os.File
		expectedUseColor bool
	}{
		{
			name:             "always use color",
			colorSetting:     colorAlways,
			writer:           func() *os.File { return os.Stdout },
			expectedUseColor: true,
		},
		{
			name:             "never use color",
			colorSetting:     colorNever,
			writer:           func() *os.File { return os.Stdout },
			expectedUseColor: false,
		},
		{
			name:             "auto with stdout in test",
			colorSetting:     colorAuto,
			writer:           func() *os.File { return os.Stdout },
			expectedUseColor: false,
		},
		{
			name:             "auto with stderr in test",
			colorSetting:     colorAuto,
			writer:           func() *os.File { return os.Stderr },
			expectedUseColor: false,
		},
		{
			name:             "default setting in test",
			colorSetting:     "",
			writer:           func() *os.File { return os.Stdout },
			expectedUseColor: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writer := testCase.writer()
			options := NewOutputOptions(writer).WithColor(testCase.colorSetting)

			result := options.ShouldUseColor()

			check.Equal(t, testCase.expectedUseColor, result)
		})
	}
}

func TestOutputOptions_WithResolvedEnvironment(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	env := func(key string) string {
		switch key {
		case "GITHUB_JOB":
			return "lint"
		case "NO_COLOR":
			return "1"
		default:
			return ""
		}
	}

	options := NewOutputOptions(&buf).
		WithColor(colorAlways).
		WithResolvedEnvironment(env)

	check.Equal(t, "lint", options.GetAnnotationTitle())
	check.False(t, options.ShouldUseColor())
}

func TestOutputOptions_WithUseColorOverridesEnvironmentFallback(t *testing.T) {
	t.Parallel()

	options := NewOutputOptions(os.Stdout).
		WithColor(colorNever).
		WithUseColor(true)

	check.True(t, options.ShouldUseColor())
}

// TestShouldUseColorWithEnv_DecisionMatrix exercises the REAL colour-decision
// function (shouldUseColorWithEnv) across the flag × NO_COLOR × TERM matrix,
// with a non-terminal writer (isTTY=false). This is the unit-test half of the
// no-ANSI conformance contract; the isTTY=true `auto` path is covered by the
// PTY blackbox test. Precedence under test: NO_COLOR > TERM=dumb > --color.
func TestShouldUseColorWithEnv_DecisionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		color   string
		noColor string
		term    string
		want    bool
	}{
		// NO_COLOR wins over everything, including --color always.
		{name: "NO_COLOR overrides always", color: colorAlways, noColor: "1", term: "", want: false},
		{name: "NO_COLOR overrides auto", color: colorAuto, noColor: "1", term: "", want: false},
		{name: "NO_COLOR overrides even TERM", color: colorAlways, noColor: "1", term: "xterm", want: false},
		// TERM=dumb disables colour when NO_COLOR is unset.
		{name: "TERM=dumb overrides always", color: colorAlways, noColor: "", term: "dumb", want: false},
		{name: "TERM=dumb overrides auto", color: colorAuto, noColor: "", term: "dumb", want: false},
		// --color always / never are honoured on a non-dumb terminal.
		{name: "always on non-TTY", color: colorAlways, noColor: "", term: "xterm", want: true},
		{name: "never on non-TTY", color: colorNever, noColor: "", term: "xterm", want: false},
		// auto with a non-terminal writer stays off.
		{name: "auto on non-TTY writer", color: colorAuto, noColor: "", term: "xterm", want: false},
		{name: "default (empty) on non-TTY writer", color: "", noColor: "", term: "xterm", want: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			env := func(key string) string {
				switch key {
				case "NO_COLOR":
					return testCase.noColor
				case "TERM":
					return testCase.term
				default:
					return ""
				}
			}

			// A bytes.Buffer is not an *os.File, so isTerminal reports false.
			got := shouldUseColorWithEnv(testCase.color, &bytes.Buffer{}, env)
			check.Equal(t, testCase.want, got,
				"%s: shouldUseColorWithEnv(%q, buf, env)", testCase.name, testCase.color)
		})
	}
}

func TestOutputOptions_IsTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		writer         func() any
		expectedResult bool
	}{
		{
			name: "stdout is terminal",
			writer: func() any {
				return os.Stdout
			},
			expectedResult: false, // In test environment, stdout is not a terminal
		},
		{
			name: "stderr is terminal",
			writer: func() any {
				return os.Stderr
			},
			expectedResult: false, // In test environment, stderr is not a terminal
		},
		{
			name: "bytes buffer is not terminal",
			writer: func() any {
				return &bytes.Buffer{}
			},
			expectedResult: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			writer := testCase.writer()

			w, ok := writer.(interface {
				Write(p []byte) (n int, err error)
			})
			check.True(t, ok)

			result := isTerminal(w)

			check.Equal(t, testCase.expectedResult, result)
		})
	}
}

func TestOutputOptions_WriteReport(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	options := NewOutputOptions(buf)

	report := domain.Report{
		Summary: domain.ReportSummary{
			AllPassed:     true,
			TotalCommits:  1,
			PassedCommits: 1,
			FailedCommits: 0,
		},
		Commits: []domain.CommitReport{
			{
				Commit: domain.Commit{
					Hash:    cliTestCommitHashABC,
					Subject: "Test commit",
				},
				RuleResults: []domain.RuleReport{},
				Passed:      true,
			},
		},
	}

	err := options.WriteReport(report)
	check.NoError(t, err)
	check.Positive(t, buf.Len())
}

func TestRuleNames_ReturnsAllValidOptions(t *testing.T) {
	t.Parallel()

	t.Run("returns all valid rule names", func(t *testing.T) {
		t.Parallel()

		rules := getValidRuleNames()

		expected := settings.GetValidRuleNames()
		slices.Sort(expected)

		check.Equal(t, expected, rules)
	})

	t.Run("returns sorted list", func(t *testing.T) {
		t.Parallel()

		rules := getValidRuleNames()

		// Verify the list is sorted alphabetically
		check.True(t, slices.IsSorted(rules), "rule names should be sorted alphabetically")
	})
}

// TestOutputOptionsMethodChaining tests method chaining behavior.
// TestOutputOptionsChaining_EachSetterLandsOnItsOwnField chains the setters
// setters and checks every field afterwards, which catches a setter writing to
// the wrong field and a setter dropping values set before it.
//
// This is the one round-trip test the file needs. The setters are plain
// assignments on a value receiver, so a level of 999 or MaxInt32 traverses the
// same code as 3: TestOutputOptionsBoundaryValues asserted those and added
// nothing. TestOutputOptions_RepeatedCalls compared two variables that had both
// been set to 3 and called the result idempotent -- with WithVerboseLevel
// changed to ignore its argument entirely it still passed, while this test and
// TestOutputOptions_VerboseLevelInteraction both failed.
//
// Non-mutation of the receiver is covered at the end of
// TestOutputOptionsThreadSafety.
func TestOutputOptionsChaining_EachSetterLandsOnItsOwnField(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	baseOpts := NewOutputOptions(&buf)

	// Test complex chaining
	finalOpts := baseOpts.
		WithVerboseLevel(3).
		WithQuiet(true).
		WithFormat("json").
		WithColor(colorNever).
		WithRuleHelp("subject")

	// Verify all settings were applied
	check.Equal(t, 3, finalOpts.VerboseLevel, "verbose level should be 3")
	check.True(t, finalOpts.Quiet, "quiet should be true")
	check.Equal(t, "json", finalOpts.Format, "format should be json")
	check.Equal(t, colorNever, finalOpts.Color, "color should be never")
	check.Equal(t, "subject", finalOpts.RuleHelp, "rule help should be subject")
	check.Equal(t, &buf, finalOpts.Writer, "writer should be preserved")
}

// TestOutputOptions_ReturnsNewInstances tests that methods return new instances.

// TestOutputOptionsWithNilWriter tests behavior with nil writer.
func TestOutputOptionsWithNilWriter(t *testing.T) {
	t.Parallel()

	// Test creation with nil writer
	check.NotPanics(t, func() {
		opts := NewOutputOptions(nil)

		// Test that methods still work with nil writer
		opts = opts.WithVerboseLevel(2)
		check.Equal(t, 2, opts.VerboseLevel)

		opts = opts.WithQuiet(true)
		check.True(t, opts.Quiet)
	}, "should handle nil writer gracefully")
}

func TestOutputOptions_IsFile(t *testing.T) {
	t.Parallel()

	t.Run("returns false without closer", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		opts := NewOutputOptions(&buf)

		check.False(t, opts.IsFile())
	})

	t.Run("returns true with closer", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		opts := NewOutputOptions(&buf).WithCloser(&mockCloser{})

		check.True(t, opts.IsFile())
	})
}

func TestOutputOptions_CloseWithCloser(t *testing.T) {
	t.Parallel()

	t.Run("calls closer when set", func(t *testing.T) {
		t.Parallel()

		closer := &mockCloser{}

		var buf bytes.Buffer

		opts := NewOutputOptions(&buf).WithCloser(closer)

		err := opts.Close()

		check.NoError(t, err)
		check.True(t, closer.closed)
	})

	t.Run("returns closer error", func(t *testing.T) {
		t.Parallel()

		closer := &mockCloser{err: errMockClose}

		var buf bytes.Buffer

		opts := NewOutputOptions(&buf).WithCloser(closer)

		err := opts.Close()

		check.ErrorIs(t, err, errMockClose)
	})

	t.Run("returns nil without closer", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		opts := NewOutputOptions(&buf)

		err := opts.Close()

		check.NoError(t, err)
	})
}

// mockCloser implements io.Closer for testing.
type mockCloser struct {
	closed bool
	err    error
}

func (m *mockCloser) Close() error {
	m.closed = true

	return m.err
}

// errMockClose is a test error for close failures.
var errMockClose = errors.New("mock close error")

// TestOutputOptionsThreadSafety tests thread safety of immutable operations.
func TestOutputOptionsThreadSafety(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	baseOpts := NewOutputOptions(&buf)

	// Test concurrent access to immutable methods
	done := make(chan bool, 100)

	for level := range 100 {
		go func(lvl int) {
			defer func() { done <- true }()

			// Each goroutine creates its own modified options
			opts := baseOpts.
				WithVerboseLevel(lvl).
				WithQuiet(lvl%2 == 0)

			// Verify the options were set correctly (can't use require in goroutine)
			if opts.VerboseLevel != lvl {
				t.Errorf("VerboseLevel should be %d, got %d", lvl, opts.VerboseLevel)
			}

			if opts.Quiet != (lvl%2 == 0) {
				t.Errorf("Quiet should be %v, got %v", lvl%2 == 0, opts.Quiet)
			}
		}(level)
	}

	// Wait for all goroutines to complete
	for range 100 {
		<-done
	}

	// Original options should be unchanged
	check.Equal(t, 0, baseOpts.VerboseLevel)
	check.False(t, baseOpts.Quiet)
}

// TestReportRenderers_CompleteAgainstFormatTable is the completeness invariant
// that closes the silent-fallback gap: every supported output format must have
// a reportRenderers entry. Without this, a format added to the metadata table
// but not wired here would silently fall back to the buffered string path.
func TestReportRenderers_CompleteAgainstFormatTable(t *testing.T) {
	t.Parallel()

	for _, format := range output.SupportedFormats() {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			_, ok := reportRenderers[format]
			check.True(t, ok,
				"format %q is in the output metadata table but has no reportRenderers entry", format)
		})
	}
}
