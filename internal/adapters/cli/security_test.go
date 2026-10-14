// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func TestSecurityValidator_ValidateRepoPath(t *testing.T) {
	t.Parallel()

	// Using direct function calls now
	// Create a temporary git repository for testing
	tempDir := t.TempDir()
	gitDir := filepath.Join(tempDir, ".git")
	err := os.MkdirAll(gitDir, 0o750)
	check.NoError(t, err)
	// Create minimal git config to make it a valid git repo
	configFile := filepath.Join(gitDir, "config")
	err = os.WriteFile(configFile, []byte("[core]\n"), 0o600)
	check.NoError(t, err)

	nestedDir := filepath.Join(tempDir, "nested", "deeper")
	check.NoError(t, os.MkdirAll(nestedDir, 0o750))

	tests := []struct {
		name          string
		repoPath      string
		expectedPath  string
		expectError   bool
		expectedErr   error
		expectedCause error
		description   string
	}{
		{
			name:         "valid git repository",
			repoPath:     tempDir,
			expectedPath: tempDir,
			expectError:  false,
			description:  "Valid git repository should be accepted",
		},
		{
			name:         "nested directory resolves repository root",
			repoPath:     nestedDir,
			expectedPath: tempDir,
			description:  "Discovery should return the exact repository root",
		},
		{
			name:        cliTestPathTraversalAttack,
			repoPath:    "../../../etc",
			expectError: true,
			expectedErr: domain.ErrPathTraversal,
			description: cliTestPathTraversalBlocked,
		},
		{
			name:        cliTestNullByteInjectionName,
			repoPath:    "test\x00malicious",
			expectError: true,
			expectedErr: domain.ErrPathDangerous,
			description: cliTestNullByteBlocked,
		},
		{
			name:        cliTestVeryLongPathName,
			repoPath:    strings.Repeat("a", 2000),
			expectError: true,
			expectedErr: domain.ErrParameterTooLong,
			description: "OS will reject excessively long paths",
		},
		{
			name:        "non-existent path",
			repoPath:    "/nonexistent/path",
			expectError: true,
			expectedErr: ErrInvalidRepoPath,
			description: "Non-existent paths should be blocked",
		},
		{
			name:          "explicit file path inside repository",
			repoPath:      configFile,
			expectError:   true,
			expectedErr:   ErrInvalidRepoPath,
			expectedCause: fileops.ErrRepositoryPathNotDir,
			description:   "An explicit repository path must identify a directory",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			path, err := ValidateRepoPath(testCase.repoPath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
				}

				if testCase.expectedCause != nil {
					check.ErrorIs(t, err, testCase.expectedCause, testCase.description)
				}
			} else {
				check.NoError(t, err, testCase.description)
				check.Equal(t, testCase.expectedPath, path, testCase.description)
			}
		})
	}
}

