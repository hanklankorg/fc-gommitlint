// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func requireCommitBodyValidation(t *testing.T, errors []domain.ValidationError, expectError bool, expectedErrCode expectedReason) {
	t.Helper()

	if !expectError {
		check.Empty(t, errors, "expected no validation errors")

		return
	}

	requireCommitBodyErrorCode(t, errors, expectedErrCode)
}

func requireCommitBodyErrorCode(t *testing.T, errors []domain.ValidationError, expectedErrCode expectedReason) {
	t.Helper()
	check.NotEmpty(t, errors, "expected validation error")

	for _, bodyErr := range errors {
		if reasonOf(bodyErr) == expectedErrCode {
			return
		}
	}

	check.Failf(t, "missing validation error code", "expected error code %s not found", expectedErrCode)
}

func commitBodyConfig(body settings.BodyConfig) settings.Config {
	return settings.Config{Message: settings.MessageConfig{Body: body}}
}

func validateCommitBody(cfg settings.Config, message string) []domain.ValidationError {
	rule := rules.NewCommitBodyRule(cfg)
	commit := domain.ParseCommitMessage(message)

	return rule.Validate(commit)
}

func TestCommitBodyRule_BlankLineValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		isMergeCommit   bool
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name:        "Valid structure with blank line",
			message:     "feat: add feature\n\nThis is the body explaining the feature",
			expectError: false,
		},
		{
			name:            "Missing blank line",
			message:         "feat: add feature\nThis is the body without blank line",
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMissingBlankLine,
		},
		{
			name:        "Subject only - no error",
			message:     testConventionalFeatAdd,
			expectError: false,
		},
		{
			name:        "Multiple blank lines - still valid",
			message:     "feat: add feature\n\n\nThis is the body with multiple blank lines",
			expectError: false,
		},
		{
			name:            "Merge commit is validated at rule level (dispatcher handles skip)",
			message:         "Merge branch 'feature'\nSome content",
			isMergeCommit:   true,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMissingBlankLine,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewCommitBodyRule(settings.Config{})
			commit := domain.ParseCommitMessage(testCase.message)
			commit.IsMergeCommit = testCase.isMergeCommit

			errors := rule.Validate(commit)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)
		})
	}
}

func TestCommitBodyRule_StructureValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          settings.Config
		message         string
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name: "Valid three-part structure when required",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(10),
			}),
			message:     "feat: add feature\n\nDetailed explanation of the feature",
			expectError: false,
		},
		{
			name: "Missing body when body required",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(10),
			}),
			message:         testConventionalFeatAdd,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMissingBody,
		},
		{
			name:        "No structure requirement when body not required",
			config:      commitBodyConfig(settings.BodyConfig{Required: new(false)}),
			message:     testConventionalFeatAdd,
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validateCommitBody(testCase.config, testCase.message)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)
		})
	}
}

func TestCommitBodyRule_SignOffPositioning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		message         string
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name: "Valid sign-off at end",
			message: `feat: add feature

This is a detailed explanation of the feature.

Signed-off-by: John Doe <john@example.com>`,
			expectError: false,
		},
		{
			name: "Multiple sign-offs at end",
			message: `feat: add feature

This is a detailed explanation.

Signed-off-by: John Doe <john@example.com>
Signed-off-by: Jane Smith <jane@example.com>`,
			expectError: false,
		},
		{
			name: "Co-authored-by after sign-off is invalid",
			message: `feat: add feature

This is a detailed explanation.

Signed-off-by: John Doe <john@example.com>
Co-authored-by: Jane Smith <jane@example.com>
Reviewed-by: Bob Wilson <bob@example.com>`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMisplacedSignoff,
		},
		{
			name: "Trailers after sign-off are invalid",
			message: `feat: add feature

This fixes a critical bug in the authentication system.

Signed-off-by: John Doe <john@example.com>
Co-authored-by: Jane Smith <jane@example.com>
Reviewed-by: Bob Wilson <bob@example.com>
Tested-by: Alice Cooper <alice@example.com>
Reported-by: Charlie Brown <charlie@example.com>
Fixes: #123
Closes: https://github.com/org/repo/issues/456`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMisplacedSignoff,
		},
		{
			name: "Content after sign-off",
			message: `feat: add feature

This is a detailed explanation.

Signed-off-by: John Doe <john@example.com>

Additional content after sign-off`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMisplacedSignoff,
		},
		{
			name: "Sign-off in middle",
			message: `feat: add feature

This is a detailed explanation.

Signed-off-by: John Doe <john@example.com>

More explanation here`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMisplacedSignoff,
		},
		{
			name: "No sign-offs - no error",
			message: `feat: add feature

This is a detailed explanation without any sign-offs.`,
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewCommitBodyRule(settings.Config{})
			commit := domain.ParseCommitMessage(testCase.message)

			errors := rule.Validate(commit)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)
		})
	}
}

