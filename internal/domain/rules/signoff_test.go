// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// createSignoffTestCommit creates a test commit with the given message.
func createSignoffTestCommit(message string) domain.Commit {
	commit := domain.Commit{
		Hash:          "abc123def456",
		Subject:       message,
		Message:       message,
		Author:        "Test User",
		AuthorEmail:   "test@example.com",
		CommitDate:    time.Now().Format(time.RFC3339),
		IsMergeCommit: false,
	}

	// Parse subject and body from message
	lines := strings.Split(message, "\n")
	if len(lines) > 0 {
		commit.Subject = lines[0]
	}

	// Parse body - everything after the subject line
	if len(lines) > 1 {
		bodyStart := 1

		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "" {
				bodyStart = i + 1

				break
			}
		}

		if bodyStart < len(lines) {
			commit.Body = strings.Join(lines[bodyStart:], "\n")
		} else if bodyStart == 1 {
			commit.Body = strings.Join(lines[1:], "\n")
		}
	}

	return commit
}

func newSignoffRule(minCount int) rules.SignOffRule {
	return rules.NewSignOffRule(settings.Config{
		Message: settings.MessageConfig{
			Body: settings.BodyConfig{
				MinSignoffCount: new(minCount),
			},
		},
	})
}

func requireSignoffValidation(t *testing.T, failures []domain.ValidationError, wantValid bool, description string, wantCode expectedReason) {
	t.Helper()

	if wantValid {
		check.Empty(t, failures, "Expected no validation errors for case: %s", description)

		return
	}

	check.NotEmpty(t, failures, "Expected validation errors but got none for case: %s", description)

	if wantCode != "" {
		check.Equal(t, wantCode, reasonOf(failures[0]), "Expected specific error code")
	}
}

func requireSignoffMessageContains(t *testing.T, failures []domain.ValidationError, contains string, description string) {
	t.Helper()

	if contains == "" {
		return
	}

	found := false

	for _, failure := range failures {
		if strings.Contains(failure.Message, contains) {
			found = true

			break
		}
	}

	check.True(t, found, "Expected error message to contain '%s' for case: %s", contains, description)
}

