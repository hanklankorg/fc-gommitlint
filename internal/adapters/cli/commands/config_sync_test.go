// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestConfigSyncErrorDescriptorFor_NamesEachSentinel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err     error
		message string
	}{
		{err: configuration.ErrManagedSourceNotFound, message: "managed source not found"},
		{err: configuration.ErrManagedSourceIntegrity, message: "managed source integrity check failed"},
		{err: configuration.ErrManagedSourceEnvNotSet, message: "managed source environment variable is not set"},
		{err: configuration.ErrManagedSourceInvalidURL, message: "managed source URL is invalid"},
		{err: configuration.ErrManagedSourceOutOfDate, message: "managed source is out of date"},
		{err: configuration.ErrManagedSourcePath, message: "managed source destination is invalid"},
		{err: configuration.ErrManagedSourceUnexpectedHost, message: "managed source redirect changed host"},
		{err: configuration.ErrManagedSourceFetchFailed, message: "failed to fetch managed source"},
	}

	for _, testCase := range testCases {
		check.Equal(t, testCase.message, configSyncErrorDescriptorFor(testCase.err).message.String())
	}
}

func TestConfigSyncErrorDetails_DescribesTypedErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		err         error
		message     string
		suggestions []string
	}{
		{
			name:    "integrity_mismatch",
			err:     &configuration.ManagedSourceIntegrityError{ID: testTeamPolicyID},
			message: "integrity mismatch for managed source team-policy",
			suggestions: []string{
				"Review fetched content and update integrity.sha256 only if the change is trusted",
				"gommitlint config sync --dry-run --help",
			},
		},
		{
			name: "missing_environment",
			err: &configuration.ManagedSourceEnvError{
				ConfiguredSource: "${POLICY_URL}",
				ID:               testTeamPolicyID,
				Variable:         "POLICY_URL",
			},
			message: "managed source environment variable is not set for team-policy (POLICY_URL)",
			suggestions: []string{
				"Source id: team-policy",
				"Env var: POLICY_URL",
				"Configured source: ${POLICY_URL}",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			details := configSyncErrorDetails(output.EnsureLocalizer(nil), testCase.err)
			check.Equal(t, testCase.message, details.message.String())

			joinedSuggestions := strings.Join(details.suggestions, "\n")
			for _, suggestion := range testCase.suggestions {
				check.Contains(t, joinedSuggestions, suggestion)
			}
		})
	}
}

func TestConfigSyncError_SanitizesURLDetails(t *testing.T) {
	t.Parallel()

	const secret = "private-token"

	err := configSyncError(output.EnsureLocalizer(nil), &configuration.ManagedSourceURLError{
		ConfiguredSource: "https://configured:" + secret + "@example.test/policy.yaml?token=" + secret + "#" + secret,
		ID:               "team-policy",
		ResolvedURL:      "https://resolved:" + secret + "@example.test/final.yaml?token=" + secret + "#" + secret,
	})

	check.NotContains(t, err.Error(), secret)
	check.Contains(t, err.Error(), "https://example.test/policy.yaml")
	check.Contains(t, err.Error(), "https://example.test/final.yaml")
	check.True(t, errors.Is(err, configuration.ErrManagedSourceInvalidURL))
}

func TestWriteConfigSyncOutput_SanitizesUntrustedFields(t *testing.T) {
	t.Parallel()

	const hostile = "value\x1b[31m\ninjected"

	var buf bytes.Buffer

	cmd := &cli.Command{Writer: &buf}
	result := configuration.ManagedSourceSyncResult{Items: []configuration.ManagedSourceSyncItem{{
		Destination: hostile,
		ID:          hostile,
		Status:      hostile,
	}}}

	err := writeConfigSyncOutput(cmd, output.EnsureLocalizer(nil), result)
	check.NoError(t, err)
	check.NotContains(t, buf.String(), "\x1b")
	check.NotContains(t, buf.String(), "\ninjected")
	check.Contains(t, buf.String(), `value\x1b[31m injected`)
}

func TestWriteConfigSyncOutput_WriteFailurePreservesErrOutputWrite(t *testing.T) {
	t.Parallel()

	cmd := &cli.Command{Writer: failingWriter{err: errMockWrite}}

	err := writeConfigSyncOutput(cmd, output.EnsureLocalizer(nil), configuration.ManagedSourceSyncResult{})

	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)
}
