// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"sync"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

func TestNewCommit_InvalidUTF8_HandlesGracefully(t *testing.T) {
	t.Parallel()

	// Test that commits handle invalid UTF-8 gracefully
	invalidUTF8 := string([]byte{0xff, 0xfe, 0xfd})
	hash := "abc123"

	// Should not panic when parsing invalid UTF-8
	commit := NewCommit(hash, invalidUTF8, "Author", "email@example.com", "2024-01-01", "", false)
	check.NotNil(t, commit)
	check.Equal(t, hash, commit.Hash)

	// Body with invalid UTF-8 should still be accessible
	bodyWithInvalid := "Valid text " + invalidUTF8 + " more text"
	commit2 := NewCommit(hash, "Subject\n\n"+bodyWithInvalid, "Author", "email@example.com", "2024-01-01", "", false)
	check.Contains(t, commit2.Body, "Valid text")
	check.Contains(t, commit2.Body, "more text")
}

// TestNewCommit_EdgeCases_ParsesCorrectly tests edge cases and boundary conditions.
func TestNewCommit_EdgeCases_ParsesCorrectly(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name     string
		message  string
		validate func(t *testing.T, commit Commit)
	}{
		{
			name:    "empty message",
			message: "",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Empty(t, commit.Subject)
				check.Empty(t, commit.Body)
				check.Empty(t, commit.Message)
			},
		},
		{
			name:    "only whitespace",
			message: "   \n\t\n   ",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Empty(t, commit.Subject)
				check.Empty(t, commit.Body)
				check.Equal(t, "   \n\t\n   ", commit.Message)
			},
		},
		{
			name:    "single character",
			message: "x",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "x", commit.Subject)
				check.Empty(t, commit.Body)
				check.Equal(t, "x", commit.Message)
			},
		},
		{
			name:    "very long subject (over 1000 chars)",
			message: string(make([]byte, 1500)),
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				// Should still parse, even if invalid
				check.Len(t, commit.Subject, 1500)
				check.Empty(t, commit.Body)
			},
		},
		{
			name:    "unicode emojis in subject",
			message: "✨ feat: Add sparkles 🎉",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "✨ feat: Add sparkles 🎉", commit.Subject)
				check.Empty(t, commit.Body)
			},
		},
		{
			name:    "null bytes in message",
			message: "Subject\x00with\x00nulls\n\nBody\x00text",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "Subject\x00with\x00nulls", commit.Subject)
				check.Equal(t, "Body\x00text", commit.Body)
			},
		},
		{
			name:    "only newlines",
			message: "\n\n\n\n",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Empty(t, commit.Subject)
				check.Empty(t, commit.Body)
			},
		},

		{
			name:    "tabs in subject and body",
			message: "Subject\twith\ttabs\n\nBody\twith\ttabs",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "Subject\twith\ttabs", commit.Subject)
				check.Equal(t, "Body\twith\ttabs", commit.Body)
			},
		},
		{
			name:    "multiple blank lines between subject and body",
			message: "Subject\n\n\n\n\nBody after many blanks",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "Subject", commit.Subject)
				// Body is trimmed of leading/trailing whitespace
				check.Equal(t, "Body after many blanks", commit.Body)
			},
		},
		{
			name:    "no blank line between subject and body",
			message: "Subject\nBody immediately after",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				// Without blank line, it's all considered the subject
				check.Equal(t, "Subject", commit.Subject)
				check.Empty(t, commit.Body)
			},
		},
		{
			name:    "unicode control characters",
			message: "Subject\u0001\u0002\n\nBody\u0003\u0004",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "Subject\u0001\u0002", commit.Subject)
				check.Equal(t, "Body\u0003\u0004", commit.Body)
			},
		},
		{
			name:    "RTL unicode text",
			message: "מסר בעברית\n\nגוף ההודעה",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "מסר בעברית", commit.Subject)
				check.Equal(t, "גוף ההודעה", commit.Body)
			},
		},
		{
			name:    "multi-byte unicode characters",
			message: "🚀 Deploy: 部署新功能 🎯\n\n详细说明 📝",
			validate: func(t *testing.T, commit Commit) {
				t.Helper()
				check.Equal(t, "🚀 Deploy: 部署新功能 🎯", commit.Subject)
				check.Equal(t, "详细说明 📝", commit.Body)
			},
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := ParseCommitMessage(testCase.message)
			testCase.validate(t, commit)
		})
	}
}

