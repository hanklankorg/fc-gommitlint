// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestFormatManagedHookRemovalResult_ReportsPerHookResults(t *testing.T) {
	t.Parallel()

	results := map[string]hookRemovalAttempt{
		hookTypeCommitMsg: {
			result: HookRemovalResult{
				RepoPath: testHookRepoPath,
				HookPath: testHookCommitMsgPath,
				HookType: hookTypeCommitMsg,
			},
		},
		hookTypePrePush: {
			result: HookRemovalResult{RepoPath: testHookRepoPath, HookType: hookTypePrePush},
		},
	}

	formatted := formatManagedHookRemovalResult(results, false, output.DefaultTheme, nil)

	for _, expected := range []string{
		testOutputRepoPath,
		"Removing git hooks",
		"commit-msg hook:",
		"Removed: /test/repo/.git/hooks/commit-msg",
		"pre-push hook:",
		"Hook not found",
		"Git hooks removed successfully",
		"Reinstall: gommitlint hook install",
		"Check status: gommitlint hook status",
	} {
		check.Contains(t, formatted, expected)
	}
}

func TestFormatGlobalHookRemovalResultWithStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		statuses          []GlobalHookRemovalStatus
		hooksPathAction   apphooks.PathAction
		restoredHooksPath string
		wantContains      []string
	}{
		{
			name: "no_hooks_found",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: false, Removed: false},
				{HookType: hookTypePrePush, Found: false, Removed: false},
			},
			wantContains: []string{
				testOutputCommitMsgHook,
				testOutputHookNotFound,
				testOutputPrePushHook,
				testOutputHookNotFound,
				"No global hooks found to remove",
				"'core.hooksPath' unset",
			},
		},
		{
			name: "hooks_removed",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: true, Removed: true},
				{HookType: hookTypePrePush, Found: true, Removed: true},
			},
			wantContains: []string{
				testOutputCommitMsgHook,
				testOutputHookRemoved,
				testOutputPrePushHook,
				testOutputHookRemoved,
				"✓ Global git hooks removed successfully!",
			},
		},
		{
			name: "hooks_found_but_not_gommitlint",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: true, Removed: false},
				{HookType: hookTypePrePush, Found: true, Removed: false},
			},
			wantContains: []string{
				testOutputCommitMsgHook,
				"ℹ Not a gommitlint hook",
				testOutputPrePushHook,
				"ℹ Not a gommitlint hook",
				"No gommitlint hooks found to remove",
			},
		},
		{
			name: "mixed_status",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: true, Removed: true},
				{HookType: hookTypePrePush, Found: false, Removed: false},
			},
			wantContains: []string{
				testOutputCommitMsgHook,
				testOutputHookRemoved,
				testOutputPrePushHook,
				testOutputHookNotFound,
				"✓ Global git hooks removed successfully!",
			},
		},
		{
			name: "foreign_plugins_keep_dispatcher_and_hooks_path",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: true, Removed: true, ForeignPlugins: []string{"/global/commit-msg.d/10-other.sh"}},
				{HookType: hookTypePrePush, Found: true, Removed: true},
			},
			hooksPathAction: apphooks.HooksPathUntouched,
			wantContains: []string{
				"ℹ Dispatcher kept: other tools still plug into it",
				"/global/commit-msg.d/10-other.sh",
				"'core.hooksPath' left unchanged",
			},
		},
		{
			name: "restored_hooks_path",
			statuses: []GlobalHookRemovalStatus{
				{HookType: hookTypeCommitMsg, Found: true, Removed: true},
				{HookType: hookTypePrePush, Found: true, Removed: true},
			},
			hooksPathAction:   apphooks.HooksPathRestored,
			restoredHooksPath: "/home/user/.githooks",
			wantContains: []string{
				"'core.hooksPath' restored to: /home/user/.githooks",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			action := testCase.hooksPathAction
			if action == "" {
				action = apphooks.HooksPathUnset
			}

			result := GlobalRemovalResult{
				Hooks:             testCase.statuses,
				HooksPathAction:   action,
				RestoredHooksPath: testCase.restoredHooksPath,
			}

			output := formatGlobalHookRemovalResultWithStatus(result, false, "", nil)

			for _, want := range testCase.wantContains {
				check.Contains(t, output, want, "output should contain: %s", want)
			}
		})
	}
}

