//
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"context"
	"errors"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	repoAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/repository"
	"github.com/urfave/cli/v3"
)

var errCompletionRepositoryUnsupported = errors.New("completion repository does not support reference listing")

type completionReferenceLister interface {
	ListBranchNames(ctx context.Context) ([]string, error)
	ListReferenceNames(ctx context.Context) ([]string, error)
}

func completionGitReferences(ctx context.Context, cmd *cli.Command) []string {
	repo, err := completionRepository(ctx, cmd)
	if err != nil {
		return nil
	}

	refs, err := repo.ListReferenceNames(ctx)
	if err != nil {
		return nil
	}

	return refs
}

func completionGitBranches(ctx context.Context, cmd *cli.Command) []string {
	repo, err := completionRepository(ctx, cmd)
	if err != nil {
		return nil
	}

	branches, err := repo.ListBranchNames(ctx)
	if err != nil {
		return nil
	}

	return branches
}

func completionGitRanges(ctx context.Context, cmd *cli.Command) []string {
	refs := completionGitReferences(ctx, cmd)
	if len(refs) == 0 {
		return nil
	}

	ranges := make([]string, 0, len(refs))
	for _, ref := range refs {
		ranges = append(ranges, ref+"..")
	}

	return ranges
}

func completionRepository(ctx context.Context, cmd *cli.Command) (completionReferenceLister, error) {
	repoPath := commands.GetRepoPath(cmd)

	validatedRepoPath, err := cliadapter.ValidateRepoPath(repoPath)
	if err != nil {
		return nil, err
	}

	if validatedRepoPath == "" {
		return nil, errCompletionRepositoryUnsupported
	}

	repo, err := repoAdapter.NewOpener(nil, commands.VirtualRepositoryFactory(cmd)).Open(ctx, validatedRepoPath)
	if err != nil {
		return nil, err
	}

	lister, ok := repo.(completionReferenceLister)
	if !ok {
		return nil, errCompletionRepositoryUnsupported
	}

	return lister, nil
}
