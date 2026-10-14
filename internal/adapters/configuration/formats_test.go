// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

// TestParseConfigFileStrict_ToleratesMinimalConfigs covers lenient shapes that
// must parse without error; syntax errors, IO errors, and full happy paths are
// covered in formats_strict_test.go.
func TestParseConfigFileStrict_ToleratesMinimalConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		content  string
	}{
		{
			name:     "empty YAML config",
			filename: "test-configuration.yaml",
			content:  testYAMLRoot,
		},
		{
			name:     "YAML without gommitlint root",
			filename: "test-configuration.yaml",
			content: `rules:
  enabled:
    - subject`,
		},
		{
			name:     "empty TOML config",
			filename: "test-configuration.toml",
			content:  `[gommitlint]`,
		},
		{
			name:     "yml extension is accepted as YAML",
			filename: "test-configuration.yml",
			content:  testYAMLRoot,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mockReader := func(_ string, _ int64) ([]byte, error) {
				return []byte(testCase.content), nil
			}

			result := configuration.ParseConfigFileStrictWithReader(testCase.filename, mockReader)
			check.False(t, result.HasError())
		})
	}
}

// A file without a `gommitlint:` mapping parses as empty, so the only thing
// telling the user their config is ignored is the bad-shape warning. An empty
// root key is the non-mapping shape that gets that far; a scalar or list root
// is refused as a structure error before any warning is produced.
func TestParseConfigBytesStrict_WarnsWhenTheRootKeyIsMissingOrNotAMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		content   string
		wantIssue string
		wantErr   error
	}{
		{
			name:      "missing root key",
			content:   "rules:\n  enabled:\n    - subject\n",
			wantIssue: "missing_root_key",
		},
		{
			name:      "empty root key is not a mapping",
			content:   "gommitlint:\n",
			wantIssue: "root_not_a_map",
		},
		{
			name:    "scalar root is a structure error",
			content: "gommitlint: 3\n",
			wantErr: configuration.ErrConfigStructure,
		},
		{
			name:    "the control: a mapping root",
			content: "gommitlint:\n  output: json\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := configuration.ParseConfigBytesStrict("config.yaml", []byte(testCase.content))

			if testCase.wantErr != nil {
				check.ErrorIs(t, result.Error, testCase.wantErr)
				check.Empty(t, result.Warnings)

				return
			}

			check.False(t, result.HasError())

			if testCase.wantIssue == "" {
				check.Empty(t, result.Warnings)

				return
			}

			check.Len(t, result.Warnings, 1)
			check.Equal(t, domain.WarnConfigBadShape, result.Warnings[0].Code)
			check.Equal(t, testCase.wantIssue, result.Warnings[0].Context[domain.WarningContextIssue])
			check.Equal(t, "config.yaml", result.Warnings[0].Context[domain.WarningContextPath])
		})
	}
}

func TestParseConfigFileStrict_UnknownNestedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content string
		want    []string
	}{
		{
			name: "yaml nested structs and managed sources",
			path: "config.yaml",
			content: `gommitlint:
  message:
    subject:
      max_lenght: 50
  managed_sources:
    - id: team
      source: https://example.test/policy.yaml
      destination: policy.yaml
      headers:
        X-Custom-Header: literal
      integrity:
        sha25x: abc
`,
			want: []string{"managed_sources[0].integrity.sha25x", "message.subject.max_lenght"},
		},
		{
			name: "toml nested struct",
			path: "config.toml",
			content: `[gommitlint.message.body]
min_signoff_cont = 2
`,
			want: []string{"message.body.min_signoff_cont"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := configuration.ParseConfigBytesStrict(testCase.path, []byte(testCase.content))
			check.False(t, result.HasError())
			check.Len(t, result.Warnings, len(testCase.want))

			for i, want := range testCase.want {
				check.Equal(t, want, result.Warnings[i].Context["key"])
			}
		})
	}
}

// TestParseConfigFileStrictWithFS_ParsesYAMLFields asserts the parsed field
// values coming out of a YAML file read through the filesystem adapter.
func TestParseConfigFileStrictWithFS_ParsesYAMLFields(t *testing.T) {
	t.Parallel()

	yamlContent := `gommitlint:
  rules:
    enabled:
      - subject
      - conventional
  message:
    subject:
      max_length: 72
  output: yaml_test`

	fs := testhelpers.NewMemoryTestFS(t)
	afs := signing.NewAferoAdapter(fs.Fs)

	tmpFile := "/test-config.yaml"
	fs.WriteFile(tmpFile, []byte(yamlContent))

	result := configuration.ParseConfigFileStrictWithFS(tmpFile, afs)
	check.False(t, result.HasError())
	check.Equal(t, "yaml_test", result.Config.Output)
	check.Equal(t, 72, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
	check.Contains(t, result.Config.Rules.Enabled, "subject")
	check.Contains(t, result.Config.Rules.Enabled, "conventional")
}

// TestParseConfigFileStrictWithFS_ParsesTOMLFields mirrors the YAML variant for TOML.
func TestParseConfigFileStrictWithFS_ParsesTOMLFields(t *testing.T) {
	t.Parallel()

	tomlContent := `[gommitlint]
output = "toml_test"

[gommitlint.rules]
enabled = ["subject", "imperative"]
disabled = ["spell"]

[gommitlint.message.subject]
max_length = 80
require_imperative = true`

	fs := testhelpers.NewMemoryTestFS(t)
	afs := signing.NewAferoAdapter(fs.Fs)

	tmpFile := "/test-config.toml"
	fs.WriteFile(tmpFile, []byte(tomlContent))

	result := configuration.ParseConfigFileStrictWithFS(tmpFile, afs)
	check.False(t, result.HasError())
	check.Equal(t, "toml_test", result.Config.Output)
	check.Equal(t, 80, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
	check.True(t, settings.DerefBool(result.Config.Message.Subject.RequireImperative, false))
	check.Contains(t, result.Config.Rules.Enabled, "subject")
	check.Contains(t, result.Config.Rules.Enabled, "imperative")
	check.Contains(t, result.Config.Rules.Disabled, "spell")
}