func TestSignOffRule(t *testing.T) {
	t.Parallel()

	// Standard sign-off format for tests
	const validSignOff = "Signed-off-by: Dev Eloper <dev@example.com>"

	tests := []struct {
		name            string
		message         string
		minSignoffCount int
		expectedValid   bool
		errorContains   string
		description     string
	}{
		{
			name: "Valid sign-off",
			message: `Add feature
This is a detailed description of the feature.
` + validSignOff,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with valid sign-off",
		},

		{
			name:            "Valid sign-off with multiple signers",
			message:         testMultiSignoffMessage,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with multiple sign-offs",
		},
		{
			name: "Missing sign-off (required)",
			message: `Add feature
This is a detailed description of the feature.`,
			minSignoffCount: 1,
			expectedValid:   false,
			errorContains:   testMissingRequiredSignoff,
			description:     "Should fail when sign-off is required but missing",
		},
		{
			name: "Missing sign-off (not required)",
			message: `Add feature
This is a detailed description of the feature.`,
			minSignoffCount: 0,
			expectedValid:   true,
			description:     "Should pass when sign-off is not required",
		},
		{
			name:            "Empty message",
			message:         "",
			minSignoffCount: 1,
			expectedValid:   false,
			errorContains:   testMissingRequiredSignoff,
			description:     "Should fail with empty message when sign-off required",
		},
		{
			name:            "Whitespace only message",
			message:         "   \t\n  ",
			minSignoffCount: 1,
			expectedValid:   false,
			errorContains:   testMissingRequiredSignoff,
			description:     "Should fail with whitespace-only message when sign-off required",
		},
		// Additional test cases
		{
			name: "Multiple sign-offs are allowed by default",
			message: `Fix critical bug
Signed-off-by: Developer One <dev1@example.com>
Signed-off-by: Developer Two <dev2@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Multiple sign-offs are actually allowed by this rule",
		},
		{
			name: "Single sign-off when required",
			message: `Fix bug
Detailed description of the fix.
Signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with single sign-off when required",
		},
		{
			name: "Sign-off with complex email format",
			message: `Update documentation
Signed-off-by: John Doe Jr. <john.doe+dev@company.co.uk>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle complex email formats in sign-offs",
		},
		{
			name: "Sign-off with special characters in name",
			message: `Fix encoding issue
Signed-off-by: José María González <jose@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle special characters in names",
		},
		{
			name: "Malformed sign-off missing email is rejected",
			message: `Add feature
Signed-off-by: Developer Name`,
			minSignoffCount: 1,
			expectedValid:   false,
			errorContains:   testMissingRequiredSignoff,
			description:     "Strict DCO validation rejects malformed sign-offs without proper email",
		},
		{
			name: "Malformed sign-off missing name is rejected",
			message: `Add feature
Signed-off-by: <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   false,
			errorContains:   testMissingRequiredSignoff,
			description:     "Strict DCO validation rejects malformed sign-offs without proper name",
		},
		{
			name: "Sign-off must be at end of message",
			message: `Add feature
Signed-off-by: Developer <dev@example.com>
Additional description after sign-off.`,
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Sign-off is only detected at the end of message",
		},
		{
			name: "Case sensitive sign-off detection",
			message: `Fix issue
signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Sign-off detection is case sensitive",
		},
		{
			name: "Sign-off with trailing whitespace",
			message: `Update code
Signed-off-by: Developer <dev@example.com>   `,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle sign-offs with trailing whitespace",
		},
		{
			name: "Mixed case sign-offs not detected",
			message: `Collaborative fix
Signed-off-by: Developer One <dev1@example.com>
signed-off-by: Developer Two <dev2@example.com>
Signed-Off-By: Developer Three <dev3@example.com>`,
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Only exact case sign-offs are detected",
		},
		{
			name: "Sign-off without required field when not required",
			message: `Simple fix
No sign-off here`,
			minSignoffCount: 0,
			expectedValid:   true,
			description:     "Should pass without sign-off when not required",
		},
		{
			name: "Co-authored-by with sign-off",
			message: `Joint development
Co-authored-by: Partner <partner@example.com>
Signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should work with other commit metadata like Co-authored-by",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minSignoffCount)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, "")
			requireSignoffMessageContains(t, failures, testCase.errorContains, testCase.description)
		})
	}
}

func TestSignOffRuleWithConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		minSignoffCount int
		expectedValid   bool
		description     string
	}{
		{
			name: "SignOff required in config",
			message: `Add feature
This is a commit without a sign-off.`,
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Should fail when sign-off is required by config but missing",
		},
		{
			name: "SignOff not required in config",
			message: `Add feature
This is a detailed description.`,
			minSignoffCount: 0,
			expectedValid:   true,
			description:     "Should pass when sign-off is not required by config",
		},
		{
			name:            "Multiple sign-offs accepted",
			message:         testMultiSignoffMessage,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with multiple sign-offs",
		},
		{
			name:            "Multiple sign-offs with minimum count",
			message:         testMultiSignoffMessage,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Should pass when multiple sign-offs meet minimum count",
		},
		// Additional config-based test cases
		{
			name: "Valid single sign-off",
			message: `Implement feature
Detailed implementation notes.
Signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with single sign-off",
		},
		{
			name: "No sign-off with permissive config",
			message: `Quick fix
Simple one-line change.`,
			minSignoffCount: 0,
			expectedValid:   true,
			description:     "Should pass without sign-off under permissive config",
		},
		{
			name: "Complex commit with multiple contributors",
			message: `Major refactoring
This is a collaborative effort involving multiple developers.

Co-authored-by: Alice <alice@example.com>
Co-authored-by: Bob <bob@example.com>
Signed-off-by: Charlie <charlie@example.com>
Signed-off-by: Diana <diana@example.com>`,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Should handle complex multi-contributor commits",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minSignoffCount)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, "")
		})
	}
}

func TestSignOffRule_MalformedCandidateDoesNotSatisfyCount(t *testing.T) {
	t.Parallel()

	rule := newSignoffRule(2)
	errors := rule.Validate(createSignoffTestCommit(`feat: update policy

Signed-off-by: Valid Signer <valid@example.com>
Signed-off-by: Missing Email`))

	check.Len(t, errors, 2)
	check.Equal(t, diagnostics.ReasonMissingSignoff, errors[0].Diagnostic.Reason)
	check.Equal(t, diagnostics.ReasonInvalidSignoffFormat, errors[1].Diagnostic.Reason)
}

