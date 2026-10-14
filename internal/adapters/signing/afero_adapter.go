// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// ErrNotImplementedInAfero indicates an operation that cannot be implemented
// with afero's in-memory filesystem due to Go type constraints.
// afero.File cannot be cast to *os.File, so methods requiring *os.File
// return this error. These methods are unused in test scenarios.
var ErrNotImplementedInAfero = errors.New("operation not supported in afero adapter: requires *os.File which afero cannot provide")

// AferoAdapter adapts an afero.Fs to the filesystem.FileSystem interface.
// This allows using afero filesystems (including in-memory) for testing.
type AferoAdapter struct {
	fs afero.Fs
}

// NewAferoAdapter creates a new adapter for the given afero filesystem.
func NewAferoAdapter(fs afero.Fs) *AferoAdapter {
	return &AferoAdapter{fs: fs}
}

// ReadFile reads and returns the contents of the named file.
func (a *AferoAdapter) ReadFile(path string) ([]byte, error) {
	return afero.ReadFile(a.fs, path)
}

// WriteFile writes data to the named file, creating it if necessary.
func (a *AferoAdapter) WriteFile(path string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(a.fs, path, data, perm)
}

// ReadDir reads the directory and returns a list of directory entries.
func (a *AferoAdapter) ReadDir(path string) ([]os.DirEntry, error) {
	infos, err := afero.ReadDir(a.fs, path)
	if err != nil {
		return nil, err
	}
	// Convert FileInfo to DirEntry
	entries := make([]os.DirEntry, len(infos))
	for i, info := range infos {
		entries[i] = dirEntry{info}
	}

	return entries, nil
}

// dirEntry wraps os.FileInfo to implement os.DirEntry.
type dirEntry struct {
	os.FileInfo
}

func (d dirEntry) Type() os.FileMode {
	return d.Mode().Type()
}

func (d dirEntry) Info() (os.FileInfo, error) {
	return d.FileInfo, nil
}

// Stat returns file info for the given path.
func (a *AferoAdapter) Stat(path string) (os.FileInfo, error) {
	return a.fs.Stat(path)
}

// Lstat avoids following symlinks when the wrapped filesystem supports afero.Lstater.
// Filesystems without that optional capability fall back to Stat.
func (a *AferoAdapter) Lstat(path string) (os.FileInfo, error) {
	if lstater, ok := a.fs.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)

		return info, err
	}

	return a.fs.Stat(path)
}

// Open opens the named file for reading.
func (a *AferoAdapter) Open(path string) (*os.File, error) {
	file, err := a.fs.Open(path)
	if err != nil {
		return nil, err
	}

	osFile, ok := file.(*os.File)
	if ok {
		return osFile, nil
	}

	if closeErr := file.Close(); closeErr != nil {
		return nil, fmt.Errorf("failed to close non-os file handle: %w", closeErr)
	}

	return nil, ErrNotImplementedInAfero
}

// CreateTemp creates a temporary file.
func (a *AferoAdapter) CreateTemp(dir, pattern string) (*os.File, error) {
	file, err := afero.TempFile(a.fs, dir, pattern)
	if err != nil {
		return nil, err
	}

	osFile, ok := file.(*os.File)
	if ok {
		return osFile, nil
	}

	if closeErr := file.Close(); closeErr != nil {
		return nil, fmt.Errorf("failed to close non-os file handle: %w", closeErr)
	}

	return nil, ErrNotImplementedInAfero
}

// Remove removes the named file or empty directory.
func (a *AferoAdapter) Remove(path string) error {
	return a.fs.Remove(path)
}

// Rename renames a file.
func (a *AferoAdapter) Rename(oldPath, newPath string) error {
	return a.fs.Rename(oldPath, newPath)
}

// Chmod changes the mode of the named file.
func (a *AferoAdapter) Chmod(path string, mode os.FileMode) error {
	return a.fs.Chmod(path, mode)
}

// MkdirAll creates a directory path and all parents that don't exist.
func (a *AferoAdapter) MkdirAll(path string, perm os.FileMode) error {
	return a.fs.MkdirAll(path, perm)
}

// ReadFileWithLimit returns file contents only when they fit within maxSize.
func (a *AferoAdapter) ReadFileWithLimit(path string, maxSize int64) ([]byte, error) {
	file, err := a.fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return filesystem.ReadWithLimit(file, maxSize)
}

// WriteFileAtomic replaces path with a same-directory temporary file rename.
func (a *AferoAdapter) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := a.fs.MkdirAll(dir, domain.FilePermPublicExec); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := afero.TempFile(a.fs, dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	success := false
	closed := false

	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}

		if !success {
			_ = a.fs.Remove(tmpPath)
		}
	}()

	if err := a.fs.Chmod(tmpPath, perm); err != nil {
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

	closed = true

	if err := a.fs.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	success = true

	return nil
}
