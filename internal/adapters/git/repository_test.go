// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//nolint:varnamelen // Short names follow the git objects they hold.
package git

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

const (
	testGitAuthorEmail    = "test@example.com"
	testGitAuthorName     = "Test Author"
	testGitBranchMain     = "main"
	testGitNonexistentRef = "nonexistent-ref"
)

// TestNewRepository tests repository creation.
func TestRepository_InitializesWithValidWorkingDir(t *testing.T) {
	testCases := []struct {
		name        string
		setupRepo   func(t *testing.T) string
		expectError bool
	}{
		{
			name:      "valid repository",
			setupRepo: createTestRepo,
		},
		{
			name: "nonexistent path",
			setupRepo: func(_ *testing.T) string {
				return "/nonexistent/path"
			},
			expectError: true,
		},
		{
			name: "empty directory",
			setupRepo: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()

				return dir
			},
			expectError: true,
		},
		{
			name: "file instead of directory",
			setupRepo: func(t *testing.T) string {
				t.Helper()
				env := testhelpers.NewRealFSTestEnv(t)
				file := filepath.Join(env.TempDir, "file.txt")
				env.FS.WriteFile(file, []byte("test"))

				return file
			},
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			path := testCase.setupRepo(t)

			repo, err := NewRepositoryFromPath(path)

			if testCase.expectError {
				check.Error(t, err)
				check.Nil(t, repo)
			} else {
				check.NoError(t, err)
				check.NotNil(t, repo)
				check.NotNil(t, repo.repo)
			}
		})
	}
}

// TestRepositoryGetCommit tests GetCommit functionality.
func TestRepositoryGetCommit(t *testing.T) {
	// Use in-memory repository for testing git logic
	repo := createInMemoryRepoWithCommits(t)

	ctx := t.Context()

	testCases := []struct {
		name        string
		ref         string
		expectError bool
	}{
		{
			name: "HEAD reference",
			ref:  HeadRefName,
		},
		{
			name:        "nonexistent reference",
			ref:         "nonexistent-branch",
			expectError: true,
		},
		{
			name:        "invalid hash",
			ref:         "invalid-hash-123",
			expectError: true,
		},
		{
			// Rejected for being unresolvable, not for looking like a path:
			// see the security policy's traversal case in internal/domain.
			name:        "traversal-looking reference does not resolve",
			ref:         "../../../etc/passwd",
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			commit, err := repo.GetCommit(ctx, testCase.ref)

			if testCase.expectError {
				check.Error(t, err)
				check.Empty(t, commit.Hash)
			} else {
				check.NoError(t, err)
				check.NotEmpty(t, commit.Hash)
				check.NotEmpty(t, commit.Message)
				check.NotEmpty(t, commit.Author)
			}
		})
	}
}

// TestRepositoryGetCommitRange tests GetCommitRange functionality.
func TestRepositoryGetCommitRange(t *testing.T) {
	// Use in-memory repository for testing git logic
	repo := createInMemoryRepoWithCommits(t)

	ctx := t.Context()

	testCases := []struct {
		name        string
		fromRef     string
		toRef       string
		expectError bool
		minCommits  int
	}{
		{
			name:        "nonexistent from ref",
			fromRef:     "nonexistent",
			toRef:       HeadRefName,
			expectError: true,
		},
		{
			name:        "nonexistent to ref",
			fromRef:     HeadRefName,
			toRef:       "nonexistent",
			expectError: true,
		},
		{
			// The input policy rejects selected metacharacters only and leaves
			// ref-name shape to git, so this
			// fails as an unresolvable reference like any other typo.
			name:        "traversal-looking from ref does not resolve",
			fromRef:     "../invalid",
			toRef:       HeadRefName,
			expectError: true,
		},
		{
			name:       "relative reference HEAD~1 to HEAD",
			fromRef:    "HEAD~1",
			toRef:      HeadRefName,
			minCommits: 1,
		},
		{
			name:       "relative reference HEAD~2 to HEAD",
			fromRef:    "HEAD~2",
			toRef:      HeadRefName,
			minCommits: 2,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			commits, err := repo.GetCommitRange(ctx, testCase.fromRef, testCase.toRef)

			if testCase.expectError {
				check.ErrorIs(t, err, ErrReferenceNotFound)
				check.Nil(t, commits)
			} else {
				check.NoError(t, err)
				check.GreaterOrEqual(t, len(commits), testCase.minCommits)

				for _, commit := range commits {
					check.NotEmpty(t, commit.Hash)
					check.NotEmpty(t, commit.Message)
				}
			}
		})
	}
}

