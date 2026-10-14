// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestInspectConfigProvenance(t *testing.T) {
	t.Parallel()

	fs := testhelpers.NewMemoryTestFS(t)
	afs := signing.NewAferoAdapter(fs.Fs)

	fs.WriteFile(testRepoBaseConfig, []byte(`gommitlint:
  output: text
  conventional:
    max_description_length: 72
  message:
    body:
      required: true
    subject:
      case: sentence
      max_length: 72
  repo:
    max_commits_ahead: 8
  rules:
    enabled:
      - conventional
  spell:
    locale: en_GB
`))
	fs.WriteFile(testRepoProjectConfig, []byte(`gommitlint:
  output: json
  conventional:
    require_scope: true
  message:
    body:
      required: false
    subject:
      case: lower
      max_length: 50
  rules:
    disabled:
      - spell
    enabled:
      - conventional
`))

	entries := configuration.InspectConfigProvenance([]configuration.ConfigSource{
		{Path: testConfigSourceDefaults, Type: configuration.SourceTypeDefaults},
		{Path: testRepoBaseConfig, Type: configuration.SourceTypeExtended},
		{Path: testRepoProjectConfig, Type: configuration.SourceTypeProject},
	}, afs)

	check.NotEmpty(t, entries)
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "output",
		Reason: testReasonExplicitConfig,
		Source: testRepoProjectConfig,
		Value:  "json",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testRepoBaseConfig,
			Value:  testOutputText,
		}, {
			Source: testConfigSourceDefaults,
			Value:  testOutputText,
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "message.subject.max_length",
		Reason: testReasonExplicitConfig,
		Source: testRepoProjectConfig,
		Value:  "50",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testRepoBaseConfig,
			Value:  "72",
		}, {
			Source: testConfigSourceDefaults,
			Value:  "100",
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "message.body.required",
		Reason: testReasonExplicitConfig,
		Source: testRepoProjectConfig,
		Value:  testValueFalse,
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testRepoBaseConfig,
			Value:  "true",
		}, {
			Source: testConfigSourceDefaults,
			Value:  testValueFalse,
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "message.subject.case",
		Reason: testReasonExplicitConfig,
		Source: testRepoProjectConfig,
		Value:  "lower",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testRepoBaseConfig,
			Value:  "sentence",
		}, {
			Source: testConfigSourceDefaults,
			Value:  "auto",
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "conventional.max_description_length",
		Reason: testReasonExplicitConfig,
		Source: testRepoBaseConfig,
		Value:  "72",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  "72",
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "conventional.require_scope",
		Reason: testReasonExplicitConfig,
		Source: testRepoProjectConfig,
		Value:  "true",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  testValueFalse,
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:      "rules.disabled",
		Reason:    testReasonExplicitConfig,
		Source:    testRepoProjectConfig,
		Value:     "[spell]",
		Overrides: nil,
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "repo.max_commits_ahead",
		Reason: testReasonExplicitConfig,
		Source: testRepoBaseConfig,
		Value:  "8",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  "10",
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "rule.spell",
		Reason: "explicitly disabled in rules.disabled",
		Source: testRepoProjectConfig,
		Value:  "disabled",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  testValueEnabled,
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "rule.conventional",
		Reason: "explicitly enabled in rules.enabled",
		Source: testRepoProjectConfig,
		Value:  testValueEnabled,
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testRepoBaseConfig,
			Value:  testValueEnabled,
		}, {
			Source: testConfigSourceDefaults,
			Value:  testValueEnabled,
		}},
	})
	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "spell.locale",
		Reason: testReasonExplicitConfig,
		Source: testRepoBaseConfig,
		Value:  "en_GB",
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  "both",
		}},
	})
}

func TestInspectConfigProvenance_WithPresetSources(t *testing.T) {
	t.Parallel()

	entries := configuration.InspectConfigProvenance([]configuration.ConfigSource{
		{Path: testConfigSourceDefaults, Type: configuration.SourceTypeDefaults},
		{Path: "preset:secure", Type: configuration.SourceTypePreset},
	}, nil)

	check.Contains(t, entries, configuration.ConfigProvenanceEntry{
		Path:   "rule.identity",
		Reason: "explicitly enabled in rules.enabled",
		Source: "preset:secure",
		Value:  testValueEnabled,
		Overrides: []configuration.ConfigProvenanceOverride{{
			Source: testConfigSourceDefaults,
			Value:  "disabled",
		}},
	})
}