func TestCommitBodyRule_BodyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          settings.Config
		message         string
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name: "Minimum length validation",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(20),
			}),
			message:         "feat: add feature\n\nShort body",
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyTooShort,
		},
		{
			name: "Minimum length counts characters, not bytes",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(10),
			}),
			// 9 characters, 18 bytes: counting bytes would accept this body.
			message:         "feat: add feature\n\n" + strings.Repeat("ä", 9),
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyTooShort,
		},
		{
			name: "Minimum length met by multibyte characters",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(10),
			}),
			message:     "feat: add feature\n\n" + strings.Repeat("ä", 10),
			expectError: false,
		},
		{
			name: "Sign-off only when not allowed",
			config: commitBodyConfig(settings.BodyConfig{
				Required:         new(true),
				AllowSignoffOnly: new(false),
			}),
			message:         "feat: add feature\n\nSigned-off-by: John Doe <john@example.com>",
			expectError:     true,
			expectedErrCode: diagnostics.ReasonInvalidBody,
		},
		{
			name: "Sign-off only when allowed",
			config: commitBodyConfig(settings.BodyConfig{
				Required:         new(true),
				AllowSignoffOnly: new(true),
			}),
			message:     "feat: add feature\n\nSigned-off-by: John Doe <john@example.com>",
			expectError: false,
		},
		{
			name: "Sign-off first when not allowed",
			config: commitBodyConfig(settings.BodyConfig{
				Required:         new(true),
				AllowSignoffOnly: new(false),
			}),
			// Not sign-off-only, but the descriptive content comes after the
			// sign-off; the misplaced sign-off is reported alongside.
			message:         "feat: add feature\n\nSigned-off-by: John Doe <john@example.com>\nThe explanation comes after the sign-off",
			expectError:     true,
			expectedErrCode: diagnostics.ReasonInvalidBody,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validateCommitBody(testCase.config, testCase.message)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)
		})
	}
}

func TestCommitBodyRule_EmptyAndMalformedBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          settings.Config
		message         string
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name:            "Empty body when required",
			config:          commitBodyConfig(settings.BodyConfig{Required: new(true)}),
			message:         testConventionalFeatAdd,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMissingBody,
		},
		{
			name: "RequireSignoff true but no signoff",
			config: commitBodyConfig(settings.BodyConfig{
				Required:        new(true),
				MinSignoffCount: new(1),
			}),
			message:     "feat: add feature\n\nThis is a detailed body without signoff",
			expectError: false, // SignOff rule handles this, not CommitBody
		},
		{
			name: "Zero minLength when required",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(0),
			}),
			message:     "feat: add feature\n\nX",
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validateCommitBody(testCase.config, testCase.message)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)
		})
	}
}

