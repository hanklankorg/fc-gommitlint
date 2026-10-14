// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestGlobalHooksPath_PrefersXDGConfigHome(t *testing.T) {
	homeDir := t.TempDir()

	tests := []struct {
		name          string
		xdgConfigHome string
		want          string
	}{
		{
			name:          "uses XDG_CONFIG_HOME when set",
			xdgConfigHome: "/custom/config",
			want:          filepath.Join("/custom/config", "git", "git-hooks"),
		},
		{
			name:          "uses HOME/.config when XDG not set",
			xdgConfigHome: "",
			want:          filepath.Join(homeDir, ".config", "git", "git-hooks"),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("HOME", homeDir)
			t.Setenv("XDG_CONFIG_HOME", testCase.xdgConfigHome)

			check.Equal(t, testCase.want, GlobalHooksPath())
		})
	}
}

func TestGlobalHooksPath_IgnoresRelativeXDGConfigHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", "relative-config")

	check.Equal(t, filepath.Join(homeDir, ".config", "git", "git-hooks"), GlobalHooksPath())
}

func TestGlobalHooksPath_RejectsRelativeHomeFallback(t *testing.T) {
	t.Setenv("HOME", "relative-home")
	t.Setenv("XDG_CONFIG_HOME", "")

	check.Empty(t, GlobalHooksPath())
}

func TestReadGlobalHooksPath_PropagatesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := readGlobalHooksPath(ctx)

	check.ErrorIs(t, err, context.Canceled)
}

func TestReadGlobalHooksPath_PropagatesMalformedConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "gitconfig")
	check.NoError(t, os.WriteFile(configPath, []byte("[broken"), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", configPath)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	_, err := readGlobalHooksPath(t.Context())

	check.Error(t, err)
}

func TestGlobalHookPluginPath_AppendsDotDDirectory(t *testing.T) {
	t.Parallel()

	path := GlobalHookPluginPath(commitMsgHookType)

	check.Contains(t, path, "git-hooks")
	check.Contains(t, path, "commit-msg.d")
}

func TestGlobalHookDispatcherPath_OmitsPluginSuffix(t *testing.T) {
	t.Parallel()

	path := GlobalHookDispatcherPath(commitMsgHookType)

	check.Contains(t, path, "git-hooks")
	check.Contains(t, path, "commit-msg")
	check.NotContains(t, path, ".d")
}

func TestCreateDispatcherScript_EmbedsHookType(t *testing.T) {
	t.Parallel()

	script := createDispatcherScript("commit-msg")

	check.Contains(t, script, "#!/bin/sh")
	check.Contains(t, script, "commit-msg")
	check.Contains(t, script, ".d")
	check.Contains(t, script, "dispatcher")
}

func TestCreateCommitMsgPluginScript_InvokesValidate(t *testing.T) {
	t.Parallel()

	script := createCommitMsgPluginScript()

	check.Contains(t, script, "#!/bin/sh")
	check.Contains(t, script, "gommitlint")
	check.Contains(t, script, "validate")
	check.Contains(t, script, "COMMIT_MSG_FILE")
}

func TestCreatePrePushPluginScript_DelegatesToHookRun(t *testing.T) {
	t.Parallel()

	script := createPrePushPluginScript()

	check.Contains(t, script, "#!/bin/sh")
	check.Contains(t, script, "gommitlint")
	check.Contains(t, script, "hook run pre-push")
	check.NotContains(t, script, "git merge-base")
}

func TestInstallGlobalHooksWithFS_CreatesDispatchersAndPlugins(t *testing.T) {
	isolateGlobalGitConfig(t)

	fs := filesystem.NewOSFileSystem()

	_, err := InstallGlobalHooksWithFS(t.Context(), fs, false, nil)
	check.NoError(t, err)

	// Check dispatcher scripts were created
	commitMsgDispatcher := GlobalHookDispatcherPath("commit-msg")
	prePushDispatcher := GlobalHookDispatcherPath("pre-push")

	_, err = fs.Stat(commitMsgDispatcher)
	check.NoError(t, err, "commit-msg dispatcher should exist")

	_, err = fs.Stat(prePushDispatcher)
	check.NoError(t, err, "pre-push dispatcher should exist")

	// Check plugin scripts were created
	commitMsgPlugin := filepath.Join(GlobalHookPluginPath("commit-msg"), "50-gommitlint.sh")
	prePushPlugin := filepath.Join(GlobalHookPluginPath("pre-push"), "50-gommitlint.sh")

	_, err = fs.Stat(commitMsgPlugin)
	check.NoError(t, err, "commit-msg plugin should exist")

	_, err = fs.Stat(prePushPlugin)
	check.NoError(t, err, "pre-push plugin should exist")

	// Verify dispatcher content
	content, err := fs.ReadFile(commitMsgDispatcher)
	check.NoError(t, err)
	check.Contains(t, string(content), "#!/bin/sh")
	check.Contains(t, string(content), "dispatcher")
}

func TestInstallGlobalHooks_ReinstallRequiresForce(t *testing.T) {
	isolateGlobalGitConfig(t)

	fs := filesystem.NewOSFileSystem()

	_, err := InstallGlobalHooksWithFS(t.Context(), fs, false, nil)
	check.NoError(t, err)

	_, err = InstallGlobalHooksWithFS(t.Context(), fs, false, nil)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrGlobalHookExists)

	_, err = InstallGlobalHooksWithFS(t.Context(), fs, true, nil)
	check.NoError(t, err)
}

