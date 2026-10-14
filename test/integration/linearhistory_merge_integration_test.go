//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"context"
	"slices"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	"codefloe.com/itiquette/gommitlint/internal/adapters/spell"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

// TestLinearHistory_MergeCommitsSkippedByDefault verifies that merge commits
// skip other commit rules while linearhistory is default-disabled.
func TestLinearHistory_MergeCommitsSkippedByDefault(t *testing.T) {
	t.Parallel()

	// Default configuration: linearhistory is default-disabled, not explicitly enabled.
	cfg := configuration.NewConfigWithDefaults()

	check.NotContains(t, cfg.Rules.Enabled, rulecatalog.LinearHistory,
		"precondition: linearhistory must not be explicitly enabled")
	check.Contains(t, rulecatalog.DefaultDisabled(), rulecatalog.LinearHistory,
		"precondition: linearhistory must be default-disabled")

	repo, _, headHash := gitrepo.NewRepoWithMergeHistory(t)

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), 5)
	check.NoError(t, err)
	check.NotEmpty(t, commits, "expected commits in repository")

	mergeCount := countMergeCommits(commits)
	check.Greater(t, mergeCount, 0, "test setup must include at least one merge commit")
	t.Logf("scenario contains %d merge commits, head=%s", mergeCount, headHash)

	for _, c := range commits {
		if c.IsMergeCommit {
			requireMergeCommitSkipped(t, c, cfg, "should produce ZERO errors when linearhistory is disabled")
		}
	}
}

// TestLinearHistory_EnabledRuleEmitsMergeError verifies that when linearhistory
// IS enabled, the dispatcher invokes the rule (and other rules) on merges, and
// LinearHistory itself produces the expected merge-commit error.
func TestLinearHistory_EnabledRuleEmitsMergeError(t *testing.T) {
	t.Parallel()

	cfg := configuration.NewConfigWithDefaults()
	// Enable linearhistory so it overrides the default-disabled rule metadata.
	cfg.Rules.Enabled = append(cfg.Rules.Enabled, rulecatalog.LinearHistory)

	repo, _, _ := gitrepo.NewRepoWithMergeHistory(t)

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), 5)
	check.NoError(t, err)

	commitRules := buildCommitRules(t, cfg)

	requireRule(t, commitRules, rulecatalog.LinearHistory)

	sawMergeError := false
	for _, c := range commits {
		if !c.IsMergeCommit {
			continue
		}
		result := domain.ValidateCommit(c, commitRules, cfg, &mockLogger{})
		for _, e := range result.Errors {
			if e.Rule == rulecatalog.LinearHistory && e.Diagnostic.Reason == diagnostics.ReasonMergeCommitFound {
				sawMergeError = true
			}
		}
	}

	check.True(t, sawMergeError,
		"with linearhistory enabled, at least one merge commit must produce diagnostics.ReasonMergeCommitFound")
}

// TestLinearHistory_NonMergeCommitsAlwaysValidated verifies the dispatcher's
// merge-skip does NOT affect non-merge commits. Other rules still run on them.
func TestLinearHistory_NonMergeCommitsAlwaysValidated(t *testing.T) {
	t.Parallel()

	cfg := configuration.NewConfigWithDefaults()

	// A regular commit with a clearly invalid (non-conventional) subject.
	repo := gitrepo.NewRepoWithCommits(t, []gitrepo.CommitSpec{
		{Message: "Not a conventional commit subject at all"},
	})

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), 1)
	check.NoError(t, err)
	check.Len(t, commits, 1)
	check.False(t, commits[0].IsMergeCommit, "precondition: commit must not be a merge")

	commitRules := buildCommitRules(t, cfg)
	result := domain.ValidateCommit(commits[0], commitRules, cfg, &mockLogger{})

	check.NotEmpty(t, result.Errors,
		"non-merge commit with invalid subject should still produce errors")
}

