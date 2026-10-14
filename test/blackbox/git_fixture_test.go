//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const signoffDisabledConfig = "gommitlint:\n  rules:\n    disabled:\n      - signoff\n"

type blackboxGitRepo struct {
	Path        string
	repository  *git.Repository
	t           *testing.T
	commitCount int
}

func newEmptyGitRepo(t *testing.T) *blackboxGitRepo {
	t.Helper()

	path := t.TempDir()
	repository, err := git.PlainInit(path, false)
	if err != nil {
		t.Fatalf("initialize Git repository: %v", err)
	}

	config, err := repository.Config()
	if err != nil {
		t.Fatalf("read Git repository config: %v", err)
	}
	config.User.Name = "Black Box"
	config.User.Email = "blackbox@example.invalid"
	config.Raw.SetOption("commit", "", "gpgSign", "false")
	config.Raw.SetOption("core", "", "hooksPath", filepath.Join(path, ".git", "hooks"))
	config.Raw.SetOption("core", "", "autocrlf", "false")
	config.Raw.SetOption("core", "", "filemode", "false")
	config.Raw.SetOption("core", "", "symlinks", "false")
	if err := repository.SetConfig(config); err != nil {
		t.Fatalf("write Git repository config: %v", err)
	}

	if err := os.WriteFile(filepath.Join(path, ".gommitlint.yaml"), []byte(signoffDisabledConfig), 0o600); err != nil {
		t.Fatalf("write black-box config: %v", err)
	}

	return &blackboxGitRepo{Path: path, repository: repository, t: t}
}

func (repo *blackboxGitRepo) withCommit(message string) *blackboxGitRepo {
	repo.t.Helper()

	repo.commitCount++
	name := fmt.Sprintf("commit-%d.txt", repo.commitCount)
	if err := os.WriteFile(filepath.Join(repo.Path, name), []byte(message), 0o600); err != nil {
		repo.t.Fatalf("write commit fixture: %v", err)
	}

	worktree, err := repo.repository.Worktree()
	if err != nil {
		repo.t.Fatalf("open Git worktree: %v", err)
	}
	if _, err := worktree.Add(name); err != nil {
		repo.t.Fatalf("stage commit fixture: %v", err)
	}

	signature := &object.Signature{
		Name:  "Black Box",
		Email: "blackbox@example.invalid",
		When:  time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(repo.commitCount) * time.Minute),
	}
	if _, err := worktree.Commit(message, &git.CommitOptions{Author: signature, Committer: signature}); err != nil {
		repo.t.Fatalf("create Git commit: %v", err)
	}

	return repo
}

func gitRepoPathWithCommits(t *testing.T, messages ...string) string {
	t.Helper()

	repo := newEmptyGitRepo(t)
	for _, message := range messages {
		repo.withCommit(message)
	}

	return repo.Path
}
