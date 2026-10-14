// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package git

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/go-git/go-billy/v5"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage"

	"codefloe.com/itiquette/gommitlint/internal/adapters/gitrefs"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Repository errors.
var (
	ErrGitRefEmpty         = errors.New("git reference cannot be empty")
	ErrGitRefTooLong       = errors.New("git reference too long")
	ErrGitRefNullByte      = errors.New("git reference contains null byte")
	ErrGitRefInvalidChars  = errors.New("git reference contains unsupported metacharacters")
	ErrReferenceNotFound   = domain.ErrReferenceNotFound
	ErrRepositoryAccess    = domain.ErrRepositoryAccess
	ErrCommitCountExceeded = errors.New("git commit count exceeded maximum")
	ErrFoundReference      = errors.New("found reference")
	ErrNotGitRepository    = domain.ErrNotGitRepository
)

// ErrUnsupportedRepositoryFormat indicates a repository whose format
// extensions the embedded reader does not support, such as SHA-256 objects.
var ErrUnsupportedRepositoryFormat = errors.New("repository format not supported by the embedded reader")

// Repository implements the CommitRepository port.
type Repository struct {
	repo *gogit.Repository
}

// Compile-time interface compliance checks.
var (
	_ domain.Repository     = (*Repository)(nil)
	_ domain.CommitReader   = (*Repository)(nil)
	_ domain.BranchAnalyzer = (*Repository)(nil)
)

// validateGitRefSafety applies the shared reference input policy.
func validateGitRefSafety(ref string) error {
	return domain.DefaultSecurityPolicy().ValidateGitReference(ref)
}

func wrapRepositoryAccessError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %w", ErrRepositoryAccess, err)
	}

	return err
}

// wrapOp annotates a low-level git error with the operation name and the
// access-error wrapping policy. Use for any go-git call where a generic
// permission-related failure should be reportable as ErrRepositoryAccess.
func wrapOp(op string, err error) error {
	if err == nil {
		return nil
	}

	return fmt.Errorf("%s: %w", op, wrapRepositoryAccessError(err))
}

// RepositoryOption defines a configuration option for Repository.
type RepositoryOption func(*Repository)

// NewRepository opens a git repository with flexible storage backend.
// For filesystem repositories, pass filesystem storage and worktree.
// For in-memory repositories (tests), pass memory storage and memfs.
func NewRepository(storer storage.Storer, worktree billy.Filesystem, opts ...RepositoryOption) (*Repository, error) {
	repo, err := gogit.Open(storer, worktree)
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}

	repository := &Repository{
		repo: repo,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(repository)
	}

	return repository, nil
}

// NewRepositoryFromPath opens a git repository from a filesystem path.
func NewRepositoryFromPath(path string, opts ...RepositoryOption) (*Repository, error) {
	// Linked worktrees keep HEAD locally but share objects and branch refs
	// through commondir. Enable that layout when opening filesystem storage.
	goGitRepo, err := gogit.PlainOpenWithOptions(path, &gogit.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		if errors.Is(err, gogit.ErrRepositoryNotExists) {
			return nil, fmt.Errorf("%w: %s", ErrNotGitRepository, path)
		}

		if errors.Is(err, gogit.ErrUnsupportedExtensionRepositoryFormatVersion) || errors.Is(err, gogit.ErrUnknownExtension) {
			return nil, fmt.Errorf("open repository: %w: %w", ErrUnsupportedRepositoryFormat, err)
		}

		wrappedErr := wrapRepositoryAccessError(err)
		if errors.Is(wrappedErr, ErrRepositoryAccess) {
			return nil, fmt.Errorf("open repository: %w", wrappedErr)
		}

		return nil, fmt.Errorf("open repository: %w", err)
	}

	repository := &Repository{
		repo: goGitRepo,
	}

	// Apply functional options
	for _, opt := range opts {
		opt(repository)
	}

	return repository, nil
}

// EXPORTED METHODS (Repository interface implementations)

