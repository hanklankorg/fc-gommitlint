// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestConfig_DefaultAppliesProductDefaults(t *testing.T) {
	t.Parallel()

	cfg := settings.NewDefault()

	check.True(t, settings.DerefBool(cfg.Message.Subject.RequireImperative, false))
	check.Empty(t, cfg.Rules.Enabled)
	check.Empty(t, cfg.Rules.Disabled)
}
