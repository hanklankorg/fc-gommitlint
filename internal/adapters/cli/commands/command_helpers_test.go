// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/i18n"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func testLocalizer(t *testing.T, locale string) output.Localizer {
	t.Helper()

	localeDir, err := filepath.Abs("../../../../locale")
	check.NoError(t, err)

	return i18n.NewLocalizerFromDir(locale, localeDir)
}

func hookCommandTestRoot(repo string, writer io.Writer) *cli.Command {
	return &cli.Command{
		Name:      gommitlintKey,
		Writer:    writer,
		ErrWriter: io.Discard,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flagNameRepoPath, Value: repo},
			&cli.StringFlag{Name: flagNameConfig},
			&cli.BoolFlag{Name: flags.Quiet},
			&cli.StringFlag{Name: flags.Theme, Value: output.DefaultTheme},
			&cli.StringFlag{Name: flagNameColor, Value: "never"},
		},
		Commands: []*cli.Command{NewHookCommand(nil)},
	}
}

func TestRejectUnexpectedArgs_ClassifiesFirstArgumentAndSuggestsRecovery(t *testing.T) {
	const suggestion = "Use flags instead"

	cmd := &cli.Command{
		Name: "test",
		Action: func(_ context.Context, cmd *cli.Command) error {
			return rejectUnexpectedArgs(cmd, nil, "gommitlint test", suggestion)
		},
	}

	err := cmd.Run(t.Context(), []string{"test", "first", "second"})
	check.ErrorIs(t, err, ErrUnexpectedArgument)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.Equal(t, "unexpected argument: first", recoveryErr.Msg.String())
	check.Equal(t, []string{suggestion, "Run 'gommitlint test --help' for more information"}, recoveryErr.Suggestions)
}