// GetCommit retrieves a single commit by hash or reference.
func (r Repository) GetCommit(ctx context.Context, ref string) (domain.Commit, error) {
	// Try to resolve as a reference first (handles HEAD, branch names, etc.)
	hash, err := r.resolveReference(ref)
	if err != nil {
		// If reference resolution fails, try as a direct hash
		hash = plumbing.NewHash(ref)
	}

	commit, err := r.repo.CommitObject(hash)
	if err != nil {
		// Normalise go-git's "object not found" into our domain-level
		// ErrReferenceNotFound so the CLI router can recognise it and emit
		// actionable suggestions (e.g. "git branch -a") instead of the
		// generic "validation failed" fallback. Same shape as resolveReference
		// already does at line 415.
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return domain.Commit{}, fmt.Errorf("%w: %s", ErrReferenceNotFound, ref)
		}

		return domain.Commit{}, fmt.Errorf("get commit: %w", err)
	}

	domainCommit := r.convertCommit(commit)

	return domainCommit, nil
}

// GetCommitRange retrieves commits in a range using two-dot semantics (from..to).
// Returns commits reachable from 'to' but not reachable from 'from'.
func (r Repository) GetCommitRange(ctx context.Context, fromRef, toRef string) ([]domain.Commit, error) {
	fromHash, toHash, err := r.resolveReferenceRange(ctx, fromRef, toRef)
	if err != nil {
		return nil, fmt.Errorf("resolve reference range %s..%s: %w", fromRef, toRef, err)
	}

	gitCommits, err := r.commitsInRange(ctx, fromHash, toHash, MaxCommitCount)
	if err != nil {
		return nil, fmt.Errorf("walk commit range %s..%s: %w", fromRef, toRef, err)
	}

	// Sort by committer time (oldest first) - reflects actual order after rebase
	slices.SortFunc(gitCommits, func(a, b *object.Commit) int {
		return a.Committer.When.Compare(b.Committer.When)
	})

	commits := make([]domain.Commit, len(gitCommits))
	for i, gc := range gitCommits {
		commits[i] = r.convertCommit(gc)
	}

	return commits, nil
}

// GetHeadCommits retrieves the latest N commits from HEAD.
func (r Repository) GetHeadCommits(ctx context.Context, count int) ([]domain.Commit, error) {
	ref, err := r.repo.Head()
	if err != nil {
		return nil, wrapOp("get HEAD", err)
	}

	iter, err := r.repo.Log(&gogit.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, wrapOp("create iterator", err)
	}
	defer iter.Close()

	commits := make([]domain.Commit, 0, count)
	collected := 0

	err = iter.ForEach(func(commit *object.Commit) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if collected >= count {
			return object.ErrCanceled
		}

		commits = append(commits, r.convertCommit(commit))
		collected++

		return nil
	})
	if err != nil && !errors.Is(err, object.ErrCanceled) {
		return nil, fmt.Errorf("iterate commits: %w", err)
	}

	return commits, nil
}

// BranchExists checks if a branch reference exists (local or remote).
func (r Repository) BranchExists(ctx context.Context, branchName string) (bool, error) {
	if err := validateGitRefSafety(branchName); err != nil {
		return false, fmt.Errorf("invalid git reference: %w", err)
	}

	for _, refName := range gitrefs.ReferenceCandidates(branchName) {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		_, err := r.repo.Reference(plumbing.ReferenceName(refName), true)
		if err == nil {
			return true, nil
		}

		if errors.Is(err, plumbing.ErrReferenceNotFound) || errors.Is(err, plumbing.ErrInvalidReferenceName) {
			continue
		}

		return false, fmt.Errorf("check git reference %s: %w", refName, err)
	}

	return false, nil
}

// GetCurrentBranch returns the name of the current branch.
func (r Repository) GetCurrentBranch(ctx context.Context) (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", wrapOp("get HEAD", err)
	}

	// Check if HEAD is detached
	if !head.Name().IsBranch() {
		return HeadRefName, nil // Return HEAD for detached state
	}

	// Extract branch name from refs/heads/branch-name
	return head.Name().Short(), nil
}

