// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestParseConfigFileStrict_SurvivesPathologicalInputs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		content          string
		format           string
		expectError      bool
		expectedBehavior string
		// wantOutput, when set, is the output value the content must yield.
		wantOutput string
	}{
		{
			name: "YAML with partial content",
			content: `gommitlint:
  rules:
    enabled:
      - subject
  message:
    subject:
      max_length: 72
  # Incomplete section below
  conventional:`,
			format:           settings.ConfigFormatYAML,
			expectError:      false,
			expectedBehavior: "should parse valid sections and ignore incomplete ones",
		},
		{
			name: "YAML with circular reference attempt",
			content: `gommitlint: &anchor
  rules:
    enabled: *anchor`,
			format:           settings.ConfigFormatYAML,
			expectError:      true,
			expectedBehavior: "should detect and report circular references",
		},
		{
			name: "TOML with mixed types",
			content: `[gommitlint]
output = "json"
output = 123  # Duplicate key with different type`,
			format:           "toml",
			expectError:      true,
			expectedBehavior: "should handle type conflicts",
		},
		{
			name: "YAML with very deep nesting",
			content: `gommitlint:
  level1:
    level2:
      level3:
        level4:
          level5:
            level6:
              level7:
                level8:
                  level9:
                    level10:
                      max_depth: true`,
			format:           settings.ConfigFormatYAML,
			expectError:      false,
			expectedBehavior: "should handle deep nesting without stack overflow",
		},
		{
			name: "Config with UTF-8 BOM",
			content: "\xEF\xBB\xBF" + `gommitlint:
  output: "json"`,
			format:           settings.ConfigFormatYAML,
			expectError:      false,
			expectedBehavior: "should handle UTF-8 BOM correctly",
			wantOutput:       "json",
		},
		{
			name:             "Config with null bytes",
			content:          "gommitlint:\x00  output: " + output.FormatJSON,
			format:           settings.ConfigFormatYAML,
			expectError:      true,
			expectedBehavior: "should handle null bytes safely",
		},
		{
			name: "TOML with invalid escape sequences",
			content: `[gommitlint]
output = "test\q\z"`,
			format:           "toml",
			expectError:      true,
			expectedBehavior: "should handle invalid escape sequences",
		},
		{
			name: "Config with extremely long lines",
			content: `gommitlint:
  rules:
    enabled: ["` + strings.Repeat("a", 10000) + `"]`,
			format:           settings.ConfigFormatYAML,
			expectError:      false,
			expectedBehavior: "should handle very long lines without memory issues",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Write config to temp file
			tmpFile := filepath.Join(t.TempDir(), "config."+testCase.format)
			err := os.WriteFile(tmpFile, []byte(testCase.content), 0o600)
			check.NoError(t, err)

			// Parse config using strict mode
			result := configuration.ParseConfigFileStrict(tmpFile)

			if testCase.expectError {
				check.True(t, result.HasError(), testCase.expectedBehavior)
			} else {
				// Should parse without error (may have empty config for unusual content)
				check.False(t, result.HasError(), testCase.expectedBehavior)
			}

			// For partial content, valid sections must actually be parsed —
			// an empty rule list here would mean the whole file was dropped.
			if strings.Contains(testCase.name, "partial content") {
				check.Contains(t, result.Config.Rules.Enabled, "subject", "valid sections should be parsed")
			}

			// "Handled" means the content was read, not silently dropped.
			if testCase.wantOutput != "" {
				check.Equal(t, testCase.wantOutput, result.Config.Output, testCase.expectedBehavior)
			}
		})
	}
}
