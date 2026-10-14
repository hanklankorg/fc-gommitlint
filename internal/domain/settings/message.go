// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

import (
	"slices"
	"strings"
)

// Subject case values.
//
// The set spans three distinct kinds of setting, which is why it reads as
// larger than it is:
//
//   - CaseLower, CaseUpper, CaseSentence enforce that specific case.
//   - CaseAuto is a resolution policy, not a case: it defers the choice to
//     the commit style (see rules.SubjectRule).
//   - CaseAny switches the check off.
const (
	CaseAuto     = "auto"
	CaseAny      = "any"
	CaseLower    = "lower"
	CaseUpper    = "upper"
	CaseSentence = "sentence"
)

// NormalizeMessageConfig standardizes message settings whose values are
// compared as enums downstream. Applied once by the config merger so rule
// construction can match exactly rather than each consumer re-normalizing —
// the same seam NormalizeRulesConfig and NormalizeChecksConfig use.
func NormalizeMessageConfig(config MessageConfig) MessageConfig {
	// MessageConfig is taken by value and SubjectConfig is a value field, so
	// repointing Case here cannot mutate the caller's pointee.
	if config.Subject.Case != nil {
		normalized := NormalizeSubjectCase(*config.Subject.Case)
		config.Subject.Case = &normalized
	}

	return config
}

// NormalizeSubjectCase standardizes a subject-case value for comparison.
// The user's "Upper" and our canonical "upper" should mean the same thing.
func NormalizeSubjectCase(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// GetValidSubjectCases returns the canonical subject-case values.
// Kept in the order users are most likely to want them in CLI hints.
func GetValidSubjectCases() []string {
	return []string{CaseAuto, CaseAny, CaseLower, CaseUpper, CaseSentence}
}

// IsValidSubjectCase reports whether the value is a recognised subject-case
// setting. Comparison is case-insensitive.
func IsValidSubjectCase(value string) bool {
	return slices.Contains(GetValidSubjectCases(), NormalizeSubjectCase(value))
}
