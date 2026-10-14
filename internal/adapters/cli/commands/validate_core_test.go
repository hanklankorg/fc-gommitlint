// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"testing"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestApplyVerbosity_UsesParsedFlagCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          []string
		expectedLevel int
		expectedFile  string
		expectError   bool
	}{
		{name: "none", args: nil},
		{name: "short", args: []string{"-v"}, expectedLevel: 1},
		{name: "long", args: []string{"--verbose"}, expectedLevel: 1},
		{name: "combined short", args: []string{"-vv"}, expectedLevel: 2},
		{name: "repeated short", args: []string{"-v", "-v"}, expectedLevel: 2},
		{name: "too many", args: []string{"-vv", "-v"}, expectError: true},
		{name: "flag-like value", args: []string{"--message-file", "-vv"}, expectedFile: "-vv"},
		{name: "after terminator", args: []string{"--", "-vv"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var options cliAdapter.OutputOptions

			command := &cli.Command{
				Name:                   gommitlintKey,
				UseShortOptionHandling: true,
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: flagNameVerbose, Aliases: []string{"v"}},
					&cli.StringFlag{Name: flagNameMessageFile},
				},
				Action: func(_ context.Context, cmd *cli.Command) error {
					var err error

					options, err = applyVerbosity(cliAdapter.NewOutputOptions(&bytes.Buffer{}), cmd, false, nil)

					return err
				},
			}

			err := command.Run(t.Context(), append([]string{gommitlintKey}, testCase.args...))
			if testCase.expectError {
				check.ErrorIs(t, err, ErrVerboseFlagCount)

				return
			}

			check.NoError(t, err)
			check.Equal(t, testCase.expectedLevel, options.VerboseLevel)
			check.Equal(t, testCase.expectedFile, command.String(flagNameMessageFile))
		})
	}
}
