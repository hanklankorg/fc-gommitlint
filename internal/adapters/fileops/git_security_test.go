// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestGitOperations_HandleGitFile_ResolvesGitdirAndRejectsMalformedFiles(t *testing.T) {
	testCases := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		expectError bool
		errorCheck  func(t *testing.T, err error)
		pathCheck   func(t *testing.T, gitDir string)
	}{
		{
			name: "valid absolute gitfile",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				gitFilePath := filepath.Join(tmpDir, ".git")
				gitDirPath := filepath.Join(tmpDir, "actual-git-dir")

				// Create the actual git directory with config file
				err := os.MkdirAll(gitDirPath, 0o755)
				check.NoError(t, err)

				// Create a mock git config file
				configPath := filepath.Join(gitDirPath, "config")
				err = os.WriteFile(configPath, []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
				check.NoError(t, err)

				// Create gitfile content with absolute path
				gitfileContent := "gitdir: " + gitDirPath
				err = os.WriteFile(gitFilePath, []byte(gitfileContent), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: false,
			pathCheck: func(t *testing.T, gitDir string) {
				t.Helper()
				check.Contains(t, gitDir, "actual-git-dir")
				check.True(t, filepath.IsAbs(gitDir))
			},
		},
		{
			name: "valid relative gitfile",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				repoDir := filepath.Join(tmpDir, "repo")
				err := os.MkdirAll(repoDir, 0o755)
				check.NoError(t, err)

				gitFilePath := filepath.Join(repoDir, ".git")

				// Create relative git directory with config
				gitDirPath := filepath.Join(tmpDir, "git-modules", "repo")
				err = os.MkdirAll(gitDirPath, 0o755)
				check.NoError(t, err)

				// Create a mock git config file
				configPath := filepath.Join(gitDirPath, "config")
				err = os.WriteFile(configPath, []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
				check.NoError(t, err)

				// Create gitfile content with relative path
				gitfileContent := "gitdir: ../git-modules/repo"
				err = os.WriteFile(gitFilePath, []byte(gitfileContent), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: false,
			pathCheck: func(t *testing.T, gitDir string) {
				t.Helper()
				check.Contains(t, gitDir, "git-modules")
				check.True(t, filepath.IsAbs(gitDir))
			},
		},
		{
			name: "gitfile with whitespace",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				gitFilePath := filepath.Join(tmpDir, ".git")
				gitDirPath := filepath.Join(tmpDir, "git-dir")

				err := os.MkdirAll(gitDirPath, 0o755)
				check.NoError(t, err)

				// Create a mock git config file
				configPath := filepath.Join(gitDirPath, "config")
				err = os.WriteFile(configPath, []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
				check.NoError(t, err)

				// Create gitfile content with extra whitespace
				gitfileContent := "  gitdir: " + gitDirPath + "  \n"
				err = os.WriteFile(gitFilePath, []byte(gitfileContent), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: false,
			pathCheck: func(t *testing.T, gitDir string) {
				t.Helper()
				check.Contains(t, gitDir, "git-dir")
			},
		},
		{
			name: "invalid gitfile format - missing gitdir prefix",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				gitFilePath := filepath.Join(tmpDir, ".git")

				// Invalid content without gitdir: prefix
				gitfileContent := "/some/path"
				err := os.WriteFile(gitFilePath, []byte(gitfileContent), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				t.Helper()
				check.ErrorIs(t, err, ErrInvalidGitFileFormat)
			},
		},
		{
			name: "invalid gitfile format - empty content",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				gitFilePath := filepath.Join(tmpDir, ".git")

				// Empty content
				err := os.WriteFile(gitFilePath, []byte(""), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				t.Helper()
				check.ErrorIs(t, err, ErrInvalidGitFileFormat)
			},
		},
		{
			name: "non-existent gitfile",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()

				return filepath.Join(tmpDir, "nonexistent.git")
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				t.Helper()
				check.ErrorIs(t, err, os.ErrNotExist)
			},
		},
		{
			name: "gitfile with only whitespace after gitdir:",
			setupFunc: func(t *testing.T) string {
				t.Helper()
				tmpDir := t.TempDir()
				gitFilePath := filepath.Join(tmpDir, ".git")

				// Gitdir prefix but no actual path - this should fail validation
				gitfileContent := "gitdir:   \n"
				err := os.WriteFile(gitFilePath, []byte(gitfileContent), 0o644)
				check.NoError(t, err)

				return gitFilePath
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				t.Helper()
				check.ErrorIs(t, err, ErrInvalidGitFileFormat)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ops := NewGitOperations(NewFileOperations(filesystem.NewOSFileSystem()))
			gitFilePath := testCase.setupFunc(t)

			gitDir, err := ops.handleGitFile(gitFilePath)

			if testCase.expectError {
				check.Error(t, err)
				check.Empty(t, gitDir)

				if testCase.errorCheck != nil {
					testCase.errorCheck(t, err)
				}
			} else {
				check.NoError(t, err)

				if testCase.pathCheck != nil {
					testCase.pathCheck(t, gitDir)
				}
			}
		})
	}
}

// TestGitFilePath_ResolvesRelativeGitdirsToCleanAbsolutePaths pins that
// relative gitdir values resolve against the gitfile's directory and come
// back as clean absolute paths. It does not test rejection: handleGitFile
// accepts any resolvable directory that holds a git config, and refusing
// paths is the path validator's job.
// Every case keeps its gitdir inside the test's own temporary tree.
func TestGitFilePath_ResolvesRelativeGitdirsToCleanAbsolutePaths(t *testing.T) {
	testCases := []struct {
		name string
		// gitfileDir is where the .git file lives, relative to the temporary
		// root; gitdir is the value written into it.
		gitfileDir string
		gitdir     string
		// wantGitDir is the resolved directory, relative to the same root.
		wantGitDir string
	}{
		{
			name:       "single-level relative path",
			gitfileDir: "repo",
			gitdir:     "../git-modules/repo",
			wantGitDir: "git-modules/repo",
		},
		{
			name:       "multi-level relative path",
			gitfileDir: "workspace/projects/repo",
			gitdir:     "../../../git-storage/projects/repo",
			wantGitDir: "git-storage/projects/repo",
		},
		{
			name:       "absolute path",
			gitfileDir: "repo",
			gitdir:     "", // set below to an absolute path inside the root
			wantGitDir: "absolute-git-dir",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()

			gitfileDir := filepath.Join(root, testCase.gitfileDir)
			err := os.MkdirAll(gitfileDir, 0o755)
			check.NoError(t, err)

			wantGitDir := filepath.Join(root, testCase.wantGitDir)
			err = os.MkdirAll(wantGitDir, 0o755)
			check.NoError(t, err)

			err = os.WriteFile(filepath.Join(wantGitDir, "config"),
				[]byte("[core]\n\trepositoryformatversion = 0\n"), 0o644)
			check.NoError(t, err)

			gitdir := testCase.gitdir
			if gitdir == "" {
				gitdir = wantGitDir
			}

			gitFilePath := filepath.Join(gitfileDir, ".git")
			err = os.WriteFile(gitFilePath, []byte("gitdir: "+gitdir), 0o644)
			check.NoError(t, err)

			ops := NewGitOperations(NewFileOperations(filesystem.NewOSFileSystem()))

			gitDir, err := ops.handleGitFile(gitFilePath)
			check.NoError(t, err)

			// The exact directory, not merely "absolute and free of dot-dot":
			// a resolver that walked to the wrong parent would still satisfy
			// those weaker claims.
			check.Equal(t, wantGitDir, gitDir)
		})
	}
}

// A linked worktree's .git file points at <main>/.git/worktrees/<name>, whose
// "commondir" file names the directory that really holds config and hooks.
// Hook paths must resolve to that common directory.
func TestGitFile_LinkedWorktreeResolvesToCommonDir(t *testing.T) {
	t.Parallel()

	mainRepo := t.TempDir()
	commonDir := filepath.Join(mainRepo, ".git")
	worktreeGitDir := filepath.Join(commonDir, "worktrees", "feature")
	check.NoError(t, os.MkdirAll(filepath.Join(commonDir, "hooks"), 0o755))
	check.NoError(t, os.MkdirAll(worktreeGitDir, 0o755))
	check.NoError(t, os.WriteFile(filepath.Join(commonDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"), 0o600))
	check.NoError(t, os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0o600))

	worktree := t.TempDir()
	check.NoError(t, os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+worktreeGitDir+"\n"), 0o600))

	ops := NewGitOperations(NewFileOperations(filesystem.NewOSFileSystem()))

	gitDir, err := ops.ResolveGitDir(worktree)
	check.NoError(t, err)
	check.Equal(t, commonDir, gitDir)

	hookPath, err := ops.GetGitHookPath(worktree, "commit-msg")
	check.NoError(t, err)
	check.Equal(t, filepath.Join(commonDir, "hooks", "commit-msg"), hookPath)
}
