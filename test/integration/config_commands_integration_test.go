//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

// TestConfigCommands_ShowInitAndFallbackWorkflows drives config show in all
// three formats, config init's template, the unparseable-config fallback to
// defaults, and --repo-path config discovery through the real CLI.
func TestConfigCommands_ShowInitAndFallbackWorkflows(t *testing.T) {
	t.Run("config show integration workflow", func(t *testing.T) {
		tmpDir := gitrepo.PathWithCommits(t, "feat: initial commit for config test")

		fs := testhelpers.NewTestFS(t, tmpDir)

		configContent := `gommitlint:
  rules:
    conventionalcommit:
      enabled: true
      types: ["feat", "fix", "docs"]
    subjectlength:
      enabled: true
      minlength: 10
      maxlength: 72
  message:
    maxlength: 500
`
		fs.WriteConfig(".gommitlint.yaml", configContent)

		tests := []struct {
			name           string
			format         string
			expectContains []string
			expectError    bool
		}{
			{
				name:   "show config in text format",
				format: output.FormatText,

				expectContains: []string{
					"Configuration",
					rulecatalog.Conventional,
					"Subject max length: 100",
					"Enabled Rules",
				},
				expectError: false,
			},
			{
				name:   "show config in json format",
				format: output.FormatJSON,

				expectContains: []string{
					`"conventional"`,
					fmt.Sprintf("%q", rulecatalog.Conventional),
					`"max_length": 100`,
				},
				expectError: false,
			},
			{
				name:   "show config in yaml format",
				format: configTypes.ConfigFormatYAML,

				expectContains: []string{
					"conventional:",
					"max_length: 100",
					"gommitlint:",
				},
				expectError: false,
			},
		}

		for _, testCase := range tests { //nolint:varnamelen // Short variable is idiomatic in Go tests
			t.Run(testCase.name, func(t *testing.T) {
				args := []string{"gommitlint", "config", "show", "--format", testCase.format, "--repo-path", tmpDir}
				outputStr, err := RunCLICommand(t, args, tmpDir)

				if testCase.expectError {
					check.Error(t, err)
				} else {
					check.NoError(t, err)

					for _, expected := range testCase.expectContains {
						check.Contains(t, outputStr, expected, "Expected '%s' in output: %s", expected, outputStr)
					}

					switch testCase.format {
					case output.FormatJSON:
						var jsonData map[string]any
						check.NoError(t, json.Unmarshal([]byte(outputStr), &jsonData))
					case configTypes.ConfigFormatYAML:
						check.Contains(t, outputStr, "gommitlint:")
					}
				}
			})
		}
	})

	t.Run("config init integration workflow", func(t *testing.T) {
		tmpDir := gitrepo.PathWithCommits(t, "feat: initial commit for config init test")

		// --output points the template into tmpDir explicitly. Config init's
		// destination resolution honours --output; --repo-path is a
		// validation-time flag that init ignores. --force overwrites any
		// .gommitlint.yaml gitrepo.PathWithCommits may have seeded in the temp repo.
		configPath := filepath.Join(tmpDir, ".gommitlint.yaml")
		args := []string{"gommitlint", "config", "init", "--output", configPath, "--force"}
		_, err := RunCLICommand(t, args, tmpDir)
		check.NoError(t, err)

		configBytes, err := os.ReadFile(configPath)
		check.NoError(t, err, "config init should produce .gommitlint.yaml at the --output path")

		outputStr := string(configBytes)

		expectedContents := []string{
			"# Gommitlint Configuration",
			"gommitlint:",
			"rules:",
			"conventional:",
			"enabled:",
			"max_length:",
			"message:",
		}

		for _, expected := range expectedContents {
			check.Contains(t, outputStr, expected, "Config init output should contain '%s'", expected)
		}

		check.Contains(t, outputStr, "gommitlint:", "Should contain root gommitlint key")
		check.Contains(t, outputStr, "  rules:", "Should contain properly indented rules section")
	})

	t.Run("config commands error handling", func(t *testing.T) {
		tmpDir := t.TempDir()

		fs := testhelpers.NewTestFS(t, tmpDir)
		fs.WriteConfig(".gommitlint.yaml", "invalid: yaml: content: [")

		args := []string{"gommitlint", "config", "show", "--repo-path", tmpDir}
		outputStr, err := RunCLICommand(t, args, tmpDir)

		// Discovery skips malformed config and reports defaults as the source.
		check.NoError(t, err, "an unparseable config should not fail config show")
		check.Contains(t, outputStr, "Configuration Source: defaults",
			"the skipped file must not be reported as the source in use")
	})

	t.Run("config commands with repo-path integration", func(t *testing.T) {
		mainDir := gitrepo.PathWithCommits(t, "feat: initial commit for main repo")

		fs := testhelpers.NewTestFS(t, mainDir)
		fs.CreateDir("subproject")

		configContent := `gommitlint:
  rules:
    conventionalcommit:
      enabled: true
      types: ["feat", "fix"]
`
		fs.WriteConfig(".gommitlint.yaml", configContent)

		args := []string{"gommitlint", "config", "show", "--repo-path", mainDir}
		outputStr, err := RunCLICommand(t, args, mainDir)
		check.NoError(t, err)
		check.Contains(t, outputStr, rulecatalog.Conventional)
		check.Contains(t, outputStr, "feat")
		check.Contains(t, outputStr, "fix")
	})
}
