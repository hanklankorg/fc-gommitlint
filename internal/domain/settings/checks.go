// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

import (
	"fmt"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// NormalizeChecksConfig standardizes check IDs and removes empties while preserving order.
func NormalizeChecksConfig(config ChecksConfig) ChecksConfig {
	config.Enabled = normalizeCheckList(config.Enabled)
	config.Disabled = normalizeCheckList(config.Disabled)

	return config
}

// ValidateChecksConfig checks check enable/disable configuration.
func ValidateChecksConfig(config ChecksConfig) []string {
	errors := make([]string, 0, len(config.Enabled)+len(config.Disabled))

	errors = append(errors, validateCheckList("enabled", config.Enabled)...)
	errors = append(errors, validateCheckList("disabled", config.Disabled)...)

	return errors
}

// RuleNamesForEnabledChecks returns rule config names activated by checks.enabled.
func RuleNamesForEnabledChecks(config ChecksConfig) []string {
	normalized := NormalizeChecksConfig(config)
	rules := make([]string, 0, len(normalized.Enabled))
	seen := make(map[string]struct{}, len(normalized.Enabled))

	for _, id := range normalized.Enabled {
		definition, ok := diagnostics.ByID(id)
		if !ok || definition.RuleID == "" {
			continue
		}

		if _, exists := seen[definition.RuleID]; exists {
			continue
		}

		seen[definition.RuleID] = struct{}{}
		rules = append(rules, definition.RuleID)
	}

	return rules
}

// IsCheckDisabled reports whether a public check ID is explicitly disabled.
func IsCheckDisabled(config ChecksConfig, checkID string) bool {
	normalized := NormalizeChecksConfig(config)
	want := diagnostics.NormalizeID(checkID)

	return slices.Contains(normalized.Disabled, want)
}

func validateCheckList(listName string, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	validList := strings.Join(diagnostics.IDs(), ", ")
	errors := make([]string, 0, len(ids))

	for _, id := range ids {
		cleaned := diagnostics.NormalizeID(id)
		if cleaned == "" {
			errors = append(errors, fmt.Sprintf("checks %s contains empty check ID", listName))

			continue
		}

		definition, ok := diagnostics.ByID(cleaned)
		if !ok {
			errors = append(errors, fmt.Sprintf("checks %s contains unknown check ID %q (valid: %s)", listName, id, validList))

			continue
		}

		if err := validateConfigurableCheckID(listName, definition); err != "" {
			errors = append(errors, err)
		}
	}

	return errors
}

func validateConfigurableCheckID(listName string, definition diagnostics.Definition) string {
	if definition.SourceKind == diagnostics.SourceRule {
		return ""
	}

	if listName == "disabled" && definition.ID == diagnostics.CheckFixupCommit {
		return ""
	}

	return fmt.Sprintf("checks %s contains dispatcher check ID %q, which is not configurable with checks.%s", listName, definition.ID, listName)
}

func normalizeCheckList(ids []string) []string {
	if len(ids) == 0 {
		return ids
	}

	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))

	for _, id := range ids {
		cleaned := diagnostics.NormalizeID(id)
		if cleaned == "" {
			continue
		}

		if _, ok := seen[cleaned]; ok {
			continue
		}

		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}

	return normalized
}
