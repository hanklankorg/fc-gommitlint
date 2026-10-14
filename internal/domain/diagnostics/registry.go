// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package diagnostics defines stable validation diagnostic identifiers.
package diagnostics

import (
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
)

// ID is the stable public GLxxx diagnostic identifier.
type ID string

// Reason is the precise internal validation reason behind a diagnostic.
type Reason string

// Category is the diagnostic taxonomy path, such as message.subject.
type Category string

// CheckFixupCommit is the dispatcher check for rejected fixup/squash commits.
const CheckFixupCommit ID = "GL901"

// SourceKind identifies the diagnostic producer kind.
type SourceKind string

const (
	// SourceRule diagnostics are emitted by configurable validation rules.
	SourceRule SourceKind = "rule"
	// SourceDispatcher diagnostics are emitted by validation orchestration.
	SourceDispatcher SourceKind = "dispatcher"
)

// Dispatcher diagnostic source IDs.
const (
	SourceIDFixup      = "fixup"
	SourceIDValidation = "validation"
)

// Diagnostic describes one concrete validation failure occurrence.
type Diagnostic struct {
	ID       ID
	Reason   Reason
	Category Category
	Summary  string
}

// IsZero reports whether no public diagnostic metadata is attached.
func (d Diagnostic) IsZero() bool {
	return d.ID == "" && d.Reason == "" && d.Category == "" && d.Summary == ""
}

// Definition describes one public diagnostic check.
type Definition struct {
	ID         ID
	SourceKind SourceKind
	RuleID     string
	SourceID   string
	Reasons    []Reason
	Category   Category
	Summary    string
}

// Diagnostic returns the concrete diagnostic for this definition and reason.
func (d Definition) Diagnostic(reason Reason) Diagnostic {
	return Diagnostic{
		ID:       d.ID,
		Reason:   reason,
		Category: d.Category,
		Summary:  d.Summary,
	}
}

// CategoryRange reserves an ID interval for related checks.
type CategoryRange struct {
	Name  string
	Start int
	End   int
}

