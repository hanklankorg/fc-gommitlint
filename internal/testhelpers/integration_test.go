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

// TestTestFS_FileConfigAndCopyOperations exercises the memory-FS helper's
// write, config, directory and copy operations together.
func TestTestFS_FileConfigAndCopyOperations(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	// Create isolated test directory
	baseDir := "/integration-test"
	env.FS.CreateDir(baseDir)

	// Test WriteFile creates parent directories
	deepFile := filepath.Join(baseDir, "deep", "nested", "dir", "file.txt")
	env.FS.WriteFile(deepFile, []byte("deep content"))

	// Verify file was created with content
	check.True(t, env.FS.FileExists(deepFile))
	env.FS.AssertFileEquals(deepFile, "deep content")

	// Test WriteConfig for gommitlint config
	configPath := filepath.Join(baseDir, "config.yaml")
	env.FS.WriteConfig(configPath, map[string]any{
		"subject": true,
	})
	env.FS.AssertFileContains(configPath, "subject")
	env.FS.AssertFileContains(configPath, "enabled: true")

	// Test directory creation
	dirPath := filepath.Join(baseDir, "test-dir")
	env.FS.CreateDir(dirPath)
	check.True(t, env.FS.DirExists(dirPath))

	// Test copy operations
	srcFile := filepath.Join(baseDir, "source.txt")
	dstFile := filepath.Join(baseDir, "dest.txt")

	env.FS.WriteFile(srcFile, []byte("source data"))
	env.FS.CopyFile(srcFile, dstFile)
	env.FS.AssertFileEquals(dstFile, "source data")

	// Test TempFile creation - skipped due to memory filesystem limitations
	// CreateTemp requires *os.File which memory filesystem cannot provide
}

// TestFileSystemAssertions_VerifyFileExistenceAndContent verifies the
// existence and content assertion helpers.
func TestFileSystemAssertions_VerifyFileExistenceAndContent(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	filePath := "/test.txt"

	// File doesn't exist yet
	check.False(t, env.FS.FileExists(filePath))

	// Create file
	env.FS.WriteFile(filePath, []byte("test content\nwith multiple lines"))

	// Now it exists
	check.True(t, env.FS.FileExists(filePath))

	// Test assertions
	env.FS.AssertFileContains(filePath, "test content")
	env.FS.AssertFileContains(filePath, "multiple lines")

	// Test ReadFile
	content := env.FS.ReadFile(filePath)
	check.Contains(t, content, "test content")
}

// TestCreateDir_CreatesAllNestedLevels verifies nested directory creation.
func TestCreateDir_CreatesAllNestedLevels(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	baseDir := "/test"

	// Create nested directories
	nestedDir := filepath.Join(baseDir, "level1", "level2", "level3")
	env.FS.CreateDir(nestedDir)

	// Verify all levels were created
	check.True(t, env.FS.DirExists(filepath.Join(baseDir, "level1")))
	check.True(t, env.FS.DirExists(filepath.Join(baseDir, "level1", "level2")))
	check.True(t, env.FS.DirExists(nestedDir))

	// Verify it's a directory, not a file
	check.False(t, env.FS.FileExists(nestedDir))
}

// TestCopyDir_CopiesNestedTree verifies recursive directory copying.
func TestCopyDir_CopiesNestedTree(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	srcDir := "/source"
	dstDir := "/dest"

	// Create source directory with files
	files := map[string]string{
		"file1.txt":          "content1",
		"sub/file2.txt":      "content2",
		"sub/deep/file3.txt": "content3",
	}

	for relPath, content := range files {
		fullPath := filepath.Join(srcDir, relPath)
		env.FS.WriteFile(fullPath, []byte(content))
	}

	// Copy directory
	env.FS.CopyDir(srcDir, dstDir)

	// Verify all files were copied
	env.FS.AssertFileEquals(filepath.Join(dstDir, "file1.txt"), "content1")
	env.FS.AssertFileEquals(filepath.Join(dstDir, "sub", "file2.txt"), "content2")
	env.FS.AssertFileEquals(filepath.Join(dstDir, "sub", "deep", "file3.txt"), "content3")
}
