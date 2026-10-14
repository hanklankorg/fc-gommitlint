// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/gitrefs"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Repository provides a git CLI-backed repository adapter.
type Repository struct {
	CommitReader
}

var _ domain.Repository = (*Repository)(nil)

// NewRepository creates a CLI-backed repository adapter.
func NewRepository(repoPath string) *Repository {
	return &Repository{repoPath: repoPath}
}

// ListBranchNames lists local and remote branch names.
func (r Repository) ListBranchNames(ctx context.Context) ([]string, error) {
	return r.listReferenceNames(ctx, false, "refs/heads", "refs/remotes")
}

// ListReferenceNames lists branch, tag, and HEAD references.
func (r Repository) ListReferenceNames(ctx context.Context) ([]string, error) {
	return r.listReferenceNames(ctx, true, "refs/heads", "refs/remotes", "refs/tags")
}

// GetCommitsAheadCount returns how many commits HEAD is ahead of the reference.
func (r Repository) GetCommitsAheadCount(ctx context.Context, referenceBranch string) (int, error) {
	if err := validateGitRefSafety(referenceBranch); err != nil {
		return 0, fmt.Errorf("invalid git reference: %w", err)
	}

	// Count against the ref this name resolves to, not against the name. git's
	// own revision rules read refs/tags before refs/heads, so handing git the
	// shorthand would count from a tag that shares the branch's name.
	resolved, found, err := r.resolveReferenceName(ctx, referenceBranch)
	if err != nil {
		return 0, err
	}

	if !found {
		return 0, fmt.Errorf("%w: %s", domain.ErrReferenceNotFound, referenceBranch)
	}

	output, err := runGitCommand(ctx, r.repoPath, "rev-list", "--count", gitEndOfOptions, resolved+"..HEAD")
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("parse ahead count: %w", err)
	}

	return count, nil
}

// GetCurrentBranch returns the current branch name.
func (r Repository) GetCurrentBranch(ctx context.Context) (string, error) {
	output, err := runGitCommand(ctx, r.repoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// BranchExists checks if a branch reference exists locally or remotely.
func (r Repository) BranchExists(ctx context.Context, branchName string) (bool, error) {
	if err := validateGitRefSafety(branchName); err != nil {
		return false, fmt.Errorf("invalid git reference: %w", err)
	}

	_, found, err := r.resolveReferenceName(ctx, branchName)

	return found, err
}

// resolveReferenceName returns the fully qualified ref a user-provided name
// resolves to, using the documented candidate priority shared with the go-git
// adapter.
func (r Repository) resolveReferenceName(ctx context.Context, name string) (string, bool, error) {
	for _, candidate := range gitrefs.ReferenceCandidates(name) {
		exists, err := runGitCommandQuiet(ctx, r.repoPath, "show-ref", "--verify", "--quiet", gitEndOfOptions, candidate)
		if err != nil {
			return "", false, err
		}

		if exists {
			return candidate, true, nil
		}
	}

	return "", false, nil
}

func (r Repository) listReferenceNames(ctx context.Context, includeHead bool, patterns ...string) ([]string, error) {
	args := append([]string{"for-each-ref", "--format=%(refname:short)"}, patterns...)

	output, err := runGitCommand(ctx, r.repoPath, args...)
	if err != nil {
		return nil, err
	}

	unique := make(map[string]struct{})
	if includeHead {
		unique["HEAD"] = struct{}{}
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || name == "origin/HEAD" {
			continue
		}

		unique[name] = struct{}{}
	}

	result := make([]string, 0, len(unique))
	for name := range unique {
		result = append(result, name)
	}

	slices.Sort(result)

	return result, nil
}