// TestRepositoryGetHeadCommits tests GetHeadCommits functionality.
func TestRepositoryGetHeadCommits(t *testing.T) {
	// Use in-memory repository for testing git logic
	repo := createInMemoryRepoWithCommits(t)

	ctx := t.Context()

	testCases := []struct {
		name        string
		count       int
		expectedMin int
		expectedMax int
	}{
		{
			name:        "single commit",
			count:       1,
			expectedMin: 1,
			expectedMax: 1,
		},
		{
			name:        "multiple commits",
			count:       3,
			expectedMin: 3,
			expectedMax: 3,
		},
		{
			name:        "zero commits",
			count:       0,
			expectedMin: 0,
			expectedMax: 0,
		},
		{
			// The fixture has exactly three commits, so asking for more must
			// return those three -- a min/max spread here would pass on any
			// count in between and say nothing.
			name:        "more than available",
			count:       100,
			expectedMin: 3,
			expectedMax: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			commits, err := repo.GetHeadCommits(ctx, testCase.count)

			check.NoError(t, err)
			check.GreaterOrEqual(t, len(commits), testCase.expectedMin)
			check.LessOrEqual(t, len(commits), testCase.expectedMax)

			for _, commit := range commits {
				check.NotEmpty(t, commit.Hash)
				check.NotEmpty(t, commit.Message)
				check.NotEmpty(t, commit.Author)
			}
		})
	}
}

// TestRepositoryGetCommitsAheadCount tests GetCommitsAheadCount functionality.
func TestRepositoryGetCommitsAheadCount(t *testing.T) {
	testCases := []struct {
		name            string
		setupRepo       func(t *testing.T) *Repository
		ctx             func() context.Context
		referenceBranch string
		expectedCount   int
		expectedErr     error
	}{
		{
			name:            "nonexistent branch",
			setupRepo:       createInMemoryRepoWithReferenceBranches,
			ctx:             t.Context,
			referenceBranch: "nonexistent-branch",
			expectedErr:     ErrReferenceNotFound,
		},
		{
			name:            "explicit local main",
			setupRepo:       createInMemoryRepoWithReferenceBranches,
			ctx:             t.Context,
			referenceBranch: testGitBranchMain,
			expectedCount:   2,
		},
		{
			name:            "explicit origin main",
			setupRepo:       createInMemoryRepoWithReferenceBranches,
			ctx:             t.Context,
			referenceBranch: "origin/main",
			expectedCount:   1,
		},
		{
			name:            "reference matching current branch returns zero",
			setupRepo:       createInMemoryRepoWithReferenceBranches,
			ctx:             t.Context,
			referenceBranch: "feature/a-test",
			expectedCount:   0,
		},
		{
			name:            "fully qualified local ref",
			setupRepo:       createInMemoryRepoWithReferenceBranches,
			ctx:             t.Context,
			referenceBranch: "refs/heads/main",
			expectedCount:   2,
		},
		{
			name:            "diverged branch counts only commits not in reference",
			setupRepo:       createInMemoryRepoWithDivergedReferenceBranches,
			ctx:             t.Context,
			referenceBranch: testGitBranchMain,
			expectedCount:   2,
		},
		{
			name:            "detached HEAD still compares against branch reference",
			setupRepo:       createInMemoryRepoWithDetachedHead,
			ctx:             t.Context,
			referenceBranch: testGitBranchMain,
			expectedCount:   2,
		},
		{
			name:            "sibling branch still counts commits unique to HEAD",
			setupRepo:       createInMemoryRepoWithReferenceAheadBranch,
			ctx:             t.Context,
			referenceBranch: testGitBranchMain,
			expectedCount:   1,
		},
		{
			name:            "merged HEAD counts feature and merge commits",
			setupRepo:       createInMemoryRepoWithMergedHeadBranch,
			ctx:             t.Context,
			referenceBranch: testGitBranchMain,
			expectedCount:   3,
		},
		{
			name:      "context canceled",
			setupRepo: createInMemoryRepoWithReferenceBranches,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx
			},
			referenceBranch: testGitBranchMain,
			expectedErr:     context.Canceled,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := testCase.setupRepo(t)
			count, err := repo.GetCommitsAheadCount(testCase.ctx(), testCase.referenceBranch)

			if testCase.expectedErr != nil {
				check.Error(t, err)
				check.ErrorIs(t, err, testCase.expectedErr)

				return
			}

			check.NoError(t, err)
			check.Equal(t, testCase.expectedCount, count)
		})
	}
}

