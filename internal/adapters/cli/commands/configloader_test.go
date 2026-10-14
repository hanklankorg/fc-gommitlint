// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestLoadConfigFromCommand_ResolvesFlagCombinations drives the real loader
// through the flag combinations that have deterministic outcomes. Discovery
// from the current directory (no flags at all) depends on the host checkout,
// so it is covered by the workingDir-based tests instead.
func TestLoadConfigFromCommand_ResolvesFlagCombinations(t *testing.T) {
	t.Parallel()

	newApp := func() *cli.Command {
		return &cli.Command{
			Flags: []cli.Flag{
				&cli.StringFlag{Name: flagNameConfig},
				&cli.BoolFlag{Name: flagNameIgnoreConfig},
			},
		}
	}

	t.Run("conflicting flags are refused", func(t *testing.T) {
		t.Parallel()

		app := newApp()
		check.NoError(t, app.Set(flagNameConfig, "/some/settings.yaml"))
		check.NoError(t, app.Set(flagNameIgnoreConfig, "true"))

		_, err := LoadConfigFromCommandWithWorkingDir(app, t.TempDir())
		check.ErrorIs(t, err, ErrConflictingFlags)
	})

	t.Run("ignore-config loads defaults", func(t *testing.T) {
		t.Parallel()

		app := newApp()
		check.NoError(t, app.Set(flagNameIgnoreConfig, "true"))

		result, err := LoadConfigFromCommandWithWorkingDir(app, t.TempDir())
		check.NoError(t, err)
		check.Equal(t, "defaults (--ignore-config)", result.DisplaySource())
	})

	t.Run("explicit config path is loaded and labeled", func(t *testing.T) {
		t.Parallel()

		workingDir := t.TempDir()
		configName := "test-settings.yaml"
		configPath := filepath.Join(workingDir, configName)
		content := "gommitlint:\n  message:\n    subject:\n      max_length: 72"
		check.NoError(t, os.WriteFile(configPath, []byte(content), 0o600))

		app := newApp()
		check.NoError(t, app.Set(flagNameConfig, configName))

		result, err := LoadConfigFromCommandWithWorkingDir(app, workingDir)
		check.NoError(t, err)
		check.Contains(t, result.DisplaySource(), configName)
		check.Contains(t, result.DisplaySource(), "(--config)")
		check.Equal(t, 72, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
	})
}

func TestConfigResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sourcePath  string
		loadMode    configLoadMode
		wantSource  string
		description string
	}{
		{
			name:        "basic config result",
			sourcePath:  "/test/mock/to/settings.yaml",
			loadMode:    configLoadModeExplicitPath,
			wantSource:  "/test/mock/to/settings.yaml (--config)",
			description: "should create proper config result",
		},
		{
			name:        "default source",
			sourcePath:  "defaults",
			wantSource:  "defaults",
			description: "should handle default source",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ConfigResult{
				Sources:  []configuration.ConfigSource{{Path: testCase.sourcePath, Type: configuration.SourceTypeProject}},
				loadMode: testCase.loadMode,
			}

			check.Equal(t, testCase.wantSource, result.DisplaySource(), testCase.description)
		})
	}
}

// TestLoadConfigFromPath_UnreadableFileErrors keeps the one path-validation
// case the security suite does not cover: a file the process cannot read.
// Traversal, null bytes, and missing files are asserted with error identity in
// configloader_security_test.go through the public flag entry point.
func TestLoadConfigFromPath_UnreadableFileErrors(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "restricted-settings.yaml")
	check.NoError(t, os.WriteFile(configPath, []byte("gommitlint:\n  message:\n    subject:\n      max_length: 50"), 0o000))

	_, err := loadConfigFromPathWithWorkingDir(configPath, "")
	check.Error(t, err, "unreadable config file must fail the load")
}

func TestValidateRepoPath_ErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repoPath    string
		expectError bool
		description string
	}{
		{
			name:        "empty repo path",
			repoPath:    "",
			expectError: false,
			description: "should allow empty repo path",
		},
		{
			name:        "nonexistent repo path",
			repoPath:    "/nonexistent/repo",
			expectError: true,
			description: "should error for nonexistent repository",
		},
		{
			name:        "non-git directory",
			repoPath:    t.TempDir(),
			expectError: false,
			description: "should gracefully degrade for non-git dir — discovery fails, downstream uses defaults",
		},
		{
			name:        "path traversal in repo path",
			repoPath:    "../../etc",
			expectError: true,
			description: "should error on path traversal in repo path",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := cliAdapter.ValidateRepoPath(testCase.repoPath)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestLoadConfigWithRepoPathFromWorkingDir_ErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repoPath    string
		workingDir  string
		expectError bool
		description string
	}{
		{
			name:        "invalid repo path",
			repoPath:    "/nonexistent/repo",
			expectError: true,
			description: "should error when repo path validation fails",
		},
		{
			name:        "empty repo path loads from current directory",
			repoPath:    "",
			workingDir:  "",
			expectError: false,
			description: "should work with empty repo path",
		},
		{
			name:        "path traversal in repo path",
			repoPath:    "../../../etc",
			expectError: true,
			description: "should error on path traversal in repo path",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := loadConfigWithRepoPathFromWorkingDir(testCase.repoPath, testCase.workingDir)

			if testCase.expectError {
				check.Error(t, err, testCase.description)
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}
