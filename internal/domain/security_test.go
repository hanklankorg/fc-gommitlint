// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSecurityPolicy_ValidatePath(t *testing.T) {
	t.Parallel()

	policy := DefaultSecurityPolicy()

	tests := []struct {
		name        string
		path        string
		expectedErr error // nil means the path must validate
		description string
	}{
		{
			name:        "valid relative path",
			path:        "src/main.go",
			description: "simple relative paths should be valid",
		},
		{
			name:        "empty path",
			path:        "",
			expectedErr: ErrPathEmpty,
			description: "empty paths should be rejected",
		},
		{
			name:        "path traversal attempt",
			path:        "../../../etc/passwd",
			expectedErr: ErrPathTraversal,
			description: "directory traversal should be blocked",
		},
		{
			name:        "windows path traversal",
			path:        "..\\..\\windows\\system32",
			expectedErr: ErrPathTraversal,
			description: "windows traversal should be blocked",
		},
		{
			name:        "mixed path traversal",
			path:        "some/path/../../../etc/passwd",
			expectedErr: ErrPathTraversal,
			description: "traversal in middle should be caught by filepath.Clean",
		},
		{
			name:        "null byte injection",
			path:        "file.txt\x00.exe",
			expectedErr: ErrPathDangerous,
			description: "null bytes should be rejected",
		},
		{
			name:        "very long path",
			path:        string(make([]byte, MaxPathLength+1)),
			expectedErr: ErrParameterTooLong,
			description: "overly long paths should be rejected",
		},
		{
			name:        "path with spaces allowed",
			path:        "/test/some file.txt",
			description: "spaces are ordinary path characters, not a threat",
		},
		{
			name:        "tab character allowed",
			path:        "path\tfile",
			description: "tab characters are not a real threat and stay allowed",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := policy.ValidatePath(testCase.path)
			if testCase.expectedErr != nil {
				check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestSecurityPolicy_ValidatePathWithinBase(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	policy := DefaultSecurityPolicy()

	tests := []struct {
		name        string
		targetPath  string
		expectError bool
	}{
		{
			name:       "base directory itself",
			targetPath: baseDir,
		},
		{
			name:       "nested path",
			targetPath: filepath.Join(baseDir, "sub", "file.txt"),
		},
		{
			name:        "sibling prefix path",
			targetPath:  filepath.Join(baseDir+"-sibling", "file.txt"),
			expectError: true,
		},
		{
			name:        "parent path",
			targetPath:  filepath.Dir(baseDir),
			expectError: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := policy.ValidatePathWithinBase(baseDir, testCase.targetPath)
			if testCase.expectError {
				check.Error(t, err)
			} else {
				check.NoError(t, err)
			}
		})
	}
}

func TestSecurityPolicy_ValidateGitReference(t *testing.T) {
	t.Parallel()

	policy := DefaultSecurityPolicy()

	tests := []struct {
		name        string
		ref         string
		expectedErr error // nil means the reference must validate
		description string
	}{
		{
			name:        "valid branch name",
			ref:         "main",
			description: "simple branch names should be valid",
		},
		{
			name:        "valid commit hash",
			ref:         "abc123def456",
			description: "commit hashes should be valid",
		},
		{
			name:        "empty reference",
			ref:         "",
			expectedErr: ErrGitRefEmpty,
			description: "empty references should be rejected",
		},
		{
			name:        "semicolon metacharacter",
			ref:         "branch; rm -rf /",
			expectedErr: ErrGitRefDangerous,
			description: "semicolon input should be blocked",
		},
		{
			name:        "embedded newline",
			ref:         "branch\nrm -rf /",
			expectedErr: ErrGitRefDangerous,
			description: "embedded newlines should be blocked",
		},
		{
			name:        "embedded carriage return",
			ref:         "branch\rrm -rf /",
			expectedErr: ErrGitRefDangerous,
			description: "embedded carriage returns should be blocked",
		},
		{
			name:        "embedded null byte",
			ref:         "branch\x00",
			expectedErr: ErrGitRefDangerous,
			description: "null bytes should be rejected",
		},
		{
			name:        "very long reference",
			ref:         strings.Repeat("a", MaxRefLength+1),
			expectedErr: ErrParameterTooLong,
			description: "overly long references should be rejected",
		},
		{
			name:        "HEAD reference",
			ref:         "HEAD",
			description: "HEAD should be valid",
		},
		{
			name:        "branch with slashes",
			ref:         "feature/add-new-thing",
			description: "branch names with slashes should be allowed",
		},
		{
			name:        "dollar-parenthesis metacharacters",
			ref:         "main$(rm -rf /)",
			expectedErr: ErrGitRefDangerous,
			description: "dollar-parenthesis input should be blocked",
		},
		{
			name:        "backtick metacharacter",
			ref:         "main`rm -rf /`",
			expectedErr: ErrGitRefDangerous,
			description: "backtick input should be blocked",
		},
		{
			name:        "pipe metacharacter",
			ref:         "main|rm -rf /",
			expectedErr: ErrGitRefDangerous,
			description: "pipes should be blocked",
		},
		{
			name:        "ampersand metacharacter",
			ref:         "main&rm -rf /",
			expectedErr: ErrGitRefDangerous,
			description: "ampersands should be blocked",
		},
		{
			name:        "branch with hyphens and dots",
			ref:         "release-v1.0.0",
			description: "hyphens and dots are ordinary ref characters",
		},
		{
			name:        "branch with underscores",
			ref:         "feature_branch_name",
			description: "underscores are ordinary ref characters",
		},
		{
			name:        "traversal-looking ref allowed",
			ref:         "../../../tmp/file",
			description: "only policy metacharacters are blocked; git validates ref names itself",
		},
		{
			name:        "long-but-legal reference",
			ref:         strings.Repeat("a", 300),
			description: "long references under the cap are allowed",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := policy.ValidateGitReference(testCase.ref)
			if testCase.expectedErr != nil {
				check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestRequiresSecurePermissions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "GPG public key",
			path: "/path/to/key.gpg",
			want: false,
		},
		{
			name: "PEM private key",
			path: "/path/to/key.pem",
			want: true,
		},
		{
			name: "SSH directory",
			path: "/home/user/.ssh/id_rsa",
			want: true,
		},
		{
			name: "Git directory",
			path: "/repo/.git/config",
			want: false,
		},
		{
			name: "Private directory",
			path: "/home/user/private/secret.txt",
			want: false,
		},
		{
			name: "Regular file",
			path: "/path/to/README.md",
			want: false,
		},
		{
			name: "Config file",
			path: "/path/to/settings.yaml",
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := RequiresSecurePermissions(testCase.path)
			check.Equal(t, testCase.want, result)
		})
	}
}

func TestValidateCommitRange(t *testing.T) {
	t.Parallel()

	policy := DefaultSecurityPolicy()

	tcs := []struct {
		name        string
		commitRange string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid two-dot range with SHAs",
			commitRange: "abc123..def456",
			expectError: false,
		},
		{
			name:        "three-dot range is unsupported",
			commitRange: "abc123...def456",
			expectError: true,
			expectedErr: ErrCommitRangeFormat,
		},
		{
			name:        "valid range with branch names",
			commitRange: "main..feature-branch",
			expectError: false,
		},
		{
			name:        "valid range with HEAD reference",
			commitRange: "HEAD~5..HEAD",
			expectError: false,
		},
		{
			name:        "valid range with tag names",
			commitRange: "v1.0.0..v2.0.0",
			expectError: false,
		},
		{
			name:        "valid range with mixed references",
			commitRange: "abc123..main",
			expectError: false,
		},
		{
			name:        "valid range with spaces around refs",
			commitRange: " abc123 .. def456 ",
			expectError: false,
		},
		{
			name:        "range with unsupported dot sequence in second part",
			commitRange: "abc123..../../etc/passwd",
			expectError: true,
			expectedErr: ErrCommitRangeFormat,
		},
		{
			name:        "four dots is invalid",
			commitRange: "abc123....def456",
			expectError: true,
			expectedErr: ErrCommitRangeFormat,
		},
		{
			name:        "empty range",
			commitRange: "",
			expectError: true,
			expectedErr: ErrCommitRangeEmpty,
		},
		{
			name:        "single commit without range operator",
			commitRange: testHashABC123,
			expectError: true,
			expectedErr: ErrCommitRangeFormat,
		},
		{
			name:        "range with semicolon metacharacter",
			commitRange: "abc123..def456; rm -rf /",
			expectError: true,
			expectedErr: ErrGitRefDangerous,
		},
		{
			name:        "range with null bytes",
			commitRange: "abc123\x00..def456",
			expectError: true,
			expectedErr: ErrGitRefDangerous,
		},
		{
			name:        "range with path traversal in first part",
			commitRange: "../etc/passwd..def456",
			expectError: true,
			expectedErr: ErrCommitRangeParts,
		},
		{
			name:        "range with too many parts",
			commitRange: "abc123..def456..ghi789",
			expectError: true,
			expectedErr: ErrCommitRangeParts,
		},
		{
			name:        "range with empty first part",
			commitRange: "..def456",
			expectError: true,
			expectedErr: ErrGitRefEmpty,
		},
		{
			name:        "range with empty second part",
			commitRange: "abc123..",
			expectError: true,
			expectedErr: ErrGitRefEmpty,
		},
		{
			name:        "range with backticks",
			commitRange: "abc123..`cat /etc/passwd`",
			expectError: true,
			expectedErr: ErrGitRefDangerous,
		},
		{
			name:        "range with dollar parentheses",
			commitRange: "abc123..$(whoami)",
			expectError: true,
			expectedErr: ErrGitRefDangerous,
		},
		{
			name:        "range with pipe character",
			commitRange: "abc123..def456 | grep secret",
			expectError: true,
			expectedErr: ErrGitRefDangerous,
		},
		{
			name:        "single dot is invalid",
			commitRange: "abc123.def456",
			expectError: true,
			expectedErr: ErrCommitRangeFormat,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := policy.ValidateCommitRange(testCase.commitRange)

			if testCase.expectError {
				check.Error(t, err)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr)
				}
			} else {
				check.NoError(t, err)
			}
		})
	}
}

