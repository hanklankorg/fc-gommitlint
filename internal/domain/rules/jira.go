// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Pre-compiled regex patterns for JIRA validation performance.
var (
	// jiraKeyRegex matches a key standing alone: "[PROJ-123]" and "(PROJ-123)"
	// carry one, "xPROJ-123" and "PROJ-123abc" do not.
	jiraKeyRegex         = regexp.MustCompile(`\b[A-Z]+-\d+\b`)
	refsLineRegex        = regexp.MustCompile(`^Refs:\s+[A-Z]+-\d+`)
	jiraLikePrefixRegex  = regexp.MustCompile(`^[A-Za-z]+-`)
	validJiraFormatRegex = regexp.MustCompile(`^[A-Z]+-\d+$`)
	refsWithSpaceRegex   = regexp.MustCompile(`^Refs\s+[A-Z]+-\d+`)
	refsLowercaseRegex   = regexp.MustCompile(`^Refs:\s+[a-z]+-\d+`)
)

// JiraReferenceRule validates that commit messages reference JIRA issues.
type JiraReferenceRule struct {
	cfg                   settings.JiraConfig
	ignorePatterns        []*regexp.Regexp
	isConventionalEnabled bool
	configErrors          []domain.ValidationError
}

// NewJiraReferenceRule creates a new rule for validating JIRA references.
func NewJiraReferenceRule(cfg settings.Config, isConventionalEnabled bool) JiraReferenceRule {
	rule := JiraReferenceRule{
		cfg:                   cfg.Jira,
		isConventionalEnabled: isConventionalEnabled,
	}

	ignorePatterns, patternErrors := settings.CompileJiraIgnorePatterns(cfg.Jira.IgnoreTicketPatterns)
	rule.ignorePatterns = ignorePatterns

	for _, patternErr := range patternErrors {
		rule.configErrors = append(rule.configErrors, domain.New(
			rulecatalog.JiraReference,
			diagnostics.ReasonInvalidConfig,
			"Invalid regex in ignore_ticket_patterns",
		).WithContextMap(diagnostics.Context{
			diagnostics.ContextPattern: patternErr.Pattern,
			diagnostics.ContextError:   patternErr.Err.Error(),
		}))
	}

	return rule
}

// Name implements Rule interface.
func (r JiraReferenceRule) Name() string {
	return rulecatalog.JiraReference
}

// Validate implements Rule interface.
func (r JiraReferenceRule) Validate(commit domain.Commit) []domain.ValidationError {
	// Return config errors first
	if len(r.configErrors) > 0 {
		return r.configErrors
	}

	return validateJiraReferences(
		commit,
		r.cfg,
		r.ignorePatterns,
		r.isConventionalEnabled,
	)
}

// Pure functions below - no receiver, explicit parameters

type jiraSubjectContext struct {
	parsed   domain.ConventionalCommitFormat
	excluded bool
}

func newJiraSubjectContext(subject string, isConventionalEnabled bool) jiraSubjectContext {
	parsed := domain.ParseConventionalCommit(subject)

	return jiraSubjectContext{
		parsed:   parsed,
		excluded: isConventionalEnabled && parsed.IsValid && isJiraExcludedConventionalType(parsed.Type),
	}
}

// validateJiraReferences validates JIRA reference requirements.
func validateJiraReferences(commit domain.Commit, cfg settings.JiraConfig, ignorePatterns []*regexp.Regexp, isConventionalEnabled bool) []domain.ValidationError {
	subjectContext := newJiraSubjectContext(commit.Subject, isConventionalEnabled)

	if subjectContext.excluded {
		return nil
	}

	var errors []domain.ValidationError

	// Validate subject if required
	if settings.DerefBool(cfg.RequireInSubject, false) {
		errors = append(errors, validateSubjectJira(commit.Subject, subjectContext)...)
	}

	// Validate body if required
	if settings.DerefBool(cfg.RequireInBody, false) {
		errors = append(errors, validateBodyJira(commit.Body, cfg)...)
	}

	// General presence validation
	errors = append(errors, validateGeneralJiraPresence(commit, cfg, ignorePatterns)...)

	return errors
}

func isJiraExcludedConventionalType(commitType string) bool {
	switch strings.ToLower(commitType) {
	case "docs", "chore", "style", "refactor", "test":
		return true
	default:
		return false
	}
}

// validateGeneralJiraPresence validates overall JIRA presence in commit.
func validateGeneralJiraPresence(commit domain.Commit, cfg settings.JiraConfig, ignorePatterns []*regexp.Regexp) []domain.ValidationError {
	// Extract all JIRA references
	references := extractAllJiraReferences(commit, ignorePatterns)

	// With project prefixes configured, only a key carrying one of them is a
	// reference: "SHA-256" or an unlisted project does not satisfy presence.
	if !hasListedReference(references, cfg.ProjectPrefixes) && !settings.DerefBool(cfg.RequireInSubject, false) && !settings.DerefBool(cfg.RequireInBody, false) {
		return []domain.ValidationError{createMissingJiraError(commit.Subject)}
	}

	// Validate project prefixes if configured
	if len(cfg.ProjectPrefixes) > 0 {
		return validateProjectPrefixes(references, cfg.ProjectPrefixes)
	}

	return nil
}

