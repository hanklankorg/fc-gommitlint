// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"
	"unicode"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// DisplayName returns a user-friendly, localized name for a rule.
func DisplayName(ruleName string, localizer Localizer) string {
	if ruleName == "" {
		return ""
	}

	localizer = ensureLocalizer(localizer)

	normalized := strings.ToLower(ruleName)
	if display, ok := ruleDisplayNames(localizer)[normalized]; ok {
		return display
	}

	return strings.Join(splitPascalCase(ruleName), " ")
}

func ruleDisplayNames(localizer Localizer) map[string]string {
	names := make(map[string]string, len(rulecatalog.Rules()))

	for _, rule := range rulecatalog.Rules() {
		names[rule.ID] = Msg(rule.DisplayName).Localize(localizer)
	}

	return names
}

func splitPascalCase(text string) []string {
	estimatedWords := len(text)/3 + 1
	words := make([]string, 0, estimatedWords)

	var current strings.Builder

	for _, char := range text {
		if unicode.IsSpace(char) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}

			continue
		}

		if current.Len() > 0 && 'A' <= char && char <= 'Z' {
			words = append(words, current.String())
			current.Reset()
		}

		current.WriteRune(char)
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}
