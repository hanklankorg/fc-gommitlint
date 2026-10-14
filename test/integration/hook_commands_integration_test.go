//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// TestHookCommands_LifecycleWorkflows drives status, install and remove
// through the real CLI against real repositories.
func TestHookCommands_LifecycleWorkflows(t *testing.T) {
	t.Run("hook status integration workflow", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repoDir}, repoDir)
		check.NoError(t, err)

		check.Contains(t, outputStr, "Gommitlint Git Hooks", "Hook status should contain header")
	})

	t.Run("hook install integration workflow", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--force", "--repo-path", repoDir}, repoDir)

		check.NoError(t, err, "hook install should succeed in a fresh repository")
		check.NotEmpty(t, outputStr)

		hookPath := filepath.Join(repoDir, ".git", "hooks", "commit-msg")

		content, readErr := os.ReadFile(hookPath)
		check.NoError(t, readErr, "install should have written .git/hooks/commit-msg")
		check.Contains(t, string(content), "gommitlint",
			"the installed hook should be the one gommitlint generates")

		// An unexecutable hook is skipped by git in silence, which is the
		// failure mode a "the file exists" check cannot see.
		info, statErr := os.Stat(hookPath)
		check.NoError(t, statErr)
		check.True(t, info.Mode().Perm()&0o111 != 0,
			"the hook must be executable, got mode %v", info.Mode().Perm())
	})

	t.Run("hook install with existing hook", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		hooksDir := filepath.Join(repoDir, ".git", "hooks")
		err := os.MkdirAll(hooksDir, FilePermPublicExec)
		check.NoError(t, err)

		existingHook := filepath.Join(hooksDir, "commit-msg")
		existingContent := "#!/bin/sh\n# existing hook\nexit 0\n"
		err = os.WriteFile(existingHook, []byte(existingContent), FilePermPublicExec)
		check.NoError(t, err)

		_, err = RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repoDir}, repoDir)
		check.Error(t, err, "Should fail with custom hook without --force")

		_, err = RunCLICommand(t, []string{"gommitlint", "hook", "install", "--force", "--repo-path", repoDir}, repoDir)
		check.Error(t, err, "Should fail even with --force for custom hooks (safety feature)")
		check.ErrorIs(t, err, commands.ErrCustomHookExists)

		content, readErr := os.ReadFile(existingHook)
		check.NoError(t, readErr, "both refusals must leave the custom hook in place")
		check.Equal(t, existingContent, string(content), "a refused install must not change the custom hook")
	})

	t.Run("hook status refuses a directory that is not a repository", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", tmpDir}, tmpDir)
		check.Error(t, err, "hook status must fail when the directory holds no repository")
	})

	t.Run("hook status with config integration", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		configContent := `gommitlint:
  rules:
    conventionalcommit:
      enabled: true`

		configFile := filepath.Join(repoDir, ".gommitlint.yaml")
		err := os.WriteFile(configFile, []byte(configContent), FilePermPublicRead)
		check.NoError(t, err)

		outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repoDir}, repoDir)
		check.NoError(t, err)

		check.Contains(t, outputStr, "Gommitlint Git Hooks", "Should show hook header")
	})

	t.Run("hook remove refuses a hook installed by another tool", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		hooksDir := filepath.Join(repoDir, ".git", "hooks")
		check.NoError(t, os.MkdirAll(hooksDir, FilePermPublicExec))

		hookFile := filepath.Join(hooksDir, "commit-msg")
		foreignHookContent := `#!/bin/sh
# Gommitlint hook
gommitlint validate
`
		check.NoError(t, os.WriteFile(hookFile, []byte(foreignHookContent), FilePermPublicExec))

		_, err := RunCLICommand(t, []string{"gommitlint", "hook", "remove", "--repo-path", repoDir}, repoDir)
		check.Error(t, err, "remove must refuse a hook gommitlint did not install")
		check.ErrorIs(t, err, commands.ErrNotGommitlintHook)

		content, readErr := os.ReadFile(hookFile)
		check.NoError(t, readErr, "the foreign hook must be left in place")
		check.Equal(t, foreignHookContent, string(content))
	})

	t.Run("full hook lifecycle integration", func(t *testing.T) {
		repoDir := gitrepo.PathWithCommits(t, "feat: initial commit")

		initialStatus, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repoDir}, repoDir)
		check.NoError(t, err)
		check.Contains(t, initialStatus, "Gommitlint Git Hooks", "Should show hook header")

		installOutput, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repoDir}, repoDir)
		check.NoError(t, err)
		check.Contains(t, installOutput, "installed successfully", "Should show successful installation")

		finalStatus, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repoDir}, repoDir)
		check.NoError(t, err)
		check.Contains(t, finalStatus, "commit-msg", "Should show commit-msg hook installed")
		check.Contains(t, finalStatus, "installed", "Should show hooks are installed")
	})
}

// TestHookCommands_RejectMaliciousPaths checks path-policy rejection before
// hook commands access the filesystem.
func TestHookCommands_RejectMaliciousPaths(t *testing.T) {
	t.Parallel()

	nullBytePath := filepath.Join(t.TempDir(), "\x00malicious")

	tests := []struct {
		name      string
		args      []string
		repoPath  string
		expectErr error // nil when the command reports a generic failure
	}{
		{
			name:     "hook status refuses a traversing repo path",
			repoPath: "../../../etc/passwd",
			args:     []string{"gommitlint", "hook", "status", "--repo-path", "../../../etc/passwd"},
		},
		{
			name:      "hook install refuses a path containing a null byte",
			repoPath:  nullBytePath,
			args:      []string{"gommitlint", "hook", "install", "--repo-path", nullBytePath},
			expectErr: domain.ErrPathDangerous,
		},
		{
			name:      "hook remove refuses a traversing repo path",
			repoPath:  "../../../../bin/sh",
			args:      []string{"gommitlint", "hook", "remove", "--repo-path", "../../../../bin/sh"},
			expectErr: domain.ErrPathTraversal,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := RunCLICommand(t, testCase.args, testCase.repoPath)

			check.Error(t, err, "a hostile repo path must be refused, not acted on")

			if testCase.expectErr != nil {
				check.ErrorIs(t, err, testCase.expectErr,
					"the refusal should name why the path was rejected")
			}
		})
	}
}

// TestHookCommands_RefuseUnusableRepositories covers nonexistent paths and
// directories that are not Git repositories.
func TestHookCommands_RefuseUnusableRepositories(t *testing.T) {
	t.Parallel()

	emptyDir := t.TempDir()

	tests := []struct {
		name      string
		args      []string
		repoPath  string
		expectErr error // nil when the command reports a generic failure
	}{
		{
			name:     "hook status on a path that does not exist",
			repoPath: "/nonexistent/path",
			args:     []string{"gommitlint", "hook", "status", "--repo-path", "/nonexistent/path"},
		},
		{
			name:      "hook remove on a directory that is not a repository",
			repoPath:  emptyDir,
			args:      []string{"gommitlint", "hook", "remove", "--repo-path", emptyDir},
			expectErr: fileops.ErrNotGitRepository,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := RunCLICommand(t, testCase.args, testCase.repoPath)

			check.Error(t, err, "an unusable repository path must be refused")

			if testCase.expectErr != nil {
				check.ErrorIs(t, err, testCase.expectErr,
					"the refusal should name why the path is unusable")
			}
		})
	}
}
