// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestHookCommand_HasCorrectStructure(t *testing.T) {
	t.Parallel()

	cmd := NewHookCommand(nil)

	check.Equal(t, commandNameHook, cmd.Name)
	check.Equal(t, "manage Gommitlint Git hooks", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.Contains(t, cmd.Description, "when required by the active configuration")
	check.Len(t, cmd.Commands, 4) // status, install, remove, hidden hook runner

	// Verify subcommands
	subcommands := make(map[string]*cli.Command)
	for _, subcmd := range cmd.Commands {
		subcommands[subcmd.Name] = subcmd
	}

	check.Contains(t, subcommands, commandNameHookStatus)
	check.Contains(t, subcommands, commandNameHookInstall)
	check.Contains(t, subcommands, commandNameHookRemove)
	check.Contains(t, subcommands, commandNameHookRun)
	check.True(t, subcommands[commandNameHookRun].Hidden)
	check.Len(t, subcommands[commandNameHookRun].Commands, 1)
	check.Equal(t, commandNameHookPrePush, subcommands[commandNameHookRun].Commands[0].Name)
	check.True(t, subcommands[commandNameHookRun].Commands[0].Hidden)
}

func TestHookStatusCommand_HasCorrectConfiguration(t *testing.T) {
	t.Parallel()

	cmd := NewHookStatusCommand(nil)

	check.Equal(t, commandNameHookStatus, cmd.Name)
	check.Equal(t, "show current Gommitlint Git hook status", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.NotContains(t, cmd.Description, "configuration")
	check.NotNil(t, cmd.Action)
	check.Len(t, cmd.Flags, 1) // verbose flag

	// Verify verbose flag
	check.Equal(t, flagNameVerbose, cmd.Flags[0].Names()[0])
}

func TestHookInstallCommand_HasCorrectConfiguration(t *testing.T) {
	t.Parallel()

	cmd := NewHookInstallCommand(nil)

	check.Equal(t, commandNameHookInstall, cmd.Name)
	check.Equal(t, "install Gommitlint Git hooks", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.Contains(t, cmd.Description, "commit-msg and pre-push")
	check.NotNil(t, cmd.Action)
	check.Len(t, cmd.Flags, 2) // force and global flags

	// Verify flags
	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		flagNames[flag.Names()[0]] = true
		if flag.Names()[0] == flagNameForce {
			forceFlag, ok := flag.(*cli.BoolFlag)
			check.True(t, ok)
			check.Contains(t, forceFlag.Usage, "existing Gommitlint hooks")
			check.NotContains(t, forceFlag.Usage, "overwrite existing hook if it exists")
		}

		if flag.Names()[0] == flagNameGlobal {
			check.Len(t, flag.Names(), 1, "global flag should not have aliases")
			globalFlag, ok := flag.(*cli.BoolFlag)
			check.True(t, ok)
			check.NotContains(t, globalFlag.Usage, "applies to all repositories")
		}
	}

	check.True(t, flagNames["force"], "missing force flag")
	check.True(t, flagNames[flagNameGlobal], "missing global flag")
}

func TestHookRemoveCommand_HasCorrectConfiguration(t *testing.T) {
	t.Parallel()

	cmd := NewHookRemoveCommand(nil)

	check.Equal(t, commandNameHookRemove, cmd.Name)
	check.Equal(t, "remove Gommitlint Git hooks", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.Contains(t, cmd.Description, "preserving custom hooks")
	check.NotNil(t, cmd.Action)
	check.Len(t, cmd.Flags, 2) // global and dry-run flags

	// Verify global flag exists and has no aliases
	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		flagNames[flag.Names()[0]] = true
		if flag.Names()[0] == flagNameGlobal {
			check.Len(t, flag.Names(), 1, "global flag should not have aliases")
			globalFlag, ok := flag.(*cli.BoolFlag)
			check.True(t, ok)
			check.Contains(t, globalFlag.Usage, "restore, unset, or preserve git config core.hooksPath")
		}

		if flag.Names()[0] == "dry-run" {
			dryRunFlag, ok := flag.(*cli.BoolFlag)
			check.True(t, ok)
			check.Contains(t, dryRunFlag.Usage, "repository-local")
		}
	}

	check.True(t, flagNames[flagNameGlobal], "missing global flag")
	check.True(t, flagNames["dry-run"], "missing dry-run flag")
}

func TestHookCommandDescriptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		command       *cli.Command
		shouldContain []string
	}{
		{
			name:    "hook main command description",
			command: NewHookCommand(nil),
			shouldContain: []string{
				"Manage Gommitlint Git hooks",
				commandNameHookStatus,
				commandNameHookInstall,
				commandNameHookRemove,
				testExamplesHeader,
			},
		},
		{
			name:    "hook status description",
			command: NewHookStatusCommand(nil),
			shouldContain: []string{
				"Shows the current state",
				testExamplesHeader,
			},
		},
		{
			name:    "hook install description",
			command: NewHookInstallCommand(nil),
			shouldContain: []string{
				"Installs commit-msg and pre-push Git hooks",
				testExamplesHeader,
				"--force",
			},
		},
		{
			name:    "hook remove description",
			command: NewHookRemoveCommand(nil),
			shouldContain: []string{
				"Removes Gommitlint-managed Git hooks",
				testExamplesHeader,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			description := testCase.command.Description
			check.NotEmpty(t, description, "command should have description")

			for _, expectedText := range testCase.shouldContain {
				check.Contains(t, description, expectedText,
					"description should contain '%s'", expectedText)
			}
		})
	}
}
