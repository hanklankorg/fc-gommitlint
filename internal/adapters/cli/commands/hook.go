// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"github.com/urfave/cli/v3"
)

// Hook type constants used across hook install, remove, and status commands.
const (
	commandNameHook   = "hook"
	hookTypeCommitMsg = apphooks.HookTypeCommitMsg
	hookTypePrePush   = apphooks.HookTypePrePush
)

// NewHookCommand creates the hook parent command with install and remove subcommands.
func NewHookCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameHook,
		Usage: localizer.Get("manage Gommitlint Git hooks"),
		Description: localizer.Get(`Manage Gommitlint Git hooks for automatic commit message and signature validation.

Gommitlint manages two hooks:
  commit-msg   Validates message format (blocks invalid commits)
  pre-push     Validates signatures when required by the active configuration

Examples:
  # Show current hook status
  gommitlint hook status

  # Install hooks in the current repository
  gommitlint hook install

  # Install hooks globally
  gommitlint hook install --global

  # Remove hooks from the current repository
  gommitlint hook remove`),

		Commands: []*cli.Command{
			NewHookStatusCommand(localizer),
			NewHookInstallCommand(localizer),
			NewHookRemoveCommand(localizer),
			newHookRunCommand(localizer),
		},
	}
}
