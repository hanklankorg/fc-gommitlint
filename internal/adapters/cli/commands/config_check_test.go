// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"gopkg.in/yaml.v3"
)

func TestCollectConfigCheckIssues_MergesWarningsAndSourceIssues(t *testing.T) {
	t.Parallel()

	issues := collectConfigCheckIssues(
		[]domain.Warning{{Message: testWarningIssue}},
		[]configuration.ManagedSourceStatus{{
			ID:          testMissingPolicyID,
			Destination: testRepoTeamConfigPath,
			NextStep:    "gommitlint config sync --source missing-policy",
			Status:      configuration.ManagedSourceStatusMissing,
		}},
		output.EnsureLocalizer(nil),
	)

	check.Len(t, issues, 2)
	check.Equal(t, configCheckCodeWarning, issues[0].Code)
	check.Equal(t, configCheckSourceWarning, issues[0].Source)
	check.Equal(t, "managed_source_missing", issues[1].Code)
	check.Contains(t, issues[1].Hint, "gommitlint config sync --source missing-policy")
	check.Contains(t, issues[1].Message, "missing vendored file")
}

func TestCollectConfigCheckIssues_DescribesManagedSourceFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		status          configuration.ManagedSourceStatus
		expectedCode    string
		expectedMessage string
		expectedHint    string
	}{
		{
			name: "invalid",
			status: configuration.ManagedSourceStatus{
				ID:          testTeamPolicyID,
				Destination: testRepoTeamConfigPath,
				Status:      configuration.ManagedSourceStatusInvalid,
			},
			expectedCode:    "managed_source_invalid",
			expectedMessage: "invalid vendored file",
			expectedHint:    "inspect the vendored file manually",
		},
		{
			name: "integrity_mismatch",
			status: configuration.ManagedSourceStatus{
				ID:              testTeamPolicyID,
				Destination:     testRepoTeamConfigPath,
				IntegrityStatus: configuration.ManagedSourceIntegrityMismatch,
				Status:          configuration.ManagedSourceStatusVendored,
			},
			expectedCode:    "managed_source_integrity_mismatch",
			expectedMessage: "does not match integrity.sha256",
			expectedHint:    "update integrity.sha256",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			issues := collectConfigCheckIssues(nil, []configuration.ManagedSourceStatus{testCase.status}, nil)

			check.Len(t, issues, 1)
			check.Equal(t, testCase.expectedCode, issues[0].Code)
			check.Contains(t, issues[0].Message, testCase.expectedMessage)
			check.Contains(t, issues[0].Hint, testCase.expectedHint)
		})
	}
}

func TestBuildConfigCheckReport_FlagsIssuesAndSummarizes(t *testing.T) {
	t.Parallel()

	report := BuildConfigCheckReport(
		ConfigResult{
			Sources:  []configuration.ConfigSource{{Path: testGommitlintConfigFile, Type: configuration.SourceTypeProject}},
			Warnings: []domain.Warning{{Message: testWarningIssue}},
		},
		[]configuration.ManagedSourceStatus{{
			ID:          testMissingPolicyID,
			Destination: testRepoTeamConfigPath,
			Status:      configuration.ManagedSourceStatusMissing,
		}},
		output.EnsureLocalizer(nil),
	)

	check.False(t, report.OK)
	check.Equal(t, testGommitlintConfigFile, report.ConfigSource)
	check.Len(t, report.Issues, 2)
	check.Equal(t, []string{testWarningIssue}, report.Warnings)
	check.Len(t, report.ManagedSources, 1)
	check.Equal(t, 1, report.Summary.Counts[configuration.ManagedSourceStatusMissing])
	check.Equal(t, configuration.ManagedSourceStatusMissing, report.Summary.Items[0].Status)
	check.Empty(t, report.RecommendedNextCommand)
}

func TestRecommendNextConfigCommand_ConfigWarningsSuggestInspection(t *testing.T) {
	t.Parallel()

	command := recommendNextConfigCommand(nil, []ConfigCheckIssue{{Code: configCheckCodeWarning}})

	check.Equal(t, "gommitlint config show --format=yaml", command)
}

