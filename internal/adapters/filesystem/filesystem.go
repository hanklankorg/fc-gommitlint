// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package filesystem

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// File system errors.
var (
	ErrFileTooLarge = errors.New("file too large")
)

// ReadWithLimit returns at most maxSize bytes and reports ErrFileTooLarge when
// the maxSize+1 sentinel byte is available.
func ReadWithLimit(reader io.Reader, maxSize int64) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		return nil, err
	}

	if int64(len(content)) > maxSize {
		return nil, fmt.Errorf("%w: exceeds max %d bytes", ErrFileTooLarge, maxSize)
	}

	return content, nil
}

// FileSystem defines operations for file system access.
// Enables dependency injection and testing.
//
//nolint:interfacebloat // This interface groups related filesystem operations for security
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	Lstat(path string) (os.FileInfo, error)
	Open(path string) (*os.File, error)
	CreateTemp(dir, pattern string) (*os.File, error)
	Remove(path string) error
	Rename(oldPath, newPath string) error
	Chmod(path string, mode os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	ReadFileWithLimit(path string, maxSize int64) ([]byte, error)
	WriteFileAtomic(path string, data []byte, perm os.FileMode) error
}

// OSFileSystem implements FileSystem using the standard library.
type OSFileSystem struct{}

// NewOSFileSystem creates a new OS-based file system.
func NewOSFileSystem() OSFileSystem {
	return OSFileSystem{}
}

// ReadFile reads and returns the contents of the named file.
func (fs OSFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path) // #nosec G304 - callers intentionally select the path to read
}

// WriteFile writes data to the named file with the specified permissions.
func (fs OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// ReadDir reads the named directory and returns all its entries.
func (fs OSFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// Stat returns file information for the named file.
func (fs OSFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// Lstat returns file information for the named file, without following symbolic links.
func (fs OSFileSystem) Lstat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

// Open opens the named file for reading.
func (fs OSFileSystem) Open(path string) (*os.File, error) {
	return os.Open(path) // #nosec G304 - callers intentionally select the path to open
}

// CreateTemp creates a new temporary file in the directory dir with a name beginning with pattern.
func (fs OSFileSystem) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// Remove removes the named file or directory.
func (fs OSFileSystem) Remove(path string) error {
	return os.Remove(path)
}

// Rename renames (moves) oldPath to newPath.
func (fs OSFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

// Chmod changes the mode of the named file to mode.
func (fs OSFileSystem) Chmod(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (fs OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// ReadFileWithLimit returns file contents only when they fit within maxSize.
func (fs OSFileSystem) ReadFileWithLimit(path string, maxSize int64) ([]byte, error) {
	file, err := os.Open(path) // #nosec G304 - callers intentionally select the path to read
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return ReadWithLimit(file, maxSize)
}

// WriteFileAtomic writes data atomically using temp file and rename.
func (fs OSFileSystem) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	success := false

	defer func() {
		_ = tmpFile.Close()

		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync data: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	success = true

	return nil
}
