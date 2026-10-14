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

// TestValidateCommit_BothInMemoryRepoMethodsAgree runs the same validation
// through both in-memory repository constructions and requires identical
// outcomes, then checks the outcome against expectation.
func TestValidateCommit_BothInMemoryRepoMethodsAgree(t *testing.T) {
	cfg := ComposeConfigs(
		func(c settings.Config) settings.Config {
			c.Rules.Enabled = []string{rulecatalog.Conventional, rulecatalog.Subject}
			c.Rules.Disabled = []string{rulecatalog.CryptoSignature, rulecatalog.SignOff, rulecatalog.Identity, rulecatalog.JiraReference, rulecatalog.CommitBody, rulecatalog.Spell}
			c.Conventional.Types = []string{"feat", "fix", "docs", "test"}
			c.Message.Subject.MaxLength = new(72)
			return c
		},
	)

	tcs := []struct {
		name          string
		commitMessage string
		expectValid   bool
		description   string
	}{
		{
			name:          "valid conventional commit",
			commitMessage: "feat: implement user authentication system",
			expectValid:   true,
			description:   "should pass all validation rules",
		},
		{
			name:          "invalid - no conventional type",
			commitMessage: "implement user authentication system",
			expectValid:   false,
			description:   "should fail conventional commit validation",
		},
		{
			name:          "invalid - subject too long",
			commitMessage: "feat: implement a very very very very very very very very long feature description",
			expectValid:   false,
			description:   "should fail subject length validation",
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			inMemoryRepo1 := gitrepo.NewInMemoryRepo(t).WithCommit(testCase.commitMessage)
			inMemoryResult1 := validateWithRepository(t, inMemoryRepo1.Adapter, testCase.commitMessage, cfg)

			repoPath := gitrepo.QuickRepoPath(t, testCase.commitMessage)

			factory := gitrepo.InMemoryFactory()
			gitRepo, err := factory.CreateRepository(repoPath)
			check.NoError(t, err)

			inMemoryResult2 := validateWithRepository(t, gitRepo, testCase.commitMessage, cfg)

			check.Equal(t, inMemoryResult1.Valid, inMemoryResult2.Valid,
				"Both in-memory repo methods should produce identical validation results")
			check.Equal(t, len(inMemoryResult1.Errors), len(inMemoryResult2.Errors),
				"Both in-memory repo methods should produce same number of errors")

			check.Equal(t, testCase.expectValid, inMemoryResult1.Valid, testCase.description)
		})
	}
}

// validateWithRepository performs validation using a git repository adapter.
func validateWithRepository(t *testing.T, gitRepo domain.Repository, message string, cfg settings.Config) ValidationResult {
	t.Helper()

	ctx := context.Background()

	commit, err := gitRepo.GetCommit(ctx, "HEAD")
	check.NoError(t, err, "Should be able to get HEAD commit")

	spellChecker, err := spell.NewMisspellAdapter(settings.DerefString(cfg.Spell.Locale, settings.DefaultSpellLocale))
	check.NoError(t, err)

	mockLog := &mockLogger{}
	ruleFactory := rulefactory.NewFactory(cfg, spellChecker, mockLog, "")
	commitRules := ruleFactory.CreateCommitRules()

	result := domain.ValidateCommit(commit, commitRules, cfg, mockLog)

	return ValidationResult{
		Valid:  len(result.Errors) == 0,
		Errors: result.Errors,
	}
}