// Definitions returns all public diagnostics in stable order.
func Definitions() []Definition {
	return []Definition{
		ruleDefinition("GL101", rulecatalog.Subject, "message.subject", "Subject must be present", "empty_subject", "missing_subject", "empty_message"),
		ruleDefinition("GL102", rulecatalog.Subject, "message.subject", "Subject must fit the configured length", "subject_too_long"),
		ruleDefinition("GL103", rulecatalog.Subject, "message.subject", "Subject text must be parseable", "invalid_format", "invalid_conventional_format", "missing_conventional_subject"),
		ruleDefinition("GL104", rulecatalog.Subject, "message.subject", "Subject must use the configured case", "invalid_case", "wrong_case_upper", "wrong_case_lower", "wrong_case_sentence"),
		ruleDefinition("GL105", rulecatalog.Subject, "message.subject", "Subject must be valid UTF-8", "invalid_utf8"),
		ruleDefinition("GL106", rulecatalog.Subject, "message.subject", "Subject must not end with forbidden punctuation", "invalid_suffix"),
		ruleDefinition("GL107", rulecatalog.Subject, "message.subject", "Subject must use imperative mood", "no_first_word", "non_imperative", "non_verb", "past_tense", "gerund", "third_person"),

		ruleDefinition("GL201", rulecatalog.Conventional, "message.conventional", "Commit subject must follow conventional format", "invalid_format", "invalid_conventional_format"),
		ruleDefinition("GL202", rulecatalog.Conventional, "message.conventional", "Conventional type must be allowed", "invalid_conventional_type"),
		ruleDefinition("GL203", rulecatalog.Conventional, "message.conventional", "Conventional description must fit the configured length", "description_too_long", "conventional_desc_too_long"),
		ruleDefinition("GL204", rulecatalog.Conventional, "message.conventional", "Conventional separator spacing must be exact", "invalid_spacing"),
		ruleDefinition("GL205", rulecatalog.Conventional, "message.conventional", "Conventional description must be present", "empty_conventional_desc", "missing_conventional_subject"),
		ruleDefinition("GL206", rulecatalog.Conventional, "message.conventional", "Conventional scope is required", "missing_conventional_scope"),
		ruleDefinition("GL207", rulecatalog.Conventional, "message.conventional", "Conventional scope must be allowed", "invalid_conventional_scope"),
		ruleDefinition("GL208", rulecatalog.Conventional, "message.conventional", "Conventional multi-scope syntax must be allowed and valid", "multi_scope_not_allowed", "invalid_multi_scope"),
		ruleDefinition("GL209", rulecatalog.Conventional, "message.conventional", "Breaking change marker must be allowed", "breaking_change_not_allowed"),

		ruleDefinition("GL301", rulecatalog.CommitBody, "message.body", "Body structure must include the required blank-line separation", "invalid_structure"),
		ruleDefinition("GL302", rulecatalog.CommitBody, "message.body", "Body must be separated from subject by a blank line", "missing_blank_line"),
		ruleDefinition("GL303", rulecatalog.CommitBody, "message.body", "Required body must be present", "missing_body"),
		ruleDefinition("GL304", rulecatalog.CommitBody, "message.body", "Body must meet the configured minimum length", "body_too_short"),
		ruleDefinition("GL305", rulecatalog.CommitBody, "message.body", "Body lines must fit the configured width", "body_line_too_long"),
		ruleDefinition("GL306", rulecatalog.CommitBody, "message.body", "Body must contain descriptive content", "invalid_body"),
		ruleDefinition("GL307", rulecatalog.CommitBody, "message.body", "Body trailers must be placed at the end", "misplaced_signoff"),

		ruleDefinition("GL401", rulecatalog.JiraReference, "message.references", "Commit message must contain a JIRA reference", "missing_jira"),
		ruleDefinition("GL402", rulecatalog.JiraReference, "message.references", "Subject must contain a JIRA reference", "missing_jira_key_subject", "empty_subject"),
		ruleDefinition("GL403", rulecatalog.JiraReference, "message.references", "Body must contain a JIRA reference", "missing_jira_key_body"),
		ruleDefinition("GL404", rulecatalog.JiraReference, "message.references", "JIRA key format must be valid", "invalid_jira_format"),
		ruleDefinition("GL405", rulecatalog.JiraReference, "message.references", "JIRA key must be in an allowed subject position", "jira_key_not_at_end"),
		ruleDefinition("GL406", rulecatalog.JiraReference, "message.references", "JIRA project prefix must be allowed", "invalid_project"),
		ruleDefinition("GL407", rulecatalog.JiraReference, "message.references", "Refs trailer format must be valid", "invalid_refs_format"),
		ruleDefinition("GL408", rulecatalog.JiraReference, "message.references", "Refs trailer key must use canonical JIRA case", "invalid_key_format"),
		ruleDefinition("GL409", rulecatalog.JiraReference, "message.references", "Refs trailers must appear before sign-offs", "refs_after_signoff"),
		ruleDefinition("GL410", rulecatalog.JiraReference, "message.references", "JIRA reference configuration must be valid", "invalid_config"),
		ruleDefinition("GL421", rulecatalog.SignOff, "message.trailers", "Required sign-off must be present", "missing_signoff"),
		ruleDefinition("GL422", rulecatalog.SignOff, "message.trailers", "Sign-off trailer format must be valid", "invalid_signoff_format"),
		ruleDefinition("GL423", rulecatalog.SignOff, "message.trailers", "Sign-off trailers must be placed at the end", "misplaced_signoff"),
		ruleDefinition("GL424", rulecatalog.SignOff, "message.trailers", "Required sign-offs must come from enough distinct signers", "insufficient_signoffs"),

		ruleDefinition("GL501", rulecatalog.CryptoSignature, "identity.signature", "Required cryptographic signature must be present", "missing_signature"),
		ruleDefinition("GL502", rulecatalog.CryptoSignature, "identity.signature", "Signature type must be recognized", "unknown_signature_format"),
		ruleDefinition("GL503", rulecatalog.CryptoSignature, "identity.signature", "Signature armor must be complete", "invalid_signature_format", "incomplete_gpg_signature", "incomplete_ssh_signature"),
		ruleDefinition("GL504", rulecatalog.CryptoSignature, "identity.signature", "Signature type policy must be satisfied", "invalid_config", "signature_type_not_allowed", "disallowed_signature_type"),
		ruleDefinition("GL505", rulecatalog.CryptoSignature, "identity.signature", "Signing key directory must be usable", "no_key_dir", "invalid_key_dir", "missing_key_directory", "invalid_key_directory"),
		ruleDefinition("GL506", rulecatalog.CryptoSignature, "identity.signature", "Signing keys must load successfully", "key_load_failed"),
		ruleDefinition("GL507", rulecatalog.CryptoSignature, "identity.signature", "Trusted signing keys must be available", "no_trusted_keys", "key_not_trusted"),
		ruleDefinition("GL508", rulecatalog.CryptoSignature, "identity.signature", "Signing key must satisfy trust policy", "weak_key", "verification_failed", "invalid_signature"),
		ruleDefinition("GL520", rulecatalog.Identity, "identity.author", "Author identity must be allowed", "identity_not_allowed"),
		ruleDefinition("GL521", rulecatalog.Identity, "identity.author", "Identity configuration must be valid", "invalid_config"),

		ruleDefinition("GL601", rulecatalog.Spell, "content.spelling", "Commit message words must be spelled correctly", "spelling_error", "misspelled_word"),
		ruleDefinition("GL602", rulecatalog.Spell, "content.spelling", "Spell checker must complete successfully", "spell_check_failed"),

		ruleDefinition("GL701", rulecatalog.BranchAhead, "repository.branch", "Branch must not be too far ahead of the reference", "too_many_commits"),
		ruleDefinition("GL702", rulecatalog.BranchAhead, "repository.branch", "Reference branch must exist", "missing_reference", "invalid_reference"),
		ruleDefinition("GL703", rulecatalog.BranchAhead, "repository.branch", "Repository must be accessible", "invalid_repo"),
		ruleDefinition("GL704", rulecatalog.BranchAhead, "repository.branch", "Git branch comparison must complete", "git_operation_failed"),
		ruleDefinition("GL705", rulecatalog.BranchAhead, "repository.branch", "Branch-ahead configuration must be valid", "invalid_config"),
		ruleDefinition("GL710", rulecatalog.LinearHistory, "repository.history", "Merge commits must not be present", "merge_commit_found"),

		dispatcherDefinition(CheckFixupCommit, SourceIDFixup, "dispatcher.fixup", "Fixup and squash commits must be squashed before validation", "fixup_commit_found"),
		dispatcherDefinition("GL902", SourceIDValidation, "dispatcher.internal", "Validation must complete without internal failure", "validation_failed"),
		dispatcherDefinition("GL903", SourceIDValidation, "dispatcher.input", "Commit input must be valid", "invalid_commit", "invalid_reference"),
		dispatcherDefinition("GL904", SourceIDValidation, "dispatcher.input", "Configuration input must exist", "config_not_found"),
		dispatcherDefinition("GL905", SourceIDValidation, "dispatcher.repository", "Repository rule requires repository context", "repo_rule_no_repo"),
		dispatcherDefinition("GL906", SourceIDValidation, "dispatcher.cancel", "Validation must not be cancelled", "operation_cancelled", "context_cancelled"),
		dispatcherDefinition("GL907", SourceIDValidation, "dispatcher.input", "Requested commit must exist", "commit_not_found"),
		dispatcherDefinition("GL908", SourceIDValidation, "dispatcher.input", "Requested commit range must exist", "range_not_found"),
		dispatcherDefinition("GL909", SourceIDValidation, "dispatcher.input", "Commit range must be valid", "invalid_commit_range"),
		dispatcherDefinition("GL999", SourceIDValidation, "dispatcher.unknown", "Unknown validation failure", "unknown_error"),
	}
}

