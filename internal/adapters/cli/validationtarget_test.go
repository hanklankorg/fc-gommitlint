// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const (
	// Test constants.
	testMainBranch        = "main"
	testValidationRefHead = "HEAD"
)

// TestNewValidationTarget_HigherPriorityInputWinsOverLower pins the order the
// CLI resolves its five ways of naming what to validate.
//
// Each row supplies several inputs at once and names the one that must win, so
// a reordering of the precedence chain in NewValidationTarget shows up here
// rather than as a user validating something they did not ask for.
func TestNewValidationTarget_HigherPriorityInputWinsOverLower(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		messageFile  string
		gitRef       string
		commitRange  string
		baseBranch   string
		commitCount  int
		expectedType string
		expectedSrc  string
		expectedTgt  string
		description  string
	}{
		{
			name:         "message file beats every other input",
			messageFile:  "-",
			gitRef:       testValidationRefHead,
			commitRange:  cliTestCommitRangeMainFeature,
			baseBranch:   testMainBranch,
			commitCount:  5,
			expectedType: appValidation.TargetTypeMessage,
			expectedSrc:  "-",
			description:  "should prioritize message file over all other options",
		},
		{
			name:         "base branch beats range, ref and count",
			gitRef:       testValidationRefHead,
			commitRange:  cliTestCommitRangeMainFeature,
			baseBranch:   testMainBranch,
			commitCount:  5,
			expectedType: appValidation.TargetTypeRange,
			expectedSrc:  testMainBranch,
			expectedTgt:  testValidationRefHead,
			description:  "should use base branch when no message file",
		},
		{
			name:         "base branch fallback list selects first candidate",
			baseBranch:   "main,master",
			commitCount:  1,
			expectedType: appValidation.TargetTypeRange,
			expectedSrc:  testMainBranch,
			expectedTgt:  testValidationRefHead,
			description:  "target creation should use the first parsed base branch candidate",
		},
		{
			name:         "range beats ref and count",
			gitRef:       testValidationRefHead,
			commitRange:  cliTestCommitRangeMainFeature,
			commitCount:  5,
			expectedType: appValidation.TargetTypeRange,
			expectedSrc:  testMainBranch,
			expectedTgt:  "feature",
			description:  "should use commit range when no message file or base branch",
		},
		{
			name:         "ref beats count",
			gitRef:       testMainBranch,
			commitCount:  5,
			expectedType: appValidation.TargetTypeCommit,
			expectedSrc:  testMainBranch,
			description:  "should use git reference when no higher priority options",
		},
		{
			name:         "message file ignores invalid lower priority inputs",
			messageFile:  "-",
			gitRef:       "bad;ref",
			commitRange:  "invalid-range",
			baseBranch:   "bad;branch",
			commitCount:  0,
			expectedType: appValidation.TargetTypeMessage,
			expectedSrc:  "-",
			description:  "only the selected message input should be validated",
		},
		{
			name:         "base branch ignores invalid lower priority inputs",
			gitRef:       "bad;ref",
			commitRange:  "invalid-range",
			baseBranch:   testMainBranch,
			commitCount:  0,
			expectedType: appValidation.TargetTypeRange,
			expectedSrc:  testMainBranch,
			expectedTgt:  testValidationRefHead,
			description:  "only the selected base branch should be validated",
		},
		{
			name:         "range ignores invalid lower priority inputs",
			gitRef:       "bad;ref",
			commitRange:  cliTestCommitRangeMainFeature,
			commitCount:  0,
			expectedType: appValidation.TargetTypeRange,
			expectedSrc:  testMainBranch,
			expectedTgt:  "feature",
			description:  "only the selected range should be validated",
		},
		{
			name:         "ref ignores invalid lower priority count",
			gitRef:       testMainBranch,
			commitCount:  0,
			expectedType: appValidation.TargetTypeCommit,
			expectedSrc:  testMainBranch,
			description:  "only the selected reference should be validated",
		},
		{
			name:         "count is used when nothing else is given",
			commitCount:  5,
			expectedType: appValidation.TargetTypeCount,
			expectedSrc:  "5",
			description:  "should use commit count when no other options",
		},
		{
			name:         "a count of one means HEAD",
			commitCount:  1,
			expectedType: appValidation.TargetTypeCommit,
			expectedSrc:  testValidationRefHead,
			description:  "should default to HEAD when count is 1",
		},
		{
			// Sat among the rejection rows under the name "invalid git
			// reference - too long" while expecting no error at all. Git
			// enforces its own reference limits, so length is not this
			// function's business; the row says so now.
			name:         "a long git reference is accepted",
			gitRef:       strings.Repeat("a", 300),
			commitCount:  1,
			expectedType: appValidation.TargetTypeCommit,
			expectedSrc:  strings.Repeat("a", 300),
			description:  "long git references are allowed (git handles limits)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			target, err := NewValidationTarget(
				testCase.messageFile,
				testCase.gitRef,
				testCase.commitRange,
				testCase.baseBranch,
				testCase.commitCount,
				settings.DefaultMaxCommitCount,
			)

			check.NoError(t, err, testCase.description)
			check.Equal(t, testCase.expectedType, target.Type, testCase.description)
			check.Equal(t, testCase.expectedSrc, target.Source, testCase.description)
			check.Equal(t, testCase.expectedTgt, target.Target, testCase.description)
		})
	}
}

