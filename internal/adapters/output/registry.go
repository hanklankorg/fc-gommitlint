// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"slices"
	"strings"
)

// Output format name constants. These are the single source of truth
// for valid format identifiers throughout the adapter layer.
const (
	FormatText    = "text"
	FormatPlain   = "plain"
	FormatJSON    = "json"
	FormatGitHub  = "github"
	FormatGitLab  = "gitlab"
	FormatForgejo = "forgejo"
	FormatTable   = "table"
	FormatSARIF   = "sarif"
)

// DefaultFormat is the default output format.
const DefaultFormat = FormatText

// DefaultTheme is the default output theme.
const DefaultTheme = "default"

// NormalizeThemeName standardizes output theme names for comparisons.
func NormalizeThemeName(theme string) string {
	cleaned := strings.TrimSpace(theme)
	cleaned = strings.Trim(cleaned, "\"'")
	cleaned = strings.ToLower(strings.TrimSpace(cleaned))

	return cleaned
}

// formatMeta describes the classification of a single output format. It is the
// single source of truth from which the format predicates below are derived,
// mirroring the descriptor-table pattern of diagnostics.Definitions().
type formatMeta struct {
	name             string
	machineReadable  bool
	gitHubCompatible bool
}

// formats returns the metadata for every supported output format in stable order.
//
// Note: "forgejo" is an alias for "github" since Forgejo Actions uses
// GitHub-compatible workflow commands.
func formats() []formatMeta {
	return []formatMeta{
		{name: FormatText},
		{name: FormatPlain, machineReadable: true},
		{name: FormatJSON, machineReadable: true},
		{name: FormatGitHub, machineReadable: true, gitHubCompatible: true},
		{name: FormatGitLab, machineReadable: true},
		{name: FormatForgejo, machineReadable: true, gitHubCompatible: true},
		{name: FormatTable},
		{name: FormatSARIF, machineReadable: true},
	}
}

// byName returns the metadata for a format name, mirroring diagnostics.ByID.
func byName(format string) (formatMeta, bool) {
	for _, meta := range formats() {
		if meta.name == format {
			return meta, true
		}
	}

	return formatMeta{}, false
}

// SupportedFormats provides the complete, sorted list of available output formats.
func SupportedFormats() []string {
	metas := formats()
	names := make([]string, 0, len(metas))

	for _, meta := range metas {
		names = append(names, meta.name)
	}

	slices.Sort(names)

	return names
}

// IsValidFormat reports whether the given output format is supported by the formatter registry.
func IsValidFormat(format string) bool {
	_, ok := byName(format)

	return ok
}

// IsMachineReadableFormat reports whether the format produces machine-parseable output.
func IsMachineReadableFormat(format string) bool {
	meta, ok := byName(format)

	return ok && meta.machineReadable
}

// IsGitHubCompatibleFormat reports whether the format is compatible with GitHub Actions workflow commands.
func IsGitHubCompatibleFormat(format string) bool {
	meta, ok := byName(format)

	return ok && meta.gitHubCompatible
}

// IsHumanReadableFormat reports whether the format is intended for human consumption.
func IsHumanReadableFormat(format string) bool {
	return !IsMachineReadableFormat(format)
}
