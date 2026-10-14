// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestMergeConfigs(t *testing.T) {
	t.Parallel()
	t.Run("empty configs returns default", func(t *testing.T) {
		t.Parallel()

		result, err := configuration.MergeAndValidateConfigs()
		check.NoError(t, err)
		check.Equal(t, configTypes.NewDefault(), result)
	})

	t.Run("single config", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.NewDefault()
		cfg.Output = output.FormatJSON
		cfg.Theme = testThemeDracula

		result, err := configuration.MergeAndValidateConfigs(cfg)
		check.NoError(t, err)
		check.Equal(t, output.FormatJSON, result.Output)
		check.Equal(t, testThemeDracula, result.Theme)
	})

	t.Run("multiple configs with precedence", func(t *testing.T) {
		t.Parallel()

		base := configTypes.NewDefault()
		base.Output = output.FormatText
		base.Theme = testThemeDefault
		base.Message.Subject.MaxLength = new(50)

		overlay := configTypes.Config{}
		overlay.Output = output.FormatJSON
		overlay.Theme = testThemeDracula
		overlay.Message.Subject.Case = new("lower")

		result, err := configuration.MergeAndValidateConfigs(base, overlay)
		check.NoError(t, err)
		check.Equal(t, output.FormatJSON, result.Output)                              // overlay wins
		check.Equal(t, testThemeDracula, result.Theme)                                // overlay wins
		check.Equal(t, 50, configTypes.DerefInt(result.Message.Subject.MaxLength, 0)) // base preserved
		check.Equal(t, "lower", configTypes.DerefString(result.Message.Subject.Case, ""))
	})
}

func TestMergeConfig(t *testing.T) {
	t.Parallel()
	t.Run("string field merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{Output: output.FormatText, Theme: testThemeDefault}
		overlay := configTypes.Config{Output: output.FormatJSON, Theme: testThemeDracula}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, output.FormatJSON, result.Output)
		check.Equal(t, testThemeDracula, result.Theme)
	})

	t.Run("empty overlay preserves base", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{Output: output.FormatText, Theme: testThemeDefault}
		overlay := configTypes.Config{}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, output.FormatText, result.Output)
		check.Equal(t, testThemeDefault, result.Theme)
	})

	t.Run("message config merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Message.Subject.MaxLength = new(50)
		base.Message.Subject.Case = new("lower")

		overlay := configTypes.Config{}
		overlay.Message.Subject.MaxLength = new(72)
		overlay.Message.Body.Required = new(true)

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, 72, configTypes.DerefInt(result.Message.Subject.MaxLength, 0)) // overlay wins
		check.Equal(t, "lower", configTypes.DerefString(result.Message.Subject.Case, ""))
		check.True(t, configTypes.DerefBool(result.Message.Body.Required, false)) // overlay wins
	})

	// Config validation accepts subject case case-insensitively, but SubjectRule
	// matches the canonical spelling exactly. Without normalization here, a
	// config saying "Upper" passed validation and then silently enforced the
	// context-aware default instead.
	t.Run("subject case is normalized", func(t *testing.T) {
		t.Parallel()

		overlay := configTypes.Config{}
		overlay.Message.Subject.Case = new(" Lower ")

		result := configuration.MergeConfig(configTypes.Config{}, overlay)
		check.Equal(t, configTypes.CaseLower, configTypes.DerefString(result.Message.Subject.Case, ""))

		// Non-conventional, so an unnormalized value would fall back to
		// "sentence" and invert both expectations below.
		rule := rules.NewSubjectRule(result, false)
		check.Empty(t, rule.Validate(domain.Commit{Subject: "add feature"}))
		check.NotEmpty(t, rule.Validate(domain.Commit{Subject: "Add feature"}))
	})

	t.Run("slice merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Rules.Enabled = []string{rulecatalog.Subject, rulecatalog.Conventional}

		overlay := configTypes.Config{}
		overlay.Rules.Disabled = []string{rulecatalog.Spell}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, []string{rulecatalog.Subject, rulecatalog.Conventional}, result.Rules.Enabled)
		check.Equal(t, []string{rulecatalog.Spell}, result.Rules.Disabled)
	})

	t.Run("slice overlay takes precedence", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Rules.Enabled = []string{rulecatalog.Subject}

		overlay := configTypes.Config{}
		overlay.Rules.Enabled = []string{rulecatalog.Conventional, rulecatalog.Spell}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, []string{rulecatalog.Conventional, rulecatalog.Spell}, result.Rules.Enabled)
	})

	t.Run("empty slice clears list", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Rules.Enabled = []string{rulecatalog.Subject}

		overlay := configTypes.Config{}
		overlay.Rules.Enabled = []string{}

		result := configuration.MergeConfig(base, overlay)
		check.Empty(t, result.Rules.Enabled)
	})

	t.Run("merge preserves explicit rule lists", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Rules.Enabled = []string{rulecatalog.Subject, rulecatalog.Spell}

		overlay := configTypes.Config{}
		overlay.Rules.Disabled = []string{rulecatalog.Spell}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, []string{rulecatalog.Subject, rulecatalog.Spell}, result.Rules.Enabled)
		check.Equal(t, []string{rulecatalog.Spell}, result.Rules.Disabled)
	})
}

