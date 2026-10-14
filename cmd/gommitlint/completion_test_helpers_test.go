//
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/urfave/cli/v3"
)

func createTestRepository(t *testing.T) string {
	t.Helper()

	repoDir := t.TempDir()
	repo, err := gogit.PlainInit(repoDir, false)
	check.NoError(t, err)

	commitHash := createTestCommit(t, repo, repoDir)
	check.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", commitHash)))
	check.NoError(t, repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")))
	check.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", commitHash)))
	check.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference("refs/tags/v1.0.0", commitHash)))

	return repoDir
}

func createTestCommit(t *testing.T, repo *gogit.Repository, repoDir string) plumbing.Hash {
	t.Helper()

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	filePath := filepath.Join(repoDir, "README.md")
	check.NoError(t, os.WriteFile(filePath, []byte("test"), 0o600))

	_, err = worktree.Add("README.md")
	check.NoError(t, err)

	commitHash, err := worktree.Commit("feat: initial", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	check.NoError(t, err)

	return commitHash
}

func addTestBranch(t *testing.T, repoPath string, branchName string) {
	t.Helper()

	repo, err := gogit.PlainOpen(repoPath)
	check.NoError(t, err)

	head, err := repo.Head()
	check.NoError(t, err)

	name := plumbing.ReferenceName("refs/heads/" + branchName)
	check.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(name, head.Hash())))
}

func runCompletionValues(t *testing.T, repoPath string, fn func(context.Context, *cli.Command) []string) []string {
	t.Helper()

	var values []string

	validate := &cli.Command{
		Name: commandValidate,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			values = fn(ctx, cmd)

			return nil
		},
	}
	root := &cli.Command{
		Name: appName,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flags.RepoPath},
		},
		Commands: []*cli.Command{validate},
	}

	args := []string{appName, commandValidate}
	if repoPath != "" {
		args = []string{appName, "--" + flags.RepoPath, repoPath, commandValidate}
	}

	check.NoError(t, root.Run(t.Context(), args))

	return values
}
