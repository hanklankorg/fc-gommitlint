// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// GitRepo represents a test git repository with filesystem abstraction.
type GitRepo struct {
	fs   *FileSystem
	path string
	env  []string
	t    *testing.T
}

// gitEnvironment returns the environment for the fixture's git calls: the
// process environment without any GIT_* variable, and with the user's git
// configuration replaced by an empty test-owned one.
//
// git exports GIT_DIR and GIT_WORK_TREE to its hooks, and a fixture that
// inherited them would run every command in that repository instead of its
// own. The fixture writes the settings it needs into the repository's config.
func gitEnvironment(t *testing.T) []string {
	t.Helper()

	home := t.TempDir()
	environment := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GIT_") })

	return append(environment,
		"HOME="+home,
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
	)
}

// NewGitRepo creates a new git repository for testing using memory filesystem where possible.
func NewGitRepo(t *testing.T, useRealFS bool) *GitRepo {
	t.Helper()

	var fs *FileSystem
	if useRealFS {
		// Integration tests need real filesystem for git operations
		fs = NewFileSystem(t)
	} else {
		// Unit tests can use memory filesystem for file operations
		fs = NewMemoryFileSystem(t)
	}

	repo := &GitRepo{
		fs:   fs,
		path: fs.Path(""),
		env:  gitEnvironment(t),
		t:    t,
	}

	if useRealFS {
		repo.Init()
	}

	return repo
}

// Path returns the repository path.
func (g *GitRepo) Path() string {
	return g.path
}

// FileSystem returns the underlying filesystem.
func (g *GitRepo) FileSystem() *FileSystem {
	return g.fs
}

// Init initializes a git repository.
func (g *GitRepo) Init() {
	g.t.Helper()

	output, err := g.command("init").CombinedOutput()
	check.NoError(g.t, err, "Failed to init git repo: %s", string(output))

	// Set up basic git config
	g.Config("user.email", "test@example.com")
	g.Config("user.name", "Test User")
	g.Config("commit.gpgsign", "false")
	// Isolate from the developer's global hooks (core.hooksPath). Point at the
	// repo-local hooks dir, which is empty in fresh test repos — otherwise
	// globally-installed hooks like gommitlint's own pre-commit hook run on
	// test commits and can fail tests for reasons unrelated to the code.
	g.Config("core.hooksPath", filepath.Join(g.path, ".git", "hooks"))
}

// Config sets a git configuration value.
func (g *GitRepo) Config(key, value string) {
	g.t.Helper()

	output, err := g.command("config", key, value).CombinedOutput()
	check.NoError(g.t, err, "Failed to set git config %s=%s: %s", key, value, string(output))
}

// Add stages files for commit.
func (g *GitRepo) Add(files ...string) {
	g.t.Helper()

	output, err := g.command(append([]string{"add"}, files...)...).CombinedOutput()
	check.NoError(g.t, err, "Failed to add files: %s", string(output))
}

// Commit creates a commit with the given message.
func (g *GitRepo) Commit(message string) string {
	g.t.Helper()

	output, err := g.command("commit", "-m", message).CombinedOutput()
	check.NoError(g.t, err, "Failed to commit: %s", string(output))

	return g.HeadSHA()
}

// HeadSHA returns the SHA of HEAD.
func (g *GitRepo) HeadSHA() string {
	g.t.Helper()

	output, err := g.command("rev-parse", "HEAD").CombinedOutput()
	check.NoError(g.t, err, "Failed to get HEAD SHA: %s", string(output))

	return string(output[:domain.ShortHashLength])
}

// WriteFile writes a file using the filesystem abstraction.
func (g *GitRepo) WriteFile(path string, content []byte) {
	g.t.Helper()
	g.fs.WriteFile(path, content)
}

// WriteConfig writes a configuration file.
func (g *GitRepo) WriteConfig(filename, content string) {
	g.t.Helper()
	g.fs.WriteFile(filename, []byte(content))
}

// WriteHook writes a git hook file.
func (g *GitRepo) WriteHook(hookName, content string) {
	g.t.Helper()

	hookPath := filepath.Join(".git", "hooks", hookName)
	g.fs.WriteExecutable(hookPath, []byte(content))
}

// CreateAndCommit creates a file and commits it.
func (g *GitRepo) CreateAndCommit(filename, content, message string) string {
	g.t.Helper()

	g.WriteFile(filename, []byte(content))
	g.Add(filename)

	return g.Commit(message)
}

// CreateBranch creates and checks out a new branch.
func (g *GitRepo) CreateBranch(name string) {
	g.t.Helper()

	output, err := g.command("checkout", "-b", name).CombinedOutput()
	check.NoError(g.t, err, "Failed to create branch %s: %s", name, string(output))
}

// Checkout switches to a branch or commit.
func (g *GitRepo) Checkout(ref string) {
	g.t.Helper()

	output, err := g.command("checkout", ref).CombinedOutput()
	check.NoError(g.t, err, "Failed to checkout %s: %s", ref, string(output))
}

// Tag creates a tag.
func (g *GitRepo) Tag(name string) {
	g.t.Helper()

	output, err := g.command("tag", name).CombinedOutput()
	check.NoError(g.t, err, "Failed to create tag %s: %s", name, string(output))
}

// Log returns the git log output.
func (g *GitRepo) Log(args ...string) string {
	g.t.Helper()

	output, err := g.command(append([]string{"log"}, args...)...).CombinedOutput()
	check.NoError(g.t, err, "Failed to get log: %s", string(output))

	return string(output)
}

// AssertFileExists checks if a file exists in the repo.
func (g *GitRepo) AssertFileExists(path string) {
	g.t.Helper()
	check.True(g.t, g.fs.FileExists(path), "File %s should exist", path)
}

// AssertFileContains checks if a file contains expected content.
func (g *GitRepo) AssertFileContains(path, expected string) {
	g.t.Helper()
	g.fs.AssertFileContains(path, expected)
}

// Cleanup cleans up the test repository.
// This is usually handled automatically by t.TempDir() but can be called explicitly.
func (g *GitRepo) Cleanup() {
	g.t.Helper()
	// Cleanup is handled by t.TempDir() automatically
}

// command builds a git invocation in the repository with the fixture's environment.
func (g *GitRepo) command(args ...string) *exec.Cmd {
	cmd := exec.CommandContext(g.t.Context(), "git", args...) //nolint:gosec // Test helper with controlled arguments
	cmd.Dir = g.path
	cmd.Env = g.env

	return cmd
}
