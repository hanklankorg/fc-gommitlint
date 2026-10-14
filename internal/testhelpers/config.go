// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers

// MinimalConfigYAML returns a minimal YAML config with an output format.
func MinimalConfigYAML(output string) string {
	return "gommitlint:\n  output: " + output
}