// extractAllJiraReferences extracts all JIRA references from commit.
func extractAllJiraReferences(commit domain.Commit, ignorePatterns []*regexp.Regexp) []string {
	// Extract from subject and body
	subjectRefs := extractJiraReferences(commit.Subject)
	bodyRefs := extractJiraReferences(commit.Body)

	references := make([]string, 0, len(subjectRefs)+len(bodyRefs))
	references = append(references, subjectRefs...)
	references = append(references, bodyRefs...)

	// Filter ignored patterns
	return filterIgnoredPatterns(references, ignorePatterns)
}

// hasListedReference reports whether any reference carries an allowed project
// prefix; with no prefixes configured every reference counts.
func hasListedReference(references []string, validPrefixes []string) bool {
	for _, ref := range references {
		if isValidProject(extractProjectFromReference(ref), validPrefixes) {
			return true
		}
	}

	return false
}

// createMissingJiraError creates error for missing JIRA reference.
func createMissingJiraError(subject string) domain.ValidationError {
	return domain.New(
		rulecatalog.JiraReference,
		diagnostics.ReasonMissingJira,
		"No JIRA ticket reference found in commit message",
	).WithContextMap(diagnostics.Context{
		diagnostics.ContextSubject: subject,
	})
}

// extractJiraReferences extracts JIRA references from text.
func extractJiraReferences(text string) []string {
	if text == "" {
		return nil
	}

	matches := jiraKeyRegex.FindAllString(text, -1)

	// Deduplicate
	seen := make(map[string]bool, len(matches))
	unique := make([]string, 0, len(matches))

	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			unique = append(unique, match)
		}
	}

	return unique
}

// extractProjectFromReference extracts project prefix from JIRA reference.
func extractProjectFromReference(reference string) string {
	parts := strings.Split(reference, "-")
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// isValidProject checks if project is in valid prefixes list.
func isValidProject(project string, validPrefixes []string) bool {
	if len(validPrefixes) == 0 {
		return true // No restriction if no prefixes configured
	}

	return slices.Contains(validPrefixes, project)
}

// filterIgnoredPatterns filters out ignored JIRA patterns.
func filterIgnoredPatterns(references []string, ignorePatterns []*regexp.Regexp) []string {
	if len(ignorePatterns) == 0 {
		return references
	}

	var filtered []string

	for _, ref := range references {
		ignored := false

		for _, pattern := range ignorePatterns {
			if pattern.MatchString(ref) {
				ignored = true

				break
			}
		}

		if !ignored {
			filtered = append(filtered, ref)
		}
	}

	return filtered
}

// validateSubjectJira validates JIRA presence in subject line.
func validateSubjectJira(subject string, subjectContext jiraSubjectContext) []domain.ValidationError {
	if subject == "" {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonEmptySubject,
				"Subject line is empty",
			),
		}
	}

	if subjectContext.parsed.IsValid {
		if errors := validateJiraScope(subjectContext.parsed.RawScope); len(errors) > 0 {
			return errors
		}
	}

	jiraRefs := extractJiraReferences(subject)

	// No JIRA found in subject
	if len(jiraRefs) == 0 {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonMissingJiraKeySubject,
				"JIRA ticket reference missing from subject line",
			).WithContextMap(diagnostics.Context{
				diagnostics.ContextSubject: subject,
			}),
		}
	}

	// Validate placement for conventional commits. Project prefixes are
	// checked once for the whole message by validateGeneralJiraPresence.
	if subjectContext.parsed.IsValid {
		return validateConventionalPlacement(subject, subjectContext.parsed, jiraRefs)
	}

	return nil
}

func validateJiraScope(scope string) []domain.ValidationError {
	if scope == "" || !jiraLikePrefixRegex.MatchString(scope) || validJiraFormatRegex.MatchString(scope) {
		return nil
	}

	return []domain.ValidationError{
		domain.New(
			rulecatalog.JiraReference,
			diagnostics.ReasonInvalidJiraFormat,
			"Invalid JIRA format in scope: "+scope,
		).WithContextMap(diagnostics.Context{
			diagnostics.ContextScope: scope,
		}),
	}
}

