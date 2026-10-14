//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package gitrepo provides reusable git repository test utilities.
// All repositories are created in isolated temporary directories.
package gitrepo

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
)

// TestRepo represents an isolated git repository for testing.
// All operations are performed in temporary directories.
type TestRepo struct {
	Path      string            // Absolute path to repository directory
	GoGitRepo *gogit.Repository // Low-level go-git repository
	Adapter   *git.Repository   // High-level gommitlint adapter
	t         *testing.T        // Test context for cleanup

	// Isolated configuration - doesn't affect host environment
	Author    *object.Signature // Default author for commits
	Committer *object.Signature // Default committer for commits
}

// CommitSpec defines a commit to be created in test repository.
type CommitSpec struct {
	Message  string
	Author   string
	Email    string
	Parents  []string // Parent commit hashes
	GPGKeyID string   // Optional GPG key for signing
}

// NewEmptyRepo creates a new empty git repository in an isolated temporary directory.
// The repository is automatically cleaned up when the test completes.
// Git operations are isolated from the host environment through explicit configuration.
func NewEmptyRepo(t *testing.T) *TestRepo {
	t.Helper()

	// Create isolated temporary directory
	tmpDir := t.TempDir() // Automatically cleaned up by testing framework

	// PlainInit creates the proper .git directory structure including objects/refs/config
	goGitRepo, err := gogit.PlainInit(tmpDir, false)
	check.NoError(t, err, "Failed to initialize git repository")

	// Configure git repository with isolated settings to prevent host interference
	cfg, err := goGitRepo.Config()
	check.NoError(t, err, "Failed to get repository config")

	// Set isolated user configuration
	cfg.User.Name = "Test Author"
	cfg.User.Email = "test@example.com"

	// Security and isolation settings
	cfg.Raw.SetOption("commit", "", "gpgSign", "false") // Disable GPG signing
	// Isolate from the developer's global core.hooksPath by pointing at the
	// repo-local hooks dir, which is empty in a fresh repo. /dev/null would
	// also silence host hooks, but hook install refuses a repo whose hooks
	// run from somewhere other than .git/hooks, and the hook tests use this
	// fixture.
	cfg.Raw.SetOption("core", "", "hooksPath", filepath.Join(tmpDir, ".git", "hooks"))
	cfg.Raw.SetOption("core", "", "askPass", "")              // Disable credential helpers
	cfg.Raw.SetOption("core", "", "autocrlf", "false")        // Consistent line endings
	cfg.Raw.SetOption("core", "", "filemode", "false")        // Ignore file mode changes
	cfg.Raw.SetOption("core", "", "symlinks", "false")        // Disable symlinks
	cfg.Raw.SetOption("credential", "", "helper", "")         // Clear credential helper
	cfg.Raw.SetOption("http", "", "sslVerify", "false")       // For test repos only
	cfg.Raw.SetOption("fetch", "", "prune", "false")          // Disable auto-prune
	cfg.Raw.SetOption("pull", "", "rebase", "false")          // Disable auto-rebase
	cfg.Raw.SetOption("init", "", "defaultBranch", "main")    // Consistent branch name
	cfg.Raw.SetOption("safe", "", "directory", tmpDir)        // Mark as safe directory
	cfg.Raw.SetOption("protocol", "file", "allow", "always")  // Allow file protocol
	cfg.Raw.SetOption("transfer", "", "fsckobjects", "false") // Skip object checks for speed

	// Save configuration
	err = goGitRepo.SetConfig(cfg)
	check.NoError(t, err, "Failed to set repository config")

	// Write a minimal gommitlint config that disables sign-off so integration
	// tests creating unsigned/unsigned-off commits pass validation.
	minimalConfig := "gommitlint:\n  rules:\n    disabled:\n      - signoff\n"
	err = os.WriteFile(filepath.Join(tmpDir, ".gommitlint.yaml"), []byte(minimalConfig), 0o644)
	check.NoError(t, err, "Failed to write test gommitlint config")

	// Create gommitlint repository adapter
	adapter, err := git.NewRepositoryFromPath(tmpDir)
	check.NoError(t, err, "Failed to create repository adapter")

	// Create isolated signatures for commits
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	author := &object.Signature{
		Name:  "Test Author",
		Email: "test@example.com",
		When:  testTime,
	}
	committer := &object.Signature{
		Name:  "Test Committer",
		Email: "test@example.com",
		When:  testTime,
	}

	return &TestRepo{
		Path:      tmpDir,
		GoGitRepo: goGitRepo,
		Adapter:   adapter,
		t:         t,
		Author:    author,
		Committer: committer,
	}
}

