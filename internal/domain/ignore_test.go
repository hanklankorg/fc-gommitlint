// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestParseSHA_VariousFormats_ReturnsValidSHA(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name  string
		given string
		want  string
	}{
		{
			name:  "valid full SHA-1",
			given: testSHAFullA,
			want:  testSHAFullA,
		},
		{
			name:  "valid full SHA-256",
			given: strings.Repeat("a", 64),
			want:  strings.Repeat("a", 64),
		},
		{
			name:  "valid abbreviated SHA",
			given: testSHAAbbrevA,
			want:  testSHAAbbrevA,
		},
		{
			name:  "uppercase SHA converted to lowercase",
			given: "A1B2C3D",
			want:  testSHAAbbrevA,
		},
		{
			name:  "SHA with whitespace",
			given: "  " + testSHAAbbrevA + "  ",
			want:  testSHAAbbrevA,
		},
		{
			name:  "too short SHA",
			given: "a1b2c3",
			want:  "",
		},
		{
			name:  "too long SHA",
			given: strings.Repeat("a", 65),
			want:  "",
		},
		{
			name:  "invalid characters in SHA",
			given: "a1b2c3g",
			want:  "",
		},
		{
			name:  testNameEmptyString,
			given: "",
			want:  "",
		},
		{
			name:  "non-hex characters",
			given: "hello123",
			want:  "",
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ParseSHA(testCase.given)
			check.Equal(t, testCase.want, result)
		})
	}
}

func TestIgnoreChecker_ContainsSHA1AndSHA256Prefixes(t *testing.T) {
	t.Parallel()

	sha256 := strings.Repeat("a", 64)
	tests := []struct {
		name       string
		ignoreList []string
		commitSHA  string
		want       bool
	}{
		{name: "seven character SHA-256 prefix", ignoreList: []string{sha256[:7]}, commitSHA: sha256, want: true},
		{name: "forty character SHA-256 prefix", ignoreList: []string{sha256[:40]}, commitSHA: sha256, want: true},
		{name: "full SHA-256", ignoreList: []string{sha256}, commitSHA: sha256, want: true},
		{name: "full SHA-1", ignoreList: []string{testSHAFullA}, commitSHA: testSHAFullA, want: true},
		{name: "longer configured value cannot match shorter object ID", ignoreList: []string{sha256}, commitSHA: sha256[:40], want: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			checker := BuildIgnoreChecker(testCase.ignoreList)
			check.Equal(t, testCase.want, checker.Contains(testCase.commitSHA))
		})
	}
}

func TestFilterIgnoredCommits_PartitionsByNormalizedSHA(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name           string
		commits        []Commit
		config         settings.Config
		wantCommits    []Commit
		wantIgnoreInfo CommitIgnoreInfo
	}{
		{
			name:        "no ignore list returns all commits",
			commits:     []Commit{{Hash: testSHAAbbrevA}, {Hash: testSHAAbbrevE}},
			config:      settings.Config{},
			wantCommits: []Commit{{Hash: testSHAAbbrevA}, {Hash: testSHAAbbrevE}},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   2,
				ValidatedCount: 2,
				IgnoredCount:   0,
			},
		},
		{
			name:        "empty ignore list returns all commits",
			commits:     []Commit{{Hash: testSHAAbbrevA}, {Hash: testSHAAbbrevE}},
			config:      settings.Config{IgnoreCommits: []string{}},
			wantCommits: []Commit{{Hash: testSHAAbbrevA}, {Hash: testSHAAbbrevE}},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   2,
				ValidatedCount: 2,
				IgnoredCount:   0,
			},
		},
		{
			name:        "ignore one commit",
			commits:     []Commit{{Hash: testSHAFullA}, {Hash: testSHAFullE}},
			config:      settings.Config{IgnoreCommits: []string{testSHAAbbrevA}},
			wantCommits: []Commit{{Hash: testSHAFullE}},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   2,
				ValidatedCount: 1,
				IgnoredCount:   1,
				IgnoredSHAs:    []string{testSHAFullA},
			},
		},
		{
			name:        "ignore all commits",
			commits:     []Commit{{Hash: testSHAFullA}, {Hash: testSHAFullE}},
			config:      settings.Config{IgnoreCommits: []string{testSHAAbbrevA, "e4f5a6b1"}},
			wantCommits: []Commit{},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   2,
				ValidatedCount: 0,
				IgnoredCount:   2,
				IgnoredSHAs:    []string{testSHAFullA, testSHAFullE},
			},
		},
		{
			name:        "invalid SHA in config generates warning",
			commits:     []Commit{{Hash: testSHAFullA}},
			config:      settings.Config{IgnoreCommits: []string{"invalid", testSHAAbbrevA}},
			wantCommits: []Commit{},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   1,
				ValidatedCount: 0,
				IgnoredCount:   1,
				IgnoredSHAs:    []string{testSHAFullA},
			},
		},
		{
			name:        "mixed valid and invalid config",
			commits:     []Commit{{Hash: testSHAFullA}, {Hash: testSHAFullE}},
			config:      settings.Config{IgnoreCommits: []string{"bad", testSHAAbbrevA, "invalid"}},
			wantCommits: []Commit{{Hash: testSHAFullE}},
			wantIgnoreInfo: CommitIgnoreInfo{
				TotalCommits:   2,
				ValidatedCount: 1,
				IgnoredCount:   1,
				IgnoredSHAs:    []string{testSHAFullA},
			},
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			validatedCommits, ignoreInfo := FilterIgnoredCommits(testCase.commits, testCase.config)

			check.Equal(t, testCase.wantCommits, validatedCommits)
			check.Equal(t, testCase.wantIgnoreInfo, ignoreInfo)
		})
	}
}
