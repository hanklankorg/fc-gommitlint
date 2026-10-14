// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestInspectManagedSources(t *testing.T) {
	t.Parallel()

	t.Run("reports vendored and missing managed sources", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		fs.WriteFile("/repo/"+testManagedTeamFile, []byte("gommitlint:\n  output: json\n"))

		statuses := configuration.InspectManagedSources("/repo/.gommitlint.yaml", configTypes.Config{
			ManagedSources: []configTypes.ManagedSource{
				{
					ID:          testManagedTeamID,
					Integrity:   configTypes.ManagedSourceIntegrity{SHA256: "fc610c9b44de6df7e8b804a49b37d4d796f6062cf252d025df216988b0d5a59d"},
					Source:      testManagedTeamURL,
					Destination: testManagedTeamFile,
					Resolved: configTypes.ManagedSourceResolved{
						FinalURL:  testManagedTeamURL,
						FetchedAt: "2026-03-09T12:00:00Z",
					},
				},
				{
					ID:          "missing-policy",
					Source:      "https://example.test/missing.yaml",
					Destination: ".gommitlint.missing.yaml",
				},
			},
		}, afs)

		check.Len(t, statuses, 2)
		check.Equal(t, configuration.ManagedSourceStatusVendored, statuses[0].Status)
		check.Equal(t, "direct", statuses[0].SourceMode)
		check.Equal(t, configuration.ManagedSourceIntegrityVerified, statuses[0].IntegrityStatus)
		check.Equal(t, "/repo/"+testManagedTeamFile, statuses[0].Destination)
		check.Equal(t, configuration.ManagedSourceStatusMissing, statuses[1].Status)
		check.Equal(t, "direct", statuses[1].SourceMode)
		check.Equal(t, configuration.ManagedSourceIntegrityUnknown, statuses[1].IntegrityStatus)
		check.Equal(t, "/repo/.gommitlint.missing.yaml", statuses[1].Destination)
	})

	t.Run("reports invalid vendored managed source content", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		fs.WriteFile("/repo/"+testManagedTeamFile, []byte("not: [valid"))

		statuses := configuration.InspectManagedSources("/repo/.gommitlint.yaml", configTypes.Config{
			ManagedSources: []configTypes.ManagedSource{{
				ID:          testManagedTeamID,
				Source:      testManagedTeamURL,
				Destination: testManagedTeamFile,
			}},
		}, afs)

		check.Len(t, statuses, 1)
		check.Equal(t, configuration.ManagedSourceStatusInvalid, statuses[0].Status)
	})

	t.Run("reports integrity mismatch for vendored file", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		fs.WriteFile("/repo/"+testManagedTeamFile, []byte("gommitlint:\n  output: json\n"))

		statuses := configuration.InspectManagedSources("/repo/.gommitlint.yaml", configTypes.Config{
			ManagedSources: []configTypes.ManagedSource{{
				ID:          testManagedTeamID,
				Source:      testManagedTeamURL,
				Destination: testManagedTeamFile,
				Integrity:   configTypes.ManagedSourceIntegrity{SHA256: "0000000000000000000000000000000000000000000000000000000000000000"},
			}},
		}, afs)

		check.Len(t, statuses, 1)
		check.Equal(t, configuration.ManagedSourceStatusVendored, statuses[0].Status)
		check.Equal(t, configuration.ManagedSourceIntegrityMismatch, statuses[0].IntegrityStatus)
	})

	t.Run("reports environment-based source details", func(t *testing.T) {
		t.Parallel()

		statuses := configuration.InspectManagedSources("/repo/.gommitlint.yaml", configTypes.Config{
			ManagedSources: []configTypes.ManagedSource{{
				ID:          testManagedTeamID,
				Source:      "${GOMMITLINT_POLICY_URL}",
				Destination: testManagedTeamFile,
			}},
		}, nil)

		check.Len(t, statuses, 1)
		check.Equal(t, "environment", statuses[0].SourceMode)
		check.Equal(t, "GOMMITLINT_POLICY_URL", statuses[0].EnvVar)
		check.False(t, statuses[0].ResolvedFromEnv)
	})

	t.Run("sanitizes URL metadata for display", func(t *testing.T) {
		t.Parallel()

		statuses := configuration.InspectManagedSources("/repo/.gommitlint.yaml", configTypes.Config{
			ManagedSources: []configTypes.ManagedSource{{
				ID:          testManagedTeamID,
				Source:      "https://source-user:source-secret@example.test/policy.yaml?source-token=private#source-fragment",
				Destination: testManagedTeamFile,
				Resolved: configTypes.ManagedSourceResolved{
					FinalURL: "https://final-user:final-secret@example.test/final.yaml?final-token=private#final-fragment",
				},
			}},
		}, nil)

		check.Len(t, statuses, 1)
		check.Equal(t, "https://example.test/policy.yaml", statuses[0].Source)
		check.Equal(t, "https://example.test/policy.yaml", statuses[0].ConfiguredSource)
		check.Equal(t, "https://example.test/final.yaml", statuses[0].FinalURL)
	})
}

func TestFormatConfigSourceType_NamesEverySourceType(t *testing.T) {
	t.Parallel()

	check.Equal(t, "defaults", configuration.FormatConfigSourceType(configuration.SourceTypeDefaults))
	check.Equal(t, "extends", configuration.FormatConfigSourceType(configuration.SourceTypeExtended))
	check.Equal(t, "preset", configuration.FormatConfigSourceType(configuration.SourceTypePreset))
	check.Equal(t, "global", configuration.FormatConfigSourceType(configuration.SourceTypeGlobal))
	check.Equal(t, "project", configuration.FormatConfigSourceType(configuration.SourceTypeProject))
	check.Equal(t, "explicit", configuration.FormatConfigSourceType(configuration.SourceTypeCLI))
}
