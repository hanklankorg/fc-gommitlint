//
// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestShowConfigWarnings_PrintsUnlessQuiet(t *testing.T) {
	t.Parallel()

	warnings := []domain.Warning{
		{Code: domain.WarnConfigParse, Message: "global config skipped"},
		{Code: domain.WarnConfigParse, Message: "project config skipped"},
	}

	t.Run("prints warnings when not quiet", func(t *testing.T) {
		t.Parallel()

		var buffer bytes.Buffer

		err := showConfigWarnings(&buffer, false, warnings, nil)
		check.NoError(t, err)

		output := buffer.String()
		check.Contains(t, output, "Warning: global config skipped")
		check.Contains(t, output, "Warning: project config skipped")
	})

	t.Run("skips warnings when quiet", func(t *testing.T) {
		t.Parallel()

		var buffer bytes.Buffer

		err := showConfigWarnings(&buffer, true, warnings, nil)
		check.NoError(t, err)
		check.Empty(t, buffer.String())
	})
}

func TestShowConfigWarnings_PropagatesWriteFailure(t *testing.T) {
	t.Parallel()

	err := showConfigWarnings(
		failingWriter{err: errMockWrite},
		false,
		[]domain.Warning{{Code: domain.WarnConfigParse, Message: "config skipped"}},
		nil,
	)

	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)
}

func TestShowConfigWarnings_SanitizesTerminalText(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer

	err := showConfigWarnings(
		&buffer,
		false,
		[]domain.Warning{{Code: domain.WarnConfigParse, Message: "config\x1b[31m\nforged"}},
		nil,
	)

	check.NoError(t, err)
	check.Equal(t, "Warning: config\\x1b[31m forged\n", buffer.String())
}
