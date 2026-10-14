// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rulefactory

import (
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	"codefloe.com/itiquette/gommitlint/internal/adapters/stemming"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// IsRepositoryOnlyRule returns true if the rule name is a repository-only rule.
func IsRepositoryOnlyRule(ruleName string) bool {
	return rulecatalog.IsRepositoryRule(ruleName)
}

func defaultCommitEnabledRules() []string {
	return rulecatalog.DefaultEnabledByKind(rulecatalog.KindCommit)
}

func defaultRepositoryEnabledRules() []string {
	return rulecatalog.DefaultEnabledByKind(rulecatalog.KindRepository)
}

// Factory creates rule instances based on configuration, wiring their
// dependencies for domain rules.
type Factory struct {
	baseBranch string
	config     settings.Config
	deps       Dependencies
	logger     domain.Logger
}

// Dependencies groups external dependencies needed by rule constructors.
type Dependencies struct {
	CryptoKeyProvidersFactory func(settings.Config, domain.Logger) rules.CryptoKeyProviders
	SpellChecker              rules.SpellChecker
	Stemmer                   rules.Stemmer
}

// NewFactory creates a new rule factory with the given dependencies.
func NewFactory(cfg settings.Config, spellChecker rules.SpellChecker, logger domain.Logger, baseBranch string) *Factory {
	return NewFactoryWithDependencies(cfg, Dependencies{
		CryptoKeyProvidersFactory: defaultCryptoKeyProviders,
		SpellChecker:              spellChecker,
		Stemmer:                   stemming.NewSnowballStemmer(),
	}, logger, baseBranch)
}

// NewFactoryWithDependencies creates a new rule factory with explicit dependencies.
func NewFactoryWithDependencies(cfg settings.Config, deps Dependencies, logger domain.Logger, baseBranch string) *Factory {
	if deps.CryptoKeyProvidersFactory == nil {
		deps.CryptoKeyProvidersFactory = defaultCryptoKeyProviders
	}

	return &Factory{
		baseBranch: baseBranch,
		config:     cfg,
		deps:       deps,
		logger:     logger,
	}
}

// EnabledCommitRuleNames returns the configured commit rule names after filtering.
func EnabledCommitRuleNames(cfg settings.Config, ruleFilter []string) []string {
	_, enabledRules := resolveEnabledRuleNames(cfg, ruleFilter, defaultCommitEnabledRules())

	return enabledRules
}

func resolveEnabledRuleNames(cfg settings.Config, ruleFilter []string, defaultEnabled []string) (settings.RuleResolution, []string) {
	resolution := settings.ResolveRuleConfiguration(defaultEnabled, cfg.Rules)
	enabledRules := resolution.FinalEnabled
	enabledRules = appendCheckEnabledRuleNames(enabledRules, cfg)

	if len(ruleFilter) > 0 {
		enabledRules = filterRuleNames(enabledRules, ruleFilter)
	}

	return resolution, enabledRules
}

// IsCommitRuleEnabled reports whether a commit rule is active after filtering.
func IsCommitRuleEnabled(cfg settings.Config, ruleFilter []string, ruleName string) bool {
	return slices.Contains(EnabledCommitRuleNames(cfg, ruleFilter), ruleName)
}

// CreateCommitRules creates commit rules based on configuration.
func (f *Factory) CreateCommitRules() []domain.CommitRule {
	return f.CreateCommitRulesFiltered(nil)
}

// CreateCommitRulesFiltered creates commit rules with optional filtering.
func (f *Factory) CreateCommitRulesFiltered(ruleFilter []string) []domain.CommitRule {
	defaultEnabled := defaultCommitEnabledRules()
	resolution, enabledRules := resolveEnabledRuleNames(f.config, ruleFilter, defaultEnabled)

	f.logger.Debug(
		"Rule configuration evaluated",
		"default_enabled",
		resolution.DefaultEnabled,
		"default_disabled",
		resolution.DefaultDisabled,
		"config_enabled",
		resolution.ConfigEnabled,
		"config_disabled",
		resolution.ConfigDisabled,
		"final_enabled",
		enabledRules,
		"filter",
		ruleFilter,
	)

	// Conventional-awareness is a config property of Subject (and Jira), not a
	// runtime one. Check the resolved configuration BEFORE the --rule filter so
	// that "--rule subject" against a repo that has ConventionalCommit enabled
	// still produces conventional-aware Subject diagnostics ("'no' is not a
	// verb" instead of "Use uppercase for the first word: 'Feat:'"). The
	// previous behaviour silently lost conventional context when other rules
	// were filtered out.
	isConventionalEnabled := slices.Contains(resolution.FinalEnabled, rulecatalog.Conventional)
	rulesEnabledByRulesConfig := ruleNameSet(resolution.FinalEnabled)

	f.logger.Debug("Conventional rule dependency checked",
		"conventional_enabled", isConventionalEnabled,
		"selected_for_run", slices.Contains(enabledRules, rulecatalog.Conventional))

	// Create enabled rules
	commitRules := make([]domain.CommitRule, 0, len(enabledRules))

	factories := commitRuleFactories()

	for _, ruleName := range enabledRules {
		if IsRepositoryOnlyRule(ruleName) {
			continue
		}

		factory, ok := factories[ruleName]
		if !ok {
			f.logger.Debug("Rule creation skipped", "rule", ruleName, "reason", "unknown commit rule")

			continue
		}

		rule := factory(f, isConventionalEnabled)
		if rule == nil {
			f.logger.Debug("Rule creation skipped", "rule", ruleName, "reason", "unknown commit rule")

			continue
		}

		_, enabledByRulesConfig := rulesEnabledByRulesConfig[ruleName]

		wrappedRule, ok := wrapCommitRuleWithChecks(rule, f.config, enabledByRulesConfig)
		if !ok {
			f.logger.Debug("Rule creation skipped", "rule", ruleName, "reason", "all checks disabled")

			continue
		}

		f.logger.Debug("Rule instance created", "rule", ruleName, "type", "commit")

		commitRules = append(commitRules, wrappedRule)
	}

	return commitRules
}

// CreateRepositoryRules creates repository rules based on configuration.
func (f *Factory) CreateRepositoryRules() []domain.RepositoryRule {
	return f.CreateRepositoryRulesFiltered(nil)
}

// CreateRepositoryRulesFiltered creates repository rules with optional filtering.
func (f *Factory) CreateRepositoryRulesFiltered(ruleFilter []string) []domain.RepositoryRule {
	defaultEnabled := defaultRepositoryEnabledRules()
	resolution, enabledRules := resolveEnabledRuleNames(f.config, ruleFilter, defaultEnabled)

	f.logger.Debug(
		"Repository rule configuration evaluated",
		"default_enabled",
		resolution.DefaultEnabled,
		"config_enabled",
		resolution.ConfigEnabled,
		"config_disabled",
		resolution.ConfigDisabled,
		"final_enabled",
		enabledRules,
		"filter",
		ruleFilter,
	)

	// Pre-allocate with maximum possible size
	repoRules := make([]domain.RepositoryRule, 0, len(enabledRules))
	rulesEnabledByRulesConfig := ruleNameSet(resolution.FinalEnabled)

	// Create only enabled rules using explicit switch (consistent with commit rules)
	repoFactories := repositoryRuleFactories()
	for _, ruleName := range enabledRules {
		factory, ok := repoFactories[ruleName]
		if !ok {
			continue
		}

		rule := factory(f)
		_, enabledByRulesConfig := rulesEnabledByRulesConfig[ruleName]

		wrappedRule, ok := wrapRepositoryRuleWithChecks(rule, f.config, enabledByRulesConfig)
		if !ok {
			f.logger.Debug("Rule creation skipped", "rule", ruleName, "reason", "all checks disabled")

			continue
		}

		repoRules = append(repoRules, wrappedRule)

		f.logger.Debug("Rule instance created", "rule", ruleName, "type", "repository")
	}

	return repoRules
}

func commitRuleFactories() map[string]func(*Factory, bool) domain.CommitRule {
	return map[string]func(*Factory, bool) domain.CommitRule{
		rulecatalog.Subject: func(factory *Factory, isConventionalEnabled bool) domain.CommitRule {
			if factory.deps.Stemmer == nil {
				return rules.NewSubjectRule(factory.config, isConventionalEnabled)
			}

			return rules.NewSubjectRuleWithStemmer(factory.config, isConventionalEnabled, factory.deps.Stemmer)
		},
		rulecatalog.JiraReference: func(factory *Factory, isConventionalEnabled bool) domain.CommitRule {
			return rules.NewJiraReferenceRule(factory.config, isConventionalEnabled)
		},
		rulecatalog.Conventional: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewConventionalCommitRule(factory.config)
		},
		rulecatalog.CommitBody: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewCommitBodyRule(factory.config)
		},
		rulecatalog.Spell: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewSpellRule(factory.config, factory.deps.SpellChecker)
		},
		rulecatalog.SignOff: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewSignOffRule(factory.config)
		},
		rulecatalog.CryptoSignature: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewCryptoSignatureRule(factory.config, factory.createCryptoKeyProviders())
		},
		rulecatalog.Identity: func(factory *Factory, _ bool) domain.CommitRule {
			return rules.NewIdentityRule(factory.config)
		},
		rulecatalog.LinearHistory: func(_ *Factory, _ bool) domain.CommitRule {
			return rules.NewLinearHistoryRule()
		},
	}
}