// TestNewValidationTarget_RejectsUnsafeAndOutOfRangeInputs covers what the
// function refuses, and why.
//
// Each row asserts the specific sentinel or classified reason so it must reach
// the validation branch named by the case.
func TestNewValidationTarget_RejectsUnsafeAndOutOfRangeInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		messageFile  string
		gitRef       string
		commitRange  string
		commitCount  int
		expectErr    error              // for failures carrying a sentinel
		expectReason diagnostics.Reason // for failures carrying a classified reason
		description  string
	}{
		{
			name:        "zero commit count",
			commitCount: 0,
			expectErr:   appValidation.ErrCommitCountNotPositive,
			description: "should reject zero commit count",
		},
		{
			name:        "negative commit count",
			commitCount: -1,
			expectErr:   appValidation.ErrCommitCountNotPositive,
			description: "should reject negative commit counts",
		},
		{
			name:        "commit count above the configured maximum",
			commitCount: settings.DefaultMaxCommitCount + 1,
			expectErr:   ErrCommitCountExceedsMax,
			description: "should reject commit counts that are too large",
		},
		{
			name:        "message file path too long",
			messageFile: strings.Repeat("a", 5000),
			commitCount: 1,
			expectErr:   domain.ErrParameterTooLong,
			description: "over-long paths are rejected before the filesystem sees them",
		},
		{
			name:        "message file path with a null byte",
			messageFile: "/test/mock/with\x00null",
			commitCount: 1,
			expectErr:   domain.ErrPathDangerous,
			description: "should reject paths with null bytes",
		},
		{
			name:        "message file path with traversal",
			messageFile: "/test/mock/../../../etc/passwd",
			commitCount: 1,
			expectErr:   domain.ErrPathTraversal,
			description: "should reject paths with traversal attempts",
		},
		{
			name:         "git reference with policy metacharacters",
			gitRef:       "HEAD; rm -rf /",
			commitCount:  1,
			expectReason: diagnostics.ReasonInvalidReference,
			description:  "should reject git references with dangerous characters",
		},
		{
			// commitCount must be valid for the range to be reached at all.
			name:         "unparseable commit range",
			commitRange:  "invalid-range-format",
			commitCount:  1,
			expectReason: diagnostics.ReasonInvalidCommitRange,
			description:  "should reject invalid range formats",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if testCase.expectErr == nil && testCase.expectReason == "" {
				t.Fatal("an error row must state which failure it expects, or it asserts nothing")
			}

			_, err := NewValidationTarget(
				testCase.messageFile,
				testCase.gitRef,
				testCase.commitRange,
				"",
				testCase.commitCount,
				settings.DefaultMaxCommitCount,
			)

			check.Error(t, err, testCase.description)

			if testCase.expectErr != nil {
				check.ErrorIs(t, err, testCase.expectErr, testCase.description)
			}

			if testCase.expectReason != "" {
				var validationErr domain.ValidationError

				check.ErrorAs(t, err, &validationErr, testCase.description)
				check.Equal(t, testCase.expectReason, validationErr.Diagnostic.Reason, testCase.description)
			}
		})
	}
}

func TestValidationTarget_TypeCheckers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		target            ValidationTarget
		expectedIsMessage bool
		expectedIsCommit  bool
		expectedIsRange   bool
		expectedIsCount   bool
		description       string
	}{
		{
			name: "message target",
			target: ValidationTarget{
				Type:   appValidation.TargetTypeMessage,
				Source: "/test/mock/to/file",
			},
			expectedIsMessage: true,
			expectedIsCommit:  false,
			expectedIsRange:   false,
			expectedIsCount:   false,
			description:       "should correctly identify message target",
		},
		{
			name: "commit target",
			target: ValidationTarget{
				Type:   appValidation.TargetTypeCommit,
				Source: testValidationRefHead,
			},
			expectedIsMessage: false,
			expectedIsCommit:  true,
			expectedIsRange:   false,
			expectedIsCount:   false,
			description:       "should correctly identify commit target",
		},
		{
			name: "range target",
			target: ValidationTarget{
				Type:   appValidation.TargetTypeRange,
				Source: testMainBranch,
				Target: "feature",
			},
			expectedIsMessage: false,
			expectedIsCommit:  false,
			expectedIsRange:   true,
			expectedIsCount:   false,
			description:       "should correctly identify range target",
		},
		{
			name: "count target",
			target: ValidationTarget{
				Type:   appValidation.TargetTypeCount,
				Source: "5",
			},
			expectedIsMessage: false,
			expectedIsCommit:  false,
			expectedIsRange:   false,
			expectedIsCount:   true,
			description:       "should correctly identify count target",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.expectedIsMessage, testCase.target.IsMessageFile(), testCase.description)
			check.Equal(t, testCase.expectedIsCommit, testCase.target.IsCommit(), testCase.description)
			check.Equal(t, testCase.expectedIsRange, testCase.target.IsRange(), testCase.description)
			check.Equal(t, testCase.expectedIsCount, testCase.target.IsCount(), testCase.description)
		})
	}
}

