// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package repository selects repository adapter implementations.
package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/adapters/gitcli"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// ErrVirtualFactoryRequired indicates a virtual repository path was opened without a virtual factory.
var ErrVirtualFactoryRequired = errors.New("virtual repository factory required")

// ErrSystemGitRequired indicates a repository the embedded reader cannot open
// (a SHA-256 repository, for example) where system git could not be used
// either: it is not installed, or it refused the repository.
var ErrSystemGitRequired = errors.New("this repository can only be read with system git, which failed")

const (
	// ObjectFormatSHA1 identifies SHA-1 Git object repositories.
	ObjectFormatSHA1 = "sha1"
	// ObjectFormatSHA256 identifies SHA-256 Git object repositories.
	ObjectFormatSHA256 = "sha256"
)

// Opener selects the repository adapter for a validated repository path.
type Opener struct {
	DetectObjectFormat ObjectFormatDetector
	Logger             domain.Logger
	OpenSHA1           OpenFunc
	OpenSHA256         OpenFunc
	VirtualFactory     git.RepositoryFactory
}

// ObjectFormatDetector detects the Git object format for a repository path.
type ObjectFormatDetector func(context.Context, string) (string, error)

// OpenFunc opens a repository path with a concrete adapter.
type OpenFunc func(string) (domain.Repository, error)

// NewOpener creates a repository opener with explicit dependencies.
//
// The adapter dependencies (DetectObjectFormat/OpenSHA1/OpenSHA256) are left
// unset here; Open applies them via withDefaults, keeping a single defaulting
// site.
func NewOpener(logger domain.Logger, virtualFactory git.RepositoryFactory) Opener {
	return Opener{
		Logger:         logger,
		VirtualFactory: virtualFactory,
	}
}

// Open opens repoPath with the adapter matching repository capabilities.
func (o Opener) Open(ctx context.Context, repoPath string) (domain.Repository, error) {
	o = o.withDefaults()

	if git.IsVirtualPath(repoPath) {
		if o.VirtualFactory == nil {
			return nil, ErrVirtualFactoryRequired
		}

		return o.VirtualFactory.CreateRepository(repoPath)
	}

	format, err := o.DetectObjectFormat(ctx, repoPath)
	if err != nil {
		return o.openWithoutDetection(repoPath, err)
	}

	if strings.EqualFold(format, ObjectFormatSHA256) {
		if o.Logger != nil {
			o.Logger.Debug("Using git CLI repository adapter for SHA-256 repository")
		}

		return o.OpenSHA256(repoPath)
	}

	return o.OpenSHA1(repoPath)
}

// openWithoutDetection falls back to the embedded reader when the object format
// could not be detected. The fallback opens every ordinary SHA-1 repository
// without git. When it fails on a format it does not support, the repository
// needed git and the detection error is the actual cause: git is not installed,
// or it refused the repository (an owner it does not trust, for example). The
// error then leads with that cause instead of an internal format-extension name.
func (o Opener) openWithoutDetection(repoPath string, detectErr error) (domain.Repository, error) {
	repo, err := o.OpenSHA1(repoPath)
	if err != nil && errors.Is(err, git.ErrUnsupportedRepositoryFormat) {
		return nil, fmt.Errorf("%w: %w (%w)", ErrSystemGitRequired, detectErr, err)
	}

	return repo, err
}

func (o Opener) withDefaults() Opener {
	if o.DetectObjectFormat == nil {
		o.DetectObjectFormat = gitcli.DetectObjectFormat
	}

	if o.OpenSHA1 == nil {
		o.OpenSHA1 = openGoGitRepository
	}

	if o.OpenSHA256 == nil {
		o.OpenSHA256 = openGitCLIRepository
	}

	return o
}

func openGoGitRepository(repoPath string) (domain.Repository, error) {
	return git.NewRepositoryFromPath(repoPath)
}

func openGitCLIRepository(repoPath string) (domain.Repository, error) {
	return gitcli.NewRepository(repoPath), nil
}
