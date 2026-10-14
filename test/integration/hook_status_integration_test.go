//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

const (
	commitMsgHookType = "commit-msg"
)

// TestHookStatus_ReportsHooksAcrossRepoStates runs hook status against an
// empty repository, one with a foreign hook, and a nonexistent path,
// asserting on the rendered report.
func TestHookStatus_ReportsHooksAcrossRepoStates(t *testing.T) {
	tcs := []struct {
		name         string
		setupRepo    func(t *testing.T) string
		expectError  bool
		wantInOutput []string
	}{
		{
			name:         "repository without a hooks directory",
			setupRepo:    createTestRepoWithoutHooks,
			wantInOutput: []string{"No hooks directory found"},
		},
		{
			// The fixture writes the hook without the executable bit; git
			// would skip such a hook in silence, so status must call it out.
			name:         "repository with a non-executable hook",
			setupRepo:    createTestRepoWithHooks,
			wantInOutput: []string{commitMsgHookType, "not executable"},
		},
		{
			name: "nonexistent repository",
			setupRepo: func(_ *testing.T) string {
				return "/nonexistent/path"
			},
			expectError: true,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			repoPath := testCase.setupRepo(t)

			outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "status", "--repo-path", repoPath}, repoPath)

			if testCase.expectError {
				check.Error(t, err)
			} else {
				check.NoError(t, err)
				check.NotEmpty(t, outputStr, "Hook status should produce output")
				check.Contains(t, outputStr, "Gommitlint Git Hooks", "Output should contain hook header")

				for _, want := range testCase.wantInOutput {
					check.Contains(t, outputStr, want)
				}
			}
		})
	}
}

// TestExecuteHookStatus_ReportsInvalidRepoDiagnostic pins the diagnostic
// identity when status is pointed at a path with no repository.
func TestExecuteHookStatus_ReportsInvalidRepoDiagnostic(t *testing.T) {
	t.Run("error handling with invalid repository", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		ctx = logadapter.InitLogger(ctx, output.FormatText, logadapter.LogLevelInfoName)

		var output strings.Builder

		cmd := &cli.Command{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "repo-path", Value: "/nonexistent/path"},
			},
			Writer: &output,
		}

		err := commands.ExecuteHookStatus(ctx, cmd)
		check.Error(t, err, "Should return error for invalid repository")
		var validationErr domain.ValidationError
		check.ErrorAs(t, err, &validationErr)
		check.Equal(t, diagnostics.ReasonInvalidRepo, validationErr.Diagnostic.Reason)
	})
}

// createTestRepoWithHooks creates a test repository with git hooks using gitrepo utilities.
func createTestRepoWithHooks(t *testing.T) string {
	t.Helper()

	repo := gitrepo.NewEmptyRepo(t)

	hooksDir := filepath.Join(repo.Path, ".git", "hooks")
	check.NoError(t, os.MkdirAll(hooksDir, 0o750))

	commitMsgPath := filepath.Join(hooksDir, "commit-msg")
	hookContent := "#!/bin/sh\necho 'Hook executed'\n"
	check.NoError(t, os.WriteFile(commitMsgPath, []byte(hookContent), 0o600))

	return repo.Path
}

// createTestRepoWithoutHooks creates a test repository without hooks directory using gitrepo utilities.
func createTestRepoWithoutHooks(t *testing.T) string {
	t.Helper()

	repo := gitrepo.NewEmptyRepo(t)

	return repo.Path
}
