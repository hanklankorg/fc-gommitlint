// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
)

// ErrCreateTempNotSupported is returned when CreateTemp is called on a memory filesystem.
var ErrCreateTempNotSupported = errors.New("CreateTemp not supported in memory filesystem")

// FileSystem provides an abstraction over filesystem operations for testing.
// It can use either real OS filesystem or in-memory filesystem.
type FileSystem struct {
	fs      afero.Fs
	baseDir string
	t       *testing.T
}

// NewFileSystem creates a new FileSystem using the OS filesystem.
// For integration tests that need real git operations.
func NewFileSystem(t *testing.T) *FileSystem {
	t.Helper()
	baseDir := t.TempDir()

	return &FileSystem{
		fs:      afero.NewOsFs(),
		baseDir: baseDir,
		t:       t,
	}
}

// NewMemoryFileSystem creates a new FileSystem using in-memory storage.
// Perfect for unit tests that don't need real filesystem operations.
func NewMemoryFileSystem(t *testing.T) *FileSystem {
	t.Helper()

	return &FileSystem{
		fs:      afero.NewMemMapFs(),
		baseDir: "/test",
		t:       t,
	}
}

// NewCopyOnWriteFileSystem creates a filesystem that reads from OS but writes to memory.
// Useful for tests that need to read real files but shouldn't modify them.
func NewCopyOnWriteFileSystem(t *testing.T, readOnlyBase string) *FileSystem {
	t.Helper()

	osFs := afero.NewOsFs()
	memFs := afero.NewMemMapFs()
	copyOnWriteFs := afero.NewCopyOnWriteFs(osFs, memFs)

	return &FileSystem{
		fs:      copyOnWriteFs,
		baseDir: readOnlyBase,
		t:       t,
	}
}

// Path returns the full path for a relative path within the test filesystem.
func (fs *FileSystem) Path(relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}

	return filepath.Join(fs.baseDir, relativePath)
}

// WriteFile writes a file with the given content.
func (fs *FileSystem) WriteFile(path string, content []byte) {
	fs.t.Helper()

	fullPath := fs.Path(path)
	dir := filepath.Dir(fullPath)

	if dir != "." && dir != "/" {
		err := fs.fs.MkdirAll(dir, 0755)
		check.NoError(fs.t, err, "Failed to create directory: %s", dir)
	}

	err := afero.WriteFile(fs.fs, fullPath, content, 0644)
	check.NoError(fs.t, err, "Failed to write file: %s", fullPath)
}

// WriteExecutable writes an executable file.
func (fs *FileSystem) WriteExecutable(path string, content []byte) {
	fs.t.Helper()

	fullPath := fs.Path(path)
	dir := filepath.Dir(fullPath)

	if dir != "." && dir != "/" {
		err := fs.fs.MkdirAll(dir, 0755)
		check.NoError(fs.t, err, "Failed to create directory: %s", dir)
	}

	err := afero.WriteFile(fs.fs, fullPath, content, 0755)
	check.NoError(fs.t, err, "Failed to write executable: %s", fullPath)
}

// ReadFile reads a file and returns its content.
func (fs *FileSystem) ReadFile(path string) string {
	fs.t.Helper()

	fullPath := fs.Path(path)
	content, err := afero.ReadFile(fs.fs, fullPath)
	check.NoError(fs.t, err, "Failed to read file: %s", fullPath)

	return string(content)
}

// CreateDir creates a directory.
func (fs *FileSystem) CreateDir(path string) {
	fs.t.Helper()

	fullPath := fs.Path(path)
	err := fs.fs.MkdirAll(fullPath, 0755)
	check.NoError(fs.t, err, "Failed to create directory: %s", fullPath)
}

// FileExists checks if a file exists.
func (fs *FileSystem) FileExists(path string) bool {
	fs.t.Helper()

	fullPath := fs.Path(path)

	exists, err := afero.Exists(fs.fs, fullPath)
	if err != nil {
		return false
	}

	info, err := fs.fs.Stat(fullPath)
	if err != nil {
		return false
	}

	return exists && !info.IsDir()
}