func TestCommitBodyRule_MaxLineLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          settings.Config
		message         string
		expectError     bool
		expectedErrCode expectedReason
		expectedLines   int // number of lines that should fail
	}{
		{
			name:   "Lines within limit",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

This line is short.
This line is also short and within the 72 char limit.`,
			expectError: false,
		},
		{
			name:   "Line exceeds limit",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

This line is way too long and exceeds the seventy-two character limit that we have configured for body lines.`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Multiple lines exceed limit",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(50)}),
			message: `feat: add feature

This is a very long line that exceeds fifty characters limit.
This is another very long line that also exceeds the limit.
Short line.`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   2,
		},
		{
			name:   "Zero max length disables check",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(0)}),
			message: `feat: add feature

This line is extremely long but should not fail because max line length is set to zero which disables the check entirely.`,
			expectError: false,
		},
		{
			name:        "Empty body",
			config:      commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message:     testConventionalFeatAdd,
			expectError: false,
		},
		{
			name:   "Identity trailer is exempt",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

Short body.

Signed-off-by: A Very Long Author Name With A Very Long Email Address That Exceeds The Line Limit <verylongemailaddress@verylongdomainname.example.com>`,
			expectError: false,
		},
		{
			name:   "Every identity trailer key is exempt",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(40)}),
			message: `feat: add feature

Short body.

Co-authored-by: A Rather Long Author Name <author@example.com>
Reviewed-by: A Rather Long Reviewer Name <reviewer@example.com>
Acked-by: A Rather Long Maintainer Name <maintainer@example.com>
Tested-by: A Rather Long Tester Name <tester@example.com>
Reported-by: A Rather Long Reporter Name <reporter@example.com>`,
			expectError: false,
		},
		{
			name:   "Prose after the trailer address is checked",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

Short body.

Reviewed-by: Bob <bob@example.com> and I also think we should rewrite the whole module`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Prose before the trailer address is not a name",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

Short body.

Signed-off-by: see <https://example.com/x> for the long story of this change <bob@example.com>`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Indented trailer is body text",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(40)}),
			message: `feat: add feature

Short body.

  Signed-off-by: A Rather Long Author Name <author@example.com>`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Other trailer keys are checked",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(40)}),
			message: `feat: add feature

Short body.

Helped-by: A Rather Long Helper Name <helper@example.com>`,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Exactly at limit",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			// Exactly 72 characters in two tokens: a single token is exempt
			// whatever its length, so it could not see the limit.
			message:     "feat: add feature\n\n" + strings.Repeat("x", 36) + " " + strings.Repeat("x", 35),
			expectError: false,
		},
		{
			name:   "Multibyte characters count as one each",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			// 72 characters, 144 bytes: counting bytes would flag this line.
			message:     "feat: add feature\n\n" + strings.Repeat("ä", 36) + " " + strings.Repeat("ö", 35),
			expectError: false,
		},
		{
			name:            "Multibyte line one over limit",
			config:          commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message:         "feat: add feature\n\n" + strings.Repeat("ä", 36) + " " + strings.Repeat("ö", 36),
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "One over limit",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			// 73 characters - just over
			message:         "feat: add feature\n\n" + strings.Repeat("x", 36) + " " + strings.Repeat("x", 36),
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyLineTooLong,
			expectedLines:   1,
		},
		{
			name:   "Single unbreakable token is exempt",
			config: commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message: `feat: add feature

See the discussion at:
https://example.com/very/long/path/that/cannot/be/wrapped/because/it/is/one/token`,
			expectError: false,
		},
		{
			name:        "Indented single token is exempt",
			config:      commitBodyConfig(settings.BodyConfig{MaxLineLength: new(72)}),
			message:     "feat: add feature\n\nReference:\n    " + strings.Repeat("x", 80),
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validateCommitBody(testCase.config, testCase.message)

			if testCase.expectError {
				check.NotEmpty(t, errors, "expected validation error")

				errorCount := 0

				for _, bodyErr := range errors {
					if reasonOf(bodyErr) == testCase.expectedErrCode {
						errorCount++

						// Verify context contains line number info
						check.Contains(t, bodyErr.Context, "line", "error should contain line number")
						check.Contains(t, bodyErr.Context, diagnostics.ContextActual, "error should contain actual length")
						check.Contains(t, bodyErr.Context, diagnostics.ContextExpected, "error should contain expected length")
					}
				}

				if testCase.expectedLines > 0 {
					check.Equal(t, testCase.expectedLines, errorCount,
						"expected %d lines to fail, got %d", testCase.expectedLines, errorCount)
				}
			} else {
				// Filter out non-line-length errors for this test
				var lineLengthErrors []domain.ValidationError

				for _, bodyErr := range errors {
					if reasonOf(bodyErr) == diagnostics.ReasonBodyLineTooLong {
						lineLengthErrors = append(lineLengthErrors, bodyErr)
					}
				}

				check.Empty(t, lineLengthErrors, "expected no body line length errors")
			}
		})
	}
}

