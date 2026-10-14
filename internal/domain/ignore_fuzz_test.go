// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"testing"
)

// FuzzParseSHA tests SHA normalization with random inputs.
// It verifies:
// 1. The function never panics on any input.
// 2. Output is always lowercase hex or empty.
// 3. Normalization is idempotent.
func FuzzParseSHA(f *testing.F) {
	// Seed corpus with representative examples.
	seeds := []string{
		// Valid full SHAs.
		testSHAFullLong,
		"ABCDEF1234567890ABCDEF1234567890ABCDEF12",
		strings.Repeat("a", 64),

		// Valid abbreviated SHAs.
		testSHAAbbrevA,
		"1234567",
		"abcdef1234",

		// Edge cases.
		"",
		" ",
		"      ",
		"a1b2c3",                // Too short (6 chars).
		strings.Repeat("a", 65), // Too long.

		// Invalid characters.
		"ghijklm",
		"1234567890abcdefghij",
		"a1b2c3d!",
		"a1b2c3d@example",

		// Whitespace.
		"  " + testSHAAbbrevA + "  ",
		"\t" + testSHAAbbrevA + "\n",
		testSHAAbbrevA + "\r\n",

		// Mixed case.
		"A1B2C3D4E5F6789012345678901234567890ABCD",
		"a1B2c3D4e5F6789012345678901234567890AbCd",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, sha string) {
		// Should never panic.
		result := ParseSHA(sha)

		// Verify result validity.
		verifyNormalizedSHA(t, result)

		// Normalization should be idempotent.
		result2 := ParseSHA(result)
		if result != result2 {
			t.Errorf("Normalization not idempotent: %q -> %q -> %q", sha, result, result2)
		}
	})
}

// verifyNormalizedSHA checks that a normalized SHA is valid.
func verifyNormalizedSHA(t *testing.T, result string) {
	t.Helper()

	if result == "" {
		return
	}

	// Must be 7-64 chars.
	if len(result) < 7 || len(result) > 64 {
		t.Errorf("Invalid result length %d: %q", len(result), result)
	}

	// Must be lowercase hex.
	for _, char := range result {
		isDigit := char >= '0' && char <= '9'
		isHexLower := char >= 'a' && char <= 'f'

		if !isDigit && !isHexLower {
			t.Errorf("Result contains non-hex character: %q in %q", string(char), result)
		}
	}

	// Must be lowercase.
	if result != strings.ToLower(result) {
		t.Errorf("Result not lowercase: %q", result)
	}
}

// FuzzBuildIgnoreChecker tests the ignore checker with random SHA lists.
func FuzzBuildIgnoreChecker(f *testing.F) {
	// Seed with SHA combinations.
	f.Add(testSHAAbbrevA, "b2c3d4e5f6g")
	f.Add(testSHAFullLong, "")
	f.Add("", "")
	f.Add("invalid", testSHAAbbrevA)

	f.Fuzz(func(t *testing.T, sha1, sha2 string) {
		ignoreList := []string{sha1, sha2}

		// Should never panic.
		checker := BuildIgnoreChecker(ignoreList)

		// Nothing matches the empty string, and a checker built from two
		// invalid values is not nil, so this holds for every input.
		if checker.Contains("") {
			t.Error("the empty string must never be ignored")
		}

		// Every valid configured value ignores itself, however the commit's
		// hash is spelled: Contains normalises the query the way the list
		// was normalised, so case and surrounding whitespace do not matter.
		for _, sha := range ignoreList {
			if ParseSHA(sha) == "" {
				continue
			}

			for _, query := range []string{sha, strings.ToUpper(sha), " " + sha + " "} {
				if !checker.Contains(query) {
					t.Errorf("configured %q must ignore %q", sha, query)
				}
			}
		}

		// Nil checker should handle Contains gracefully.
		var nilChecker *IgnoreChecker
		if nilChecker.Contains(sha1) {
			t.Error("Nil checker should return false")
		}
	})
}

// FuzzIgnoreCheckerContains tests the Contains method with random inputs.
func FuzzIgnoreCheckerContains(f *testing.F) {
	// Seed with test SHAs.
	f.Add(testSHAFullLong)                            // Exact match.
	f.Add("b2c3d4e")                                  // Prefix in list.
	f.Add("b2c3d4e5f6789012345678901234567890abcdef") // Should match prefix.
	f.Add("zzzzzzz")                                  // No match.
	f.Add("")                                         // Empty.
	f.Add("invalid!")                                 // Invalid chars.

	f.Fuzz(func(t *testing.T, sha string) {
		// Build checker inside fuzz target to avoid global state.
		knownSHAs := []string{
			testSHAFullLong,
			"b2c3d4e",
			"1234567890",
		}
		checker := BuildIgnoreChecker(knownSHAs)

		// Should never panic.
		result := checker.Contains(sha)

		// If SHA normalizes to a known value, it should match.
		normalized := ParseSHA(sha)
		if normalized == testSHAFullLong && !result {
			t.Errorf("Should match known full SHA: %q", sha)
		}

		// Prefix matching: if normalized starts with "b2c3d4e", should match.
		if normalized != "" && strings.HasPrefix(normalized, "b2c3d4e") && !result {
			t.Errorf("Should match known prefix: %q (normalized: %q)", sha, normalized)
		}
	})
}
