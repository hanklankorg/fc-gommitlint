// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

// WarningCode identifies the warning category.
type WarningCode string

// WarningContextKey identifies structured warning context fields.
type WarningContextKey = string

// Warning code constants for non-fatal issues.
const (
	WarnInvalidIgnoreCommit WarningCode = "invalid_ignore_commit"
	WarnConfigSkipped       WarningCode = "config_skipped"
	WarnConfigParse         WarningCode = "config_parse"
	WarnConfigInsecurePerms WarningCode = "config_insecure_perms"
	// WarnConfigUnknownKey fires when a config file contains a key that
	// doesn't match any field in the typed Config struct. Common cause is
	// a typo ("subjct:" for "subject:"). Forward-compat dictates a warning
	// rather than an error — newer versions adding fields shouldn't break
	// older configs.
	WarnConfigUnknownKey WarningCode = "config_unknown_key"
	// WarnConfigBadShape fires when a config file's structure doesn't match
	// the expected schema at the top level — either the 'gommitlint:' root
	// key is missing entirely, or its value isn't a mapping. Without this
	// warning, malformed configs silently load as empty.
	WarnConfigBadShape WarningCode = "config_bad_shape"
	// WarnConfigRulesOverlap fires when a rule name appears in both
	// rules.enabled and rules.disabled. The resolver chooses disabled-wins
	// silently; surfacing the contradiction lets users fix the config.
	WarnConfigRulesOverlap WarningCode = "config_rules_overlap"
	WarnBaseBranchFallback WarningCode = "base_branch_fallback"
	WarnBaseBranchNotFound WarningCode = "base_branch_not_found"
)

// Common warning context keys.
const (
	WarningContextSHA        WarningContextKey = "sha"
	WarningContextPath       WarningContextKey = "path"
	WarningContextSource     WarningContextKey = "source"
	WarningContextCause      WarningContextKey = "cause"
	WarningContextConfigKey  WarningContextKey = "key"
	WarningContextRules      WarningContextKey = "rules"
	WarningContextIssue      WarningContextKey = "issue"
	WarningContextResolved   WarningContextKey = "resolved"
	WarningContextTried      WarningContextKey = "tried"
	WarningContextCandidates WarningContextKey = "candidates"
)

// Warning represents a non-fatal validation warning.
type Warning struct {
	Code    WarningCode
	Message string
	Context map[string]string
}
