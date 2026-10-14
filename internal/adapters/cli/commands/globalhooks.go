// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/scripts/hooks"
)

var errUnknownGlobalHookType = errors.New("unknown global hook type")

// GlobalHookRemovalStatus represents removal status for a hook type.
type GlobalHookRemovalStatus = apphooks.GlobalHookRemovalStatus

// GlobalInstallResult describes a completed global hook installation.
type GlobalInstallResult = apphooks.GlobalInstallResult

// GlobalRemovalResult describes a completed global hook removal.
type GlobalRemovalResult = apphooks.GlobalRemovalResult

// GlobalHooksPath returns the path to the global git hooks directory.
// Uses XDG_CONFIG_HOME if set, otherwise ~/.config.
func GlobalHooksPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(configHome) {
		home, err := os.UserHomeDir()
		if err != nil || !filepath.IsAbs(home) {
			return ""
		}

		configHome = filepath.Join(home, ".config")
	}

	return filepath.Join(configHome, "git", "git-hooks")
}

// GlobalHookPluginPath returns the path to a specific hook's plugin directory.
func GlobalHookPluginPath(hookType string) string {
	return apphooks.GlobalHookPluginPath(GlobalHooksPath(), hookType)
}

// GlobalHookDispatcherPath returns the path to a specific hook's dispatcher script.
func GlobalHookDispatcherPath(hookType string) string {
	return apphooks.GlobalHookDispatcherPath(GlobalHooksPath(), hookType)
}

// createDispatcherScript returns the dispatcher script content for a given hook type.
func createDispatcherScript(hookType string) string {
	return strings.ReplaceAll(hooks.DispatcherTemplate, "HOOK_TYPE", hookType)
}

// InstallGlobalHooks installs hooks using the global dispatcher pattern.
func InstallGlobalHooks(ctx context.Context, force bool, localizer output.Localizer) (GlobalInstallResult, error) {
	return InstallGlobalHooksWithFS(ctx, filesystem.NewOSFileSystem(), force, localizer)
}

// InstallGlobalHooksWithFS installs global hooks with filesystem injection for testing.
func InstallGlobalHooksWithFS(ctx context.Context, fs filesystem.FileSystem, force bool, localizer output.Localizer) (GlobalInstallResult, error) {
	result, err := newGlobalHookService(ctx, fs).InstallGlobal(ctx, apphooks.GlobalInstallRequest{Force: force})
	if err != nil {
		return GlobalInstallResult{}, appGlobalHookError(err, localizer)
	}

	return result, nil
}

// RemoveGlobalHooksWithStatusFS removes global hooks and returns per-hook
// status plus the core.hooksPath outcome.
func RemoveGlobalHooksWithStatusFS(ctx context.Context, fs filesystem.FileSystem) (GlobalRemovalResult, error) {
	result, err := newGlobalHookService(ctx, fs).RemoveGlobalWithStatus(ctx)
	if err != nil {
		return result, appGlobalHookError(err, nil)
	}

	return result, nil
}

// createCommitMsgPluginScript returns the embedded commit-msg plugin script.
func createCommitMsgPluginScript() string {
	return hooks.WithInstalledBinary(hooks.CommitMsgPlugin, installedBinaryPath())
}

// createPrePushPluginScript returns the embedded pre-push plugin script.
func createPrePushPluginScript() string {
	return hooks.WithInstalledBinary(hooks.PrePushPlugin, installedBinaryPath())
}

func newGlobalHookService(ctx context.Context, fs filesystem.FileSystem) apphooks.Service {
	service := newLocalHookService(ctx, fs)
	service.Global.DispatcherScript = createDispatcherScript
	service.Global.PluginScript = globalPluginScript
	service.Global.ResolveGlobalHooksPath = func(context.Context) string { return GlobalHooksPath() }
	service.Global.SetGlobalHooksPath = setGlobalHooksPath
	service.Global.UnsetGlobalHooksPath = unsetGlobalHooksPath

	return service
}

func globalPluginScript(hookType string) (string, error) {
	switch hookType {
	case hookTypeCommitMsg:
		return createCommitMsgPluginScript(), nil
	case hookTypePrePush:
		return createPrePushPluginScript(), nil
	default:
		return "", fmt.Errorf("%w: %s", errUnknownGlobalHookType, hookType)
	}
}

func appGlobalHookError(err error, localizer output.Localizer) error {
	path := appHookPath(err)

	switch {
	case errors.Is(err, apphooks.ErrGlobalHooksPathRequired), errors.Is(err, apphooks.ErrGlobalHooksPathUnavailable):
		return globalHooksError(err, output.Msg("failed to resolve global hooks path"), localizer)
	case errors.Is(err, apphooks.ErrCustomHookExists):
		return customHookExistsError(path, "gommitlint hook install --global", localizer)
	case errors.Is(err, apphooks.ErrHookAlreadyExists):
		return globalHookAlreadyExistsError(path, localizer)
	case errors.Is(err, apphooks.ErrHooksPathInUse):
		return hooksPathInUseError(path, localizer)
	default:
		return appGlobalHookOperationError(err, localizer)
	}
}

func appGlobalHookOperationError(err error, localizer output.Localizer) error {
	switch {
	case errors.Is(err, apphooks.ErrCreateHookDirectory):
		return globalHooksError(err, output.Msg("failed to create global hooks directory"), localizer)
	case errors.Is(err, apphooks.ErrWriteHook):
		return globalHooksError(err, output.Msg("failed to write global hook"), localizer)
	case errors.Is(err, apphooks.ErrConfigureGlobalHooks), errors.Is(err, apphooks.ErrGlobalGitConfigSetterRequired):
		return globalHooksError(err, output.Msg("failed to configure git"), localizer)
	case errors.Is(err, apphooks.ErrStatHook):
		return globalHooksError(err, output.Msg("failed to check global hooks directory"), localizer)
	case errors.Is(err, apphooks.ErrHookNotFound):
		return globalHooksError(err, output.Msg("failed to read global hook"), localizer)
	case errors.Is(err, apphooks.ErrRemoveHook):
		return globalHooksError(err, output.Msg("failed to remove global hook"), localizer)
	default:
		return globalHooksError(err, output.Msg("global hook operation failed"), localizer)
	}
}

func globalHooksError(err error, message output.LocaleMsg, localizer output.Localizer) error {
	if _, ok := errors.AsType[RecoveryError](err); ok {
		return err
	}

	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		commandOperationFailure(err),
		message,
		helpSuggestion(localizer, "gommitlint hook install --global"),
		localizer.Get("Check permissions and global git config"),
	)
}
