// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

func TestValidateConfigView(t *testing.T) {
	t.Parallel()

	check.NoError(t, validateConfigView(output.FormatText, configViewCompact, nil))
	check.NoError(t, validateConfigView(output.FormatText, configViewVerbose, nil))
	check.NoError(t, validateConfigView(output.FormatJSON, configViewCompact, nil))
	err := validateConfigView(output.FormatText, "full", nil)
	check.ErrorIs(t, err, ErrUnsupportedConfigView)

	var recoveryErr RecoveryError
	check.ErrorAs(t, err, &recoveryErr)
	check.Equal(t, "unsupported view 'full'", recoveryErr.Msg.String())
	check.Equal(t, []string{"Valid views: compact, verbose", "Use --view to choose a supported text view"}, recoveryErr.Suggestions)
}

func TestPrintConfigText_CompactView_ShowsNextStep(t *testing.T) {
	t.Parallel()

	effective := EffectiveConfig{
		ConfigSource: testConfigSourceDefault,
		EnabledRules: []string{rulecatalog.Conventional},
		ManagedSources: []configuration.ManagedSourceStatus{{
			ID:       testTeamPolicyID,
			NextStep: testTeamPolicySync,
			Status:   configuration.ManagedSourceStatusMissing,
			Source:   testEnvPolicySource,
		}},
	}

	var buf bytes.Buffer

	err := printConfigText(effective, configViewCompact, &buf, output.EnsureLocalizer(nil))
	check.NoError(t, err)
	check.Contains(t, buf.String(), "Next: gommitlint config sync --source team-policy")
	check.Contains(t, buf.String(), "Managed Sources:")
	check.Contains(t, buf.String(), "Summary: 1 missing")
	check.NotContains(t, buf.String(), "Configuration Graph:")
	check.NotContains(t, buf.String(), "Configuration Provenance:")
	check.NotContains(t, buf.String(), "Source mode:")
}

func TestPrintConfigText_CompactView_IntegrityMismatchShowsSourceNextStep(t *testing.T) {
	t.Parallel()

	effective := EffectiveConfig{ManagedSources: []configuration.ManagedSourceStatus{{
		ID:              testTeamPolicyID,
		IntegrityStatus: configuration.ManagedSourceIntegrityMismatch,
		NextStep:        testTeamPolicySync,
		Status:          configuration.ManagedSourceStatusVendored,
	}}}

	var buf bytes.Buffer

	err := printConfigText(effective, configViewCompact, &buf, output.EnsureLocalizer(nil))
	check.NoError(t, err)
	check.Contains(t, buf.String(), "Next: "+testTeamPolicySync)
}

func TestPrintConfigText_ReportsEarlyWriteFailure(t *testing.T) {
	t.Parallel()

	for _, view := range []configTextView{configViewCompact, configViewVerbose} {
		t.Run(string(view), func(t *testing.T) {
			t.Parallel()

			err := printConfigText(EffectiveConfig{}, view, &failOnceConfigWriter{}, output.EnsureLocalizer(nil))

			check.ErrorIs(t, err, ErrOutputWrite)
			check.ErrorIs(t, err, errMockWrite)
		})
	}
}

func TestPrintConfigText_SanitizesUntrustedValues(t *testing.T) {
	t.Parallel()

	const hostile = "value\x1b[31m\ninjected"

	effective := EffectiveConfig{
		Config: configTypes.Config{
			Message: configTypes.MessageConfig{Subject: configTypes.SubjectConfig{Case: new(hostile)}},
			Rules:   configTypes.RulesConfig{Enabled: []string{hostile}},
		},
		ConfigSource:  hostile,
		ConfigSources: []configuration.ConfigSource{{Path: hostile, Type: configuration.SourceTypeProject}},
		EnabledRules:  []string{rulecatalog.Subject},
		ManagedSources: []configuration.ManagedSourceStatus{{
			ConfiguredSource: hostile,
			Destination:      hostile,
			EnvVar:           hostile,
			FinalURL:         hostile,
			FetchedAt:        hostile,
			ID:               hostile,
			IntegrityStatus:  hostile,
			NextStep:         hostile,
			Revision:         hostile,
			Source:           hostile,
			SourceMode:       hostile,
			Status:           hostile,
		}},
		Provenance: []configuration.ConfigProvenanceEntry{{
			Overrides: []configuration.ConfigProvenanceOverride{{Source: hostile, Value: hostile}},
			Path:      hostile,
			Reason:    hostile,
			Source:    hostile,
			Value:     hostile,
		}},
		SourceGraph: []configuration.ConfigSourceNode{{Path: hostile, Type: configuration.SourceTypeProject}},
	}

	var buf bytes.Buffer

	err := printConfigText(effective, configViewVerbose, &buf, output.EnsureLocalizer(nil))
	check.NoError(t, err)
	check.NotContains(t, buf.String(), "\x1b")
	check.NotContains(t, buf.String(), "\ninjected")
	check.Contains(t, buf.String(), `value\x1b[31m injected`)
}

