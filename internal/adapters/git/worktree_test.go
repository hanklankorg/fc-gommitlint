// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package git

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestRepository_LinkedWorktreeReadsSharedHistoryAndOwnHEAD(t *testing.T) {
	mainPath := createTestRepo(t)
	mainRepo, err := gogit.PlainOpen(mainPath)
	check.NoError(t, err)
	base, err := mainRepo.Head()
	check.NoError(t, err)
	check.NoError(t, mainRepo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", base.Hash())))

	mainWorktree, err := mainRepo.Worktree()
	check.NoError(t, err)
	featureHash, err := mainWorktree.Commit("feat: linked worktree change", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
			When:  time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
		},
	})
	check.NoError(t, err)
	check.NoError(t, mainRepo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature", featureHash)))
	check.NoError(t, mainRepo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")))

	linkedPath := t.TempDir()
	gitDir := filepath.Join(mainPath, ".git", "worktrees", "feature")
	check.NoError(t, os.MkdirAll(gitDir, 0o700))
	check.NoError(t, os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../..\n"), 0o600))
	check.NoError(t, os.WriteFile(filepath.Join(gitDir, "gitdir"), []byte(filepath.Join(linkedPath, ".git")+"\n"), 0o600))
	check.NoError(t, os.WriteFile(filepath.Join(linkedPath, ".git"), []byte("gitdir: "+gitDir+"\n"), 0o600))

	for _, testCase := range []struct {
		name string
		head string
	}{
		{name: "branch", head: "ref: refs/heads/feature\n"},
		{name: "detached", head: featureHash.String() + "\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			check.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(testCase.head), 0o600))

			repo, err := NewRepositoryFromPath(linkedPath)
			check.NoError(t, err)

			exists, err := repo.BranchExists(t.Context(), "main")
			check.NoError(t, err)
			check.True(t, exists)
			exists, err = repo.BranchExists(t.Context(), "missing")
			check.NoError(t, err)
			check.False(t, exists)

			head, err := repo.GetCommit(t.Context(), HeadRefName)
			check.NoError(t, err)
			check.Equal(t, featureHash.String(), head.Hash)
			commits, err := repo.GetCommitRange(t.Context(), "main", HeadRefName)
			check.NoError(t, err)
			check.Len(t, commits, 1)
			check.Equal(t, featureHash.String(), commits[0].Hash)

			mainHead, err := mainRepo.Head()
			check.NoError(t, err)
			check.Equal(t, base.Hash(), mainHead.Hash())
		})
	}
}