func TestInstallGlobalHooks_RefusesToReplaceCustomPlugin(t *testing.T) {
	isolateGlobalGitConfig(t)

	fs := filesystem.NewOSFileSystem()

	pluginDir := GlobalHookPluginPath("commit-msg")
	check.NoError(t, fs.MkdirAll(pluginDir, 0o755))

	customPlugin := filepath.Join(pluginDir, "50-gommitlint.sh")
	check.NoError(t, fs.WriteFile(customPlugin, []byte("#!/bin/sh\n# custom\n"), 0o755))

	_, err := InstallGlobalHooksWithFS(t.Context(), fs, true, nil)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrCustomHookExists)
}

func TestRemoveGlobalHooksWithStatusFS_RemovesPluginsAndIsIdempotent(t *testing.T) {
	isolateGlobalGitConfig(t)

	fs := filesystem.NewOSFileSystem()
	ctx := t.Context()

	_, err := InstallGlobalHooksWithFS(ctx, fs, false, nil)
	check.NoError(t, err)

	_, err = fs.Stat(GlobalHooksPath())
	check.NoError(t, err, "hooks directory should exist after install")

	result, err := RemoveGlobalHooksWithStatusFS(ctx, fs)
	check.NoError(t, err)
	check.True(t, result.Hooks[0].Removed, "commit-msg hook should be removed")
	check.True(t, result.Hooks[1].Removed, "pre-push hook should be removed")

	commitMsgPlugin := filepath.Join(GlobalHookPluginPath("commit-msg"), "50-gommitlint.sh")
	_, err = fs.Stat(commitMsgPlugin)
	check.Error(t, err, "gommitlint plugin should be removed")

	result, err = RemoveGlobalHooksWithStatusFS(ctx, fs)
	check.NoError(t, err)
	check.False(t, result.Hooks[0].Found, "second removal should find nothing")
	check.False(t, result.Hooks[1].Found, "second removal should find nothing")
}

func TestRemoveGlobalHooksWithStatusFS_NoHooksDirectory(t *testing.T) {
	// Test case where hooks directory doesn't exist
	isolateGlobalGitConfig(t)

	fs := filesystem.NewOSFileSystem()

	result, err := RemoveGlobalHooksWithStatusFS(t.Context(), fs)

	check.NoError(t, err)
	check.Len(t, result.Hooks, 2)
	check.Equal(t, "commit-msg", result.Hooks[0].HookType)
	check.False(t, result.Hooks[0].Found)
	check.False(t, result.Hooks[0].Removed)
	check.Equal(t, "pre-push", result.Hooks[1].HookType)
	check.False(t, result.Hooks[1].Found)
	check.False(t, result.Hooks[1].Removed)
	check.Equal(t, apphooks.HooksPathUntouched, result.HooksPathAction)
}