func TestPrintEnabledRules_DoesNotMutateInputAndLocalizesDisabledNames(t *testing.T) {
	t.Parallel()

	rules := []string{rulecatalog.Subject, rulecatalog.Conventional}
	original := append([]string(nil), rules...)

	var buf bytes.Buffer

	printEnabledRules(&buf, rules, testLocalizer(t, "sv"))

	check.Equal(t, original, rules)
	check.Contains(t, buf.String(), "  - Stavning")
}

func TestPrintHeader_LocalizesSourceTypes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	sources := []configuration.ConfigSource{
		{Path: "global.yaml", Type: configuration.SourceTypeGlobal},
		{Path: "project.yaml", Type: configuration.SourceTypeProject},
	}

	printHeader(&buf, "project.yaml", sources, testLocalizer(t, "sv"))

	check.Contains(t, buf.String(), "project.yaml (projekt)")
}

func TestBuildEffectiveConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		cfg          configTypes.Config
		commitRules  []domain.CommitRule
		repoRules    []domain.RepositoryRule
		configSource string
		description  string
	}{
		{
			name:         "empty configuration",
			cfg:          configTypes.Config{},
			commitRules:  []domain.CommitRule{},
			repoRules:    []domain.RepositoryRule{},
			configSource: testConfigSourceDefault,
			description:  "should handle empty configuration",
		},
		{
			name: "configuration with rules",
			cfg: configTypes.Config{
				Message: configTypes.MessageConfig{
					Subject: configTypes.SubjectConfig{MaxLength: new(50)},
				},
			},
			commitRules: []domain.CommitRule{
				mockCommitRule{name: rulecatalog.Subject},
				mockCommitRule{name: rulecatalog.Conventional},
			},
			repoRules: []domain.RepositoryRule{
				mockRepoRule{name: ruleBranchAhead},
			},
			configSource: testGommitlintConfigFile,
			description:  "should include all rules in enabled rules list",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfgResult := ConfigResult{
				SourceGraph: []configuration.ConfigSourceNode{{Path: testCase.configSource, Type: configuration.SourceTypeProject}},
				Sources:     []configuration.ConfigSource{{Path: testCase.configSource, Type: configuration.SourceTypeProject}},
				Warnings:    []domain.Warning{{Message: configCheckSourceWarning}},
			}
			managedSources := []configuration.ManagedSourceStatus{{ID: testTeamPolicyID, Status: configuration.ManagedSourceStatusVendored}}
			effective := BuildEffectiveConfig(
				testCase.cfg,
				testCase.commitRules,
				testCase.repoRules,
				cfgResult,
				managedSources,
				output.EnsureLocalizer(nil),
			)

			check.Equal(t, testCase.cfg, effective.Config)
			check.Equal(t, testCase.configSource, effective.ConfigSource)
			check.Equal(t, cfgResult.SourceGraph, effective.SourceGraph)
			check.Equal(t, managedSources, effective.ManagedSources)
			check.Nil(t, effective.Provenance)
			check.Equal(t, []string{configCheckSourceWarning}, effective.Warnings)
			check.Equal(t, 1, effective.SchemaVersion)

			expectedRulesCount := len(testCase.commitRules) + len(testCase.repoRules)
			check.Len(t, effective.EnabledRules, expectedRulesCount)

			ruleNameMap := make(map[string]bool)
			for _, ruleName := range effective.EnabledRules {
				ruleNameMap[ruleName] = true
			}

			for _, rule := range testCase.commitRules {
				check.True(t, ruleNameMap[rule.Name()], "commit rule %s should be in enabled rules", rule.Name())
			}

			for _, rule := range testCase.repoRules {
				check.True(t, ruleNameMap[rule.Name()], "repo rule %s should be in enabled rules", rule.Name())
			}
		})
	}
}

func TestPrintConfigJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		effectiveConfig EffectiveConfig
		description     string
	}{
		{
			name: testNameBasicConfig,
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Message: configTypes.MessageConfig{
						Subject: configTypes.SubjectConfig{MaxLength: new(50)},
					},
				},
				EnabledRules: []string{rulecatalog.Subject, rulecatalog.Conventional},
				ConfigSource: testRepoConfigPath,
				SourceGraph: []configuration.ConfigSourceNode{{
					Path: testRepoConfigPath,
					Type: configuration.SourceTypeProject,
					Children: []configuration.ConfigSourceNode{{
						Path: testPresetConventional,
						Type: configuration.SourceTypePreset,
					}},
				}},
				Provenance: []configuration.ConfigProvenanceEntry{{
					Path:   flagNameOutput,
					Reason: "explicitly set in config",
					Source: testPresetConventional,
					Value:  "text",
					Overrides: []configuration.ConfigProvenanceOverride{{
						Source: configSourceDefaults,
						Value:  "json",
					}},
				}, {
					Path:   "rule.conventional",
					Reason: "explicitly enabled in rules.enabled",
					Source: testPresetConventional,
					Value:  "enabled",
					Overrides: []configuration.ConfigProvenanceOverride{{
						Source: configSourceDefaults,
						Value:  "disabled",
					}},
				}},
				ManagedSources: []configuration.ManagedSourceStatus{{
					ConfiguredSource: testEnvPolicySource,
					EnvVar:           "GOMMITLINT_POLICY_URL",
					ID:               testTeamPolicyID,
					IntegrityStatus:  configuration.ManagedSourceIntegrityVerified,
					ResolvedFromEnv:  true,
					SourceMode:       "environment",
					Status:           configuration.ManagedSourceStatusVendored,
					Source:           testEnvPolicySource,
					Destination:      testRepoTeamConfigPath,
				}},
			},
			description: "should output valid JSON",
		},
		{
			name: "empty configuration",
			effectiveConfig: EffectiveConfig{
				Config:       configTypes.Config{},
				EnabledRules: []string{},
				ConfigSource: testConfigSourceDefault,
				SourceGraph: []configuration.ConfigSourceNode{{
					Path: testConfigSourceDefault,
					Type: configuration.SourceTypeDefaults,
				}},
				Provenance: []configuration.ConfigProvenanceEntry{},
			},
			description: "should handle empty configuration",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary file for testing
			tmpFile, err := os.CreateTemp(t.TempDir(), "config-test-*.json")
			check.NoError(t, err)

			defer func() {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}()

			err = printConfigJSON(testCase.effectiveConfig, tmpFile)
			check.NoError(t, err)

			// Read back the file content
			_, err = tmpFile.Seek(0, 0)
			check.NoError(t, err)

			content := make([]byte, 10240)
			n, _ := tmpFile.Read(content)
			jsonOutput := string(content[:n])

			// Verify it's valid JSON
			var parsed EffectiveConfig

			err = json.Unmarshal([]byte(jsonOutput), &parsed)
			check.NoError(t, err, testCase.description)

			// Verify content matches
			check.Equal(t, testCase.effectiveConfig.ConfigSource, parsed.ConfigSource)
			check.Equal(t, testCase.effectiveConfig.EnabledRules, parsed.EnabledRules)
			check.Equal(t, testCase.effectiveConfig.SourceGraph, parsed.SourceGraph)
			check.Equal(t, testCase.effectiveConfig.ManagedSources, parsed.ManagedSources)
			check.Equal(t, testCase.effectiveConfig.Provenance, parsed.Provenance)
		})
	}
}

func TestPrintConfigYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		effectiveConfig EffectiveConfig
		description     string
	}{
		{
			name: testNameBasicConfig,
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Message: configTypes.MessageConfig{
						Subject: configTypes.SubjectConfig{MaxLength: new(50)},
					},
				},
				EnabledRules: []string{rulecatalog.Subject},
				ConfigSource: testConfigSourceDefault,
			},
			description: "should output valid YAML",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary file for testing
			tmpFile, err := os.CreateTemp(t.TempDir(), "config-test-*.yaml")
			check.NoError(t, err)

			defer func() {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}()

			err = printConfigYAML(testCase.effectiveConfig, tmpFile, output.EnsureLocalizer(nil))
			check.NoError(t, err)

			// Read back the file content
			_, err = tmpFile.Seek(0, 0)
			check.NoError(t, err)

			content := make([]byte, 10240)
			n, _ := tmpFile.Read(content)
			yamlOutput := string(content[:n])

			// Verify it contains expected content
			check.Contains(t, yamlOutput, "# Gommitlint Configuration")
			check.Contains(t, yamlOutput, "gommitlint:")
			check.Contains(t, yamlOutput, testCase.effectiveConfig.ConfigSource)

			// Verify it's valid YAML
			var parsed map[string]any

			err = yaml.Unmarshal([]byte(yamlOutput), &parsed)
			check.NoError(t, err, testCase.description)

			check.Contains(t, parsed, "gommitlint")
		})
	}
}

func TestPrintConfigYAML_SanitizesConfigurationSourceComment(t *testing.T) {
	t.Parallel()

	effective := EffectiveConfig{ConfigSource: "source\x1b[31m\nmalicious: true"}

	var buf bytes.Buffer

	err := printConfigYAML(effective, &buf, output.EnsureLocalizer(nil))
	check.NoError(t, err)
	check.NotContains(t, buf.String(), "\x1b")
	check.NotContains(t, buf.String(), "\nmalicious: true")

	var parsed map[string]any
	check.NoError(t, yaml.Unmarshal(buf.Bytes(), &parsed))
	check.NotContains(t, parsed, "malicious")
}

func TestConfigShow_SanitizesManagedSourceSecrets(t *testing.T) {
	t.Parallel()

	cfg := configTypes.Config{ManagedSources: []configTypes.ManagedSource{{
		ID:     "team-policy",
		Source: "https://source-user:source-secret@example.test/policy.yaml?source-token=query-secret#source-fragment",
		Headers: map[string]string{
			"Authorization": "Bearer header-secret",
			"X-Token":       "${POLICY_TOKEN}",
		},
		Resolved: configTypes.ManagedSourceResolved{
			FinalURL: "https://final-user:final-secret@example.test/final.yaml?final-token=query-secret#final-fragment",
		},
	}}}.Sanitized()
	effective := EffectiveConfig{
		Config:       cfg,
		ConfigSource: testConfigSourceDefault,
		ManagedSources: []configuration.ManagedSourceStatus{{
			ID:       "team-policy",
			Source:   cfg.ManagedSources[0].Source,
			FinalURL: cfg.ManagedSources[0].Resolved.FinalURL,
			Status:   configuration.ManagedSourceStatusVendored,
		}},
	}

	var jsonOutput bytes.Buffer

	err := printConfigJSON(effective, &jsonOutput)
	check.NoError(t, err)

	var yamlOutput bytes.Buffer

	err = printConfigYAML(effective, &yamlOutput, output.EnsureLocalizer(nil))
	check.NoError(t, err)

	var textOutput bytes.Buffer

	err = printConfigText(effective, configViewVerbose, &textOutput, output.EnsureLocalizer(nil))
	check.NoError(t, err)

	for _, rendered := range []string{jsonOutput.String(), yamlOutput.String()} {
		check.Contains(t, rendered, "[REDACTED]")
		check.Contains(t, rendered, "${POLICY_TOKEN}")
		check.NotContains(t, rendered, "header-secret")
	}

	for _, rendered := range []string{jsonOutput.String(), yamlOutput.String(), textOutput.String()} {
		check.Contains(t, rendered, "https://example.test/policy.yaml")
		check.Contains(t, rendered, "https://example.test/final.yaml")
		check.NotContains(t, rendered, "source-secret")
		check.NotContains(t, rendered, "final-secret")
		check.NotContains(t, rendered, "query-secret")
		check.NotContains(t, rendered, "source-fragment")
		check.NotContains(t, rendered, "final-fragment")
	}
}

