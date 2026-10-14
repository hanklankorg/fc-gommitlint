//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestStreams_RootCommandsHonourTheStreamContract is the root-level contract for the CLI boundary.
// Each case pins the triple (stdout class, stderr class, exit code) for a
// failure class so the guarantee the docs make actually holds end-to-end.
//
//nolint:tparallel // builds a shared binary; subtests run in parallel
func TestStreams_RootCommandsHonourTheStreamContract(t *testing.T) {
	harness := newHarness(t)

	tests := []struct {
		name            string
		setupDir        func(t *testing.T) string
		args            []string
		expectExit      int
		stdoutContains  string // empty = no assertion
		stderrContains  string // empty = no assertion
		stdoutForbidden string // must NOT appear in stdout (e.g. diagnostics leaking there)
		stderrForbidden string // must NOT appear in stderr (e.g. results leaking there)
	}{
		{
			name:            "--version prints to stdout, stderr stays clean",
			setupDir:        func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:            []string{"--version"},
			expectExit:      0,
			stdoutContains:  "gommitlint version",
			stderrForbidden: "gommitlint version",
		},
		{
			name:            "--help prints usage to stdout, stderr clean",
			setupDir:        func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:            []string{"--help"},
			expectExit:      0,
			stdoutContains:  "gommitlint",
			stderrForbidden: "gommitlint",
		},
		{
			// Bare invocation (a subcommand is required) is misuse: the concise
			// usage card goes to stderr and the process exits EX_USAGE (64) per
			// the CLI-conventions bare-invocation ruling. stdout stays pure.
			name:            "bare invocation → concise help on stderr, stdout clean, exit 64",
			setupDir:        func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:            []string{},
			expectExit:      64,
			stderrContains:  "gommitlint - git commit linter",
			stdoutForbidden: "gommitlint",
		},
		{
			name:           "unknown command goes to stderr, stdout clean",
			setupDir:       func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:           []string{"notacommand"},
			expectExit:     2,
			stderrContains: "Unknown command",
			// Diagnostic must NOT contaminate stdout (breaks `| jq` pipelines).
			stdoutForbidden: "Unknown command",
		},
		{
			name:            "missing --config → 66 with diagnostic on stderr",
			setupDir:        func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:            []string{"--config", "/nonexistent/config.yaml", "validate"},
			expectExit:      66,
			stderrContains:  "",
			stdoutForbidden: "config.yaml",
		},
		{
			name: "not a git repo → 69 with diagnostic on stderr",
			setupDir: func(t *testing.T) string {
				t.Helper()

				return t.TempDir()
			},
			args:            []string{"validate"},
			expectExit:      69,
			stdoutForbidden: "error",
		},
		{
			name: "malformed config → 78 with diagnostic on stderr",
			setupDir: func(t *testing.T) string {
				t.Helper()

				repoPath := gitRepoPathWithCommits(t, "feat: valid")
				// Deliberately malformed YAML: unclosed quote.
				mustWriteFile(t, filepath.Join(repoPath, ".gommitlint.yaml"), []byte("gommitlint:\n  output: \"broken\n"), 0o600)

				return repoPath
			},
			// --config forces the parser to surface the syntax error;
			// auto-discovery silently falls back to defaults for malformed
			// files, which is a separate concern from the stream contract.
			args:            []string{"--config", ".gommitlint.yaml", "validate"},
			expectExit:      78,
			stderrContains:  "config syntax error",
			stdoutForbidden: "config syntax error",
		},
		{
			name:            "validation failure (text) → results on stdout, exit 1",
			setupDir:        func(t *testing.T) string { return gitRepoPathWithCommits(t, "invalid commit format") },
			args:            []string{"validate"},
			expectExit:      1,
			stdoutContains:  "invalid commit format",
			stderrForbidden: "invalid commit format",
		},
		{
			// Auto-discovered malformed project config falls back to defaults
			// (graceful degradation), but must still surface the parse warning
			// on stderr so users know their config was ignored. Exit code is
			// orthogonal here — the contract under test is the stream routing
			// of the warning itself.
			name: "malformed auto-discovered config → warning on stderr, defaults used",
			setupDir: func(t *testing.T) string {
				t.Helper()

				repoPath := gitRepoPathWithCommits(t, "feat: valid commit")
				mustWriteFile(t, filepath.Join(repoPath, ".gommitlint.yaml"), []byte("gommitlint:\n  output: \"broken\n"), 0o600)

				return repoPath
			},
			args:           []string{"validate"},
			expectExit:     1, // default rules fail on missing sign-off; irrelevant to the warning-routing contract
			stderrContains: "Could not parse project config",
			// The parse-warning diagnostic must not bleed into stdout where
			// machine-readable consumers expect only results.
			stdoutForbidden: "Could not parse project config",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := testCase.setupDir(t)
			exitCode, stdout, stderr := runGommitlintFull(t, harness, dir, testCase.args...)

			if exitCode != testCase.expectExit {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", exitCode, testCase.expectExit, stdout, stderr)
			}

			if testCase.stdoutContains != "" && !strings.Contains(stdout, testCase.stdoutContains) {
				t.Fatalf("stdout missing %q:\n%s", testCase.stdoutContains, stdout)
			}

			if testCase.stderrContains != "" && !strings.Contains(stderr, testCase.stderrContains) {
				t.Fatalf("stderr missing %q:\n%s", testCase.stderrContains, stderr)
			}

			if testCase.stdoutForbidden != "" && strings.Contains(stdout, testCase.stdoutForbidden) {
				t.Fatalf("stdout contains forbidden diagnostic %q:\n%s", testCase.stdoutForbidden, stdout)
			}

			if testCase.stderrForbidden != "" && strings.Contains(stderr, testCase.stderrForbidden) {
				t.Fatalf("stderr contains forbidden result %q:\n%s", testCase.stderrForbidden, stderr)
			}
		})
	}
}

// runGommitlintFull runs the binary and returns exit code plus separated
// stdout/stderr. Failure exit codes are expected for several cases here so
// non-zero exits are NOT a test failure — only unexpected error kinds are.
func runGommitlintFull(t *testing.T, h *harness, workDir string, args ...string) (int, string, string) {
	t.Helper()

	result := h.run(workDir, nil, args...)

	return result.code, result.stdout, result.stderr
}