func TestSecurityValidator_ValidateOutputFilePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	requestedDir := filepath.Join(root, "requested")
	outsideFile := filepath.Join(root, "outside.txt")
	symlinkedDir := filepath.Join(root, "symlinked-dir")
	symlinkPath := filepath.Join(requestedDir, "report.txt")
	check.NoError(t, os.Mkdir(requestedDir, 0o750))
	check.NoError(t, os.WriteFile(outsideFile, []byte("keep"), 0o600))
	check.NoError(t, os.Symlink(outsideFile, symlinkPath))
	check.NoError(t, os.Symlink(requestedDir, symlinkedDir))

	tests := []struct {
		name        string
		filePath    string
		expectError bool
		expectedErr error
		description string
	}{
		{
			name:        "empty path",
			filePath:    "",
			expectError: true,
			expectedErr: ErrFilePathEmpty,
			description: "Empty path should be rejected",
		},
		{
			name:        "valid relative path",
			filePath:    "output.txt",
			expectError: false,
			description: "Valid relative path should be accepted",
		},
		{
			name:        cliTestPathTraversalAttack,
			filePath:    cliTestTraversalOutsidePath,
			expectError: true,
			expectedErr: domain.ErrPathTraversal,
			description: cliTestPathTraversalBlocked,
		},
		{
			name:        cliTestNullByteInjectionName,
			filePath:    "output\x00.txt",
			expectError: true,
			expectedErr: domain.ErrPathDangerous,
			description: cliTestNullByteBlocked,
		},
		{
			name:        "normal file with spaces",
			filePath:    "output file.txt",
			expectError: false,
			description: "Files with spaces should be allowed",
		},
		{
			name:        "normal subdirectory",
			filePath:    "test/subdir/output.txt",
			expectError: false,
			description: "Normal subdirectories should be allowed",
		},
		{
			name:        "existing symlink",
			filePath:    symlinkPath,
			expectError: true,
			expectedErr: fileops.ErrSymlinkNotAllowed,
			description: "Output creation must not follow an existing symlink",
		},
		{
			name:        "symlinked parent",
			filePath:    filepath.Join(symlinkedDir, "new-report.txt"),
			expectError: true,
			expectedErr: fileops.ErrSymlinkNotAllowed,
			description: "Output creation must not follow a symlinked parent component",
		},
		{
			name:        "existing directory",
			filePath:    requestedDir,
			expectError: true,
			expectedErr: ErrOutputFileNotRegular,
			description: "An existing output target must be a regular file",
		},
		{
			name:        cliTestVeryLongPathName,
			filePath:    strings.Repeat("a", domain.MaxPathLength+1) + ".txt",
			expectError: true,
			expectedErr: domain.ErrParameterTooLong,
			description: "Excessively long paths should be blocked",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ValidateOutputFilePath(testCase.filePath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
				}
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestSecurityValidator_ValidateMessageFilePath(t *testing.T) {
	t.Parallel()

	// Using direct function calls now
	// Create a temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test-message.txt")
	err := os.WriteFile(testFile, []byte("test message"), 0o600)
	check.NoError(t, err)

	tests := []struct {
		name        string
		filePath    string
		expectError bool
		expectedErr error
		description string
	}{
		{
			name:        "empty path",
			filePath:    "",
			expectError: true,
			expectedErr: ErrFilePathEmpty,
			description: "Empty path should be rejected",
		},
		{
			name:        "valid existing file",
			filePath:    testFile,
			expectError: false,
			description: "Valid existing file should be accepted",
		},
		{
			name:        "directory is not a message file",
			filePath:    tempDir,
			expectError: true,
			expectedErr: ErrMessageFileNotRegular,
			description: "Only regular files and explicit stdin should be accepted",
		},
		{
			name:        "stdin dash skips validation",
			filePath:    "-",
			expectError: false,
			description: "'-' means stdin and must pass through untouched",
		},
		{
			name:        cliTestNonExistentFileName,
			filePath:    "non-existent.txt",
			expectError: true,
			expectedErr: ErrMessageFileNotExist,
			description: "Non-existent file should be rejected",
		},
		{
			name:        cliTestPathTraversalAttack,
			filePath:    cliTestTraversalOutsidePath,
			expectError: true,
			expectedErr: domain.ErrPathTraversal,
			description: cliTestPathTraversalBlocked,
		},
		{
			name:        cliTestNullByteInjectionName,
			filePath:    "message\x00.txt",
			expectError: true,
			expectedErr: domain.ErrPathDangerous,
			description: cliTestNullByteBlocked,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := ValidateMessageFilePath(testCase.filePath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
				}
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

// ParseSecureCommitRange and ValidateGitReference are one-line delegations to
// domain.SecurityPolicy; their tables live in internal/domain/security_test.go.
// This pins only that the aliases exist and propagate both value and error.
func TestParseSecureCommitRange_DelegatesToDomainPolicy(t *testing.T) {
	t.Parallel()

	parsed, err := ParseSecureCommitRange(cliTestCommitRangeMainFeature)
	check.NoError(t, err)
	check.Equal(t, domain.CommitRange{From: "main", To: "feature"}, parsed)

	_, err = ParseSecureCommitRange("main..feature; rm -rf /")
	check.ErrorIs(t, err, domain.ErrGitRefDangerous)

	check.ErrorIs(t, ValidateGitReference(""), domain.ErrGitRefEmpty)
}
