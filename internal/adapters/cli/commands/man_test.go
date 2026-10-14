// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestNewManCommand_Properties(t *testing.T) {
	t.Parallel()

	cmd := NewManCommand(nil)

	check.Equal(t, commandNameMan, cmd.Name)
	check.False(t, cmd.Hidden, "man command must be visible in help")
	check.NotNil(t, cmd.Action, "man command must have an action")
}

// TestExecuteMan_RendersManualFromCheckout asserts that an explicitly trusted
// source checkout writes the manual to stdout and exits 0.
func TestExecuteMan_RendersManualFromCheckout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	manualPath := filepath.Join(root, manDocPath)
	manual := "GOMMITLINT 1\ntrusted manual\n"

	check.NoError(t, os.MkdirAll(filepath.Dir(manualPath), 0o750))
	check.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(manModuleLine+"\n"), 0o600))
	check.NoError(t, os.WriteFile(manualPath, []byte(manual), 0o600))

	var stdout, stderr bytes.Buffer

	cmd := &cli.Command{
		Name:      commandNameMan,
		Writer:    &stdout,
		ErrWriter: &stderr,
		Metadata: map[string]any{
			manDocCandidatesMetadataKey: []string{manualPath},
		},
	}

	err := ExecuteMan(t.Context(), cmd)
	check.NoError(t, err, "rendering the manual from a checkout must succeed")
	check.Equal(t, manual, stdout.String(), "man command must render the trusted manual")
	check.Empty(t, stderr.String(), "no warning when the manual source resolves")
}

// TestExecuteMan_WhenSourceMissing_WarnsOnStderrAndSucceeds asserts the
// in-terminal-manual ruling: a missing manual source warns on stderr and
// still exits 0 (never fails the run).
func TestExecuteMan_WhenSourceMissing_WarnsOnStderrAndSucceeds(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer

	cmd := &cli.Command{
		Name:      commandNameMan,
		Writer:    &stdout,
		ErrWriter: &stderr,
		Metadata: map[string]any{
			manDocCandidatesMetadataKey: []string{filepath.Join(t.TempDir(), manDocPath)},
		},
	}

	err := ExecuteMan(t.Context(), cmd)
	check.NoError(t, err, "a missing manual source must not fail the command")
	check.Empty(t, stdout.String(), "nothing written to stdout when the source is missing")
	check.Contains(t, stderr.String(), "man gommitlint", "warning must point at man(1)")
	check.Contains(t, stderr.String(), manDocumentationURL, "warning must point at the online manual")
}

func TestExecuteMan_MissingSourcePropagatesWarningWriteFailure(t *testing.T) {
	t.Parallel()

	errWrite := errMockWrite
	cmd := &cli.Command{
		Name:      commandNameMan,
		Writer:    &bytes.Buffer{},
		ErrWriter: failingWriter{err: errWrite},
		Metadata: map[string]any{
			manDocCandidatesMetadataKey: []string{filepath.Join(t.TempDir(), manDocPath)},
		},
	}

	err := ExecuteMan(t.Context(), cmd)
	check.ErrorIs(t, err, errWrite)
	check.ErrorIs(t, err, ErrOutputWrite)
}

func TestExecuteMan_PropagatesSubstantiveManualReadError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	check.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module codefloe.com/itiquette/gommitlint\n"), 0o600))
	check.NoError(t, os.MkdirAll(filepath.Join(root, manDocPath), 0o750))

	var stdout, stderr bytes.Buffer

	cmd := &cli.Command{
		Name:      commandNameMan,
		Writer:    &stdout,
		ErrWriter: &stderr,
		Metadata: map[string]any{
			manDocCandidatesMetadataKey: []string{filepath.Join(root, manDocPath)},
		},
	}
	err := ExecuteMan(t.Context(), cmd)

	check.ErrorIs(t, err, errManualSourceNotRegular)
	check.Empty(t, stdout.String())
	check.Empty(t, stderr.String())
}

func TestExecuteMan_IgnoresSpoofedWorkingDirectoryCheckout(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	manualPath := filepath.Join(root, manDocPath)
	check.NoError(t, os.MkdirAll(filepath.Dir(manualPath), 0o750))
	check.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte(manModuleLine+"\n"), 0o600))
	check.NoError(t, os.WriteFile(manualPath, []byte("untrusted manual"), 0o600))

	var stdout, stderr bytes.Buffer

	cmd := &cli.Command{Name: commandNameMan, Writer: &stdout, ErrWriter: &stderr}

	err := ExecuteMan(t.Context(), cmd)
	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.NotContains(t, stderr.String(), "untrusted manual")
	check.Contains(t, stderr.String(), manDocumentationURL)
}

func TestExecuteMan_PreCancelledContextStopsBeforeReading(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr bytes.Buffer

	read := false
	cmd := &cli.Command{Name: commandNameMan, Writer: &stdout, ErrWriter: &stderr}
	err := executeMan(ctx, cmd, func() (string, error) {
		read = true

		return "manual", nil
	})

	check.ErrorIs(t, err, context.Canceled)
	check.False(t, read, "a pre-cancelled command must not read the manual")
	check.Empty(t, stdout.String())
	check.Empty(t, stderr.String())
}

func TestExecuteMan_ContextCancelledDuringReadWritesNoOutput(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())

	var stdout, stderr bytes.Buffer

	cmd := &cli.Command{Name: commandNameMan, Writer: &stdout, ErrWriter: &stderr}
	err := executeMan(ctx, cmd, func() (string, error) {
		cancel()

		return "manual", nil
	})

	check.ErrorIs(t, err, context.Canceled)
	check.Empty(t, stdout.String())
	check.Empty(t, stderr.String())
}
