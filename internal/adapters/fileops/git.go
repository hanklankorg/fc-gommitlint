// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Git operations errors.
var (
	ErrRepositoryPathEmpty    = errors.New("repository path cannot be empty")
	ErrRepositoryPathNotExist = errors.New("repository path does not exist")
	ErrRepositoryPathNotDir   = errors.New("repository path is not a directory")
	ErrInvalidHookType        = errors.New("invalid hook type")
	ErrNotGitRepository       = errors.New("not a git repository (no .git directory found)")
	ErrInvalidGitConfig       = errors.New("invalid git repository (missing config file)")
	ErrInvalidGitStructure    = errors.New("invalid git repository structure")
	ErrInvalidGitFileFormat   = errors.New("invalid git file format")
	ErrInvalidGitSubmoduleRef = errors.New("invalid git submodule reference")
)

// GitOperations provides Git-specific file operations.
type GitOperations struct {
	fileOps *FileOperations
}

// NewGitOperations creates Git operations with file operations dependency.
func NewGitOperations(fileOps FileOperations) *GitOperations {
	return &GitOperations{fileOps: &fileOps}
}

// ValidateGitRepository validates a Git repository path and returns the canonical path.
func (ops *GitOperations) ValidateGitRepository(repoPath string) (string, error) {
	if repoPath == "" {
		return "", ErrRepositoryPathEmpty
	}

	// Validate path structure
	if err := domain.DefaultSecurityPolicy().ValidatePath(repoPath); err != nil {
		return "", fmt.Errorf("invalid repository path: %w", err)
	}

	// Clean the path first
	cleanPath := filepath.Clean(repoPath)

	// For testing with memory filesystems, we need to handle absolute paths differently
	// If the path is already absolute, use it; otherwise try to make it absolute
	absPath := cleanPath
	if !filepath.IsAbs(cleanPath) {
		// For real filesystems, convert to absolute
		// For memory filesystems in tests, the path should already be absolute
		if tempAbs, err := filepath.Abs(cleanPath); err == nil {
			absPath = tempAbs
		}
	}

	info, err := ops.fileOps.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s", ErrRepositoryPathNotExist, absPath)
		}

		return "", fmt.Errorf("invalid repository path: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrRepositoryPathNotDir, absPath)
	}

	// Validate Git directory
	gitDirPath := filepath.Join(absPath, ".git")
	if _, err := ops.verifyGitDirectory(gitDirPath); err != nil {
		return "", err
	}

	return absPath, nil
}

// ResolveGitDir returns the directory that holds the repository's config and
// hooks: ".git" itself, a submodule's gitdir, or a linked worktree's common
// directory.
func (ops *GitOperations) ResolveGitDir(repoPath string) (string, error) {
	canonPath, err := ops.ValidateGitRepository(repoPath)
	if err != nil {
		return "", fmt.Errorf("invalid repository: %w", err)
	}

	gitDir, err := ops.verifyGitDirectory(filepath.Join(canonPath, ".git"))
	if err != nil {
		return "", fmt.Errorf("invalid git directory: %w", err)
	}

	return gitDir, nil
}

// GetGitHookPath safely computes the path to a Git hook.
func (ops *GitOperations) GetGitHookPath(repoPath, hookType string) (string, error) {
	// Validate repository
	canonPath, err := ops.ValidateGitRepository(repoPath)
	if err != nil {
		return "", fmt.Errorf("invalid repository: %w", err)
	}

	// Validate hook type
	if !IsValidGitHookType(hookType) {
		return "", fmt.Errorf("%w: %s", ErrInvalidHookType, hookType)
	}

	// Get Git directory
	gitDirPath := filepath.Join(canonPath, ".git")

	gitDir, err := ops.verifyGitDirectory(gitDirPath)
	if err != nil {
		return "", fmt.Errorf("invalid git directory: %w", err)
	}

	hookPath, err := ops.fileOps.SafeJoin(gitDir, "hooks", hookType)
	if err != nil {
		return "", fmt.Errorf("invalid hook path: %w", err)
	}

	return hookPath, nil
}

// verifyGitDirectory verifies that a .git directory is valid.
func (ops *GitOperations) verifyGitDirectory(gitDirPath string) (string, error) {
	info, err := ops.fileOps.Stat(gitDirPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotGitRepository
		}

		return "", fmt.Errorf("error accessing .git directory: %w", err)
	}

	if info.IsDir() {
		configPath := filepath.Join(gitDirPath, "config")
		if _, err := ops.fileOps.Stat(configPath); err != nil {
			return "", ErrInvalidGitConfig
		}

		return gitDirPath, nil
	}

	if info.Mode().IsRegular() {
		return ops.handleGitFile(gitDirPath)
	}

	return "", ErrInvalidGitStructure
}

// handleGitFile processes a gitfile (used in submodules).
func (ops *GitOperations) handleGitFile(gitFilePath string) (string, error) {
	content, err := ops.fileOps.ReadFile(gitFilePath)
	if err != nil {
		return "", fmt.Errorf("error reading git file: %w", err)
	}

	gitfileContent := strings.TrimSpace(string(content))

	// Extract git directory path
	actualGitDir, found := strings.CutPrefix(gitfileContent, "gitdir: ")
	if !found {
		return "", ErrInvalidGitFileFormat
	}

	// Resolve relative paths
	if !filepath.IsAbs(actualGitDir) {
		repoRoot := filepath.Dir(gitFilePath)
		actualGitDir = filepath.Join(repoRoot, actualGitDir)
	}

	// Clean and validate path
	actualGitDir = filepath.Clean(actualGitDir)
	if err := domain.DefaultSecurityPolicy().ValidatePath(actualGitDir); err != nil {
		return "", fmt.Errorf("invalid git submodule path: %w", err)
	}

	// A linked worktree's git dir holds a "commondir" pointer: config and
	// hooks live in the common directory it names, not beside it.
	if commonDir, ok := ops.worktreeCommonDir(actualGitDir); ok {
		actualGitDir = commonDir
	}

	// Verify it's a valid Git directory
	configPath := filepath.Join(actualGitDir, "config")
	if _, err := ops.fileOps.Stat(configPath); err != nil {
		return "", ErrInvalidGitSubmoduleRef
	}

	return actualGitDir, nil
}

// worktreeCommonDir resolves the "commondir" file git writes into a linked
// worktree's git directory. It returns the cleaned, validated common directory,
// or false when gitDir is not a linked worktree.
func (ops *GitOperations) worktreeCommonDir(gitDir string) (string, bool) {
	content, err := ops.fileOps.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return "", false
	}

	commonDir := strings.TrimSpace(string(content))
	if commonDir == "" {
		return "", false
	}

	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}

	commonDir = filepath.Clean(commonDir)
	if err := domain.DefaultSecurityPolicy().ValidatePath(commonDir); err != nil {
		return "", false
	}

	return commonDir, true
}
