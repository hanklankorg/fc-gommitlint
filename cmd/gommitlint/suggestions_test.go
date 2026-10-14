// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"io"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestCommandNotFound_FailsWithSafeRelevantOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		args       []string
		contains   []string
		notContain []string
	}{
		{
			name:     "nested_typo",
			args:     []string{appName, commandConfig, "scheam"}, // typos:disable-line
			contains: []string{`Did you mean "gommitlint config schema"?`},
		},
		{
			name:       "unrelated_root_command",
			args:       []string{appName, "zxqwerty"},
			contains:   []string{"Available commands:"},
			notContain: []string{"Did you mean"},
		},
		{
			name:       "hidden_root_command",
			args:       []string{appName, "translatons"},
			contains:   []string{"Available commands:"},
			notContain: []string{"manage translation catalogs", `Did you mean "gommitlint translations"?`},
		},
		{
			name:       "hidden_nested_command",
			args:       []string{appName, commandHook, "123456"},
			contains:   []string{"Available commands:"},
			notContain: []string{"  run  "},
		},
		{
			name:       "terminal_control_sequence",
			args:       []string{appName, "bad\x1b[2Jfake"},
			contains:   []string{`bad\x1b[2Jfake`},
			notContain: []string{"\x1b"},
		},
		{
			name:     "unknown_help_topic",
			args:     []string{appName, commandHelp, "nonsense"},
			contains: []string{`Unknown command "nonsense"`},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer

			app := createCLIApp(testLocalizer())
			app.Writer = io.Discard
			app.ErrWriter = &stderr

			err := app.Run(t.Context(), testCase.args)

			check.ErrorIs(t, err, ErrUnknownCommand)
			check.Equal(t, int(domain.ExitCodeUsage), resolveExitCode(t.Context(), err))

			for _, expected := range testCase.contains {
				check.Contains(t, stderr.String(), expected)
			}

			for _, forbidden := range testCase.notContain {
				check.NotContains(t, stderr.String(), forbidden)
			}
		})
	}
}

func TestCommandNotFound_PropagatesWriteFailure(t *testing.T) {
	t.Parallel()

	app := createCLIApp(testLocalizer())
	app.Writer = io.Discard
	app.ErrWriter = failingWriter{err: io.ErrClosedPipe}

	err := app.Run(t.Context(), []string{appName, commandConfig, "nonsense"})

	check.ErrorIs(t, err, ErrUnknownCommand)
	check.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestCommandNotFoundState_ClearsHandledError(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	root := &cli.Command{
		Name:      appName,
		ErrWriter: &stderr,
		Commands: []*cli.Command{
			{Name: commandValidate},
		},
	}

	state := newCommandNotFoundState(testLocalizer())

	state.handle(t.Context(), root, "nonsense")
	firstErr := state.after(t.Context(), root)
	secondErr := state.after(t.Context(), root)

	check.ErrorIs(t, firstErr, ErrUnknownCommand)
	check.NoError(t, secondErr)
}

func TestCommandTypoDistance_HandlesAdjacentTransposition(t *testing.T) {
	t.Parallel()

	check.Equal(t, 1, commandTypoDistance("sycn", "sync")) // typos:disable-line
	check.True(t, commandTypoDistance("zxqwerty", "completion") > maximumCommandTypoDistance("completion"))
}
