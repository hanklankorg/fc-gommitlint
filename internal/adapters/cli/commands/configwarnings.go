//
// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"io"
	"os"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func showConfigWarnings(writer io.Writer, quiet bool, warnings []domain.Warning, localizer output.Localizer) error {
	if quiet || len(warnings) == 0 {
		return nil
	}

	// Fall back to stderr when the caller didn't wire an explicit writer (mostly
	// test scaffolding). Better than panicking on nil — and stderr is the right
	// destination for diagnostic warnings anyway.
	if writer == nil {
		writer = os.Stderr
	}

	localizer = output.EnsureLocalizer(localizer)

	for _, msg := range output.FormatWarnings(warnings, localizer) {
		if err := writeCommandOutput(writer, localizer.Get("Warning: %s", output.CleanForTerminal(msg))+"\n", localizer); err != nil {
			return err
		}
	}

	return nil
}