func TestInstallGlobalHooks_RefusesForeignHooksPathUnlessForced(t *testing.T) {
	isolateGlobalGitConfig(t)

	ctx := t.Context()

	foreign := filepath.Join(t.TempDir(), "my-hooks")
	check.NoError(t, globalGitConfig{}.SetHooksPath(ctx, foreign))

	_, err := InstallGlobalHooks(ctx, false, nil)
	check.ErrorIs(t, err, ErrHooksPathInUse)
	check.Equal(t, foreign, getGlobalHooksPath(ctx))

	result, err := InstallGlobalHooks(ctx, true, nil)
	check.NoError(t, err)
	check.Equal(t, apphooks.HooksPathReplaced, result.HooksPathAction)
	check.Equal(t, foreign, result.PreviousHooksPath)
	check.Equal(t, GlobalHooksPath(), getGlobalHooksPath(ctx))

	removal, err := RemoveGlobalHooksWithStatusFS(ctx, filesystem.NewOSFileSystem())
	check.NoError(t, err)
	check.Equal(t, apphooks.HooksPathRestored, removal.HooksPathAction)
	check.Equal(t, foreign, getGlobalHooksPath(ctx))
}

func TestRemoveGlobalHooks_KeepsDispatcherForForeignPlugins(t *testing.T) {
	isolateGlobalGitConfig(t)

	ctx := t.Context()
	fs := filesystem.NewOSFileSystem()

	_, err := InstallGlobalHooksWithFS(ctx, fs, false, nil)
	check.NoError(t, err)

	foreignPlugin := filepath.Join(GlobalHookPluginPath("commit-msg"), "10-other-tool.sh")
	check.NoError(t, fs.WriteFile(foreignPlugin, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	result, err := RemoveGlobalHooksWithStatusFS(ctx, fs)
	check.NoError(t, err)
	check.Equal(t, []string{foreignPlugin}, result.Hooks[0].ForeignPlugins)
	check.Equal(t, apphooks.HooksPathUntouched, result.HooksPathAction)
	check.Equal(t, GlobalHooksPath(), getGlobalHooksPath(ctx))

	_, err = fs.Stat(GlobalHookDispatcherPath("commit-msg"))
	check.NoError(t, err, "dispatcher must stay while foreign plugins depend on it")
	_, err = fs.Stat(GlobalHookDispatcherPath("pre-push"))
	check.Error(t, err, "unused dispatcher is removed")
}

func TestGlobalHooksError_LocalizesMessage(t *testing.T) {
	t.Parallel()

	err := globalHooksError(errMockWrite, output.Msg("failed to write global hook"), nil)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.Contains(t, recoveryErr.Format(testLocalizer(t, "sv")), "det gick inte att skriva global hook")
}

func TestAppGlobalHookOperationError_ExitsSoftware(t *testing.T) {
	t.Parallel()

	err := appGlobalHookOperationError(apphooks.HookError{Err: apphooks.ErrWriteHook, Cause: errMockWrite}, nil)

	var exitErr interface{ ExitCode() int }
	check.ErrorAs(t, err, &exitErr)
	check.Equal(t, int(domain.ExitCodeSoftware), exitErr.ExitCode())
	check.ErrorIs(t, err, apphooks.ErrWriteHook)
	check.ErrorIs(t, err, errMockWrite)
}

func TestGlobalGitConfig_SettersPropagateCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	check.ErrorIs(t, globalGitConfig{}.SetHooksPath(ctx, "/tmp/hooks"), context.Canceled)
	check.ErrorIs(t, globalGitConfig{}.UnsetHooksPath(ctx), context.Canceled)
}

// isolateGlobalGitConfig points XDG_CONFIG_HOME and the global git config at a
// temporary directory, so install/remove never touch the developer's own
// ~/.gitconfig core.hooksPath.
func isolateGlobalGitConfig(t *testing.T) {
	t.Helper()

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmpDir, "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}