func TestValidationTarget_InvalidReferenceUsesDiagnosticID(t *testing.T) {
	t.Parallel()

	_, err := NewValidationTarget("", "HEAD; rm -rf /", "", "", 1, settings.DefaultMaxCommitCount)

	check.Error(t, err)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ID("GL903"), validationErr.Diagnostic.ID)
	check.Equal(t, diagnostics.ReasonInvalidReference, validationErr.Diagnostic.Reason)
}

func TestCommitCount_ValidatesPositiveIntegers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		count       int
		expectError bool
		description string
	}{
		{
			name:        "positive count is valid",
			count:       5,
			expectError: false,
			description: "should accept positive counts",
		},
		{
			name:        "large valid count",
			count:       100,
			expectError: false,
			description: "should accept large valid counts",
		},
		{
			name:        "maximum allowed count",
			count:       settings.DefaultMaxCommitCount,
			expectError: false,
			description: "should accept maximum allowed count",
		},
		{
			name:        cliTestZeroCountName,
			count:       0,
			expectError: true,
			description: "should reject zero counts",
		},
		{
			name:        cliTestNegativeCountName,
			count:       -1,
			expectError: true,
			description: "should reject negative counts",
		},
		{
			name:        "count exceeds maximum",
			count:       settings.DefaultMaxCommitCount + 1,
			expectError: true,
			description: "should reject counts that exceed maximum",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validateCommitCount(testCase.count, settings.DefaultMaxCommitCount)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestParseBaseBranchCandidates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "single branch", input: testMainBranch, expected: []string{testMainBranch}},
		{name: "CSV multiple", input: "main,master,develop", expected: []string{testMainBranch, cliTestMasterBranch, cliTestDevelopBranch}},
		{name: "whitespace trimmed", input: " main , master ", expected: []string{testMainBranch, cliTestMasterBranch}},
		{name: "empty entries skipped", input: ",,main,,master", expected: []string{testMainBranch, cliTestMasterBranch}},
		{name: cliTestEmptyStringName, input: "", expected: []string{}},
		{name: "only commas", input: ",,,", expected: []string{}},
		{name: "only whitespace entries", input: " , , ", expected: []string{}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ParseBaseBranchCandidates(testCase.input)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// TestValidateBaseBranch_RejectsEachHostileCandidateInACSV covers the security
// half of this parser: every candidate in a comma-separated list is validated,
// so a hostile entry cannot ride along behind a benign first one.
//
// The classified reason is asserted, not just "an error", using the same
// expectReason mechanism the target test above uses -- and for the same stated
// reason: a row that only says "this fails" asserts nothing. A bare check here
// would stay green if the null byte were rejected for some unrelated reason, or
// if the reference check were dropped and a different guard happened to fire.
// The offending candidate must also appear in the message, because with a CSV
// the operator needs to know which entry was refused.
func TestValidateBaseBranch_RejectsEachHostileCandidateInACSV(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		expectReason diagnostics.Reason
		expectNames  string
	}{
		{name: "single valid branch", input: testMainBranch},
		{name: "CSV valid branches", input: "main,master,develop"},
		{name: "CSV with whitespace", input: "main, master , develop"},
		{name: "CSV with empty entries", input: "main,,master"},
		{name: cliTestEmptyStringName, input: ""},
		{
			name:         "only commas",
			input:        ",,,",
			expectReason: diagnostics.ReasonInvalidReference,
			expectNames:  ",,,",
		},
		{
			name:         "only whitespace entries",
			input:        " , , ",
			expectReason: diagnostics.ReasonInvalidReference,
			expectNames:  " , , ",
		},
		{
			name:         "CSV with null byte in ref",
			input:        "main,bad\x00ref",
			expectReason: diagnostics.ReasonInvalidReference,
			expectNames:  "bad\x00ref",
		},
		{
			name:         "CSV with rejected metacharacters",
			input:        "main,master; rm -rf /",
			expectReason: diagnostics.ReasonInvalidReference,
			expectNames:  "master; rm -rf /",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := validateBaseBranch(testCase.input)

			if testCase.expectReason == "" {
				check.NoError(t, err)

				return
			}

			var validationErr domain.ValidationError

			check.ErrorAs(t, err, &validationErr)
			check.Equal(t, testCase.expectReason, validationErr.Diagnostic.Reason)
			check.Contains(t, err.Error(), testCase.expectNames)
		})
	}
}
