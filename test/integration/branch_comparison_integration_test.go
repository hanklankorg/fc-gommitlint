//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"context"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// GetCommitRange over diverged branches is covered by
// TestCommitRange_RetrievesCommitsFromGitRepository in
// git_repository_integration_test.go; this file pins the one adapter
// method no other test exercises.

// TestGetCommitsAheadCount_CountsCommitsAheadOfMain verifies the count of
// commits the checked-out feature branch is ahead of main.
func TestGetCommitsAheadCount_CountsCommitsAheadOfMain(t *testing.T) {
	t.Parallel()

	repo, mainBranch, _ := gitrepo.NewInMemoryRepoWithDivergedBranches(t)

	count, err := repo.Adapter.GetCommitsAheadCount(context.Background(), mainBranch)
	check.NoError(t, err)
	check.Equal(t, 2, count)
}
