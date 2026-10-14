// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Conventional commit validation errors.
var (
	ErrSubjectNotConventional = errors.New("subject does not match conventional format")
)

type conventionalParts = domain.ConventionalCommitFormat

// ConventionalCommitRule validates the supported Conventional Commit subject
// shape and configured type, scope, description, and breaking-change policy.
type ConventionalCommitRule struct {
	allowedTypes     []string
	allowedScopes    []string
	requireScope     bool
	disallowBreaking bool
	allowMultiScope  bool
	maxDescLength    int
}

// NewConventionalCommitRule creates a new rule for validating conventional commits from settings.
func NewConventionalCommitRule(cfg settings.Config) ConventionalCommitRule {
	allowedTypes := cfg.Conventional.Types

	maxDescLength := settings.DerefPositiveInt(cfg.Conventional.MaxDescriptionLength, settings.DefaultConventionalMaxDescLength)

	return ConventionalCommitRule{
		allowedTypes:     allowedTypes,
		allowedScopes:    cfg.Conventional.Scopes,
		requireScope:     settings.DerefBool(cfg.Conventional.RequireScope, false),
		disallowBreaking: settings.DerefBool(cfg.Conventional.DisallowBreaking, false),
		allowMultiScope:  settings.DerefBool(cfg.Conventional.AllowMultiScope, false),
		maxDescLength:    maxDescLength,
	}
}

// Name implements Rule interface.
func (r ConventionalCommitRule) Name() string {
	return rulecatalog.Conventional
}

// Validate implements CommitRule interface.
func (r ConventionalCommitRule) Validate(commit domain.Commit) []domain.ValidationError {
	// Parse conventional format with strict spacing if enabled
	parts, err := r.parseConventionalFormat(commit.Subject)
	if err != nil {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidConventionalFormat, "Must follow format: "+domain.ConventionalCommitFormatTemplate).
				WithActualExpected(commit.Subject, domain.ConventionalCommitFormatTemplate)}
	}

	var typeErrors []domain.ValidationError
	if !isValidType(parts.Type, r.allowedTypes) {
		typeErrors = []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidConventionalType,
				fmt.Sprintf("Invalid type '%s'", parts.Type)).
				WithActualExpected(parts.Type, strings.Join(r.allowedTypes, ", "))}
	}

	// Characters, not bytes: the subject and body rules count the same way,
	// so one multibyte description must not measure differently here.
	descriptionLength := utf8.RuneCountInString(parts.Description)

	var lengthErrors []domain.ValidationError
	if r.maxDescLength > 0 && descriptionLength > r.maxDescLength {
		lengthErrors = []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonConventionalDescTooLong,
				fmt.Sprintf("Description too long (%d > %d)", descriptionLength, r.maxDescLength)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextActual:      strconv.Itoa(descriptionLength),
					diagnostics.ContextExpectedMax: strconv.Itoa(r.maxDescLength),
				})}
	}

	// Combine all validation errors using functional composition
	return domain.CombineValidationErrors(
		r.validateSpacing(commit.Subject, parts),
		r.validateDescription(parts),
		typeErrors,
		r.validateScopes(parts),
		r.validateBreakingChange(parts),
		lengthErrors,
	)
}

// parseConventionalFormat parses a commit subject into conventional commit parts.
func (r ConventionalCommitRule) parseConventionalFormat(subject string) (conventionalParts, error) {
	parsed := domain.ParseConventionalCommit(subject)
	if !parsed.IsValid {
		return conventionalParts{}, ErrSubjectNotConventional
	}

	return parsed, nil
}

// validateSpacing validates colon spacing per specification.
func (r ConventionalCommitRule) validateSpacing(subject string, _ conventionalParts) []domain.ValidationError {
	var failures []domain.ValidationError

	colonIndex := strings.Index(subject, ":")
	if colonIndex == -1 || colonIndex >= len(subject)-1 {
		return failures
	}

	afterColon := subject[colonIndex+1:]

	// Check for missing space after colon
	if len(afterColon) == 0 || afterColon[0] != ' ' {
		failures = append(failures,
			domain.New(r.Name(), diagnostics.ReasonInvalidSpacing, "Missing space after colon").
				WithActualExpected(subject, "type: description"))

		return failures
	}

	// Check for extra whitespace after the required single space
	if len(afterColon) > 1 && (afterColon[1] == ' ' || afterColon[1] == '\t' || afterColon[1] == '\n' || afterColon[1] == '\r') {
		failures = append(failures,
			domain.New(r.Name(), diagnostics.ReasonInvalidSpacing, "Invalid whitespace after colon").
				WithActualExpected(subject, "type: description"))
	}

	return failures
}

// validateDescription validates the description content.
func (r ConventionalCommitRule) validateDescription(parts conventionalParts) []domain.ValidationError {
	var failures []domain.ValidationError

	trimmedDesc := strings.TrimSpace(parts.Description)
	if trimmedDesc == "" {
		failures = append(failures,
			domain.New(r.Name(), diagnostics.ReasonEmptyConventionalDesc, "Description cannot be empty").
				WithActualExpected("empty", "meaningful description"))
	}

	return failures
}

