//
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestCompletionGitReferences_InRepo(t *testing.T) {
	repoPath := createTestRepository(t)

	refs := runCompletionValues(t, repoPath, completionGitReferences)

	check.Contains(t, refs, "HEAD")
	check.Contains(t, refs, "main")
	check.Contains(t, refs, "origin/main")
	check.Contains(t, refs, "v1.0.0")
}

func TestCompletionGitBranches_InRepo(t *testing.T) {
	repoPath := createTestRepository(t)

	branches := runCompletionValues(t, repoPath, completionGitBranches)

	check.Contains(t, branches, "main")
	check.Contains(t, branches, "origin/main")
	check.NotContains(t, branches, "v1.0.0")
}

func TestCompletionGitRanges_InRepo(t *testing.T) {
	repoPath := createTestRepository(t)

	ranges := runCompletionValues(t, repoPath, completionGitRanges)

	check.Contains(t, ranges, "main..")
	check.NotContains(t, ranges, "main...")
}

func TestCompletionGitReferences_NoRepo(t *testing.T) {
	t.Chdir(createTestRepository(t))

	repoPath := t.TempDir()

	refs := runCompletionValues(t, repoPath, completionGitReferences)

	check.Empty(t, refs)
}

func TestCompletionGitReferences_DiscoversRepositoryFromSubdirectory(t *testing.T) {
	repoPath := createTestRepository(t)
	nestedPath := filepath.Join(repoPath, "nested", "directory")
	check.NoError(t, os.MkdirAll(nestedPath, 0o700))
	t.Chdir(nestedPath)

	refs := runCompletionValues(t, "", completionGitReferences)

	check.Contains(t, refs, "HEAD")
	check.Contains(t, refs, "main")
}

func TestCompletionGitValues_SHA256Repository(t *testing.T) {
	repoPath := createSHA256CompletionRepository(t)

	refs := runCompletionValues(t, repoPath, completionGitReferences)
	branches := runCompletionValues(t, repoPath, completionGitBranches)
	ranges := runCompletionValues(t, repoPath, completionGitRanges)

	check.Contains(t, refs, "HEAD")
	check.Contains(t, refs, "main")
	check.Contains(t, refs, "v1.0.0")
	check.Contains(t, branches, "main")
	check.NotContains(t, branches, "v1.0.0")
	check.Contains(t, ranges, "main..")
}

func createSHA256CompletionRepository(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	home := t.TempDir()
	// An inherited GIT_DIR (a hook or rebase --exec) would redirect the fixture into another repository.
	environment := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GIT_") })
	environment = append(environment,
		"HOME="+home,
		"XDG_CONFIG_HOME="+home,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	for _, args := range [][]string{
		{"init", "--object-format=sha256", "--initial-branch=main"},
		{"commit", "--allow-empty", "--no-gpg-sign", "-m", "feat: initial"},
		{"tag", "v1.0.0"},
	} {
		command := exec.CommandContext(t.Context(), "git", append([]string{"-C", repoPath}, args...)...)
		command.Env = environment

		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}

	return repoPath
}
