//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestStreams_ResultsGoToStdoutDiagnosticsToStderr tests that stdout and stderr are used correctly according to Unix conventions.
//
//nolint:tparallel // Disabled parallelism - test builds binaries and captures stdout/stderr
func TestStreams_ResultsGoToStdoutDiagnosticsToStderr(t *testing.T) {
	harness := newHarness(t)

	tests := []struct {
		name         string
		setupRepo    func(*testing.T) (string, func())
		args         []string
		expectStdout string
		expectStderr string
		description  string
	}{
		{
			name: "text format - validation results to stdout, logs to stderr",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:         []string{"validate", "--log-level", "debug"},
			expectStdout: "",              // No violations = empty stdout
			expectStderr: "Rules created", // Debug logs go to stderr
			description:  "text format should send results to stdout, logs to stderr",
		},
		{
			name: "json format - validation results to stdout only",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:         []string{"validate", "--format", "json", "--log-level", "debug"},
			expectStdout: `"commits"`,     // JSON results go to stdout
			expectStderr: "Rules created", // Debug logs go to stderr (console format)
			description:  "json format should send JSON to stdout, logs to stderr",
		},
		{
			name: "github format - validation results to stdout, logs to stderr",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "invalid commit format"), func() {}
			},
			args:         []string{"validate", "--format", "github", "--log-level", "info"},
			expectStdout: "::error",                   // GitHub format goes to stdout
			expectStderr: "Command execution failed:", // Error logs go to stderr
			description:  "github format should send results to stdout, logs to stderr",
		},
		{
			name: "gitlab format - validation results to stdout, logs to stderr",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "invalid commit format"), func() {}
			},
			args:         []string{"validate", "--format", "gitlab", "--log-level", "info"},
			expectStdout: "section_start:",            // GitLab format goes to stdout
			expectStderr: "Command execution failed:", // Error logs go to stderr
			description:  "gitlab format should send results to stdout, logs to stderr",
		},
		{
			name: "rule help to stdout - respects configured writer",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:         []string{"validate", "--rule-help", "subject"},
			expectStdout: "SUBJECT RULE", // Rule help goes to stdout (configured writer)
			expectStderr: "",             // No logs expected for this case
			description:  "rule help should use configured writer (stdout by default)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoPath, _ := testCase.setupRepo(t)

			stdout, stderr := runGommitlintSeparated(t, harness, repoPath, testCase.args...)

			if testCase.expectStdout != "" && !strings.Contains(stdout, testCase.expectStdout) {
				t.Fatalf("%s: stdout missing %q:\n%s", testCase.description, testCase.expectStdout, stdout)
			}

			if testCase.expectStderr != "" && !strings.Contains(stderr, testCase.expectStderr) {
				t.Fatalf("%s: stderr missing %q:\n%s", testCase.description, testCase.expectStderr, stderr)
			}
		})
	}
}

// TestStreams_JSONStdoutStaysParseableWithLogsEnabled tests that JSON output is valid JSON without contamination.
//
//nolint:tparallel // Disabled parallelism - builds binaries and captures output
func TestStreams_JSONStdoutStaysParseableWithLogsEnabled(t *testing.T) {
	harness := newHarness(t)

	tests := []struct {
		name        string
		setupRepo   func(*testing.T) (string, func())
		args        []string
		description string
	}{
		{
			name: "json output with debug logs enabled",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:        []string{"validate", "--format", "json", "--log-level", "debug"},
			description: "JSON output should be valid JSON even with debug logs",
		},
		{
			name: "json output with trace logs enabled",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:        []string{"validate", "--format", "json", "--log-level", "trace"},
			description: "JSON output should be valid JSON even with trace logs",
		},
		{
			name: "json output with validation errors",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "invalid commit format"), func() {}
			},
			args:        []string{"validate", "--format", "json", "--log-level", "debug"},
			description: "JSON output should be valid JSON even with validation errors and debug logs",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repoPath, _ := testCase.setupRepo(t)

			stdout, stderr := runGommitlintSeparated(t, harness, repoPath, testCase.args...)

			// Verify stdout contains valid JSON
			var jsonResult any

			err := json.Unmarshal([]byte(stdout), &jsonResult)
			if err != nil {
				t.Fatalf("%s: stdout is not valid JSON: %v\n%s", testCase.description, err, stdout)
			}

			// Verify stderr contains logs (if any)
			if (strings.Contains(strings.Join(testCase.args, " "), "debug") || strings.Contains(strings.Join(testCase.args, " "), "trace")) && stderr == "" {
				t.Fatalf("%s: stderr should contain debug/trace logs", testCase.description)
			}

			// Verify JSON structure contains expected fields
			jsonMap, ok := jsonResult.(map[string]any)
			if !ok {
				t.Fatalf("%s: JSON should be an object", testCase.description)
			}
			if _, ok := jsonMap["commits"]; !ok {
				t.Fatalf("%s: JSON should contain 'commits' field", testCase.description)
			}
			summary, ok := jsonMap["summary"].(map[string]any)
			if !ok {
				t.Fatalf("%s: JSON summary should be an object", testCase.description)
			}
			for _, key := range []string{"allPassed", "totalCommits"} {
				if _, ok := summary[key]; !ok {
					t.Fatalf("%s: JSON summary should contain %q field", testCase.description, key)
				}
			}
		})
	}
}