// NewInMemoryRepo creates a new in-memory git repository for fast testing.
// Ideal for integration tests that don't need actual filesystem operations.
// Git operations are isolated from the host environment through explicit configuration.
func NewInMemoryRepo(t *testing.T) *TestRepo {
	t.Helper()

	// Create in-memory storage and filesystem
	storer := memory.NewStorage()
	fs := memfs.New()

	goGitRepo, err := gogit.Init(storer, fs)
	check.NoError(t, err, "Failed to initialize in-memory git repository")

	// Configure git repository with isolated settings
	cfg, err := goGitRepo.Config()
	check.NoError(t, err, "Failed to get repository config")

	// Set isolated user configuration
	cfg.User.Name = "Test Author"
	cfg.User.Email = "test@example.com"

	// Security and isolation settings (same as filesystem repo)
	cfg.Raw.SetOption("commit", "", "gpgSign", "false")       // Disable GPG signing
	cfg.Raw.SetOption("core", "", "askPass", "")              // Disable credential helpers
	cfg.Raw.SetOption("core", "", "autocrlf", "false")        // Consistent line endings
	cfg.Raw.SetOption("core", "", "filemode", "false")        // Ignore file mode changes
	cfg.Raw.SetOption("core", "", "symlinks", "false")        // Disable symlinks
	cfg.Raw.SetOption("credential", "", "helper", "")         // Clear credential helper
	cfg.Raw.SetOption("http", "", "sslVerify", "false")       // For test repos only
	cfg.Raw.SetOption("fetch", "", "prune", "false")          // Disable auto-prune
	cfg.Raw.SetOption("pull", "", "rebase", "false")          // Disable auto-rebase
	cfg.Raw.SetOption("init", "", "defaultBranch", "main")    // Consistent branch name
	cfg.Raw.SetOption("protocol", "file", "allow", "always")  // Allow file protocol
	cfg.Raw.SetOption("transfer", "", "fsckobjects", "false") // Skip object checks for speed

	err = goGitRepo.SetConfig(cfg)
	check.NoError(t, err, "Failed to set repository config")

	// Create gommitlint repository adapter
	adapter, err := git.NewRepository(storer, fs)
	check.NoError(t, err, "Failed to create repository adapter")

	// Create isolated signatures for commits
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	author := &object.Signature{
		Name:  "Test Author",
		Email: "test@example.com",
		When:  testTime,
	}
	committer := &object.Signature{
		Name:  "Test Committer",
		Email: "test@example.com",
		When:  testTime,
	}

	// Create initial commit to have a valid git repository
	worktree, err := goGitRepo.Worktree()
	check.NoError(t, err)

	file, err := fs.Create("test.txt")
	check.NoError(t, err)
	_, err = file.Write([]byte("test content"))
	check.NoError(t, err)
	err = file.Close()
	check.NoError(t, err)

	// Add and commit with isolated configuration
	_, err = worktree.Add("test.txt")
	check.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
		Author:    author,
		Committer: committer,
	})
	check.NoError(t, err)

	return &TestRepo{
		GoGitRepo: goGitRepo,
		Adapter:   adapter,
		t:         t,
		Author:    author,
		Committer: committer,
	}
}

