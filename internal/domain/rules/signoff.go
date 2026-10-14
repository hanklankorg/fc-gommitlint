// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"fmt"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// SignOffRule validates that commit messages include a sign-off line.
type SignOffRule struct {
	minSignoffCount int
}

// NewSignOffRule creates a new rule for validating commit sign-offs from settings.
func NewSignOffRule(cfg settings.Config) SignOffRule {
	return SignOffRule{
		minSignoffCount: settings.DerefInt(cfg.Message.Body.MinSignoffCount, settings.DefaultMinSignoffCount),
	}
}

// Validate implements CommitRule interface.
func (r SignOffRule) Validate(commit domain.Commit) []domain.ValidationError {
	// Skip validation if no sign-offs required
	if r.minSignoffCount == 0 {
		return nil
	}

	var errors []domain.ValidationError

	analysis := analyzeTrailers(commit.Body)
	signoffs := analysis.signOffLines()
	candidates := analysis.signOffCandidates()

	// Validate sign-off count
	if countErrors := r.validateSignoffCount(signoffs); len(countErrors) > 0 {
		errors = append(errors, countErrors...)
	}

	// Validate malformed candidates independently from the valid-signoff count.
	if len(candidates) > 0 {
		signoffErrors := r.validateSignoffDetails(candidates, signoffs, commit.Body)
		errors = append(errors, signoffErrors...)
	}

	return errors
}

// Name implements Rule interface.
func (r SignOffRule) Name() string {
	return rulecatalog.SignOff
}

// validateSignoffDetails validates format, placement, and uniqueness of signoffs.
func (r SignOffRule) validateSignoffDetails(candidates, validSignoffs []string, body string) []domain.ValidationError {
	var errors []domain.ValidationError

	// Validate sign-off format
	if formatErrors := r.validateSignoffFormat(candidates); len(formatErrors) > 0 {
		errors = append(errors, formatErrors...)
	}

	// Validate sign-off placement
	if placementErrors := r.validateSignoffPlacement(body); len(placementErrors) > 0 {
		errors = append(errors, placementErrors...)
	}

	// Validate unique signers (only if multiple required)
	if r.minSignoffCount > 1 {
		if uniqueErrors := r.validateUniqueSigners(validSignoffs); len(uniqueErrors) > 0 {
			errors = append(errors, uniqueErrors...)
		}
	}

	return errors
}

// validateSignoffCount checks if required number of sign-offs are present.
func (r SignOffRule) validateSignoffCount(signoffs []string) []domain.ValidationError {
	if r.minSignoffCount == 0 {
		return nil // No sign-offs required
	}

	actualCount := len(signoffs)
	if actualCount < r.minSignoffCount {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMissingSignoff, "Missing required sign-off").
				WithActualExpected(strconv.Itoa(actualCount), strconv.Itoa(r.minSignoffCount)),
		}
	}

	return nil
}

// validateSignoffFormat validates that all sign-offs have strict DCO format.
// Enforces "Signed-off-by: Name <email@domain>" format for DCO compliance.
func (r SignOffRule) validateSignoffFormat(signoffs []string) []domain.ValidationError {
	var errors []domain.ValidationError

	// Strict DCO compliance: require proper "Name <email@domain>" format
	for _, signoff := range signoffs {
		if !SignOffPattern.MatchString(signoff) {
			errors = append(errors,
				domain.New(r.Name(), diagnostics.ReasonInvalidSignoffFormat, "Invalid DCO sign-off format").
					WithActualExpected(signoff, "Signed-off-by: Real Name <email@domain.com>"))
		}
	}

	return errors
}

// validateSignoffPlacement validates that sign-offs are placed at the end of the body.
func (r SignOffRule) validateSignoffPlacement(body string) []domain.ValidationError {
	return validateSignOffPlacement(
		r.Name(),
		body,
		"Sign-offs must be at the end of the commit message",
		"Content found after sign-off",
		"Sign-offs at end",
	)
}

// validateUniqueSigners validates that when multiple sign-offs are required, they are from different signers.
func (r SignOffRule) validateUniqueSigners(signoffs []string) []domain.ValidationError {
	if r.minSignoffCount <= 1 || len(signoffs) <= 1 {
		return nil // Uniqueness validation not applicable
	}

	seenSigners := make(map[string]struct{})
	duplicates := []string{}

	for _, signoff := range signoffs {
		// Extract email from sign-off line
		email := r.extractEmailFromSignoff(signoff)
		if email == "" {
			continue // Skip malformed sign-offs (format validation handles this)
		}

		if _, ok := seenSigners[email]; ok {
			duplicates = append(duplicates, email)
		} else {
			seenSigners[email] = struct{}{}
		}
	}

	if len(duplicates) > 0 {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInsufficientSignoffs, fmt.Sprintf("Duplicate signers found: %v", duplicates)).
				WithActualExpected(strings.Join(duplicates, ", "), fmt.Sprintf("%d unique signers", r.minSignoffCount)),
		}
	}

	return nil
}

// extractEmailFromSignoff extracts the email address from a sign-off line,
// normalised the way the identity rule compares emails (trimmed, lower case),
// so "<Dev@Example.com>" and "<dev@example.com>" are one signer.
func (r SignOffRule) extractEmailFromSignoff(signoff string) string {
	// Match email in angle brackets
	matches := EmailPattern.FindStringSubmatch(signoff)
	if len(matches) >= 2 {
		return strings.ToLower(strings.TrimSpace(matches[1]))
	}

	return ""
}