func TestApplyRulePriority(t *testing.T) {
	t.Parallel()
	t.Run("no disabled rules", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.Config{}
		cfg.Rules.Enabled = []string{rulecatalog.Subject, rulecatalog.Conventional}

		result := configuration.ApplyRulePriority(cfg)
		check.Equal(t, []string{rulecatalog.Subject, rulecatalog.Conventional}, result.Rules.Enabled)
	})

	t.Run("disabled rules take precedence", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.Config{}
		cfg.Rules.Enabled = []string{rulecatalog.Subject, rulecatalog.Conventional, rulecatalog.Spell}
		cfg.Rules.Disabled = []string{rulecatalog.Spell, rulecatalog.JiraReference}

		cfg.Rules = configTypes.NormalizeRulesConfig(cfg.Rules)
		result := configuration.ApplyRulePriority(cfg)
		check.Equal(t, []string{rulecatalog.Subject, rulecatalog.Conventional}, result.Rules.Enabled)
		check.Equal(t, []string{rulecatalog.Spell, rulecatalog.JiraReference}, result.Rules.Disabled)
	})

	t.Run("all enabled rules disabled", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.Config{}
		cfg.Rules.Enabled = []string{rulecatalog.Spell}
		cfg.Rules.Disabled = []string{rulecatalog.Spell}

		cfg.Rules = configTypes.NormalizeRulesConfig(cfg.Rules)
		result := configuration.ApplyRulePriority(cfg)
		check.Empty(t, result.Rules.Enabled)
		check.Equal(t, []string{rulecatalog.Spell}, result.Rules.Disabled)
	})
}

