// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"os"
	"time"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/messagefile"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	repoAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/repository"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// executeValidationAndReport executes validation and writes the report.
func executeValidationAndReport(ctx context.Context, cmd *cli.Command, setup ValidationSetup,
	outputOptions cliAdapter.OutputOptions, ruleFilter []string, excludeRules []string,
) error {
	logger := logadapter.GetDomainLogger(ctx)

	logger.Trace("Entering executeValidationAndReport", "target_type", setup.Target.Type, "format", outputOptions.Format)

	defer func() {
		logger.Trace("Exiting executeValidationAndReport", "target_type", setup.Target.Type)
	}()

	loc := output.EnsureLocalizer(outputOptions.Localizer)
	service := appValidation.Service{
		BuildRules: func(cfg configTypes.Config, ruleFilter []string, excludeRules []string, baseBranch string) (appValidation.Rules, error) {
			commitRules, repoRules, err := buildRules(cfg, logger, ruleFilter, excludeRules, baseBranch, loc)
			if err != nil {
				return appValidation.Rules{}, err
			}

			return appValidation.Rules{Commit: commitRules, Repository: repoRules}, nil
		},
		IsMissingRepository: isMissingRepository,
		Logger:              logger,
		OpenRepository: func(ctx context.Context) (domain.Repository, error) {
			repo, err := createRepositoryAdapter(ctx, cmd, logger, loc)
			if err != nil {
				return nil, err
			}

			return repo, nil
		},
		ReadMessage: messagefile.Read,
	}

	report, err := service.Validate(ctx, appValidation.Request{
		BaseBranchCandidates: setup.BaseBranch,
		BaseBranchRequired:   setup.BaseBranchSet,
		Config:               setup.Config,
		ConfigSources:        setup.ConfigSources,
		ExcludeRules:         excludeRules,
		IgnoreRepoRules:      cmd.Bool(flagNameIgnoreRepoRules),
		RuleFilter:           ruleFilter,
		Target:               setup.Target,
		WarningDetails:       setup.WarningDetails,
	})
	if err != nil {
		return errors.Join(
			mapValidationServiceError(err, setup, outputOptions, loc),
			closeValidationOutput(logger, outputOptions, loc),
		)
	}

	logger.Debug("Validation completed", "passed", report.Summary.AllPassed, "commit_count", len(report.Commits), "failed_commits", report.Summary.FailedCommits)

	if err := writeValidationReport(logger, report, outputOptions, loc); err != nil {
		return err
	}

	logger.Debug("Validation and report completed successfully", "target_type", setup.Target.Type, "format", outputOptions.Format)

	if !report.Summary.AllPassed {
		logger.Debug("Validation failed, will return validation error", "failed_commits", report.Summary.FailedCommits)
		// Validation error will be classified as exit code 1
		failureMsg := generateFailureMessage(report, outputOptions.Localizer)

		return errors.Join(
			ErrValidationReportWritten,
			domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonValidationFailed, failureMsg),
		)
	}

	return nil
}

