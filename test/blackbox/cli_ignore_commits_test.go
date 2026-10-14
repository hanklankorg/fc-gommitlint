// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build blackbox

package blackbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIgnoreCommits_SkipsMatchingCommitsDuringValidation(t *testing.T) {
	t.Parallel()

	t.Run("basic ignore functionality", func(t *testing.T) {
		t.Parallel()

		// Setup test repository
		repo := newEmptyGitRepo(t)

		repo.withCommit("feat: add new feature")

		configContent := `
 gommitlint:
   ignore_commits:
     - "a1b2c3d4e5f6789012345678901234567890abcd"
   rules:
     enabled:
       - conventional
     disabled:
       - signoff
 `

		warningConfig := `
 gommitlint:
   ignore_commits:
     - "bad"
     - "a1b2c3d4e5f6789012345678901234567890abcd"
   rules:
     enabled:
       - conventional
     disabled:
       - signoff
 `

		writeConfigFile(t, filepath.Join(repo.Path, ".gommitlint.yaml"), configContent)

		harness := newHarness(t)
		exitCode, output := runGommitlint(t, harness, repo.Path, "validate", "--ref", "HEAD")

		// Should pass since commit is valid and ignore list doesn't affect it
		if exitCode != 0 {
			t.Fatalf("validation should pass, exit code = %d\n%s", exitCode, output)
		}
		if !strings.Contains(output, "✓") {
			t.Fatalf("validation output did not show success:\n%s", output)
		}

		writeConfigFile(t, filepath.Join(repo.Path, ".gommitlint.yaml"), warningConfig)
		_, warningOutput := runGommitlint(t, harness, repo.Path, "validate", "--ref", "HEAD")
		if !strings.Contains(warningOutput, "'bad' is not a valid commit SHA in ignore_commits, skipping it") {
			t.Fatalf("validation output did not contain invalid-SHA warning:\n%s", warningOutput)
		}
	})
}

func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	// Use OS directly for integration test - already in temp directory
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}
