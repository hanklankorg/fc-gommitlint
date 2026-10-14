// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// IdentityRule validates that commit authors are in the allowed list.
// Matching is done by email only (case-insensitive).
// Supports domain wildcards: "*@company.com" matches any email from that domain.
type IdentityRule struct {
	allowedAuthors []string
	configErrors   []domain.ValidationError
}

// NewIdentityRule creates a new rule for validating author identity from settings.
func NewIdentityRule(cfg settings.Config) IdentityRule {
	rule := IdentityRule{
		allowedAuthors: make([]string, 0, len(cfg.Identity.AllowedAuthors)),
	}

	for _, author := range cfg.Identity.AllowedAuthors {
		normalized, err := settings.ValidateIdentityAuthorEntry(author)
		if err != nil {
			validationErr := domain.New(rulecatalog.Identity, diagnostics.ReasonInvalidConfig, err.Message).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextInvalidEntry: author,
					"valid_formats":                 err.ValidFormats,
				})
			rule.configErrors = append(rule.configErrors, validationErr)
		} else if normalized != "" {
			rule.allowedAuthors = append(rule.allowedAuthors, normalized)
		}
	}

	return rule
}

// Name implements Rule interface.
func (r IdentityRule) Name() string {
	return rulecatalog.Identity
}

// Validate implements CommitRule interface.
func (r IdentityRule) Validate(commit domain.Commit) []domain.ValidationError {
	// Return config errors first
	if len(r.configErrors) > 0 {
		return r.configErrors
	}

	// If no allowed authors configured, allow all authors
	if len(r.allowedAuthors) == 0 {
		return nil
	}

	return r.validateAuthorEmail(commit)
}

// validateAuthorEmail checks if commit author email matches allowed list.
func (r IdentityRule) validateAuthorEmail(commit domain.Commit) []domain.ValidationError {
	authorEmail := strings.ToLower(strings.TrimSpace(commit.AuthorEmail))

	for _, allowed := range r.allowedAuthors {
		if matchesEmail(allowed, authorEmail) {
			return nil
		}
	}

	return []domain.ValidationError{
		domain.New(r.Name(), diagnostics.ReasonIdentityNotAllowed, "Author email not authorized").
			WithContextMap(diagnostics.Context{
				diagnostics.ContextAuthorEmail: commit.AuthorEmail,
			}),
	}
}

// matchesEmail checks if an allowed pattern matches the author email.
// Supports exact email match and domain wildcard (*@domain.com).
func matchesEmail(allowed, authorEmail string) bool {
	// Domain wildcard: *@domain.com
	if strings.HasPrefix(allowed, "*@") {
		domainSuffix := allowed[1:] // "@domain.com"

		return strings.HasSuffix(authorEmail, domainSuffix)
	}

	// Exact email match (already lowercased)
	return allowed == authorEmail
}
