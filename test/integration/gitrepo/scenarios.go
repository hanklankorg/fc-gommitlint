//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitrepo

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/go-git/go-git/v5/plumbing"
)

// Predefined test scenarios for common git repository patterns.

// NewInMemoryRepoWithDivergedBranches creates an in-memory repository with diverged branches for testing.
// Returns the main branch name and feature commit hash.
func NewInMemoryRepoWithDivergedBranches(t *testing.T) (*TestRepo, string, string) {
	t.Helper()

	repo := NewInMemoryRepo(t)

	// Create diverged history:
	//   Initial -> B -> C (main)
	//           \-> D -> E (feature)
	// Start with the existing initial commit
	head, err := repo.GoGitRepo.Head()
	check.NoError(t, err)
	initialHash := head.Hash()

	hashB := repo.createCommit("fix: main commit 1", "Test Author", "test@example.com", []plumbing.Hash{initialHash})
	hashC := repo.createCommit("docs: main commit 2", "Test Author", "test@example.com", []plumbing.Hash{hashB})

	// Create feature branch from initial commit
	hashD := repo.createCommit("feat: feature commit 1", "Test Author", "test@example.com", []plumbing.Hash{initialHash})
	hashE := repo.createCommit("feat: feature commit 2", "Test Author", "test@example.com", []plumbing.Hash{hashD})

	// Create main branch reference
	mainRef := plumbing.NewHashReference("refs/heads/main", hashC)
	err = repo.GoGitRepo.Storer.SetReference(mainRef)
	if err != nil {
		t.Fatalf("Failed to create main branch reference: %v", err)
	}

	return repo, "main", hashE.String()
}

// NewInMemoryRepoWithLinearHistory creates an in-memory repository with a simple linear commit history.
// Returns commit hashes for testing commit ranges.
func NewInMemoryRepoWithLinearHistory(t *testing.T) (*TestRepo, []string) {
	t.Helper()

	repo := NewInMemoryRepo(t)

	// Create linear history: Initial -> B -> C -> D
	// Start with the existing initial commit
	head, err := repo.GoGitRepo.Head()
	check.NoError(t, err)
	initialHash := head.Hash()

	hashB := repo.createCommit("fix: second commit", "Test Author", "test@example.com", []plumbing.Hash{initialHash})
	hashC := repo.createCommit("docs: third commit", "Test Author", "test@example.com", []plumbing.Hash{hashB})
	hashD := repo.createCommit("test: fourth commit", "Test Author", "test@example.com", []plumbing.Hash{hashC})

	hashes := []string{
		initialHash.String(),
		hashB.String(),
		hashC.String(),
		hashD.String(),
	}

	return repo, hashes
}

// NewInMemoryRepoWithSingleCommit creates an in-memory repository with exactly one commit for edge case testing.
func NewInMemoryRepoWithSingleCommit(t *testing.T) (*TestRepo, string) {
	t.Helper()

	repo := NewInMemoryRepo(t)
	// The NewInMemoryRepo already creates an initial commit, so get its hash
	head, err := repo.GoGitRepo.Head()
	check.NoError(t, err)

	return repo, head.Hash().String()
}

// NewRepoWithMergeHistory creates a repository with a merge commit and returns the hash range.
func NewRepoWithMergeHistory(t *testing.T) (*TestRepo, string, string) {
	t.Helper()

	repo := NewEmptyRepo(t)

	// Create history with merge:
	//   A -> B -> D (merge) -> E
	//    \-> C ---/
	hashA := repo.createCommit("feat: initial commit", "Test Author", "test@example.com", nil)
	hashB := repo.createCommit("fix: main branch commit", "Test Author", "test@example.com", []plumbing.Hash{hashA})
	hashC := repo.createCommit("feat: feature branch commit", "Test Author", "test@example.com", []plumbing.Hash{hashA})
	hashD := repo.createCommit("feat: merge commit", "Test Author", "test@example.com", []plumbing.Hash{hashB, hashC}) // Merge commit
	hashE := repo.createCommit("docs: post-merge commit", "Test Author", "test@example.com", []plumbing.Hash{hashD})

	return repo, hashA.String(), hashE.String()
}

// PathWithCommits builds a repository from commit subject lines and returns its
// path. It is the shape a scenario wants when it only needs somewhere on disk
// to point the product at, rather than the TestRepo handle.
//
// It lives here because multiple in-process integration scenarios use it.
func PathWithCommits(t *testing.T, commits ...string) string {
	t.Helper()

	if len(commits) == 0 {
		commits = []string{"Initial commit"}
	}

	specs := make([]CommitSpec, len(commits))
	for i, message := range commits {
		specs[i] = CommitSpec{Message: message}
	}

	return NewRepoWithCommits(t, specs).Path
}
