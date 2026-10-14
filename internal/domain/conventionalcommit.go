// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"regexp"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// ConventionalCommitFormat the parsed components of a conventional commit.
type ConventionalCommitFormat struct {
	Type        string   // e.g., "feat", "fix", "docs"
	Scopes      []string // e.g., ["ui", "api"] from "feat(ui,api):"
	Breaking    bool     // true if "!" is present
	Description string   // the text after the colon
	RawScope    string   // original scope string for error reporting
	IsValid     bool     // whether the subject matches the supported parser shape
}

var (
	// conventionalCommitRegex parses gommitlint's supported Conventional Commit shape.
	// Spacing and content requirements are validated separately by the rule.
	// Pattern breakdown:
	//   ^(?P<type>[a-z]+)            - Required lowercase ASCII type
	//   (?:\((?P<scope>[...]+)\))?   - Optional scope in parentheses
	//   (?P<breaking>!)?             - Optional breaking change indicator
	//   :(?P<space>\s?)              - Required colon, optional space
	//   (?P<description>.*)          - Commit description (rest of line)
	//
	// Valid examples:
	//   ✓ "feat: add user authentication"
	//   ✓ "fix(api): resolve timeout issue"
	//   ✓ "feat(ui,auth)!: breaking login changes"
	//   ✗ "Add feature" (no type/colon)
	//   ✗ "feat : space before colon"
	conventionalCommitRegex = regexp.MustCompile(
		`^(?P<type>[a-z]+)(?:\((?P<scope>[a-zA-Z0-9/,-]+)\))?(?P<breaking>!)?:(?P<space>\s?)(?P<description>.*)`,
	)

	// partialConventionalRegex detects attempted conventional format with errors.
	// Matches: type-like prefix followed by colon/paren/exclamation.
	// Examples: "feat(" "fix:" "docs!" - helps provide specific error messages.
	// Matches: "feat:", "fix(scope):", "docs!:" even with spacing issues.
	partialConventionalRegex = regexp.MustCompile(`^(?:\w+)(?:\([^)]*\))?(?:!)?:`)

	// malformedScopePattern detects unclosed parentheses in scope.
	// Matches: "feat(api" without closing parenthesis.
	malformedScopePattern = regexp.MustCompile(`^[a-zA-Z]+\([^)]*$`)
)

func conventionalTypePrefixPattern(types []string) *regexp.Regexp {
	quoted := make([]string, 0, len(types))
	for _, commitType := range types {
		quoted = append(quoted, regexp.QuoteMeta(commitType))
	}

	return regexp.MustCompile(`^(?:` + strings.Join(quoted, "|") + `)\s+`)
}

// ParseConventionalCommit parses a commit subject and returns its conventional commit components.
// Uses the same parsing logic as the ConventionalCommit rule for consistency.
func ParseConventionalCommit(subject string) ConventionalCommitFormat {
	// Don't trim the subject as spacing is part of the conventional format validation
	match := conventionalCommitRegex.FindStringSubmatch(subject)
	if match == nil {
		return ConventionalCommitFormat{
			IsValid: false,
		}
	}

	// Extract named groups using the same approach as ConventionalCommit rule
	subexpNames := conventionalCommitRegex.SubexpNames()
	groups := make(map[string]string, len(subexpNames))

	for i, name := range subexpNames {
		if i != 0 && name != "" && i < len(match) {
			groups[name] = match[i]
		}
	}

	// Parse scopes (support multiple scopes like "ui,api")
	var scopes []string

	rawScope := groups["scope"]
	if rawScope != "" {
		if strings.Contains(rawScope, ",") {
			// Split by comma and trim whitespace
			parts := strings.Split(rawScope, ",")

			scopes = make([]string, 0, len(parts))
			for _, scope := range parts {
				if trimmed := strings.TrimSpace(scope); trimmed != "" {
					scopes = append(scopes, trimmed)
				}
			}
		} else {
			scopes = []string{rawScope}
		}
	}

	description := strings.TrimSpace(groups["description"])

	// A valid conventional commit structure just requires the basic format
	// Individual rules handle spacing and content validation
	isValidFormat := true

	return ConventionalCommitFormat{
		Type:        groups["type"],
		Scopes:      scopes,
		Breaking:    groups["breaking"] == "!",
		Description: description,
		RawScope:    rawScope,
		IsValid:     isValidFormat,
	}
}

// IsConventionalCommitLike reports whether the subject looks like it's trying to be
// a conventional commit but may have formatting issues. When allowedTypes is empty,
// the standard conventional types are used.
func IsConventionalCommitLike(subject string, allowedTypes []string) bool {
	subject = strings.TrimSpace(subject)

	if partialConventionalRegex.MatchString(subject) {
		return true
	}

	if malformedScopePattern.MatchString(subject) {
		return true
	}

	if len(allowedTypes) == 0 {
		allowedTypes = settings.GetDefaultConventionalTypes()
	}

	return conventionalTypePrefixPattern(allowedTypes).MatchString(subject)
}
