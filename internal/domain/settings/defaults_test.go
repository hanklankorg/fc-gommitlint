// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestConfig_MessageDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Test subject defaults - 100 allows room for type(scope): + 72 char description
	check.Equal(t, 100, settings.DerefInt(cfg.Message.Subject.MaxLength, 0))
	// Case is "auto" to signal context-aware defaults (lower for conventional, sentence for non-conventional)
	check.Equal(t, "auto", settings.DerefString(cfg.Message.Subject.Case, ""))
	check.False(t, settings.DerefBool(cfg.Message.Subject.RequireImperative, true))
	check.Equal(t, []string{".", ",", ";", ":", "!", "?"}, cfg.Message.Subject.ForbidEndings)

	// Test body defaults
	check.False(t, settings.DerefBool(cfg.Message.Body.Required, true))
	check.Equal(t, 0, settings.DerefInt(cfg.Message.Body.MinLength, -1))
	check.False(t, settings.DerefBool(cfg.Message.Body.AllowSignoffOnly, true))
	// The schema documents "Default: 1"; stated literally so a changed constant is noticed here.
	check.Equal(t, 1, settings.DerefInt(cfg.Message.Body.MinSignoffCount, -1))
}

func TestConfig_ConventionalDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Test conventional commit defaults
	check.False(t, settings.DerefBool(cfg.Conventional.RequireScope, true))
	check.False(t, settings.DerefBool(cfg.Conventional.DisallowBreaking, true))
	check.Equal(t, 72, settings.DerefInt(cfg.Conventional.MaxDescriptionLength, 0))

	// Test default types include standard conventional commit types (alphabetically sorted)
	expectedTypes := []string{"build", "chore", "ci", settingsTestTypeDocs, settingsTestTypeFeat, settingsTestTypeFix, "perf", "refactor", "revert", "style", "test"}
	check.Equal(t, expectedTypes, cfg.Conventional.Types)

	// Test scopes are empty by default (project-specific)
	check.Empty(t, cfg.Conventional.Scopes)
}

func TestConfig_SecurityDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Test crypto signature defaults (disabled by default for ease of use)
	check.False(t, settings.DerefBool(cfg.CryptoSignature.Required, true))
	check.False(t, settings.DerefBool(cfg.CryptoSignature.VerifyFormat, true))
	check.Empty(t, cfg.CryptoSignature.KeyDirectory)
	check.Empty(t, cfg.CryptoSignature.AllowedSigners)
	check.Equal(t, 2048, settings.DerefInt(cfg.CryptoSignature.MinRSABits, 0))
	check.Equal(t, 256, settings.DerefInt(cfg.CryptoSignature.MinECBits, 0))
	check.False(t, settings.DerefBool(cfg.CryptoSignature.RequireGPG, true))
	check.False(t, settings.DerefBool(cfg.CryptoSignature.RequireSSH, true))

	// Test identity defaults (permissive by default)
	check.Empty(t, cfg.Identity.AllowedAuthors)
}

func TestConfig_RepoDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// rules.adoc and the schema document "default: 10"; stated literally so a changed constant is noticed here.
	check.Equal(t, 10, settings.DerefInt(cfg.Repo.MaxCommitsAhead, 0))
}

func TestConfig_JiraDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Test JIRA defaults (disabled by default)
	check.Empty(t, cfg.Jira.ProjectPrefixes)
	check.False(t, settings.DerefBool(cfg.Jira.RequireInBody, true))
	check.False(t, settings.DerefBool(cfg.Jira.RequireInSubject, true))
	check.Empty(t, cfg.Jira.IgnoreTicketPatterns)
}

func TestConfig_SpellDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Default is dialect-neutral ("both"): accept US and UK spellings without
	// flagging cross-variant words. Avoids forcing global teams to pick a side.
	check.Equal(t, "both", settings.DerefString(cfg.Spell.Locale, ""))
	check.Empty(t, cfg.Spell.IgnoreWords)
}

func TestConfig_RulesDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Test rules defaults (empty to use factory defaults)
	check.Empty(t, cfg.Rules.Enabled)
	check.Empty(t, cfg.Rules.Disabled)
}

func TestConfig_OutputDefaultsSetCorrectly(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()

	// Output format and theme are presentation concerns set by the adapter layer, not the domain.
	check.Empty(t, cfg.Output)
	check.Empty(t, cfg.Theme)
}

func TestConfig_DefaultCreatesIndependentInstances(t *testing.T) {
	t.Parallel()

	// Test that multiple calls return independent instances
	cfg1 := newBaseConfig()
	cfg2 := newBaseConfig()

	// Modify cfg1
	cfg1.Message.Subject.MaxLength = new(50)
	cfg1.Conventional.Types = append(cfg1.Conventional.Types, "custom")
	cfg1.Output = settingsTestOutputJSON
	cfg1.Theme = settingsTestThemeDracula

	// cfg2 should be unaffected
	check.Equal(t, 100, settings.DerefInt(cfg2.Message.Subject.MaxLength, 0))
	check.NotContains(t, cfg2.Conventional.Types, "custom")
	check.Empty(t, cfg2.Output)
	check.Empty(t, cfg2.Theme)
}

func TestConfig_DefaultCreatesConsistentValues(t *testing.T) {
	t.Parallel()

	// Test that default values are consistent with validation rules
	cfg := newBaseConfig()

	// Should pass its own validation
	errors := settings.ValidateConfig(cfg)
	check.Empty(t, errors, "Default config should pass validation: %v", errors)

	// Should have sensible, non-zero defaults where appropriate
	check.Positive(t, settings.DerefInt(cfg.Message.Subject.MaxLength, 0))
	check.NotEmpty(t, cfg.Conventional.Types)
	// Output format and theme are set by the adapter layer, not the domain
	check.Empty(t, cfg.Output)
	check.Empty(t, cfg.Theme)
	check.NotEmpty(t, settings.DerefString(cfg.Spell.Locale, ""))
}
