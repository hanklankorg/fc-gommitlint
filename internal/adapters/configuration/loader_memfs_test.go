// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"fmt"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

const (
	testConfigPath        = "/config/.gommitlint.yaml"
	testXDGConfigDir      = `/home/user/.config`
	testProjectDir        = `/project`
	testProjectConfigPath = testProjectDir + "/.gommitlint.yaml"
)

// TestLoadConfigFromPath_MemFS demonstrates using in-memory filesystem for configuration tests.
// This approach provides better test isolation and performance compared to real filesystem.
func TestLoadConfigFromPath_MemFS(t *testing.T) {
	t.Parallel()

	t.Run("extends local config files in merge order", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		basePath := "/project/config/base.yaml"
		rootPath := testProjectConfigPath

		fs.WriteFile(basePath, []byte(`gommitlint:
  output: text
  message:
    subject:
      max_length: 72
`))
		fs.WriteFile(rootPath, []byte(`gommitlint:
  extends:
    - config/base.yaml
  output: json
`))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(rootPath, afs)
		check.NoError(t, err)
		check.Equal(t, "json", result.Config.Output)
		check.Equal(t, 72, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
		check.Len(t, result.SourceGraph, 1)
		check.Equal(t, rootPath, result.SourceGraph[0].Path)
		check.Equal(t, configuration.SourceTypeCLI, result.SourceGraph[0].Type)
		check.Len(t, result.SourceGraph[0].Children, 1)
		check.Equal(t, basePath, result.SourceGraph[0].Children[0].Path)
		check.Equal(t, configuration.SourceTypeExtended, result.SourceGraph[0].Children[0].Type)
		check.Len(t, result.Sources, 3)
		check.Equal(t, configuration.SourceTypeExtended, result.Sources[1].Type)
		check.Equal(t, configuration.SourceTypeCLI, result.Sources[2].Type)
	})

	t.Run("detects extends cycles", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		fs.WriteFile("/project/base.yaml", []byte(`gommitlint:
  extends:
    - .gommitlint.yaml
`))
		fs.WriteFile(testProjectConfigPath, []byte(`gommitlint:
  extends:
    - base.yaml
`))

		_, err := configuration.LoadConfigFromPathDetailedWithFS(testProjectConfigPath, afs)
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrConfigExtendsCycle)
	})

	t.Run("extends built-in presets in merge order", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		rootPath := testProjectConfigPath
		fs.WriteFile(rootPath, []byte(`gommitlint:
  extends:
    - preset:conventional
    - preset:secure
  rules:
    disabled:
      - cryptosignature
`))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(rootPath, afs)
		check.NoError(t, err)
		check.Len(t, result.SourceGraph, 1)
		check.Equal(t, rootPath, result.SourceGraph[0].Path)
		check.Len(t, result.SourceGraph[0].Children, 2)
		check.Equal(t, "preset:conventional", result.SourceGraph[0].Children[0].Path)
		check.Equal(t, configuration.SourceTypePreset, result.SourceGraph[0].Children[0].Type)
		check.Equal(t, "preset:secure", result.SourceGraph[0].Children[1].Path)
		check.Equal(t, configuration.SourceTypePreset, result.SourceGraph[0].Children[1].Type)
		check.Contains(t, result.Config.Rules.Enabled, rulecatalog.Identity)
		check.Contains(t, result.Config.Rules.Disabled, rulecatalog.CryptoSignature)
	})

	t.Run("managed source destination auto-applies without extends", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		rootPath := testProjectConfigPath
		// The vendored policy file that `config sync` would have written.
		fs.WriteFile(testProjectDir+"/.gommitlint.team.yaml", []byte(`gommitlint:
  output: text
  message:
    subject:
      max_length: 72
`))
		// Config declares the managed source but NO `extends` entry.
		fs.WriteFile(rootPath, []byte(`gommitlint:
  managed_sources:
    - id: team-policy
      source: https://example.test/policy.yaml
      destination: .gommitlint.team.yaml
      integrity:
        sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  output: json
`))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(rootPath, afs)
		check.NoError(t, err)
		check.Equal(t, rootPath, result.ManagedSourcesPath)
		// Local file wins (json); the vendored base supplies max_length 72.
		check.Equal(t, "json", result.Config.Output)
		check.Equal(t, 72, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
		// The vendored file is merged as a child source — no extends needed.
		check.Len(t, result.SourceGraph[0].Children, 1)
		check.Equal(t, testProjectDir+"/.gommitlint.team.yaml", result.SourceGraph[0].Children[0].Path)
	})

	t.Run("unvendored managed source warns and still loads", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		rootPath := testProjectConfigPath
		// No destination file on disk — declared but not yet synced.
		fs.WriteFile(rootPath, []byte(`gommitlint:
  managed_sources:
    - id: team-policy
      source: https://example.test/policy.yaml
      destination: .gommitlint.team.yaml
      integrity:
        sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  output: json
`))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(rootPath, afs)
		check.NoError(t, err)
		check.Equal(t, "json", result.Config.Output)

		msgs := make([]string, 0, len(result.Warnings))
		for _, w := range result.Warnings {
			msgs = append(msgs, w.Message)
		}

		check.Contains(t, strings.Join(msgs, "\n"), "team-policy")
	})

	t.Run("managed source also in extends loads once without cycle", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		rootPath := testProjectConfigPath

		fs.WriteFile(testProjectDir+"/.gommitlint.team.yaml", []byte(`gommitlint:
  message:
    subject:
      max_length: 60`))
		// Belt-and-suspenders config: the destination is named in BOTH
		// managed_sources and extends. It must load exactly once, not fail as a
		// cycle (the managed pass skips it; the extends pass loads it).
		fs.WriteFile(rootPath, []byte(`gommitlint:
  extends:
    - .gommitlint.team.yaml
  managed_sources:
    - id: team-policy
      source: https://example.test/policy.yaml
      destination: .gommitlint.team.yaml
      integrity:
        sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(rootPath, afs)
		check.NoError(t, err)
		check.Equal(t, 60, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
		// Loaded exactly once — a single child in the source graph.
		check.Len(t, result.SourceGraph[0].Children, 1)
	})

	t.Run("complete configuration with memory filesystem", func(t *testing.T) {
		t.Parallel()

		// Create an in-memory filesystem - no disk I/O, fully isolated
		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Write configuration to memory filesystem
		configFile := testConfigPath
		configContent := fmt.Sprintf(`gommitlint:
  output: %s
  rules:
    enabled:
      - subject
      - conventional
    disabled:
      - spell
  message:
    subject:
      max_length: 72
      min_length: 5`, "json")

		fs.WriteFile(configFile, []byte(configContent))

		// Load configuration using the memory filesystem
		cfg, err := configuration.LoadConfigFromPathWithFS(configFile, afs)
		check.NoError(t, err)

		// Verify configuration was loaded correctly
		check.Equal(t, "json", cfg.Output)
		check.Contains(t, cfg.Rules.Enabled, "subject")
		check.Contains(t, cfg.Rules.Enabled, "conventional")
		check.Contains(t, cfg.Rules.Disabled, "spell")
		check.Equal(t, 72, settings.DerefInt(cfg.Message.Subject.MaxLength, 0))
	})

	t.Run("TOML configuration with memory filesystem", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		configFile := testProjectDir + "/.gommitlint.toml"
		configContent := fmt.Sprintf(`[gommitlint]
output = "%s"

[gommitlint.rules]
enabled = ["subject", "signoff"]
disabled = ["jirareference"]

[gommitlint.message.subject]
max_length = 50`, "text")

		fs.WriteFile(configFile, []byte(configContent))

		cfg, err := configuration.LoadConfigFromPathWithFS(configFile, afs)
		check.NoError(t, err)

		check.Equal(t, "text", cfg.Output)
		check.Contains(t, cfg.Rules.Enabled, "signoff")
		check.Contains(t, cfg.Rules.Disabled, "jirareference")
		check.Equal(t, 50, settings.DerefInt(cfg.Message.Subject.MaxLength, 0))
	})

	t.Run("missing configuration file returns error", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// With strict parsing, missing file should return error
		_, err := configuration.LoadConfigFromPathWithFS("/missing.yaml", afs)
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrConfigUnreadable)
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		configFile := "/invalid.yaml"
		configContent := fmt.Sprintf(`gommitlint:
  output: %s
  rules:
    enabled: [
      - this is invalid YAML`, "json")

		fs.WriteFile(configFile, []byte(configContent))

		// With strict parsing, invalid YAML should return error
		_, err := configuration.LoadConfigFromPathWithFS(configFile, afs)
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrConfigSyntax)
	})

	t.Run("loads each config file by its own path", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Three config files at different depths, each loaded directly; no
		// discovery or precedence between them is involved.
		configs := map[string]string{
			"/.gommitlint.yaml":                testhelpers.MinimalConfigYAML(testOutputText),
			testProjectConfigPath:              testhelpers.MinimalConfigYAML("json"),
			"/project/subdir/.gommitlint.yaml": testhelpers.MinimalConfigYAML("github"),
		}

		for path, content := range configs {
			fs.WriteFile(path, []byte(content))
		}

		// Test loading from different paths
		tests := []struct {
			path           string
			expectedOutput string
		}{
			{"/.gommitlint.yaml", testOutputText},
			{testProjectConfigPath, "json"},
			{"/project/subdir/.gommitlint.yaml", "github"},
		}

		for _, testCase := range tests {
			cfg, err := configuration.LoadConfigFromPathWithFS(testCase.path, afs)
			check.NoError(t, err)
			check.Equal(t, testCase.expectedOutput, cfg.Output, "Config from %s", testCase.path)
		}
	})
}

// TestLoadConfig_UnparseableFileWarnsAndFallsBack covers a skip cause the other
// skip tests do not reach, and the warning detail none of them assert.
//
// loadConfigSource raises one WarnConfigParse for every reason a config file is
// passed over. TestLoadConfigHierarchy_SkipsInsecureConfig and
// SkipsSymlinkConfig already drive that path, but both are about security
// rejections: the file parses fine and is refused on its permissions or because
// it is a symlink. Malformed YAML is the other way in, and it is the one a user
// causes by hand.
//
// The cause matters as much as the code. "Could not parse X" without the YAML
// error underneath leaves users guessing, which is why the loader carries it in
// WarningContextCause -- and blanking that field fails only this test, so
// nothing else was holding it.
func TestLoadConfig_UnparseableFileWarnsAndFallsBack(t *testing.T) {
	t.Parallel()

	fs := testhelpers.NewMemoryTestFS(t)
	fs.CreateDir(testProjectDir)
	fs.WriteFile(testProjectConfigPath, []byte("gommitlint:\n  output: [unterminated\n"))

	result, err := configuration.LoadConfigHierarchyWithEnvAndFS(
		testProjectDir, "",
		configuration.NewMapEnvironment(map[string]string{"XDG_CONFIG_HOME": testXDGConfigDir}),
		signing.NewAferoAdapter(fs.Fs),
	)

	// Skipping the file is the documented behaviour, not failing the load.
	check.NoError(t, err)
	check.True(t, result.HasWarnings(), "an unparseable config must be reported, not silently skipped")

	var parseWarning domain.Warning

	for _, warning := range result.Warnings {
		if warning.Code == domain.WarnConfigParse {
			parseWarning = warning

			break
		}
	}

	check.Equal(t, domain.WarnConfigParse, parseWarning.Code,
		"the skipped file should raise a config_parse warning")
	check.Contains(t, parseWarning.Message, testProjectConfigPath,
		"the warning should name the file that was skipped")
	check.NotEmpty(t, parseWarning.Context[domain.WarningContextCause],
		"the underlying parse error is the actionable part")

	// And the effective config is the defaults, unaffected by the broken file.
	check.Equal(t, configuration.NewConfigWithDefaults().Output, result.Config.Output)
}

func TestLoadConfigHierarchy_ManagedSourcesOwnership(t *testing.T) {
	t.Parallel()

	const globalPath = testXDGConfigDir + "/gommitlint/settings.yaml"

	managedSource := `
  managed_sources:
    - id: %s
      source: https://example.test/policy.yaml
      destination: policy.yaml
      integrity:
        sha256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
`

	t.Run("global declaration remains owned by global file", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		fs.CreateDir(testXDGConfigDir + "/gommitlint")
		fs.CreateDir(testProjectDir)
		fs.WriteFile(globalPath, []byte("gommitlint:\n  output: text"+fmt.Sprintf(managedSource, "global")))
		fs.WriteFile(testProjectConfigPath, []byte("gommitlint:\n  output: json\n"))

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(testProjectDir, "", configuration.NewMapEnvironment(map[string]string{"XDG_CONFIG_HOME": testXDGConfigDir}), signing.NewAferoAdapter(fs.Fs))
		check.NoError(t, err)
		check.Equal(t, globalPath, result.ManagedSourcesPath)
		check.Equal(t, "global", result.Config.ManagedSources[0].ID)
		check.Equal(t, "json", result.Config.Output)
	})

	t.Run("project declaration owns project sources", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		fs.CreateDir(testXDGConfigDir + "/gommitlint")
		fs.CreateDir(testProjectDir)
		fs.WriteFile(globalPath, []byte("gommitlint:"+fmt.Sprintf(managedSource, "global")))
		fs.WriteFile(testProjectConfigPath, []byte("gommitlint:"+fmt.Sprintf(managedSource, "project")))

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(testProjectDir, "", configuration.NewMapEnvironment(map[string]string{"XDG_CONFIG_HOME": testXDGConfigDir}), signing.NewAferoAdapter(fs.Fs))
		check.NoError(t, err)
		check.Equal(t, testProjectConfigPath, result.ManagedSourcesPath)
		check.Equal(t, "project", result.Config.ManagedSources[0].ID)
	})

	t.Run("extends declaration is rejected", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		fs.CreateDir(testProjectDir)
		fs.WriteFile(testProjectDir+"/base.yaml", []byte("gommitlint:\n  output: json"+fmt.Sprintf(managedSource, "extended")))
		fs.WriteFile(testProjectConfigPath, []byte("gommitlint:\n  extends: [base.yaml]\n"))

		result, err := configuration.LoadConfigFromPathDetailedWithFS(testProjectConfigPath, signing.NewAferoAdapter(fs.Fs))
		check.ErrorIs(t, err, configuration.ErrConfigValidation)
		check.Empty(t, result.ManagedSourcesPath)
		check.Empty(t, result.Config.ManagedSources)
	})
}

// TestParseConfigFileStrictWithFS_MemFS demonstrates direct parsing with memory filesystem.
func TestParseConfigFileStrictWithFS_MemFS(t *testing.T) {
	t.Parallel()

	t.Run("parse various config formats", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		tests := []struct {
			filename string
			content  string
			expected string
		}{
			{
				filename: "/config.yaml",
				content: `gommitlint:
  output: yaml_format`,
				expected: "yaml_format",
			},
			{
				filename: "/config.yml",
				content: `gommitlint:
  output: yml_format`,
				expected: "yml_format",
			},
			{
				filename: "/config.toml",
				content: `[gommitlint]
output = "toml_format"`,
				expected: "toml_format",
			},
		}

		for _, testCase := range tests {
			fs.WriteFile(testCase.filename, []byte(testCase.content))
			result := configuration.ParseConfigFileStrictWithFS(testCase.filename, afs)
			check.False(t, result.HasError(), "Format: %s", testCase.filename)
			check.Equal(t, testCase.expected, result.Config.Output, "Format: %s", testCase.filename)
		}
	})

	t.Run("empty filesystem returns error", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// No files written to filesystem
		result := configuration.ParseConfigFileStrictWithFS("/any/path.yaml", afs)
		check.True(t, result.HasError())
		check.ErrorIs(t, result.Error, configuration.ErrConfigUnreadable)
	})
}

func TestLoadConfigFromPathWithFS_DisallowBreaking(t *testing.T) {
	t.Parallel()

	t.Run("disallow_breaking_true_from_config_file", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		configContent := `gommitlint:
  conventional:
    disallow_breaking: true`

		fs.WriteFile(testConfigPath, []byte(configContent))

		cfg, err := configuration.LoadConfigFromPathWithFS(testConfigPath, afs)
		check.NoError(t, err)
		check.True(t, settings.DerefBool(cfg.Conventional.DisallowBreaking, false), "disallow_breaking: true should be applied")
	})

	t.Run("default_disallow_breaking_is_false", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Config file without disallow_breaking setting
		configContent := `gommitlint:
  conventional:
    require_scope: false`

		fs.WriteFile(testConfigPath, []byte(configContent))

		cfg, err := configuration.LoadConfigFromPathWithFS(testConfigPath, afs)
		check.NoError(t, err)
		check.False(t, settings.DerefBool(cfg.Conventional.DisallowBreaking, false), "default disallow_breaking should be false")
	})
}

// TestLoadConfigHierarchyWithFS tests hierarchical config merging.
func TestLoadConfigHierarchyWithFS(t *testing.T) {
	t.Parallel()

	t.Run("merges global and project configs", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create global config with some settings
		xdgConfigDir := testXDGConfigDir
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(fmt.Sprintf(`gommitlint:
  output: %s
  message:
    subject:
      max_length: 100`, "json")))

		// Create project config that overrides some settings
		projectDir := testProjectDir
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(`gommitlint:
  message:
    subject:
      max_length: 72`))

		// Use custom environment pointing to the memory filesystem paths
		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		// Load using hierarchy with custom env and fs
		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Project config should override global
		check.Equal(t, 72, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
		// Global output setting should be preserved (not overridden by project)
		check.Equal(t, "json", result.Config.Output)

		// Check sources are tracked
		check.Len(t, result.Sources, 3) // defaults + global + project
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
		check.Equal(t, configuration.SourceTypeGlobal, result.Sources[1].Type)
		check.Equal(t, configuration.SourceTypeProject, result.Sources[2].Type)
	})

	t.Run("global config may extend an absolute path outside its dir", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		xdgConfigDir := testXDGConfigDir
		// Central policy on the host, outside the XDG and project trees.
		fs.WriteFile("/etc/gommitlint/policy.yaml", []byte(`gommitlint:
  message:
    subject:
      max_length: 50`))
		// The global config is trusted, so it may extend the absolute path —
		// this is how an operator applies one central policy to every repo.
		fs.WriteFile(xdgConfigDir+`/gommitlint/settings.yaml`, []byte(`gommitlint:
  extends:
    - /etc/gommitlint/policy.yaml`))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(testProjectDir, "", env, afs)
		check.NoError(t, err)
		check.Equal(t, 50, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))
	})

	t.Run("project config may not extend outside its subtree", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		fs.WriteFile("/etc/gommitlint/policy.yaml", []byte(`gommitlint:
  message:
    subject:
      max_length: 50`))
		// An untrusted project config (could be a cloned repo) stays confined.
		fs.WriteFile(testProjectDir+"/.gommitlint.yaml", []byte(`gommitlint:
  extends:
    - /etc/gommitlint/policy.yaml`))

		env := configuration.NewMapEnvironment(map[string]string{})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(testProjectDir, "", env, afs)
		// Hierarchy loader degrades gracefully: the offending source is skipped
		// with a warning rather than failing the whole load.
		check.NoError(t, err)
		// The outside policy must NOT have been applied.
		check.NotEqual(t, 50, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))

		msgs := make([]string, 0, len(result.Warnings))
		for _, w := range result.Warnings {
			msgs = append(msgs, w.Message)
		}

		check.Contains(t, strings.Join(msgs, "\n"), "/etc/gommitlint/policy.yaml")
	})

	t.Run("only defaults when no config files exist", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Empty environment - no XDG paths
		env := configuration.NewMapEnvironment(map[string]string{})

		// Empty filesystem - no config files
		result, err := configuration.LoadConfigHierarchyWithEnvAndFS("/empty/project", "", env, afs)
		check.NoError(t, err)

		check.Empty(t, result.Config.Rules.Enabled)
		check.Empty(t, result.Config.Rules.Disabled)

		// Only defaults source
		check.Len(t, result.Sources, 1)
		check.Equal(t, "defaults", result.Sources[0].Path)
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
	})

	t.Run("only global config when no project config", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create only global config
		xdgConfigDir := testXDGConfigDir
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(fmt.Sprintf(`gommitlint:
  output: %s
  message:
    subject:
      max_length: 80`, "github")))

		// Project dir exists but has no config
		projectDir := testProjectDir
		check.NoError(t, fs.MkdirAll(projectDir, 0o755))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Should have global settings merged with defaults
		check.Equal(t, "github", result.Config.Output)
		check.Equal(t, 80, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))

		// Defaults + global sources
		check.Len(t, result.Sources, 2)
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
		check.Equal(t, configuration.SourceTypeGlobal, result.Sources[1].Type)
	})

	t.Run("invalid ignore_commits generates warning", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		projectDir := testProjectDir
		check.NoError(t, fs.MkdirAll(projectDir, 0o755))
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(`gommitlint:
  ignore_commits:
    - "bad"
    - "a1b2c3d"`))

		env := configuration.NewMapEnvironment(map[string]string{})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		messages := configuration.WarningMessages(result.Warnings)
		check.Contains(t, messages, "Invalid SHA in ignore_commits: bad")
		check.Equal(t, []string{"bad", "a1b2c3d"}, result.Config.IgnoreCommits)
	})

	t.Run("only project config when no global config", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Empty environment - no XDG paths
		env := configuration.NewMapEnvironment(map[string]string{})

		// Create only project config
		projectDir := testProjectDir
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(fmt.Sprintf(`gommitlint:
  output: %s
  message:
    subject:
      max_length: 50`, "text")))

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Should have project settings merged with defaults
		check.Equal(t, "text", result.Config.Output)
		check.Equal(t, 50, settings.DerefInt(result.Config.Message.Subject.MaxLength, 0))

		// Defaults + project sources
		check.Len(t, result.Sources, 2)
		check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
		check.Equal(t, configuration.SourceTypeProject, result.Sources[1].Type)
	})

	t.Run("project can disable boolean that global enabled", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Global enables a boolean setting
		xdgConfigDir := testXDGConfigDir
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(`gommitlint:
  conventional:
    require_scope: true`))

		// Project disables it
		projectDir := testProjectDir
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(`gommitlint:
  conventional:
    require_scope: false`))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		// Project config should override global - require_scope should be false
		check.False(t, settings.DerefBool(result.Config.Conventional.RequireScope, true))
	})

	t.Run("SourcesSummary returns human-readable string", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create both configs
		xdgConfigDir := testXDGConfigDir
		globalConfig := xdgConfigDir + `/gommitlint/settings.yaml`
		fs.WriteFile(globalConfig, []byte(testhelpers.MinimalConfigYAML("json")))

		projectDir := testProjectDir
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(testhelpers.MinimalConfigYAML("text")))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)
		check.NoError(t, err)

		summary := result.SourcesSummary()
		// Should show in reverse order (highest priority first)
		check.Contains(t, summary, ".gommitlint.yaml")
		check.Contains(t, summary, "settings.yaml")
	})

	t.Run("validation failure returns error", func(t *testing.T) {
		t.Parallel()

		fs := testhelpers.NewMemoryTestFS(t)
		afs := signing.NewAferoAdapter(fs.Fs)

		// Create config with invalid value (e.g., negative max_length)
		projectDir := testProjectDir
		projectConfig := projectDir + "/.gommitlint.yaml"
		fs.WriteFile(projectConfig, []byte(`gommitlint:
  message:
    subject:
      max_length: -10`))

		env := configuration.NewMapEnvironment(map[string]string{})

		result, err := configuration.LoadConfigHierarchyWithEnvAndFS(projectDir, "", env, afs)

		// Should return validation error
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrConfigValidation)
		check.Empty(t, result.Config.Output)
	})
}

// The rules-overlap warning is a property of the merged config, not of how the
// file was found: an explicit --config path must warn like the hierarchy does.
func TestLoadConfigFromPathDetailed_WarnsOnRulesOverlap(t *testing.T) {
	t.Parallel()

	fs := testhelpers.NewMemoryTestFS(t)
	afs := signing.NewAferoAdapter(fs.Fs)

	fs.WriteFile(testProjectConfigPath, []byte(`gommitlint:
  rules:
    enabled: [spell]
    disabled: [spell]
`))

	result, err := configuration.LoadConfigFromPathDetailedWithFS(testProjectConfigPath, afs)
	check.NoError(t, err)

	var overlap []domain.Warning

	for _, warning := range result.Warnings {
		if warning.Code == domain.WarnConfigRulesOverlap {
			overlap = append(overlap, warning)
		}
	}

	check.Len(t, overlap, 1)
	check.Equal(t, "spell", overlap[0].Context[domain.WarningContextRules])
}
