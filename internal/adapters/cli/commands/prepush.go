// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/gitcli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/githook"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

const (
	commandNameHookRun     = "run"
	commandNameHookPrePush = "pre-push"
)

// newHookRunCommand creates the hidden entry point used by generated hooks.
func newHookRunCommand(localizer output.Localizer) *cli.Command {
	return &cli.Command{
		Name:   commandNameHookRun,
		Hidden: true,
		Commands: []*cli.Command{
			{
				Name:   commandNameHookPrePush,
				Hidden: true,
				Action: executePrePushHook(localizer),
			},
		},
	}
}

func executePrePushHook(localizer output.Localizer) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		localizer = output.EnsureLocalizer(localizer)

		if err := ctx.Err(); err != nil {
			return err
		}

		if os.Getenv("GOMMITLINT_DEBUG") != "" {
			ctx = logadapter.InitLogger(ctx, output.FormatText, logadapter.LogLevelDebugName)
		}

		reader := cmd.Reader
		if reader == nil {
			reader = os.Stdin
		}

		updates, err := githook.DecodePrePush(commandArgs(cmd), reader)
		if err != nil {
			return prePushProtocolError(err)
		}

		cfgResult, err := LoadConfigFromCommand(cmd.Root())
		if err != nil {
			return configLoadError(err, localizer)
		}

		if _, err := resolveTheme(cmd, cfgResult.Config, localizer); err != nil {
			return err
		}

		if _, err := resolveColor(cmd, localizer); err != nil {
			return err
		}

		if err := showConfigWarnings(cmd.ErrWriter, cmd.Root().Bool(flags.Quiet), configWarningDetails(cfgResult.Warnings), localizer); err != nil {
			return err
		}

		if !rulefactory.IsCommitRuleEnabled(cfgResult.Config, nil, rulecatalog.CryptoSignature) {
			return ctx.Err()
		}

		requestedRepoPath := GetRepoPath(cmd)

		repoPath, err := cliAdapter.ValidateRepoPath(requestedRepoPath)
		if err != nil {
			return invalidRepositoryError(err, localizer)
		}

		if repoPath == "" {
			err = fmt.Errorf("%w: %w", cliAdapter.ErrInvalidRepoPath, domain.ErrNotGitRepository)

			return invalidRepositoryError(err, localizer)
		}

		commits, err := apphooks.SelectPrePushCommits(ctx, updates, gitcli.NewPrePushResolver(repoPath))
		if err != nil {
			return prePushSelectionError(err, localizer)
		}

		if len(commits) == 0 {
			return ctx.Err()
		}

		return validatePrePushCommits(ctx, cmd, commits, cfgResult, localizer)
	}
}

func commandArgs(cmd *cli.Command) []string {
	args := make([]string, cmd.Args().Len())
	for index := range args {
		args[index] = cmd.Args().Get(index)
	}

	return args
}

func validatePrePushCommits(ctx context.Context, cmd *cli.Command, commits []string, cfgResult ConfigResult, localizer output.Localizer) error {
	logger := logadapter.GetDomainLogger(ctx)

	repo, err := createRepositoryAdapter(ctx, cmd, logger, localizer)
	if err != nil {
		return err
	}

	options, err := prePushOutputOptions(cmd, cfgResult, localizer)
	if err != nil {
		return err
	}

	service := appValidation.Service{
		BuildRules: func(cfg configTypes.Config, ruleFilter []string, excludeRules []string, baseBranch string) (appValidation.Rules, error) {
			commitRules, repoRules, buildErr := buildRules(cfg, logger, ruleFilter, excludeRules, baseBranch, localizer)
			if buildErr != nil {
				return appValidation.Rules{}, buildErr
			}

			return appValidation.Rules{Commit: commitRules, Repository: repoRules}, nil
		},
		Logger: logger,
		OpenRepository: func(context.Context) (domain.Repository, error) {
			return repo, nil
		},
	}

	target := appValidation.CommitsTarget(commits)

	report, validateErr := service.Validate(ctx, appValidation.Request{
		Config:          cfgResult.Config,
		ConfigSources:   configSourcePaths(cfgResult.Sources),
		IgnoreRepoRules: true,
		RuleFilter:      []string{rulecatalog.CryptoSignature},
		Target:          target,
		WarningDetails:  configWarningDetails(cfgResult.Warnings),
	})
	if validateErr != nil {
		setup := ValidationSetup{Config: cfgResult.Config, Target: target}

		return mapValidationServiceError(validateErr, setup, options, localizer)
	}

	if len(report.Commits) == 0 || report.Summary.ValidatedCommits == 0 {
		return nil
	}

	if !report.Summary.AllPassed {
		if err := writeValidationReport(logger, report, options, localizer); err != nil {
			return err
		}

		return errors.Join(
			ErrValidationReportWritten,
			domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonValidationFailed, generateFailureMessage(report, localizer)),
		)
	}

	if options.Quiet {
		return nil
	}

	return writeCommandOutput(cmd.ErrWriter, localizer.Get("✓ All checks passed")+"\n", localizer)
}

func prePushOutputOptions(cmd *cli.Command, cfgResult ConfigResult, localizer output.Localizer) (cliAdapter.OutputOptions, error) {
	theme, err := resolveTheme(cmd, cfgResult.Config, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	color, err := resolveColor(cmd, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	options := cliAdapter.NewOutputOptions(cmd.ErrWriter).
		WithFormat(output.FormatText).
		WithColor(color).
		WithTheme(theme).
		WithQuiet(cmd.Root().Bool(flags.Quiet)).
		WithLocalizer(localizer).
		WithResolvedEnvironment(os.Getenv).
		WithToolVersion(toolVersionFrom(cmd))
	display := cfgResult.Display(currentConfigDisplayEnv(), localizer)
	options.ConfigSources = display.Sources
	options.ConfigSourcesUnexpected = display.Unexpected

	return options, nil
}

func prePushProtocolError(err error) error {
	cause := errors.Join(
		domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonInvalidCommit, err.Error()),
		err,
	)

	return newRecoveryErrorWithCause(cause, output.Msg("invalid pre-push input"))
}

func prePushSelectionError(err error, localizer output.Localizer) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	cause := errors.Join(
		domain.New(diagnostics.SourceIDValidation, diagnostics.ReasonGitOperationFailed, err.Error()),
		err,
	)

	return newRecoveryErrorWithCause(
		cause,
		output.Msg("pre-push commit selection failed"),
		localizer.Get("Fetch the remote and retry the push"),
	)
}