// CategoryRanges returns the reserved diagnostic-ID ranges.
func CategoryRanges() []CategoryRange {
	return []CategoryRange{
		{Name: "subject", Start: 100, End: 199},
		{Name: "conventional", Start: 200, End: 299},
		{Name: "body", Start: 300, End: 399},
		{Name: "trailers/references", Start: 400, End: 499},
		{Name: "signatures/identity", Start: 500, End: 599},
		{Name: "spelling/content", Start: 600, End: 699},
		{Name: "repository/history", Start: 700, End: 799},
		{Name: "dispatcher", Start: 900, End: 999},
	}
}

// NormalizeID standardizes a diagnostic ID for comparisons.
func NormalizeID(id string) string {
	cleaned := strings.TrimSpace(id)
	cleaned = strings.Trim(cleaned, "\"'")

	return strings.ToUpper(strings.TrimSpace(cleaned))
}

// IDs returns all diagnostic IDs in stable order.
func IDs() []string {
	definitions := Definitions()
	ids := make([]string, 0, len(definitions))

	for _, definition := range definitions {
		ids = append(ids, string(definition.ID))
	}

	return ids
}

// ByID returns metadata for a diagnostic ID.
func ByID(id string) (Definition, bool) {
	normalized := NormalizeID(id)
	for _, definition := range Definitions() {
		if string(definition.ID) == normalized {
			return definition, true
		}
	}

	return Definition{}, false
}

