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

// TestHookInstall_InstallsFreshAndRefusesExisting covers the install
// outcomes: a fresh repository installs; existing gommitlint or custom
// hooks refuse without --force.
func TestHookInstall_InstallsFreshAndRefusesExisting(t *testing.T) {
	tcs := []struct {
		name        string
		setupRepo   func(t *testing.T) *gitrepo.TestRepo
		expectError bool
		validate    func(t *testing.T, repo *gitrepo.TestRepo)
	}{
		{
			name:        "install hook in empty repository",
			setupRepo:   gitrepo.NewRepoForHooks,
			expectError: false,
			validate: func(t *testing.T, repo *gitrepo.TestRepo) {
				t.Helper()
				hookPath := filepath.Join(repo.Path, ".git", "hooks", "commit-msg")
				check.FileExists(t, hookPath, "Hook should be installed")
			},
		},
		{
			name:        "install hook with existing gommitlint hook should fail without force",
			setupRepo:   gitrepo.NewRepoWithGommitlintHook,
			expectError: true, // Should fail without --force even for existing gommitlint hooks
		},
		{
			name:        "install hook with existing custom hook without force",
			setupRepo:   gitrepo.NewRepoWithCustomHook,
			expectError: true, // Should fail without --force flag
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			repo := testCase.setupRepo(t)
			hookPath := filepath.Join(repo.Path, ".git", "hooks", "commit-msg")

			var before []byte

			if testCase.expectError {
				var readErr error

				before, readErr = os.ReadFile(hookPath)
				check.NoError(t, readErr, "the fixture must have installed a hook to refuse over")
			}

			outputStr, err := RunCLICommand(t, []string{"gommitlint", "hook", "install", "--repo-path", repo.Path}, repo.Path)

			if testCase.expectError {
				check.Error(t, err)

				after, readErr := os.ReadFile(hookPath)
				check.NoError(t, readErr, "a refused install must leave the existing hook in place")
				check.Equal(t, string(before), string(after), "a refused install must not change the existing hook")
			} else {
				check.NoError(t, err)
				check.NotEmpty(t, outputStr, "Hook install should produce output")

				if testCase.validate != nil {
					testCase.validate(t, repo)
				}
			}
		})
	}
}

// TestExecuteInstallHook_ReportsInvalidRepoDiagnostic pins the diagnostic
// identity when install is pointed at a path with no repository.
func TestExecuteInstallHook_ReportsInvalidRepoDiagnostic(t *testing.T) {
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

		err := commands.ExecuteInstallHook(ctx, cmd)
		check.Error(t, err, "Should return error for invalid repository")
		var validationErr domain.ValidationError
		check.ErrorAs(t, err, &validationErr)
		check.Equal(t, diagnostics.ReasonInvalidRepo, validationErr.Diagnostic.Reason)
	})
}
