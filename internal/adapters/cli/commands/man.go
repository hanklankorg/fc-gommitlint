// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

const (
	commandNameMan = "man"

	// manDocPath is the repo-relative location of the top-level manual
	// source. It is the same markdown go-md2man compiles into the installed
	// gommitlint.1.gz, so `gommitlint man` shows the same content as
	// `man gommitlint` without requiring man(1) to be installed.
	manDocPath = "docs/manpages/gommitlint.1.md"

	// manDocumentationURL is the rendered online manual, shown when the local
	// manual source is unavailable (i.e. a packaged install, not a checkout).
	manDocumentationURL = "https://docs.itiquette.org/gommitlint/"
	manModuleLine       = "module codefloe.com/itiquette/gommitlint"

	manDocCandidatesMetadataKey = "gommitlint.manDocCandidates"
)

// NewManCommand creates the 'man' subcommand, which prints the bundled manual
// page to the terminal for users who do not have man(1) installed.
func NewManCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameMan,
		Usage: localizer.Get("show the terminal manual"),
		Description: localizer.Get(`Prints the gommitlint manual to standard output.

This is a convenience for environments where man(1) is not installed. The
content is identical to 'man gommitlint'. The manual source ships only in a
source checkout; from a packaged install use 'man gommitlint' or see the
online manual.`),
		Action: ExecuteMan,
	}
}

// manDocCandidatePaths returns trusted locations to probe for the manual source.
//
// The manual source is only present in a development checkout: packaged
// installations ship the compiled man page (gommitlint.1.gz under
// /usr/share/man/man1 or ~/.local/share/man/man1) and never this markdown
// file, so there is no installed location to derive. Source checkouts are
// discovered only relative to the executable; the working directory is
// caller-controlled and must not be treated as a checkout trust anchor.
func manDocCandidatePaths() []string {
	exePath, err := os.Executable()
	if err != nil || !filepath.IsAbs(exePath) {
		return nil
	}

	exeDir := filepath.Dir(filepath.Clean(exePath))

	return []string{
		filepath.Join(exeDir, manDocPath),       // binary at repo root
		filepath.Join(exeDir, "..", manDocPath), // binary in <repo>/bin
	}
}

func manDocCandidatePathsForCommand(cmd *cli.Command) []string {
	if cmd != nil && cmd.Metadata != nil {
		if paths, ok := cmd.Metadata[manDocCandidatesMetadataKey].([]string); ok {
			return paths
		}
	}

	return manDocCandidatePaths()
}

// getManContent returns the manual markdown from the first candidate path that
// resolves. Resolution is lazy: it only runs inside the man action, so a
// missing source never affects any other command.
func getManContent(cmd *cli.Command) (string, error) {
	return getManContentFromCandidates(manDocCandidatePathsForCommand(cmd))
}

func getManContentFromCandidates(paths []string) (string, error) {
	for _, path := range paths {
		content, found, err := readTrustedManCandidate(path)
		if err != nil {
			return "", err
		}

		if found {
			return content, nil
		}
	}

	return "", fmt.Errorf("%w: %s", ErrManualSourceNotFound, manDocPath)
}

func readTrustedManCandidate(path string) (string, bool, error) {
	if !filepath.IsAbs(path) {
		return "", false, nil
	}

	absPath := filepath.Clean(path)

	info, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, err
	}

	if !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("%w: %s", errManualSourceNotRegular, absPath)
	}

	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(absPath)))

	moduleData, err := os.ReadFile(filepath.Join(repoRoot, "go.mod")) //nolint:gosec // The executable-relative or explicitly injected candidate selects a fixed checkout marker.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}

		return "", false, err
	}

	if !hasModuleLine(string(moduleData), manModuleLine) {
		return "", false, nil
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return "", false, err
	}

	return string(content), true, nil
}

func hasModuleLine(content, expected string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}

	return false
}

// ExecuteMan prints the manual, or a short pointer to man(1) and the online
// manual when the source is not available locally. A missing manual source is
// not a failure: the command warns on stderr and exits 0 (per the
// CLI-conventions in-terminal-manual ruling — a missing manual never fails a
// run).
func ExecuteMan(ctx context.Context, cmd *cli.Command) error {
	return executeMan(ctx, cmd, func() (string, error) {
		return getManContent(cmd)
	})
}

func executeMan(ctx context.Context, cmd *cli.Command, readContent func() (string, error)) error {
	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint man", localizer.Get("The 'man' command doesn't accept arguments")); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	content, err := readContent()
	if err != nil {
		if !errors.Is(err, ErrManualSourceNotFound) {
			return newRecoveryErrorWithCause(
				err,
				output.Msg("failed to read terminal manual"),
				localizer.Get("Try 'man gommitlint' if the man page is installed"),
				localizer.Get("See the online manual: %s", manDocumentationURL),
			)
		}

		warning := localizer.Get("Warning: the terminal manual source (%s) is not available outside a source checkout.", manDocPath) + "\n" +
			localizer.Get("Try 'man gommitlint' if the man page is installed, or see: %s", manDocumentationURL) + "\n"

		return writeCommandOutput(cmd.ErrWriter, warning, localizer)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return writeCommandOutput(cmd.Writer, content, localizer)
}