// validateBreakingChange validates breaking change indicator when not allowed.
func (r ConventionalCommitRule) validateBreakingChange(parts conventionalParts) []domain.ValidationError {
	if !r.disallowBreaking || !parts.Breaking {
		return nil
	}

	return []domain.ValidationError{
		domain.New(r.Name(), diagnostics.ReasonBreakingChangeNotAllowed, "Breaking change indicator (!) is not allowed").
			WithActualExpected(parts.Type+"!", parts.Type),
	}
}

// validateScopes validates scope requirements and allowed scopes.
func (r ConventionalCommitRule) validateScopes(parts conventionalParts) []domain.ValidationError {
	// Required scope check must pass first - can't validate missing scope
	if errs := r.validateRequiredScope(parts); len(errs) > 0 {
		return errs
	}

	// Multi-scope allowance must pass - format check is irrelevant if disallowed
	if errs := r.validateMultiScopeAllowed(parts); len(errs) > 0 {
		return errs
	}

	// Collect all remaining scope errors so user sees them all at once
	allowedScopeErrors := r.validateAllowedScopes(parts)
	formatErrors := r.validateMultiScopeFormat(parts)

	if len(allowedScopeErrors) == 0 && len(formatErrors) == 0 {
		return nil
	}

	allErrors := make([]domain.ValidationError, 0, len(allowedScopeErrors)+len(formatErrors))
	allErrors = append(allErrors, allowedScopeErrors...)
	allErrors = append(allErrors, formatErrors...)

	return allErrors
}

// validateRequiredScope checks if scope is required but missing.
func (r ConventionalCommitRule) validateRequiredScope(parts conventionalParts) []domain.ValidationError {
	if r.requireScope && len(parts.Scopes) == 0 {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMissingConventionalScope, "Scope is required but not provided").
				WithActualExpected(
					fmt.Sprintf("%s: %s", parts.Type, parts.Description),
					fmt.Sprintf("%s(scope): %s", parts.Type, parts.Description),
				),
		}
	}

	return nil
}

// validateMultiScopeAllowed checks if multiple scopes are used when not allowed.
func (r ConventionalCommitRule) validateMultiScopeAllowed(parts conventionalParts) []domain.ValidationError {
	if !r.allowMultiScope && len(parts.Scopes) > 1 {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMultiScopeNotAllowed, "Multiple scopes are not allowed").
				WithActualExpected(parts.RawScope, "single scope"),
		}
	}

	return nil
}

// validateAllowedScopes validates each scope against allowed scopes.
func (r ConventionalCommitRule) validateAllowedScopes(parts conventionalParts) []domain.ValidationError {
	if len(parts.Scopes) == 0 || len(r.allowedScopes) == 0 {
		return nil
	}

	var failures []domain.ValidationError

	for _, scope := range parts.Scopes {
		if !isValidScope(scope, r.allowedScopes) {
			failures = append(failures,
				domain.New(r.Name(), diagnostics.ReasonInvalidConventionalScope,
					fmt.Sprintf("Invalid scope '%s'", scope)).
					WithActualExpected(scope, strings.Join(r.allowedScopes, ", ")))
		}
	}

	return failures
}

// validateMultiScopeFormat refuses malformed comma syntax in both modes. The
// parser drops empty segments, so "feat(api,): x" reaches here as one scope
// and would otherwise pass when multiple scopes are not allowed.
func (r ConventionalCommitRule) validateMultiScopeFormat(parts conventionalParts) []domain.ValidationError {
	if parts.RawScope != "" && strings.Contains(parts.RawScope, ",") {
		if !isValidMultiScopeFormat(parts.RawScope) {
			return []domain.ValidationError{
				domain.New(r.Name(), diagnostics.ReasonInvalidMultiScope, "Invalid multi-scope format").
					WithActualExpected(parts.RawScope, "scope1,scope2"),
			}
		}
	}

	return nil
}

// isValidType validates commit type against allowed or standard types.
func isValidType(commitType string, allowedTypes []string) bool {
	if len(allowedTypes) == 0 {
		return slices.Contains(settings.GetDefaultConventionalTypes(), commitType)
	}

	return slices.Contains(allowedTypes, commitType)
}

// isValidScope validates scope against allowed scopes list.
func isValidScope(scope string, allowedScopes []string) bool {
	if len(allowedScopes) == 0 {
		return true
	}

	return slices.Contains(allowedScopes, scope)
}

// isValidMultiScopeFormat checks if multi-scope format is valid.
func isValidMultiScopeFormat(rawScope string) bool {
	// Valid: "ui,api", "frontend,backend"
	// Invalid: "ui, api", "ui ,api", "ui , api"
	// Should not have spaces around commas
	if strings.Contains(rawScope, " ,") || strings.Contains(rawScope, ", ") {
		return false
	}

	// Should not start or end with comma
	if strings.HasPrefix(rawScope, ",") || strings.HasSuffix(rawScope, ",") {
		return false
	}

	// Should not have consecutive commas
	if strings.Contains(rawScope, ",,") {
		return false
	}

	return true
}