// TestCommit_IsValid tests the IsValid method with various commit configurations.
func TestCommit_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		commit Commit
		want   bool
	}{
		{
			name: "valid commit with hash and subject",
			commit: Commit{
				Hash:    "abc123",
				Subject: "feat: add new feature",
			},
			want: true,
		},
		{
			name: "commit with empty hash",
			commit: Commit{
				Hash:    "",
				Subject: "feat: add new feature",
			},
			want: false,
		},
		{
			name: "commit with empty subject",
			commit: Commit{
				Hash:    "abc123",
				Subject: "",
			},
			want: false,
		},
		{
			name: "commit with whitespace-only subject",
			commit: Commit{
				Hash:    "abc123",
				Subject: "   \n\t  ",
			},
			want: false,
		},
		{
			name: "commit with both empty hash and subject",
			commit: Commit{
				Hash:    "",
				Subject: "",
			},
			want: false,
		},
		{
			name: "commit with subject containing leading/trailing whitespace",
			commit: Commit{
				Hash:    "abc123",
				Subject: "  feat: add new feature  ",
			},
			want: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, testCase.commit.IsValid())
		})
	}
}

// TestCommit_IsSigned tests the IsSigned method with various signature configurations.
func TestCommit_IsSigned(t *testing.T) {
	t.Parallel()

	// IsSigned just checks if signature is non-empty
	check.True(t, Commit{Signature: "any signature"}.IsSigned())
	check.False(t, Commit{Signature: ""}.IsSigned())
}

// TestSplitCommitMessage tests the SplitCommitMessage function with various message formats.
func TestSplitCommitMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		message     string
		wantSubject string
		wantBody    string
	}{
		{
			name:        "empty message",
			message:     "",
			wantSubject: "",
			wantBody:    "",
		},
		{
			name:        "single line message",
			message:     "feat: add new feature",
			wantSubject: "feat: add new feature",
			wantBody:    "",
		},
		{
			name:        "message with subject and blank line only",
			message:     "feat: add new feature\n\n",
			wantSubject: "feat: add new feature",
			wantBody:    "",
		},
		{
			name:        "message with subject and body",
			message:     "feat: add new feature\n\nThis adds a new feature to the application.",
			wantSubject: "feat: add new feature",
			wantBody:    "This adds a new feature to the application.",
		},
		{
			name:        "message with subject, blank line, and multiline body",
			message:     "feat: add new feature\n\nThis adds a new feature.\n\nIt includes:\n- Feature A\n- Feature B",
			wantSubject: "feat: add new feature",
			wantBody:    "This adds a new feature.\n\nIt includes:\n- Feature A\n- Feature B",
		},
		{
			name:        "message without proper blank line separation",
			message:     "feat: add new feature\nThis is not properly separated",
			wantSubject: "feat: add new feature",
			wantBody:    "",
		},
		{
			name:        "whitespace-only separator line still separates the body",
			message:     "fix: y\n \nBody text",
			wantSubject: "fix: y",
			wantBody:    "Body text",
		},
		{
			name:        "message with whitespace in subject",
			message:     "  feat: add new feature  \n\nBody content",
			wantSubject: "feat: add new feature",
			wantBody:    "Body content",
		},
		{
			name:        "message with only newlines",
			message:     "\n\n\n",
			wantSubject: "",
			wantBody:    "",
		},
		{
			name:        "message with trailing body whitespace",
			message:     "feat: add feature\n\n  Body with whitespace  \n\n  ",
			wantSubject: "feat: add feature",
			wantBody:    "Body with whitespace",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			subject, body := SplitCommitMessage(testCase.message)

			check.Equal(t, testCase.wantSubject, subject)
			check.Equal(t, testCase.wantBody, body)
		})
	}
}

// TestCommitHasRequiredFields tests the CommitHasRequiredFields function.

// TestValidationResult_Passed tests the Passed method of ValidationResult.
func TestValidationResult_Passed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result ValidationResult
		want   bool
	}{
		{
			name: "validation result with no errors",
			result: ValidationResult{
				Errors: []ValidationError{},
			},
			want: true,
		},
		{
			name: "validation result with single error",
			result: ValidationResult{
				Errors: []ValidationError{
					New("test", diagnostics.ReasonInvalidFormat, "Test error"),
				},
			},
			want: false,
		},
		{
			name: "validation result with multiple errors",
			result: ValidationResult{
				Errors: []ValidationError{
					New("test1", diagnostics.ReasonInvalidFormat, "Test error 1"),
					New("test2", diagnostics.ReasonInvalidFormat, "Test error 2"),
				},
			},
			want: false,
		},
		{
			name: "validation result with nil errors slice",
			result: ValidationResult{
				Errors: nil,
			},
			want: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := !testCase.result.HasFailures()

			check.Equal(t, testCase.want, result)
		})
	}
}

