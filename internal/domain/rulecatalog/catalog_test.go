// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rulecatalog_test

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestNormalize_ForgivesPresentationNotIdentity covers the one function every
// rule lookup in the codebase funnels through: ByID, IsValid and
// IsRepositoryRule all normalize before comparing, and settings and diagnostics
// reach it through their own wrappers. Nothing in this package had a test.
//
// The rule it implements is that presentation is forgiven and identity is not.
// Surrounding whitespace, wrapping quotes and letter case are all ways of
// writing the same rule; a different name is a different rule.
func TestNormalize_ForgivesPresentationNotIdentity(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "canonical id is unchanged", input: rulecatalog.Subject, want: rulecatalog.Subject},
		{name: "surrounding whitespace is trimmed", input: "  subject  ", want: rulecatalog.Subject},
		{name: "single quotes are stripped", input: "'subject'", want: rulecatalog.Subject},
		{name: "double quotes are stripped", input: `"subject"`, want: rulecatalog.Subject},
		{name: "upper case is folded", input: "SUBJECT", want: rulecatalog.Subject},
		{name: "mixed case is folded", input: "SuBjEcT", want: rulecatalog.Subject},
		{name: "the three are applied together", input: `  "JiraReference"  `, want: rulecatalog.JiraReference},

		// Identity is preserved: folding must not erase the difference
		// between one rule and another, or between a rule and a near miss.
		{name: "inner whitespace is not removed", input: "commit body", want: "commit body"},
		{name: "an unknown name is passed through", input: "nosuchrule", want: "nosuchrule"},
		{name: "the empty string stays empty", input: "   ", want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := rulecatalog.Normalize(testCase.input)
			check.Equal(t, testCase.want, got)

			// Normalization is the identity on its own output. Lookups
			// normalize their input and compare against stored IDs that are
			// assumed already canonical, so a second pass that changed the
			// answer would make matching depend on how many times a name had
			// been through the function.
			check.Equal(t, got, rulecatalog.Normalize(got), "Normalize must be idempotent")
		})
	}
}

// TestByID_AcceptsEveryCatalogIDHoweverItIsWritten runs the whole catalog
// rather than a sample, so a rule added with a non-canonical ID cannot slip in
// unreachable. The claim is joined to Normalize deliberately: an ID that is not
// already in normalized form can never be found by any caller, because every
// lookup normalizes what it is given before comparing.
func TestByID_AcceptsEveryCatalogIDHoweverItIsWritten(t *testing.T) {
	t.Parallel()

	rules := rulecatalog.Rules()
	check.NotEmpty(t, rules, "an empty catalog would make every row below vacuous")

	for _, rule := range rules {
		t.Run(rule.ID, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, rule.ID, rulecatalog.Normalize(rule.ID),
				"a catalog ID that is not already normalized is unreachable")

			for _, spelling := range []string{
				rule.ID,
				strings.ToUpper(rule.ID),
				"  " + rule.ID + "  ",
				`"` + strings.ToUpper(rule.ID) + `"`,
			} {
				found, ok := rulecatalog.ByID(spelling)
				check.True(t, ok, "ByID(%q) must resolve", spelling)
				check.Equal(t, rule.ID, found.ID)
				check.True(t, rulecatalog.IsValid(spelling), "IsValid(%q) must agree with ByID", spelling)
			}
		})
	}
}

// TestIsValid_RejectsNamesThatAreNotRules is the negative control for the test
// above: without it, an IsValid that simply returned true would satisfy every
// row there.
func TestIsValid_RejectsNamesThatAreNotRules(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"",
		"   ",
		"nosuchrule",
		"subjectt",
		"sub",
		// Display names are not aliases. Folding case does not change that,
		// because the space survives normalization.
		"Commit body",
		"JIRA reference",
		"Branch Ahead",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			check.False(t, rulecatalog.IsValid(name))

			_, ok := rulecatalog.ByID(name)
			check.False(t, ok)
		})
	}
}

// TestIsRepositoryRule_PartitionsTheCatalogByKind pins that the predicate
// agrees with the catalog's own Kind for every rule, in both directions, rather
// than for the single repository rule that happens to exist today.
func TestIsRepositoryRule_PartitionsTheCatalogByKind(t *testing.T) {
	t.Parallel()

	var repositoryRules int

	for _, rule := range rulecatalog.Rules() {
		isRepository := rulecatalog.IsRepositoryRule(rule.ID)

		check.Equal(t, rule.Kind == rulecatalog.KindRepository, isRepository,
			"IsRepositoryRule(%q) must agree with its catalog Kind %q", rule.ID, rule.Kind)

		if isRepository {
			repositoryRules++
		}
	}

	check.NotZero(t, repositoryRules,
		"no repository rules at all would make the agreement above one-sided")
	check.False(t, rulecatalog.IsRepositoryRule("nosuchrule"),
		"a name that is not a rule is not a repository rule")
}
