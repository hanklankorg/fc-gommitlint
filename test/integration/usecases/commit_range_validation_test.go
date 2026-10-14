//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package usecases

import (
	"context"
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// GetCommitRange (ranges, diverged branches, and error identity) is covered by
// TestCommitRange_RetrievesCommitsFromGitRepository in the parent package; this
// file holds the GetHeadCommits contract, which no other test pins.

// TestGetHeadCommits_ReturnsNewestFirst pins the ordering a review tool
// depends on: HEAD first, then its ancestors.
func TestGetHeadCommits_ReturnsNewestFirst(t *testing.T) {
	t.Parallel()

	repo := gitrepo.NewRepoWithCommits(t, []gitrepo.CommitSpec{
		{Message: "feat: base commit"},
		{Message: "feat: review commit 1"},
		{Message: "fix: review commit 2"},
		{Message: "docs: review commit 3"},
	})

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), 3)
	check.NoError(t, err)
	check.Len(t, commits, 3)

	expectedSubjects := []string{"docs: review commit 3", "fix: review commit 2", "feat: review commit 1"}
	for i, commit := range commits {
		check.Equal(t, expectedSubjects[i], commit.Subject, "commits must arrive newest first")
		check.NotEmpty(t, commit.Hash)
	}
}

// TestGetHeadCommits_HandlesLargerHistories exercises a request well past the
// small fixtures the other tests use.
//
// The per-commit loop checks only that fields are populated, which twenty
// arbitrary — or twenty identical — commits would satisfy. The window is named
// explicitly as well: from a fifty-commit history the newest twenty are numbers
// 49 down to 30, so this pins WHICH commits came back and in what order, not
// merely how many.
func TestGetHeadCommits_HandlesLargerHistories(t *testing.T) {
	t.Parallel()

	const (
		historySize = 50
		requested   = 20
	)

	commitSpecs := make([]gitrepo.CommitSpec, historySize)
	for i := range commitSpecs {
		commitSpecs[i] = gitrepo.CommitSpec{
			Message: fmt.Sprintf("feat: performance test commit %d", i),
		}
	}
	repo := gitrepo.NewRepoWithCommits(t, commitSpecs)

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), requested)
	check.NoError(t, err)
	check.Len(t, commits, requested)

	seen := make(map[string]struct{}, len(commits))

	for i, commit := range commits {
		check.Equal(t,
			fmt.Sprintf("feat: performance test commit %d", historySize-1-i),
			commit.Subject,
			"the newest %d commits, newest first", requested)
		check.NotEmpty(t, commit.Hash)
		check.NotEmpty(t, commit.Author)

		_, duplicate := seen[commit.Hash]
		check.False(t, duplicate, "the same commit was returned twice: %s", commit.Hash)
		seen[commit.Hash] = struct{}{}
	}
}

// TestGetHeadCommits_BoundsTheRequestedCount covers the edges of the count
// argument, which nothing exercised. Asking for more history than exists is the
// ordinary case for a young repository — a shallow clone, or a project's first
// week — and it is exactly where an off-by-one or a slice out of range lives.
func TestGetHeadCommits_BoundsTheRequestedCount(t *testing.T) {
	t.Parallel()

	repo := gitrepo.NewRepoWithCommits(t, []gitrepo.CommitSpec{
		{Message: "feat: only commit"},
		{Message: "fix: second and last"},
	})

	t.Run("a request larger than the history returns all of it", func(t *testing.T) {
		t.Parallel()

		commits, err := repo.Adapter.GetHeadCommits(context.Background(), 100)
		check.NoError(t, err)
		check.Len(t, commits, 2, "the whole history, not an error and not a truncation")
		check.Equal(t, "fix: second and last", commits[0].Subject)
		check.Equal(t, "feat: only commit", commits[1].Subject)
	})

	t.Run("a request for exactly the history size returns all of it", func(t *testing.T) {
		t.Parallel()

		commits, err := repo.Adapter.GetHeadCommits(context.Background(), 2)
		check.NoError(t, err)
		check.Len(t, commits, 2)
	})

	t.Run("a request for one returns only HEAD", func(t *testing.T) {
		t.Parallel()

		commits, err := repo.Adapter.GetHeadCommits(context.Background(), 1)
		check.NoError(t, err)
		check.Len(t, commits, 1)
		check.Equal(t, "fix: second and last", commits[0].Subject)
	})
}