// DirExists checks if a directory exists.
func (fs *FileSystem) DirExists(path string) bool {
	fs.t.Helper()

	fullPath := fs.Path(path)

	exists, err := afero.Exists(fs.fs, fullPath)
	if err != nil {
		return false
	}

	info, err := fs.fs.Stat(fullPath)
	if err != nil {
		return false
	}

	return exists && info.IsDir()
}

// Remove removes a file or directory.
func (fs *FileSystem) Remove(path string) {
	fs.t.Helper()

	fullPath := fs.Path(path)
	err := fs.fs.RemoveAll(fullPath)

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fs.t.Logf("Warning: failed to remove %s: %v", fullPath, err)
	}
}

// CopyFile copies a file from source to destination.
func (fs *FileSystem) CopyFile(src, dst string) {
	fs.t.Helper()

	srcPath := fs.Path(src)
	dstPath := fs.Path(dst)

	content, err := afero.ReadFile(fs.fs, srcPath)
	check.NoError(fs.t, err, "Failed to read source file: %s", srcPath)

	fs.WriteFile(dstPath, content)
}

// CopyDir copies a directory tree from source to destination.
func (fs *FileSystem) CopyDir(src, dst string) {
	fs.t.Helper()

	srcPath := fs.Path(src)
	dstPath := fs.Path(dst)

	err := afero.Walk(fs.fs, srcPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dstPath, rel)
		if info.IsDir() {
			return fs.fs.MkdirAll(target, 0755)
		}

		content, err := afero.ReadFile(fs.fs, path)
		if err != nil {
			return err
		}

		perm := info.Mode().Perm()
		if perm == 0 {
			perm = 0644
		}

		return afero.WriteFile(fs.fs, target, content, perm)
	})
	check.NoError(fs.t, err, "Failed to copy directory: %s", srcPath)
}

// CreateFiles creates multiple files at once.
func (fs *FileSystem) CreateFiles(files map[string]string) {
	fs.t.Helper()

	for relativePath, content := range files {
		fs.WriteFile(relativePath, []byte(content))
	}
}

// AssertFileContains checks if file contains expected content.
func (fs *FileSystem) AssertFileContains(path, expected string) {
	fs.t.Helper()

	content := fs.ReadFile(path)
	check.Contains(fs.t, content, expected, "File %s should contain %q", path, expected)
}

// AssertFileEquals verifies a file has exact content.
func (fs *FileSystem) AssertFileEquals(path, expected string) {
	fs.t.Helper()

	content := fs.ReadFile(path)
	check.Equal(fs.t, expected, content, "File content mismatch in %s", path)
}

// Walk walks the filesystem tree and calls fn for each file.
func (fs *FileSystem) Walk(fn filepath.WalkFunc) error {
	return afero.Walk(fs.fs, fs.baseDir, fn)
}

// Glob returns names of files matching the given pattern.
func (fs *FileSystem) Glob(pattern string) ([]string, error) {
	return afero.Glob(fs.fs, fs.Path(pattern))
}

// SigningFileSystemAdapter adapts our FileSystem to implement filesystem.FileSystem.
type SigningFileSystemAdapter struct {
	fs afero.Fs
}

// NewSigningFileSystemAdapter creates a new adapter for the signing package.
func NewSigningFileSystemAdapter(fs *FileSystem) *SigningFileSystemAdapter {
	return &SigningFileSystemAdapter{
		fs: fs.fs,
	}
}

// ReadFile reads and returns the contents of the named file.
func (a *SigningFileSystemAdapter) ReadFile(path string) ([]byte, error) {
	return afero.ReadFile(a.fs, path)
}

// WriteFile writes data to the named file with the specified permissions.
func (a *SigningFileSystemAdapter) WriteFile(path string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(a.fs, path, data, perm)
}

// ReadDir reads the named directory and returns all its entries.
func (a *SigningFileSystemAdapter) ReadDir(path string) ([]os.DirEntry, error) {
	return afero.NewIOFS(a.fs).ReadDir(path)
}

// Stat returns file information for the named file.
func (a *SigningFileSystemAdapter) Stat(path string) (os.FileInfo, error) {
	return a.fs.Stat(path)
}

