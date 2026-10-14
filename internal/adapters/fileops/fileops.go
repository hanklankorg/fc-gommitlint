// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// MaxCheckedFileSize bounds content inspected by checked file mutations.
const MaxCheckedFileSize int64 = 1024 * 1024

// File system errors.
var (
	ErrSymlinkNotAllowed     = errors.New("refusing to write to symlink")
	ErrPathEscapesBase       = errors.New("path escapes base directory")
	ErrLockAcquisitionFailed = errors.New("failed to acquire lock")
	ErrCheckFailed           = errors.New("pre-write check failed")
	ErrNotRegularFile        = errors.New("refusing to modify non-regular file")
)

// FileOperations provides secure file operations using dependency injection.
type FileOperations struct {
	fs     FileSystem
	locker fileLocker
}

// FileSystem provides the file operations needed by FileOperations.
type FileSystem interface {
	Lstat(path string) (os.FileInfo, error)
	ReadDir(path string) ([]os.DirEntry, error)
	ReadFile(path string) ([]byte, error)
	ReadFileWithLimit(path string, maxSize int64) ([]byte, error)
	Remove(path string) error
	Stat(path string) (os.FileInfo, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
}

// NewFileOperations creates file operations with the given file system.
func NewFileOperations(fs FileSystem) FileOperations {
	return newFileOperationsWithLocker(fs, osFileLocker{})
}

func newFileOperationsWithLocker(fs FileSystem, locker fileLocker) FileOperations {
	if locker == nil {
		locker = osFileLocker{}
	}

	return FileOperations{fs: fs, locker: locker}
}

// FindFilesWithExtensions finds files with specified extensions.
func (ops FileOperations) FindFilesWithExtensions(dir string, extensions []string) ([]string, error) {
	// Validate directory path
	if err := domain.DefaultSecurityPolicy().ValidatePath(dir); err != nil {
		return nil, fmt.Errorf("invalid directory: %w", err)
	}

	entries, err := ops.fs.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	extMap := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		extMap[ext] = true
	}

	var files []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := filepath.Ext(entry.Name())
		if extMap[ext] {
			// Safe path joining - OS already validates directory entry names
			filePath := filepath.Join(dir, entry.Name())

			// Validate final joined path using domain policy
			if err := domain.DefaultSecurityPolicy().ValidatePath(filePath); err != nil {
				continue // Skip invalid paths
			}

			files = append(files, filePath)
		}
	}

	return files, nil
}

// ReadDir reads a directory after validating the path.
func (ops FileOperations) ReadDir(path string) ([]os.DirEntry, error) {
	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return nil, fmt.Errorf("invalid directory: %w", err)
	}

	return ops.fs.ReadDir(path)
}

// SafeJoin safely joins path elements with validation.
func (ops FileOperations) SafeJoin(baseDir string, elements ...string) (string, error) {
	// Validate base directory
	if err := domain.DefaultSecurityPolicy().ValidatePath(baseDir); err != nil {
		return "", fmt.Errorf("invalid base directory: %w", err)
	}

	// Elements from filepath operations are already safe - just validate final result

	// Join paths
	result := baseDir

	for _, element := range elements {
		if element == "" {
			continue
		}

		result = filepath.Join(result, element)
	}

	// Final validation
	if !IsWithinPath(filepath.Join(elements...), baseDir) {
		return "", ErrPathEscapesBase
	}

	return result, nil
}

// GetFileInfo returns file information.
func (ops FileOperations) GetFileInfo(path string) (os.FileInfo, error) {
	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	return ops.fs.Stat(path)
}

// ReadFile reads a file after basic path validation.
func (ops FileOperations) ReadFile(path string) ([]byte, error) {
	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	return ops.fs.ReadFile(path)
}

// Stat returns file information after validating the path.
func (ops FileOperations) Stat(path string) (os.FileInfo, error) {
	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	return ops.fs.Stat(path)
}

// CheckFunc is a function that validates file content before overwriting.
// It receives the current file content (nil if file doesn't exist).
// Returns nil to allow the write, or an error to abort.
type CheckFunc func(existingContent []byte) error

// SafeWriteFileWithCheck writes a file atomically after passing a check function.
// The lock serializes cooperating Gommitlint writers around the check and write;
// it does not exclude unrelated processes from modifying the target.
// The check function receives current file content (nil if file doesn't exist).
func (ops FileOperations) SafeWriteFileWithCheck(path string, data []byte, perm os.FileMode, check CheckFunc) error {
	if err := ops.validateWritePath(path); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	lockPath := filepath.Join(dir, ".gommitlint.lock")

	locker := ops.locker
	if locker == nil {
		locker = osFileLocker{}
	}

	return locker.WithLock(lockPath, func() error {
		existingContent, err := ops.fs.ReadFileWithLimit(path, MaxCheckedFileSize)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to read existing file: %w", err)
		}

		if check != nil {
			if err := check(existingContent); err != nil {
				return fmt.Errorf("%w: %w", ErrCheckFailed, err)
			}
		}

		return ops.fs.WriteFileAtomic(path, data, perm)
	})
}

// SafeRemoveFileWithCheck removes a file after checking its current content
// while holding the same cooperative lock used by checked writes.
func (ops FileOperations) SafeRemoveFileWithCheck(path string, check CheckFunc) error {
	if err := ops.validateWritePath(path); err != nil {
		return err
	}

	lockPath := filepath.Join(filepath.Dir(path), ".gommitlint.lock")

	locker := ops.locker
	if locker == nil {
		locker = osFileLocker{}
	}

	return locker.WithLock(lockPath, func() error {
		existingContent, err := ops.fs.ReadFileWithLimit(path, MaxCheckedFileSize)
		if err != nil {
			return fmt.Errorf("failed to read existing file: %w", err)
		}

		if check != nil {
			if err := check(existingContent); err != nil {
				return fmt.Errorf("%w: %w", ErrCheckFailed, err)
			}
		}

		return ops.fs.Remove(path)
	})
}

type fileLocker interface {
	WithLock(lockPath string, operation func() error) error
}

type osFileLocker struct{}

func (osFileLocker) WithLock(lockPath string, operation func() error) error {
	// #nosec G304 - lockPath is constructed from a validated target directory.
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: another installation may be in progress", ErrLockAcquisitionFailed)
		}

		return fmt.Errorf("%w: %w", ErrLockAcquisitionFailed, err)
	}

	defer func() {
		_ = lockFile.Close()
		_ = os.Remove(lockPath)
	}()

	return operation()
}

// validateWritePath validates path and checks for security issues.
func (ops FileOperations) validateWritePath(path string) error {
	// Validate path first
	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	info, err := ops.fs.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("inspect target path: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrSymlinkNotAllowed, path)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}

	return nil
}
