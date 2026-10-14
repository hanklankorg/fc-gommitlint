// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestWriteIndentedJSON_UsesTwoSpaceIndentationAndTrailingNewline(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	value := struct {
		Name string `json:"name"`
	}{Name: "gommitlint"}

	err := writeIndentedJSON(&buf, value, output.Msg("failed to encode test JSON"))
	check.NoError(t, err)
	check.Equal(t, "{\n  \"name\": \"gommitlint\"\n}\n", buf.String())
}

func TestWriteIndentedJSON_LocalizesEncodingFailure(t *testing.T) {
	t.Parallel()

	err := writeIndentedJSON(failingWriter{err: errMockWrite}, struct{}{}, output.Msg("failed to encode config as JSON"))

	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.Contains(t, recoveryErr.Format(testLocalizer(t, "sv")), "det gick inte att koda konfigurationen som JSON")
}
