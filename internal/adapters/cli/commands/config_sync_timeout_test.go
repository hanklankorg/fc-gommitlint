// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func syncCommandForTest(t *testing.T, stdout *bytes.Buffer) *cli.Command {
	t.Helper()

	return &cli.Command{
		Name:     "gommitlint",
		Writer:   stdout,
		Commands: []*cli.Command{NewConfigCommand(nil)},
	}
}

func TestConfigSync_TimeoutFlagDefaultsToSyncDefault(t *testing.T) {
	t.Parallel()

	var found *cli.DurationFlag

	for _, sub := range NewConfigCommand(nil).Commands {
		if sub.Name != commandNameConfigSync {
			continue
		}

		for _, flag := range sub.Flags {
			if durationFlag, ok := flag.(*cli.DurationFlag); ok && durationFlag.Name == flagNameTimeout {
				found = durationFlag
			}
		}
	}

	check.NotNil(t, found, "config sync should declare --timeout")
	check.Equal(t, configuration.DefaultSyncTimeout, found.Value)
}

func TestConfigSync_TimeoutFlagRefusesNonPositiveDurations(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"0", "-1s"} {
		var stdout bytes.Buffer

		err := syncCommandForTest(t, &stdout).Run(t.Context(), []string{"gommitlint", "config", "sync", "--timeout", raw})
		check.Error(t, err, "--timeout "+raw+" must be refused")
		check.Contains(t, err.Error(), "positive")
	}
}

func TestConfigSync_HelpShowsTimeoutPlaceholder(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	err := syncCommandForTest(t, &stdout).Run(t.Context(), []string{"gommitlint", "config", "sync", "--help"})
	check.NoError(t, err)
	check.Contains(t, stdout.String(), "--timeout TIMEOUT")
	check.Contains(t, stdout.String(), (10 * time.Second).String())
}
