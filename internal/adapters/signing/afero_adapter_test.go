// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
)

func TestAferoAdapter_OpenNotImplemented(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	err := adapter.WriteFile("/any/path", []byte("content"), 0o644)
	check.NoError(t, err)

	file, err := adapter.Open("/any/path")
	check.Nil(t, file)
	check.ErrorIs(t, err, ErrNotImplementedInAfero)
}

func TestAferoAdapter_CreateTempNotImplemented(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	file, err := adapter.CreateTemp("/tmp", "test-*")
	check.Nil(t, file)
	check.ErrorIs(t, err, ErrNotImplementedInAfero)
}

func TestAferoAdapter_ReadWriteFile(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Write a file
	content := []byte("test content")
	err := adapter.WriteFile("/test.txt", content, 0o644)
	check.NoError(t, err)

	// Read it back
	readContent, err := adapter.ReadFile("/test.txt")
	check.NoError(t, err)
	check.Equal(t, content, readContent)
}

func TestAferoAdapter_Stat(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Create a file
	err := adapter.WriteFile("/test.txt", []byte("content"), 0o644)
	check.NoError(t, err)

	// Stat it
	info, err := adapter.Stat("/test.txt")
	check.NoError(t, err)
	check.Equal(t, "test.txt", info.Name())
	check.Equal(t, int64(7), info.Size())
}

func TestAferoAdapter_LstatDetectsSymlinkWhenSupported(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	check.NoError(t, os.WriteFile(target, []byte("content"), 0o600))

	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks not supported")
	}

	info, err := NewAferoAdapter(afero.NewOsFs()).Lstat(link)
	check.NoError(t, err)
	check.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink)
}

func TestAferoAdapter_MkdirAllAndReadDir(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Create directory structure
	err := adapter.MkdirAll("/a/b/c", 0o755)
	check.NoError(t, err)

	// Create files
	err = adapter.WriteFile("/a/b/file1.txt", []byte("1"), 0o644)
	check.NoError(t, err)
	err = adapter.WriteFile("/a/b/file2.txt", []byte("2"), 0o644)
	check.NoError(t, err)

	// Read directory
	entries, err := adapter.ReadDir("/a/b")
	check.NoError(t, err)
	check.Len(t, entries, 3) // c, file1.txt, file2.txt
}

func TestAferoAdapter_RemoveAndRename(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Create a file
	err := adapter.WriteFile("/test.txt", []byte("content"), 0o644)
	check.NoError(t, err)

	// Rename it
	err = adapter.Rename("/test.txt", "/renamed.txt")
	check.NoError(t, err)

	// Old path should not exist
	_, err = adapter.Stat("/test.txt")
	check.Error(t, err)

	// New path should exist
	_, err = adapter.Stat("/renamed.txt")
	check.NoError(t, err)

	// Remove it
	err = adapter.Remove("/renamed.txt")
	check.NoError(t, err)

	// Should not exist anymore
	_, err = adapter.Stat("/renamed.txt")
	check.Error(t, err)
}

func TestAferoAdapter_ReadFileWithLimit(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Create a file with known content
	content := []byte("this is some test content")
	err := adapter.WriteFile("/test.txt", content, 0o644)
	check.NoError(t, err)

	// Read with limit larger than content - should succeed
	readContent, err := adapter.ReadFileWithLimit("/test.txt", 1000)
	check.NoError(t, err)
	check.Equal(t, content, readContent)

	// Read with limit smaller than content - should fail
	_, err = adapter.ReadFileWithLimit("/test.txt", 5)
	check.ErrorIs(t, err, filesystem.ErrFileTooLarge)
}

func TestAferoAdapter_ReadFileWithLimit_RejectsFileThatGrowsOnOpen(t *testing.T) {
	t.Parallel()

	const path = "/growing.txt"

	baseFS := afero.NewMemMapFs()
	check.NoError(t, afero.WriteFile(baseFS, path, []byte("1234"), 0o600))

	adapter := NewAferoAdapter(growOnOpenFS{
		Fs:      baseFS,
		path:    path,
		content: []byte("12345"),
	})

	content, err := adapter.ReadFileWithLimit(path, 4)
	check.ErrorIs(t, err, filesystem.ErrFileTooLarge)
	check.Empty(t, content)
}

func TestAferoAdapter_WriteFileAtomic(t *testing.T) {
	t.Parallel()

	adapter := NewAferoAdapter(afero.NewMemMapFs())

	// Write to a nested path that doesn't exist yet
	content := []byte("atomic content")
	err := adapter.WriteFileAtomic("/deep/nested/path/file.txt", content, 0o644)
	check.NoError(t, err)

	// Verify it was written
	readContent, err := adapter.ReadFile("/deep/nested/path/file.txt")
	check.NoError(t, err)
	check.Equal(t, content, readContent)
}

func TestAferoAdapter_WriteFileAtomicPreservesTargetWhenRenameFails(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	check.NoError(t, afero.WriteFile(fs, "/target", []byte("old"), 0o600))
	adapter := NewAferoAdapter(renameFailFS{Fs: fs})

	err := adapter.WriteFileAtomic("/target", []byte("new"), 0o600)
	check.ErrorIs(t, err, os.ErrPermission)

	content, err := afero.ReadFile(fs, "/target")
	check.NoError(t, err)
	check.Equal(t, "old", string(content))

	entries, err := afero.ReadDir(fs, "/")
	check.NoError(t, err)
	check.Len(t, entries, 1)
}

type renameFailFS struct {
	afero.Fs
}

func (renameFailFS) Rename(_, _ string) error {
	return os.ErrPermission
}

type growOnOpenFS struct {
	afero.Fs

	path    string
	content []byte
}

func (fs growOnOpenFS) Open(name string) (afero.File, error) {
	if name == fs.path {
		if err := afero.WriteFile(fs.Fs, name, fs.content, 0o600); err != nil {
			return nil, err
		}
	}

	return fs.Fs.Open(name)
}
