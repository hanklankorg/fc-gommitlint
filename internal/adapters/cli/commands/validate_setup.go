// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"os"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/ci"
	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// ValidationSetup holds all components needed for validation execution.
type ValidationSetup struct {
	Config         configTypes.Config
	ConfigSource   string
	ConfigSources  []string
	WarningDetails []domain.Warning
	Target         appValidation.Target
	OutputOptions  cliAdapter.OutputOptions
	RuleFilter     []string
	ExcludeRules   []string
	BaseBranch     []string // branch candidates in fallback order, parsed from CSV
	BaseBranchSet  bool
}

// prepareValidationSetup creates validation setup from CLI command.
func prepareValidationSetup(cmd *cli.Command, loc output.Localizer) (ValidationSetup, error) {
	cfgResult, err := LoadConfigFromCommand(cmd.Root())
	if err != nil {
		return ValidationSetup{}, configLoadError(err, loc)
	}

	// Surface config-load warnings on stderr immediately. Auto-discovered
	// configs degrade gracefully on parse errors (see loader.go docstring),
	// but the user still needs to see the diagnostic — the text formatter
	// only renders warnings on some code paths, so routing them to stderr
	// here keeps the stream contract consistent across pass/fail outcomes.
	if err := showConfigWarnings(cmd.ErrWriter, cmd.Bool(flags.Quiet), configWarningDetails(cfgResult.Warnings), loc); err != nil {
		return ValidationSetup{}, err
	}

	// Detect CI environment once, used for both target and output auto-detection.
	// The --no-ci-detect flag and GOMMITLINT_NO_CI_DETECT env var disable this.
	ciEnv := detectCIEnvironment(cmd)

	target, baseBranch, err := createValidationTarget(cmd, cfgResult.Config, ciEnv)
	if err != nil {
		return ValidationSetup{}, validationTargetError(err, loc)
	}

	ruleFilter, err := parseAndValidateRuleFilter(flagNameRule, cmd.String(flagNameRule), loc)
	if err != nil {
		return ValidationSetup{}, err
	}

	outputOptions, err := createOutputOptions(cmd, cfgResult.Config, ciEnv, loc)
	if err != nil {
		// Inner helpers (resolveTheme, resolveOutputFormat, createOutputWriter, applyVerbosity)
		// already return well-formed RecoveryErrors with flag-specific suggestions. Wrapping
		// them in a generic "failed to create output options" message would hide the actionable
		// text — propagate the inner error verbatim.
		if _, ok := errors.AsType[RecoveryError](err); ok {
			return ValidationSetup{}, err
		}

		return ValidationSetup{}, newRecoveryErrorWithCause(
			err,
			output.Msg("failed to create output options: %s", err),
			helpSuggestion(loc, "gommitlint validate"),
			loc.Get("Use --format text for detailed output"),
		)
	}

	excludeRules, err := parseAndValidateRuleFilter(flagNameExcludeRule, cmd.String(flagNameExcludeRule), loc)
	if err != nil {
		return ValidationSetup{}, err
	}

	// Config provenance is rendered once, here, where the working directory and
	// $HOME are available to shorten paths. Formatters print what they are given.
	configDisplay := cfgResult.Display(currentConfigDisplayEnv(), loc)
	outputOptions.ConfigSources = configDisplay.Sources
	outputOptions.ConfigSourcesUnexpected = configDisplay.Unexpected

	return ValidationSetup{
		Config:         cfgResult.Config,
		ConfigSource:   cfgResult.DisplaySource(),
		ConfigSources:  configSourcePaths(cfgResult.Sources),
		WarningDetails: configWarningDetails(cfgResult.Warnings),
		RuleFilter:     ruleFilter,
		ExcludeRules:   excludeRules,
		Target:         target,
		OutputOptions:  outputOptions,
		BaseBranch:     baseBranch,
		BaseBranchSet:  cmd.IsSet(flagNameBaseBranch),
	}, nil
}

// parseRuleFilter parses comma-separated rule names from string.
func parseRuleFilter(ruleStr string) []string {
	if ruleStr == "" {
		return nil
	}

	rules := strings.Split(ruleStr, ",")
	filtered := make([]string, 0, len(rules))

	for _, rule := range rules {
		trimmed := strings.TrimSpace(rule)
		if trimmed != "" {
			filtered = append(filtered, strings.ToLower(trimmed))
		}
	}

	return filtered
}

// parseAndValidateRuleFilter combines parsing with unknown-rule validation
// and catches the in-between case: the user passed a non-empty value but it
// contained nothing parseable (e.g. `--rule ","` or `--rule "   "`). Without
// this check those values silently degraded to "no filter" — the user
// thought they'd restricted the rule set but actually disabled the filter.
func parseAndValidateRuleFilter(flag, rawValue string, localizer output.Localizer) ([]string, error) {
	filter := parseRuleFilter(rawValue)

	// User typed something but it parsed to nothing. Could be ',' (commas
	// with no rules) or '   ' (whitespace only). Either way, surface it
	// instead of silently running all rules — the user's intent was almost
	// certainly to filter, and silently disabling the filter masks the typo.
	if rawValue != "" && len(filter) == 0 {
		loc := output.EnsureLocalizer(localizer)

		return nil, newRecoveryErrorWithCause(
			ErrUnknownRule,
			output.Msg("--%s value %q contains no rule names", flag, rawValue),
			loc.Get("Use commas to separate rules: --%s subject,signoff", flag),
			loc.Get("Or omit --%s to run all configured rules", flag),
		)
	}

	if err := validateRuleNames(flag, filter, localizer); err != nil {
		return nil, err
	}

	return filter, nil
}

