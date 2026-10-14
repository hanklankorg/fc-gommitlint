//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

func TestHookInstall_InstallsCommitMsgAndPrePushHooks(t *testing.T) {
	repo := gitrepo.NewRepoForHooks(t)

	outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)

	commitMsgHook := filepath.Join(repo.Path, ".git", "hooks", "commit-msg")
	prePushHook := filepath.Join(repo.Path, ".git", "hooks", "pre-push")

	check.FileExists(t, commitMsgHook, "commit-msg hook should be installed")
	check.FileExists(t, prePushHook, "pre-push hook should be installed")

	commitMsgContent, err := os.ReadFile(commitMsgHook)
	check.NoError(t, err)
	check.Contains(t, string(commitMsgContent), "--message-file", "commit-msg hook should validate with message-file")
	check.Contains(t, string(commitMsgContent), "gommitlint", "commit-msg hook should contain gommitlint command")

	prePushContent, err := os.ReadFile(prePushHook)
	check.NoError(t, err)
	check.Contains(t, string(prePushContent), `hook run pre-push "$@"`, "pre-push hook should delegate protocol handling")
	check.Contains(t, string(prePushContent), "exec", "pre-push hook should preserve gommitlint's failure code")

	check.Contains(t, outputStr, "commit-msg hook", "output should mention commit-msg hook")
	check.Contains(t, outputStr, "pre-push hook", "output should mention pre-push hook")
}

func TestHookRemove_RemovesBothHooks(t *testing.T) {
	repo := gitrepo.NewRepoForHooks(t)

	_, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)

	commitMsgHook := filepath.Join(repo.Path, ".git", "hooks", "commit-msg")
	prePushHook := filepath.Join(repo.Path, ".git", "hooks", "pre-push")

	check.FileExists(t, commitMsgHook, "commit-msg hook should exist before removal")
	check.FileExists(t, prePushHook, "pre-push hook should exist before removal")

	outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "remove", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)

	check.NoFileExists(t, commitMsgHook, "commit-msg hook should be removed")
	check.NoFileExists(t, prePushHook, "pre-push hook should be removed")

	check.Contains(t, outputStr, "commit-msg hook", "output should mention commit-msg hook")
	check.Contains(t, outputStr, "pre-push hook", "output should mention pre-push hook")
}

func TestHookStatus_ReportsBothHooksInstalled(t *testing.T) {
	repo := gitrepo.NewRepoForHooks(t)

	_, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)

	outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)
	check.Contains(t, outputStr, "commit-msg", "status should show commit-msg hook after install")
	check.Contains(t, outputStr, "pre-push", "status should show pre-push hook after install")
	check.Contains(t, outputStr, "installed", "status should indicate hooks are installed")
}

func TestPrePushHook_ForwardsGitProtocol(t *testing.T) {
	repo := gitrepo.NewRepoForHooks(t)

	prePushHook := filepath.Join(repo.Path, ".git", "hooks", "pre-push")

	_, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repo.Path}, repo.Path)
	check.NoError(t, err)

	content, err := os.ReadFile(prePushHook)
	check.NoError(t, err)

	hookContent := string(content)

	check.Contains(t, hookContent, "exec", "pre-push hook should preserve gommitlint's failure code")
	check.Contains(t, hookContent, "#!/bin/sh", "pre-push hook should be a shell script")
	check.Contains(t, hookContent, "pre-push hook", "pre-push hook should identify itself")
	check.Contains(t, hookContent, `"$GOMMITLINT" hook run pre-push "$@"`, "pre-push hook should forward Git's protocol")
}
