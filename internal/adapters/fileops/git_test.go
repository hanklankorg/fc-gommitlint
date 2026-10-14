// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops_test

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func TestGitOperations_ValidateGitRepository(t *testing.T) {
	t.Parallel()

	const testRepoPath = "/test/mock/repo"

	t.Run("validates valid Git repository", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		repoPath := testRepoPath
		gitDir := filepath.Join(repoPath, ".git")
		configPath := filepath.Join(gitDir, "config")

		// Set up mock repository
		mockFS.AddDir(repoPath, []os.DirEntry{})
		mockFS.AddDir(gitDir, []os.DirEntry{})
		mockFS.AddFile(configPath, []byte("git config content"))

		result, err := gitOps.ValidateGitRepository(repoPath)
		check.NoError(t, err)
		check.Equal(t, repoPath, result)
	})

	t.Run("rejects empty repository path", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		_, err := gitOps.ValidateGitRepository("")
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrRepositoryPathEmpty)
	})

	t.Run("rejects invalid path structure", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		_, err := gitOps.ValidateGitRepository("../../../etc")
		check.Error(t, err)
		check.ErrorIs(t, err, domain.ErrPathTraversal)
	})

	t.Run("rejects non-existent repository", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		mockFS.SetError("/nonexistent", os.ErrNotExist)

		_, err := gitOps.ValidateGitRepository("/nonexistent")
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrRepositoryPathNotExist)
	})

	t.Run("rejects path that is not a directory", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		filePath := "/test/mock/notadir"
		mockFS.AddFile(filePath, []byte("content"))

		_, err := gitOps.ValidateGitRepository(filePath)
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrRepositoryPathNotDir)
	})

	t.Run("rejects directory without .git", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		repoPath := "/test/mock/notgit"
		mockFS.AddDir(repoPath, []os.DirEntry{})
		mockFS.SetError(filepath.Join(repoPath, ".git"), os.ErrNotExist)

		_, err := gitOps.ValidateGitRepository(repoPath)
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrNotGitRepository)
	})

	// An in-memory repository is served by the virtual factory and refused for
	// hooks before this layer is reached; the filesystem layer must not vouch
	// for a path that names nothing on disk.
	t.Run("rejects a virtual mem:// path", func(t *testing.T) {
		t.Parallel()

		gitOps := fileops.NewGitOperations(fileops.NewFileOperations(filesystem.NewOSFileSystem()))

		_, err := gitOps.ValidateGitRepository("mem://repo")
		check.ErrorIs(t, err, fileops.ErrRepositoryPathNotExist)
	})
}

func TestGitOperations_GetGitHookPath(t *testing.T) {
	t.Parallel()

	const testRepoPath = "/test/mock/repo"

	t.Run("generates valid hook path", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		// Set up valid repository
		repoPath := testRepoPath
		gitDir := filepath.Join(repoPath, ".git")
		configPath := filepath.Join(gitDir, "config")

		mockFS.AddDir(repoPath, []os.DirEntry{})
		mockFS.AddDir(gitDir, []os.DirEntry{})
		mockFS.AddFile(configPath, []byte("git config"))

		hookPath, err := gitOps.GetGitHookPath(repoPath, "commit-msg")
		check.NoError(t, err)
		check.Equal(t, filepath.Join(gitDir, "hooks", "commit-msg"), hookPath)
	})

	t.Run("rejects invalid hook type", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		// Set up valid repository
		repoPath := testRepoPath
		gitDir := filepath.Join(repoPath, ".git")
		configPath := filepath.Join(gitDir, "config")

		mockFS.AddDir(repoPath, []os.DirEntry{})
		mockFS.AddDir(gitDir, []os.DirEntry{})
		mockFS.AddFile(configPath, []byte("git config"))

		_, err := gitOps.GetGitHookPath(repoPath, "malicious-hook")
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrInvalidHookType)
	})

	t.Run("rejects missing repository", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		// The mock answers Stat for any path unless told otherwise; a path
		// that does not exist must be reported as missing, not as a file.
		mockFS.SetError("/nonexistent", os.ErrNotExist)

		_, err := gitOps.GetGitHookPath("/nonexistent", "commit-msg")
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrRepositoryPathNotExist)
	})

	t.Run("rejects unsupported hook type", func(t *testing.T) {
		t.Parallel()

		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		gitOps := fileops.NewGitOperations(fileOps)

		// Set up valid repository
		repoPath := testRepoPath
		gitDir := filepath.Join(repoPath, ".git")
		configPath := filepath.Join(gitDir, "config")

		mockFS.AddDir(repoPath, []os.DirEntry{})
		mockFS.AddDir(gitDir, []os.DirEntry{})
		mockFS.AddFile(configPath, []byte("git config"))

		_, err := gitOps.GetGitHookPath(repoPath, "commit-msg; rm -rf /")
		check.Error(t, err)
		check.ErrorIs(t, err, fileops.ErrInvalidHookType)
	})
}

func TestGitOperations_RealGitRepository(t *testing.T) {
	t.Parallel()

	// Test with real filesystem - create a temporary git repository
	tmpDir := t.TempDir()

	// Create a minimal .git directory structure
	gitDir := filepath.Join(tmpDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	check.NoError(t, err)

	configPath := filepath.Join(gitDir, "config")
	err = os.WriteFile(configPath, []byte("[core]\n\trepositoryformatversion = 0\n"), 0o600)
	check.NoError(t, err)

	// Test with real filesystem operations
	realFS := filesystem.NewOSFileSystem()
	fileOps := fileops.NewFileOperations(realFS)
	gitOps := fileops.NewGitOperations(fileOps)

	t.Run("validates real git repository", func(t *testing.T) {
		t.Parallel()

		result, err := gitOps.ValidateGitRepository(tmpDir)
		check.NoError(t, err)
		check.Contains(t, result, tmpDir)
	})

	t.Run("generates real hook path", func(t *testing.T) {
		t.Parallel()

		hookPath, err := gitOps.GetGitHookPath(tmpDir, "pre-commit")
		check.NoError(t, err)
		check.Equal(t, filepath.Join(gitDir, "hooks", "pre-commit"), hookPath)
	})
}
