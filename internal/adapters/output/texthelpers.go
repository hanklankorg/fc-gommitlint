// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// Constants for text formatting.
const (
	// MaxLineWidth follows terminal display best practices for readability
	// and compatibility with standard 80-column terminal windows.
	MaxLineWidth  = 80
	DefaultIndent = "    "
	// DividerLength matches terminal width standards for visual separation.
	DividerLength = 80
)

// formatContextKey formats a context key for display.
func formatContextKey(key string) string {
	// Convert snake_case or kebab-case to Title Case
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-'
	})

	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}

	return strings.Join(parts, " ")
}

func truncateSHA(sha string) string {
	return sha[:min(domain.ShortHashLength, len(sha))]
}

// formatTechnicalContextKey formats technical context keys for display.
func formatTechnicalContextKey(key string, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)
	if formatted, ok := formatTechnicalContextKeyPrimary(key, localizer); ok {
		return formatted
	}

	if formatted, ok := formatTechnicalContextKeySecondary(key, localizer); ok {
		return formatted
	}

	if formatted, ok := formatTechnicalContextKeyTertiary(key, localizer); ok {
		return formatted
	}

	return formatContextKey(key)
}

func formatTechnicalContextKeyPrimary(key string, localizer Localizer) (string, bool) {
	switch key {
	case contextCharPosition:
		return localizer.Get("Position"), true
	case contextLineNumber:
		return localizer.Get("Line"), true
	case contextColumnNumber:
		return localizer.Get("Column"), true
	case contextExpectedPattern:
		return localizer.Get(messageExpectedLabel), true
	case contextActualValue:
		return localizer.Get("Found"), true
	default:
		return "", false
	}
}

func formatTechnicalContextKeySecondary(key string, localizer Localizer) (string, bool) {
	switch key {
	case diagnostics.ContextSuggestion:
		return localizer.Get("Suggestion"), true
	case diagnostics.ContextExpected:
		return localizer.Get(messageExpectedLabel), true
	case diagnostics.ContextLocale:
		return localizer.Get("Locale"), true
	case diagnostics.ContextLine:
		return localizer.Get("Line"), true
	case diagnostics.ContextWord:
		return localizer.Get("Word"), true
	case diagnostics.ContextRef:
		return localizer.Get("Reference"), true
	case diagnostics.ContextMax:
		return localizer.Get("Max"), true
	case diagnostics.ContextMin:
		return localizer.Get("Min"), true
	default:
		return "", false
	}
}

func formatTechnicalContextKeyTertiary(key string, localizer Localizer) (string, bool) {
	switch key {
	case contextExpectedWord:
		return localizer.Get("Expected word"), true
	case contextFirstWord:
		return localizer.Get("Current Word"), true
	default:
		return "", false
	}
}
