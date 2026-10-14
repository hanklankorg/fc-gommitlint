// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
)

// Test error constants for err113 compliance.
var (
	ErrNotImplementedInMock = errors.New("not implemented in mock")
)

// MockFileSystem implements filesystem.FileSystem for testing.
type MockFileSystem struct {
	files       map[string][]byte
	dirs        map[string][]os.DirEntry
	permissions map[string]os.FileMode
	shouldError map[string]error
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files:       make(map[string][]byte),
		dirs:        make(map[string][]os.DirEntry),
		permissions: make(map[string]os.FileMode),
		shouldError: make(map[string]error),
	}
}

func (m *MockFileSystem) ReadFile(path string) ([]byte, error) {
	if err := m.shouldError[path]; err != nil {
		return nil, err
	}

	if data, exists := m.files[path]; exists {
		return data, nil
	}

	return nil, os.ErrNotExist
}

func (m *MockFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := m.shouldError[path]; err != nil {
		return err
	}

	m.files[path] = data
	m.permissions[path] = perm

	return nil
}

func (m *MockFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	if err := m.shouldError[path]; err != nil {
		return nil, err
	}

	if entries, exists := m.dirs[path]; exists {
		return entries, nil
	}

	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Stat(path string) (os.FileInfo, error) {
	if err := m.shouldError[path]; err != nil {
		return nil, err
	}
	// Return mock file info with permissions if set
	mode := m.permissions[path]

	return &mockFileInfo{name: filepath.Base(path), isDir: m.dirs[path] != nil, mode: mode}, nil
}

func (m *MockFileSystem) Lstat(path string) (os.FileInfo, error) {
	// For mock purposes, Lstat behaves the same as Stat
	return m.Stat(path)
}

func (m *MockFileSystem) Open(_ string) (*os.File, error) {
	return nil, ErrNotImplementedInMock
}

func (m *MockFileSystem) CreateTemp(_, _ string) (*os.File, error) {
	return nil, ErrNotImplementedInMock
}

func (m *MockFileSystem) Remove(path string) error {
	delete(m.files, path)

	return nil
}

func (m *MockFileSystem) Rename(oldPath, newPath string) error {
	if data, exists := m.files[oldPath]; exists {
		m.files[newPath] = data
		delete(m.files, oldPath)
	}

	return nil
}

func (m *MockFileSystem) Chmod(path string, mode os.FileMode) error {
	m.permissions[path] = mode

	return nil
}

func (m *MockFileSystem) MkdirAll(path string, _ os.FileMode) error {
	m.dirs[path] = []os.DirEntry{}

	return nil
}

func (m *MockFileSystem) ReadFileWithLimit(path string, maxSize int64) ([]byte, error) {
	if err, exists := m.shouldError[path]; exists && err != nil {
		return nil, err
	}

	content, exists := m.files[path]
	if !exists {
		return nil, os.ErrNotExist
	}

	if int64(len(content)) > maxSize {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", filesystem.ErrFileTooLarge, len(content), maxSize)
	}

	return content, nil
}

func (m *MockFileSystem) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	// For mock, just write directly
	m.files[path] = data
	m.permissions[path] = perm

	return nil
}

func (m *MockFileSystem) AddFile(path string, content []byte) {
	m.files[path] = content
}

func (m *MockFileSystem) AddDir(path string, entries []os.DirEntry) {
	m.dirs[path] = entries
}

func (m *MockFileSystem) AddDirWithEntries(path string, entries []mockDirEntry) {
	dirEntries := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		dirEntries = append(dirEntries, &entry)
	}

	m.dirs[path] = dirEntries
}

func (m *MockFileSystem) SetError(path string, err error) {
	m.shouldError[path] = err
}

// mockFileInfo implements os.FileInfo for testing.
type mockFileInfo struct {
	name  string
	isDir bool
	mode  os.FileMode
}

func (m *mockFileInfo) Name() string { return m.name }
func (m *mockFileInfo) Size() int64  { return 0 }
func (m *mockFileInfo) Mode() os.FileMode {
	if m.mode != 0 {
		return m.mode
	}
	// Default permissions based on file type for security tests
	if strings.HasSuffix(m.name, ".gpg") || strings.HasSuffix(m.name, ".key") {
		return 0o600 // Private files
	}

	return 0o644 // Default
}
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Sys() any           { return nil }

// mockDirEntry implements os.DirEntry for testing.
type mockDirEntry struct {
	name  string
	isDir bool
}

func (m *mockDirEntry) Name() string      { return m.name }
func (m *mockDirEntry) IsDir() bool       { return m.isDir }
func (m *mockDirEntry) Type() os.FileMode { return 0 }
func (m *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{name: m.name, isDir: m.isDir}, nil
}

func TestFileOperations_FindFilesWithExtensions(t *testing.T) {
	t.Parallel()

	t.Run("finds files with specified extensions", func(t *testing.T) {
		t.Parallel()

		// Create isolated mock instance for this test
		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)
		// Set up test directory with files
		testDir := functionalTestKeyDir
		entries := []os.DirEntry{
			&mockDirEntry{name: functionalTestGPGKeyFile, isDir: false},
			&mockDirEntry{name: functionalTestPubKeyFile, isDir: false},
			&mockDirEntry{name: "key3.txt", isDir: false},
			&mockDirEntry{name: "subdir", isDir: true},
		}
		mockFS.AddDir(testDir, entries)

		extensions := []string{functionalTestGPGExtension, functionalTestPubExtension}
		files, err := fileOps.FindFilesWithExtensions(testDir, extensions)

		check.NoError(t, err)
		check.Len(t, files, 2)
		check.Contains(t, files, filepath.Join(testDir, functionalTestGPGKeyFile))
		check.Contains(t, files, filepath.Join(testDir, functionalTestPubKeyFile))
	})

	t.Run("handles non-existent directory", func(t *testing.T) {
		t.Parallel()

		// Create isolated mock instance for this test
		mockFS := NewMockFileSystem()
		fileOps := fileops.NewFileOperations(mockFS)

		extensions := []string{functionalTestGPGExtension}
		_, err := fileOps.FindFilesWithExtensions("/nonexistent", extensions)

		check.Error(t, err)
		check.ErrorIs(t, err, os.ErrNotExist)
	})
}

// The write lock is released and removed once the write completes, so a later
// run does not find a stale lock beside the file it is writing.
func TestFileOperations_SafeWriteFileWithCheck_RemovesLockFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "cleanup.txt")
	lockPath := filepath.Join(tmpDir, ".gommitlint.lock")

	fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())

	err := fileOps.SafeWriteFileWithCheck(filePath, []byte("content"), 0600, nil)
	check.NoError(t, err)

	_, err = os.Stat(lockPath)
	check.True(t, errors.Is(err, os.ErrNotExist), "lock file should be removed after operation")
}