// TestLinearHistory_MergeSkipAcrossRangeValidation verifies that range
// validation (the path used by CI for PR/MR validation) skips merges in the
// range when linearhistory is disabled — this is the exact CI flow the user
// reported as broken.
func TestLinearHistory_MergeSkipAcrossRangeValidation(t *testing.T) {
	t.Parallel()

	cfg := configuration.NewConfigWithDefaults()

	repo, fromHash, toHash := gitrepo.NewRepoWithMergeHistory(t)

	commits, err := repo.Adapter.GetCommitRange(context.Background(), fromHash, toHash)
	check.NoError(t, err)
	check.Greater(t, countMergeCommits(commits), 0, "the range must include the merge commit")

	for _, c := range commits {
		if c.IsMergeCommit {
			requireMergeCommitSkipped(t, c, cfg, "in PR/MR range should not be flagged when linearhistory is disabled")
		}
	}
}

// TestLinearHistory_MergeSkipWithMinimalPreset verifies the behavior under the
// 'minimal' preset (also has linearhistory disabled).
func TestLinearHistory_MergeSkipWithMinimalPreset(t *testing.T) {
	t.Parallel()

	cfg, err := configuration.ResolvePreset("minimal")
	check.NoError(t, err)
	check.Contains(t, cfg.Rules.Disabled, rulecatalog.LinearHistory)

	repo, _, _ := gitrepo.NewRepoWithMergeHistory(t)

	commits, err := repo.Adapter.GetHeadCommits(context.Background(), 5)
	check.NoError(t, err)
	check.Greater(t, countMergeCommits(commits), 0, "test setup must include at least one merge commit")

	for _, c := range commits {
		if c.IsMergeCommit {
			requireMergeCommitSkipped(t, c, cfg, "should be skipped under 'minimal' preset")
		}
	}
}

// --- helpers ---

// requireMergeCommitSkipped asserts the merge commit produces no errors under
// cfg and, as the control, that the same commit is rejected once the skip is
// off: activating linearhistory in the same configuration makes the dispatcher
// run the rules on it. Without the control an empty rule set, or a fixture the
// configured rules accept anyway, would satisfy the first assertion for the
// wrong reason.
func requireMergeCommitSkipped(t *testing.T, commit domain.Commit, cfg settings.Config, why string) {
	t.Helper()

	check.True(t, commit.IsMergeCommit, "precondition: %s must be a merge commit", commit.Hash)

	result := domain.ValidateCommit(commit, buildCommitRules(t, cfg), cfg, &mockLogger{})
	check.Empty(t, result.Errors, "merge commit %s %s, got: %v", commit.Hash, why, result.Errors)

	withSkipOff := cfg
	withSkipOff.Rules.Disabled = slices.DeleteFunc(slices.Clone(cfg.Rules.Disabled), func(rule string) bool {
		return rule == rulecatalog.LinearHistory
	})
	withSkipOff.Rules.Enabled = append(slices.Clone(cfg.Rules.Enabled), rulecatalog.LinearHistory)

	control := domain.ValidateCommit(commit, buildCommitRules(t, withSkipOff), withSkipOff, &mockLogger{})
	check.NotEmpty(t, control.Errors, "control: merge commit %s must fail once the skip is off", commit.Hash)
}

func buildCommitRules(t *testing.T, cfg settings.Config) []domain.CommitRule {
	t.Helper()

	checker, err := spell.NewMisspellAdapter(settings.DerefString(cfg.Spell.Locale, settings.DefaultSpellLocale))
	check.NoError(t, err)

	factory := rulefactory.NewFactory(cfg, checker, &mockLogger{}, "main")
	return factory.CreateCommitRules()
}

func countMergeCommits(commits []domain.Commit) int {
	n := 0
	for _, c := range commits {
		if c.IsMergeCommit {
			n++
		}
	}
	return n
}

func requireRule(t *testing.T, rs []domain.CommitRule, name string) {
	t.Helper()
	for _, r := range rs {
		if r.Name() == name {
			return
		}
	}
	t.Fatalf("expected rule %q to be in rule set, got: %v", name, ruleNames(rs))
}

func ruleNames(rs []domain.CommitRule) []string {
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.Name()
	}
	return names
}