// TestResolveReference tests the resolveReference helper method.
func TestResolveReference(t *testing.T) {
	// Use in-memory repository for testing git logic
	repo := createInMemoryRepoWithCommits(t)

	testCases := []struct {
		name        string
		ref         string
		expectError bool
	}{
		{
			name: "HEAD reference",
			ref:  HeadRefName,
		},
		{
			name: "HEAD tilde reference",
			ref:  "HEAD~1",
		},
		{
			name: "HEAD tilde two",
			ref:  "HEAD~2",
		},
		{
			name: "HEAD caret reference",
			ref:  "HEAD^",
		},
		{
			// Rejected for being unresolvable, not for looking like a path:
			// see the security policy's traversal case in internal/domain.
			name:        "traversal-looking reference does not resolve",
			ref:         "../../../etc/passwd",
			expectError: true,
		},
		{
			name:        "empty reference",
			ref:         "",
			expectError: true,
		},
		{
			name:        "nonexistent reference",
			ref:         testGitNonexistentRef,
			expectError: true,
		},
		{
			name:        "HEAD tilde beyond history",
			ref:         "HEAD~100",
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			hash, err := repo.resolveReference(testCase.ref)

			if testCase.expectError {
				check.Error(t, err)
				check.Equal(t, plumbing.ZeroHash, hash)
			} else {
				check.NoError(t, err)
				check.NotEqual(t, plumbing.ZeroHash, hash)
			}
		})
	}
}

// TestCommitsInRange_LimitsCommitsLoadedForRange pins the resource bound: it
// counts the commits a range walk loads, not the history below the range. A
// linear range of n commits loads those n and the boundary commit.
func TestCommitsInRange_LimitsCommitsLoadedForRange(t *testing.T) {
	const limit = 4

	repo, hashes := createInMemoryLinearHistory(t, 50)
	ctx := t.Context()
	head := hashes[49]

	commits, err := repo.commitsInRange(ctx, hashes[46], head, limit)
	check.NoError(t, err)
	check.Len(t, commits, 3)

	commits, err = repo.commitsInRange(ctx, hashes[45], head, limit)
	check.ErrorIs(t, err, ErrCommitCountExceeded)
	check.Nil(t, commits)
}

// TestCommitsInRange_ExcludesAncestorsReachedAfterExpansion covers a walk where
// the included side overtakes the excluded side: X is newer than its child F,
// so X and its parent are queued as part of the range before F reaches them.
// Both must still be excluded once F does.
func TestCommitsInRange_ExcludesAncestorsReachedAfterExpansion(t *testing.T) {
	storer := memory.NewStorage()
	goGitRepo, err := gogit.Init(storer, memfs.New())
	check.NoError(t, err)

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := func(message string, seconds int, parents ...plumbing.Hash) plumbing.Hash {
		signature := object.Signature{Name: testGitAuthorName, Email: testGitAuthorEmail, When: base.Add(time.Duration(seconds) * time.Second)}
		commit := &object.Commit{Author: signature, Committer: signature, Message: message, ParentHashes: parents}
		obj := storer.NewEncodedObject()
		check.NoError(t, commit.Encode(obj))

		hash, err := storer.SetEncodedObject(obj)
		check.NoError(t, err)

		return hash
	}

	root := store("B", 0)
	skewed := store("X", 50, root)
	from := store("F", 20, skewed)
	to := store("T", 30, skewed)

	repo := &Repository{repo: goGitRepo}

	commits, err := repo.commitsInRange(t.Context(), from, to, MaxCommitCount)
	check.NoError(t, err)
	check.Len(t, commits, 1)
	check.Equal(t, to, commits[0].Hash)
}

