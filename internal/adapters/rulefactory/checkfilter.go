// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rulefactory

import (
	"context"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

type checkFilteredCommitRule struct {
	rule         domain.CommitRule
	activeChecks []string
	activeSet    map[string]struct{}
}

func (r checkFilteredCommitRule) Name() string { return r.rule.Name() }

func (r checkFilteredCommitRule) Validate(commit domain.Commit) []domain.ValidationError {
	return filterValidationErrorsByCheck(r.rule.Validate(commit), r.activeSet)
}

func (r checkFilteredCommitRule) ActiveChecks() []string {
	result := make([]string, len(r.activeChecks))
	copy(result, r.activeChecks)

	return result
}

type checkFilteredRepositoryRule struct {
	rule         domain.RepositoryRule
	activeChecks []string
	activeSet    map[string]struct{}
}

func (r checkFilteredRepositoryRule) Name() string { return r.rule.Name() }

func (r checkFilteredRepositoryRule) Validate(ctx context.Context, repo domain.BranchAnalyzer) (string, []domain.ValidationError) {
	info, errs := r.rule.Validate(ctx, repo)

	return info, filterValidationErrorsByCheck(errs, r.activeSet)
}

func (r checkFilteredRepositoryRule) ActiveChecks() []string {
	result := make([]string, len(r.activeChecks))
	copy(result, r.activeChecks)

	return result
}

func appendCheckEnabledRuleNames(enabledRules []string, cfg settings.Config) []string {
	disabledRules := ruleNameSet(settings.NormalizeRulesConfig(cfg.Rules).Disabled)
	result := make([]string, 0, len(enabledRules)+len(cfg.Checks.Enabled))
	result = append(result, enabledRules...)
	seen := ruleNameSet(result)

	for _, ruleName := range settings.RuleNamesForEnabledChecks(cfg.Checks) {
		if _, disabled := disabledRules[ruleName]; disabled {
			continue
		}

		if _, exists := seen[ruleName]; exists {
			continue
		}

		seen[ruleName] = struct{}{}
		result = append(result, ruleName)
	}

	return result
}

func wrapCommitRuleWithChecks(rule domain.CommitRule, cfg settings.Config, enabledByRules bool) (domain.CommitRule, bool) {
	knownChecks := diagnostics.ForRule(rule.Name())
	if len(knownChecks) == 0 {
		return rule, true
	}

	activeChecks := activeCheckIDsForRule(rule.Name(), cfg, enabledByRules)
	if len(activeChecks) == 0 {
		return nil, false
	}

	return checkFilteredCommitRule{
		rule:         rule,
		activeChecks: activeChecks,
		activeSet:    stringSet(activeChecks),
	}, true
}

func wrapRepositoryRuleWithChecks(rule domain.RepositoryRule, cfg settings.Config, enabledByRules bool) (domain.RepositoryRule, bool) {
	knownChecks := diagnostics.ForRule(rule.Name())
	if len(knownChecks) == 0 {
		return rule, true
	}

	activeChecks := activeCheckIDsForRule(rule.Name(), cfg, enabledByRules)
	if len(activeChecks) == 0 {
		return nil, false
	}

	return checkFilteredRepositoryRule{
		rule:         rule,
		activeChecks: activeChecks,
		activeSet:    stringSet(activeChecks),
	}, true
}

func activeCheckIDsForRule(ruleName string, cfg settings.Config, includeRuleDefaults bool) []string {
	definitions := diagnostics.ForRule(ruleName)
	if len(definitions) == 0 {
		return nil
	}

	normalized := settings.NormalizeChecksConfig(cfg.Checks)
	active := make(map[string]struct{}, len(definitions)+len(normalized.Enabled))

	if includeRuleDefaults {
		for _, definition := range definitions {
			active[string(definition.ID)] = struct{}{}
		}
	}

	for _, id := range normalized.Enabled {
		definition, ok := diagnostics.ByID(id)
		if ok && definition.RuleID == ruleName {
			active[id] = struct{}{}
		}
	}

	for _, id := range normalized.Disabled {
		delete(active, id)
	}

	result := make([]string, 0, len(active))
	for _, definition := range definitions {
		if _, ok := active[string(definition.ID)]; ok {
			result = append(result, string(definition.ID))
		}
	}

	return result
}

func filterValidationErrorsByCheck(errs []domain.ValidationError, activeSet map[string]struct{}) []domain.ValidationError {
	if len(errs) == 0 {
		return errs
	}

	filtered := make([]domain.ValidationError, 0, len(errs))
	for _, err := range errs {
		if err.Diagnostic.ID == "" {
			filtered = append(filtered, err)

			continue
		}

		if _, ok := activeSet[string(err.Diagnostic.ID)]; ok {
			filtered = append(filtered, err)
		}
	}

	return filtered
}

func ruleNameSet(rules []string) map[string]struct{} {
	set := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		set[settings.NormalizeRuleName(rule)] = struct{}{}
	}

	return set
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}

	return set
}
