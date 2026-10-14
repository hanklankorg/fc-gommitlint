// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestDisplayName_KnownRules(t *testing.T) { //nolint:tparallel // t.Setenv pins the locale for the whole table, so the top level stays serial.
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANG", "C")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "subject", input: rulecatalog.Subject, expected: testRuleSubjectName},
		{name: "conventional", input: rulecatalog.Conventional, expected: "Commit type"},
		{name: "commit_body", input: rulecatalog.CommitBody, expected: "Commit body"},
		{name: "jira", input: rulecatalog.JiraReference, expected: "JIRA reference"},
		{name: "sign_off", input: rulecatalog.SignOff, expected: "Sign-off"},
		{name: "signature", input: rulecatalog.CryptoSignature, expected: "Signature"},
		{name: "identity", input: rulecatalog.Identity, expected: "Identity"},
		{name: "spell", input: rulecatalog.Spell, expected: "Spelling"},
		{name: "branch_ahead", input: rulecatalog.BranchAhead, expected: "Branch Ahead"},
		{name: "linear_history", input: rulecatalog.LinearHistory, expected: "Linear History"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := DisplayName(testCase.input, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestDisplayName_UnknownRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "pascal_case", input: "CustomRuleName", expected: "Custom Rule Name"},
		{name: "single_word", input: "Custom", expected: "Custom"},
		{name: "empty", input: "", expected: ""},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := DisplayName(testCase.input, nil)
			check.Equal(t, testCase.expected, result)
		})
	}
}
