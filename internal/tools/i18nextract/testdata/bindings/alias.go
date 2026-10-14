// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package bindings

import translated "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func aliasedOutput() {
	_ = translated.Msg("Aliased output message")
}