// validateRuleNames returns an error if any name is not a known rule.
func validateRuleNames(flag string, names []string, localizer output.Localizer) error {
	if len(names) == 0 {
		return nil
	}

	loc := output.EnsureLocalizer(localizer)

	validSet := make(map[string]struct{}, len(configTypes.GetValidRuleNames()))
	for _, v := range configTypes.GetValidRuleNames() {
		validSet[v] = struct{}{}
	}

	var unknown []string

	for _, name := range names {
		if _, ok := validSet[name]; !ok {
			unknown = append(unknown, name)
		}
	}

	if len(unknown) == 0 {
		return nil
	}

	suggestions := []string{
		loc.Get("Valid rules: %s", strings.Join(configTypes.GetValidRuleNamesSorted(), ", ")),
		loc.Get("Run 'gommitlint config show' to see active rules"),
	}

	// Detect the most common typing mistake: passing rules as whitespace-
	// separated tokens (`--rule "subject signoff"`) when the parser expects
	// comma separation. Without this hint the user just sees their concatenated
	// input echoed back as "unknown" without a clear way forward.
	if anyContainsWhitespace(unknown) {
		suggestions = append([]string{
			loc.Get("Separate rules with commas: --%s subject,signoff", flag),
		}, suggestions...)
	}

	return newRecoveryErrorWithCause(
		ErrUnknownRule,
		output.Msg("--%s contains unknown rule(s): %s", flag, strings.Join(unknown, ", ")),
		suggestions...,
	)
}

func anyContainsWhitespace(names []string) bool {
	for _, name := range names {
		if strings.ContainsAny(name, " \t") {
			return true
		}
	}

	return false
}

func configSourcePaths(sources []configuration.ConfigSource) []string {
	paths := make([]string, 0, len(sources))
	for _, source := range sources {
		if source.Path == "" {
			continue
		}

		paths = append(paths, source.Path)
	}

	return paths
}

func configWarningDetails(warnings []domain.Warning) []domain.Warning {
	return append([]domain.Warning(nil), warnings...)
}

// createValidationTarget builds validation scope from CLI flags.
// Validation and security checks are handled by NewValidationTarget which returns
// proper domain error codes for user-friendly messages and exit code classification.
// Returns the validation target and the parsed branch candidates.
func createValidationTarget(cmd *cli.Command, cfg configTypes.Config, ciEnv ci.Environment) (appValidation.Target, []string, error) {
	messageFile := cmd.String(flagNameMessageFile)
	gitRef := cmd.String(flagNameRef)
	commitRange := cmd.String(flagNameRange)
	baseBranch := cmd.String(flagNameBaseBranch)
	commitCount := cmd.Int(flagNameCount)
	maxCommitCount := configTypes.DerefInt(cfg.Limits.MaxCommitCount, configTypes.DefaultMaxCommitCount)

	// CI provides one literal branch name. Unlike the CLI's --base-branch value,
	// it is not a comma-separated fallback list; commas are valid in Git refs.
	baseBranchFromCI := !cmd.IsSet(flagNameBaseBranch) && ciEnv.BaseBranch != ""
	if baseBranchFromCI {
		baseBranch = ciEnv.BaseBranch
	}

	candidates := cliAdapter.ParseBaseBranchCandidates(baseBranch)
	if baseBranchFromCI {
		candidates = []string{baseBranch}
	}

	if baseBranchFromCI && messageFile == "" {
		if err := cliAdapter.ValidateGitReference(baseBranch); err != nil {
			return appValidation.Target{}, candidates, domain.New(
				diagnostics.SourceIDValidation,
				diagnostics.ReasonInvalidReference,
				"Invalid base branch: "+baseBranch,
			)
		}

		return appValidation.BaseBranchTarget(baseBranch), candidates, nil
	}

	cliTarget, err := cliAdapter.NewValidationTarget(messageFile, gitRef, commitRange, baseBranch, commitCount, maxCommitCount)
	if err != nil {
		return appValidation.Target{}, candidates, err
	}

	return cliTarget, candidates, nil
}

// detectCIEnvironment detects the CI environment, respecting the --no-ci-detect flag.
func detectCIEnvironment(cmd *cli.Command) ci.Environment {
	if cmd.Bool("no-ci-detect") {
		return ci.Environment{}
	}

	return ci.Detect(os.Getenv)
}

// GetRepoPath returns the repository path the user supplied via --repo-path.
// It returns an empty string when repository discovery should start at the CWD.
func GetRepoPath(cmd *cli.Command) string {
	return cmd.Root().String(flagNameRepoPath)
}
