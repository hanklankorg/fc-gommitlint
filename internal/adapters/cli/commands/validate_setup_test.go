// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"testing"

	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/ci"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestGetRepoPath_UnsetFlagStaysEmptySoDiscoveryRuns pins the one decision
// GetRepoPath makes.
//
// Returning "." for an unset flag looks harmless and is not: it is a valid
// path, so ValidateRepoPath accepts it and never reaches its
// discover-from-CWD branch, which silently forces users to stand at the
// repository root. The empty string is what defers to discovery, so it is the
// return value worth holding still.
//
// Everything else about this function -- that a value set on the flag comes
// back out -- is urfave/cli storing a string, and asserting it tests that
// library rather than gommitlint.
func TestGetRepoPath_UnsetFlagStaysEmptySoDiscoveryRuns(t *testing.T) {
	t.Parallel()

	app := &cli.Command{}
	app.Root().Flags = []cli.Flag{&cli.StringFlag{Name: flagNameRepoPath}}

	check.Equal(t, "", GetRepoPath(app),
		`an unset --repo-path must stay empty; "." short-circuits repository discovery`)
}

func TestCreateValidationTarget_RejectsBaseBranchListWithoutNames(t *testing.T) {
	t.Parallel()

	cmd := createTestValidateCommand("", "", "", ",,,", 1, "", "")
	_, candidates, err := createValidationTarget(cmd, configTypes.Config{}, ci.Environment{})

	check.Empty(t, candidates)
	check.Error(t, err)

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonInvalidReference, validationErr.Diagnostic.Reason)
}

func TestCreateValidationTarget_PreservesCommaInCIDetectedBranch(t *testing.T) {
	t.Parallel()

	cmd := createTestValidateCommand("", "", "", "", 1, "", "")
	target, candidates, err := createValidationTarget(cmd, configTypes.Config{}, ci.Environment{
		Platform:   "github",
		BaseBranch: "release,2026",
	})

	check.NoError(t, err)
	check.Equal(t, appValidation.BaseBranchTarget("release,2026"), target)
	check.Equal(t, []string{"release,2026"}, candidates)
}