// Lstat delegates to afero's optional Lstater capability when available.
// Filesystems without it fall back to Stat and cannot model symlink checks.
func (a *SigningFileSystemAdapter) Lstat(path string) (os.FileInfo, error) {
	if lstater, ok := a.fs.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)

		return info, err
	}

	return a.fs.Stat(path)
}

// Open opens the named file for reading.
func (a *SigningFileSystemAdapter) Open(path string) (*os.File, error) {
	// Note: This is a limitation - afero doesn't return *os.File
	// For testing purposes, we'll need to handle this differently
	// or mock at a higher level
	return nil, os.ErrInvalid
}

// CreateTemp always fails closed with ErrCreateTempNotSupported.
//
// The filesystem.FileSystem interface requires an *os.File, which afero cannot
// provide, so neither the memory nor the real-FS test environment offers it.
// Code that needs a temp file takes filesystem.NewOSFileSystem instead.
func (a *SigningFileSystemAdapter) CreateTemp(dir, pattern string) (*os.File, error) {
	return nil, ErrCreateTempNotSupported
}

// Remove removes the named file or directory.
func (a *SigningFileSystemAdapter) Remove(path string) error {
	return a.fs.Remove(path)
}

// Rename renames (moves) oldPath to newPath.
func (a *SigningFileSystemAdapter) Rename(oldPath, newPath string) error {
	return a.fs.Rename(oldPath, newPath)
}

// Chmod changes the mode of the named file to mode.
func (a *SigningFileSystemAdapter) Chmod(path string, mode os.FileMode) error {
	return a.fs.Chmod(path, mode)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (a *SigningFileSystemAdapter) MkdirAll(path string, perm os.FileMode) error {
	return a.fs.MkdirAll(path, perm)
}

// ReadFileWithLimit returns file contents only when they fit within maxSize.
func (a *SigningFileSystemAdapter) ReadFileWithLimit(path string, maxSize int64) ([]byte, error) {
	file, err := a.fs.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	return filesystem.ReadWithLimit(file, maxSize)
}

// WriteFileAtomic satisfies the production interface but does not model atomic
// replacement; this test double writes directly to its isolated filesystem.
func (a *SigningFileSystemAdapter) WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(a.fs, path, data, perm)
}

// Convenience methods for FileSystem

// WriteConfig writes a gommitlint configuration file with common defaults.
func (fs *FileSystem) WriteConfig(path string, rules map[string]any) {
	fs.t.Helper()

	var config strings.Builder

	config.WriteString("gommitlint:\n")

	if len(rules) > 0 {
		config.WriteString("  rules:\n")

		for rule, value := range rules {
			if enabled, ok := value.(bool); ok {
				fmt.Fprintf(&config, "    %s:\n      enabled: %v\n", rule, enabled)
			} else if settings, ok := value.(map[string]any); ok {
				fmt.Fprintf(&config, "    %s:\n", rule)

				for key, val := range settings {
					fmt.Fprintf(&config, "      %s: %v\n", key, val)
				}
			}
		}
	}

	fs.WriteFile(path, []byte(config.String()))
}

// AssertNoFile verifies a file does not exist.
func (fs *FileSystem) AssertNoFile(path string) {
	fs.t.Helper()
	check.False(fs.t, fs.FileExists(path), "File should not exist: %s", path)
}

// GetAferoFS returns the underlying Afero filesystem for advanced operations.
// This returns the concrete Afero filesystem, not an interface.
func (fs *FileSystem) GetAferoFS() afero.Fs {
	return fs.fs
}

// StatFile returns file information for the named file.
func (fs *FileSystem) StatFile(path string) os.FileInfo {
	fs.t.Helper()

	fullPath := fs.Path(path)
	info, err := fs.fs.Stat(fullPath)
	check.NoError(fs.t, err, "Failed to stat file: %s", fullPath)

	return info
}

// ChmodDir changes the permissions of a directory.
func (fs *FileSystem) ChmodDir(path string, mode os.FileMode) {
	fs.t.Helper()

	fullPath := fs.Path(path)
	err := fs.fs.Chmod(fullPath, mode)
	check.NoError(fs.t, err, "Failed to chmod directory: %s", fullPath)
}
