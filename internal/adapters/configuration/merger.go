// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"errors"
	"fmt"

	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Config validation errors.
var (
	ErrConfigValidationFailed = errors.New("configuration validation failed")
)

// MergeConfigs merges multiple configurations with later configs taking precedence.
// Uses functional composition without validation.
func MergeConfigs(configs ...configTypes.Config) configTypes.Config {
	if len(configs) == 0 {
		return configTypes.NewDefault()
	}

	// Start with first config and merge subsequent ones
	result := configs[0]
	for _, cfg := range configs[1:] {
		result = MergeConfig(result, cfg)
	}

	return result
}

// MergeAndValidateConfigs merges multiple configurations and validates the result.
func MergeAndValidateConfigs(configs ...configTypes.Config) (configTypes.Config, error) {
	merged := MergeConfigs(configs...)

	// Validate final configuration
	if validationErrors := configTypes.ValidateConfigErrors(merged); len(validationErrors) > 0 {
		messages := configTypes.ConfigErrorMessages(validationErrors)

		return configTypes.Config{}, fmt.Errorf("%w: %v", ErrConfigValidationFailed, messages)
	}

	return merged, nil
}

// MergeConfig merges two configurations with the overlay taking precedence.
// Uses explicit field merging.
func MergeConfig(base, overlay configTypes.Config) configTypes.Config {
	result := base

	result.Output = overlayString(result.Output, overlay.Output)
	result.Theme = overlayString(result.Theme, overlay.Theme)
	result.AllowFixupCommits = overlayPtr(result.AllowFixupCommits, overlay.AllowFixupCommits)
	result.IgnoreCommits = overlaySlice(result.IgnoreCommits, overlay.IgnoreCommits)
	result.Extends = overlaySlice(result.Extends, overlay.Extends)
	result.ManagedSources = overlaySlice(result.ManagedSources, overlay.ManagedSources)
	result.Message = mergeMessageConfig(result.Message, overlay.Message)
	result.Conventional = mergeConventionalConfig(result.Conventional, overlay.Conventional)
	result.Repo = mergeRepoConfig(result.Repo, overlay.Repo)
	result.Rules = mergeRulesConfig(result.Rules, overlay.Rules)
	result.Checks = mergeChecksConfig(result.Checks, overlay.Checks)
	result.Jira = mergeJiraConfig(result.Jira, overlay.Jira)
	result.Spell = mergeSpellConfig(result.Spell, overlay.Spell)
	result.CryptoSignature = mergeCryptoSignatureConfig(result.CryptoSignature, overlay.CryptoSignature)
	result.Identity = mergeIdentityConfig(result.Identity, overlay.Identity)
	result.Limits = mergeLimitsConfig(result.Limits, overlay.Limits)

	result.Rules = configTypes.NormalizeRulesConfig(result.Rules)
	result.Checks = configTypes.NormalizeChecksConfig(result.Checks)
	result.Message = configTypes.NormalizeMessageConfig(result.Message)

	return result
}

func overlayString(base, overlay string) string {
	if overlay != "" {
		return overlay
	}

	return base
}

func overlayPtr[T any](base, overlay *T) *T {
	if overlay != nil {
		return overlay
	}

	return base
}

func overlaySlice[T any](base, overlay []T) []T {
	if overlay != nil {
		return overlay
	}

	return base
}

// ApplyRulePriority resolves explicit enabled rules with disabled rules taking precedence.
func ApplyRulePriority(cfg configTypes.Config) configTypes.Config {
	result := cfg
	normalized := configTypes.NormalizeRulesConfig(cfg.Rules)
	result.Rules.Enabled = configTypes.ResolveEnabledRules(nil, normalized)
	result.Rules.Disabled = normalized.Disabled

	return result
}

// mergeMessageConfig merges message configuration sections.
func mergeMessageConfig(base, overlay configTypes.MessageConfig) configTypes.MessageConfig {
	result := base

	result.Subject.MaxLength = overlayPtr(result.Subject.MaxLength, overlay.Subject.MaxLength)
	result.Subject.Case = overlayPtr(result.Subject.Case, overlay.Subject.Case)
	result.Subject.RequireImperative = overlayPtr(result.Subject.RequireImperative, overlay.Subject.RequireImperative)
	result.Subject.ForbidEndings = overlaySlice(result.Subject.ForbidEndings, overlay.Subject.ForbidEndings)
	result.Body.Required = overlayPtr(result.Body.Required, overlay.Body.Required)
	result.Body.MinLength = overlayPtr(result.Body.MinLength, overlay.Body.MinLength)
	result.Body.MaxLineLength = overlayPtr(result.Body.MaxLineLength, overlay.Body.MaxLineLength)
	result.Body.AllowSignoffOnly = overlayPtr(result.Body.AllowSignoffOnly, overlay.Body.AllowSignoffOnly)
	result.Body.MinSignoffCount = overlayPtr(result.Body.MinSignoffCount, overlay.Body.MinSignoffCount)

	return result
}

