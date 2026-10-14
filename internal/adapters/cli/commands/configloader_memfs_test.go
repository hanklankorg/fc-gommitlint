// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
)

// TestConfigLoaderWithMockCommand tests config loading with mock CLI commands.
func TestConfigLoaderWithMockCommand(t *testing.T) {
	// Not parallel: t.Chdir is process-wide. The default-loading path reads
	// ambient config from the working directory, so these subtests must run
	// from an empty directory — the repository's own .gommitlint.yaml would
	// otherwise leak into the "defaults" assertions.
	t.Run("default config loading", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// The user settings file is discovered under XDG_CONFIG_HOME, else HOME.
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		t.Setenv("HOME", t.TempDir())

		// Create a mock command
		app := &cli.Command{
			Name: "test",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "config"},
				&cli.BoolFlag{Name: "ignore-config"},
				&cli.StringFlag{Name: "repo-path"},
			},
		}

		// No flags set, should load defaults
		result, err := commands.LoadConfigFromCommand(app)
		check.NoError(t, err)
		check.NotNil(t, result.Config)
		check.Empty(t, result.Config.Rules.Enabled)
		check.Empty(t, result.Config.Rules.Disabled)
	})

	t.Run("ignore config flag", func(t *testing.T) {
		t.Chdir(t.TempDir())

		// Create a mock command with ignore-config flag
		app := &cli.Command{
			Name: "test",
			Flags: []cli.Flag{
				&cli.StringFlag{Name: "config"},
				&cli.BoolFlag{
					Name:  "ignore-config",
					Value: true,
				},
				&cli.StringFlag{Name: "repo-path"},
			},
		}

		result, err := commands.LoadConfigFromCommand(app)
		check.NoError(t, err)
		check.Equal(t, "defaults (--ignore-config)", result.DisplaySource())
	})
}
