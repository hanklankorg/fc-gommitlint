// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	"codefloe.com/itiquette/gommitlint/internal/adapters/spell"
	"codefloe.com/itiquette/gommitlint/internal/adapters/stemming"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"

	"github.com/urfave/cli/v3"
)

// Validation errors.
var (
	ErrUnsupportedFormat   = errors.New("unsupported format")
	ErrUnsupportedTheme    = errors.New("unsupported theme")
	ErrUnsupportedColor    = errors.New("unsupported color setting")
	ErrUnsupportedLogLevel = errors.New("unsupported log level")
	ErrUnknownRule         = errors.New("unknown rule name")
)

// NewValidateCommand creates the validate subcommand.
func NewValidateCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameValidate,
		Usage: localizer.Get("validate commit messages against configured rules"),
		Description: localizer.Get(`validate searches for commit messages that match or violate the configured
rule and prints each violation found. By default, validate examines the HEAD
commit in the current repository.

A FILE of "-" stands for standard input. If no validation target is specified,
validate examines the HEAD commit. If --base-branch is given, validate examines
all commits from the base branch to HEAD.

The exit status is 0 if no violations are found, 1 if violations are found,
and 2 for fatal errors.

Examples:
  # Validate the HEAD commit
  gommitlint validate

  # Validate the last 3 commits
  gommitlint validate --count 3

  # Validate every commit between main and HEAD
  gommitlint validate --base-branch main

  # Validate a range (git range syntax)
  gommitlint validate --range main..feature

  # Validate a message from a file (commit-msg hook)
  gommitlint validate --message-file .git/COMMIT_EDITMSG

  # Validate a message from stdin
  echo "feat: add feature" | gommitlint validate --message-file -

  # Only run a subset of rules
  gommitlint validate --rule conventional,signoff

  # Emit machine-readable JSON for a CI pipeline
  gommitlint validate --format json`),

		Flags: []cli.Flag{
			// Input Selection
			&cli.StringFlag{
				Name:     flagNameMessageFile,
				Aliases:  []string{"f"},
				Usage:    localizer.Get("read commit message from `FILE` (use - for stdin)"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.BoolFlag{
				Name:     flagNameIgnoreRepoRules,
				Usage:    localizer.Get("only run commit message rules, ignore all repository rules"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.StringFlag{
				Name:     flagNameBaseBranch,
				Aliases:  []string{"b"},
				Usage:    localizer.Get("validate all commits from `BRANCH` to HEAD (comma-separated list for fallback, e.g., main,master,develop)"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.StringFlag{
				Name:     flagNameRange,
				Usage:    localizer.Get("validate commits in git `RANGE` (e.g., main..feature)"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.StringFlag{
				Name:     flagNameRef,
				Aliases:  []string{"r"},
				Usage:    localizer.Get("validate specific git `REF` (default: HEAD)"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.IntFlag{
				Name:     flagNameCount,
				Aliases:  []string{"n"},
				Value:    1,
				Usage:    localizer.Get("validate last N commits from HEAD"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.StringFlag{
				Name:     flagNameRule,
				Usage:    localizer.Get("validate only specific `RULES` (comma-separated, e.g., conventional,signoff). Default: enabled-by-config set"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.StringFlag{
				Name:     flagNameExcludeRule,
				Usage:    localizer.Get("exclude specific `RULES` from validation (comma-separated). Applied after --rule"),
				Category: localizer.Get("Input Selection"),
			},
			&cli.BoolFlag{
				Name:     "no-ci-detect",
				Usage:    localizer.Get("disable automatic CI environment detection (also: GOMMITLINT_NO_CI_DETECT)"),
				Category: localizer.Get("Input Selection"),
			},

			// Output Control
			&cli.StringFlag{
				Name:     flagNameFormat,
				Value:    output.FormatText,
				Usage:    outputFormatUsage(localizer),
				Category: localizer.Get("Output Control"),
			},
			&cli.BoolFlag{
				Name:     flagNameVerbose,
				Aliases:  []string{"v"},
				Usage:    localizer.Get("increase output verbosity (-v or -vv)"),
				Category: localizer.Get("Output Control"),
			},
			&cli.StringFlag{
				Name:     flagNameRuleHelp,
				Usage:    localizer.Get("show detailed help for specific `RULE` and exit"),
				Category: localizer.Get("Output Control"),
			},
			&cli.StringFlag{
				Name:     flagNameReportFile,
				Usage:    localizer.Get("write validation results to `FILE` (use - for stdout)"),
				Category: localizer.Get("Output Control"),
			},
		},

		Action: ExecuteValidation,
	}
}

// generateFailureMessage creates an accurate error message based on what failed.
func generateFailureMessage(report domain.Report, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)
	hasCommitFailures := report.Summary.FailedCommits > 0
	hasRepoFailures := hasRepositoryFailures(report.Repository.RuleResults)

	switch {
	case hasCommitFailures && hasRepoFailures:
		return localizer.Get("validation failed: commit and repository violations found")
	case hasCommitFailures:
		return localizer.Get("validation failed: one or more commits have violations")
	case hasRepoFailures:
		return localizer.Get("validation failed: repository violations found")
	default:
		return localizer.Get("validation failed")
	}
}

// hasRepositoryFailures checks if any repository rules failed.
func hasRepositoryFailures(rules []domain.RuleReport) bool {
	for _, rule := range rules {
		if rule.Status != domain.StatusPassed {
			return true
		}
	}

	return false
}

// writeJobSummaryIfNeeded writes a job summary to $GITHUB_STEP_SUMMARY when using CI formats.
func writeJobSummaryIfNeeded(logger domain.Logger, report domain.Report, outputOptions cliAdapter.OutputOptions, getEnv func(string) string) {
	if !output.IsGitHubCompatibleFormat(outputOptions.Format) {
		return
	}

	summaryFile := getEnv("GITHUB_STEP_SUMMARY")
	if summaryFile == "" {
		return
	}

	if err := domain.DefaultSecurityPolicy().ValidatePath(summaryFile); err != nil {
		logger.Debug("Skipping job summary due to invalid path", "error", err, "file", summaryFile)

		return
	}

	if !filepath.IsAbs(summaryFile) {
		logger.Debug("Skipping job summary with non-absolute path", "file", summaryFile)

		return
	}

	summaryOpts := output.SummaryOptions{
		Title:         outputOptions.GetAnnotationTitle(),
		Localizer:     outputOptions.Localizer,
		ConfigSources: outputOptions.ConfigSources,
	}
	summary := output.JobSummary(report, summaryOpts)

	//nolint:gosec // G306: 0644 is appropriate for markdown summary readable by CI system
	if writeErr := os.WriteFile(summaryFile, []byte(summary), 0o644); writeErr != nil {
		logger.Debug("Failed to write job summary", "error", writeErr, "file", summaryFile)
		// Don't fail the command - the main report was written successfully
	} else {
		logger.Debug("Job summary written", "file", summaryFile)
	}
}

// getRuleHelpContent provides help documentation for each validation rule.
func getRuleHelpContent(ruleName string, localizer output.Localizer) string {
	normalizedRule := configTypes.NormalizeRuleName(ruleName)

	localizer = output.EnsureLocalizer(localizer)

	entry, exists := ruleHelpRegistry()[normalizedRule]
	if !exists {
		return localizer.Get("Help for rule: %s", ruleName) + "\n\n" +
			localizer.Get("No help available for rule '%s'.", ruleName) + "\n\n" +
			localizer.Get("Available rules:") + "\n  " +
			strings.Join(getAvailableRuleNames(), ", ") + "\n"
	}

	localizedTemplate, key, ok := localizedRuleHelpTemplate(localizer, normalizedRule)
	if !ok {
		return localizer.Get("Help for rule: %s", ruleName) + "\n\n" +
			localizer.Get("No help available for rule '%s'.", ruleName) + "\n\n" +
			localizer.Get("Available rules:") + "\n  " +
			strings.Join(getAvailableRuleNames(), ", ") + "\n"
	}

	if localizedTemplate == key {
		localizedTemplate = entry.template
	}

	content := localizedTemplate
	if len(entry.args) > 0 {
		content = fmt.Sprintf(localizedTemplate, entry.args...)
		if strings.Contains(content, "%!") {
			content = fmt.Sprintf(entry.template, entry.args...)
		}
	}

	return formatRuleHelpContent(normalizedRule, content, localizer)
}

// getAvailableRuleNames returns list of all available rule names for help (sorted alphabetically).
func getAvailableRuleNames() []string {
	return configTypes.GetValidRuleNamesSorted()
}

func ensureRuleExcluded(excludeRules []string, ruleName string) []string {
	if ruleName == "" {
		return excludeRules
	}

	for _, excluded := range excludeRules {
		if strings.EqualFold(excluded, ruleName) {
			return excludeRules
		}
	}

	return append(excludeRules, ruleName)
}

func buildRules(cfg configTypes.Config, logger domain.Logger, ruleFilter []string, excludeRules []string, baseBranch string, localizer output.Localizer) ([]domain.CommitRule, []domain.RepositoryRule, error) {
	ruleFactory, err := createRuleFactory(cfg, logger, ruleFilter, excludeRules, baseBranch)
	if err != nil {
		loc := output.EnsureLocalizer(localizer)

		return nil, nil, newRecoveryErrorWithCause(
			err,
			output.Msg("failed to create spell checker"),
			loc.Get("Check spell locale in configuration"),
			loc.Get("Run 'gommitlint config show' to inspect settings"),
		)
	}

	// Create rules through the factory
	commitRules := ruleFactory.CreateCommitRulesFiltered(ruleFilter)
	repoRules := ruleFactory.CreateRepositoryRulesFiltered(ruleFilter)

	commitRules, repoRules = applyRuleExclusions(commitRules, repoRules, excludeRules, logger)

	return commitRules, repoRules, nil
}

func createRuleFactory(cfg configTypes.Config, logger domain.Logger, ruleFilter []string, excludeRules []string, baseBranch string) (*rulefactory.Factory, error) {
	deps, err := createRuleDependencies(cfg, ruleFilter, excludeRules)
	if err != nil {
		return nil, err
	}

	return rulefactory.NewFactoryWithDependencies(cfg, deps, logger, baseBranch), nil
}

func createRuleDependencies(cfg configTypes.Config, ruleFilter []string, excludeRules []string) (rulefactory.Dependencies, error) {
	deps := rulefactory.Dependencies{Stemmer: stemming.NewSnowballStemmer()}

	if shouldBuildSpellChecker(cfg, ruleFilter, excludeRules) {
		checker, err := spell.NewMisspellAdapter(configTypes.DerefString(cfg.Spell.Locale, configTypes.DefaultSpellLocale))
		if err != nil {
			return rulefactory.Dependencies{}, err
		}

		deps.SpellChecker = checker
	}

	return deps, nil
}

func shouldBuildSpellChecker(cfg configTypes.Config, ruleFilter []string, excludeRules []string) bool {
	if !rulefactory.IsCommitRuleEnabled(cfg, ruleFilter, rulecatalog.Spell) {
		return false
	}

	for _, excluded := range excludeRules {
		if strings.EqualFold(excluded, rulecatalog.Spell) {
			return false
		}
	}

	return true
}

func applyRuleExclusions(commitRules []domain.CommitRule, repoRules []domain.RepositoryRule, excludeRules []string, logger domain.Logger) ([]domain.CommitRule, []domain.RepositoryRule) {
	if len(excludeRules) == 0 {
		return commitRules, repoRules
	}

	commitRules = excludeRulesByName(commitRules, excludeRules)
	repoRules = excludeRulesByName(repoRules, excludeRules)
	logger.Debug("Rules excluded", "excluded_rules", excludeRules, "commit_rules_after", len(commitRules), "repo_rules_after", len(repoRules))

	return commitRules, repoRules
}

// excludeRulesByName filters out excluded rules from any rule list.
func excludeRulesByName[T interface{ Name() string }](rules []T, excludeRules []string) []T {
	if len(excludeRules) == 0 {
		return rules
	}

	excludeSet := make(map[string]struct{}, len(excludeRules))
	for _, ruleName := range excludeRules {
		excludeSet[strings.ToLower(ruleName)] = struct{}{}
	}

	filtered := make([]T, 0, len(rules))
	for _, rule := range rules {
		if _, ok := excludeSet[strings.ToLower(rule.Name())]; !ok {
			filtered = append(filtered, rule)
		}
	}

	return filtered
}
