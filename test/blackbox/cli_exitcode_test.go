//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"strings"
	"testing"
)

// TestExitCode_MatchesTheOutcome tests the complete CLI exit code flow by running the actual binary.
//
//nolint:tparallel // Disabled parallelism - builds binaries and creates Git repositories
func TestExitCode_MatchesTheOutcome(t *testing.T) {
	harness := newHarness(t)

	tests := []struct {
		name         string
		setupRepo    func(*testing.T) (string, func())
		args         []string
		expectExit   int
		expectOutput string
		description  string
	}{
		{
			name: "success - valid commit",
			setupRepo: func(t *testing.T) (string, func()) {
				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "feat: add new feature"), func() {}
			},
			args:         []string{"validate"},
			expectExit:   0,
			expectOutput: "",
			description:  "should return exit 0 for valid commit",
		},
		{
			name: "validation failure - invalid format",
			setupRepo: func(t *testing.T) (string, func()) {
				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "invalid commit message format"), func() {}
			},
			args:         []string{"validate"},
			expectExit:   1,
			expectOutput: "",
			description:  "should return exit 1 for validation failures",
		},
		{
			name: "bare invocation returns EX_USAGE",
			setupRepo: func(t *testing.T) (string, func()) {
				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "feat: any commit"), func() {}
			},
			args:         []string{},
			expectExit:   64,
			expectOutput: "",
			description:  "should return exit 64 (EX_USAGE) for a bare invocation with no subcommand",
		},
		{
			name: "missing config file returns EX_NOINPUT",
			setupRepo: func(t *testing.T) (string, func()) {

				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:         []string{"--config", "/nonexistent/config.yaml", "validate"},
			expectExit:   66,
			expectOutput: "",
			description:  "should return exit 66 (EX_NOINPUT) when the named config file is absent",
		},
		{
			name: "validation failure - subject too long",
			setupRepo: func(t *testing.T) (string, func()) {

				longMessage := "feat: " + strings.Repeat("a", 100) + " this is way too long"

				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, longMessage), func() {}
			},
			args:         []string{"validate"},
			expectExit:   1,
			expectOutput: "",
			description:  "should return exit 1 for subject length validation failure",
		},
		{
			name: "success - help command",
			setupRepo: func(t *testing.T) (string, func()) {

				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "feat: any commit"), func() {}
			},
			args:         []string{"--help"},
			expectExit:   0,
			expectOutput: "gommitlint",
			description:  "should return exit 0 for help command",
		},
		{
			name: "success - version command",
			setupRepo: func(t *testing.T) (string, func()) {

				// MUST USE FILESYSTEM: Binary execution requires real filesystem paths
				return gitRepoPathWithCommits(t, "feat: any commit"), func() {}
			},
			args:         []string{"--version"},
			expectExit:   0,
			expectOutput: "dev",
			description:  "should return exit 0 for version command",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoPath, _ := testCase.setupRepo(t)

			// Run gommitlint with specified args
			exitCode, output := runGommitlint(t, harness, repoPath, testCase.args...)

			// Verify exit code
			if exitCode != testCase.expectExit {
				t.Fatalf("%s: exit code = %d, want %d. Output: %s",
					testCase.description, exitCode, testCase.expectExit, output)
			}

			// Verify output contains expected content if specified
			if testCase.expectOutput != "" && !strings.Contains(output, testCase.expectOutput) {
				t.Fatalf("%s: output missing %q:\n%s", testCase.description, testCase.expectOutput, output)
			}
		})
	}
}

// runGommitlint runs the gommitlint binary and returns the exit code and output.
func runGommitlint(t *testing.T, h *harness, repoPath string, args ...string) (int, string) {
	t.Helper()

	result := h.run(repoPath, nil, args...)

	return result.code, result.stdout + result.stderr
}
