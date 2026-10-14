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
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestTranslationsActions_RejectUnexpectedArgumentsBeforeDiscovery(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("PATH", t.TempDir())

	commands := []struct {
		name string
		cmd  *cli.Command
	}{
		{name: "update", cmd: NewTranslationsUpdateCommand(nil)},
		{name: "compile", cmd: NewTranslationsCompileCommand(nil)},
	}

	removedDir := filepath.Join(t.TempDir(), "removed")
	check.NoError(t, os.Mkdir(removedDir, 0o750))
	t.Chdir(removedDir)
	check.NoError(t, os.Remove(removedDir))

	for _, testCase := range commands {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.cmd.Run(t.Context(), []string{testCase.name, "unexpected"})

			check.ErrorIs(t, err, ErrUnexpectedArgument)
			check.NotContains(t, err.Error(), "locate repo root")

			var recoveryErr RecoveryError
			check.ErrorAs(t, err, &recoveryErr)
		})
	}
}

func TestTranslationsActions_RejectUnsafeDomainsBeforePrerequisites(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("PATH", t.TempDir())

	root := t.TempDir()
	localeDir := filepath.Join(root, "locale")
	outputDir := filepath.Join(root, "generated", "locale")

	check.NoError(t, os.MkdirAll(localeDir, 0o750))

	testCases := []struct {
		name   string
		domain string
	}{
		{name: "empty", domain: ""},
		{name: "dot", domain: "."},
		{name: "dotdot", domain: ".."},
		{name: "slash", domain: "../escape"},
		{name: "backslash", domain: `..\escape`},
		{name: "absolute", domain: "/tmp/escape"},
		{name: "drive_relative", domain: "C:escape"},
		{name: "drive_absolute", domain: `C:\escape`},
		{name: "nul", domain: "bad\x00name"},
		{name: "control", domain: "bad\nname"},
		{name: "unicode_control", domain: "bad\u0085name"},
	}

	commands := []struct {
		name string
		run  func(string) error
	}{
		{
			name: "update",
			run: func(domain string) error {
				return NewTranslationsUpdateCommand(nil).Run(t.Context(), []string{
					"update", "--locale-dir", localeDir, "--domain", domain,
				})
			},
		},
		{
			name: "compile",
			run: func(domain string) error {
				return NewTranslationsCompileCommand(nil).Run(t.Context(), []string{
					"compile", "--source-dir", localeDir, "--output-dir", outputDir, "--domain", domain,
				})
			},
		},
	}

	for _, command := range commands {
		for _, testCase := range testCases {
			t.Run(command.name+"_"+testCase.name, func(t *testing.T) {
				err := command.run(testCase.domain)

				check.Error(t, err)
				check.Contains(t, err.Error(), "invalid translation domain")
				check.ErrorIs(t, err, ErrInvalidTranslationDomain)
				check.False(t, errors.Is(err, errMissingTranslationDependency), "domain validation must precede tool lookup")

				var recoveryErr RecoveryError
				check.ErrorAs(t, err, &recoveryErr)
				check.NotEmpty(t, recoveryErr.Suggestions)
			})
		}
	}
}

func TestValidateTranslationDomain_AcceptsSafeStemsWithoutLookup(t *testing.T) {
	t.Parallel()

	for _, domain := range []string{"gommitlint", "messages.v2", ".hidden", "domain-name_1"} {
		t.Run(domain, func(t *testing.T) {
			t.Parallel()

			lookedUp := false
			err := validateTranslationDomain(domain, func() output.Localizer {
				lookedUp = true

				return nil
			})

			check.NoError(t, err)
			check.False(t, lookedUp, "validating a safe domain must not perform prerequisite lookup")
		})
	}
}

type translationTestLocalizer struct{}

func (translationTestLocalizer) Get(message string, args ...any) string {
	return "localized: " + fmt.Sprintf(message, args...)
}

func (translationTestLocalizer) GetN(singular string, _ string, _ int, args ...any) string {
	return "localized: " + fmt.Sprintf(singular, args...)
}

func TestValidateTranslationDomain_LocalizesRecoverySuggestions(t *testing.T) {
	t.Parallel()

	err := validateTranslationDomain("..", func() output.Localizer {
		return translationTestLocalizer{}
	})

	check.ErrorIs(t, err, ErrInvalidTranslationDomain)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.Len(t, recoveryErr.Suggestions, 2)

	for _, suggestion := range recoveryErr.Suggestions {
		check.True(t, strings.HasPrefix(suggestion, "localized: "))
	}

	check.Contains(t, recoveryErr.Format(translationTestLocalizer{}), "localized: invalid translation domain")
}

func TestTranslationsActions_UnsafeDomainsCannotWriteOutsideRoots(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("PATH", t.TempDir())

	root := t.TempDir()
	localeDir := filepath.Join(root, "locale")
	sourceDir := filepath.Join(root, "a", "b", "source")
	outputDir := filepath.Join(root, "output")
	check.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "sv"), 0o750))
	check.NoError(t, os.MkdirAll(filepath.Join(root, "outside"), 0o750))
	check.NoError(t, os.MkdirAll(localeDir, 0o750))

	t.Run("update_report", func(t *testing.T) {
		existing := filepath.Join(root, "outside", "existing-report.pot")
		missing := filepath.Join(root, "outside", "new-report.pot")
		assertTranslationTargetPreserved(t, existing, missing, func(domain string) error {
			return NewTranslationsUpdateCommand(nil).Run(t.Context(), []string{
				"update", "--locale-dir", localeDir, "--domain", domain,
			})
		}, "../outside/existing-report", "../outside/new-report")
	})

	t.Run("compiled_catalog", func(t *testing.T) {
		existing := filepath.Join(root, "outside", "existing-catalog.mo")
		missing := filepath.Join(root, "outside", "new-catalog.mo")
		assertTranslationTargetPreserved(t, existing, missing, func(domain string) error {
			return NewTranslationsCompileCommand(nil).Run(t.Context(), []string{
				"compile", "--source-dir", sourceDir, "--output-dir", outputDir, "--domain", domain,
			})
		}, "../../../outside/existing-catalog", "../../../outside/new-catalog")
	})
}

func assertTranslationTargetPreserved(
	t *testing.T,
	existing string,
	missing string,
	run func(string) error,
	existingDomain string,
	missingDomain string,
) {
	t.Helper()

	const original = "must remain unchanged"
	check.NoError(t, os.WriteFile(existing, []byte(original), 0o600))

	err := run(existingDomain)
	check.ErrorIs(t, err, ErrInvalidTranslationDomain)

	content, err := os.ReadFile(existing)
	check.NoError(t, err)
	check.Equal(t, original, string(content))

	err = run(missingDomain)
	check.ErrorIs(t, err, ErrInvalidTranslationDomain)

	_, err = os.Stat(missing)
	check.ErrorIs(t, err, os.ErrNotExist)
}