func TestPrintConfigText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		effectiveConfig EffectiveConfig
		expectedContent []string
		description     string
	}{
		{
			name: testNameBasicConfig,
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Message: configTypes.MessageConfig{
						Subject: configTypes.SubjectConfig{
							MaxLength:         new(50),
							Case:              new("any"),
							RequireImperative: new(false),
						},
						Body: configTypes.BodyConfig{
							Required:  new(false),
							MinLength: new(0),
						},
					},
					Conventional: configTypes.ConventionalConfig{
						DisallowBreaking:     new(false),
						MaxDescriptionLength: new(100),
						RequireScope:         new(false),
					},
					CryptoSignature: configTypes.CryptoSignatureConfig{
						Required:     new(false),
						VerifyFormat: new(false),
					},
					Identity: configTypes.IdentityConfig{
						AllowedAuthors: []string{},
					},
					Repo: configTypes.RepoConfig{
						MaxCommitsAhead: new(10),
					},
					Jira: configTypes.JiraConfig{
						RequireInSubject: new(false),
						RequireInBody:    new(false),
					},
					Spell: configTypes.SpellConfig{
						Locale: new("en_US"),
					},
					Output: output.FormatText,
				},
				EnabledRules: []string{rulecatalog.Subject, ruleConventional},
				ConfigSource: "/repo/.gommitlint.yaml",
				SourceGraph: []configuration.ConfigSourceNode{{
					Path: "/repo/.gommitlint.yaml",
					Type: configuration.SourceTypeProject,
					Children: []configuration.ConfigSourceNode{{
						Path: testPresetConventional,
						Type: configuration.SourceTypePreset,
					}},
				}},
				Provenance: []configuration.ConfigProvenanceEntry{{
					Path:   flagNameOutput,
					Reason: "explicitly set in config",
					Source: testPresetConventional,
					Value:  "text",
					Overrides: []configuration.ConfigProvenanceOverride{{
						Source: configSourceDefaults,
						Value:  "json",
					}},
				}, {
					Path:   "rule.conventional",
					Reason: "explicitly enabled in rules.enabled",
					Source: testPresetConventional,
					Value:  "enabled",
					Overrides: []configuration.ConfigProvenanceOverride{{
						Source: configSourceDefaults,
						Value:  "disabled",
					}},
				}},
				ManagedSources: []configuration.ManagedSourceStatus{{
					ConfiguredSource: "${GOMMITLINT_POLICY_URL}",
					EnvVar:           "GOMMITLINT_POLICY_URL",
					ID:               "team-policy",
					IntegrityStatus:  configuration.ManagedSourceIntegrityVerified,
					ResolvedFromEnv:  true,
					SourceMode:       "environment",
					Status:           configuration.ManagedSourceStatusVendored,
					Source:           "${GOMMITLINT_POLICY_URL}",
					Destination:      "/repo/.gommitlint.team.yaml",
				}},
			},
			expectedContent: []string{
				"Gommitlint Configuration",
				"Configuration Source: /repo/.gommitlint.yaml",
				"Enabled Rules (2 of 10):",
				"Configuration Graph:",
				"/repo/.gommitlint.yaml (project)",
				"preset:conventional (preset)",
				"Configuration Provenance:",
				"output = text",
				"Reason: explicitly set in config",
				"Overrides:",
				"defaults = json",
				"rule.conventional = enabled",
				"Reason: explicitly enabled in rules.enabled",
				"defaults = disabled",
				"Managed Sources:",
				"team-policy [vendored]",
				"Source mode: environment",
				"Env var: GOMMITLINT_POLICY_URL",
				"Configured source: ${GOMMITLINT_POLICY_URL}",
				"Resolved from env during last sync: true",
				"Integrity: verified",
				"- conventional",
				"- subject",
				"Subject max length: 50",
				"Subject case: any",
				"Require imperative: false",
				"Body min length: 0",
				"Conventional Commit Configuration:",
				"Max description length: 100",
				"Signature Configuration:",
				"Repository Configuration:",
				"JIRA Configuration:",
				"Spell Configuration:",
				"Limits Configuration:",
				"Max commit count: 2000",
				"Max key files: 200",
				"Output Configuration:",
			},

			description: "should output text format",
		},
		{
			name: "empty rules",
			effectiveConfig: EffectiveConfig{
				Config:         configTypes.Config{},
				EnabledRules:   []string{},
				ConfigSource:   "empty",
				SourceGraph:    []configuration.ConfigSourceNode{{Path: "empty", Type: configuration.SourceTypeProject}},
				Provenance:     []configuration.ConfigProvenanceEntry{},
				ManagedSources: []configuration.ManagedSourceStatus{},
			},
			expectedContent: []string{
				"Enabled Rules (0 of 10):",
				"Configuration Graph:",
				"Configuration Provenance:",
				"Managed Sources:",
				"(none)",
				"Configuration Source: empty",
			},
			description: "should handle empty rules list",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create a temporary file for testing
			tmpFile, err := os.CreateTemp(t.TempDir(), "config-test-*.txt")
			check.NoError(t, err)

			defer func() {
				_ = tmpFile.Close()
				_ = os.Remove(tmpFile.Name())
			}()

			err = printConfigText(testCase.effectiveConfig, configViewVerbose, tmpFile, output.EnsureLocalizer(nil))
			check.NoError(t, err)

			// Read back the file content
			_, err = tmpFile.Seek(0, 0)
			check.NoError(t, err)

			content := make([]byte, 10240)
			n, _ := tmpFile.Read(content)
			textOutput := string(content[:n])

			// Verify expected content is present
			for _, expected := range testCase.expectedContent {
				check.Contains(t, textOutput, expected, "%s should contain '%s'", testCase.description, expected)
			}
		})
	}
}

