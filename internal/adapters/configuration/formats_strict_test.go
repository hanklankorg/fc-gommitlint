// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"errors"
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

// Static test errors.
var errPermissionDenied = errors.New("permission denied")

// TestParseConfigFileStrict tests strict parsing behavior.
func TestParseConfigFileStrict(t *testing.T) {
	t.Parallel()

	t.Run("empty path returns error", func(t *testing.T) {
		t.Parallel()

		result := configuration.ParseConfigFileStrictWithReader("", nil)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigEmpty)
	})

	t.Run("unsupported extension returns error", func(t *testing.T) {
		t.Parallel()

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return []byte("{}"), nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.json", mockReader)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigFormat)
	})

	t.Run("file read error returns error", func(t *testing.T) {
		t.Parallel()

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return nil, errPermissionDenied
		}

		result := configuration.ParseConfigFileStrictWithReader("config.yaml", mockReader)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigUnreadable)
	})

	t.Run("file too large returns error", func(t *testing.T) {
		t.Parallel()

		largeContent := make([]byte, configuration.MaxConfigSize+1)
		mockReader := func(_ string, maxSize int64) ([]byte, error) {
			if int64(len(largeContent)) > maxSize {
				return nil, filesystem.ErrFileTooLarge
			}

			return largeContent, nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.yaml", mockReader)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigTooLarge)
	})

	t.Run("invalid YAML syntax returns error", func(t *testing.T) {
		t.Parallel()

		invalidYAML := `gommitlint:
  output: [invalid
  broken: yaml`

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return []byte(invalidYAML), nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.yaml", mockReader)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigSyntax)
	})

	t.Run("invalid TOML syntax returns error", func(t *testing.T) {
		t.Parallel()

		invalidTOML := `[gommitlint
output = "broken`

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return []byte(invalidTOML), nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.toml", mockReader)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigSyntax)
	})

	t.Run("valid YAML config returns success", func(t *testing.T) {
		t.Parallel()

		validYAML := fmt.Sprintf(`gommitlint:
  output: %s
  rules:
    enabled:
      - subject`, output.FormatJSON)

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return []byte(validYAML), nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.yaml", mockReader)
		check.False(t, result.HasError())
		check.Equal(t, output.FormatJSON, result.Config.Output)
		check.Contains(t, result.Config.Rules.Enabled, "subject")
	})

	t.Run("valid TOML config returns success", func(t *testing.T) {
		t.Parallel()

		validTOML := `[gommitlint]
output = "text"

[gommitlint.rules]
enabled = ["conventional"]`

		mockReader := func(_ string, _ int64) ([]byte, error) {
			return []byte(validTOML), nil
		}

		result := configuration.ParseConfigFileStrictWithReader("config.toml", mockReader)
		check.False(t, result.HasError())
		check.Equal(t, output.FormatText, result.Config.Output)
		check.Contains(t, result.Config.Rules.Enabled, "conventional")
	})
}

// TestParseConfigFileStrictWithFS tests strict parsing with filesystem.
func TestParseConfigFileStrictWithFS(t *testing.T) {
	t.Parallel()

	t.Run("missing file returns error", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		result := configuration.ParseConfigFileStrictWithFS("/nonexistent.yaml", afs)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigUnreadable)
	})

	t.Run("valid file returns success", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		configContent := "gommitlint:\n  output: " + output.FormatGitHub

		fs.WriteFile("/config.yaml", []byte(configContent))

		result := configuration.ParseConfigFileStrictWithFS("/config.yaml", afs)
		check.False(t, result.HasError())
		check.Equal(t, output.FormatGitHub, result.Config.Output)
	})
}

