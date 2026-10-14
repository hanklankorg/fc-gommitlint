// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Presentation defaults sourced from the output registry, which owns them.
// Referencing the exported constants (rather than re-typing "text"/"default")
// keeps this adapter's defaults from drifting away from the formatter's.
const (
	defaultFormat = output.DefaultFormat
	defaultTheme  = output.DefaultTheme
)

// NewConfigWithDefaults creates configuration with application-specific defaults.
// Presentation defaults (output format, theme) are applied here in the adapter layer.
func NewConfigWithDefaults() settings.Config {
	cfg := settings.NewDefault()
	cfg.Output = defaultFormat
	cfg.Theme = defaultTheme

	return cfg
}