func TestConfigShow_EmitsWarningsToStderr(t *testing.T) {
	// The assertion is on English text, so pin the locale: on a translated
	// workstation the command would otherwise render the localized warning.
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")

	tempDir := t.TempDir()

	t.Chdir(tempDir)

	config := `gommitlint:
  ignore_commits:
    - "bad"`
	check.NoError(t, os.WriteFile(filepath.Join(tempDir, testGommitlintConfigFile), []byte(config), 0o600))

	cmd := createTestConfigCommand(commandNameConfigShow, io.Discard)
	cmd.Flags = []cli.Flag{&cli.StringFlag{Name: flagNameFormat, Value: output.FormatText}}

	var stderrBuf bytes.Buffer

	cmd.ErrWriter = &stderrBuf

	rootCmd := &cli.Command{Name: gommitlintKey}
	rootCmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: flagNameConfig, Value: ""},
		&cli.BoolFlag{Name: flagNameIgnoreConfig, Value: false},
		&cli.StringFlag{Name: flagNameRepoPath, Value: ""},
	}

	if cmd.Metadata == nil {
		cmd.Metadata = make(map[string]any)
	}

	cmd.Metadata["root"] = rootCmd

	check.NoError(t, ExecuteConfigShow(t.Context(), cmd))
	check.Contains(t, stderrBuf.String(), "Warning: 'bad' is not a valid commit SHA in ignore_commits, skipping it")
}

func TestConfigShow_EmitsWarningsForExplicitConfigToStderr(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_MESSAGES", "C")

	configPath := filepath.Join(t.TempDir(), testGommitlintConfigFile)
	config := `gommitlint:
  ignore_commits:
    - "bad"`
	check.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))

	cmd := createTestConfigCommand(commandNameConfigShow, io.Discard)
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: flagNameFormat, Value: output.FormatText},
		&cli.StringFlag{Name: flagNameConfig, Value: configPath},
		&cli.BoolFlag{Name: flagNameIgnoreConfig, Value: false},
		&cli.StringFlag{Name: flagNameRepoPath, Value: ""},
	}

	var stderrBuf bytes.Buffer

	cmd.ErrWriter = &stderrBuf

	check.NoError(t, ExecuteConfigShow(t.Context(), cmd))
	check.Contains(t, stderrBuf.String(), "Warning: 'bad' is not a valid commit SHA in ignore_commits, skipping it")
}

