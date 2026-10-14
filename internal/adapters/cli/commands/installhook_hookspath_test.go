// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	// An inherited GIT_DIR (a hook or rebase --exec) would redirect the fixture into another repository.
	environment := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GIT_") })
	environment = append(environment,
		"GIT_CONFIG_GLOBAL="+filepath.Join(t.TempDir(), "gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	cmd.Env = environment

	out, err := cmd.CombinedOutput()
	check.NoError(t, err, string(out))
}

// A repository whose hooks are redirected by core.hooksPath must refuse a local
// install instead of writing a file git never runs.
func TestInstallHook_RefusesWhenCoreHooksPathRedirects(t *testing.T) {
	isolateGlobalGitConfig(t)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "config", "core.hooksPath", ".githooks")
	check.NoError(t, os.Mkdir(filepath.Join(repo, ".githooks"), 0o755))

	_, err := installSingleHook(t.Context(), false, repo, hookTypeCommitMsg, createCommitMsgHookScript(), nil)

	check.ErrorIs(t, err, ErrHooksPathRedirected)
	check.Contains(t, err.Error(), filepath.Join(repo, ".githooks"))
	_, statErr := os.Stat(filepath.Join(repo, ".git", "hooks", "commit-msg"))
	check.True(t, errors.Is(statErr, os.ErrNotExist), "no inert hook may be written under .git/hooks")
}

// In a linked worktree hooks live in the main repository's common git dir;
// install must land there and report it.
func TestInstallHook_LinkedWorktreeInstallsIntoCommonDir(t *testing.T) {
	isolateGlobalGitConfig(t)

	mainRepo := t.TempDir()
	gitIn(t, mainRepo, "init", "-q")
	gitIn(t, mainRepo, "commit", "-q", "--allow-empty", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "wt")
	gitIn(t, mainRepo, "worktree", "add", "-q", worktree, "-b", "feature")

	result, err := installSingleHook(t.Context(), false, worktree, hookTypeCommitMsg, createCommitMsgHookScript(), nil)

	check.NoError(t, err)
	check.Equal(t, filepath.Join(mainRepo, ".git", "hooks", "commit-msg"), result.HookPath)
	check.FileExists(t, result.HookPath)
}

func TestHooksPathRedirect_ReportsOnlyConfiguredRedirects(t *testing.T) {
	isolateGlobalGitConfig(t)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")

	_, redirected, err := hooksPathRedirect(t.Context(), repo)
	check.NoError(t, err)
	check.False(t, redirected, "default hooks dir is not a redirect")

	gitIn(t, repo, "config", "core.hooksPath", ".githooks")
	dir, redirected, err := hooksPathRedirect(t.Context(), repo)
	check.NoError(t, err)
	check.True(t, redirected)
	check.Equal(t, filepath.Join(repo, ".githooks"), dir)
}

func TestHooksPathRedirect_PropagatesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, err := hooksPathRedirect(ctx, t.TempDir())
	check.ErrorIs(t, err, context.Canceled)
}

// git exports the variables that located its own repository to a hook: GIT_DIR
// whenever it did not find the repository as ./.git (a subdirectory, git -C, a
// worktree), GIT_WORK_TREE, and GIT_INDEX_FILE for a partial commit. A hook
// that installs into another repository must act on --repo-path, not on them.
func TestHooksPathRedirect_IgnoresInheritedRepositoryEnvironment(t *testing.T) {
	isolateGlobalGitConfig(t)

	decoy := t.TempDir()
	gitIn(t, decoy, "init", "-q")
	gitIn(t, decoy, "config", "core.hooksPath", ".githooks")
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)

	dir, redirected, err := hooksPathRedirect(t.Context(), repo)
	check.NoError(t, err)
	check.False(t, redirected, "the hook repository's core.hooksPath was reported for %s: %s", repo, dir)

	result, err := installSingleHook(t.Context(), false, repo, hookTypeCommitMsg, createCommitMsgHookScript(), nil)
	check.NoError(t, err)
	check.Equal(t, filepath.Join(repo, ".git", "hooks", "commit-msg"), result.HookPath)
	check.FileExists(t, result.HookPath)

	gitIn(t, decoy, "config", "--unset", "core.hooksPath")
	gitIn(t, repo, "config", "core.hooksPath", ".githooks")

	dir, redirected, err = hooksPathRedirect(t.Context(), repo)
	check.NoError(t, err)
	check.True(t, redirected, "the target repository's core.hooksPath must be read")
	check.Equal(t, filepath.Join(repo, ".githooks"), dir)
}

func TestHookStatus_WarnsWhenCoreHooksPathRedirects(t *testing.T) {
	isolateGlobalGitConfig(t)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	gitIn(t, repo, "config", "core.hooksPath", ".githooks")

	status, err := defaultLocalHookService(t.Context()).Status(t.Context(), repo)

	check.NoError(t, err)
	check.True(t, status.HooksPathRedirected)
	check.Equal(t, filepath.Join(repo, ".githooks"), status.EffectiveHooksPath)
	check.Contains(t, formatHookStatus(status, false, nil), "local entries under")
	check.Contains(t, formatHookStatus(status, false, nil), "are inactive")
}

func TestInstallHook_CancelledBeforeCreatingHooksDirectory(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	check.NoError(t, os.RemoveAll(filepath.Join(repo, ".git", "hooks")))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := installSingleHook(ctx, false, repo, hookTypeCommitMsg, createCommitMsgHookScript(), nil)

	check.ErrorIs(t, err, context.Canceled)

	_, statErr := os.Stat(filepath.Join(repo, ".git", "hooks"))
	check.True(t, errors.Is(statErr, os.ErrNotExist), "cancelled install must not create the hooks directory")
}

func TestRemoveHook_AllowsStaleLocalHookAfterRedirect(t *testing.T) {
	isolateGlobalGitConfig(t)

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	result, err := installSingleHookWithFS(t.Context(), filesystem.NewOSFileSystem(), false, repo, hookTypeCommitMsg, createCommitMsgHookScript(), nil)
	check.NoError(t, err)
	gitIn(t, repo, "config", "core.hooksPath", ".githooks")

	_, err = newLocalHookService(t.Context(), filesystem.NewOSFileSystem()).Remove(apphooks.RemovalRequest{
		HookType: hookTypeCommitMsg,
		RepoPath: repo,
	})

	check.NoError(t, err)

	_, statErr := os.Stat(result.HookPath)
	check.True(t, errors.Is(statErr, os.ErrNotExist), "stale managed hook should be removed")
}
