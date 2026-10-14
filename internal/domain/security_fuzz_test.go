// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"testing"
)

// FuzzValidatePath tests path validation with random inputs.
// Security-critical: ensures path traversal detection works correctly.
func FuzzValidatePath(f *testing.F) {
	// Seed with attack patterns and edge cases.
	seeds := []string{
		// Valid paths.
		"/home/user/file.txt",
		"relative/path/file",
		"file.txt",
		".",

		// Path traversal attempts.
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"foo/../../../etc/passwd",
		"foo/bar/../../../etc/passwd",
		"/foo/bar/../../../etc/passwd",

		// Null byte injection.
		"file.txt\x00.jpg",
		"/path/to\x00/file",

		// Edge cases.
		"",
		" ",
		truncateEllipsis,
		"..",
		"./",
		"../",
		"/..",
		"/../../",

		// Unicode tricks.
		"foo\u2024bar", // One dot leader.
		"foo\u2025bar", // Two dot leader.
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	policy := DefaultSecurityPolicy()

	f.Fuzz(func(t *testing.T, path string) {
		// Should never panic.
		err := policy.ValidatePath(path)

		// Empty path should always error.
		if path == "" && err == nil {
			t.Error("Empty path should return error")
		}

		// Null byte should always error.
		if strings.Contains(path, "\x00") && err == nil {
			t.Error("Path with null byte should return error")
		}

		// Obvious traversal patterns should error.
		if (strings.Contains(path, "../") || strings.Contains(path, "..\\")) && err == nil {
			t.Errorf("Path traversal not detected: %q", path)
		}
	})
}

// FuzzValidateGitReference tests git reference validation.
// It covers the shared metacharacter and control-character input policy.
func FuzzValidateGitReference(f *testing.F) {
	// Seed with attack patterns and valid refs.
	seeds := []string{
		// Valid refs.
		"main",
		"HEAD",
		"v1.0.0",
		"feature/my-branch",
		"refs/heads/main",
		"abc123def456",

		// Rejected metacharacter combinations.
		"$(whoami)",
		"`whoami`",
		"main; rm -rf /",
		"main & whoami",
		"main | cat /etc/passwd",
		"main\nwhoami",
		"main\rwhoami",

		// Null byte.
		"main\x00whoami",

		// Edge cases.
		"",
		" ",
		"$HOME",
		"${HOME}",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	policy := DefaultSecurityPolicy()

	f.Fuzz(func(t *testing.T, ref string) {
		// Should never panic.
		err := policy.ValidateGitReference(ref)

		// Empty ref should error.
		if ref == "" && err == nil {
			t.Error("Empty ref should return error")
		}

		// Policy metacharacters should error.
		dangerousChars := "$`;&|\n\r\x00"
		for _, c := range dangerousChars {
			if strings.ContainsRune(ref, c) && err == nil {
				t.Errorf("Dangerous char %q not detected in: %q", string(c), ref)
			}
		}

		// Dollar-parenthesis input should error.
		if strings.Contains(ref, "$(") && err == nil {
			t.Errorf("Command substitution not detected: %q", ref)
		}
	})
}

// FuzzValidateCommitRange tests commit range validation.
func FuzzValidateCommitRange(f *testing.F) {
	// Seed with valid ranges and attack patterns.
	seeds := []string{
		// Valid ranges.
		"HEAD~5..HEAD",
		"main..feature",
		"v1.0.0..v2.0.0",
		"abc123..def456",

		// Invalid formats.
		"",
		"single-ref",
		"no-dots-here",
		"too..many..dots",

		// Injection in range parts.
		"$(whoami)..HEAD",
		"HEAD..$(whoami)",
		"main; rm -rf /..HEAD",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	policy := DefaultSecurityPolicy()

	f.Fuzz(func(t *testing.T, commitRange string) {
		// Should never panic.
		err := policy.ValidateCommitRange(commitRange)

		// Empty should error.
		if commitRange == "" && err == nil {
			t.Error("Empty commit range should return error")
		}

		// No dots should error.
		if !strings.Contains(commitRange, "..") && err == nil {
			t.Errorf("Commit range without .. should error: %q", commitRange)
		}

		if strings.Contains(commitRange, "...") && err == nil {
			t.Errorf("Unsupported three-dot commit range should error: %q", commitRange)
		}

		// If valid and contains separator, verify no dangerous chars in parts.
		if err == nil {
			assertNoInjectionInParts(t, commitRange)
		}
	})
}

// assertNoInjectionInParts checks that commit range parts don't contain dangerous characters.
func assertNoInjectionInParts(t *testing.T, commitRange string) {
	t.Helper()

	parts := strings.SplitSeq(commitRange, "..")
	for part := range parts {
		trimmed := strings.TrimSpace(part)
		if strings.ContainsAny(trimmed, "$`;&|\n\r\x00") {
			t.Errorf("Dangerous chars in commit range part: %q", part)
		}
	}
}