// TestStreams_OutputSurvivesAPipe tests that output works correctly with Unix pipelines.
func TestStreams_OutputSurvivesAPipe(t *testing.T) {
	t.Parallel()
	harness := newHarness(t)

	tests := []struct {
		name         string
		setupRepo    func(*testing.T) (string, func())
		args         []string
		pipeCommand  []string
		expectOutput string
		description  string
	}{
		{
			name: "json output piped to jq",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "feat: valid commit"), func() {}
			},
			args:         []string{"validate", "--format", "json"},
			pipeCommand:  []string{"jq", ".summary.allPassed"},
			expectOutput: "true",
			description:  "JSON output should be pipeable to jq",
		},
		{
			name: "text output piped to grep",
			setupRepo: func(t *testing.T) (string, func()) {
				t.Helper()

				return gitRepoPathWithCommits(t, "invalid commit format"), func() {}
			},
			args:         []string{"validate", "--format", "text", "-vv"},
			pipeCommand:  []string{"grep", "-c", "Commit type"},
			expectOutput: "1",
			description:  "text output should be pipeable to grep",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			pipeCommand := append([]string{requireTool(t, testCase.pipeCommand[0])}, testCase.pipeCommand[1:]...)

			repoPath, _ := testCase.setupRepo(t)

			output := runGommitlintPiped(t, harness, repoPath, testCase.args, pipeCommand)
			if !strings.Contains(output, testCase.expectOutput) {
				t.Fatalf("%s: piped output missing %q:\n%s", testCase.description, testCase.expectOutput, output)
			}
		})
	}
}

// runGommitlintSeparated runs gommitlint and returns stdout and stderr separately.
func runGommitlintSeparated(t *testing.T, h *harness, repoPath string, args ...string) (string, string) {
	t.Helper()

	result := h.run(repoPath, nil, args...)

	return result.stdout, result.stderr
}

// runGommitlintPiped runs gommitlint piped to another command.
func runGommitlintPiped(t *testing.T, h *harness, repoPath string, gommitlintArgs, pipeArgs []string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Spawned by hand rather than through h.run: the point of this test is
	// that the product's stdout is a real pipe to another process, which a
	// captured buffer would not reproduce. The harness still supplies the
	// binary and the isolated environment, so the two paths cannot disagree
	// about which artifact runs or what it inherits.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- test harness; h.bin is the freshly-built gommitlint, args come from in-tree test code.
	gommitlintCmd := exec.CommandContext(ctx, h.bin, gommitlintArgs...)
	gommitlintCmd.Dir = repoPath
	gommitlintCmd.Env = h.env

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- pipeArgs are fixed shell literals from in-tree test code.
	pipeCmd := exec.CommandContext(ctx, pipeArgs[0], pipeArgs[1:]...) // #nosec G204 - Fixed args in test
	pipeCmd.Env = h.env

	// Connect the commands with a pipe
	pipe, err := gommitlintCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("create gommitlint stdout pipe: %v", err)
	}

	pipeCmd.Stdin = pipe

	// Start both commands
	err = gommitlintCmd.Start()
	if err != nil {
		t.Fatalf("start gommitlint: %v", err)
	}

	output, err := pipeCmd.Output()
	if err != nil {
		t.Fatalf("run pipe command: %v", err)
	}

	_ = gommitlintCmd.Wait() // Ignore error - validation failures are expected

	return string(output)
}