// mapValidationServiceError converts errors from the validation service into
// CLI-friendly recovery errors with targeted suggestions. Extracted from
// executeValidationAndReport so the orchestrator stays linear; the routing
// is essentially a dispatch table over sentinel error types.
func mapValidationServiceError(err error, setup ValidationSetup, outputOptions cliAdapter.OutputOptions, loc output.Localizer) error {
	loc = output.EnsureLocalizer(loc)

	if errors.Is(err, appValidation.ErrRepositoryRequired) {
		return validationRequiresRepositoryError(setup.Target.Type, loc)
	}

	if errors.Is(err, appValidation.ErrEmptyCommitMessage) {
		// Surface the real cause — the user provided no message to validate —
		// instead of the catch-all "validation failed" with generic suggestions.
		return newRecoveryErrorWithCause(
			validationInputCause(err, diagnostics.ReasonInvalidCommit),
			output.Msg("empty commit message"),
			loc.Get("Provide a commit subject line, e.g. 'feat: add feature'"),
			loc.Get("Validate from a file: gommitlint validate --message-file commit-msg.txt"),
			loc.Get("Validate from stdin: echo 'feat: message' | gommitlint validate --message-file -"),
			loc.Get("Validate HEAD instead: gommitlint validate"),
		)
	}

	if errors.Is(err, domain.ErrReferenceNotFound) {
		// --range / --base-branch / --ref with an unknown ref: name the ref and
		// point at git log so the user can find a real one quickly.
		return newRecoveryErrorWithCause(
			referenceNotFoundCause(err, setup.Target),
			output.Msg("git reference not found: %s", err),
			loc.Get("List available branches: git branch -a"),
			loc.Get("List recent commits: git log --oneline -10"),
			loc.Get("Validate HEAD instead: gommitlint validate"),
		)
	}

	if errors.Is(err, context.Canceled) {
		return newRecoveryErrorWithCause(
			validationInputCause(err, diagnostics.ReasonContextCancelled),
			output.Msg("validation cancelled: %s", err),
			loc.Get("Retry validation"),
		)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return newRecoveryErrorWithCause(
			err,
			output.Msg("validation timed out: %s", err),
			loc.Get("Retry validation"),
		)
	}

	if isMissingRepository(err) {
		// Convert the sentinel into a domain ValidationError with
		// diagnostics.ReasonInvalidRepo so the exit-code mapping (GetExitCodeType in
		// domain/errors.go) routes it to ExitCodeUnavailable (69 —
		// EX_UNAVAILABLE per sysexits.h). Without this conversion the
		// catch-all falls through to ExitCodeUsage (2), which conflates
		// "env not ready" with "user typo'd the CLI."
		repoErr := domain.New("validate", diagnostics.ReasonInvalidRepo, err.Error())

		return newRecoveryErrorWithCause(
			repoErr,
			output.Msg("failed to create repository"),
			loc.Get("Run inside a git repository"),
			loc.Get("Specify repository path: gommitlint validate --repo-path /path/to/repo"),
		)
	}

	// Errors already carrying recovery / domain-validation shapes pass through
	// untouched — wrapping again would just nest the suggestions.
	if _, ok := errors.AsType[RecoveryError](err); ok {
		return err
	}

	if validationErr, ok := errors.AsType[domain.ValidationError](err); ok {
		if validationErr.Diagnostic.Reason == diagnostics.ReasonRangeNotFound {
			return newRecoveryErrorWithCause(
				validationErr,
				output.Msg(validationErr.Message),
				loc.Get("List available branches: git branch -a"),
				loc.Get("Use an existing base branch: gommitlint validate --base-branch main"),
				loc.Get("Validate HEAD instead: gommitlint validate"),
			)
		}

		return err
	}

	if errors.Is(err, repoAdapter.ErrSystemGitRequired) {
		return newRecoveryErrorWithCause(
			err,
			output.Msg("validation failed: %s", err),
			loc.Get("Install Git, or use the container image that includes it"),
			loc.Get("Run as the user that owns the repository; in the container image, mount the repository at /repo"),
			loc.Get("Check that Git can read it: git -C <path> rev-parse --show-object-format"),
		)
	}

	return newRecoveryErrorWithCause(
		err,
		output.Msg("validation failed: %s", err),
		helpSuggestion(outputOptions.Localizer, "gommitlint validate"),
		loc.Get("Use --format text for detailed output"),
	)
}

func validationInputCause(err error, reason diagnostics.Reason) error {
	return errors.Join(
		domain.New(diagnostics.SourceIDValidation, reason, err.Error()),
		err,
	)
}

func referenceNotFoundCause(err error, target appValidation.Target) error {
	reason := diagnostics.ReasonCommitNotFound
	if target.Type == appValidation.TargetTypeRange {
		reason = diagnostics.ReasonRangeNotFound
	}

	return validationInputCause(err, reason)
}

func writeValidationReport(logger domain.Logger, report domain.Report, outputOptions cliAdapter.OutputOptions, loc output.Localizer) (err error) {
	logger.Trace("Writing report", "format", outputOptions.Format)
	defer func() {
		err = errors.Join(err, closeValidationOutput(logger, outputOptions, loc))
	}()

	if err := outputOptions.WriteReport(report); err != nil {
		return newRecoveryErrorWithCause(
			errors.Join(ErrOutputWrite, err),
			output.Msg("failed to write report"),
			loc.Get("Check report file path or permissions"),
			loc.Get("Use --report-file - to write to stdout"),
		)
	}

	writeJobSummaryIfNeeded(logger, report, outputOptions, os.Getenv)

	return nil
}

