// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/adapters/gitcli"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// The contract tests compare go-git with the git CLI, so a missing git is a
// tool requirement stated as a failure, not a skip: a skip reads as a pass in a
// long run, which is how the cross-adapter comparison would silently disappear
// from a machine (the gitcli signing test fails for the same reason).
func TestRepositoryAdapters_CommitReaderContract(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required by the adapter contract tests: %v", err)
	}

	isolateContractGit(t)

	repoPath := newContractRepository(t)
	adapters := repositoryContractAdapters()

	for _, adapter := range adapters {
		t.Run(adapter.name, func(t *testing.T) {
			repo := adapter.open(t, repoPath)

			headCommit, err := repo.GetCommit(t.Context(), "HEAD")
			check.NoError(t, err)
			check.Equal(t, "fix: second", headCommit.Subject)
			check.Equal(t, "Test User", headCommit.Author)
			check.Equal(t, "test@example.com", headCommit.AuthorEmail)
			check.False(t, headCommit.IsMergeCommit)

			headCommits, err := repo.GetHeadCommits(t.Context(), 2)
			check.NoError(t, err)
			check.Len(t, headCommits, 2)
			check.Equal(t, "fix: second", headCommits[0].Subject)
			check.Equal(t, "feat: first", headCommits[1].Subject)

			rangeCommits, err := repo.GetCommitRange(t.Context(), headCommits[1].Hash, "HEAD")
			check.NoError(t, err)
			check.Len(t, rangeCommits, 1)
			check.Equal(t, "fix: second", rangeCommits[0].Subject)
		})
	}
}

func TestRepositoryAdapters_BranchAnalyzerContract(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required by the adapter contract tests: %v", err)
	}

	isolateContractGit(t)

	repoPath := newContractRepository(t)
	adapters := repositoryContractAdapters()

	for _, adapter := range adapters {
		t.Run(adapter.name, func(t *testing.T) {
			repo := adapter.open(t, repoPath)

			branch, err := repo.GetCurrentBranch(t.Context())
			check.NoError(t, err)
			check.Equal(t, "main", branch)

			exists, err := repo.BranchExists(t.Context(), "main")
			check.NoError(t, err)
			check.True(t, exists)

			// The other direction. Asserting only that an existing branch is
			// found leaves the predicate half-tested: BranchExists returning
			// true unconditionally satisfies the line above.
			missing, err := repo.BranchExists(t.Context(), "no-such-branch")
			check.NoError(t, err)
			check.False(t, missing, "a branch that was never created must not be reported as existing")

			// Zero here is correct — HEAD is main — but zero is also what a
			// GetCommitsAheadCount that never counted anything returns. The
			// non-zero case lives in
			// TestRepositoryAdapters_BranchAnalyzerCountsCommitsAhead.
			ahead, err := repo.GetCommitsAheadCount(t.Context(), "main")
			check.NoError(t, err)
			check.Zero(t, ahead)
		})
	}
}

// TestRepositoryAdapters_BranchAnalyzerCountsCommitsAhead is the positive
// control for the ahead-count assertion in the test above, which can only ever
// observe zero because its fixture has HEAD on the base branch itself. An
// implementation that returned 0 unconditionally — or that counted the wrong
// direction — passes there and fails here.
//
// Both adapters must agree on the number, which is the point of a contract
// test: go-git and the git CLI compute this independently.
func TestRepositoryAdapters_BranchAnalyzerCountsCommitsAhead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required by the adapter contract tests: %v", err)
	}

	isolateContractGit(t)

	repoPath := newContractRepository(t)
	runContractGit(t, repoPath, "checkout", "-b", "feature")
	writeContractCommit(t, repoPath, "third", "feat: third")
	writeContractCommit(t, repoPath, "fourth", "feat: fourth")

	for _, adapter := range repositoryContractAdapters() {
		t.Run(adapter.name, func(t *testing.T) {
			repo := adapter.open(t, repoPath)

			ahead, err := repo.GetCommitsAheadCount(t.Context(), "main")
			check.NoError(t, err)
			check.Equal(t, 2, ahead, "feature is two commits ahead of main")

			// Counted against the branch itself, the answer is zero again —
			// which is what makes the 2 above a measurement rather than a
			// constant.
			none, err := repo.GetCommitsAheadCount(t.Context(), "feature")
			check.NoError(t, err)
			check.Zero(t, none)
		})
	}
}

