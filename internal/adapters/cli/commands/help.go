// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func helpSuggestion(localizer output.Localizer, command string) string {
	localizer = output.EnsureLocalizer(localizer)

	return localizer.Get("Run '%s --help' for more information", command)
}
