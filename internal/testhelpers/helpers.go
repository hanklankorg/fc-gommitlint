// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers

import (
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// TestFS provides filesystem operations for tests using afero.
// This is a simpler alternative to the full FileSystem struct when you
// just need basic filesystem operations in a test.
type TestFS struct {
	*testing.T
	afero.Fs

	root string
}

// NewTestFS creates a new test filesystem rooted at the given path.
// If root is empty, uses t.TempDir().
func NewTestFS(t *testing.T, root string) *TestFS {
	t.Helper()

	if root == "" {
		root = t.TempDir()
	}

	// Use OS filesystem but rooted at test directory
	baseFs := afero.NewOsFs()
	fs := afero.NewBasePathFs(baseFs, root)

	return &TestFS{
		T:    t,
		Fs:   fs,
		root: root,
	}
}

// NewMemoryTestFS creates an in-memory test filesystem.
func NewMemoryTestFS(t *testing.T) *TestFS {
	t.Helper()

	return &TestFS{
		T:    t,
		Fs:   afero.NewMemMapFs(),
		root: "/",
	}
}

// WriteFile writes a file with the given content.
func (fs *TestFS) WriteFile(path string, content []byte) {
	fs.Helper()

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		err := fs.MkdirAll(dir, domain.FilePermPublicExec)
		check.NoError(fs.T, err, "Failed to create directory: %s", dir)
	}

	err := afero.WriteFile(fs.Fs, path, content, domain.FilePermPublicRead)
	check.NoError(fs.T, err, "Failed to write file: %s", path)
}

// WriteExecutable writes an executable file.
func (fs *TestFS) WriteExecutable(path string, content []byte) {
	fs.Helper()

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		err := fs.MkdirAll(dir, domain.FilePermPublicExec)
		check.NoError(fs.T, err, "Failed to create directory: %s", dir)
	}

	err := afero.WriteFile(fs.Fs, path, content, domain.FilePermPublicExec)
	check.NoError(fs.T, err, "Failed to write executable: %s", path)
}

// WriteConfig writes a configuration file.
func (fs *TestFS) WriteConfig(path, content string) {
	fs.Helper()
	fs.WriteFile(path, []byte(content))
}

// CreateDir creates a directory.
func (fs *TestFS) CreateDir(path string) {
	fs.Helper()

	err := fs.MkdirAll(path, domain.FilePermPublicExec)
	check.NoError(fs.T, err, "Failed to create directory: %s", path)
}

// ReadFile reads a file and returns its content.
func (fs *TestFS) ReadFile(path string) string {
	fs.Helper()

	content, err := afero.ReadFile(fs.Fs, path)
	check.NoError(fs.T, err, "Failed to read file: %s", path)

	return string(content)
}

// FileExists checks if a file exists.
func (fs *TestFS) FileExists(path string) bool {
	fs.Helper()

	exists, err := afero.Exists(fs.Fs, path)
	if err != nil {
		return false
	}

	return exists
}

// DirExists checks if a directory exists.
func (fs *TestFS) DirExists(path string) bool {
	fs.Helper()

	exists, err := afero.DirExists(fs.Fs, path)
	if err != nil {
		return false
	}

	return exists
}

// Root returns the root path of the filesystem.
func (fs *TestFS) Root() string {
	return fs.root
}

// Path joins the root with the given path segments.
func (fs *TestFS) Path(segments ...string) string {
	parts := append([]string{fs.root}, segments...)

	return filepath.Join(parts...)
}
