// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"os/exec"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestRepository_GetCurrentBranch(t *testing.T) {
	t.Parallel()

	repo := createDivergedGitRepo(t)
	adapter := NewRepository(repo.Path())

	branch, err := adapter.GetCurrentBranch(t.Context())
	check.NoError(t, err)
	check.Equal(t, "feature/a-test", branch)
}

func TestRepository_BranchExists(t *testing.T) {
	t.Parallel()

	repo := createDivergedGitRepo(t)
	adapter := NewRepository(repo.Path())

	exists, err := adapter.BranchExists(t.Context(), "main")
	check.NoError(t, err)
	check.True(t, exists)

	exists, err = adapter.BranchExists(t.Context(), "refs/heads/feature/a-test")
	check.NoError(t, err)
	check.True(t, exists)

	exists, err = adapter.BranchExists(t.Context(), "missing-branch")
	check.NoError(t, err)
	check.False(t, exists)
}

func TestRepository_GetCommitsAheadCount(t *testing.T) {
	t.Parallel()

	repo := createDivergedGitRepo(t)
	adapter := NewRepository(repo.Path())

	count, err := adapter.GetCommitsAheadCount(t.Context(), "main")
	check.NoError(t, err)
	check.Equal(t, 2, count)

	_, err = adapter.GetCommitsAheadCount(t.Context(), "missing-branch")
	check.Error(t, err)
	check.ErrorIs(t, err, domain.ErrReferenceNotFound)
}

func createDivergedGitRepo(t *testing.T) *testhelpers.GitRepo {
	t.Helper()

	repo := testhelpers.NewGitRepo(t, true)
	repo.CreateAndCommit("test.txt", "initial\n", "feat: initial commit")
	renameCurrentBranch(t, repo.Path(), "main")
	repo.CreateAndCommit("test.txt", "main-1\n", "fix: main commit 1")
	repo.CreateAndCommit("test.txt", "main-2\n", "docs: main commit 2")
	repo.Checkout("HEAD~2")
	repo.CreateBranch("feature/a-test")
	repo.CreateAndCommit("test.txt", "feature-1\n", "feat: feature commit 1")
	repo.CreateAndCommit("test.txt", "feature-2\n", "fix: feature commit 2")

	return repo
}

func renameCurrentBranch(t *testing.T, repoPath, name string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", "branch", "-M", name)
	cmd.Dir = repoPath
	// An inherited GIT_DIR (a hook or rebase --exec) would rename the branch of another repository.
	cmd.Env = Environment()
	output, err := cmd.CombinedOutput()
	check.NoError(t, err, "failed to rename branch: %s", string(output))
}
