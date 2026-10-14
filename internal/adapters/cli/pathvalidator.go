// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Path validation errors.
var (
	ErrInvalidHookType  = fileops.ErrInvalidHookType
	ErrInvalidHooksPath = errors.New("hooks path exists but is not a directory")
	ErrPathOutsideRepo  = errors.New("path is outside repository boundaries")
)

// PathValidator provides path validation with injected filesystem.
type PathValidator struct {
	fs     filesystem.FileSystem
	gitOps *fileops.GitOperations
}

// NewPathValidator creates a new PathValidator with the given filesystem.
func NewPathValidator(fs filesystem.FileSystem) *PathValidator {
	fileOps := fileops.NewFileOperations(fs)
	gitOps := fileops.NewGitOperations(fileOps)

	return &PathValidator{
		fs:     fs,
		gitOps: gitOps,
	}
}

// ValidateHookPath validates and returns the hook file path.
func (p *PathValidator) ValidateHookPath(repoPath, hookType string) (string, error) {
	// Restrict hook names before path construction.
	if !fileops.IsValidGitHookType(hookType) {
		return "", fmt.Errorf("%w: %s", ErrInvalidHookType, hookType)
	}

	validatedRepo, err := p.gitOps.ValidateGitRepository(repoPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRepoPath, err)
	}

	hookPath, err := p.gitOps.GetGitHookPath(validatedRepo, hookType)
	if err != nil {
		return "", err
	}

	// The hook must live under the repository or under its git directory,
	// which for a linked worktree is the common directory outside the tree.
	if err := p.ValidateHookLocation(validatedRepo, hookPath); err != nil {
		return "", err
	}

	return hookPath, nil
}

// ValidateHookLocation accepts a hook path inside the repository or inside the
// repository's resolved git directory (submodule gitdir, worktree common dir).
func (p *PathValidator) ValidateHookLocation(repoPath, hookPath string) error {
	if err := p.ValidatePathWithinRepo(repoPath, hookPath); err == nil {
		return nil
	}

	gitDir, err := p.gitOps.ResolveGitDir(repoPath)
	if err != nil {
		return fmt.Errorf("%w: %s is not within %s", ErrPathOutsideRepo, hookPath, repoPath)
	}

	if err := p.validatePathWithinBase(gitDir, hookPath); err != nil {
		return fmt.Errorf("%w: %s is not within %s or %s: %w", ErrPathOutsideRepo, hookPath, repoPath, gitDir, err)
	}

	return nil
}

// ValidatePathWithinRepo ensures a path is within the repository boundaries.
// This prevents path traversal attacks.
func (p *PathValidator) ValidatePathWithinRepo(repoPath, targetPath string) error {
	if err := p.validatePathWithinBase(repoPath, targetPath); err != nil {
		return fmt.Errorf("%w: %s is not within %s: %w", ErrPathOutsideRepo, targetPath, repoPath, err)
	}

	return nil
}

// EnsureHooksDirectory creates the hooks directory if it doesn't exist.
func (p *PathValidator) EnsureHooksDirectory(repoPath string) error {
	gitDir, err := p.gitOps.ResolveGitDir(repoPath)
	if err != nil {
		return fmt.Errorf("invalid repository path: %w", err)
	}

	hooksDir := filepath.Join(gitDir, "hooks")

	stat, err := p.fs.Lstat(hooksDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Create hooks directory with appropriate permissions
			return p.fs.MkdirAll(hooksDir, 0o755)
		}

		return fmt.Errorf("cannot access hooks directory: %w", err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("%w: %s", ErrInvalidHooksPath, hooksDir)
	}

	return nil
}

func (p *PathValidator) validatePathWithinBase(basePath, targetPath string) error {
	if err := domain.DefaultSecurityPolicy().ValidatePathWithinBase(basePath, targetPath); err != nil {
		return err
	}

	absBase, err := filepath.Abs(filepath.Clean(basePath))
	if err != nil {
		return fmt.Errorf("resolve base path: %w", err)
	}

	absTarget, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}

	for path := absTarget; path != absBase; path = filepath.Dir(path) {
		info, statErr := p.fs.Lstat(path)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				continue
			}

			return fmt.Errorf("inspect path component %s: %w", path, statErr)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", fileops.ErrSymlinkNotAllowed, path)
		}
	}

	return nil
}
