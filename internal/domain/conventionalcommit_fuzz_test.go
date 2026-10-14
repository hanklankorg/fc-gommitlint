// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParseConventionalCommit tests the ParseConventionalCommit function with random inputs.
// It verifies the function never panics, valid UTF-8 produces valid output,
// and parse results are consistent when reformatted.
func FuzzParseConventionalCommit(f *testing.F) {
	// Seed corpus with representative examples.
	seeds := []string{
		// Valid conventional commits
		testSubjectFeatAddNewFeature,
		"fix(auth): resolve login issue",
		"feat(ui,api)!: breaking change",
		"docs: update README",
		"refactor(core): simplify logic",
		"test: add unit tests",
		"chore(deps): update dependencies",

		// Edge cases
		"feat:",
		"feat: ",
		"feat:x",
		"a: b",

		// Invalid formats
		"not conventional",
		"feat add feature",
		"feat(scope: missing paren",
		"feat(: empty scope",
		"(scope): no type",
		": no type",

		// Special characters
		"feat: add émojis 🎉",
		"fix: handle 日本語",
		"docs: support Ñoño",

		// Boundary cases
		"",
		" ",
		"\n",
		"\t",
		"feat:\n",
		"feat: line1\nline2",

		// Potentially problematic
		"feat(a,b,c,d,e,f,g): many scopes",
		"feat()!: empty scope with breaking",
		strings.Repeat("a", 1000) + ": very long type",
		"feat: " + strings.Repeat("x", 1000),
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, subject string) {
		// The function should never panic.
		result := ParseConventionalCommit(subject)

		// Verify UTF-8 safety.
		verifyConventionalCommitUTF8(t, subject, result)

		// Verify consistency for valid results.
		verifyConventionalCommitConsistency(t, result)
	})
}

// verifyConventionalCommitUTF8 checks that valid UTF-8 input produces valid UTF-8 output.
func verifyConventionalCommitUTF8(t *testing.T, subject string, result ConventionalCommitFormat) {
	t.Helper()

	if !utf8.ValidString(subject) {
		return
	}

	if !utf8.ValidString(result.Type) {
		t.Errorf("Type contains invalid UTF-8: %q", result.Type)
	}

	if !utf8.ValidString(result.Description) {
		t.Errorf("Description contains invalid UTF-8: %q", result.Description)
	}

	if !utf8.ValidString(result.RawScope) {
		t.Errorf("RawScope contains invalid UTF-8: %q", result.RawScope)
	}

	for idx, scope := range result.Scopes {
		if !utf8.ValidString(scope) {
			t.Errorf("Scope[%d] contains invalid UTF-8: %q", idx, scope)
		}
	}
}

// verifyConventionalCommitConsistency checks parsing consistency for valid commits.
func verifyConventionalCommitConsistency(t *testing.T, result ConventionalCommitFormat) {
	t.Helper()

	if !result.IsValid {
		return
	}

	// Type should not be empty for valid commits.
	if result.Type == "" {
		t.Error("IsValid=true but Type is empty")
	}

	// Re-format and re-parse should produce consistent type.
	if result.Type == "" || result.Description == "" {
		return
	}

	// Round-trip: format and re-parse to verify consistency
	scope := ""
	if result.RawScope != "" {
		scope = "(" + result.RawScope + ")"
	}

	bang := ""
	if result.Breaking {
		bang = "!"
	}

	reformatted := result.Type + scope + bang + ": " + result.Description
	reparsed := ParseConventionalCommit(reformatted)

	if reparsed.Type != result.Type {
		t.Errorf("Type changed after reformat: %q -> %q", result.Type, reparsed.Type)
	}

	if reparsed.Breaking != result.Breaking {
		t.Errorf("Breaking changed after reformat: %v -> %v", result.Breaking, reparsed.Breaking)
	}

	if reparsed.Description != result.Description {
		t.Errorf("Description changed after reformat: %q -> %q", result.Description, reparsed.Description)
	}

	if !slices.Equal(reparsed.Scopes, result.Scopes) {
		t.Errorf("Scopes changed after reformat: %q -> %q", result.Scopes, reparsed.Scopes)
	}
}

// FuzzIsConventionalCommitLike tests pattern detection on arbitrary input.
func FuzzIsConventionalCommitLike(f *testing.F) {
	seeds := []string{
		testSubjectFeatAddFeature,
		"feat(scope): add feature",
		"feat add feature",
		"feat(scope: missing paren",
		"random commit message",
		"",
		" ",
		"fix ",
		"fix something",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, subject string) {
		// Should never panic.
		result := IsConventionalCommitLike(subject, nil)

		// If ParseConventionalCommit returns valid, IsConventionalCommitLike should be true.
		parsed := ParseConventionalCommit(subject)
		if parsed.IsValid && !result {
			t.Errorf("ParseConventionalCommit valid but IsConventionalCommitLike false for: %q", subject)
		}
	})
}
