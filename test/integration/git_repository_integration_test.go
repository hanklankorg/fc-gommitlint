//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"context"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	gitadapter "codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

func TestCommitRange_RetrievesCommitsFromGitRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		setupRepo    func(t *testing.T) (*gitrepo.TestRepo, string, string)
		wantCount    int
		expectedErr  error
		checkCommits func(t *testing.T, commits []domain.Commit)
	}{
		{
			name: "Linear history - commits in range",
			setupRepo: func(t *testing.T) (*gitrepo.TestRepo, string, string) {
				t.Helper()
				repo, hashes := gitrepo.NewInMemoryRepoWithLinearHistory(t)
				return repo, hashes[1], hashes[3] // B to D
			},
			wantCount: 2, // Commits C and D (B is excluded as it's the base)
		},
		{
			name: "Diverged branches - feature branch ahead of main",
			setupRepo: func(t *testing.T) (*gitrepo.TestRepo, string, string) {
				t.Helper()
				//   A -> B -> C (main)
				//    \-> D -> E (feature)
				repo, mainBranch, featureHash := gitrepo.NewInMemoryRepoWithDivergedBranches(t)
				return repo, mainBranch, featureHash
			},
			wantCount: 2, // Only commits D and E (not in main)
			checkCommits: func(t *testing.T, commits []domain.Commit) {
				t.Helper()
				// Verify we got the feature commits
				subjects := make([]string, len(commits))
				for i, c := range commits {
					subjects[i] = c.Subject
				}
				check.Contains(t, subjects, "feat: feature commit 1")
				check.Contains(t, subjects, "feat: feature commit 2")
				check.NotContains(t, subjects, "fix: main commit 1")
				check.NotContains(t, subjects, "docs: main commit 2")
			},
		},
		{
			name: "Same commit for from and to",
			setupRepo: func(t *testing.T) (*gitrepo.TestRepo, string, string) {
				t.Helper()
				repo, hash := gitrepo.NewInMemoryRepoWithSingleCommit(t)
				return repo, hash, hash
			},
			wantCount: 0, // No commits between A and A
		},
		{
			name: "Non-existent from reference",
			setupRepo: func(t *testing.T) (*gitrepo.TestRepo, string, string) {
				t.Helper()
				repo, hash := gitrepo.NewInMemoryRepoWithSingleCommit(t)
				return repo, "nonexistent", hash
			},
			expectedErr: gitadapter.ErrReferenceNotFound,
		},
		{
			name: "Non-existent to reference",
			setupRepo: func(t *testing.T) (*gitrepo.TestRepo, string, string) {
				t.Helper()
				repo, hash := gitrepo.NewInMemoryRepoWithSingleCommit(t)
				return repo, hash, "nonexistent"
			},
			expectedErr: gitadapter.ErrReferenceNotFound,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Setup test scenario using gitrepo utilities
			repo, fromRef, toRef := testCase.setupRepo(t)

			// Use the pre-created adapter from gitrepo (works with both filesystem and in-memory)
			adapter := repo.Adapter

			commits, err := adapter.GetCommitRange(context.Background(), fromRef, toRef)

			if testCase.expectedErr != nil {
				check.Error(t, err)
				check.ErrorIs(t, err, testCase.expectedErr)

				return
			}

			check.NoError(t, err)
			check.Len(t, commits, testCase.wantCount)

			// Run custom checks if provided
			if testCase.checkCommits != nil {
				testCase.checkCommits(t, commits)
			}
		})
	}
}
