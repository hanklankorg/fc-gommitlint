// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// CommitIgnoreInfo holds information about ignored commits during validation.
type CommitIgnoreInfo struct {
	TotalCommits   int
	ValidatedCount int
	IgnoredCount   int
	IgnoredSHAs    []string
}

// IgnoreChecker checks whether a commit matches a configured object ID prefix.
type IgnoreChecker struct {
	prefixes []string
}

// BuildIgnoreChecker creates an ignore checker from the ignore list.
func BuildIgnoreChecker(ignoreList []string) *IgnoreChecker {
	if len(ignoreList) == 0 {
		return nil
	}

	checker := &IgnoreChecker{
		prefixes: []string{},
	}

	for _, sha := range ignoreList {
		normalized := ParseSHA(sha)
		if normalized == "" {
			continue // Skip invalid SHAs
		}

		// A 40-character value is either a complete SHA-1 object ID or a
		// prefix of a SHA-256 object ID, so every configured value remains a
		// prefix until the repository object format is known at match time.
		checker.prefixes = append(checker.prefixes, normalized)
	}

	return checker
}

// Contains checks if a commit SHA should be ignored.
func (ic *IgnoreChecker) Contains(commitSHA string) bool {
	if ic == nil {
		return false
	}

	normalized := ParseSHA(commitSHA)
	if normalized == "" {
		return false
	}

	// O(p), where p is the typically small configured ignore list.
	for _, prefix := range ic.prefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}

	return false
}

// ParseSHA parses and normalizes a SHA hash for consistent matching.
// Trims whitespace, converts to lowercase, validates basic format.
// Empty string returned for invalid input.
func ParseSHA(sha string) string {
	normalized := strings.TrimSpace(sha)
	normalized = strings.ToLower(normalized)

	// Git supports SHA-1 and SHA-256 object IDs, so valid prefixes are 7-64
	// hexadecimal characters.
	if len(normalized) < 7 || len(normalized) > 64 {
		return ""
	}

	for _, char := range normalized {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return ""
		}
	}

	return normalized
}

// FilterIgnoredCommits separates commits into validated and ignored groups.
// Invalid SHAs in configuration are silently ignored (they won't match any commits).
// Warning: Invalid SHA warnings are collected by the config loader, not here.
func FilterIgnoredCommits(commits []Commit, cfg settings.Config) ([]Commit, CommitIgnoreInfo) {
	// Use optimized checker for better performance
	checker := BuildIgnoreChecker(cfg.IgnoreCommits)

	// If no ignore list, return all commits
	if checker == nil {
		return commits, CommitIgnoreInfo{
			TotalCommits:   len(commits),
			ValidatedCount: len(commits),
			IgnoredCount:   0,
		}
	}

	validatedCommits := make([]Commit, 0, len(commits))

	var ignoredSHAs []string

	for _, commit := range commits {
		if checker.Contains(commit.Hash) {
			ignoredSHAs = append(ignoredSHAs, commit.Hash)
		} else {
			validatedCommits = append(validatedCommits, commit)
		}
	}

	return validatedCommits, CommitIgnoreInfo{
		TotalCommits:   len(commits),
		ValidatedCount: len(validatedCommits),
		IgnoredCount:   len(ignoredSHAs),
		IgnoredSHAs:    ignoredSHAs,
	}
}
