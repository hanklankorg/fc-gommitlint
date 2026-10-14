// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers_test

import (
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestMemoryFileSystem(t *testing.T) {
	t.Parallel()

	t.Run("basic file operations", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Write a file
		fs.WriteFile("test.txt", []byte("hello world"))

		// Check it exists
		check.True(t, fs.FileExists("test.txt"))
		check.False(t, fs.DirExists("test.txt"))

		// Read it back
		content := fs.ReadFile("test.txt")
		check.Equal(t, "hello world", content)

		// Check contains
		fs.AssertFileContains("test.txt", "hello")
		fs.AssertFileEquals("test.txt", "hello world")
	})

	t.Run("directory operations", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Create nested directories
		fs.CreateDir("foo/bar/baz")

		// Check they exist
		check.True(t, fs.DirExists("foo"))
		check.True(t, fs.DirExists("foo/bar"))
		check.True(t, fs.DirExists("foo/bar/baz"))

		// Write file in nested dir
		fs.WriteFile("foo/bar/test.txt", []byte("nested"))
		check.True(t, fs.FileExists("foo/bar/test.txt"))
		check.Equal(t, "nested", fs.ReadFile("foo/bar/test.txt"))
	})

	t.Run("executable files", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Write executable
		fs.WriteExecutable("script.sh", []byte("#!/bin/bash\necho hello"))

		// Check it exists
		check.True(t, fs.FileExists("script.sh"))
		content := fs.ReadFile("script.sh")
		check.Equal(t, "#!/bin/bash\necho hello", content)
	})

	t.Run("batch file creation", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		files := map[string]string{
			"README.md":         "# Test Project",
			"src/main.go":       "package main",
			"src/lib/helper.go": "package lib",
			"test/test.go":      testPackageGo,
		}

		fs.CreateFiles(files)

		// Verify all files exist
		for path, expectedContent := range files {
			check.True(t, fs.FileExists(path), "File %s should exist", path)
			fs.AssertFileEquals(path, expectedContent)
		}
	})

	t.Run("copy operations", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Create source file
		fs.WriteFile("source.txt", []byte("original content"))

		// Copy it
		fs.CopyFile("source.txt", "dest.txt")

		// Both should exist with same content
		check.True(t, fs.FileExists("source.txt"))
		check.True(t, fs.FileExists("dest.txt"))
		fs.AssertFileEquals("dest.txt", "original content")
	})

	t.Run("remove operations", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Create file and directory
		fs.WriteFile("file.txt", []byte("content"))
		fs.CreateDir("dir/subdir")
		fs.WriteFile("dir/file.txt", []byte("nested"))

		// Remove file
		fs.Remove("file.txt")
		check.False(t, fs.FileExists("file.txt"))

		// Remove directory tree
		fs.Remove("dir")
		check.False(t, fs.DirExists("dir"))
		check.False(t, fs.FileExists("dir/file.txt"))
	})

	t.Run("glob patterns", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryFileSystem(t)

		// Create files
		fs.CreateFiles(map[string]string{
			"foo.go":     "package foo",
			"bar.go":     "package bar",
			"baz.txt":    "text file",
			"test/a.go":  "package test",
			"test/b.txt": "text",
		})

		// Test glob patterns
		goFiles, err := fs.Glob("*.go")
		check.NoError(t, err)
		check.Len(t, goFiles, 2)

		allFiles, err := fs.Glob("*")
		check.NoError(t, err)
		check.Len(t, allFiles, 4) // 3 files + 1 directory

		testFiles, err := fs.Glob("test/*")
		check.NoError(t, err)
		check.Len(t, testFiles, 2)
	})
}

func TestOSFileSystem(t *testing.T) {
	// Not parallel - uses real filesystem operations
	// Skip in short mode as this uses real filesystem
	if testing.Short() {
		t.Skip("Skipping OS filesystem test in short mode")
	}

	t.Run("real filesystem operations", func(t *testing.T) {
		// Not parallel - uses real filesystem
		fs := testhelpers.NewFileSystem(t)

		// Write a file
		fs.WriteFile("test.txt", []byte("real file"))

		// Get the actual path
		actualPath := fs.Path("test.txt")
		check.True(t, filepath.IsAbs(actualPath))

		// Verify it exists
		check.True(t, fs.FileExists("test.txt"))
		fs.AssertFileEquals("test.txt", "real file")

		// Cleanup happens automatically via t.TempDir()
	})
}

func TestCopyOnWriteFileSystem(t *testing.T) {
	// Not parallel - uses real filesystem as base layer
	if testing.Short() {
		t.Skip("Skipping COW filesystem test in short mode")
	}

	t.Run("read from OS, write to memory", func(t *testing.T) {
		// Create a real file first
		osFs := testhelpers.NewFileSystem(t)
		osFs.WriteFile("readonly.txt", []byte("original"))
		basePath := osFs.Path("")

		// Create COW filesystem
		cowFs := testhelpers.NewCopyOnWriteFileSystem(t, basePath)

		// Should be able to read the original file
		content := cowFs.ReadFile("readonly.txt")
		check.Equal(t, "original", content)

		// Write should go to memory layer
		cowFs.WriteFile("readonly.txt", []byte("modified"))

		// Reading should get the modified version
		newContent := cowFs.ReadFile("readonly.txt")
		check.Equal(t, "modified", newContent)

		// Original file should be unchanged
		originalContent := osFs.ReadFile("readonly.txt")
		check.Equal(t, "original", originalContent)
	})
}