func repositoryRuleFactories() map[string]func(*Factory) domain.RepositoryRule {
	return map[string]func(*Factory) domain.RepositoryRule{
		rulecatalog.BranchAhead: func(factory *Factory) domain.RepositoryRule {
			return rules.NewBranchAheadRule(factory.config, factory.baseBranch)
		},
	}
}

// createCryptoKeyProviders creates key-provider capabilities for CryptoSignatureRule if configured.
// The zero value is valid when no key directory is configured; the rule can still
// check whether commits are signed without validating against specific keys.
func (f *Factory) createCryptoKeyProviders() rules.CryptoKeyProviders {
	return f.deps.CryptoKeyProvidersFactory(f.config, f.logger)
}

func defaultCryptoKeyProviders(cfg settings.Config, logger domain.Logger) rules.CryptoKeyProviders {
	if cfg.CryptoSignature.KeyDirectory != "" {
		maxKeyFiles := settings.DerefInt(cfg.Limits.MaxKeyFiles, settings.DefaultMaxKeyFiles)
		provider := signing.NewFileSystemKeyProviderWithLimit(cfg.CryptoSignature.KeyDirectory, maxKeyFiles).
			WithLogger(logger)

		return rules.CryptoKeyProviders{
			Directory: provider,
			GPG:       provider,
			SSH:       provider,
		}
	}

	if len(cfg.CryptoSignature.AllowedSigners) > 0 {
		logger.Warn("CryptoSignature allowed_signers configured but key_directory is empty - allowed_signers will have no effect")
	}

	return rules.CryptoKeyProviders{}
}

func filterRuleNames(rules []string, filter []string) []string {
	if len(filter) == 0 {
		return rules
	}

	filtered := make([]string, 0, len(filter))

	for _, rule := range rules {
		for _, filterName := range filter {
			if strings.EqualFold(strings.TrimSpace(rule), strings.TrimSpace(filterName)) {
				filtered = append(filtered, rule)

				break
			}
		}
	}

	return filtered
}