// mergeConventionalConfig merges conventional commit configuration.
func mergeConventionalConfig(base, overlay configTypes.ConventionalConfig) configTypes.ConventionalConfig {
	result := base

	result.RequireScope = overlayPtr(result.RequireScope, overlay.RequireScope)
	result.MaxDescriptionLength = overlayPtr(result.MaxDescriptionLength, overlay.MaxDescriptionLength)
	result.DisallowBreaking = overlayPtr(result.DisallowBreaking, overlay.DisallowBreaking)
	result.AllowMultiScope = overlayPtr(result.AllowMultiScope, overlay.AllowMultiScope)
	result.Types = overlaySlice(result.Types, overlay.Types)
	result.Scopes = overlaySlice(result.Scopes, overlay.Scopes)

	return result
}

// mergeRepoConfig merges repository configuration.
func mergeRepoConfig(base, overlay configTypes.RepoConfig) configTypes.RepoConfig {
	result := base
	result.MaxCommitsAhead = overlayPtr(result.MaxCommitsAhead, overlay.MaxCommitsAhead)

	return result
}

// mergeRulesConfig merges rules configuration.
func mergeRulesConfig(base, overlay configTypes.RulesConfig) configTypes.RulesConfig {
	result := base
	result.Enabled = overlaySlice(result.Enabled, overlay.Enabled)
	result.Disabled = overlaySlice(result.Disabled, overlay.Disabled)

	return result
}

// mergeChecksConfig merges public check activation configuration.
func mergeChecksConfig(base, overlay configTypes.ChecksConfig) configTypes.ChecksConfig {
	result := base
	result.Enabled = overlaySlice(result.Enabled, overlay.Enabled)
	result.Disabled = overlaySlice(result.Disabled, overlay.Disabled)

	return result
}

// mergeJiraConfig merges JIRA configuration.
func mergeJiraConfig(base, overlay configTypes.JiraConfig) configTypes.JiraConfig {
	result := base
	result.RequireInSubject = overlayPtr(result.RequireInSubject, overlay.RequireInSubject)
	result.RequireInBody = overlayPtr(result.RequireInBody, overlay.RequireInBody)
	result.ProjectPrefixes = overlaySlice(result.ProjectPrefixes, overlay.ProjectPrefixes)
	result.IgnoreTicketPatterns = overlaySlice(result.IgnoreTicketPatterns, overlay.IgnoreTicketPatterns)

	return result
}

// mergeSpellConfig merges spell checking configuration.
func mergeSpellConfig(base, overlay configTypes.SpellConfig) configTypes.SpellConfig {
	result := base
	result.Locale = overlayPtr(result.Locale, overlay.Locale)
	result.IgnoreWords = overlaySlice(result.IgnoreWords, overlay.IgnoreWords)

	return result
}

// mergeCryptoSignatureConfig merges cryptographic signature configuration.
func mergeCryptoSignatureConfig(base, overlay configTypes.CryptoSignatureConfig) configTypes.CryptoSignatureConfig {
	result := base

	result.AllowedSigners = overlaySlice(result.AllowedSigners, overlay.AllowedSigners)
	result.KeyDirectory = overlayString(result.KeyDirectory, overlay.KeyDirectory)
	result.MinECBits = overlayPtr(result.MinECBits, overlay.MinECBits)
	result.MinRSABits = overlayPtr(result.MinRSABits, overlay.MinRSABits)
	result.Required = overlayPtr(result.Required, overlay.Required)
	result.RequireGPG = overlayPtr(result.RequireGPG, overlay.RequireGPG)
	result.RequireSSH = overlayPtr(result.RequireSSH, overlay.RequireSSH)
	result.VerifyFormat = overlayPtr(result.VerifyFormat, overlay.VerifyFormat)

	return result
}

// mergeIdentityConfig merges identity configuration.
func mergeIdentityConfig(base, overlay configTypes.IdentityConfig) configTypes.IdentityConfig {
	result := base
	result.AllowedAuthors = overlaySlice(result.AllowedAuthors, overlay.AllowedAuthors)

	return result
}

// mergeLimitsConfig merges global guardrail configuration.
func mergeLimitsConfig(base, overlay configTypes.LimitsConfig) configTypes.LimitsConfig {
	result := base
	result.MaxCommitCount = overlayPtr(result.MaxCommitCount, overlay.MaxCommitCount)
	result.MaxKeyFiles = overlayPtr(result.MaxKeyFiles, overlay.MaxKeyFiles)

	return result
}
