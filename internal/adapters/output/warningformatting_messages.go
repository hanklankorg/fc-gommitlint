// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import "codefloe.com/itiquette/gommitlint/internal/domain"

// warningFormatter converts a structured warning to a localized string.
type warningFormatter func(domain.Warning, Localizer) string

// warningFormatterRegistry maps warning codes to localized formatters.
// Each formatter is a small top-level function (not an inline closure) so
// the registry itself stays trivial and each formatter is testable in
// isolation.
func warningFormatterRegistry() map[domain.WarningCode]warningFormatter {
	return map[domain.WarningCode]warningFormatter{
		domain.WarnInvalidIgnoreCommit: formatWarnInvalidIgnoreCommit,
		domain.WarnConfigParse:         formatWarnConfigParse,
		domain.WarnConfigSkipped:       formatWarnConfigSkipped,
		domain.WarnConfigUnknownKey:    formatWarnConfigUnknownKey,
		domain.WarnConfigRulesOverlap:  formatWarnConfigRulesOverlap,
		domain.WarnConfigBadShape:      formatWarnConfigBadShape,
		domain.WarnConfigInsecurePerms: formatWarnConfigInsecurePerms,
		domain.WarnBaseBranchFallback:  formatWarnBaseBranchFallback,
		domain.WarnBaseBranchNotFound:  formatWarnBaseBranchNotFound,
	}
}

func formatWarnInvalidIgnoreCommit(warning domain.Warning, localizer Localizer) string {
	if sha := warning.Context[domain.WarningContextSHA]; sha != "" {
		return localizer.Get("'%s' is not a valid commit SHA in ignore_commits, skipping it", sha)
	}

	return localizer.Get("Found an invalid commit SHA in ignore_commits")
}

func formatWarnConfigParse(warning domain.Warning, localizer Localizer) string {
	path := warning.Context[domain.WarningContextPath]
	source := warning.Context[domain.WarningContextSource]
	cause := warning.Context[domain.WarningContextCause]

	// When the loader captured the underlying parser error in WarningContextCause,
	// render it inline so the user sees *why* (e.g. "yaml: line 4: cannot
	// unmarshal !!str `foo` into int") instead of a generic "could not parse".
	if path != "" && source != "" {
		if cause != "" {
			return localizer.Get("Could not parse %s config file %s (%s), using defaults instead", source, path, cause)
		}

		return localizer.Get("Could not parse %s config file %s, using defaults instead", source, path)
	}

	// Fallback for creation sites without structured context.
	if warning.Message != "" {
		return warning.Message
	}

	return localizer.Get("Could not parse configuration file, using defaults instead")
}

func formatWarnConfigSkipped(_ domain.Warning, localizer Localizer) string {
	return localizer.Get("Configuration file was skipped")
}

func formatWarnConfigUnknownKey(warning domain.Warning, localizer Localizer) string {
	path := warning.Context[domain.WarningContextPath]
	key := warning.Context[domain.WarningContextConfigKey]

	if path != "" && key != "" {
		return localizer.Get("%s: unknown key 'gommitlint.%s' will be ignored (check spelling or remove)", path, key)
	}

	return localizer.Get("Configuration file contains an unknown key (ignored)")
}

func formatWarnConfigRulesOverlap(warning domain.Warning, localizer Localizer) string {
	rules := warning.Context[domain.WarningContextRules]
	if rules == "" {
		return localizer.Get("rules.enabled and rules.disabled share entries; disabled takes precedence")
	}

	return localizer.Get("rules listed in both enabled and disabled (disabled wins): %s — remove the conflict from your config", rules)
}

func formatWarnConfigBadShape(warning domain.Warning, localizer Localizer) string {
	path := warning.Context[domain.WarningContextPath]
	issue := warning.Context[domain.WarningContextIssue]

	switch issue {
	case "missing_root_key":
		return localizer.Get("%s: top-level 'gommitlint:' key is missing, so the file is ignored — wrap your settings under a 'gommitlint:' mapping", path)
	case "root_not_a_map":
		return localizer.Get("%s: 'gommitlint:' value is not a mapping, so the file is ignored — its contents must be a YAML/TOML object", path)
	}

	if warning.Message != "" {
		return warning.Message
	}

	return localizer.Get("Configuration file structure does not match the expected schema")
}

func formatWarnConfigInsecurePerms(warning domain.Warning, localizer Localizer) string {
	path := warning.Context[domain.WarningContextPath]
	if path == "" {
		path = "config file"
	}

	return localizer.Get(
		"%s has insecure permissions (group-writable); loaded anyway. Run 'chmod g-w %s' to silence this warning.",
		path, path)
}

func formatWarnBaseBranchFallback(warning domain.Warning, localizer Localizer) string {
	resolved := warning.Context[domain.WarningContextResolved]
	tried := warning.Context[domain.WarningContextTried]

	return localizer.Get("Using '%s' as base branch ('%s' was not found)", resolved, tried)
}

func formatWarnBaseBranchNotFound(warning domain.Warning, localizer Localizer) string {
	candidates := warning.Context[domain.WarningContextCandidates]

	return localizer.Get("None of the base branches exist (%s), validating HEAD only", candidates)
}

// FormatWarning converts a structured warning to a localized string.
// Falls back to Warning.Message for unknown codes.
func FormatWarning(warning domain.Warning, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if formatter, ok := warningFormatterRegistry()[warning.Code]; ok {
		return formatter(warning, localizer)
	}

	return warning.Message
}

// FormatWarnings converts a slice of structured warnings to localized strings.
func FormatWarnings(warnings []domain.Warning, localizer Localizer) []string {
	result := make([]string, len(warnings))
	for i, warning := range warnings {
		result[i] = FormatWarning(warning, localizer)
	}

	return result
}
