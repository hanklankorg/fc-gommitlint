// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

// Test constants for string literals.
const (
	gitConfigContentMigrated = "[core]\n\trepositoryformatversion = 0\n"
)

func TestValidateHookPath_RejectsInvalidAndAcceptsValidPaths(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	// Create a .git directory to make it a valid repository
	gitDir := filepath.Join(env.RepoPath, ".git")
	env.FS.CreateDir(gitDir)

	// Create a minimal git config file
	configFile := filepath.Join(gitDir, "config")
	env.FS.WriteFile(configFile, []byte(gitConfigContentMigrated))

	// Create a PathValidator with the test filesystem
	validator := NewPathValidator(env.SigningFS)

	tests := []struct {
		name        string
		repoPath    string
		hookType    string
		expectError bool
		description string
	}{
		{
			name:        "valid commit-msg hook",
			repoPath:    env.RepoPath,
			hookType:    "commit-msg",
			expectError: false,
			description: "should validate commit-msg hook path",
		},
		{
			name:        "invalid hook type",
			repoPath:    env.RepoPath,
			hookType:    "invalid-hook",
			expectError: true,
			description: "should reject invalid hook type",
		},
		{
			name:        "empty hook type",
			repoPath:    env.RepoPath,
			hookType:    "",
			expectError: true,
			description: "should reject empty hook type",
		},
		{
			name:        "non-existent repository",
			repoPath:    filepath.Join(env.RepoPath, "nonexistent"),
			hookType:    "commit-msg",
			expectError: true,
			description: "should reject non-existent repository",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path, err := validator.ValidateHookPath(testCase.repoPath, testCase.hookType)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
				check.Empty(t, path, "path should be empty on error")
			} else {
				check.NoError(t, err, testCase.description)
				check.NotEmpty(t, path, "path should not be empty on success")
				check.Contains(t, path, testCase.hookType, "path should contain hook type")
			}
		})
	}
}

func TestValidatePathWithinRepo_RejectsEscapes(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	// Create a .git directory
	gitDir := filepath.Join(env.RepoPath, ".git")
	env.FS.CreateDir(gitDir)

	// Create a minimal git config
	configFile := filepath.Join(gitDir, "config")
	env.FS.WriteFile(configFile, []byte(gitConfigContentMigrated))

	// Create a PathValidator with the test filesystem
	validator := NewPathValidator(env.SigningFS)

	tests := []struct {
		name        string
		setupPath   func() string
		expectError bool
		description string
	}{
		{
			name: "path within repository",
			setupPath: func() string {
				subDir := filepath.Join(env.RepoPath, "subdir")
				env.FS.CreateDir(subDir)

				return subDir
			},
			expectError: false,
			description: "should accept path within repository",
		},
		{
			name: "path outside repository",
			setupPath: func() string {
				// Return a path outside the repo
				return filepath.Join(filepath.Dir(env.RepoPath), "outside")
			},
			expectError: true,
			description: "should reject path outside repository",
		},
		{
			name: "repository root itself",
			setupPath: func() string {
				return env.RepoPath
			},
			expectError: false,
			description: "should accept repository root",
		},
		{
			name: "dot-prefixed path within repository",
			setupPath: func() string {
				return filepath.Join(env.RepoPath, ".gommitlint.yaml")
			},
			expectError: false,
			description: "should accept dot-prefixed files",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path := testCase.setupPath()
			err := validator.ValidatePathWithinRepo(env.RepoPath, path)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestValidateHookLocation_RejectsSymlinkedParentOutsideRepository(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	gitDir := filepath.Join(repoPath, ".git")
	outsideHooks := filepath.Join(root, "outside-hooks")

	check.NoError(t, os.MkdirAll(gitDir, 0o750))
	check.NoError(t, os.MkdirAll(outsideHooks, 0o750))
	check.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(gitConfigContentMigrated), 0o600))
	check.NoError(t, os.Symlink(outsideHooks, filepath.Join(gitDir, "hooks")))

	validator := NewPathValidator(filesystem.NewOSFileSystem())
	check.ErrorIs(t, validator.EnsureHooksDirectory(repoPath), ErrInvalidHooksPath)

	err := validator.ValidateHookLocation(repoPath, filepath.Join(gitDir, "hooks", "commit-msg"))

	check.ErrorIs(t, err, ErrPathOutsideRepo)
}

func TestEnsureHooksDirectory_CreatesDirectoryWhenMissing(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	// Create a PathValidator with the test filesystem
	validator := NewPathValidator(env.SigningFS)

	tests := []struct {
		name        string
		setupRepo   func() string
		expectError bool
		description string
	}{
		{
			name: "valid repository",
			setupRepo: func() string {
				// Create a valid git repository structure
				gitDir := filepath.Join(env.RepoPath, ".git")
				env.FS.CreateDir(gitDir)

				configFile := filepath.Join(gitDir, "config")
				env.FS.WriteFile(configFile, []byte(gitConfigContentMigrated))

				return env.RepoPath
			},
			expectError: false,
			description: "should create hooks directory in valid repository",
		},
		{
			name: "non-git directory",
			setupRepo: func() string {
				// Create a directory without .git
				nonGitDir := filepath.Join(env.RepoPath, "not-a-repo")
				env.FS.CreateDir(nonGitDir)

				return nonGitDir
			},
			expectError: true,
			description: "should fail for non-git directory",
		},
		{
			name: "hooks directory already exists",
			setupRepo: func() string {
				// Create git repo with existing hooks directory
				gitDir := filepath.Join(env.RepoPath, ".git")
				env.FS.CreateDir(gitDir)

				hooksDir := filepath.Join(gitDir, "hooks")
				env.FS.CreateDir(hooksDir)

				configFile := filepath.Join(gitDir, "config")
				env.FS.WriteFile(configFile, []byte(gitConfigContentMigrated))

				return env.RepoPath
			},
			expectError: false,
			description: "should handle existing hooks directory",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repoPath := testCase.setupRepo()
			err := validator.EnsureHooksDirectory(repoPath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)

				// Verify the hooks directory was created
				hooksPath := filepath.Join(repoPath, ".git", "hooks")
				check.True(t, env.FS.DirExists(hooksPath), "hooks directory should exist")
			}
		})
	}
}