// GetCommitsAheadCount calculates how many commits the current branch is ahead of the reference.
func (r Repository) GetCommitsAheadCount(ctx context.Context, referenceBranch string) (int, error) {
	head, err := r.repo.Head()
	if err != nil {
		return 0, wrapOp("get HEAD", err)
	}

	// Find the reference branch hash
	refHash, found, err := r.findReferenceBranchHash(referenceBranch)
	if err != nil {
		return 0, err
	}

	if !found {
		return 0, fmt.Errorf("%w: %s", ErrReferenceNotFound, referenceBranch)
	}

	commits, err := r.commitsInRange(ctx, refHash, head.Hash(), MaxCommitCount)
	if err != nil {
		return 0, err
	}

	return len(commits), nil
}

// ListBranchNames lists local and remote branch names.
func (r Repository) ListBranchNames(ctx context.Context) ([]string, error) {
	return r.listReferenceNames(ctx, func(name plumbing.ReferenceName) (string, bool) {
		switch {
		case name.IsBranch():
			return name.Short(), true
		case name.IsRemote():
			short := name.Short()
			if short == "origin/HEAD" {
				return "", false
			}

			return short, true
		default:
			return "", false
		}
	})
}

// ListReferenceNames lists branch, tag, and HEAD references.
func (r Repository) ListReferenceNames(ctx context.Context) ([]string, error) {
	return r.listReferenceNames(ctx, func(name plumbing.ReferenceName) (string, bool) {
		switch {
		case name.IsBranch():
			return name.Short(), true
		case name.IsRemote():
			short := name.Short()
			if short == "origin/HEAD" {
				return "", false
			}

			return short, true
		case name.IsTag():
			return name.Short(), true
		case name == plumbing.HEAD:
			return HeadRefName, true
		default:
			return "", false
		}
	})
}

func (r Repository) listReferenceNames(ctx context.Context, selectName func(plumbing.ReferenceName) (string, bool)) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	iter, err := r.repo.References()
	if err != nil {
		return nil, wrapRepositoryAccessError(err)
	}
	defer iter.Close()

	unique := map[string]struct{}{}

	if err := iter.ForEach(func(ref *plumbing.Reference) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		name, ok := selectName(ref.Name())
		if !ok || name == "" {
			return nil
		}

		unique[name] = struct{}{}

		return nil
	}); err != nil {
		return nil, wrapRepositoryAccessError(err)
	}

	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}

	slices.Sort(result)

	return result, nil
}

// UNEXPORTED HELPER METHODS

// resolveReference resolves a reference (like HEAD, branch name, HEAD~1) to a commit hash.
// Supports Git revision syntax including relative references (HEAD~N, HEAD^N).
func (r Repository) resolveReference(ref string) (plumbing.Hash, error) {
	// Apply the same input policy as the CLI-backed adapter.
	if err := validateGitRefSafety(ref); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("invalid git reference: %w", err)
	}

	// Use ResolveRevision which handles all Git revision syntax:
	// - HEAD, branch names, tags
	// - Relative references: HEAD~1, HEAD^2, main~3
	// - Full and abbreviated commit hashes
	hash, err := r.repo.ResolveRevision(plumbing.Revision(ref))
	if err != nil {
		wrappedErr := wrapRepositoryAccessError(err)
		if errors.Is(wrappedErr, ErrRepositoryAccess) {
			return plumbing.ZeroHash, wrappedErr
		}

		return plumbing.ZeroHash, fmt.Errorf("%w: %s", ErrReferenceNotFound, ref)
	}

	return *hash, nil
}

// resolveReferenceRange resolves both from and to references to hashes and validates they exist.
func (r Repository) resolveReferenceRange(_ context.Context, fromRef, toRef string) (plumbing.Hash, plumbing.Hash, error) {
	fromHash := r.resolveReferenceWithFallback(fromRef, "from")
	toHash := r.resolveReferenceWithFallback(toRef, "to")

	if err := r.validateCommitExists(fromHash, "from"); err != nil {
		return plumbing.Hash{}, plumbing.Hash{}, err
	}

	if err := r.validateCommitExists(toHash, "to"); err != nil {
		return plumbing.Hash{}, plumbing.Hash{}, err
	}

	return fromHash, toHash, nil
}

