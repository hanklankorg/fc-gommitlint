// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/githook"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

var errPrePushSelectionTest = errors.New("low-level graph detail")

func newPrePushTestRoot(reader io.Reader, stdout, stderr io.Writer, repoPath string) *cli.Command {
	return &cli.Command{
		Name:      "gommitlint",
		Flags:     cliAdapter.NewGlobalFlags(nil, repoPath),
		Commands:  []*cli.Command{NewHookCommand(nil)},
		Reader:    reader,
		Writer:    stdout,
		ErrWriter: stderr,
	}
}

func prePushTestArgs(options ...string) []string {
	args := append([]string{"gommitlint"}, options...)

	return append(args, commandNameHook, commandNameHookRun, commandNameHookPrePush, "upstream", "location")
}

func runPrePushGitIn(t *testing.T, repoPath string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", repoPath}, args...)...)
	// An inherited GIT_DIR (a hook or rebase --exec) would redirect the fixture into another repository.
	environment := slices.DeleteFunc(os.Environ(), func(entry string) bool { return strings.HasPrefix(entry, "GIT_") })
	environment = append(environment,
		"GIT_CONFIG_GLOBAL="+filepath.Join(t.TempDir(), "gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
	cmd.Env = environment
	outputBytes, err := cmd.CombinedOutput()
	check.NoError(t, err, string(outputBytes))

	return strings.TrimSpace(string(outputBytes))
}

func TestPrePushCommand_RejectsMalformedGitProtocol(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	root := &cli.Command{
		Name:      "gommitlint",
		Commands:  []*cli.Command{NewHookCommand(nil)},
		Reader:    strings.NewReader("not a ref update\n"),
		Writer:    &stdout,
		ErrWriter: &stderr,
	}

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, "run", "pre-push", "upstream", "location"})

	check.ErrorIs(t, err, githook.ErrMalformedPrePushProtocol)
	check.Equal(t, "invalid pre-push input", err.Error())
	check.Empty(t, stdout.String())

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonInvalidCommit, validationErr.Diagnostic.Reason)
}

func TestPrePushCommand_DisabledSignatureRuleSkipsRepositoryWork(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "gommitlint.yaml")
	check.NoError(t, os.WriteFile(configPath, []byte("gommitlint:\n  rules:\n    disabled:\n      - cryptosignature\n"), 0o600))

	oid := strings.Repeat("a", 40)
	remoteOID := strings.Repeat("b", 40)
	input := "refs/heads/main " + oid + " refs/heads/main " + remoteOID + "\n"

	var stdout, stderr bytes.Buffer

	root := &cli.Command{
		Name: "gommitlint",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flags.Config},
			&cli.StringFlag{Name: flags.RepoPath},
			&cli.BoolFlag{Name: flags.Quiet},
		},
		Commands:  []*cli.Command{NewHookCommand(nil)},
		Reader:    strings.NewReader(input),
		Writer:    &stdout,
		ErrWriter: &stderr,
	}

	err := root.Run(t.Context(), []string{
		"gommitlint",
		"--" + flags.Config, configPath,
		"--" + flags.RepoPath, filepath.Join(tempDir, "missing-repository"),
		commandNameHook, "run", "pre-push", "upstream", "location",
	})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.Empty(t, stderr.String())
}

// cancelOnRead cancels the run's context the moment the pre-push stream is
// read: after the hook's entry guard and before the guards that follow the
// config load and the commit selection, so each later guard is the only one
// that can report the cancellation.
type cancelOnRead struct {
	io.Reader

	cancel context.CancelFunc
}

func (r cancelOnRead) Read(buffer []byte) (int, error) {
	r.cancel()

	return r.Reader.Read(buffer)
}