func closeValidationOutput(logger domain.Logger, outputOptions cliAdapter.OutputOptions, loc output.Localizer) error {
	closeErr := outputOptions.Close()
	if closeErr == nil {
		return nil
	}

	logger.Warn("Failed to close validation output, some data may not have been written", "error", closeErr)

	return newRecoveryErrorWithCause(
		errors.Join(ErrOutputWrite, closeErr),
		output.Msg("failed to close validation output"),
		output.EnsureLocalizer(loc).Get("Check report file path or permissions"),
	)
}

func isMissingRepository(err error) bool {
	return errors.Is(err, fileops.ErrNotGitRepository) || errors.Is(err, domain.ErrNotGitRepository)
}

// executeRuleHelpStandalone prints rule documentation without loading config.
//
// The doc text is static (defined in code, not derived from config), so we
// don't need a config tree. Skipping the config load also skips the warning
// emission — users running `validate --rule-help signoff` from outside a repo
// or in a repo with a quirky config get clean docs instead of unrelated noise.
func executeRuleHelpStandalone(cmd *cli.Command, localizer output.Localizer) error {
	ruleHelp := cmd.String(flagNameRuleHelp)

	// Validate the rule name through the same allowlist that validation uses,
	// so a typo'd rule name gets the same actionable error here as it would
	// during a real validation run.
	options := cliAdapter.NewOutputOptions(cmd.Writer).
		WithRuleHelp(ruleHelp).
		WithLocalizer(localizer)
	if err := options.ValidateRuleHelp(); err != nil {
		loc := output.EnsureLocalizer(localizer)

		return newRecoveryErrorWithCause(
			err,
			output.Msg("%s", err),
			loc.Get("List configured rules: gommitlint config show"),
			loc.Get("Show docs for a rule: gommitlint validate --rule-help <name>"),
		)
	}

	help := getRuleHelpContent(ruleHelp, localizer)

	return writeCommandOutput(cmd.Writer, help, localizer)
}

// ExecuteValidation validates commit messages against configured rules.
// Supports validation of individual commits, commit ranges, or message files.
// Returns non-zero exit code if validation fails or errors occur.
func ExecuteValidation(ctx context.Context, cmd *cli.Command) error {
	logger := logadapter.GetDomainLogger(ctx)
	startTime := time.Now()

	logger.Trace("Starting validation workflow", "command", cmd.Name)

	defer func() {
		duration := time.Since(startTime)
		logger.Trace("Validation workflow completed", "command", cmd.Name, "duration_ms", duration.Milliseconds())
	}()

	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint validate", localizer.Get("Use flags to specify validation targets: --ref REF, --range RANGE, --base-branch BRANCH, --count N")); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// --rule-help is a documentation lookup, not a validation. Short-circuit
	// before loading config (and surfacing config warnings about a repo the
	// user isn't even validating). The docs are static — config-independent —
	// so this also lets `gommitlint validate --rule-help X` work cleanly
	// from outside any git repo.
	if cmd.String(flagNameRuleHelp) != "" {
		return executeRuleHelpStandalone(cmd, localizer)
	}

	setup, err := prepareValidationSetup(cmd, localizer)
	if err != nil {
		return err
	}

	logger.Debug("Configuration loaded", "source", setup.ConfigSource, "enabled_rules", len(setup.Config.Rules.Enabled), "disabled_rules", len(setup.Config.Rules.Disabled))

	logger.Trace("Starting validation and report execution", "target_type", setup.Target.Type)

	validationStartTime := time.Now()
	err = executeValidationAndReport(ctx, cmd, setup, setup.OutputOptions, setup.RuleFilter, setup.ExcludeRules)
	validationDuration := time.Since(validationStartTime)
	logger.Trace("Validation and report execution completed", "target_type", setup.Target.Type, "duration_ms", validationDuration.Milliseconds())
	logger.Debug("Validation execution completed", "command", cmd.Name, "target_type", setup.Target.Type)

	return err
}
