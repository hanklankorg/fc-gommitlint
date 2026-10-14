//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"bytes"
	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
	"context"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// TestCLIApp_MapsEachCaseToItsOutcomeOnInMemoryRepositories exercises config,
// hook, and validation commands without a disk-backed repository.
func TestCLIApp_MapsEachCaseToItsOutcomeOnInMemoryRepositories(t *testing.T) {
	lockDirectCLIExecution(t)
	t.Setenv("GOMMITLINT_NO_CI_DETECT", "1")

	t.Run("main function integration with in-memory repos", func(t *testing.T) {
		repoPath := gitrepo.QuickRepoPath(t, "feat: add new feature")
		// hook status reads .git/hooks from disk, so it is the one command here
		// a virtual repository cannot express. It gets a disk repository for the
		// success path, and the mem:// row below pins the refusal that the
		// product answers with instead.
		diskRepoPath := gitrepo.PathWithCommits(t, "feat: add new feature")
		configPath := testkit.InMemoryConfigPath(t)

		ctx := context.Background()

		tcs := []struct {
			name        string
			commands    []string
			repoPath    string // empty means the in-memory repository
			wantErr     error  // nil means the run should succeed
			wantInError string
			description string
		}{
			{
				name:        "config show command",
				commands:    []string{"config", "show"},
				description: "Should show configuration",
			},
			{
				name:        "hook status command",
				commands:    []string{"hook", "status"},
				repoPath:    diskRepoPath,
				description: "Should show hook status",
			},
			{
				name:        "hook status command on a virtual repository",
				commands:    []string{"hook", "status"},
				wantErr:     cliadapter.ErrInvalidRepoPath,
				wantInError: "virtual repositories do not support filesystem hooks",
				description: "Should refuse filesystem hooks on a virtual repository",
			},
			{
				name:        "validate command",
				commands:    []string{"validate"},
				description: "Should validate commit",
			},
		}

		for _, testCase := range tcs {
			t.Run(testCase.name, func(t *testing.T) {
				caseRepoPath := repoPath
				if testCase.repoPath != "" {
					caseRepoPath = testCase.repoPath
				}

				app := &cli.Command{
					Name: "gommitlint",
					Commands: []*cli.Command{
						commands.NewConfigCommand(nil),
						commands.NewHookCommand(nil),
						commands.NewValidateCommand(nil),
					},
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "repo-path", Value: caseRepoPath},
						&cli.StringFlag{Name: "config"},
						&cli.StringFlag{Name: "format", Value: output.FormatText},
						&cli.StringFlag{Name: "log-level", Value: logadapter.LogLevelErrorName},
					},
				}
				attachInMemoryRepositoryFactory(app, caseRepoPath)

				var output bytes.Buffer
				app.Writer = &output
				for _, cmd := range app.Commands {
					cmd.Writer = &output
					for _, subCmd := range cmd.Commands {
						subCmd.Writer = &output
					}
				}

				args := append([]string{"gommitlint"}, testCase.commands...)
				args = append(args, "--repo-path", caseRepoPath, "--config", configPath)

				err := app.Run(ctx, args)

				if testCase.wantErr != nil {
					check.ErrorIs(t, err, testCase.wantErr, testCase.description)
					check.ErrorContains(t, err, testCase.wantInError, testCase.description)
				} else {
					check.NoError(t, err, testCase.description)
				}
			})
		}
	})

	t.Run("error handling with in-memory repos", func(t *testing.T) {
		configPath := testkit.InMemoryConfigPath(t)

		tests := []struct {
			name        string
			commitMsg   string
			format      string
			expectError bool
		}{
			{
				name:        "invalid commit text format",
				commitMsg:   "bad commit message",
				format:      output.FormatText,
				expectError: true,
			},
			{
				name:        "invalid commit json format",
				commitMsg:   "bad commit message",
				format:      output.FormatJSON,
				expectError: true,
			},
			{
				name:        "valid commit text format",
				commitMsg:   "fix: correct issue",
				format:      output.FormatText,
				expectError: false,
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				repoPath := gitrepo.QuickRepoPath(t, testCase.commitMsg)

				ctx := context.Background()

				app := &cli.Command{
					Name: "gommitlint",
					Commands: []*cli.Command{
						commands.NewValidateCommand(nil),
					},
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "repo-path", Value: repoPath},
						&cli.StringFlag{Name: "config"},
						&cli.StringFlag{Name: "format", Value: testCase.format},
						&cli.StringFlag{Name: "log-level", Value: logadapter.LogLevelErrorName},
					},
				}
				attachInMemoryRepositoryFactory(app, repoPath)

				var output bytes.Buffer
				app.Writer = &output
				for _, cmd := range app.Commands {
					cmd.Writer = &output
				}

				args := []string{"gommitlint", "validate", "--repo-path", repoPath, "--format", testCase.format, "--config", configPath}
				err := app.Run(ctx, args)

				if testCase.expectError {
					check.Error(t, err, "Should fail for invalid commit")
				} else {
					check.NoError(t, err, "Should pass for valid commit")
				}
			})
		}
	})
}
