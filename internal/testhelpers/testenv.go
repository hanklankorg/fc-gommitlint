// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers

import (
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestEnv provides an isolated test environment with filesystem and environment variables.
// By default, uses in-memory filesystem for speed and isolation.
// Use NewRealFSTestEnv() when testing symlinks, permissions, or other OS-specific features.
type TestEnv struct {
	t        *testing.T
	FS       *FileSystem
	TempDir  string
	RepoPath string
	// SigningFS provides the filesystem.FileSystem interface for production code
	SigningFS *SigningFileSystemAdapter
}

// setIsolatedHome points HOME and every per-user location gommitlint and its
// tools read at the test's own directory. Redirecting HOME alone is not enough:
// configuration discovery prefers XDG_CONFIG_HOME, and git and gpg read
// GIT_CONFIG_GLOBAL and GNUPGHOME, so a developer's real settings would
// otherwise reach the test.
func setIsolatedHome(t *testing.T, home string) {
	t.Helper()

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GNUPGHOME", filepath.Join(home, ".gnupg"))
}

// NewTestEnv creates a test environment with in-memory filesystem.
// Use this by default for all tests unless you need real filesystem features.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Set common environment variables for isolation
	setIsolatedHome(t, t.TempDir())
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GOMMITLINT_TEST", "1")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1") // Ignore system git config
	t.Setenv("GIT_AUTHOR_NAME", "Test Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// Create memory filesystem
	fs := NewMemoryFileSystem(t)
	repoPath := fs.Path("repo")
	fs.CreateDir(repoPath)

	// Create git directory structure
	gitDir := filepath.Join(repoPath, ".git")
	fs.CreateDir(gitDir)
	fs.CreateDir(filepath.Join(gitDir, "hooks"))
	fs.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"))

	return &TestEnv{
		t:         t,
		FS:        fs,
		TempDir:   fs.Path(""),
		RepoPath:  repoPath,
		SigningFS: NewSigningFileSystemAdapter(fs),
	}
}

// NewRealFSTestEnv creates a test environment with real filesystem.
// Use this ONLY when testing features that require real OS filesystem:
// - Symlink detection and handling
// - File permissions and chmod operations
//
// SigningFS wraps afero in both environments, so its CreateTemp and Open fail
// closed here as well; code that needs an *os.File takes
// filesystem.NewOSFileSystem, not this helper.
func NewRealFSTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// Set common environment variables for isolation
	tempDir := t.TempDir()
	setIsolatedHome(t, tempDir)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("GOMMITLINT_TEST", "1")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	// Create real filesystem helper
	fs := NewFileSystem(t)
	repoPath := filepath.Join(tempDir, "repo")
	fs.CreateDir(repoPath)

	// Create git directory structure
	gitDir := filepath.Join(repoPath, ".git")
	fs.CreateDir(gitDir)
	fs.CreateDir(filepath.Join(gitDir, "hooks"))
	fs.WriteFile(filepath.Join(gitDir, "config"), []byte("[core]\n\trepositoryformatversion = 0\n"))

	return &TestEnv{
		t:         t,
		FS:        fs,
		TempDir:   tempDir,
		RepoPath:  repoPath,
		SigningFS: &SigningFileSystemAdapter{fs: fs.GetAferoFS()},
	}
}

// Path returns the full path for a relative path in the test environment.
func (env *TestEnv) Path(relativePath string) string {
	return filepath.Join(env.RepoPath, relativePath)
}

// GitDir returns the .git directory path.
func (env *TestEnv) GitDir() string {
	return filepath.Join(env.RepoPath, ".git")
}

// HooksDir returns the git hooks directory path.
func (env *TestEnv) HooksDir() string {
	return filepath.Join(env.GitDir(), "hooks")
}

// HookPath returns the path for a specific git hook.
func (env *TestEnv) HookPath(hookType string) string {
	return filepath.Join(env.HooksDir(), hookType)
}

// WriteHook writes a git hook file with executable permissions.
func (env *TestEnv) WriteHook(hookType string, content string) {
	env.t.Helper()
	hookPath := env.HookPath(hookType)
	env.FS.WriteFile(hookPath, []byte(content))
	// Note: chmod not needed for memory FS, but doesn't hurt
	_ = env.FS.GetAferoFS().Chmod(hookPath, 0755)
}

// AssertFileContains checks that a file contains the expected content.
func (env *TestEnv) AssertFileContains(path string, expected string) {
	env.t.Helper()
	content := env.FS.ReadFile(path)
	check.Contains(env.t, content, expected)
}

// AssertFileExists checks that a file exists.
func (env *TestEnv) AssertFileExists(path string) {
	env.t.Helper()
	check.True(env.t, env.FS.FileExists(path), "File should exist: %s", path)
}

// AssertNoFile checks that a file does not exist.
func (env *TestEnv) AssertNoFile(path string) {
	env.t.Helper()
	check.False(env.t, env.FS.FileExists(path), "File should not exist: %s", path)
}