func TestPrintConfigText_SortsListsAlphabetically(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		effectiveConfig EffectiveConfig
		expectedOrder   []string
		description     string
	}{
		{
			name: "enabled_rules_sorted",
			effectiveConfig: EffectiveConfig{
				Config:        configTypes.Config{},
				EnabledRules:  []string{rulecatalog.Subject, ruleConventional},
				ConfigSource:  testGommitlintConfigFile,
				ConfigSources: []configuration.ConfigSource{},
				Warnings:      []string{configCheckSourceWarning},
				SchemaVersion: 1,
			},

			expectedOrder: []string{
				"- conventional",
				"- subject",
			},
			description: "enabled rules should be sorted alphabetically",
		},
		{
			name: "config_enabled_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Rules: configTypes.RulesConfig{
						Enabled: []string{rulecatalog.Spell, ruleConventional, rulecatalog.Subject},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Enabled: [conventional spell subject]",
			},
			description: "config enabled rules should be sorted",
		},
		{
			name: "disabled_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Rules: configTypes.RulesConfig{
						Disabled: []string{rulecatalog.SignOff, ruleIdentity, rulecatalog.CommitBody},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Disabled: [commitbody identity signoff]",
			},
			description: "disabled rules should be sorted",
		},
		{
			name: "conventional_types_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Conventional: configTypes.ConventionalConfig{
						Types: []string{"fix", "feat", "docs", "build", "chore"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Types: [build chore docs feat fix]",
			},
			description: "conventional types should be sorted",
		},
		{
			name: "conventional_scopes_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Conventional: configTypes.ConventionalConfig{
						Scopes: []string{"api", "cli", "docs"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Scopes: [api cli docs]",
			},
			description: "conventional scopes should be sorted",
		},
		{
			name: "forbid_endings_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Message: configTypes.MessageConfig{
						Subject: configTypes.SubjectConfig{
							ForbidEndings: []string{"?", "!", ".", ","},
						},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Forbid endings: [! , . ?]",
			},
			description: "forbid endings should be sorted",
		},
		{
			name: "allowed_signers_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					CryptoSignature: configTypes.CryptoSignatureConfig{
						AllowedSigners: []string{"user3@example.com", "user1@example.com", "admin@example.com"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Allowed signers: [admin@example.com user1@example.com user3@example.com]",
			},
			description: "allowed signers should be sorted",
		},
		{
			name: "allowed_authors_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Identity: configTypes.IdentityConfig{
						AllowedAuthors: []string{"user3@example.com", "user1@example.com", "admin@example.com"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Allowed authors: [admin@example.com user1@example.com user3@example.com]",
			},
			description: "allowed authors should be sorted",
		},
		{
			name: "jira_project_prefixes_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Jira: configTypes.JiraConfig{
						ProjectPrefixes: []string{"PROJ", "DEV", "BUG", "FEAT"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Project prefixes: [BUG DEV FEAT PROJ]",
			},
			description: "JIRA project prefixes should be sorted",
		},
		{
			name: "jira_ignore_patterns_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Jira: configTypes.JiraConfig{
						IgnoreTicketPatterns: []string{"WIP-.*", "DRAFT-.*", "TEST-.*"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Ignore ticket patterns: [DRAFT-.* TEST-.* WIP-.*]",
			},
			description: "JIRA ignore patterns should be sorted",
		},
		{
			name: "spell_ignore_words_sorted",
			effectiveConfig: EffectiveConfig{
				Config: configTypes.Config{
					Spell: configTypes.SpellConfig{
						IgnoreWords: []string{"refactor", "config", "api", "cli"},
					},
				},
				EnabledRules:  []string{},
				ConfigSource:  testValueTest,
				Warnings:      []string{},
				SchemaVersion: 1,
			},
			expectedOrder: []string{
				"Ignore words: [api cli config refactor]",
			},
			description: "spell ignore words should be sorted",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			err := printConfigText(testCase.effectiveConfig, configViewVerbose, &buf, output.EnsureLocalizer(nil))

			check.NoError(t, err)

			output := buf.String()
			for _, expected := range testCase.expectedOrder {
				check.Contains(t, output, expected, testCase.description)
			}
		})
	}
}
