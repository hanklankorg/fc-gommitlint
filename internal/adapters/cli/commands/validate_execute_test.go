// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

var (
	errValidationReportClose = errors.New("close report")
	errRuleHelpWrite         = errors.New("write rule help")
)

func TestExecuteValidationAndReport_MessageFileSkipsMissingRepository(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	messagePath := filepath.Join(tmpDir, "COMMIT_EDITMSG")
	check.NoError(t, os.WriteFile(messagePath, []byte("feat: add test\n"), 0o600))

	var outputBuffer bytes.Buffer

	cmd := &cli.Command{Name: commandNameValidate, Writer: &outputBuffer}
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: "repo-path", Value: tmpDir},
		&cli.BoolFlag{Name: flagNameIgnoreRepoRules},
	}

	target, err := cliAdapter.NewValidationTarget(messagePath, "", "", "", 1, settings.DefaultMaxCommitCount)
	check.NoError(t, err)

	cfg := settings.Config{
		Rules: settings.RulesConfig{
			Enabled: []string{
				rulecatalog.Subject,
				rulecatalog.Conventional,
			},
			Disabled: []string{
				rulecatalog.BranchAhead,
				rulecatalog.SignOff,
				rulecatalog.Spell,
				rulecatalog.CryptoSignature,
			},
		},
	}
	outputOptions := cliAdapter.NewOutputOptions(&outputBuffer).
		WithFormat(output.FormatJSON).
		WithColor(colorModeNever)
	setup := ValidationSetup{Config: cfg, Target: target, OutputOptions: outputOptions}

	err = executeValidationAndReport(t.Context(), cmd, setup, outputOptions, nil, nil)
	check.NoError(t, err)
	check.Contains(t, outputBuffer.String(), `"allPassed": true`)
}

func TestWriteValidationReport_PropagatesCloseFailure(t *testing.T) {
	t.Parallel()

	closer := &validationErrorCloser{err: errValidationReportClose}

	var outputBuffer bytes.Buffer

	options := cliAdapter.NewOutputOptions(&outputBuffer).
		WithColor(colorModeNever).
		WithCloser(closer)

	err := writeValidationReport(&testLogger{}, domain.Report{}, options, nil)
	check.ErrorIs(t, err, errValidationReportClose)
	check.ErrorIs(t, err, ErrOutputWrite)
	check.True(t, closer.closed)
}

func TestValidateCommand_RuleHelpWriterFailurePreservesErrOutputWrite(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer

	root := &cli.Command{
		Name:      gommitlintKey,
		Writer:    failingWriter{err: errRuleHelpWrite},
		ErrWriter: &stderr,
		Commands:  []*cli.Command{NewValidateCommand(nil)},
	}

	err := root.Run(t.Context(), []string{
		gommitlintKey,
		commandNameValidate,
		"--" + flagNameRuleHelp,
		rulecatalog.Subject,
	})

	check.ErrorIs(t, err, errRuleHelpWrite)
	check.ErrorIs(t, err, ErrOutputWrite)
	check.Empty(t, stderr.String())
}

func TestValidateCommand_RuleHelpWritesOutput(t *testing.T) {
	// The assertion is on English text, so pin the locale: on a translated
	// workstation the rule help would otherwise render in that language.
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")

	var stdout, stderr bytes.Buffer

	root := &cli.Command{
		Name:      gommitlintKey,
		Writer:    &stdout,
		ErrWriter: &stderr,
		Commands:  []*cli.Command{NewValidateCommand(nil)},
	}

	err := root.Run(t.Context(), []string{
		gommitlintKey,
		commandNameValidate,
		"--" + flagNameRuleHelp,
		rulecatalog.Subject,
	})

	check.NoError(t, err)
	check.Contains(t, stdout.String(), "SUBJECT RULE")
	check.Empty(t, stderr.String())
}

func TestValidateCommand_PreCancelledRuleHelpWritesNoOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		want error
	}{
		{
			name: "cancelled",
			ctx: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				return ctx, cancel
			},
			want: context.Canceled,
		},
		{
			name: "deadline_exceeded",
			ctx: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			},
			want: context.DeadlineExceeded,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := testCase.ctx()
			defer cancel()

			var stdout, stderr bytes.Buffer

			root := &cli.Command{
				Name:      gommitlintKey,
				Writer:    &stdout,
				ErrWriter: &stderr,
				Commands:  []*cli.Command{NewValidateCommand(nil)},
			}

			err := root.Run(ctx, []string{
				gommitlintKey,
				commandNameValidate,
				"--" + flagNameRuleHelp,
				rulecatalog.Subject,
			})

			check.ErrorIs(t, err, testCase.want)
			check.Empty(t, stdout.String())
			check.Empty(t, stderr.String())
		})
	}
}

type validationErrorCloser struct {
	closed bool
	err    error
}

func (c *validationErrorCloser) Close() error {
	c.closed = true

	return c.err
}