// TestCommitsInRange_MatchesAncestrySetDifference checks every from..to pair
// of a merge-heavy history against git's definition of the range: the
// ancestors of to that are not ancestors of from. Committer dates follow the
// ancestry, so the date-based early stop must not drop or add a commit, and
// the walk returns the range newest first.
func TestCommitsInRange_MatchesAncestrySetDifference(t *testing.T) {
	const length = 24

	storer := memory.NewStorage()
	goGitRepo, err := gogit.Init(storer, memfs.New())
	check.NoError(t, err)

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	hashes := make([]plumbing.Hash, 0, length)
	parents := make(map[plumbing.Hash][]plumbing.Hash, length)

	for i := range length {
		signature := object.Signature{Name: testGitAuthorName, Email: testGitAuthorEmail, When: base.Add(time.Duration(i) * time.Second)}
		commit := &object.Commit{Author: signature, Committer: signature, Message: fmt.Sprintf("Commit %d", i)}

		// Two root commits, then side branches and merges that reach back
		// over several generations.
		switch {
		case i < 2:
		case i%3 == 0:
			commit.ParentHashes = []plumbing.Hash{hashes[i-1], hashes[(i*7)%(i-1)]}
		default:
			commit.ParentHashes = []plumbing.Hash{hashes[(i*5)%i]}
		}

		obj := storer.NewEncodedObject()
		check.NoError(t, commit.Encode(obj))

		hash, err := storer.SetEncodedObject(obj)
		check.NoError(t, err)

		hashes = append(hashes, hash)
		parents[hash] = commit.ParentHashes
	}

	ancestors := func(start plumbing.Hash) map[plumbing.Hash]bool {
		seen := map[plumbing.Hash]bool{}
		pending := []plumbing.Hash{start}

		for len(pending) > 0 {
			current := pending[len(pending)-1]
			pending = pending[:len(pending)-1]

			if !seen[current] {
				seen[current] = true
				pending = append(pending, parents[current]...)
			}
		}

		return seen
	}

	repo := &Repository{repo: goGitRepo}

	for fromIdx, from := range hashes {
		excluded := ancestors(from)

		for toIdx := range hashes {
			included := ancestors(hashes[toIdx])

			want := []plumbing.Hash{}

			for idx := toIdx; idx >= 0; idx-- {
				if included[hashes[idx]] && !excluded[hashes[idx]] {
					want = append(want, hashes[idx])
				}
			}

			commits, err := repo.commitsInRange(t.Context(), from, hashes[toIdx], MaxCommitCount)
			check.NoError(t, err)

			got := make([]plumbing.Hash, 0, len(commits))
			for _, commit := range commits {
				got = append(got, commit.Hash)
			}

			check.Equal(t, want, got, "range %d..%d", fromIdx, toIdx)
		}
	}
}

// TestCommitsInRange_StopsWhenContextIsCancelled pins that a cancelled walk
// reports the context error instead of a partial range.
func TestCommitsInRange_StopsWhenContextIsCancelled(t *testing.T) {
	repo, hashes := createInMemoryLinearHistory(t, 5)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	commits, err := repo.commitsInRange(ctx, hashes[0], hashes[4], MaxCommitCount)
	check.ErrorIs(t, err, context.Canceled)
	check.Nil(t, commits)
}

// Test utilities for repository creation

