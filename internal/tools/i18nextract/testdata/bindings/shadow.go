// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package bindings

import "codefloe.com/itiquette/gommitlint/internal/adapters/output"

type localOutput struct{}

func (localOutput) Msg(string) string {
	return ""
}

func defaultOutput() {
	_ = output.Msg("Default output message")
}

func shadowedOutput(localizer interface{ Get(string, ...any) string }) {
	output := localOutput{}
	_ = output.Msg("Shadowed output.Msg")
	_ = localizer.Get("Preserved localizer message")
}