func TestSignOffRule_MalformedCandidateAtEndDoesNotCausePlacementError(t *testing.T) {
	t.Parallel()

	errors := newSignoffRule(1).Validate(createSignoffTestCommit(`feat: update policy

Signed-off-by: Missing Email`))

	check.Len(t, errors, 2)
	check.Equal(t, diagnostics.ReasonMissingSignoff, errors[0].Diagnostic.Reason)
	check.Equal(t, diagnostics.ReasonInvalidSignoffFormat, errors[1].Diagnostic.Reason)
}

func TestSignOffRule_LowercaseTrailerIsNotASignoffCandidate(t *testing.T) {
	t.Parallel()

	errors := newSignoffRule(1).Validate(createSignoffTestCommit(`feat: update policy

signed-off-by: Missing Email`))

	check.Len(t, errors, 1)
	check.Equal(t, diagnostics.ReasonMissingSignoff, errors[0].Diagnostic.Reason)
}

// TestSignOffRule_MinSignoffCount tests minSignoffCount functionality.
func TestSignOffRule_MinSignoffCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		minSignoffCount int
		expectedValid   bool
		errorCode       expectedReason
		description     string
	}{
		{
			name: "No sign-offs required (0 count)",
			message: `Fix issue
This commit has no sign-offs.`,
			minSignoffCount: 0,
			expectedValid:   true,
			description:     "Should pass when no sign-offs required",
		},
		{
			name: "Single sign-off required and present",
			message: `Fix issue
This commit has one sign-off.
Signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with single sign-off when 1 required",
		},
		{
			name: "Single sign-off required but missing",
			message: `Fix issue
This commit has no sign-offs.`,
			minSignoffCount: 1,
			expectedValid:   false,
			errorCode:       diagnostics.ReasonMissingSignoff,
			description:     "Should fail when sign-off required but missing",
		},
		{
			name: "Two sign-offs required and present",
			message: `Fix issue
This commit has two sign-offs.
Signed-off-by: Developer One <dev1@example.com>
Signed-off-by: Developer Two <dev2@example.com>`,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Should pass with two sign-offs when 2 required",
		},
		{
			name: "Two sign-offs required but only one present",
			message: `Fix issue
This commit has one sign-off.
Signed-off-by: Developer <dev@example.com>`,
			minSignoffCount: 2,
			expectedValid:   false,
			errorCode:       diagnostics.ReasonMissingSignoff,
			description:     "Should fail when 2 required but only 1 present",
		},
		{
			name: "Three sign-offs required and present",
			message: `Fix issue
This commit has three sign-offs.
Signed-off-by: Developer One <dev1@example.com>
Signed-off-by: Developer Two <dev2@example.com>
Signed-off-by: Developer Three <dev3@example.com>`,
			minSignoffCount: 3,
			expectedValid:   true,
			description:     "Should pass with three sign-offs when 3 required",
		},
		{
			name: "More sign-offs than required",
			message: `Fix issue
This commit has three sign-offs.
Signed-off-by: Developer One <dev1@example.com>
Signed-off-by: Developer Two <dev2@example.com>
Signed-off-by: Developer Three <dev3@example.com>`,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Should pass when more sign-offs than required",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minSignoffCount)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, testCase.errorCode)
		})
	}
}

// TestSignOffRule_UniqueSigners tests validation of unique signers when multiple required.
func TestSignOffRule_UniqueSigners(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		minSignoffCount int
		expectedValid   bool
		errorCode       expectedReason
		description     string
	}{
		{
			name: "Two unique signers required and present",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev Two <dev2@example.com>`,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Should pass with two different signers",
		},
		{
			name: "Two signers required but one signer signed twice",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev One <dev1@example.com>`,
			minSignoffCount: 2,
			expectedValid:   false,
			errorCode:       diagnostics.ReasonInsufficientSignoffs,
			description:     "Should fail when same signer signs twice",
		},
		{
			name: "Three unique signers required and present",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev Two <dev2@example.com>
Signed-off-by: Dev Three <dev3@example.com>`,
			minSignoffCount: 3,
			expectedValid:   true,
			description:     "Should pass with three different signers",
		},
		{
			name: "Single sign-off does not trigger unique validation",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should pass with single sign-off (uniqueness not checked)",
		},
		{
			name: "One required sign-off accepts the same signer twice",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev One <dev1@example.com>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Uniqueness is only enforced when more than one sign-off is required",
		},
		{
			name: "Mixed duplicate and unique signers",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev Two <dev2@example.com>
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev Three <dev3@example.com>`,
			minSignoffCount: 3,
			expectedValid:   false,
			errorCode:       diagnostics.ReasonInsufficientSignoffs,
			description:     "Should fail when duplicates detected in multi-signer requirement",
		},
		{
			name: "Two signers required but one signer signed twice with different email case",
			message: `Fix issue
Signed-off-by: Dev One <Dev1@Example.com>
Signed-off-by: Dev One <dev1@example.com>`,
			minSignoffCount: 2,
			expectedValid:   false,
			errorCode:       diagnostics.ReasonInsufficientSignoffs,
			description:     "Emails compare case-insensitively, as the identity rule compares them",
		},
		{
			name: "Two signers whose names differ only in case are two signers",
			message: `Fix issue
Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: dev one <dev2@example.com>`,
			minSignoffCount: 2,
			expectedValid:   true,
			description:     "Only the email identifies a signer; names are not compared",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minSignoffCount)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, testCase.errorCode)
		})
	}
}

// TestSignOffRule_PlacementValidation tests sign-off placement validation.
func TestSignOffRule_PlacementValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		message       string
		expectedValid bool
		errorCode     expectedReason
		description   string
	}{
		{
			name: "Sign-off at proper end position",
			message: `Fix issue
Detailed description here.

Signed-off-by: Developer <dev@example.com>`,
			expectedValid: true,
			description:   "Should pass with sign-off at end",
		},
		{
			name: "Sign-off with content after",
			message: `Fix issue
Signed-off-by: Developer <dev@example.com>
Additional content after sign-off`,
			expectedValid: false,
			errorCode:     diagnostics.ReasonMisplacedSignoff,
			description:   "Should fail when content appears after sign-off",
		},
		{
			name: "Sign-off with trailer after",
			message: `Fix issue
Signed-off-by: Developer <dev@example.com>
Co-authored-by: Partner <partner@example.com>`,
			expectedValid: false,
			errorCode:     diagnostics.ReasonMisplacedSignoff,
			description:   "Should fail when trailers appear after sign-off",
		},
		{
			name: "Multiple sign-offs at end",
			message: `Fix issue
Detailed description.

Signed-off-by: Dev One <dev1@example.com>
Signed-off-by: Dev Two <dev2@example.com>`,
			expectedValid: true,
			description:   "Should pass with multiple sign-offs at end",
		},
		{
			name: "Sign-off with trailing empty lines",
			message: `Fix issue

Signed-off-by: Developer <dev@example.com>


`,
			expectedValid: true,
			description:   "Should pass with trailing empty lines after sign-off",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(1)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, testCase.errorCode)
		})
	}
}

func TestSignOffRule_MalformedSignatureFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		minSignoffCount int
		expectedValid   bool
		description     string
	}{
		{
			name: "Very long sign-off line",
			message: `Fix issue
Signed-off-by: Very Long Developer Name With Multiple Middle Names And Suffixes Jr. III <very.long.email.address.with.multiple.dots.and.plus.signs+tag@very-long-domain-name.co.uk>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle very long sign-off lines",
		},
		{
			name: "Sign-off with Unicode characters",
			message: `Update internationalization
Signed-off-by: 田中太郎 <tanaka@example.jp>`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle Unicode characters in sign-offs",
		},
		{
			name: "Empty lines around sign-off",
			message: `Fix formatting


Signed-off-by: Developer <dev@example.com>


`,
			minSignoffCount: 1,
			expectedValid:   true,
			description:     "Should handle empty lines around sign-offs",
		},
		{
			name: "Partial sign-off match requires exact format",
			message: `Update code
This line mentions Signed-off-by but is not a real sign-off
Actually signed-off-by: Real Developer <real@example.com>`,
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Rule requires exact format and position",
		},
		{
			name:            "Subject-only commit without sign-off",
			message:         "Quick fix",
			minSignoffCount: 1,
			expectedValid:   false,
			description:     "Should fail for subject-only commits when sign-off required",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minSignoffCount)
			failures := rule.Validate(commit)

			requireSignoffValidation(t, failures, testCase.expectedValid, testCase.description, "")
		})
	}
}

