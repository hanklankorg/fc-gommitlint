// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"codefloe.com/itiquette/gommitlint/internal/adapters/i18n"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

func commandLocalizer() output.Localizer {
	return i18n.NewLocalizerFromEnv()
}

func rejectUnexpectedArgs(cmd *cli.Command, localizer output.Localizer, command string, suggestion string) error {
	args := cmd.Args()
	if args == nil || args.Len() == 0 {
		return nil
	}

	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrUnexpectedArgument,
		output.Msg("unexpected argument: %s", args.Get(0)),
		suggestion,
		helpSuggestion(loc, command),
	)
}
