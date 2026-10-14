//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
)

// TestValidate_SupportsSHA256ObjectFormat runs the validate modes against a
// repository initialised with --object-format=sha256: two commits on main and
// one more on the checked-out feature branch. Each mode must report the
// commits it was asked for, so a range or base branch that silently resolved
// to nothing cannot pass as a success.
func TestValidate_SupportsSHA256ObjectFormat(t *testing.T) {
	t.Setenv("GOMMITLINT_NO_CI_DETECT", "1")

	repoPath := createSHA256Repo(t)

	testCases := []struct {
		name         string
		args         []string
		totalCommits int
	}{
		{
			name:         "validate head commit in sha256 repo",
			args:         []string{"gommitlint", "validate", "--format", "json"},
			totalCommits: 1,
		},
		{
			name:         "validate count in sha256 repo",
			args:         []string{"gommitlint", "validate", "--count", "2", "--format", "json"},
			totalCommits: 2,
		},
		{
			name:         "validate range in sha256 repo",
			args:         []string{"gommitlint", "validate", "--range", "main..HEAD", "--format", "json"},
			totalCommits: 1,
		},
		{
			name:         "validate base branch in sha256 repo",
			args:         []string{"gommitlint", "validate", "--base-branch", "main", "--format", "json"},
			totalCommits: 1,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := RunCLICommand(t, testCase.args, repoPath)
			check.NoError(t, err)

			var report struct {
				Summary struct {
					TotalCommits int `json:"totalCommits"`
				} `json:"summary"`
			}

			check.NoError(t, json.Unmarshal([]byte(output), &report), "not a JSON report: %s", output)
			check.Equal(t, testCase.totalCommits, report.Summary.TotalCommits)
		})
	}
}

func createSHA256Repo(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runGit(t, repoPath, "init", "--object-format=sha256", "--initial-branch=main")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")

	writeRepoFile(t, repoPath, "test.txt", "initial\n")
	runGit(t, repoPath, "add", "test.txt")
	runGit(t, repoPath, "commit", "--signoff", "-m", "feat: initial commit")

	writeRepoFile(t, repoPath, "test.txt", "main\n")
	runGit(t, repoPath, "add", "test.txt")
	runGit(t, repoPath, "commit", "--signoff", "-m", "fix: main branch update")

	runGit(t, repoPath, "checkout", "-b", "feature/sha256")
	writeRepoFile(t, repoPath, "test.txt", "feature\n")
	runGit(t, repoPath, "add", "test.txt")
	runGit(t, repoPath, "commit", "--signoff", "-m", "docs: feature update")

	return repoPath
}

func runGit(t *testing.T, repoPath string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = repoPath
	testkit.SetIsolatedEnvironment(t, cmd)
	output, err := cmd.CombinedOutput()
	check.NoError(t, err, "git %v failed: %s", args, string(output))
}

func writeRepoFile(t *testing.T, repoPath, name, content string) {
	t.Helper()

	filePath := filepath.Join(repoPath, name)
	check.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
}
