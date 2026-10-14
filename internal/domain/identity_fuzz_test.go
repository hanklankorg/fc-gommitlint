// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzNewIdentityFromString tests identity parsing with random inputs.
// It verifies the function never panics, valid UTF-8 produces valid UTF-8 output,
// and round-trip consistency for well-formed identities.
func FuzzNewIdentityFromString(f *testing.F) {
	// Seed corpus with representative examples.
	addIdentitySeeds(f)

	f.Fuzz(func(t *testing.T, identity string) {
		// Should never panic.
		result := NewIdentityFromString(identity)

		// Verify results.
		verifyIdentityUTF8(t, identity, result)
		verifyIdentityTrimming(t, result)
		verifyIdentityConsistency(t, result)
	})
}

//nolint:thelper // Fuzz seed helpers don't need f.Helper().
func addIdentitySeeds(f *testing.F) {
	seeds := []string{
		// Standard formats.
		"John Doe <john@example.com>",
		"Jane Smith <jane.smith@company.org>",
		"User <user@host>",

		// Edge cases.
		"<email@example.com>",
		"Name Only",
		"email@example.com",
		"",
		" ",
		"<>",
		"Name <>",
		"<email>",

		// Multiple angle brackets.
		"Name <email> <extra>",
		"<first> <second@example.com>",

		// Special characters.
		"José García <jose@example.com>",
		"日本語 <japanese@example.com>",

		// Whitespace variations.
		"  Name  <  email@example.com  >  ",

		// Long inputs.
		strings.Repeat("a", 1000) + " <" + strings.Repeat("b", 100) + "@example.com>",

		// Malformed.
		"Name <unclosed",
		"Name closed>",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}
}

func verifyIdentityUTF8(t *testing.T, input string, result Identity) {
	t.Helper()

	if !utf8.ValidString(input) {
		return
	}

	if !utf8.ValidString(result.Name) {
		t.Errorf("Name contains invalid UTF-8: %q", result.Name)
	}

	if !utf8.ValidString(result.Email) {
		t.Errorf("Email contains invalid UTF-8: %q", result.Email)
	}

	str := result.String()
	if !utf8.ValidString(str) {
		t.Errorf("String() contains invalid UTF-8: %q", str)
	}
}

func verifyIdentityTrimming(t *testing.T, result Identity) {
	t.Helper()

	if result.Name != strings.TrimSpace(result.Name) {
		t.Errorf("Name has untrimmed whitespace: %q", result.Name)
	}

	if result.Email != strings.TrimSpace(result.Email) {
		t.Errorf("Email has untrimmed whitespace: %q", result.Email)
	}
}

func verifyIdentityConsistency(t *testing.T, result Identity) {
	t.Helper()

	// IsEmpty should be consistent.
	if result.IsEmpty() && (result.Name != "" || result.Email != "") {
		t.Error("IsEmpty() returned true but Name or Email is not empty")
	}

	// If we have both name and email, String() should contain both.
	if result.Name == "" || result.Email == "" {
		return
	}

	str := result.String()
	if !strings.Contains(str, result.Name) {
		t.Errorf("String() missing Name: got %q, want Name %q", str, result.Name)
	}

	if !strings.Contains(str, result.Email) {
		t.Errorf("String() missing Email: got %q, want Email %q", str, result.Email)
	}
}

// FuzzIdentityMatches tests identity matching with random inputs.
func FuzzIdentityMatches(f *testing.F) {
	// Seed with email pairs.
	f.Add("user@example.com", "user@example.com")
	f.Add("user@example.com", "USER@EXAMPLE.COM")
	f.Add("user@example.com", "other@example.com")
	f.Add("", "user@example.com")
	f.Add("user@example.com", "")
	f.Add("", "")
	f.Add(" ", " ") // Whitespace-only emails.

	f.Fuzz(func(t *testing.T, email1, email2 string) {
		id1 := NewIdentity("Name1", email1)
		id2 := NewIdentity("Name2", email2)

		// Should never panic.
		result := id1.Matches(id2)

		// Use the actual trimmed emails for assertions (NewIdentity trims).
		trimmed1 := strings.TrimSpace(email1)
		trimmed2 := strings.TrimSpace(email2)

		// Matching should be symmetric for non-empty trimmed emails.
		if trimmed1 != "" && trimmed2 != "" {
			reverse := id2.Matches(id1)
			if result != reverse {
				t.Errorf("Matches not symmetric: %q vs %q", email1, email2)
			}
		}

		// Empty trimmed email should never match.
		if trimmed1 == "" && result {
			t.Error("Empty email1 should not match")
		}

		// Same normalized trimmed email should match.
		if strings.EqualFold(trimmed1, trimmed2) && trimmed1 != "" && !result {
			t.Errorf("Same email should match: %q vs %q (trimmed: %q vs %q)",
				email1, email2, trimmed1, trimmed2)
		}
	})
}