// TestParseError tests error type behavior.
func TestParseError(t *testing.T) {
	t.Parallel()

	t.Run("error message with cause only", func(t *testing.T) {
		t.Parallel()

		err := &configuration.ParseError{
			Path:  testStrictConfigPath,
			Phase: testPhaseRead,
			Cause: configuration.ErrConfigUnreadable,
		}

		check.Equal(t, "/test.yaml: read: cannot read config file", err.Error())
		check.ErrorIs(t, err, configuration.ErrConfigUnreadable)
	})

	t.Run("error message with message only", func(t *testing.T) {
		t.Parallel()

		err := &configuration.ParseError{
			Path:    testStrictConfigPath,
			Phase:   "validate",
			Message: "custom error message",
		}

		check.Equal(t, "/test.yaml: validate: custom error message", err.Error())
	})

	t.Run("error message with both message and cause", func(t *testing.T) {
		t.Parallel()

		err := &configuration.ParseError{
			Path:    testStrictConfigPath,
			Phase:   testPhaseRead,
			Message: "permission denied",
			Cause:   configuration.ErrConfigUnreadable,
		}

		check.Equal(t, "/test.yaml: read: cannot read config file: permission denied", err.Error())
		check.ErrorIs(t, err, configuration.ErrConfigUnreadable)
	})

	t.Run("unwrap returns cause", func(t *testing.T) {
		t.Parallel()

		err := &configuration.ParseError{
			Path:  testStrictConfigPath,
			Phase: "parse",
			Cause: configuration.ErrConfigSyntax,
		}

		check.ErrorIs(t, err, configuration.ErrConfigSyntax)
	})
}

// TestParseResult tests ParseResult methods.
func TestParseResult(t *testing.T) {
	t.Parallel()

	t.Run("HasError returns true when error present", func(t *testing.T) {
		t.Parallel()

		result := configuration.ParseResult{
			Error: &configuration.ParseError{Path: testStrictConfigPath, Phase: testPhaseRead},
		}
		check.True(t, result.HasError())
	})

	t.Run("HasError returns false when no error", func(t *testing.T) {
		t.Parallel()

		result := configuration.ParseResult{}
		check.False(t, result.HasError())
	})

	t.Run("HasWarnings returns true when warnings present", func(t *testing.T) {
		t.Parallel()

		result := configuration.ParseResult{
			Warnings: []domain.Warning{{Message: "warning 1"}},
		}
		check.True(t, result.HasWarnings())
	})

	t.Run("HasWarnings returns false when no warnings", func(t *testing.T) {
		t.Parallel()

		result := configuration.ParseResult{}
		check.False(t, result.HasWarnings())
	})
}

// TestHierarchyLoadingWithWarnings tests that hierarchy loading collects warnings.
func TestHierarchyLoadingWithWarnings(t *testing.T) {
	t.Parallel()

	t.Run("broken global config produces warning but continues", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create broken global config
		xdgConfigDir := `/home/user/.config`
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(`gommitlint:
  output: [broken yaml
  invalid`))

		// Create valid project config
		projectDir := `/project`
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(testhelpers.MinimalConfigYAML(output.FormatText)))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Should have warning about broken global config
		check.True(t, result.HasWarnings())
		check.Len(t, result.Warnings, 1)
		messages := configuration.WarningMessages(result.Warnings)
		check.Contains(t, messages[0], "global config")
		check.Contains(t, messages[0], "skipped")

		// Project config should still be applied
		check.Equal(t, output.FormatText, result.Config.Output)

		// Only defaults + project in sources (global was skipped)
		check.Len(t, result.Sources, 2)
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
		check.Equal(t, configuration.SourceTypeProject, result.Sources[1].Type)
	})

	t.Run("broken project config produces warning but continues", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create valid global config
		xdgConfigDir := `/home/user/.config`
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(testhelpers.MinimalConfigYAML(output.FormatJSON)))

		// Create broken project config
		projectDir := `/project`
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(`gommitlint:
  rules:
    enabled: [broken`))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Should have warning about broken project config
		check.True(t, result.HasWarnings())
		check.Len(t, result.Warnings, 1)
		messages := configuration.WarningMessages(result.Warnings)
		check.Contains(t, messages[0], "project config")
		check.Contains(t, messages[0], "skipped")

		// Global config should still be applied
		check.Equal(t, output.FormatJSON, result.Config.Output)

		// Only defaults + global in sources (project was skipped)
		check.Len(t, result.Sources, 2)
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
		check.Equal(t, configuration.SourceTypeGlobal, result.Sources[1].Type)
	})
}