// resolveReferenceWithFallback resolves a reference to hash, falling back to direct hash if needed.
func (r Repository) resolveReferenceWithFallback(ref, _ string) plumbing.Hash {
	hash, err := r.resolveReference(ref)
	if err != nil {
		hash = plumbing.NewHash(ref)
	}

	return hash
}

// validateCommitExists checks if a commit hash exists in the repository.
func (r Repository) validateCommitExists(hash plumbing.Hash, refType string) error {
	_, err := r.repo.CommitObject(hash)
	if err != nil {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			return fmt.Errorf("%w: %s", ErrReferenceNotFound, hash.String())
		}

		return wrapOp(fmt.Sprintf("failed to resolve '%s' reference", refType), err)
	}

	return nil
}

// MaxCommitCount bounds how many commit objects one range walk may load. The
// walk stops as soon as no remaining commit can belong to the range, so the
// bound applies to the requested range and its boundary, not to history depth.
const MaxCommitCount = 100000

// rangeNode is one loaded commit in a range walk.
type rangeNode struct {
	commit        *object.Commit
	uninteresting bool // reachable from the excluded side
	expanded      bool // parents already enqueued
}

// rangeQueue is a heap of range nodes, newest committer time first.
type rangeQueue []*rangeNode //nolint:recvcheck // container/heap's documented shape: value Len/Less/Swap, pointer Push/Pop.

func (q rangeQueue) Len() int { return len(q) }

func (q rangeQueue) Less(i, j int) bool {
	return q[i].commit.Committer.When.After(q[j].commit.Committer.When)
}

func (q rangeQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *rangeQueue) Push(x any) {
	if node, ok := x.(*rangeNode); ok {
		*q = append(*q, node)
	}
}

func (q *rangeQueue) Pop() any {
	old := *q
	node := old[len(old)-1]
	*q = old[:len(old)-1]

	return node
}

// commitsInRange returns the commits reachable from toHash but not from
// fromHash (git's from..to), loading at most limit commit objects.
//
// Like `git rev-list from..to`, it walks both sides newest first and marks
// everything reachable from fromHash as uninteresting. It stops once no
// interesting commit is queued and every queued commit is older than the
// oldest commit collected, because a parent is never newer than its child.
// Histories whose committer dates run backwards can therefore include a
// commit that a full ancestry walk would exclude, as git's own walk can.
func (r Repository) commitsInRange(ctx context.Context, fromHash, toHash plumbing.Hash, limit int) ([]*object.Commit, error) {
	walk := &rangeWalk{
		repo:  r.repo,
		limit: limit,
		nodes: make(map[plumbing.Hash]*rangeNode, InitialCommitMapCapacity),
	}

	if err := walk.enqueue(toHash, false); err != nil {
		return nil, err
	}

	if err := walk.enqueue(fromHash, true); err != nil {
		return nil, err
	}

	for walk.queue.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if walk.rangeComplete() {
			break
		}

		if err := walk.expandNext(); err != nil {
			return nil, err
		}
	}

	return walk.interestingCommits(), nil
}

// rangeWalk holds the state of one commitsInRange walk.
type rangeWalk struct {
	repo              *gogit.Repository
	limit             int
	nodes             map[plumbing.Hash]*rangeNode
	queue             rangeQueue
	interestingQueued int          // queued nodes not yet known to be uninteresting
	collected         []*rangeNode // expanded interesting nodes, in walk order
	oldest            time.Time    // oldest committer time among collected
}

// rangeComplete reports whether no queued commit can still belong to the
// range: nothing interesting is queued and every queued commit is older than
// the oldest one collected. The queue must not be empty.
func (w *rangeWalk) rangeComplete() bool {
	if w.interestingQueued != 0 {
		return false
	}

	return len(w.collected) == 0 || w.queue[0].commit.Committer.When.Before(w.oldest)
}

// expandNext pops the newest queued commit, collects it when it is still
// interesting, and enqueues its parents with its interest.
func (w *rangeWalk) expandNext() error {
	node, ok := heap.Pop(&w.queue).(*rangeNode)
	if !ok {
		return nil
	}

	node.expanded = true

	if !node.uninteresting {
		w.collect(node)
	}

	for _, parentHash := range node.commit.ParentHashes {
		if err := w.enqueue(parentHash, node.uninteresting); err != nil {
			return err
		}
	}

	return nil
}