// NewRepoWithCommits creates a repository with a series of commits.
// Commits are created in the order specified, with automatic parent linking.
func NewRepoWithCommits(t *testing.T, commits []CommitSpec) *TestRepo {
	t.Helper()

	repo := NewEmptyRepo(t)

	// Track commit hashes for parent relationships
	commitHashes := make(map[string]plumbing.Hash)
	var lastHash plumbing.Hash

	for i, spec := range commits {
		// Determine parents
		var parents []plumbing.Hash
		if len(spec.Parents) > 0 {
			// Use specified parents
			for _, parentRef := range spec.Parents {
				if hash, exists := commitHashes[parentRef]; exists {
					parents = append(parents, hash)
				} else {
					check.Failf(t, "Parent commit not found", "Parent '%s' not found for commit %d", parentRef, i)
				}
			}
		} else if i > 0 {
			// Auto-link to previous commit
			parents = []plumbing.Hash{lastHash}
		}

		// Set default author if not specified
		author := spec.Author
		email := spec.Email
		if author == "" {
			author = "Test Author"
		}
		if email == "" {
			email = "test@example.com"
		}

		// Create commit
		hash := repo.createCommit(spec.Message, author, email, parents)
		lastHash = hash

		// Store hash for future parent references
		if spec.Message != "" {
			commitHashes[spec.Message] = hash
		}
		commitHashes[hash.String()[:8]] = hash // Short hash reference
	}

	return repo
}

// WithCommit adds a single commit to the repository (builder pattern).
func (r *TestRepo) WithCommit(message string) *TestRepo {
	r.t.Helper()

	// Get current HEAD to use as parent
	head, err := r.GoGitRepo.Head()
	var parents []plumbing.Hash
	if err == nil {
		parents = []plumbing.Hash{head.Hash()}
	}

	r.createCommit(message, "Test Author", "test@example.com", parents)
	return r
}

// WithHook adds a git hook to the repository.
func (r *TestRepo) WithHook(hookType, content string) *TestRepo {
	r.t.Helper()

	hooksDir := filepath.Join(r.Path, ".git", "hooks")
	err := os.MkdirAll(hooksDir, 0o755)
	check.NoError(r.t, err, "Failed to create hooks directory")

	hookPath := filepath.Join(hooksDir, hookType)
	err = os.WriteFile(hookPath, []byte(content), 0o755)
	check.NoError(r.t, err, "Failed to write hook file")

	return r
}

// GetCommitHash returns the hash of the latest commit.
func (r *TestRepo) GetCommitHash() string {
	r.t.Helper()

	head, err := r.GoGitRepo.Head()
	check.NoError(r.t, err, "Failed to get HEAD reference")

	return head.Hash().String()
}

// createCommit creates a commit with the specified parameters.
// Internal helper that performs the actual git operations.
func (r *TestRepo) createCommit(message, author, email string, parents []plumbing.Hash) plumbing.Hash {
	r.t.Helper()

	worktree, err := r.GoGitRepo.Worktree()
	check.NoError(r.t, err, "Failed to get worktree")

	filename := fmt.Sprintf("test_%d.txt", time.Now().UnixNano())

	if r.Path != "" {
		// Filesystem-based repository
		filePath := filepath.Join(r.Path, filename)
		err = os.WriteFile(filePath, []byte(message), PermPublicRead)
		check.NoError(r.t, err, "Failed to create test file")
	} else {
		// In-memory repository
		wt, err := r.GoGitRepo.Worktree()
		check.NoError(r.t, err, "Failed to get worktree")

		fs := wt.Filesystem
		file, err := fs.Create(filename)
		check.NoError(r.t, err, "Failed to create file in memory")

		_, err = file.Write([]byte(message))
		check.NoError(r.t, err, "Failed to write file in memory")

		err = file.Close()
		check.NoError(r.t, err, "Failed to close file in memory")
	}

	// Stage the file
	_, err = worktree.Add(filename)
	check.NoError(r.t, err, "Failed to stage file")

	// Use provided author/email or fall back to isolated defaults
	authorSig := r.Author
	if author != "" && email != "" {
		authorSig = &object.Signature{
			Name:  author,
			Email: email,
			When:  r.Author.When, // Keep consistent time
		}
	}

	// Create commit with isolated configuration
	commitHash, err := worktree.Commit(message, &gogit.CommitOptions{
		Author:    authorSig,
		Committer: r.Committer, // Always use isolated committer
		Parents:   parents,
	})
	check.NoError(r.t, err, "Failed to create commit")

	return commitHash
}
