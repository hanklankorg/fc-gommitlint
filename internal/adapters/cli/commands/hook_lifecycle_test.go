// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

var errTestFilesystemFailure = errors.New("filesystem failure")

func TestValidateHookRepositoryPath_EmptyDiscoversCurrentRepository(t *testing.T) {
	env := testhelpers.NewRealFSTestEnv(t)
	t.Chdir(env.RepoPath)

	fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
	gitOps := fileops.NewGitOperations(fileOps)

	got, err := validateHookRepositoryPath(gitOps, "")
	check.NoError(t, err)
	check.Equal(t, env.RepoPath, got)
}

func TestValidateHookRepositoryPath_ExplicitNestedPathDiscoversRepository(t *testing.T) {
	env := testhelpers.NewRealFSTestEnv(t)
	nestedPath := filepath.Join(env.RepoPath, "nested", "directory")
	check.NoError(t, os.MkdirAll(nestedPath, 0o755))

	fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
	gitOps := fileops.NewGitOperations(fileOps)

	got, err := validateHookRepositoryPath(gitOps, nestedPath)
	check.NoError(t, err)
	check.Equal(t, env.RepoPath, got)
}

func TestValidateHookRepositoryPath_RejectsVirtualPath(t *testing.T) {
	t.Parallel()

	fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
	gitOps := fileops.NewGitOperations(fileOps)

	_, err := validateHookRepositoryPath(gitOps, "mem://external")
	check.Error(t, err)
}

func TestAppHookOperationalErrors_ExitSoftwareAndPreserveCause(t *testing.T) {
	t.Parallel()

	rootCause := errTestFilesystemFailure
	testCases := []struct {
		name     string
		sentinel error
		mapError func(error) error
	}{
		{
			name:     "install_write",
			sentinel: apphooks.ErrWriteHook,
			mapError: func(err error) error { return appHookInstallError(err, hookTypeCommitMsg, nil) },
		},
		{
			name:     "install_stat",
			sentinel: apphooks.ErrStatHook,
			mapError: func(err error) error { return appHookInstallError(err, hookTypeCommitMsg, nil) },
		},
		{
			name:     "removal_read",
			sentinel: apphooks.ErrHookNotFound,
			mapError: func(err error) error { return appHookRemovalError(err, hookTypeCommitMsg, nil) },
		},
		{
			name:     "removal_remove",
			sentinel: apphooks.ErrRemoveHook,
			mapError: func(err error) error { return appHookRemovalError(err, hookTypeCommitMsg, nil) },
		},
		{
			name:     "status",
			sentinel: apphooks.ErrStatHook,
			mapError: func(err error) error { return appHookStatusError(err, nil) },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mapped := testCase.mapError(apphooks.HookError{Err: testCase.sentinel, Cause: rootCause})

			var exitErr interface{ ExitCode() int }
			check.ErrorAs(t, mapped, &exitErr)
			check.Equal(t, int(domain.ExitCodeSoftware), exitErr.ExitCode())
			check.ErrorIs(t, mapped, testCase.sentinel)
			check.ErrorIs(t, mapped, rootCause)
		})
	}
}

func TestHookStore_ReadFile_RejectsOversizedHook(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "commit-msg")
	check.NoError(t, os.WriteFile(path, make([]byte, maxHookFileSize+1), 0o600))

	store := hookStore{fs: filesystem.NewOSFileSystem()}

	_, err := store.ReadFile(path)

	check.ErrorIs(t, err, filesystem.ErrFileTooLarge)
}

func TestHookStore_ReadFile_RejectsNonRegularHook(t *testing.T) {
	t.Parallel()

	store := hookStore{fs: filesystem.NewOSFileSystem()}

	_, err := store.ReadFile(t.TempDir())

	check.ErrorIs(t, err, errHookFileNotRegular)
}
