//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"context"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	"codefloe.com/itiquette/gommitlint/internal/adapters/spell"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	settings "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

func TestRuleFiltering_LimitsValidationToSelectedRules(t *testing.T) {
	cfg := ComposeConfigs(
		func(c settings.Config) settings.Config {
			c.Rules.Enabled = []string{rulecatalog.Conventional, rulecatalog.Subject, rulecatalog.CryptoSignature, rulecatalog.SignOff}
			c.Rules.Disabled = []string{rulecatalog.Identity, rulecatalog.JiraReference, rulecatalog.CommitBody, rulecatalog.Spell}
			c.Conventional.Types = []string{"feat", "fix"}
			c.Message.Subject.MaxLength = new(50)
			return c
		},
	)

	testCases := []struct {
		name            string
		commitMessage   string
		filterRules     []string
		expectValid     bool
		expectErrorFrom string
		description     string
	}{
		{
			name:          "no_filter_runs_all_rules",
			commitMessage: "this is not a conventional commit and subject is way too long for the 50 char limit",
			filterRules:   nil,
			expectValid:   false,
			description:   "should fail both ConventionalCommit and Subject rules",
		},
		{
			name:          "filter_single_rule_still_fails_when_that_rule_fails",
			commitMessage: "this is not a conventional commit and subject is way too long for the 50 char limit",
			filterRules:   []string{rulecatalog.Subject},
			expectValid:   false,
			description:   "should only fail Subject rule, ignore ConventionalCommit",
		},
		{
			name:          "filter_multiple_rules",
			commitMessage: "feat: short",
			filterRules:   []string{rulecatalog.Conventional, rulecatalog.Subject},
			expectValid:   true,
			description:   "should pass both filtered rules",
		},
		{
			name: "filter_excludes_unrelated_rules",
			// Lowercase first letter: Subject rule's default case policy
			// requires lowercase, so the message must pass Subject for the
			// "filter excludes ConventionalCommit, Subject passes" intent
			// to be exercised.
			commitMessage: "add short subject",
			filterRules:   []string{rulecatalog.Subject},
			expectValid:   true,
			description:   "should only check Subject, ignore ConventionalCommit",
		},
		{
			name:          "repository_rule_filtering",
			commitMessage: "feat: test commit",
			filterRules:   []string{rulecatalog.BranchAhead},
			expectValid:   true,
			description:   "should only run repository rule BranchAhead",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoPath := gitrepo.QuickRepoPath(t, tc.commitMessage)
			factory := gitrepo.InMemoryFactory()
			gitRepo, err := factory.CreateRepository(repoPath)
			check.NoError(t, err)

			ctx := context.Background()
			commit, err := gitRepo.GetCommit(ctx, "HEAD")
			check.NoError(t, err)

			spellChecker, err := spell.NewMisspellAdapter(settings.DerefString(cfg.Spell.Locale, settings.DefaultSpellLocale))
			check.NoError(t, err)

			mockLog := &mockLogger{}
			ruleFactory := rulefactory.NewFactory(cfg, spellChecker, mockLog, "")

			var commitRules []domain.CommitRule

			if tc.filterRules == nil {
				commitRules = ruleFactory.CreateCommitRules()
			} else {
				commitRules = ruleFactory.CreateCommitRulesFiltered(tc.filterRules)
			}

			result := domain.ValidateCommit(commit, commitRules, cfg, mockLog)

			if tc.expectValid {
				check.Empty(t, result.Errors, tc.description)
			} else {
				check.NotEmpty(t, result.Errors, tc.description)
			}
		})
	}
}

func TestRepositoryRuleFiltering_LimitsRuleSetToSelection(t *testing.T) {
	cfg := ComposeConfigs(
		func(c settings.Config) settings.Config {
			c.Rules.Enabled = []string{rulecatalog.BranchAhead}
			c.Rules.Disabled = []string{}
			return c
		},
	)

	testCases := []struct {
		name        string
		filterRules []string
		expectRules int
		description string
	}{
		{
			name:        "no_filter_returns_all_repository_rules",
			filterRules: nil,
			expectRules: 1,
			description: "should return BranchAhead rule",
		},
		{
			name:        "filter_commit_rules",
			filterRules: []string{rulecatalog.Conventional},

			expectRules: 0,
			description: "should return no rules when filter excludes all",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spellChecker, err := spell.NewMisspellAdapter(settings.DerefString(cfg.Spell.Locale, settings.DefaultSpellLocale))
			check.NoError(t, err)

			mockLog := &mockLogger{}

			ruleFactory := rulefactory.NewFactory(cfg, spellChecker, mockLog, "")

			var repoRules []domain.RepositoryRule

			if tc.filterRules == nil {
				repoRules = ruleFactory.CreateRepositoryRules()
			} else {
				repoRules = ruleFactory.CreateRepositoryRulesFiltered(tc.filterRules)
			}

			check.Len(t, repoRules, tc.expectRules, tc.description)
		})
	}
}