// TestRepositoryAdapters_BranchAnalyzerPrefersBranchOverSameNamedTag pins which
// object a bare reference name counts against when a tag and a branch share it.
// git's own revision rules read refs/tags before refs/heads, so an adapter that
// hands the shorthand to git counts from the tag while an adapter that resolves
// the name itself counts from the branch. The reference is a branch name, and
// gitrefs.ReferenceCandidates documents branches first, so both adapters must
// count against the branch.
func TestRepositoryAdapters_BranchAnalyzerPrefersBranchOverSameNamedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("git is required by the adapter contract tests: %v", err)
	}

	isolateContractGit(t)

	repoPath := newContractRepository(t)
	runContractGit(t, repoPath, "tag", "main", "HEAD~1")
	runContractGit(t, repoPath, "checkout", "-b", "feature")
	writeContractCommit(t, repoPath, "third", "feat: third")
	writeContractCommit(t, repoPath, "fourth", "feat: fourth")

	for _, adapter := range repositoryContractAdapters() {
		t.Run(adapter.name, func(t *testing.T) {
			repo := adapter.open(t, repoPath)

			ahead, err := repo.GetCommitsAheadCount(t.Context(), "main")
			check.NoError(t, err)
			check.Equal(t, 2, ahead, "the branch main is the reference, not the tag main one commit behind it")
		})
	}
}

type repositoryContractAdapter struct {
	name string
	open func(*testing.T, string) domain.Repository
}

func repositoryContractAdapters() []repositoryContractAdapter {
	return []repositoryContractAdapter{
		{
			name: "gogit",
			open: func(t *testing.T, repoPath string) domain.Repository {
				t.Helper()

				repo, err := git.NewRepositoryFromPath(repoPath)
				check.NoError(t, err)

				return repo
			},
		},
		{
			name: "gitcli",
			open: func(t *testing.T, repoPath string) domain.Repository {
				t.Helper()

				return gitcli.NewRepository(repoPath)
			},
		},
	}
}

func newContractRepository(t *testing.T) string {
	t.Helper()

	repoPath := t.TempDir()
	runContractGit(t, repoPath, "init", "--object-format=sha1", "--initial-branch=main")
	runContractGit(t, repoPath, "config", "user.name", "Test User")
	runContractGit(t, repoPath, "config", "user.email", "test@example.com")
	runContractGit(t, repoPath, "config", "commit.gpgsign", "false")
	runContractGit(t, repoPath, "config", "tag.gpgsign", "false")

	writeContractCommit(t, repoPath, "first", "feat: first")
	writeContractCommit(t, repoPath, "second", "fix: second")

	return repoPath
}

func writeContractCommit(t *testing.T, repoPath, content, message string) {
	t.Helper()

	filePath := filepath.Join(repoPath, "file.txt")
	check.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))
	runContractGit(t, repoPath, "add", "file.txt")
	runContractGit(t, repoPath, "commit", "--no-gpg-sign", "--no-verify", "-m", message)
}

func isolateContractGit(t *testing.T) {
	t.Helper()

	root := t.TempDir()
	home := filepath.Join(root, "home")
	hooks := filepath.Join(root, "hooks")

	check.NoError(t, os.MkdirAll(home, 0o700))
	check.NoError(t, os.MkdirAll(hooks, 0o700))

	globalConfig := filepath.Join(root, "gitconfig")
	check.NoError(t, os.WriteFile(globalConfig, nil, 0o600))

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	t.Setenv("GIT_ASKPASS", "")
	t.Setenv("SSH_ASKPASS", "")
	t.Setenv("GCM_INTERACTIVE", "Never")
	t.Setenv("GNUPGHOME", filepath.Join(home, ".gnupg"))
	t.Setenv("SSH_AUTH_SOCK", "")

	t.Setenv("GIT_CONFIG_COUNT", "4")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	t.Setenv("GIT_CONFIG_KEY_1", "commit.gpgSign")
	t.Setenv("GIT_CONFIG_VALUE_1", "false")
	t.Setenv("GIT_CONFIG_KEY_2", "tag.gpgSign")
	t.Setenv("GIT_CONFIG_VALUE_2", "false")
	t.Setenv("GIT_CONFIG_KEY_3", "credential.helper")
	t.Setenv("GIT_CONFIG_VALUE_3", "")
}

func runContractGit(t *testing.T, repoPath string, args ...string) {
	t.Helper()

	// #nosec G204 -- test invokes a fixed git executable with controlled arguments.
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = repoPath
	// An inherited GIT_DIR (a hook or rebase --exec) would redirect the fixture into another repository;
	// the configuration isolateContractGit sets stays.
	cmd.Env = gitcli.Environment()
	output, err := cmd.CombinedOutput()
	check.NoError(t, err, string(output))
}