// ByRuleAndReason returns metadata for the diagnostic that emits a rule/reason pair.
func ByRuleAndReason(ruleName string, reason Reason) (Definition, bool) {
	reason = Reason(strings.TrimSpace(string(reason)))
	normalizedRule := rulecatalog.Normalize(ruleName)

	for _, definition := range Definitions() {
		if definition.SourceKind != SourceRule || definition.RuleID != normalizedRule {
			continue
		}

		for _, candidate := range definition.Reasons {
			if strings.EqualFold(string(candidate), string(reason)) {
				return definition, true
			}
		}
	}

	return Definition{}, false
}

// BySourceAndReason returns metadata for a dispatcher diagnostic source/reason pair.
func BySourceAndReason(sourceID string, reason Reason) (Definition, bool) {
	reason = Reason(strings.TrimSpace(string(reason)))
	sourceID = strings.TrimSpace(sourceID)

	for _, definition := range Definitions() {
		if definition.SourceKind != SourceDispatcher || definition.SourceID != sourceID {
			continue
		}

		for _, candidate := range definition.Reasons {
			if strings.EqualFold(string(candidate), string(reason)) {
				return definition, true
			}
		}
	}

	return Definition{}, false
}

// ForRule returns all diagnostics emitted by a runtime rule name.
func ForRule(ruleName string) []Definition {
	definitions := make([]Definition, 0)
	normalizedRule := rulecatalog.Normalize(ruleName)

	for _, definition := range Definitions() {
		if definition.SourceKind == SourceRule && definition.RuleID == normalizedRule {
			definitions = append(definitions, definition)
		}
	}

	return definitions
}

// RangeForID returns the reserved range containing a diagnostic ID.
func RangeForID(id string) (CategoryRange, bool) {
	number, ok := numericID(id)
	if !ok {
		return CategoryRange{}, false
	}

	for _, categoryRange := range CategoryRanges() {
		if number >= categoryRange.Start && number <= categoryRange.End {
			return categoryRange, true
		}
	}

	return CategoryRange{}, false
}

func ruleDefinition(id ID, ruleID string, category Category, summary string, reasons ...Reason) Definition {
	return Definition{
		ID:         id,
		SourceKind: SourceRule,
		RuleID:     ruleID,
		Reasons:    reasons,
		Category:   category,
		Summary:    summary,
	}
}

func dispatcherDefinition(id ID, sourceID string, category Category, summary string, reasons ...Reason) Definition {
	return Definition{
		ID:         id,
		SourceKind: SourceDispatcher,
		SourceID:   sourceID,
		Reasons:    reasons,
		Category:   category,
		Summary:    summary,
	}
}

func numericID(id string) (int, bool) {
	normalized := NormalizeID(id)
	if len(normalized) != len("GL000") || !strings.HasPrefix(normalized, "GL") {
		return 0, false
	}

	value, err := strconv.Atoi(normalized[2:])
	if err != nil {
		return 0, false
	}

	return value, true
}
