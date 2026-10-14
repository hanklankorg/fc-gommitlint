// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"codefloe.com/itiquette/gommitlint/schemas"
)

func TestExecuteConfigSchema_Pretty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	cmd := &cli.Command{
		Name:   commandNameConfigSchema,
		Writer: &buf,
		Flags:  []cli.Flag{&cli.BoolFlag{Name: flagNameCompact}},
	}

	err := ExecuteConfigSchema(t.Context(), cmd)
	check.NoError(t, err)

	out := buf.Bytes()
	check.Positive(t, len(out), "should write schema bytes")
	check.Equal(t, schemas.Config, out)

	var parsed map[string]any
	check.NoError(t, json.Unmarshal(out, &parsed), "output should be valid JSON")

	check.Equal(t, "https://json-schema.org/draft/2020-12/schema", parsed["$schema"])
	check.True(t, strings.Contains(string(out), "\n  "), "indented JSON should contain two-space indents")
}

func TestExecuteConfigSchema_Compact(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	cmd := &cli.Command{
		Name:   commandNameConfigSchema,
		Writer: &buf,
		Flags:  []cli.Flag{&cli.BoolFlag{Name: flagNameCompact, Value: true}},
	}

	check.NoError(t, cmd.Set(flagNameCompact, "true"))

	err := ExecuteConfigSchema(t.Context(), cmd)
	check.NoError(t, err)

	out := strings.TrimRight(buf.String(), "\n")
	check.False(t, strings.Contains(out, "\n"), "compact output should be single-line")

	var parsed map[string]any
	check.NoError(t, json.Unmarshal([]byte(out), &parsed), "compact output should be valid JSON")
}

func TestExecuteConfigSchema_WriteFailureUsesLocalizedRecovery(t *testing.T) {
	t.Parallel()

	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact_%t", compact), func(t *testing.T) {
			t.Parallel()

			cmd := &cli.Command{
				Name:   commandNameConfigSchema,
				Writer: failingWriter{err: errMockWrite},
				Flags:  []cli.Flag{&cli.BoolFlag{Name: flagNameCompact}},
			}
			check.NoError(t, cmd.Set(flagNameCompact, strconv.FormatBool(compact)))

			err := ExecuteConfigSchema(t.Context(), cmd)
			check.ErrorIs(t, err, ErrOutputWrite)
			check.ErrorIs(t, err, errMockWrite)

			var recoveryErr RecoveryError
			check.ErrorAs(t, err, &recoveryErr)
			check.Contains(t, recoveryErr.Format(testLocalizer(t, "sv")), "det gick inte att skriva konfigurationsschemat")
		})
	}
}