// TestSignOffRule_ErrorContextAndHelp tests that errors include proper context and help text.
func TestSignOffRule_ErrorContextAndHelp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		message     string
		minCount    int
		wantCode    expectedReason
		wantContext map[string]string
	}{
		{
			name: "Missing sign-off error includes context",
			message: `Fix issue
No sign-offs here`,
			minCount: 1,
			wantCode: diagnostics.ReasonMissingSignoff,
			wantContext: map[string]string{
				diagnostics.ContextActual:   "0",
				diagnostics.ContextExpected: "1",
			},
		},
		{
			name: "Insufficient count error includes context",
			message: `Fix issue
Signed-off-by: Developer <dev@example.com>`,
			minCount: 2,
			wantCode: diagnostics.ReasonMissingSignoff,
			wantContext: map[string]string{
				diagnostics.ContextActual:   "1",
				diagnostics.ContextExpected: "2",
			},
		},
		{
			name: "Misplaced sign-off error includes context",
			message: `Fix issue
Signed-off-by: Developer <dev@example.com>
Content after sign-off`,
			minCount: 1,
			wantCode: diagnostics.ReasonMisplacedSignoff,
			wantContext: map[string]string{
				diagnostics.ContextActual:   "Content found after sign-off",
				diagnostics.ContextExpected: "Sign-offs at end",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := createSignoffTestCommit(testCase.message)
			rule := newSignoffRule(testCase.minCount)
			errors := rule.Validate(commit)

			check.NotEmpty(t, errors, "Expected validation error")
			check.Equal(t, testCase.wantCode, reasonOf(errors[0]))

			for key, expectedValue := range testCase.wantContext {
				check.Equal(t, expectedValue, errors[0].Context[key], "Context value mismatch for key: %s", key)
			}
		})
	}
}