// validateBodyJira validates JIRA presence in body.
func validateBodyJira(body string, _ settings.JiraConfig) []domain.ValidationError {
	if body == "" {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonMissingJiraKeyBody,
				"JIRA ticket reference required in commit body",
			),
		}
	}

	var (
		formatErrors   []domain.ValidationError
		orderingErrors []domain.ValidationError
	)

	for i, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Refs") {
			formatErrors = append(formatErrors, validateRefsLineFormat(trimmed, i)...)
		}
	}

	orderingErrors = validateRefsOrdering(body)

	// If there are ordering errors, return them first (they're more important)
	if len(orderingErrors) > 0 {
		return orderingErrors
	}

	// If there were format errors, return them
	if len(formatErrors) > 0 {
		return formatErrors
	}

	// Check for valid JIRA references
	jiraRefs := extractJiraReferences(body)
	if len(jiraRefs) == 0 {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonMissingJiraKeyBody,
				"No JIRA ticket found in commit body",
			),
		}
	}

	return nil
}

// validateConventionalPlacement validates JIRA placement in conventional commits.
func validateConventionalPlacement(subject string, parsed domain.ConventionalCommitFormat, jiraRefs []string) []domain.ValidationError {
	if len(jiraRefs) == 0 {
		return nil
	}

	anyAtEnd := false
	allInScope := true

	for _, ref := range jiraRefs {
		if isJiraAtEnd(subject, ref) {
			anyAtEnd = true
		}

		if !isJiraInValidConventionalPosition(parsed, ref) {
			allInScope = false
		}
	}

	// Valid if any JIRA is at the end or all are in scope
	if !anyAtEnd && !allInScope {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonJiraKeyNotAtEnd,
				"JIRA ticket must be at the end of the subject or in the scope",
			).WithContextMap(diagnostics.Context{
				diagnostics.ContextSubject: subject,
			}),
		}
	}

	return nil
}

// isJiraAtEnd checks if JIRA reference is at the end of subject.
func isJiraAtEnd(subject string, jiraRef string) bool {
	return strings.HasSuffix(subject, jiraRef) ||
		strings.HasSuffix(subject, jiraRef+"]") ||
		strings.HasSuffix(subject, jiraRef+")")
}

// isJiraInValidConventionalPosition checks if JIRA is in conventional scope.
func isJiraInValidConventionalPosition(parsed domain.ConventionalCommitFormat, jiraRef string) bool {
	if parsed.IsValid && parsed.RawScope != "" {
		return strings.Contains(parsed.RawScope, jiraRef)
	}

	return false
}

// validateProjectPrefixes validates JIRA project prefixes.
func validateProjectPrefixes(jiraRefs []string, validPrefixes []string) []domain.ValidationError {
	var errors []domain.ValidationError

	for _, ref := range jiraRefs {
		project := extractProjectFromReference(ref)
		if !isValidProject(project, validPrefixes) {
			errors = append(errors, domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonInvalidProject,
				"Invalid JIRA project prefix: "+project,
			).WithActualExpected(project, strings.Join(validPrefixes, ", ")))
		}
	}

	return errors
}

// validateRefsLineFormat validates the format of Refs lines.
func validateRefsLineFormat(line string, lineNum int) []domain.ValidationError {
	if !strings.HasPrefix(line, "Refs") {
		return nil
	}

	// "Refs PROJ-123" (missing colon after Refs)
	if strings.HasPrefix(line, "Refs ") && refsWithSpaceRegex.MatchString(line) {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonInvalidRefsFormat,
				fmt.Sprintf("Invalid Refs line format at line %d (missing colon)", lineNum+1),
			).WithContextMap(diagnostics.Context{
				diagnostics.ContextLine: line,
			}),
		}
	}

	// Not "Refs:" prefix — nothing more to check
	if !strings.HasPrefix(line, "Refs:") {
		return nil
	}

	// "Refs:" with valid format — pass
	if refsLineRegex.MatchString(line) {
		return nil
	}

	// "Refs: proj-123" (lowercase key)
	if refsLowercaseRegex.MatchString(line) {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonInvalidKeyFormat,
				fmt.Sprintf("Invalid JIRA key format at line %d (use uppercase)", lineNum+1),
			).WithContextMap(diagnostics.Context{
				"line": line,
			}),
		}
	}

	// General format error
	return []domain.ValidationError{
		domain.New(
			rulecatalog.JiraReference,
			diagnostics.ReasonInvalidRefsFormat,
			fmt.Sprintf("Invalid Refs line format at line %d", lineNum+1),
		).WithContextMap(diagnostics.Context{
			"line": line,
		}),
	}
}

// validateRefsOrdering validates that Refs lines come before sign-offs.
func validateRefsOrdering(body string) []domain.ValidationError {
	if analyzeTrailers(body).refsAfterFirstSignOff() {
		return []domain.ValidationError{
			domain.New(
				rulecatalog.JiraReference,
				diagnostics.ReasonRefsAfterSignoff,
				"Refs lines must appear before sign-offs",
			),
		}
	}

	return nil
}
