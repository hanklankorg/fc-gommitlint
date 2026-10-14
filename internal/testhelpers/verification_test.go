// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

// TestHelperRobustness verifies helpers handle edge cases correctly.
func TestHelperRobustness(t *testing.T) {
	t.Run("WriteFile creates nested directories", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		baseDir := "/test"
		deepPath := filepath.Join(baseDir, "a", "b", "c", "d", "e", "file.txt")

		// Should create all parent directories
		env.FS.WriteFile(deepPath, []byte("deep"))

		// Verify file and all directories exist
		check.True(t, env.FS.FileExists(deepPath))
		check.True(t, env.FS.DirExists(filepath.Join(baseDir, "a")))
		check.True(t, env.FS.DirExists(filepath.Join(baseDir, "a", "b", "c")))
	})

	t.Run("WriteFile overwrites existing files", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		path := "/file.txt"

		// Write initial content
		env.FS.WriteFile(path, []byte("initial"))
		env.FS.AssertFileEquals(path, "initial")

		// Overwrite with new content
		env.FS.WriteFile(path, []byte("updated"))
		env.FS.AssertFileEquals(path, "updated")
	})

	t.Run("CreateDir is idempotent", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		dir := "/test-dir"

		// Create directory multiple times
		env.FS.CreateDir(dir)
		env.FS.CreateDir(dir)
		env.FS.CreateDir(dir)

		// Should still exist and be a directory
		check.True(t, env.FS.DirExists(dir))
	})

	t.Run("FileExists vs DirExists differentiate correctly", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		filePath := "/file.txt"
		dirPath := "/directory"

		env.FS.WriteFile(filePath, []byte("content"))
		env.FS.CreateDir(dirPath)

		// File checks
		check.True(t, env.FS.FileExists(filePath))
		check.False(t, env.FS.DirExists(filePath))

		// Directory checks
		check.False(t, env.FS.FileExists(dirPath))
		check.True(t, env.FS.DirExists(dirPath))
	})

	t.Run("CreateFiles handles empty map", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		dir := "/test/empty-files-test"

		// Should not panic with empty map
		env.FS.CreateFiles(map[string]string{})

		// Directory should exist in memory filesystem
		env.FS.CreateDir(dir)
		check.True(t, env.FS.DirExists(dir))
	})

	t.Run("WriteExecutable sets correct permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("File permissions work differently on Windows")
		}

		env := testhelpers.NewTestEnv(t)
		path := "/test/script.sh"

		env.FS.WriteExecutable(path, []byte("#!/bin/sh\necho test"))

		// Check file exists in memory filesystem
		// Note: afero memory filesystem tracks permissions internally
		// We verify the file was created with WriteExecutable
		check.True(t, env.FS.FileExists(path))

		// Get the filesystem to check permissions
		fs := env.FS.GetAferoFS()
		info, err := fs.Stat(path)
		check.NoError(t, err)

		mode := info.Mode()
		check.NotEqual(t, os.FileMode(0), mode&0111, "File should have execute permission")
	})

	t.Run("CopyFile preserves content exactly", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		src := "/source.bin"
		dst := "/dest.bin"

		// Create binary content
		binaryContent := []byte{0x00, 0xFF, 0x42, 0x13, 0x37, 0xCA, 0xFE}
		env.FS.WriteFile(src, binaryContent)

		// Copy file
		env.FS.CopyFile(src, dst)

		// Verify exact content match
		srcContent := env.FS.ReadFile(src)
		dstContent := env.FS.ReadFile(dst)
		check.Equal(t, srcContent, dstContent)
	})

	t.Run("AssertFileContains works with multiline", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		path := "/multiline.txt"

		content := `Line 1
Line 2
Line 3 with special chars: !@#$%^&*()
Line 4`

		env.FS.WriteFile(path, []byte(content))

		// Should find partial matches
		env.FS.AssertFileContains(path, "Line 2")
		env.FS.AssertFileContains(path, "special chars")
		env.FS.AssertFileContains(path, "!@#$%^&*()")
	})

	t.Run("IsolatedDir creates nested structure", func(t *testing.T) {
		env := testhelpers.NewTestEnv(t)
		dir := "/isolated/my-feature-test"
		env.FS.CreateDir(dir)

		// Should be a real directory
		check.True(t, env.FS.DirExists(dir))

		// Parent should also exist
		parent := filepath.Dir(dir)
		check.True(t, env.FS.DirExists(parent))
	})
}

// TestConcurrentHelperUsage verifies helpers are safe for concurrent use.
func TestConcurrentHelperUsage(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	baseDir := "/concurrent"

	// Run multiple goroutines using helpers concurrently
	done := make(chan bool, 10)

	for i := range 10 { //nolint:varnamelen // i is conventional for loop index
		go func(id int) {
			defer func() { done <- true }()

			// Each goroutine creates its own files
			dirName := fmt.Sprintf("goroutine-%d", id)
			dir := filepath.Join(baseDir, "test", "dir", dirName)
			file := filepath.Join(dir, "file.txt")

			env.FS.WriteFile(file, []byte("concurrent test"))

			// Verify
			if !env.FS.FileExists(file) {
				t.Errorf("File doesn't exist: %s", file)
			}
		}(i)
	}

	// Wait for all goroutines
	for range 10 {
		<-done
	}
}