// createInMemoryRepo creates an in-memory git repository for unit testing.
// This avoids filesystem operations for logic testing.
func createInMemoryRepo(t *testing.T) *Repository {
	t.Helper()

	// Create in-memory storage and filesystem
	storer := memory.NewStorage()
	fs := memfs.New()

	// Initialize repository
	repo, err := gogit.Init(storer, fs)
	check.NoError(t, err)

	// Create initial commit
	worktree, err := repo.Worktree()
	check.NoError(t, err)

	// Create a test file in memory
	file, err := fs.Create("test.txt")
	check.NoError(t, err)
	_, err = file.Write([]byte("test content"))
	check.NoError(t, err)
	err = file.Close()
	check.NoError(t, err)

	// Add and commit
	_, err = worktree.Add("test.txt")
	check.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
		},
	})
	check.NoError(t, err)

	// Create Repository wrapper
	repository := &Repository{
		repo: repo,
	}

	return repository
}

// createInMemoryRepoWithCommits creates an in-memory repository with multiple commits.
func createInMemoryRepoWithCommits(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepo(t)
	repo := repository.repo

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	// Get filesystem from worktree
	fs := worktree.Filesystem

	// Create additional commits
	for i := 2; i <= 3; i++ {
		// Update file content
		file, err := fs.Create("test.txt")
		check.NoError(t, err)

		content := fmt.Sprintf("test content %d", i)
		_, err = file.Write([]byte(content))
		check.NoError(t, err)
		err = file.Close()
		check.NoError(t, err)

		_, err = worktree.Add("test.txt")
		check.NoError(t, err)

		commitMsg := fmt.Sprintf("Commit %d", i)
		_, err = worktree.Commit(commitMsg, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  testGitAuthorName,
				Email: testGitAuthorEmail,
			},
		})
		check.NoError(t, err)
	}

	return repository
}

func createInMemoryRepoWithReferenceBranches(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepo(t)
	repo := repository.repo

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	fs := worktree.Filesystem

	commitWithContent := func(message, content string) plumbing.Hash {
		file, err := fs.Create("test.txt")
		check.NoError(t, err)
		_, err = file.Write([]byte(content))
		check.NoError(t, err)
		check.NoError(t, file.Close())

		_, err = worktree.Add("test.txt")
		check.NoError(t, err)

		hash, err := worktree.Commit(message, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  testGitAuthorName,
				Email: testGitAuthorEmail,
			},
		})
		check.NoError(t, err)

		return hash
	}

	headRef, err := repo.Head()
	check.NoError(t, err)

	mainHash := headRef.Hash()
	originHash := commitWithContent("Commit 2", "test content 2")
	headHash := commitWithContent("Commit 3", "test content 3")

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", mainHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/remotes/origin/main", originHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature/a-test", headHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/feature/a-test"))
	check.NoError(t, err)

	return repository
}

func createInMemoryRepoWithDivergedReferenceBranches(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepo(t)
	repo := repository.repo

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	fs := worktree.Filesystem

	commitWithContent := func(message, content string) plumbing.Hash {
		file, err := fs.Create("test.txt")
		check.NoError(t, err)
		_, err = file.Write([]byte(content))
		check.NoError(t, err)
		check.NoError(t, file.Close())

		_, err = worktree.Add("test.txt")
		check.NoError(t, err)

		hash, err := worktree.Commit(message, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  testGitAuthorName,
				Email: testGitAuthorEmail,
			},
		})
		check.NoError(t, err)

		return hash
	}

	headRef, err := repo.Head()
	check.NoError(t, err)

	baseHash := headRef.Hash()

	_ = commitWithContent("Commit 2", "main content 2")
	mainHash := commitWithContent("Commit 3", "main content 3")

	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, baseHash))
	check.NoError(t, err)

	worktree, err = repo.Worktree()
	check.NoError(t, err)

	fs = worktree.Filesystem

	_ = commitWithContent("Commit 4", "feature content 1")
	featureHash := commitWithContent("Commit 5", "feature content 2")

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", mainHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature/a-test", featureHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/feature/a-test"))
	check.NoError(t, err)

	return repository
}

