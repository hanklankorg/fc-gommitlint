// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/adapters/gitcli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
)

const maxHookFileSize = fileops.MaxCheckedFileSize

var (
	errHookFileNotRegular        = errors.New("hook file is not a regular file")
	errResolveEffectiveHooksPath = errors.New("resolve effective git hooks path")
)

// HookInstallResult contains local hook installation details.
type HookInstallResult = apphooks.InstallResult

// HookRemovalResult contains local hook removal details.
type HookRemovalResult = apphooks.RemovalResult

// HookStatus represents the status of a Git hook.
type HookStatus = apphooks.HookStatus

// HookFileStatus represents the status of one local or global hook file.
type HookFileStatus = apphooks.HookFileStatus

// RepositoryHookStatus represents hook status for a repository.
type RepositoryHookStatus = apphooks.RepositoryStatus

type hookStore struct {
	fs      filesystem.FileSystem
	fileOps fileops.FileOperations
}

func newLocalHookService(ctx context.Context, fs filesystem.FileSystem) apphooks.Service {
	return newLocalHookServiceWithRedirectCheck(ctx, fs, hooksPathRedirect)
}

func newLocalHookServiceWithRedirectCheck(
	ctx context.Context,
	fs filesystem.FileSystem,
	redirectCheck func(context.Context, string) (string, bool, error),
) apphooks.Service {
	fileOps := fileops.NewFileOperations(fs)
	gitOps := fileops.NewGitOperations(fileOps)
	pathValidator := cliAdapter.NewPathValidator(fs)
	validateInstallTarget := func(repoPath string) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if redirectCheck == nil {
			return nil
		}

		redirect, redirected, err := redirectCheck(ctx, repoPath)
		if err != nil {
			return err
		}

		if redirected {
			return apphooks.NewHooksPathRedirectError("", redirect)
		}

		return nil
	}

	return apphooks.Service{
		Global: apphooks.GlobalDependencies{
			ConfiguredHooksPath: readGlobalHooksPath,
		},
		Local: apphooks.LocalDependencies{
			EnsureHooksDirectory: func(repoPath string) error {
				if err := ctx.Err(); err != nil {
					return err
				}

				return pathValidator.EnsureHooksDirectory(repoPath)
			},
			FindHookPath: func(repoPath string, hookType string) (string, error) {
				if err := ctx.Err(); err != nil {
					return "", err
				}

				return pathValidator.ValidateHookPath(repoPath, hookType)
			},
			ResolveHooksPath: redirectCheck,
			ResolveStatusPath: func(repoPath string, hookType string) (apphooks.RepositoryHookPath, error) {
				return resolveHookStatusPath(gitOps, repoPath, hookType)
			},
			ValidateInstallTarget: validateInstallTarget,
			ValidatePathInRepo:    pathValidator.ValidateHookLocation,
			ValidateRepository: func(repoPath string) (string, error) {
				if err := ctx.Err(); err != nil {
					return "", err
				}

				return validateHookRepositoryPath(gitOps, repoPath)
			},
		},
		Store: hookStore{
			fs:      fs,
			fileOps: fileOps,
		},
	}
}

func validateHookRepositoryPath(gitOps *fileops.GitOperations, repoPath string) (string, error) {
	if git.IsVirtualPath(repoPath) {
		return "", fmt.Errorf("%w: virtual repositories do not support filesystem hooks", cliAdapter.ErrInvalidRepoPath)
	}

	validatedPath, err := cliAdapter.ValidateRepoPath(repoPath)
	if err != nil {
		if _, validationErr := gitOps.ValidateGitRepository(repoPath); validationErr != nil {
			return "", validationErr
		}

		return "", err
	}

	if validatedPath == "" {
		candidate := repoPath
		if strings.TrimSpace(candidate) == "" {
			candidate, err = os.Getwd()
			if err != nil {
				return "", fmt.Errorf("resolve current directory: %w", err)
			}
		}

		return gitOps.ValidateGitRepository(candidate)
	}

	return gitOps.ValidateGitRepository(validatedPath)
}

func resolveHookStatusPath(gitOps *fileops.GitOperations, repoPath string, hookType string) (apphooks.RepositoryHookPath, error) {
	hookPath, err := gitOps.GetGitHookPath(repoPath, hookType)
	if err != nil {
		return apphooks.RepositoryHookPath{}, err
	}

	hooksDir := filepath.Dir(hookPath)

	return apphooks.RepositoryHookPath{
		GitDir:    filepath.Dir(hooksDir),
		HooksDir:  hooksDir,
		LocalPath: hookPath,
	}, nil
}
func (s hookStore) ReadDir(path string) ([]string, error) {
	entries, err := s.fs.ReadDir(path)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names, nil
}

func (s hookStore) ReadFile(path string) ([]byte, error) {
	info, err := s.fs.Lstat(path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", errHookFileNotRegular, path)
	}

	return s.fs.ReadFileWithLimit(path, maxHookFileSize)
}

func (s hookStore) IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}

func (s hookStore) MkdirAll(path string, perm os.FileMode) error {
	return s.fs.MkdirAll(path, perm)
}

func (s hookStore) Remove(path string) error {
	return s.fs.Remove(path)
}