// TestSignOffRule_StrictDCOCompliance tests that the SignOff rule enforces strict DCO compliance.
func TestSignOffRule_StrictDCOCompliance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantValid   bool
		wantErrCode expectedReason
		description string
	}{
		{
			name: "Valid DCO format - full name and email",
			body: `Fix critical bug in authentication

This commit fixes a security vulnerability.

Signed-off-by: John Doe <john.doe@example.com>`,
			wantValid:   true,
			description: "Proper DCO format should pass validation",
		},
		{
			name: "Valid DCO format - name with spaces",
			body: `Add new feature

Signed-off-by: Mary Jane Watson <mary.jane@company.org>`,
			wantValid:   true,
			description: "Names with spaces should be valid",
		},
		{
			name: "Valid DCO format - hyphenated email domain",
			body: `Update documentation

Signed-off-by: Developer Name <dev@sub-domain.example.com>`,
			wantValid:   true,
			description: "Complex email domains should be valid",
		},
		{
			name: "Invalid DCO format - missing email",
			body: `Fix bug

Signed-off-by: John Doe`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Sign-off without email should be rejected",
		},
		{
			name: "Invalid DCO format - missing name",
			body: `Fix bug

Signed-off-by: <john@example.com>`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Sign-off without name should be rejected",
		},
		{
			name: "Invalid DCO format - email without angle brackets",
			body: `Fix bug

Signed-off-by: John Doe john@example.com`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Email without angle brackets should be rejected",
		},
		{
			name: "Invalid DCO format - only angle brackets",
			body: `Fix bug

Signed-off-by: <>`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Empty angle brackets should be rejected",
		},
		{
			name: "Invalid DCO format - email without @ symbol",
			body: `Fix bug

Signed-off-by: John Doe <johndoeexample.com>`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Email without @ symbol should be rejected",
		},
		{
			name: "Invalid DCO format - just text without proper format",
			body: `Fix bug

Signed-off-by: some random text here`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonMissingSignoff,
			description: "Non-DCO format text should be rejected",
		},
		{
			name: "Valid DCO format - multiple valid sign-offs",
			body: `Fix bug collaboratively

This was a team effort.

Signed-off-by: Alice Developer <alice@example.com>
Signed-off-by: Bob Reviewer <bob@example.com>`,
			wantValid:   true,
			description: "Multiple valid DCO sign-offs should pass",
		},
		{
			name: "Mixed valid and invalid sign-offs",
			body: `Fix bug

Signed-off-by: Alice Developer <alice@example.com>
Signed-off-by: Invalid Line Without Email`,
			wantValid:   false,
			wantErrCode: diagnostics.ReasonInvalidSignoffFormat,
			description: "Mixed valid/invalid should report the malformed sign-off",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newSignoffRule(1)
			commit := domain.Commit{
				Subject: testSubjectTestCommit,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			requireSignoffValidation(t, errors, testCase.wantValid, testCase.description, testCase.wantErrCode)
		})
	}
}