func createInMemoryRepoWithReferenceAheadBranch(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepo(t)
	repo := repository.repo

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	fs := worktree.Filesystem

	commitWithContent := func(message, content string) plumbing.Hash {
		file, err := fs.Create("test.txt")
		check.NoError(t, err)
		_, err = file.Write([]byte(content))
		check.NoError(t, err)
		check.NoError(t, file.Close())

		_, err = worktree.Add("test.txt")
		check.NoError(t, err)

		hash, err := worktree.Commit(message, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  testGitAuthorName,
				Email: testGitAuthorEmail,
			},
		})
		check.NoError(t, err)

		return hash
	}

	headRef, err := repo.Head()
	check.NoError(t, err)

	baseHash := headRef.Hash()

	headHash := commitWithContent("Commit 2", "head content 2")

	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, baseHash))
	check.NoError(t, err)

	worktree, err = repo.Worktree()
	check.NoError(t, err)

	fs = worktree.Filesystem

	_ = commitWithContent("Commit 3", "main content 1")
	mainHash := commitWithContent("Commit 4", "main content 2")

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", mainHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature/a-test", headHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/feature/a-test"))
	check.NoError(t, err)

	return repository
}

func createInMemoryRepoWithMergedHeadBranch(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepo(t)
	repo := repository.repo

	worktree, err := repo.Worktree()
	check.NoError(t, err)

	fs := worktree.Filesystem

	commitWithContent := func(message, content string) plumbing.Hash {
		file, err := fs.Create("test.txt")
		check.NoError(t, err)
		_, err = file.Write([]byte(content))
		check.NoError(t, err)
		check.NoError(t, file.Close())

		_, err = worktree.Add("test.txt")
		check.NoError(t, err)

		hash, err := worktree.Commit(message, &gogit.CommitOptions{
			Author: &object.Signature{
				Name:  testGitAuthorName,
				Email: testGitAuthorEmail,
			},
		})
		check.NoError(t, err)

		return hash
	}

	headRef, err := repo.Head()
	check.NoError(t, err)

	baseHash := headRef.Hash()

	mainHash := commitWithContent("Commit 2", "main content 1")

	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, baseHash))
	check.NoError(t, err)

	worktree, err = repo.Worktree()
	check.NoError(t, err)

	fs = worktree.Filesystem

	_ = commitWithContent("Commit 3", "feature content 1")
	featureHash := commitWithContent("Commit 4", "feature content 2")
	featureCommit, err := repo.CommitObject(featureHash)
	check.NoError(t, err)

	mergeCommit := &object.Commit{
		Author: object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
		},
		Committer: object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
		},
		Message:      "Commit 5",
		TreeHash:     featureCommit.TreeHash,
		ParentHashes: []plumbing.Hash{featureHash, mainHash},
	}

	obj := repo.Storer.NewEncodedObject()
	err = mergeCommit.Encode(obj)
	check.NoError(t, err)

	mergeHash, err := repo.Storer.SetEncodedObject(obj)
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/main", mainHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/feature/a-test", mergeHash))
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/feature/a-test"))
	check.NoError(t, err)

	return repository
}

func createInMemoryRepoWithDetachedHead(t *testing.T) *Repository {
	t.Helper()

	repository := createInMemoryRepoWithReferenceBranches(t)
	repo := repository.repo

	headRef, err := repo.Head()
	check.NoError(t, err)

	err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.HEAD, headRef.Hash()))
	check.NoError(t, err)

	return repository
}

// createTestRepo creates a minimal test git repository on filesystem.
// Needed for testing filesystem-specific functionality like NewRepositoryFromPath.
func createTestRepo(t *testing.T) string {
	t.Helper()

	env := testhelpers.NewRealFSTestEnv(t)
	dir := env.TempDir

	// Use PlainInit to create proper git structure, then access via storage interfaces
	repo, err := gogit.PlainInit(dir, false)
	check.NoError(t, err)

	// Create initial commit
	worktree, err := repo.Worktree()
	check.NoError(t, err)

	// Create a test file
	testFile := filepath.Join(dir, "test.txt")
	env.FS.WriteFile(testFile, []byte("test content"))

	// Add and commit
	_, err = worktree.Add("test.txt")
	check.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
		},
	})
	check.NoError(t, err)

	return dir
}

