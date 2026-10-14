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
	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// CLI-specific validation errors.
var (
	ErrFilePathEmpty         = errors.New("file path cannot be empty")
	ErrMessageFileNotExist   = errors.New("message file does not exist")
	ErrMessageFileNotRegular = errors.New("message file is not a regular file")
	ErrOutputFileNotRegular  = errors.New("output file is not a regular file")
	// ErrInvalidRepoPath is the sentinel wrapper for any repo-path validation
	// failure. The CLI error router uses errors.Is to detect this and route to
	// repo-specific recovery suggestions instead of the (misleading) config
	// suggestions inherited from configLoadError.
	ErrInvalidRepoPath = errors.New("invalid repository path")
	// ErrRepoNotFound signals discoverRepoRootFrom walked to the filesystem
	// root without finding a `.git` entry. Wrapped (with the search-start
	// path) so callers can errors.Is-check the cause while still logging
	// the path that was tried.
	ErrRepoNotFound = errors.New("not a git repository")
)

// ValidateRepoPath resolves and validates a repository path.
//
// Mirrors git's discovery model:
//
//   - Empty input — walk up from the current working directory until a `.git`
//     entry is found. Used when no --repo-path flag was supplied.
//   - Non-empty input — anchor at the given path (must exist) and walk up
//     until a `.git` entry is found. Matches `git -C <path>` semantics where
//     the path picks the starting point but discovery still happens.
//
// Either way the returned path is the resolved repo root, so callers downstream
// (config loader, repository factory) don't re-implement discovery.
func ValidateRepoPath(repoPath string) (string, error) {
	if repoPath == "" {
		discovered, err := discoverRepoRootFromCWD()
		if err != nil {
			// Discovery is best-effort when the user gave us no path. Some
			// commands (validate --message-file -) don't need a repo at all,
			// and we shouldn't force one. Return empty and let downstream code
			// fall back to defaults / fail later if it really needs a repo.
			return "", nil //nolint:nilerr // intentional: empty-string + nil signals "no repo discovered, proceed without one".
		}

		return finalizeRepoPath(discovered)
	}

	// Virtual paths (in-memory repos, stdin) skip filesystem checks entirely.
	if git.IsVirtualPath(repoPath) {
		return repoPath, nil
	}

	if err := domain.DefaultSecurityPolicy().ValidatePath(repoPath); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRepoPath, err)
	}

	// Explicit input must point at an existing directory. A missing path or file
	// is a typo we should surface loudly; discovering upward from either could
	// land in the wrong repository and silently produce wrong results.
	info, err := os.Stat(repoPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRepoPath, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("%w: %w: %s", ErrInvalidRepoPath, fileops.ErrRepositoryPathNotDir, repoPath)
	}

	// Path exists. Walk up looking for a repo root. If discovery fails (no .git
	// up to filesystem root), fall back to empty: some validation targets
	// (--message-file -) legitimately don't need a repo, and downstream code
	// already handles the no-repo case by using config defaults. Targets that
	// actually need a repo will get their own clear error from that layer.
	discovered, err := discoverRepoRootFrom(repoPath)
	if err != nil {
		return "", nil //nolint:nilerr // intentional: same fall-back as above — downstream code handles the no-repo case.
	}

	return finalizeRepoPath(discovered)
}

func finalizeRepoPath(repoPath string) (string, error) {
	// Re-run gitOps validation on the discovered root: catches edge cases like
	// the path not actually being a directory we can open, missing permissions,
	// or .git being malformed even though it exists.
	fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
	gitOps := fileops.NewGitOperations(fileOps)

	validatedPath, err := gitOps.ValidateGitRepository(repoPath)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidRepoPath, err)
	}

	return validatedPath, nil
}

// discoverRepoRootFromCWD walks parent directories from the current working
// directory looking for a `.git` entry. Matches git's CLI behaviour: bare
// `git status` walks; so should bare `gommitlint validate`.
func discoverRepoRootFromCWD() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}

	return discoverRepoRootFrom(cwd)
}

// discoverRepoRootFrom walks the parent chain from start looking for the first
// ancestor that contains a `.git` entry. `.git` may be a directory (regular
// repo) or a file (worktree / submodule gitlink — both valid). Terminates at
// the filesystem root without infinite-looping.
func discoverRepoRootFrom(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	path := filepath.Clean(abs)
	for {
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			return "", fmt.Errorf("%w (searched from %q upward)", ErrRepoNotFound, start)
		}

		path = parent
	}
}

// ValidateOutputFilePath validates a file path for output using domain security policy.
func ValidateOutputFilePath(filePath string) (string, error) {
	validatedPath, err := validateFilePath(filePath, "output")
	if err != nil {
		return "", err
	}

	if err := validateOutputParentComponents(filePath, validatedPath); err != nil {
		return "", err
	}

	info, err := os.Lstat(validatedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return validatedPath, nil
		}

		return "", fmt.Errorf("cannot inspect output file path: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s", fileops.ErrSymlinkNotAllowed, validatedPath)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrOutputFileNotRegular, validatedPath)
	}

	return validatedPath, nil
}

func validateOutputParentComponents(filePath, validatedPath string) error {
	stop := ""

	if !filepath.IsAbs(filePath) {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine output path base: %w", err)
		}

		stop = filepath.Clean(cwd)
	}

	for path := filepath.Dir(validatedPath); path != stop; path = filepath.Dir(path) {
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			return fmt.Errorf("cannot inspect output path component %s: %w", path, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", fileops.ErrSymlinkNotAllowed, path)
		}

		if filepath.Dir(path) == path {
			return nil
		}
	}

	return nil
}

// ValidateMessageFilePath validates a message file path for reading using domain security policy.
func ValidateMessageFilePath(filePath string) (string, error) {
	if filePath == "" {
		return "", ErrFilePathEmpty
	}

	// Special case: "-" means stdin, no validation needed
	if filePath == "-" {
		return "-", nil
	}

	absPath, err := validateFilePath(filePath, "message")
	if err != nil {
		return "", err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrMessageFileNotExist
		}

		return "", fmt.Errorf("cannot access message file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s", ErrMessageFileNotRegular, absPath)
	}

	return absPath, nil
}

func validateFilePath(filePath string, kind string) (string, error) {
	if filePath == "" {
		return "", ErrFilePathEmpty
	}

	if err := domain.DefaultSecurityPolicy().ValidatePath(filePath); err != nil {
		return "", fmt.Errorf("invalid %s file path: %w", kind, err)
	}

	absPath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("cannot resolve absolute path: %w", err)
	}

	return absPath, nil
}

// ValidateGitReference validates git references using domain security policy.
func ValidateGitReference(ref string) error {
	return domain.DefaultSecurityPolicy().ValidateGitReference(ref)
}

// ParseSecureCommitRange parses a commit range and validates both refs.
func ParseSecureCommitRange(commitRange string) (domain.CommitRange, error) {
	return domain.DefaultSecurityPolicy().ParseSecureCommitRange(commitRange)
}