// collect records an expanded interesting node and tracks the oldest
// committer time collected.
func (w *rangeWalk) collect(node *rangeNode) {
	w.interestingQueued--

	if len(w.collected) == 0 || node.commit.Committer.When.Before(w.oldest) {
		w.oldest = node.commit.Committer.When
	}

	w.collected = append(w.collected, node)
}

// enqueue loads and queues the commit for hash. A commit already seen is not
// loaded again; reaching it from the excluded side marks it uninteresting.
func (w *rangeWalk) enqueue(hash plumbing.Hash, uninteresting bool) error {
	if node, ok := w.nodes[hash]; ok {
		if uninteresting {
			w.markUninteresting(node)
		}

		return nil
	}

	if len(w.nodes) >= w.limit {
		return fmt.Errorf("%w: %d", ErrCommitCountExceeded, w.limit)
	}

	commit, err := w.repo.CommitObject(hash)
	if err != nil {
		return wrapOp("get commit object", err)
	}

	node := &rangeNode{commit: commit, uninteresting: uninteresting}
	w.nodes[hash] = node
	heap.Push(&w.queue, node)

	if !uninteresting {
		w.interestingQueued++
	}

	return nil
}

// markUninteresting marks node and its already loaded ancestors as reachable
// from the excluded side. A node still queued stops counting as interesting;
// an expanded node passes the mark on to its loaded parents.
func (w *rangeWalk) markUninteresting(node *rangeNode) {
	pending := []*rangeNode{node}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]

		if current.uninteresting {
			continue
		}

		current.uninteresting = true

		if !current.expanded {
			w.interestingQueued--

			continue
		}

		for _, parentHash := range current.commit.ParentHashes {
			if parent, ok := w.nodes[parentHash]; ok {
				pending = append(pending, parent)
			}
		}
	}
}

// interestingCommits returns the collected commits that stayed interesting,
// in walk order.
func (w *rangeWalk) interestingCommits() []*object.Commit {
	commits := make([]*object.Commit, 0, len(w.collected))

	for _, node := range w.collected {
		if !node.uninteresting {
			commits = append(commits, node.commit)
		}
	}

	return commits
}

// findReferenceBranchHash finds the hash of the reference branch using different formats.
func (r Repository) findReferenceBranchHash(referenceBranch string) (plumbing.Hash, bool, error) {
	if referenceBranch == "" {
		return plumbing.Hash{}, false, nil
	}

	for _, refName := range gitrefs.ReferenceCandidates(referenceBranch) {
		refCommit, err := r.repo.Reference(plumbing.ReferenceName(refName), true)
		if err == nil {
			return refCommit.Hash(), true, nil
		}

		wrappedErr := wrapRepositoryAccessError(err)
		if errors.Is(wrappedErr, ErrRepositoryAccess) {
			return plumbing.Hash{}, false, wrappedErr
		}
	}

	return plumbing.Hash{}, false, nil
}

// convertCommit converts go-git commit to domain commit.
func (r Repository) convertCommit(commit *object.Commit) domain.Commit {
	payload, err := commitSignaturePayload(commit)
	if err != nil {
		payload = nil
	}

	return domain.NewCommitWithSignaturePayload(
		commit.Hash.String(),
		commit.Message,
		commit.Author.Name,
		commit.Author.Email,
		commit.Committer.When.Format("2006-01-02T15:04:05Z"),
		commit.PGPSignature,
		len(commit.ParentHashes) > 1,
		payload,
	)
}

var errNilCommit = errors.New("commit is nil")

func commitSignaturePayload(commit *object.Commit) ([]byte, error) {
	if commit == nil {
		return nil, errNilCommit
	}

	commitCopy := *commit
	commitCopy.PGPSignature = ""

	obj := &plumbing.MemoryObject{}
	obj.SetType(plumbing.CommitObject)

	if err := commitCopy.Encode(obj); err != nil {
		return nil, err
	}

	reader, err := obj.Reader()
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = reader.Close()
	}()

	return io.ReadAll(reader)
}
