// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rulefactory

import (
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	domainrules "codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// FuzzRuleSetComposition verifies that filtering, duplicate removal, disabled
// vetoes, and concrete factory creation agree on the final commit-rule set.
func FuzzRuleSetComposition(f *testing.F) {
	f.Add(uint8(0), uint8(1), uint8(2), false)
	f.Add(uint8(4), uint8(4), uint8(4), true)
	// A filtered seed whose filtered rule is not the disabled one: with the
	// seed above, second == disabled makes the filtered expectation nil
	// whatever the filter does, so it cannot tell padded, upper-cased
	// composition apart from no composition at all.
	f.Add(uint8(0), uint8(1), uint8(2), true)

	f.Fuzz(func(t *testing.T, firstIndex, secondIndex, disabledIndex uint8, filtered bool) {
		catalog := make([]string, 0, len(rulecatalog.Rules()))
		for _, rule := range rulecatalog.Rules() {
			if rule.Kind == rulecatalog.KindCommit {
				catalog = append(catalog, rule.ID)
			}
		}

		first := catalog[int(firstIndex)%len(catalog)]
		second := catalog[int(secondIndex)%len(catalog)]
		disabled := catalog[int(disabledIndex)%len(catalog)]
		cfg := settings.Config{Rules: settings.RulesConfig{
			Enabled:  []string{first, second, first},
			Disabled: []string{disabled, disabled},
		}}

		// Two spellings of one rule, both padded and upper-cased: an
		// unpadded duplicate would match on its own and hide a filter that
		// stopped trimming.
		var filter []string
		if filtered {
			filter = []string{"  " + strings.ToUpper(second) + "  ", " " + strings.ToUpper(second) + " "}
		}

		want := EnabledCommitRuleNames(cfg, filter)
		factory := NewFactoryWithDependencies(cfg, Dependencies{
			CryptoKeyProvidersFactory: func(settings.Config, domain.Logger) domainrules.CryptoKeyProviders {
				return domainrules.CryptoKeyProviders{}
			},
		}, &mockLogger{}, "main")

		got := ruleNames(factory.CreateCommitRulesFiltered(filter))
		if !slices.Equal(got, want) {
			t.Fatalf("factory and rule resolution disagree: got=%#v want=%#v", got, want)
		}

		if slices.Contains(got, disabled) {
			t.Fatalf("disabled rule %q survived composition: %#v", disabled, got)
		}

		if filtered {
			wantFiltered := []string{second}
			if second == disabled {
				wantFiltered = nil
			}

			if !slices.Equal(got, wantFiltered) {
				t.Fatalf("filter composition changed: got=%#v want=%#v", got, wantFiltered)
			}
		}

		seen := map[string]struct{}{}
		for _, name := range got {
			if _, duplicate := seen[name]; duplicate {
				t.Fatalf("duplicate concrete rule %q in %#v", name, got)
			}

			seen[name] = struct{}{}
		}
	})
}