// TestSignOffRule_DCOExamples tests real-world DCO examples to ensure compliance.
func TestSignOffRule_DCOExamples(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantValid   bool
		description string
	}{
		{
			name: "Linux kernel style DCO",
			body: `mm: fix memory leak in page allocation

This patch fixes a memory leak that occurs when page allocation
fails under memory pressure.

Signed-off-by: Linus Torvalds <torvalds@linux-foundation.org>`,
			wantValid:   true,
			description: "Linux kernel style should be valid",
		},
		{
			name: "Corporate email DCO",
			body: `security: patch CVE-2024-1234

Applied security patch for vulnerability.

Signed-off-by: Security Team <security@bigcorp.com>`,
			wantValid:   true,
			description: "Corporate email format should be valid",
		},
		{
			name: "International name DCO",
			body: `i18n: add support for Japanese characters

Added Unicode support for Japanese text rendering.

Signed-off-by: Tanaka Hiroshi <tanaka@example.jp>`,
			wantValid:   true,
			description: "International names should be valid",
		},
		{
			name: "Hyphenated name DCO",
			body: `docs: update README

Updated documentation for clarity.

Signed-off-by: Mary-Jane Smith-Wilson <mary-jane@example.com>`,
			wantValid:   true,
			description: "Hyphenated names should be valid",
		},
		{
			name: "Invalid - Git trailer without email",
			body: `fix: resolve authentication bug

Fixed login issue for users.

Signed-off-by: Developer`,
			wantValid:   false,
			description: "Git trailer without email should be invalid",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newSignoffRule(1)
			commit := domain.Commit{
				Subject: testSubjectTestCommit,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			requireSignoffValidation(t, errors, testCase.wantValid, testCase.description, "")
		})
	}
}

// TestSignOffRule_StrictValidationBehavior tests the specific behavior of strict validation.
func TestSignOffRule_StrictValidationBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		body            string
		minSignoffCount int
		expectFound     int
		expectError     bool
		description     string
	}{
		{
			name: "Two valid DCO sign-offs counted correctly",
			body: `Fix issue together

Signed-off-by: Alice <alice@example.com>
Signed-off-by: Bob <bob@example.com>`,
			minSignoffCount: 2,
			expectFound:     2,
			expectError:     false,
			description:     "Two valid sign-offs should meet requirement",
		},
		{
			name: "One valid, one invalid - only valid counted",
			body: `Fix issue together

Signed-off-by: Alice <alice@example.com>
Signed-off-by: Bob Without Email`,
			minSignoffCount: 2,
			expectFound:     1,
			expectError:     true,
			description:     "Only strictly valid sign-offs should be counted",
		},
		{
			name: "Mixed case - case sensitive validation",
			body: `Fix issue

SIGNED-OFF-BY: Alice <alice@example.com>`,
			minSignoffCount: 1,
			expectFound:     0,
			expectError:     true,
			description:     "Case sensitivity should be enforced",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := newSignoffRule(testCase.minSignoffCount)
			commit := domain.Commit{
				Subject: testSubjectTestCommit,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			if !testCase.expectError {
				check.Empty(t, errors, testCase.description)

				return
			}

			check.NotEmpty(t, errors, testCase.description)
			check.Equal(t, diagnostics.ReasonMissingSignoff, reasonOf(errors[0]), testCase.description)
			check.Equal(t, strconv.Itoa(testCase.expectFound), errors[0].Context[diagnostics.ContextActual],
				"Error should reflect actual valid sign-offs found")
			check.Equal(t, strconv.Itoa(testCase.minSignoffCount), errors[0].Context[diagnostics.ContextExpected])
		})
	}
}