func TestPrintConfigCheckReport_RendersTextJSONAndYAML(t *testing.T) {
	t.Parallel()

	report := ConfigCheckReport{
		ConfigSource: testGommitlintConfigFile,
		Issues: []ConfigCheckIssue{{
			Code:    "managed_source_missing",
			Hint:    "Run 'gommitlint config sync --source team-policy'",
			Message: "managed source team-policy is missing vendored file /repo/.gommitlint.team.yaml",
			Source:  testTeamPolicyID,
		}},
		ManagedSources: []configuration.ManagedSourceStatus{{
			ConfiguredSource: testEnvPolicySource,
			ID:               testTeamPolicyID,
			Destination:      testRepoTeamConfigPath,
			NextStep:         testTeamPolicySync,
			Status:           configuration.ManagedSourceStatusMissing,
		}},
		Summary: ManagedSourceSummaryReport{
			Counts: map[string]int{configuration.ManagedSourceStatusMissing: 1},
			Items: []ManagedSourceSummary{{
				ID:          testTeamPolicyID,
				Destination: testRepoTeamConfigPath,
				NextStep:    testTeamPolicySync,
				Status:      configuration.ManagedSourceStatusMissing,
			}},
		},
		RecommendedNextCommand: testTeamPolicySync,
	}

	t.Run("text", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		err := printConfigCheckReport(&buf, report, output.FormatText, configViewVerbose, output.EnsureLocalizer(nil))
		check.NoError(t, err)
		check.Contains(t, buf.String(), "Status: FAIL (1 issue)")
		check.Contains(t, buf.String(), "Summary: 1 missing")
		check.Contains(t, buf.String(), "Recommended next command: "+testTeamPolicySync)
		check.Contains(t, buf.String(), "Managed Source Summary:")
		check.Contains(t, buf.String(), "status: missing")
		check.Contains(t, buf.String(), "next step: "+testTeamPolicySync)
		check.Contains(t, buf.String(), "Warnings: 0")
		check.Contains(t, buf.String(), "Configuration Issues:")
		check.Contains(t, buf.String(), "code: managed_source_missing")
		check.Contains(t, buf.String(), "hint: Run 'gommitlint config sync --source team-policy'")
		check.Contains(t, buf.String(), "problem: managed source team-policy is missing vendored file")
		check.Contains(t, buf.String(), "- team-policy")
	})

	t.Run("json", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		err := printConfigCheckReport(&buf, report, output.FormatJSON, configViewVerbose, output.EnsureLocalizer(nil))
		check.NoError(t, err)

		var parsed ConfigCheckReport
		check.NoError(t, json.Unmarshal(buf.Bytes(), &parsed))
		check.Equal(t, report.ConfigSource, parsed.ConfigSource)
		check.Len(t, parsed.Issues, 1)
	})

	t.Run("yaml", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		err := printConfigCheckReport(&buf, report, configTypes.ConfigFormatYAML, configViewVerbose, output.EnsureLocalizer(nil))
		check.NoError(t, err)

		var parsed ConfigCheckReport
		check.NoError(t, yaml.Unmarshal(buf.Bytes(), &parsed))
		check.Equal(t, report.ConfigSource, parsed.ConfigSource)
		check.Len(t, parsed.ManagedSources, 1)
	})
}

func TestPrintConfigCheckReport_WriteFailurePreservesErrOutputWrite(t *testing.T) {
	t.Parallel()

	for _, format := range []string{output.FormatText, output.FormatJSON, configTypes.ConfigFormatYAML} {
		t.Run("format_"+format, func(t *testing.T) {
			t.Parallel()

			err := printConfigCheckReport(
				failingWriter{err: errMockWrite},
				ConfigCheckReport{OK: true},
				format,
				configViewCompact,
				output.EnsureLocalizer(nil),
			)

			check.ErrorIs(t, err, ErrOutputWrite)
		})
	}
}

func TestPrintConfigCheckReport_TextSanitizesUntrustedFields(t *testing.T) {
	t.Parallel()

	const hostile = "value\x1b[31m\ninjected"

	report := ConfigCheckReport{
		ConfigSource: hostile,
		Issues: []ConfigCheckIssue{{
			Code:    hostile,
			Hint:    hostile,
			Message: hostile,
			Source:  hostile,
		}},
		ManagedSources:         []configuration.ManagedSourceStatus{{ID: hostile}},
		RecommendedNextCommand: hostile,
		Summary: ManagedSourceSummaryReport{
			Items: []ManagedSourceSummary{{
				Destination: hostile,
				ID:          hostile,
				NextStep:    hostile,
				Status:      hostile,
			}},
		},
		Warnings: []string{hostile},
	}

	var buf bytes.Buffer

	err := printConfigCheckReport(&buf, report, output.FormatText, configViewVerbose, output.EnsureLocalizer(nil))
	check.NoError(t, err)
	check.NotContains(t, buf.String(), "\x1b")
	check.NotContains(t, buf.String(), "\ninjected")
	check.Contains(t, buf.String(), `value\x1b[31m injected`)
}

func TestFormatManagedSourceCounts_RendersCountsInFixedOrder(t *testing.T) {
	t.Parallel()

	formatted := formatManagedSourceCounts(map[string]int{
		configuration.ManagedSourceIntegrityMismatch:  4,
		configuration.ManagedSourceStatusMissing:      1,
		configuration.ManagedSourceStatusVendored:     2,
		configuration.ManagedSourceSyncStatusVerified: 3,
	}, nil)

	check.Equal(t, "4 mismatch, 2 vendored, 1 missing, 3 verified", formatted)
}

func TestWriteConfigCheckSummary_LocalizesEmptyManagedSourceCount(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := writeConfigCheckSummary(&buf, ConfigCheckReport{OK: true}, configCheckTestLocalizer{})
	check.NoError(t, err)
	check.Contains(t, buf.String(), "Summary: localized-none")
}

type configCheckTestLocalizer struct{}

func (configCheckTestLocalizer) Get(message string, args ...any) string {
	if message == "none" {
		return "localized-none"
	}

	return output.EnsureLocalizer(nil).Get(message, args...)
}

func (configCheckTestLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return output.EnsureLocalizer(nil).GetN(singular, plural, count, args...)
}