func TestConfigMerge_MergesSectionsFieldByField(t *testing.T) {
	t.Parallel()
	t.Run("bool merging logic", func(t *testing.T) {
		t.Parallel()
		// Test through public API - conventional config
		base := configTypes.Config{}
		base.Conventional.RequireScope = new(false)

		overlay := configTypes.Config{}
		overlay.Conventional.RequireScope = new(true)

		result := configuration.MergeConfig(base, overlay)
		check.True(t, configTypes.DerefBool(result.Conventional.RequireScope, false))
	})

	t.Run("disallow_breaking_true_overrides_default", func(t *testing.T) {
		t.Parallel()

		base := configTypes.NewDefault()
		check.False(t, configTypes.DerefBool(base.Conventional.DisallowBreaking, false)) // Default is false (breaking allowed)

		overlay := configTypes.Config{}
		overlay.Conventional.DisallowBreaking = new(true)

		result := configuration.MergeConfig(base, overlay)
		check.True(t, configTypes.DerefBool(result.Conventional.DisallowBreaking, false)) // Overlay wins
	})

	t.Run("int merging with zero values", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Message.Subject.MaxLength = new(50)

		overlay := configTypes.Config{}
		// overlay has nil for MaxLength (not set)

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, 50, configTypes.DerefInt(result.Message.Subject.MaxLength, 0)) // base preserved
	})

	t.Run("complete config section merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Jira.RequireInSubject = new(true)
		base.Jira.ProjectPrefixes = []string{"PROJ"}

		overlay := configTypes.Config{}
		overlay.Jira.RequireInBody = new(true)
		overlay.Jira.IgnoreTicketPatterns = []string{"^IGNORE-"}

		result := configuration.MergeConfig(base, overlay)
		check.True(t, configTypes.DerefBool(result.Jira.RequireInSubject, false)) // base preserved
		check.True(t, configTypes.DerefBool(result.Jira.RequireInBody, false))    // overlay added
		check.Equal(t, []string{"PROJ"}, result.Jira.ProjectPrefixes)             // base preserved
		check.Equal(t, []string{"^IGNORE-"}, result.Jira.IgnoreTicketPatterns)    // overlay added
	})

	t.Run("cryptosignature config merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.CryptoSignature.MinRSABits = new(2048)
		base.CryptoSignature.MinECBits = new(256)
		base.CryptoSignature.KeyDirectory = "/base/keys"

		overlay := configTypes.Config{}
		overlay.CryptoSignature.MinRSABits = new(4096)
		overlay.CryptoSignature.RequireGPG = new(true)
		overlay.CryptoSignature.AllowedSigners = []string{"user@example.com"}

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, 4096, configTypes.DerefInt(result.CryptoSignature.MinRSABits, 0))    // overlay overrides
		check.Equal(t, 256, configTypes.DerefInt(result.CryptoSignature.MinECBits, 0))      // base preserved (overlay is nil)
		check.Equal(t, "/base/keys", result.CryptoSignature.KeyDirectory)                   // base preserved
		check.True(t, configTypes.DerefBool(result.CryptoSignature.RequireGPG, false))      // overlay added
		check.Equal(t, []string{"user@example.com"}, result.CryptoSignature.AllowedSigners) // overlay added
	})

	t.Run("body config merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Message.Body.Required = new(true)
		base.Message.Body.MinLength = new(10)
		base.Message.Body.MaxLineLength = new(80)

		overlay := configTypes.Config{}
		overlay.Message.Body.MaxLineLength = new(72)
		overlay.Message.Body.AllowSignoffOnly = new(true)

		result := configuration.MergeConfig(base, overlay)
		check.True(t, configTypes.DerefBool(result.Message.Body.Required, false))         // base preserved
		check.Equal(t, 10, configTypes.DerefInt(result.Message.Body.MinLength, 0))        // base preserved (overlay is nil)
		check.Equal(t, 72, configTypes.DerefInt(result.Message.Body.MaxLineLength, 0))    // overlay overrides
		check.True(t, configTypes.DerefBool(result.Message.Body.AllowSignoffOnly, false)) // overlay added
	})

	t.Run("limits config merging", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Limits.MaxCommitCount = new(2000)
		base.Limits.MaxKeyFiles = new(200)

		overlay := configTypes.Config{}
		overlay.Limits.MaxCommitCount = new(5000)

		result := configuration.MergeConfig(base, overlay)
		check.Equal(t, 5000, configTypes.DerefInt(result.Limits.MaxCommitCount, 0)) // overlay overrides
		check.Equal(t, 200, configTypes.DerefInt(result.Limits.MaxKeyFiles, 0))     // base preserved (overlay is nil)
	})
}

