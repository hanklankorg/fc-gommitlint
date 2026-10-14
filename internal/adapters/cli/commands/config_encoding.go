// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"encoding/json"
	"io"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"gopkg.in/yaml.v3"
)

func newIndentedJSONEncoder(writer io.Writer) *json.Encoder {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	return encoder
}

func writeIndentedJSON(writer io.Writer, value any, message output.LocaleMsg) error {
	if err := newIndentedJSONEncoder(writer).Encode(value); err != nil {
		return configOutputError(err, message)
	}

	return nil
}

func newIndentedYAMLEncoder(writer io.Writer) *yaml.Encoder {
	encoder := yaml.NewEncoder(writer)
	encoder.SetIndent(2)

	return encoder
}
