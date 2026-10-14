// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// CommitBodyRule validates commit message bodies.
type CommitBodyRule struct {
	required         bool
	minLength        int
	maxLineLength    int
	allowSignOffOnly bool
}

// NewCommitBodyRule creates a new CommitBodyRule from settings.
func NewCommitBodyRule(cfg settings.Config) CommitBodyRule {
	return CommitBodyRule{
		required:         settings.DerefBool(cfg.Message.Body.Required, false),
		minLength:        settings.DerefInt(cfg.Message.Body.MinLength, 0),
		maxLineLength:    settings.DerefInt(cfg.Message.Body.MaxLineLength, 0),
		allowSignOffOnly: settings.DerefBool(cfg.Message.Body.AllowSignoffOnly, false),
	}
}

// Name implements Rule interface.
func (r CommitBodyRule) Name() string {
	return rulecatalog.CommitBody
}

// Validate implements CommitRule interface.
func (r CommitBodyRule) Validate(commit domain.Commit) []domain.ValidationError {
	trimmedBody := strings.TrimSpace(commit.Body)

	// Functional composition of validations
	// Pre-allocate with reasonable capacity (most commits have 0-3 errors)
	errors := make([]domain.ValidationError, 0, 4)

	errors = append(errors, r.validateStructure(commit)...)
	errors = append(errors, r.validateLength(trimmedBody)...)
	errors = append(errors, r.validateLineLength(commit.Body)...)
	errors = append(errors, r.validateSignOffRules(trimmedBody)...)

	return errors
}

// validateStructure validates Git message structure (subject + blank line + body).
func (r CommitBodyRule) validateStructure(commit domain.Commit) []domain.ValidationError {
	lines := strings.Split(commit.Message, "\n")

	// Always validate blank line separation when there's content after subject
	if len(lines) >= 2 && strings.TrimSpace(lines[1]) != "" {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMissingBlankLine, "Missing blank line between subject and body").
				WithActualExpected("subject + body (no blank line)", "subject + blank line + body"),
		}
	}

	if r.required && strings.TrimSpace(commit.Body) == "" {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMissingBody, "Missing body").
				WithActualExpected("no body", "subject + blank line + body"),
		}
	}

	return nil
}

// validateLength validates the minimum length of a body that is present.
// Whether a body must exist at all is required's decision, made in
// validateStructure; min_length judges any non-empty body on its own.
func (r CommitBodyRule) validateLength(trimmedBody string) []domain.ValidationError {
	if r.minLength == 0 || trimmedBody == "" {
		return nil
	}

	// min_length is documented as a character count, like max_line_length.
	bodyLength := utf8.RuneCountInString(trimmedBody)
	if bodyLength < r.minLength {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonBodyTooShort,
				fmt.Sprintf("Too short (%d/%d characters)", bodyLength, r.minLength)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextActual:   strconv.Itoa(bodyLength),
					diagnostics.ContextExpected: fmt.Sprintf("min %d", r.minLength),
					diagnostics.ContextMin:      strconv.Itoa(r.minLength),
				}),
		}
	}

	return nil
}

// validateLineLength validates that each line in the body does not exceed the max line length.
func (r CommitBodyRule) validateLineLength(body string) []domain.ValidationError {
	if r.maxLineLength == 0 || body == "" {
		return nil
	}

	lines := strings.Split(body, "\n")

	var errors []domain.ValidationError

	for lineIdx, line := range lines {
		// Characters, not bytes: the limit is documented in characters and the
		// subject rule counts the same way. A 71-character Swedish line is
		// well over 72 bytes.
		lineLen := utf8.RuneCountInString(line)
		if lineLen > r.maxLineLength {
			// A single unbreakable token (URL, long identifier) cannot be
			// wrapped, so lines without whitespace between tokens are exempt.
			if len(strings.Fields(line)) <= 1 {
				continue
			}

			// An identity trailer cannot be wrapped or shortened either: Git
			// writes it from the author's name and address. Whether it is a
			// valid sign-off is the signoff rule's decision, not this one's.
			if IdentityTrailerPattern.MatchString(line) {
				continue
			}

			// Line numbers are 1-indexed for user display
			lineNum := lineIdx + 1
			errors = append(errors, domain.New(r.Name(), diagnostics.ReasonBodyLineTooLong,
				fmt.Sprintf("Line %d exceeds %d characters (%d found)", lineNum, r.maxLineLength, lineLen)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextLine:     strconv.Itoa(lineNum),
					diagnostics.ContextActual:   strconv.Itoa(lineLen),
					diagnostics.ContextExpected: fmt.Sprintf("max %d", r.maxLineLength),
					diagnostics.ContextMax:      strconv.Itoa(r.maxLineLength),
				}))
		}
	}

	return errors
}

// validateSignOffRules validates sign-off positioning and content rules.
func (r CommitBodyRule) validateSignOffRules(trimmedBody string) []domain.ValidationError {
	if trimmedBody == "" {
		return nil
	}

	// Pre-allocate with reasonable capacity
	errors := make([]domain.ValidationError, 0, 2)

	errors = append(errors, r.validateSignOffPositioning(trimmedBody)...)

	if r.required && !r.allowSignOffOnly {
		// Show only the most relevant error to avoid confusion
		errors = append(errors, r.validateSignOffContent(trimmedBody)...)
	}

	return errors
}

// validateSignOffPositioning ensures sign-offs are at the end.
func (r CommitBodyRule) validateSignOffPositioning(body string) []domain.ValidationError {
	return validateSignOffPlacement(r.Name(), body, "Content found after sign-off lines", "", "sign-off lines at end")
}

// validateSignOffContent checks if body has only sign-offs (when not allowed).
func (r CommitBodyRule) validateSignOffContent(body string) []domain.ValidationError {
	if body == "" {
		return nil
	}

	if r.hasOnlySignOffLines(body) {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidBody, "Body contains only sign-off lines").
				WithActualExpected("only sign-off lines", "descriptive content + sign-offs"),
		}
	}

	firstLine := strings.TrimSpace(strings.Split(body, "\n")[0])

	if SignOffPattern.MatchString(firstLine) {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidBody, "Body should start with descriptive content").
				WithActualExpected("starts with sign-off", "descriptive content first"),
		}
	}

	return nil
}

// hasOnlySignOffLines checks if body contains only sign-off lines.
func (r CommitBodyRule) hasOnlySignOffLines(body string) bool {
	if body == "" {
		return false
	}

	return analyzeTrailers(body).hasOnlySignOffLines()
}
