//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// TestCLIExitCodes_ReflectValidationOutcome checks that validate errors
// exactly when the commit is invalid, and that --help always succeeds.
func TestCLIExitCodes_ReflectValidationOutcome(t *testing.T) {
	t.Setenv("GOMMITLINT_NO_CI_DETECT", "1")

	tests := []struct {
		name        string
		commitMsg   string
		args        []string
		expectError bool
		description string
	}{
		{
			name:        "valid commit returns success",
			commitMsg:   "feat: add new feature",
			args:        []string{"gommitlint", "validate"},
			expectError: false,
			description: "should succeed with valid commit",
		},
		{
			name:        "invalid format returns error",
			commitMsg:   "invalid commit message format",
			args:        []string{"gommitlint", "validate"},
			expectError: true,
			description: "should fail with invalid format",
		},
		{
			name:        "subject too long returns error",
			commitMsg:   "feat: " + strings.Repeat("a", 100) + " this is way too long",
			args:        []string{"gommitlint", "validate"},
			expectError: true,
			description: "should fail when subject is too long",
		},
		{
			name:        "help command always succeeds",
			commitMsg:   "any: commit",
			args:        []string{"gommitlint", "--help"},
			expectError: false,
			description: "help should always succeed",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repoPath := gitrepo.QuickRepoPath(t, testCase.commitMsg)

			fullArgs := append(testCase.args, "--repo-path", repoPath)

			output, err := RunCLICommand(t, fullArgs, repoPath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}

			if err != nil {
				t.Logf("Command failed with output: %s", output)
			}
		})
	}
}