// TestValidation_ConcurrentAccess tests thread safety of validation.
func TestValidation_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	t.Run("concurrent commit validation", func(t *testing.T) {
		t.Parallel()

		commit := Commit{Hash: "abc", Subject: "test"}
		rules := []CommitRule{
			createFailingRule("rule1", New("rule1", diagnostics.ReasonSubjectTooLong, "error")),
		}
		logger := noopLogger()

		// Run 100 concurrent validations
		var wg sync.WaitGroup

		errors := make(chan []ValidationError, 100)

		for range 100 {
			wg.Go(func() {
				result := ValidateCommitRules(commit, rules, logger)
				errors <- result
			})
		}

		wg.Wait()
		close(errors)

		// All validations should produce the same result
		var firstResult []ValidationError
		for result := range errors {
			if firstResult == nil {
				firstResult = result
			} else {
				check.Equal(t, firstResult, result, "concurrent validations should produce same result")
			}
		}
	})

	t.Run("concurrent repository validation", func(t *testing.T) {
		t.Parallel()

		repo := testRepository{
			commits: []Commit{
				{Hash: "abc", Subject: "test1"},
				{Hash: "def", Subject: "test2"},
			},
		}
		rules := []RepositoryRule{
			createPassingRepoRule("repo1"),
		}
		logger := noopLogger()

		// Run concurrent validations
		var wg sync.WaitGroup
		for range 50 {
			wg.Go(func() {
				ctx := t.Context()

				errors, _ := ValidateRepositoryWithInfo(ctx, rules, repo, logger)
				if len(errors) != 0 {
					t.Errorf("want no errors, got %v", errors)
				}
			})
		}

		wg.Wait()
	})
}

// TestValidation_ErrorPropagation tests that errors are properly propagated and typed.
func TestValidation_ErrorPropagation(t *testing.T) {
	t.Parallel()

	t.Run("preserves error types through validation", func(t *testing.T) {
		t.Parallel()

		wantErr := New("TestRule", diagnostics.ReasonSubjectTooLong, "Subject exceeds limit").
			WithContextMap(diagnostics.Context{
				diagnostics.ContextActual: "75",
				"limit":                   "50",
			})

		commit := Commit{Hash: "abc", Subject: strings.Repeat("a", 75)}
		rules := []CommitRule{
			createFailingRule("TestRule", wantErr),
		}

		errors := ValidateCommitRules(commit, rules, noopLogger())

		check.Len(t, errors, 1)
		actualError := errors[0]

		// Verify error details are preserved
		check.Equal(t, wantErr.Diagnostic, actualError.Diagnostic)
		check.Equal(t, wantErr.Rule, actualError.Rule)
		check.Equal(t, wantErr.Message, actualError.Message)
		check.Equal(t, wantErr.Context["actual"], actualError.Context["actual"])
		check.Equal(t, wantErr.Context["limit"], actualError.Context["limit"])
	})
}

// TestCommit_BoundaryConditions tests edge cases in commit parsing.
func TestCommit_BoundaryConditions(t *testing.T) {
	t.Parallel()

	t.Run("handles commits with only newlines", func(t *testing.T) {
		t.Parallel()

		messages := []string{
			"\n",
			"\n\n",
			"\n\n\n\n\n",
		}

		for _, msg := range messages {
			commit := NewCommit("hash", msg, "Author", "email@example.com", "2024-01-01", "", false)
			check.Empty(t, commit.Subject, "subject should be empty for message: %q", msg)
			check.Empty(t, commit.Body, "body should be empty for message: %q", msg)
		}
	})

	t.Run("handles zero-width characters", func(t *testing.T) {
		t.Parallel()

		// Zero-width space (U+200B) and zero-width joiner (U+200D)
		message := "Subject\u200B\u200D with zero-width"
		commit := NewCommit("hash", message, "Author", "email@example.com", "2024-01-01", "", false)

		check.Contains(t, commit.Subject, "Subject")
		check.Contains(t, commit.Subject, "with zero-width")
	})
}

