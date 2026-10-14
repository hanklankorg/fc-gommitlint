// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

var (
	errMissingTranslationDependency = errors.New("missing translation dependency")
	errSourceDirNotFound            = errors.New("source directory not found")
	errNoTranslationSources         = errors.New("no translation sources found")
	errLocaleDirNotFound            = errors.New("locale directory not found")
	errNoMsgIDHeader                = errors.New("no msgid header found")
	errMalformedPOHeader            = errors.New("malformed PO header")
	errGommitlintModuleNotFound     = errors.New("gommitlint module marker not found")
	errTranslationCompile           = errors.New("translation compile failed")
	errTranslationUpdate            = errors.New("translation update failed")
	errTranslationPathSymlink       = errors.New("translation path contains a symlink")
	errTranslationPathNotDirectory  = errors.New("translation path is not a directory")
	errTranslationPathNotRegular    = errors.New("translation path is not a regular file")
)

// ErrInvalidTranslationDomain identifies an unsafe translation filename stem.
var ErrInvalidTranslationDomain = errors.New("invalid translation domain")

func rejectUnexpectedTranslationArgs(cmd *cli.Command, command string) error {
	args := cmd.Args()
	if args == nil || args.Len() == 0 {
		return nil
	}

	localizer := commandLocalizer()

	return rejectUnexpectedArgs(
		cmd,
		localizer,
		command,
		localizer.Get("This command does not accept positional arguments"),
	)
}

func validateTranslationDomain(domain string, localizer func() output.Localizer) error {
	if domain != "" &&
		domain != "." &&
		domain != ".." &&
		!filepath.IsAbs(domain) &&
		filepath.VolumeName(domain) == "" &&
		!hasDrivePrefix(domain) &&
		!strings.ContainsAny(domain, `/\`) &&
		!strings.ContainsFunc(domain, unicode.IsControl) {
		return nil
	}

	loc := output.EnsureLocalizer(nil)
	if localizer != nil {
		loc = output.EnsureLocalizer(localizer())
	}

	return newRecoveryErrorWithCause(
		ErrInvalidTranslationDomain,
		output.Msg("invalid translation domain %q: expected one safe filename stem", domain),
		loc.Get("Use a filename stem without path separators, for example: --domain gommitlint"),
		loc.Get("Do not use '.', '..', absolute paths, drive prefixes, or control characters"),
	)
}

func hasDrivePrefix(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}

	return value[0] >= 'A' && value[0] <= 'Z' || value[0] >= 'a' && value[0] <= 'z'
}

func missingTranslationDependencyError(tool string, cause error, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(errMissingTranslationDependency, cause),
		output.Msg("required translation tool %q was not found", tool),
		localizer.Get("Install GNU gettext (apt install gettext / brew install gettext)"),
		localizer.Get("Ensure that name is on your PATH when invoking the command."),
	)
}

func translationSourceDirError(path string, cause error, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(errSourceDirNotFound, cause),
		output.Msg("translation source directory is unavailable: %s", path),
		localizer.Get("Check the source directory path and permissions"),
		helpSuggestion(localizer, "gommitlint translations compile"),
	)
}

func translationLocaleDirError(path string, cause error, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(errLocaleDirNotFound, cause),
		output.Msg("translation locale directory is unavailable: %s", path),
		localizer.Get("Check the locale directory path, type, and permissions"),
		helpSuggestion(localizer, "gommitlint translations update"),
	)
}

func translationUpdateError(cause error, localizer output.Localizer) error {
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}

	if _, ok := errors.AsType[RecoveryError](cause); ok {
		return cause
	}

	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(errTranslationUpdate, cause),
		output.Msg("failed to update translation catalogs: %s", cause),
		localizer.Get("Fix the reported catalog, path, or gettext error, then rerun the update"),
		helpSuggestion(localizer, "gommitlint translations update"),
	)
}

func translationCompileError(cause error, localizer output.Localizer) error {
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}

	if _, ok := errors.AsType[RecoveryError](cause); ok {
		return cause
	}

	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(errTranslationCompile, cause),
		output.Msg("failed to compile translation catalogs: %s", cause),
		localizer.Get("Fix the reported catalog or gettext error, then rerun the compile"),
		helpSuggestion(localizer, "gommitlint translations compile"),
	)
}

func noTranslationSourcesError(sourceDir, domain string, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errNoTranslationSources,
		output.Msg("no regular %s.po translation catalogs found under %s", domain, sourceDir),
		localizer.Get("Add a regular %s.po file under a real locale directory", domain),
		helpSuggestion(localizer, "gommitlint translations compile"),
	)
}

func translationOutputWriteError(cause error, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		errors.Join(ErrOutputWrite, cause),
		output.Msg("failed to write command output"),
		localizer.Get("Check output destination"),
	)
}

func translationSymlinkError(path string) error {
	return fmt.Errorf("%w: %s", errTranslationPathSymlink, path)
}
