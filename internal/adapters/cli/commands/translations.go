// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

// NewTranslationsCommand creates the developer-facing translations
// management command. Hidden from end-user help because it's a
// build-time maintenance tool, not a feature for commit-message
// validation users.
//
// Catalog drift is checked by scripts/lint/translation-drift.sh.
func NewTranslationsCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:   "translations",
		Usage:  localizer.Get("manage translation catalogs (developer tool)"),
		Hidden: true,
		Description: localizer.Get(`Developer-facing translation catalog management.

Commands:
  update   Refresh locale/<lang>/gommitlint.po from source code
           (the built-in extractor finds msgids; msgmerge folds them into existing
           catalogs while preserving translator-edited msgstr entries).

  compile  Compile every locale/<lang>/gommitlint.po to a binary
           gommitlint.mo catalog under generated/locale/<lang>/LC_MESSAGES/.

The scripts/lint/translation-drift.sh gate re-runs extraction and reports
catalogs that were not refreshed after source strings changed.`),

		Commands: []*cli.Command{
			NewTranslationsUpdateCommand(localizer),
			NewTranslationsCompileCommand(localizer),
		},
	}
}
