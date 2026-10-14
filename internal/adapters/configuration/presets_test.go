// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

func TestResolvePreset_KnownAndUnknownNames(t *testing.T) {
	t.Parallel()

	t.Run("conventional uses product defaults", func(t *testing.T) {
		t.Parallel()

		cfg, err := configuration.ResolvePreset("conventional")
		check.NoError(t, err)
		check.Equal(t, new(true), cfg.Message.Subject.RequireImperative)
		check.Empty(t, cfg.Rules.Enabled)
		check.Empty(t, cfg.Rules.Disabled)
	})

	t.Run("unknown preset returns error", func(t *testing.T) {
		t.Parallel()

		_, err := configuration.ResolvePreset("unknown")
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrUnknownPreset)
	})
}

func TestPresetContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		preset   string
		enabled  []string
		disabled []string
	}{
		{
			name:   "minimal",
			preset: "minimal",
			enabled: []string{
				rulecatalog.Conventional,
				rulecatalog.Subject,
			},
			disabled: []string{
				rulecatalog.BranchAhead,
				rulecatalog.CommitBody,
				rulecatalog.CryptoSignature,
				rulecatalog.Identity,
				rulecatalog.JiraReference,
				rulecatalog.LinearHistory,
				rulecatalog.SignOff,
				rulecatalog.Spell,
			},
		},
		{
			name:   "ambitious",
			preset: "ambitious",
			enabled: []string{
				rulecatalog.BranchAhead,
				rulecatalog.CommitBody,
				rulecatalog.Conventional,
				rulecatalog.SignOff,
				rulecatalog.Spell,
				rulecatalog.Subject,
			},
			disabled: []string{
				rulecatalog.CryptoSignature,
				rulecatalog.Identity,
				rulecatalog.JiraReference,
				rulecatalog.LinearHistory,
			},
		},
		{
			name:   "secure",
			preset: "secure",
			enabled: []string{
				rulecatalog.Conventional,
				rulecatalog.CryptoSignature,
				rulecatalog.Identity,
				rulecatalog.SignOff,
				rulecatalog.Subject,
			},
			disabled: []string{
				rulecatalog.BranchAhead,
				rulecatalog.CommitBody,
				rulecatalog.JiraReference,
				rulecatalog.LinearHistory,
				rulecatalog.Spell,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := configuration.ResolvePreset(testCase.preset)
			check.NoError(t, err)
			check.ElementsMatch(t, testCase.enabled, cfg.Rules.Enabled)
			check.ElementsMatch(t, testCase.disabled, cfg.Rules.Disabled)
		})
	}
}

func TestParsePresetReference_AcceptsOnlyPresetScheme(t *testing.T) {
	t.Parallel()

	name, ok := configuration.ParsePresetReference("preset:secure")
	check.True(t, ok)
	check.Equal(t, "secure", name)

	_, ok = configuration.ParsePresetReference("config/base.yaml")
	check.False(t, ok)
}
