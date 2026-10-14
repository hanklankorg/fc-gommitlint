// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package rulecatalog defines the canonical rule identifiers and metadata.
package rulecatalog

import (
	"strings"
)

// Rule identifiers used in config, reports, diagnostics, and filtering.
const (
	Subject         = "subject"
	Conventional    = "conventional"
	SignOff         = "signoff"
	Spell           = "spell"
	CryptoSignature = "cryptosignature"
	JiraReference   = "jirareference"
	CommitBody      = "commitbody"
	Identity        = "identity"
	BranchAhead     = "branchahead"
	LinearHistory   = "linearhistory"
)

// Kind classifies where a rule runs.
type Kind string

const (
	// KindCommit validates individual commits.
	KindCommit Kind = "commit"
	// KindRepository validates repository state.
	KindRepository Kind = "repository"
)

// Rule describes stable metadata for one configurable rule.
type Rule struct {
	ID             string
	DefaultEnabled bool
	DisplayName    string
	Kind           Kind
}

// Rules returns supported rule metadata in stable order.
func Rules() []Rule {
	return []Rule{
		{ID: Subject, DisplayName: "Subject", Kind: KindCommit, DefaultEnabled: true},
		{ID: Conventional, DisplayName: "Commit type", Kind: KindCommit, DefaultEnabled: true},
		{ID: SignOff, DisplayName: "Sign-off", Kind: KindCommit, DefaultEnabled: true},
		{ID: Spell, DisplayName: "Spelling", Kind: KindCommit, DefaultEnabled: true},
		{ID: BranchAhead, DisplayName: "Branch Ahead", Kind: KindRepository, DefaultEnabled: true},
		{ID: CryptoSignature, DisplayName: "Signature", Kind: KindCommit, DefaultEnabled: true},
		{ID: JiraReference, DisplayName: "JIRA reference", Kind: KindCommit},
		{ID: CommitBody, DisplayName: "Commit body", Kind: KindCommit, DefaultEnabled: true},
		{ID: Identity, DisplayName: "Identity", Kind: KindCommit},
		{ID: LinearHistory, DisplayName: "Linear History", Kind: KindCommit},
	}
}

// Normalize standardizes a rule ID for comparisons.
//
// Rule IDs in the catalog are all lower case, so folding case here makes
// rules.enabled/rules.disabled accept the spelling a user actually reaches for
// ("JiraReference", whose catalog DisplayName is "JIRA reference") instead of
// rejecting it as an unknown rule. It matches what this function already does
// for surrounding whitespace and quotes, what the --rules filter already does
// for case, and what diagnostics.NormalizeID does in the opposite direction for
// the upper-case GL check IDs.
//
// Folding cannot change any configuration that works today: a name that matches
// now is already lower case, and ToLower leaves it untouched.
func Normalize(ruleID string) string {
	cleaned := strings.TrimSpace(ruleID)
	cleaned = strings.Trim(cleaned, "\"'")

	return strings.ToLower(cleaned)
}

// IDs returns supported rule IDs in stable catalog order.
func IDs() []string {
	rules := Rules()
	ids := make([]string, 0, len(rules))

	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}

	return ids
}

// ByID returns metadata for a rule ID.
func ByID(ruleID string) (Rule, bool) {
	normalized := Normalize(ruleID)
	for _, rule := range Rules() {
		if rule.ID == normalized {
			return rule, true
		}
	}

	return Rule{}, false
}

// IsValid reports whether a rule ID exists in the catalog.
func IsValid(ruleID string) bool {
	_, ok := ByID(ruleID)

	return ok
}

// IsRepositoryRule reports whether a rule ID is repository-scoped.
func IsRepositoryRule(ruleID string) bool {
	rule, ok := ByID(ruleID)

	return ok && rule.Kind == KindRepository
}

// DefaultEnabled returns default-enabled rule IDs in stable catalog order.
func DefaultEnabled() []string {
	return defaultRules(true, "")
}

// DefaultDisabled returns default-disabled rule IDs in stable catalog order.
func DefaultDisabled() []string {
	return defaultRules(false, "")
}

// DefaultEnabledByKind returns default-enabled rule IDs for one rule kind.
func DefaultEnabledByKind(kind Kind) []string {
	return defaultRules(true, kind)
}

func defaultRules(enabled bool, kind Kind) []string {
	rules := make([]string, 0, len(Rules()))

	for _, rule := range Rules() {
		if rule.DefaultEnabled != enabled {
			continue
		}

		if kind != "" && rule.Kind != kind {
			continue
		}

		rules = append(rules, rule.ID)
	}

	return rules
}