// TestCommit_MessageParsing tests real-world commit message parsing scenarios.
// Focuses on behavior rather than field assignment.
func TestCommit_MessageParsing(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name        string
		message     string
		checkResult func(t *testing.T, c Commit)
	}{
		{
			name: "conventional commit with breaking change",
			message: `feat!: add new API endpoint

BREAKING CHANGE: The old endpoint /api/v1/users is now /api/v2/users`,
			checkResult: func(t *testing.T, c Commit) {
				t.Helper()
				check.Contains(t, c.Subject, "!")
				check.Contains(t, c.Body, "BREAKING CHANGE")
				check.True(t, c.HasBody())
			},
		},
		{
			name: "commit with sign-off trailer",
			message: `fix: resolve memory leak

Signed-off-by: Jane Doe <jane@example.com>`,
			checkResult: func(t *testing.T, c Commit) {
				t.Helper()
				check.True(t, c.HasBody())
				check.Contains(t, c.Body, "Signed-off-by")
			},
		},
		{
			name: "commit with co-authored-by",
			message: `docs: update README

Co-authored-by: John Smith <john@example.com>
Co-authored-by: Alice Johnson <alice@example.com>`,
			checkResult: func(t *testing.T, c Commit) {
				t.Helper()
				check.True(t, c.HasBody())
				// Verify multiple co-authors are preserved
				bodyLines := strings.Split(c.Body, "\n")
				coAuthorCount := 0

				for _, line := range bodyLines {
					if strings.HasPrefix(line, "Co-authored-by:") {
						coAuthorCount++
					}
				}

				check.Equal(t, 2, coAuthorCount)
			},
		},
		{
			name: "commit with issue references",
			message: `feat: implement user authentication

Fixes #123
Closes #456
See also: #789`,
			checkResult: func(t *testing.T, c Commit) {
				t.Helper()
				check.True(t, c.HasBody())
				check.Contains(t, c.Body, "#123")
				check.Contains(t, c.Body, "#456")
				check.Contains(t, c.Body, "#789")
			},
		},
		{
			name:    "malformed commit with no separation",
			message: "subject line\nbody without blank line separator",
			checkResult: func(t *testing.T, c Commit) { //nolint:varnamelen // test parameter
				t.Helper()
				// Without blank line separator, only first line is subject
				check.Equal(t, "subject line", c.Subject)
				// Without blank line, there's no body per Git convention
				check.Empty(t, c.Body)
				check.False(t, c.HasBody())
			},
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := ParseCommitMessage(testCase.message)
			testCase.checkResult(t, commit)
		})
	}
}

// TestCommit_ValidationBehavior tests how commits behave in validation scenarios.
// This tests actual behavior rather than just field values.
func TestCommit_ValidationBehavior(t *testing.T) {
	t.Parallel()

	t.Run("empty commit should be invalid", func(t *testing.T) {
		t.Parallel()

		c := Commit{}
		check.False(t, c.IsValid())
		check.False(t, c.HasBody())
		check.False(t, c.IsSigned())
	})

	t.Run("minimal valid commit", func(t *testing.T) {
		t.Parallel()

		c := Commit{ //nolint:varnamelen // short variable name OK in test
			Hash:    "a1b2c3",
			Subject: "fix: something",
		}
		check.True(t, c.IsValid())
		check.False(t, c.HasBody())
		check.False(t, c.IsSigned())
	})

	t.Run("fully populated commit", func(t *testing.T) {
		t.Parallel()

		c := NewCommit( //nolint:varnamelen // short variable name OK in test
			"abc123",
			"feat: add feature\n\nDetailed description",
			"John Doe",
			"john@example.com",
			"2024-01-01T00:00:00Z",
			"GPG SIGNATURE",
			false,
		)
		check.True(t, c.IsValid())
		check.True(t, c.HasBody())
		check.True(t, c.IsSigned())
		check.Equal(t, "feat: add feature", c.Subject)
		check.Equal(t, "Detailed description", c.Body)
	})
}

// TestCommit_IsFixupCommit tests detection of fixup/squash/amend commits.
func TestCommit_IsFixupCommit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		subject  string
		isFixup  bool
		wantType string
	}{
		{
			name:     "regular commit",
			subject:  "feat: add new feature",
			isFixup:  false,
			wantType: "",
		},
		{
			name:     "fixup commit",
			subject:  "fixup! feat: add new feature",
			isFixup:  true,
			wantType: "fixup",
		},
		{
			name:     "squash commit",
			subject:  "squash! feat: add new feature",
			isFixup:  true,
			wantType: "squash",
		},
		{
			name:     "amend commit (git commit --fixup=amend:<sha>)",
			subject:  "amend! feat: add new feature",
			isFixup:  true,
			wantType: "amend",
		},
		{
			name:     "fixup without space after exclamation",
			subject:  "fixup!feat: add new feature",
			isFixup:  false,
			wantType: "",
		},
		{
			name:     "fixup in middle of subject",
			subject:  "feat: fixup! something",
			isFixup:  false,
			wantType: "",
		},
		{
			name:     "case sensitive - FIXUP",
			subject:  "FIXUP! feat: add new feature",
			isFixup:  false,
			wantType: "",
		},
		{
			name:     "nested fixup",
			subject:  "fixup! fixup! feat: add new feature",
			isFixup:  true,
			wantType: "fixup",
		},
		{
			name:     "empty subject",
			subject:  "",
			isFixup:  false,
			wantType: "",
		},
		{
			name:     "just fixup prefix",
			subject:  "fixup! ",
			isFixup:  true,
			wantType: "fixup",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := Commit{Subject: testCase.subject}
			check.Equal(t, testCase.isFixup, commit.IsFixupCommit())
			check.Equal(t, testCase.wantType, commit.FixupCommitType())
		})
	}
}