func TestFormatManagedHookRemovalResult_ReportsMissingHooksAccurately(t *testing.T) {
	t.Parallel()

	results := map[string]hookRemovalAttempt{}
	for _, hookType := range apphooks.DefaultHookTypes() {
		results[hookType] = hookRemovalAttempt{result: HookRemovalResult{RepoPath: testHookRepoPath, HookType: hookType}}
	}

	formatted := formatManagedHookRemovalResult(results, false, output.DefaultTheme, nil)

	check.NotContains(t, formatted, "✓ Removed:")
	check.Contains(t, formatted, "Hook not found")
	check.Contains(t, formatted, "No local hooks found to remove")
}

func TestHookRemovalFormatters_SanitizeDynamicValues(t *testing.T) {
	t.Parallel()

	hostile := "/path\nforged\x1b]52;c;x\a"
	local := formatManagedHookRemovalResult(map[string]hookRemovalAttempt{
		hookTypeCommitMsg: {result: HookRemovalResult{RepoPath: hostile, HookPath: hostile, HookType: hookTypeCommitMsg}},
		hookTypePrePush:   {result: HookRemovalResult{RepoPath: hostile, HookPath: hostile, HookType: hookTypePrePush}},
	}, false, output.DefaultTheme, nil)
	global := formatGlobalHookRemovalResultWithStatus(GlobalRemovalResult{
		HooksPath:         hostile,
		RestoredHooksPath: hostile,
		HooksPathAction:   apphooks.HooksPathRestored,
		ForeignHooks:      []string{hostile},
		Hooks: []GlobalHookRemovalStatus{{
			HookType:       hookTypeCommitMsg,
			Found:          true,
			ForeignPlugins: []string{hostile},
		}},
	}, false, output.DefaultTheme, nil)

	for _, formatted := range []string{local, global} {
		check.NotContains(t, formatted, "\x1b")
		check.NotContains(t, formatted, "\a")
		check.NotContains(t, formatted, "\nforged")
	}
}

func TestExecuteRemoveHook_QuietSuppressesSuccessOutput(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	isolateGlobalGitConfig(t)

	hooksDir := filepath.Join(repo, ".git", "hooks")
	check.NoError(t, os.WriteFile(filepath.Join(hooksDir, hookTypeCommitMsg), []byte(createCommitMsgHookScript()), 0o755))
	check.NoError(t, os.WriteFile(filepath.Join(hooksDir, hookTypePrePush), []byte(createPrePushHookScript()), 0o755))

	var stdout bytes.Buffer

	root := hookCommandTestRoot(repo, &stdout)

	err := root.Run(t.Context(), []string{"gommitlint", "--quiet", commandNameHook, commandNameHookRemove})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
}

func TestExecuteRemoveGlobalHook_QuietSuppressesSuccessOutput(t *testing.T) {
	isolateGlobalGitConfig(t)
	_, err := InstallGlobalHooks(t.Context(), false, nil)
	check.NoError(t, err)

	var stdout bytes.Buffer

	root := hookCommandTestRoot("", &stdout)

	err = root.Run(t.Context(), []string{"gommitlint", "--quiet", commandNameHook, commandNameHookRemove, "--global"})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
}

func TestExecuteRemoveHook_RejectsGlobalDryRunBeforeMutation(t *testing.T) {
	configHome := t.TempDir()
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := hookCommandTestRoot("", io.Discard)

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, commandNameHookRemove, "--global", "--dry-run"})

	check.ErrorIs(t, err, ErrUnexpectedArgument)

	_, statErr := os.Stat(filepath.Join(configHome, "git", "git-hooks"))
	check.True(t, errors.Is(statErr, os.ErrNotExist))
}

func TestExecuteRemoveHook_PreservesOperationAndWriterErrors(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	isolateGlobalGitConfig(t)

	hooksDir := filepath.Join(repo, ".git", "hooks")
	check.NoError(t, os.WriteFile(filepath.Join(hooksDir, hookTypeCommitMsg), []byte("#!/bin/sh\necho custom"), 0o755))
	check.NoError(t, os.Mkdir(filepath.Join(hooksDir, hookTypePrePush), 0o755))

	root := hookCommandTestRoot(repo, failingWriter{err: errMockWrite})

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, commandNameHookRemove})

	check.ErrorIs(t, err, ErrNotGommitlintHook)
	check.ErrorIs(t, err, errHookFileNotRegular)
	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)
}
