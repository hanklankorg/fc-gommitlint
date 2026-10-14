//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"bytes"
	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// TestCLIApp_MapsEachArgumentSetToItsOutcome assembles the CLI in process and
// runs argument sets through it, checking that each ends the way the CLI
// contract says it should: error or success, and where it matters the shape of
// what was written.
//
//nolint:varnamelen,tparallel // Direct CLI execution is serialized below.
func TestCLIApp_MapsEachArgumentSetToItsOutcome(t *testing.T) {
	lockDirectCLIExecution(t)
	t.Setenv("GOMMITLINT_NO_CI_DETECT", "1")

	t.Run("main function integration via app execution", func(t *testing.T) {
		repoDir := gitrepo.QuickRepoPath(t, "feat: add new feature")
		// hook status reports on .git/hooks, which only a disk repository has;
		// the virtual refusal is pinned in the in-memory table next door.
		diskRepoDir := gitrepo.PathWithCommits(t, "feat: add new feature")
		configPath := testkit.InMemoryConfigPath(t)

		ctx := context.Background()

		tcs := []struct {
			name         string
			commands     []string
			repoPath     string // empty means the in-memory repository
			expectOutput bool
			description  string
		}{
			{
				name:         "config show command execution",
				commands:     []string{"config", "show"},
				expectOutput: true,
				description:  "Should execute config show command",
			},
			{
				name:         "hook status command execution",
				commands:     []string{"hook", "status"},
				repoPath:     diskRepoDir,
				expectOutput: true,
				description:  "Should execute hook status command",
			},
			{
				// A passing validation with nothing to report writes nothing,
				// so only successful completion is asserted.
				name:        "validate command execution",
				commands:    []string{"validate"},
				description: "Should execute validate command on valid commit",
			},
		}

		for _, testCase := range tcs {
			t.Run(testCase.name, func(t *testing.T) {
				caseRepoDir := repoDir
				if testCase.repoPath != "" {
					caseRepoDir = testCase.repoPath
				}

				app := &cli.Command{
					Name: "gommitlint",
					Commands: []*cli.Command{
						commands.NewConfigCommand(nil),
						commands.NewHookCommand(nil),
						commands.NewValidateCommand(nil),
					},
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "repo-path", Value: caseRepoDir},
						&cli.StringFlag{Name: "config"},
						&cli.StringFlag{Name: "format", Value: output.FormatText},
						&cli.StringFlag{Name: "log-level", Value: logadapter.LogLevelInfoName},
					},
				}
				attachInMemoryRepositoryFactory(app, caseRepoDir)

				var output bytes.Buffer

				app.Writer = &output

				for _, subCmd := range app.Commands {
					subCmd.Writer = &output
					for _, subSubCmd := range subCmd.Commands {
						subSubCmd.Writer = &output
					}
				}

				args := []string{"gommitlint"}
				args = append(args, testCase.commands...)
				args = append(args, "--repo-path", caseRepoDir, "--config", configPath)

				err := app.Run(ctx, args)
				check.NoError(t, err, testCase.description)

				if testCase.expectOutput {
					check.NotEmpty(t, output.String(), "Command should generate output")
				}
			})
		}
	})

	t.Run("main function error handling and logging paths", func(t *testing.T) {
		repoDir := gitrepo.QuickRepoPath(t, "feat: add integration test feature")
		configPath := testkit.InMemoryConfigPath(t)

		ctx := context.Background()

		errorTests := []struct {
			name        string
			logLevel    string
			format      string
			repoPath    string // empty means the valid test repository
			expectError bool
			description string
		}{
			{
				name:        "validation with debug logging",
				logLevel:    logadapter.LogLevelDebugName,
				format:      output.FormatText,
				description: "Valid commit should pass with debug logging enabled",
			},
			{
				name:        "validation with json format",
				format:      output.FormatJSON,
				description: "Should use JSON format",
			},
			{
				name:        "validation with gitlab format",
				format:      output.FormatGitLab,
				description: "Should use GitLab format",
			},
			{
				name:        "invalid repository path",
				repoPath:    "/nonexistent/path",
				expectError: true,
				description: "Should handle invalid repository paths",
			},
		}

		for _, testCase := range errorTests {
			t.Run(testCase.name, func(t *testing.T) {
				app := &cli.Command{
					Name: "gommitlint",
					Commands: []*cli.Command{
						commands.NewValidateCommand(nil),
					},
					Flags: []cli.Flag{
						&cli.StringFlag{Name: "repo-path"},
						&cli.StringFlag{Name: "config"},
						&cli.StringFlag{Name: "format"},
						&cli.StringFlag{Name: "log-level", Value: logadapter.LogLevelInfoName},
					},
				}
				attachInMemoryRepositoryFactory(app, repoDir)

				var cliOutput, stdoutOutput bytes.Buffer

				app.Writer = &cliOutput
				for _, subCmd := range app.Commands {
					subCmd.Writer = &cliOutput
					for _, subSubCmd := range subCmd.Commands {
						subSubCmd.Writer = &cliOutput
					}
				}

				oldStdout := os.Stdout
				r, w, _ := os.Pipe()
				os.Stdout = w

				args := []string{"gommitlint", "validate"}
				if testCase.format != "" {
					args = append(args, "--format", testCase.format)
				}

				if testCase.logLevel != "" {
					args = append(args, "--log-level", testCase.logLevel)
				}

				if testCase.repoPath != "" {
					args = append(args, "--repo-path", testCase.repoPath)
				} else {
					args = append(args, "--repo-path", repoDir, "--config", configPath)
				}

				err := app.Run(ctx, args)

				_ = w.Close()
				os.Stdout = oldStdout
				_, _ = io.Copy(&stdoutOutput, r)
				_ = r.Close()

				if testCase.expectError {
					check.Error(t, err, testCase.description)
				} else {
					check.NoError(t, err, testCase.description)
				}

				outputStr := cliOutput.String() + stdoutOutput.String()
				if testCase.format == output.FormatJSON && !testCase.expectError {
					hasJSON := strings.Contains(outputStr, `"summary"`) ||
						strings.Contains(outputStr, `"commits"`)
					check.True(t, hasJSON,
						"JSON format should produce the report document, got: %s", outputStr)
				}
			})
		}
	})

	// Named for the deadline, not for signals: nothing here sends one. The
	// context bounds the run, so a validate that hung would return a deadline
	// error instead of blocking the suite. Real signal delivery is covered by
	// TestSignalCause_RealSignalResolvesToPosixExitCode in adapters/signal and
	// by TestSignalCancellation_BlackBox against the built binary.
	t.Run("validate completes within its deadline", func(t *testing.T) {
		repoDir := gitrepo.QuickRepoPath(t, "feat: test deadline handling")
		configPath := testkit.InMemoryConfigPath(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		app := &cli.Command{
			Name: "gommitlint",
			Commands: []*cli.Command{
				commands.NewValidateCommand(nil),
			},
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "repo-path", Value: repoDir},
				&cli.StringFlag{Name: "config"},
				&cli.StringFlag{Name: "format", Value: output.FormatText},
			},
		}
		attachInMemoryRepositoryFactory(app, repoDir)

		var output bytes.Buffer

		app.Writer = &output

		for _, subCmd := range app.Commands {
			subCmd.Writer = &output
			for _, subSubCmd := range subCmd.Commands {
				subSubCmd.Writer = &output
			}
		}

		args := []string{"gommitlint", "validate", "--repo-path", repoDir, "--config", configPath}

		err := app.Run(ctx, args)

		// A hang shows up here as the context deadline rather than as a stuck
		// suite. Output is deliberately not asserted: a passing validation with
		// nothing to report writes nothing, and the claim is about finishing.
		check.NoError(t, err, "validate should finish inside the deadline")
	})
}
