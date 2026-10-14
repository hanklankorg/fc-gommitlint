//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitrepo

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
)

// Global counter for unique repo names (thread-safe).
var repoCounter atomic.Uint64

// sharedTestFactory is a single shared factory for all tests.
var sharedTestFactory = git.NewInMemoryFactory()

// InMemoryFactory returns the shared factory for virtual test repositories.
func InMemoryFactory() *git.InMemoryFactory {
	return sharedTestFactory
}

// QuickRepoPath creates an in-memory repository and returns its virtual path.
// This is a drop-in replacement for PathWithCommits that uses in-memory storage.
func QuickRepoPath(t *testing.T, commits ...string) string {
	t.Helper()

	if len(commits) == 0 {
		commits = []string{"Initial commit"}
	}

	// Generate unique repository name
	repoID := repoCounter.Add(1)
	repoName := fmt.Sprintf("test-repo-%d", repoID)

	// Create in-memory repository
	storer := memory.NewStorage()
	fs := memfs.New()

	goGitRepo, err := gogit.Init(storer, fs)
	check.NoError(t, err, "Failed to initialize in-memory repository")

	// Configure minimal settings (much faster than full config)
	cfg, err := goGitRepo.Config()
	check.NoError(t, err)
	cfg.User.Name = "Test"
	cfg.User.Email = "test@test.com"
	cfg.Raw.SetOption("init", "", "defaultBranch", "main")
	err = goGitRepo.SetConfig(cfg)
	check.NoError(t, err)

	// Create worktree
	worktree, err := goGitRepo.Worktree()
	check.NoError(t, err)

	// Create test file
	file, err := fs.Create("test.txt")
	check.NoError(t, err)
	_, err = file.Write([]byte("test"))
	check.NoError(t, err)
	err = file.Close()
	check.NoError(t, err)

	// Add file
	_, err = worktree.Add("test.txt")
	check.NoError(t, err)

	// Create commits
	testTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	sig := &object.Signature{
		Name:  "Test",
		Email: "test@test.com",
		When:  testTime,
	}

	for _, message := range commits {
		_, err = worktree.Commit(message, &gogit.CommitOptions{
			Author:    sig,
			Committer: sig,
		})
		check.NoError(t, err)
	}

	// Register repository in the shared factory.
	sharedTestFactory.RegisterRepository(repoName, goGitRepo)

	// Return virtual path
	return sharedTestFactory.GetVirtualPath(repoName)
}
