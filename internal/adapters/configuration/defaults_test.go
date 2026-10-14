// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestNewConfigWithDefaults_PopulatesTheDocumentedDefaults pins the values a
// user gets when no config file is present.
func TestNewConfigWithDefaults_PopulatesTheDocumentedDefaults(t *testing.T) {
	t.Parallel()

	cfg := configuration.NewConfigWithDefaults()

	// Case is "auto" to signal context-aware defaults (lower for conventional, sentence for non-conventional)
	check.Equal(t, "auto", configTypes.DerefString(cfg.Message.Subject.Case, ""))
	check.Equal(t, 100, configTypes.DerefInt(cfg.Message.Subject.MaxLength, 0))

	check.Empty(t, cfg.Rules.Enabled)
	check.Empty(t, cfg.Rules.Disabled)

	check.True(t, configTypes.DerefBool(cfg.Message.Subject.RequireImperative, false))
}

func TestLoadConfigHierarchy_NoFilesYieldsDefaults(t *testing.T) {
	t.Parallel()

	// An empty environment keeps the caller's XDG_CONFIG_HOME/HOME config out.
	result, err := configuration.LoadConfigHierarchyWithEnvAndFS("", "", configuration.NewMapEnvironment(map[string]string{}), nil)
	check.NoError(t, err)

	cfg := result.Config

	// Verify default values are loaded
	// Case is "auto" to signal context-aware defaults (lower for conventional, sentence for non-conventional)
	check.Equal(t, "auto", configTypes.DerefString(cfg.Message.Subject.Case, ""))
	check.Equal(t, 100, configTypes.DerefInt(cfg.Message.Subject.MaxLength, 0))
	check.Equal(t, output.FormatText, cfg.Output)
	check.Equal(t, output.DefaultTheme, cfg.Theme)
}

func TestMergeAndValidateConfigs_OverlayWinsAndBaseStaysImmutable(t *testing.T) {
	t.Parallel()

	// Test configuration merging
	base := configuration.NewConfigWithDefaults()

	// Create overlay with different values
	overlay := configTypes.Config{
		Message: configTypes.MessageConfig{
			Subject: configTypes.SubjectConfig{
				MaxLength: new(50), // Override default of 100
			},
		},
		Output: output.FormatJSON,
	}

	// Merge configurations
	result, err := configuration.MergeAndValidateConfigs(base, overlay)
	check.NoError(t, err)

	// Verify overlay values took precedence
	check.Equal(t, 50, configTypes.DerefInt(result.Message.Subject.MaxLength, 0))
	check.Equal(t, output.FormatJSON, result.Output)

	// Verify base values remain for unspecified fields
	// Case is "auto" to signal context-aware defaults
	check.Equal(t, "auto", configTypes.DerefString(result.Message.Subject.Case, ""))

	// Verify immutability - base config unchanged (still has default of 100)
	check.Equal(t, 100, configTypes.DerefInt(base.Message.Subject.MaxLength, 0))
}
