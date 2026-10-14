// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package bindings

import . "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func shadowedBareMsg() {
	Msg := func(string) string { return "" }
	_ = Msg("Shadowed bare Msg")
}
