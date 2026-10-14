//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
)

// This file focuses on CLI flag interactions and command-line specific config handling.

// TestLoadConfigFromCommand_FlagPrecedence pins --config beating
// --repo-path, and --ignore-config beating everything.
func TestLoadConfigFromCommand_FlagPrecedence(t *testing.T) {
	t.Parallel()

	currentRepo := gitrepo.NewEmptyRepo(t)
	repoRepo := gitrepo.NewEmptyRepo(t)
	currentDir := currentRepo.Path
	repoDir := repoRepo.Path

	currentConfigContent := testhelpers.MinimalConfigYAML(output.FormatJSON)
	testkit.WriteRepoConfig(t, currentDir, ".gommitlint.yaml", currentConfigContent)

	repoConfigContent := testhelpers.MinimalConfigYAML(output.FormatText)
	testkit.WriteRepoConfig(t, repoDir, ".gommitlint.yaml", repoConfigContent)

	explicitConfigContent := testhelpers.MinimalConfigYAML(output.FormatGitHub)
	explicitConfigFile := testkit.WriteRepoConfig(t, currentDir, "explicit.yaml", explicitConfigContent)

	tests := []struct {
		name           string
		setupFlags     func(*cli.Command) error
		wantOutput     string
		expectedSource string
		description    string
	}{
		{
			name: "--config flag takes precedence over --repo-path",
			setupFlags: func(app *cli.Command) error {
				if err := app.Set("repo-path", repoDir); err != nil {
					return err
				}
				return app.Set("config", explicitConfigFile)
			},
			wantOutput:     output.FormatGitHub,
			expectedSource: explicitConfigFile + " (--config)",
			description:    "CLI flag precedence: --config > --repo-path",
		},
		{
			name: "--ignore-config overrides all other flags",
			setupFlags: func(app *cli.Command) error {
				if err := app.Set("repo-path", repoDir); err != nil {
					return err
				}
				return app.Set("ignore-config", "true")
			},
			wantOutput:     output.FormatText,
			expectedSource: "defaults (--ignore-config)",
			description:    "CLI flag precedence: --ignore-config overrides all",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "repo-path"},
					&cli.StringFlag{Name: "config"},
					&cli.BoolFlag{Name: "ignore-config"},
				},
			}

			// Setup flags for test case
			err := testCase.setupFlags(app)
			check.NoError(t, err)

			// Test CLI flag behavior
			result, err := commands.LoadConfigFromCommandWithWorkingDir(app, currentDir)
			check.NoError(t, err, testCase.description)

			// Verify CLI flag precedence
			check.Equal(t, testCase.wantOutput, result.Config.Output, testCase.description)
			check.Equal(t, testCase.expectedSource, result.DisplaySource(), testCase.description)
		})
	}
}

// TestLoadConfigFromCommand_RepoPathFindsYAMLConfig covers --repo-path
// driven discovery of .gommitlint.yaml.
func TestLoadConfigFromCommand_RepoPathFindsYAMLConfig(t *testing.T) {
	t.Parallel()

	// Test only YAML format with --repo-path to verify CLI integration
	// Format precedence business logic is tested in use cases
	testRepo := gitrepo.NewEmptyRepo(t)
	testRepoDir := testRepo.Path

	yamlContent := testhelpers.MinimalConfigYAML(output.FormatJSON)
	testkit.WriteRepoConfig(t, testRepoDir, ".gommitlint.yaml", yamlContent)

	t.Run("--repo-path flag finds YAML config", func(t *testing.T) {
		app := &cli.Command{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "repo-path"},
			},
		}

		err := app.Set("repo-path", testRepoDir)
		check.NoError(t, err)

		// Test CLI config loading
		result, err := commands.LoadConfigFromCommand(app)
		check.NoError(t, err)

		// Verify CLI flag correctly loaded config
		check.Equal(t, output.FormatJSON, result.Config.Output)
		check.Contains(t, result.DisplaySource(), ".gommitlint.yaml")
		check.Contains(t, result.DisplaySource(), "(--repo-path)")
	})
}

// TestLoadConfigFromCommand_RejectsBadConfigFlags covers conflicting
// --config/--ignore-config and a nonexistent --config file.
func TestLoadConfigFromCommand_RejectsBadConfigFlags(t *testing.T) {
	t.Parallel()

	workingRepo := gitrepo.NewEmptyRepo(t)
	workingDir := workingRepo.Path

	tests := []struct {
		name        string
		setupFlags  func(*cli.Command) error
		expectError bool
		description string
	}{
		{
			name: "CLI flags: conflicting --config and --ignore-config",
			setupFlags: func(app *cli.Command) error {
				if err := app.Set("config", "config.yaml"); err != nil {
					return err
				}
				return app.Set("ignore-config", "true")
			},
			expectError: true,
			description: "CLI should error when conflicting config flags are specified",
		},
		{
			name: "CLI flags: non-existent --config file",
			setupFlags: func(app *cli.Command) error {
				return app.Set("config", "/nonexistent/config.yaml")
			},
			expectError: true,
			description: "CLI should error when explicit config file doesn't exist",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "repo-path"},
					&cli.StringFlag{Name: "config"},
					&cli.BoolFlag{Name: "ignore-config"},
				},
			}

			// Setup flags
			err := testCase.setupFlags(app)
			check.NoError(t, err)

			// Test CLI error handling
			_, err = commands.LoadConfigFromCommandWithWorkingDir(app, workingDir)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}