func TestCommitBodyRule_MaxLineLength_ContextNamesTheLineAndItsLength(t *testing.T) {
	t.Parallel()

	cfg := commitBodyConfig(settings.BodyConfig{MaxLineLength: new(20)})
	// Body lines 1 and 3 are over the limit (28 and 31 characters); line 2 is
	// not. Line numbers count from the first body line.
	errors := validateCommitBody(cfg, "feat: add feature\n\nthis first body line is long\nshort line\nthe third line is also too long")

	check.Len(t, errors, 2)
	check.Equal(t, diagnostics.Context{
		diagnostics.ContextLine:     "1",
		diagnostics.ContextActual:   "28",
		diagnostics.ContextExpected: "max 20",
		diagnostics.ContextMax:      "20",
	}, errors[0].Context)
	check.Equal(t, diagnostics.Context{
		diagnostics.ContextLine:     "3",
		diagnostics.ContextActual:   "31",
		diagnostics.ContextExpected: "max 20",
		diagnostics.ContextMax:      "20",
	}, errors[1].Context)
}

func TestCommitBodyRule_CompleteValidation(t *testing.T) {
	t.Parallel()
	// Test complete functionality
	t.Run("complex_commit_with_all_validations", func(t *testing.T) {
		t.Parallel()

		config := commitBodyConfig(settings.BodyConfig{
			Required:         new(true),
			MinLength:        new(20),
			AllowSignoffOnly: new(false),
		})

		// Valid complex commit
		validMessage := `feat: implement user authentication

This commit implements a user authentication system
with support for JWT tokens and refresh token rotation.

The implementation includes proper error handling and security
measures to prevent common attack vectors.

Signed-off-by: John Doe <john@example.com>`

		errors := validateCommitBody(config, validMessage)
		check.Empty(t, errors, "valid complex commit should pass all validations")

		// Invalid complex commit: the missing blank line after the subject is
		// reported even though later problems (content after sign-off) also exist.
		invalidMessage := `feat: implement user authentication
This commit implements authentication without blank line.

Signed-off-by: John Doe <john@example.com>

Additional content after sign-off`

		errors = validateCommitBody(config, invalidMessage)
		requireCommitBodyErrorCode(t, errors, diagnostics.ReasonMissingBlankLine)
	})
}

// TestCommitBodyRule_MinLengthAppliesToAnyBody pins that min_length judges
// whatever body is present on its own (0 disables it), while whether a body
// must exist at all stays with required.
func TestCommitBodyRule_MinLengthAppliesToAnyBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		config          settings.Config
		message         string
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name:            "short body without required",
			config:          commitBodyConfig(settings.BodyConfig{MinLength: new(20)}),
			message:         "feat: add feature\n\nShort body",
			expectError:     true,
			expectedErrCode: diagnostics.ReasonBodyTooShort,
		},
		{
			name:        "long enough body without required",
			config:      commitBodyConfig(settings.BodyConfig{MinLength: new(10)}),
			message:     "feat: add feature\n\nShort body",
			expectError: false,
		},
		{
			name:        "missing body without required is governed by required only",
			config:      commitBodyConfig(settings.BodyConfig{MinLength: new(20)}),
			message:     testConventionalFeatAdd,
			expectError: false,
		},
		{
			name: "missing body with required reports the missing body, not its length",
			config: commitBodyConfig(settings.BodyConfig{
				Required:  new(true),
				MinLength: new(20),
			}),
			message:         testConventionalFeatAdd,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMissingBody,
		},
		{
			name:        "zero disables the length check",
			config:      commitBodyConfig(settings.BodyConfig{MinLength: new(0)}),
			message:     "feat: add feature\n\nX",
			expectError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := validateCommitBody(testCase.config, testCase.message)

			requireCommitBodyValidation(t, errors, testCase.expectError, testCase.expectedErrCode)

			if testCase.expectError {
				check.Len(t, errors, 1)
			}
		})
	}
}