// TestConfigMerge_ExplicitFalseOverridesTrue tests the core Issue #3 fix:
// explicit false/0 values in overlay should override true/non-zero in base.
func TestConfigMerge_ExplicitFalseOverridesTrue(t *testing.T) {
	t.Parallel()

	t.Run("explicit false overrides true", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Message.Subject.RequireImperative = new(true)
		base.Message.Body.Required = new(true)
		base.Conventional.RequireScope = new(true)

		overlay := configTypes.Config{}
		overlay.Message.Subject.RequireImperative = new(false) // explicitly disable
		overlay.Message.Body.Required = new(false)             // explicitly disable
		overlay.Conventional.RequireScope = new(false)         // explicitly disable

		result := configuration.MergeConfig(base, overlay)

		// These should all be false - overlay's explicit false wins
		check.False(t, configTypes.DerefBool(result.Message.Subject.RequireImperative, true),
			"explicit false should override true for RequireImperative")
		check.False(t, configTypes.DerefBool(result.Message.Body.Required, true),
			"explicit false should override true for Body.Required")
		check.False(t, configTypes.DerefBool(result.Conventional.RequireScope, true),
			"explicit false should override true for RequireScope")
	})

	t.Run("explicit zero overrides non-zero", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Message.Subject.MaxLength = new(100)
		base.Message.Body.MinLength = new(50)
		base.Message.Body.MaxLineLength = new(72)

		overlay := configTypes.Config{}
		overlay.Message.Body.MinLength = new(0)     // explicitly set to 0 (disable check)
		overlay.Message.Body.MaxLineLength = new(0) // explicitly set to 0 (disable check)

		result := configuration.MergeConfig(base, overlay)

		// MaxLength should be preserved (overlay is nil)
		check.Equal(t, 100, configTypes.DerefInt(result.Message.Subject.MaxLength, -1),
			"nil overlay should preserve base MaxLength")

		// These should be 0 - overlay's explicit 0 wins
		check.Equal(t, 0, configTypes.DerefInt(result.Message.Body.MinLength, -1),
			"explicit 0 should override non-zero for MinLength")
		check.Equal(t, 0, configTypes.DerefInt(result.Message.Body.MaxLineLength, -1),
			"explicit 0 should override non-zero for MaxLineLength")
	})

	t.Run("nil preserves base while explicit values override", func(t *testing.T) {
		t.Parallel()

		base := configTypes.Config{}
		base.Conventional.RequireScope = new(true)
		base.Conventional.DisallowBreaking = new(true)
		base.Conventional.MaxDescriptionLength = new(72)

		overlay := configTypes.Config{}
		// RequireScope is nil - should preserve base's true
		overlay.Conventional.DisallowBreaking = new(false) // explicit false
		// MaxDescriptionLength is nil - should preserve base's 72

		result := configuration.MergeConfig(base, overlay)

		check.True(t, configTypes.DerefBool(result.Conventional.RequireScope, false),
			"nil overlay should preserve base RequireScope=true")
		check.False(t, configTypes.DerefBool(result.Conventional.DisallowBreaking, true),
			"explicit false should override DisallowBreaking=true")
		check.Equal(t, 72, configTypes.DerefInt(result.Conventional.MaxDescriptionLength, 0),
			"nil overlay should preserve base MaxDescriptionLength=72")
	})
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	t.Run("valid config passes validation", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.NewDefault()
		_, err := configuration.MergeAndValidateConfigs(cfg)
		check.NoError(t, err)
	})

	t.Run("invalid config fails validation", func(t *testing.T) {
		t.Parallel()

		cfg := configTypes.Config{}
		cfg.Message.Subject.MaxLength = new(-1) // Invalid value

		_, err := configuration.MergeAndValidateConfigs(cfg)
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrConfigValidationFailed)
	})

	for _, count := range []int{-1, 11} {
		t.Run(fmt.Sprintf("min signoff count %d fails validation", count), func(t *testing.T) {
			t.Parallel()

			cfg := configTypes.NewDefault()
			cfg.Message.Body.MinSignoffCount = new(count)

			_, err := configuration.MergeAndValidateConfigs(cfg)
			check.ErrorIs(t, err, configuration.ErrConfigValidationFailed)
			check.ErrorContains(t, err, "min_signoff_count")
		})
	}
}
