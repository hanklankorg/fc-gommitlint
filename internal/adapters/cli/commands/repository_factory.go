// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	repoAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/repository"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/urfave/cli/v3"
)

const virtualRepositoryFactoryMetadataKey = "gommitlint.virtualRepositoryFactory"

// SetVirtualRepositoryFactory configures cmd to resolve virtual repository paths.
func SetVirtualRepositoryFactory(cmd *cli.Command, factory git.RepositoryFactory) {
	if cmd == nil {
		return
	}

	if cmd.Metadata == nil {
		cmd.Metadata = map[string]any{}
	}

	if factory == nil {
		delete(cmd.Metadata, virtualRepositoryFactoryMetadataKey)

		return
	}

	cmd.Metadata[virtualRepositoryFactoryMetadataKey] = factory
}

// VirtualRepositoryFactory returns the configured virtual repository factory for cmd.
func VirtualRepositoryFactory(cmd *cli.Command) git.RepositoryFactory {
	if cmd == nil {
		return nil
	}

	for _, current := range cmd.Lineage() {
		factory, ok := current.Metadata[virtualRepositoryFactoryMetadataKey].(git.RepositoryFactory)
		if ok {
			return factory
		}
	}

	return nil
}

// createRepositoryAdapter selects the repository backend based on repository capabilities.
// SHA-1 repositories use the go-git adapter, while SHA-256 repositories use the git CLI-backed adapter.
func createRepositoryAdapter(ctx context.Context, cmd *cli.Command, logger domain.Logger, localizer output.Localizer) (domain.Repository, error) {
	repoPath := GetRepoPath(cmd)

	if !git.IsVirtualPath(repoPath) {
		validatedRepoPath, err := cliAdapter.ValidateRepoPath(repoPath)
		if err != nil {
			return nil, invalidRepositoryError(err, localizer)
		}

		repoPath = validatedRepoPath
	}

	return repoAdapter.NewOpener(logger, VirtualRepositoryFactory(cmd)).Open(ctx, repoPath)
}
