// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
)

func TestIsValidGitHookType_AcceptsOnlyKnownHookNames(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		hookType string
		expected bool
	}{
		{
			name:     "valid commit-msg hook",
			hookType: "commit-msg",
			expected: true,
		},
		{
			name:     "valid pre-commit hook",
			hookType: "pre-commit",
			expected: true,
		},
		{
			name:     "invalid hook type",
			hookType: "malicious-hook",
			expected: false,
		},
		{
			name:     "empty hook type",
			hookType: "",
			expected: false,
		},
		{
			name:     "unsupported hook name",
			hookType: "commit-msg; rm -rf /",
			expected: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := fileops.IsValidGitHookType(testCase.hookType)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestIsWithinPath_RejectsEscapes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		relPath     string
		basePath    string
		expected    bool
		description string
	}{
		{
			name:        "safe relative path",
			relPath:     "subdir/file.txt",
			basePath:    functionalTestMockUserPath,
			expected:    true,
			description: "should accept safe relative paths",
		},
		{
			name:        "traversal attempt",
			relPath:     "../../../etc/passwd",
			basePath:    functionalTestMockUserPath,
			expected:    false,
			description: "should reject directory traversal",
		},
		{
			name:        "current directory",
			relPath:     ".",
			basePath:    functionalTestMockUserPath,
			expected:    true,
			description: "should accept current directory reference",
		},
		{
			name:        "parent directory",
			relPath:     "..",
			basePath:    functionalTestMockUserPath,
			expected:    false,
			description: "should reject parent directory reference",
		},
		{
			name:        "embedded traversal",
			relPath:     "good/../bad/../../etc",
			basePath:    functionalTestMockUserPath,
			expected:    false,
			description: "should reject embedded traversal sequences",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := fileops.IsWithinPath(testCase.relPath, testCase.basePath)
			check.Equal(t, testCase.expected, result, testCase.description)
		})
	}
}
