// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/schemas"
)

// ExecuteConfigSchema prints the embedded JSON Schema for the gommitlint
// configuration file. The schema is generated from the Go types in
// internal/domain/settings by internal/tools/schemagen and embedded at build
// time, so its content is byte-identical to the file SchemaStore consumes.
func ExecuteConfigSchema(_ context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()

	if err := validateConfigSubcommandArgs(cmd, localizer, commandNameConfigSchema); err != nil {
		return err
	}

	if cmd.Bool(flagNameCompact) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, schemas.Config); err != nil {
			return fmt.Errorf("compact schema: %w", err)
		}

		if _, err := fmt.Fprintln(cmd.Writer, buf.String()); err != nil {
			return configOutputError(err, output.Msg("failed to write config schema"))
		}

		return nil
	}

	if _, err := cmd.Writer.Write(schemas.Config); err != nil {
		return configOutputError(err, output.Msg("failed to write config schema"))
	}

	return nil
}