// TestPrePushCommand_CancellationSurfacesAtEveryNoWorkExit reaches each of the
// hook's three cancellation checks separately. A context cancelled before the
// run stops it before the stream is read, which is why a malformed stream must
// still report the cancellation rather than a protocol error. The other rows
// cancel on the first read, so the entry guard has already passed and only
// the guard after the disabled rule, or the one after an empty commit
// selection, can turn the exit into context.Canceled.
func TestPrePushCommand_CancellationSurfacesAtEveryNoWorkExit(t *testing.T) {
	repoPath := t.TempDir()
	runPrePushGitIn(t, repoPath, "init", "-q")

	configPath := filepath.Join(t.TempDir(), "gommitlint.yaml")
	check.NoError(t, os.WriteFile(configPath, []byte("gommitlint:\n  rules:\n    disabled:\n      - cryptosignature\n"), 0o600))

	oid := strings.Repeat("a", 40)
	zero := strings.Repeat("0", 40)
	tests := []struct {
		name            string
		input           string
		options         []string
		cancelBeforeRun bool
	}{
		{
			name:            "before_the_stream_is_read",
			input:           "not a ref update\n",
			options:         []string{"--ignore-config"},
			cancelBeforeRun: true,
		},
		{
			name:    "after_the_config_disables_the_rule",
			input:   "refs/heads/main " + oid + " refs/heads/main " + oid + "\n",
			options: []string{"--config", configPath},
		},
		{
			name:    "after_the_selection_finds_no_commits",
			input:   "(delete) " + zero + " refs/heads/main " + oid + "\n",
			options: []string{"--ignore-config"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			var reader io.Reader = strings.NewReader(testCase.input)
			if testCase.cancelBeforeRun {
				cancel()
			} else {
				reader = cancelOnRead{Reader: reader, cancel: cancel}
			}

			root := newPrePushTestRoot(reader, io.Discard, io.Discard, repoPath)
			err := root.Run(ctx, prePushTestArgs(testCase.options...))

			check.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestPrePushCommand_QuietSuppressesSuccessfulValidation(t *testing.T) {
	repoPath := t.TempDir()
	runPrePushGitIn(t, repoPath, "init", "-q")
	check.NoError(t, os.WriteFile(filepath.Join(repoPath, "change.txt"), []byte("change\n"), 0o600))
	runPrePushGitIn(t, repoPath, "add", "change.txt")
	runPrePushGitIn(t, repoPath, "commit", "-q", "-m", "feat: test pre-push")

	localOID := runPrePushGitIn(t, repoPath, "rev-parse", "HEAD")
	zero := strings.Repeat("0", len(localOID))
	input := "refs/heads/main " + localOID + " refs/heads/main " + zero + "\n"

	var stdout, stderr bytes.Buffer

	root := newPrePushTestRoot(strings.NewReader(input), &stdout, &stderr, repoPath)

	err := root.Run(t.Context(), prePushTestArgs("--ignore-config", "--quiet"))

	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.Empty(t, stderr.String())
}

func TestPrePushOutputOptions_PropagatesQuiet(t *testing.T) {
	var quiet bool

	root := &cli.Command{
		Name:      "gommitlint",
		Flags:     cliAdapter.NewGlobalFlags(nil, ""),
		Writer:    io.Discard,
		ErrWriter: io.Discard,
		Action: func(_ context.Context, cmd *cli.Command) error {
			cfgResult, err := loadDefaultConfig()
			if err != nil {
				return err
			}

			options, err := prePushOutputOptions(cmd, cfgResult, nil)
			quiet = options.Quiet

			return err
		},
	}

	err := root.Run(t.Context(), []string{"gommitlint", "--quiet"})

	check.NoError(t, err)
	check.True(t, quiet)
}

func TestPrePushCommand_ExistingNonRepositoryUsesInvalidRepositoryRecovery(t *testing.T) {
	nonRepoPath := t.TempDir()
	oid := strings.Repeat("a", 40)
	input := "refs/heads/main " + oid + " refs/heads/main " + oid + "\n"
	root := newPrePushTestRoot(strings.NewReader(input), io.Discard, io.Discard, nonRepoPath)

	err := root.Run(t.Context(), prePushTestArgs("--ignore-config"))

	check.ErrorIs(t, err, cliAdapter.ErrInvalidRepoPath)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	suggestions := strings.Join(recoveryErr.Suggestions, "\n")
	check.Contains(t, suggestions, "Initialize git repository")
	check.NotContains(t, suggestions, "Fetch the remote")
}

func TestPrePushCommand_ValidatesPresentationBeforeDisabledRuleShortCircuit(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "gommitlint.yaml")
	check.NoError(t, os.WriteFile(configPath, []byte("gommitlint:\n  rules:\n    disabled:\n      - cryptosignature\n"), 0o600))

	oid := strings.Repeat("a", 40)
	input := "refs/heads/main " + oid + " refs/heads/main " + oid + "\n"
	tests := []struct {
		name string
		arg  string
		want error
	}{
		{name: "theme", arg: "--theme=invalid", want: ErrUnsupportedTheme},
		{name: "color", arg: "--color=invalid", want: ErrUnsupportedColor},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := newPrePushTestRoot(strings.NewReader(input), io.Discard, io.Discard, filepath.Join(t.TempDir(), "missing-repo"))
			err := root.Run(t.Context(), prePushTestArgs("--config", configPath, testCase.arg))

			check.ErrorIs(t, err, testCase.want)
		})
	}
}

func TestPrePushSelectionError_KeepsLowLevelCauseOutOfLocalizedMessage(t *testing.T) {
	t.Parallel()

	err := prePushSelectionError(errPrePushSelectionTest, output.EnsureLocalizer(nil))

	check.ErrorIs(t, err, errPrePushSelectionTest)
	check.NotContains(t, err.Error(), errPrePushSelectionTest.Error())
}

func TestPrePushSelectionError_ReturnsContextErrorsUnchanged(t *testing.T) {
	t.Parallel()

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		got := prePushSelectionError(want, output.EnsureLocalizer(nil))

		check.Equal(t, want, got)
	}
}