// TestParseSecureCommitRange_TrimsRefsAndReturnsParts covers the parsed value
// TestValidateCommitRange discards: refs are trimmed and split into From/To.
func TestParseSecureCommitRange_TrimsRefsAndReturnsParts(t *testing.T) {
	t.Parallel()

	policy := DefaultSecurityPolicy()

	parsed, err := policy.ParseSecureCommitRange("main..feature")
	check.NoError(t, err)
	check.Equal(t, CommitRange{From: "main", To: "feature"}, parsed)

	parsed, err = policy.ParseSecureCommitRange(" abc123 .. def456 ")
	check.NoError(t, err)
	check.Equal(t, CommitRange{From: "abc123", To: "def456"}, parsed)
}

func TestIsSecureFileMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode os.FileMode
		want bool
	}{
		{
			name: "0600 secure",
			mode: 0o600,
			want: true,
		},
		{
			name: "0644 secure (no world write)",
			mode: 0o644,
			want: true,
		},
		{
			name: "0666 insecure (world writable)",
			mode: 0o666,
			want: false,
		},
		{
			name: "0620 insecure (group writable)",
			mode: 0o620,
			want: false,
		},
		{
			name: "0602 insecure (world writable)",
			mode: 0o602,
			want: false,
		},
		{
			name: "0755 secure (no world write for files)",
			mode: 0o755,
			want: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := IsSecureFileMode(testCase.mode)
			check.Equal(t, testCase.want, result)
		})
	}
}