// TestGetCommitRange_ReturnsChronologicalOrder verifies commits are returned oldest-first.
func TestGetCommitRange_ReturnsChronologicalOrder(t *testing.T) {
	t.Parallel()

	repo := createInMemoryRepoWithCommits(t)
	ctx := t.Context()

	// Get all commits via GetHeadCommits (returns newest-first)
	allCommits, err := repo.GetHeadCommits(ctx, 10)
	check.NoError(t, err)
	check.GreaterOrEqual(t, len(allCommits), 3, "Need at least 3 commits for ordering test")

	// Use the oldest commit as base
	oldestHash := allCommits[len(allCommits)-1].Hash

	// Get commit range (should return oldest-first)
	rangeCommits, err := repo.GetCommitRange(ctx, oldestHash, HeadRefName)
	check.NoError(t, err)
	check.GreaterOrEqual(t, len(rangeCommits), 2, "Should have at least 2 commits in range")

	// Verify chronological order: each commit's date should be <= the next
	for i := range len(rangeCommits) - 1 {
		current := rangeCommits[i].CommitDate
		next := rangeCommits[i+1].CommitDate
		check.LessOrEqual(t, current, next,
			"Commits should be in chronological order (oldest first): commit %d (%s) should be <= commit %d (%s)",
			i, current, i+1, next)
	}

	emptyCommits, err := repo.GetCommitRange(ctx, oldestHash, oldestHash)
	check.NoError(t, err)
	check.Empty(t, emptyCommits)
}

// TestRepository_DeepHistoryDoesNotLimitShortRange pins that the depth of the
// history below a range does not decide whether the range can be read.
func TestRepository_DeepHistoryDoesNotLimitShortRange(t *testing.T) {
	const length = 10_003

	repo, hashes := createInMemoryLinearHistory(t, length)
	ctx := t.Context()

	commits, err := repo.GetCommitRange(ctx, "HEAD~1", HeadRefName)
	check.NoError(t, err)
	check.Len(t, commits, 1)
	check.Equal(t, hashes[length-1].String(), commits[0].Hash)

	err = repo.repo.Storer.SetReference(plumbing.NewHashReference("refs/heads/base", hashes[length-3]))
	check.NoError(t, err)

	ahead, err := repo.GetCommitsAheadCount(ctx, "base")
	check.NoError(t, err)
	check.Equal(t, 2, ahead)
}

// createInMemoryLinearHistory stores length commits, one second apart, directly
// in memory storage and points main and HEAD at the newest. It returns the
// hashes oldest first.
func createInMemoryLinearHistory(t *testing.T, length int) (*Repository, []plumbing.Hash) {
	t.Helper()

	storer := memory.NewStorage()
	repo, err := gogit.Init(storer, memfs.New())
	check.NoError(t, err)

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	hashes := make([]plumbing.Hash, 0, length)

	for i := range length {
		signature := object.Signature{
			Name:  testGitAuthorName,
			Email: testGitAuthorEmail,
			When:  base.Add(time.Duration(i) * time.Second),
		}
		commit := &object.Commit{
			Author:    signature,
			Committer: signature,
			Message:   fmt.Sprintf("Commit %d", i),
		}

		if i > 0 {
			commit.ParentHashes = []plumbing.Hash{hashes[i-1]}
		}

		obj := storer.NewEncodedObject()
		check.NoError(t, commit.Encode(obj))

		hash, err := storer.SetEncodedObject(obj)
		check.NoError(t, err)

		hashes = append(hashes, hash)
	}

	check.NoError(t, storer.SetReference(plumbing.NewHashReference("refs/heads/main", hashes[length-1])))
	check.NoError(t, storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, "refs/heads/main")))

	return &Repository{repo: repo}, hashes
}