func (s hookStore) RemoveWithCheck(path string, check apphooks.CheckFunc) error {
	if check == nil {
		return s.fileOps.SafeRemoveFileWithCheck(path, nil)
	}

	return s.fileOps.SafeRemoveFileWithCheck(path, func(existingContent []byte) error {
		return check(existingContent)
	})
}

func (s hookStore) Stat(path string) (apphooks.FileInfo, error) {
	return s.fileOps.GetFileInfo(path)
}

func (s hookStore) WriteFileWithCheck(path string, data []byte, perm os.FileMode, check apphooks.CheckFunc) error {
	if check == nil {
		return s.fileOps.SafeWriteFileWithCheck(path, data, perm, nil)
	}

	return s.fileOps.SafeWriteFileWithCheck(path, data, perm, func(existingContent []byte) error {
		return check(existingContent)
	})
}

func appHookPath(err error) string {
	if hookErr, ok := errors.AsType[apphooks.HookError](err); ok {
		return hookErr.Path
	}

	return ""
}

func appHookCause(err error) error {
	var hookErr apphooks.HookError
	if errors.As(err, &hookErr) && hookErr.Cause != nil {
		return hookErr.Cause
	}

	return err
}

func appHookInstallError(err error, hookType string, localizer output.Localizer) error {
	path := appHookPath(err)

	switch {
	case errors.Is(err, apphooks.ErrInvalidRepository):
		return invalidRepositoryError(appHookCause(err), localizer)
	case errors.Is(err, apphooks.ErrPathOutsideRepository):
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			appHookCause(err),
			output.Msg("security check failed"),
			loc.Get("Ensure the repository path is within your workspace"),
			helpSuggestion(loc, "gommitlint hook install"),
		)
	case errors.Is(err, apphooks.ErrHooksPathRedirected):
		return hooksPathRedirectedError(path, localizer)
	case errors.Is(err, apphooks.ErrCustomHookExists):
		return customHookExistsError(path, "gommitlint hook install", localizer)
	case errors.Is(err, apphooks.ErrHookAlreadyExists):
		return hookAlreadyExistsError(path, localizer)
	case errors.Is(err, apphooks.ErrWriteHook):
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			commandOperationFailure(err),
			output.Msg("could not write hook file"),
			loc.Get("Check hook file permissions"),
			loc.Get("Use --force to overwrite existing hooks"),
		)
	case errors.Is(err, apphooks.ErrStatHook):
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			commandOperationFailure(err),
			output.Msg("could not stat hook file"),
			loc.Get("Check hook file permissions"),
			helpSuggestion(loc, "gommitlint hook install"),
		)
	default:
		return commandOperationFailure(fmt.Errorf("%s hook installation failed: %w", hookType, err))
	}
}

func appHookRemovalError(err error, hookType string, localizer output.Localizer) error {
	path := appHookPath(err)

	switch {
	case errors.Is(err, apphooks.ErrInvalidRepository):
		return invalidRepositoryError(appHookCause(err), localizer)
	case errors.Is(err, apphooks.ErrPathOutsideRepository):
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			appHookCause(err),
			output.Msg("security check failed"),
			loc.Get("Ensure the repository path is within your workspace"),
			helpSuggestion(loc, "gommitlint hook remove"),
		)
	case errors.Is(err, apphooks.ErrNotGommitlintHook):
		return notGommitlintHookError(hookType, path, localizer)
	case errors.Is(err, apphooks.ErrStatHook):
		return hookRemovalError(localizer, output.Msg("cannot stat hook file"), err)
	case errors.Is(err, apphooks.ErrHookNotFound):
		return hookRemovalError(localizer, output.Msg("cannot read hook file"), err)
	case errors.Is(err, apphooks.ErrRemoveHook):
		return hookRemovalError(localizer, output.Msg("cannot remove hook file"), err)
	default:
		return commandOperationFailure(err)
	}
}

func appHookStatusError(err error, localizer output.Localizer) error {
	if errors.Is(err, apphooks.ErrInvalidRepository) {
		return invalidRepositoryError(appHookCause(err), localizer)
	}

	return commandOperationFailure(err)
}

func defaultLocalHookService(ctx context.Context) apphooks.Service {
	return newLocalHookService(ctx, filesystem.NewOSFileSystem())
}

// hooksPathRedirect asks git where this repository's hooks actually run from.
// It returns (dir, true) when core.hooksPath -- local, global or system --
// sends them somewhere other than the default hooks directory, and
// ("", false) when hooks run from the default location.
func hooksPathRedirect(ctx context.Context, repoPath string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--path-format=absolute", "--git-path", "hooks", "--git-common-dir") //nolint:gosec // fixed argv; repoPath is the validated repository
	cmd.Env = gitcli.Environment()

	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", false, ctxErr
		}

		return "", false, fmt.Errorf("resolve effective git hooks path: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		return "", false, fmt.Errorf("%w: expected 2 lines, got %d", errResolveEffectiveHooksPath, len(lines))
	}

	hooksDir := filepath.Clean(strings.TrimSpace(lines[0]))
	defaultDir := filepath.Join(filepath.Clean(strings.TrimSpace(lines[1])), "hooks")

	if hooksDir == defaultDir {
		return "", false, nil
	}

	return hooksDir, true, nil
}
