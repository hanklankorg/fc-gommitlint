// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestLoadConfigHierarchy_SkipsInsecureConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".gommitlint.yaml")
	configContent := testhelpers.MinimalConfigYAML("json")

	check.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o666))
	check.NoError(t, os.Chmod(configPath, 0o666))

	env := configuration.NewMapEnvironment(map[string]string{})
	result, err := configuration.LoadConfigHierarchyWithEnvAndFS(tmpDir, "", env, filesystem.NewOSFileSystem())
	check.NoError(t, err)

	check.Len(t, result.Warnings, 1)
	messages := configuration.WarningMessages(result.Warnings)
	check.Contains(t, messages[0], "world-writable")
	check.Len(t, result.Sources, 1)
	check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
}

func TestLoadConfigHierarchy_SkipsSymlinkConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configTarget := filepath.Join(tmpDir, "config.yaml")
	configPath := filepath.Join(tmpDir, ".gommitlint.yaml")
	configContent := testhelpers.MinimalConfigYAML("json")

	check.NoError(t, os.WriteFile(configTarget, []byte(configContent), 0o600))

	if err := os.Symlink(configTarget, configPath); err != nil {
		t.Skip("symlinks not supported")
	}

	env := configuration.NewMapEnvironment(map[string]string{})
	result, err := configuration.LoadConfigHierarchyWithEnvAndFS(tmpDir, "", env, filesystem.NewOSFileSystem())
	check.NoError(t, err)

	check.Len(t, result.Warnings, 1)
	messages := configuration.WarningMessages(result.Warnings)
	check.Contains(t, messages[0], "symlink")
	check.Len(t, result.Sources, 1)
	check.Equal(t, configuration.SourceTypeDefaults, result.Sources[0].Type)
}

func TestLoadConfigHierarchy_RejectsExtendedConfigThroughSymlinkedDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	outsideDir := filepath.Join(root, "outside")

	check.NoError(t, os.MkdirAll(projectDir, 0o750))
	check.NoError(t, os.MkdirAll(outsideDir, 0o750))
	check.NoError(t, os.WriteFile(
		filepath.Join(projectDir, ".gommitlint.yaml"),
		[]byte("gommitlint:\n  extends: [policies/base.yaml]\n"),
		0o600,
	))
	check.NoError(t, os.WriteFile(
		filepath.Join(outsideDir, "base.yaml"),
		[]byte(testhelpers.MinimalConfigYAML("json")),
		0o600,
	))

	if err := os.Symlink(outsideDir, filepath.Join(projectDir, "policies")); err != nil {
		t.Skip("symlinks not supported")
	}

	result, err := configuration.LoadConfigHierarchyWithEnvAndFS(
		projectDir,
		"",
		configuration.NewMapEnvironment(map[string]string{}),
		filesystem.NewOSFileSystem(),
	)
	check.NoError(t, err)
	check.Equal(t, configuration.NewConfigWithDefaults().Output, result.Config.Output)
	check.Len(t, result.Warnings, 1)
	check.Contains(t, result.Warnings[0].Message, configuration.ErrConfigExtendsEscape.Error())
}

func TestLoadConfigHierarchy_RejectsInvalidMinSignoffCount(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	configPath := filepath.Join(projectDir, ".gommitlint.yaml")
	configContent := `gommitlint:
  message:
    body:
      min_signoff_count: 11
`
	check.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	_, err := configuration.LoadConfigHierarchyWithEnvAndFS(
		projectDir,
		"",
		configuration.NewMapEnvironment(map[string]string{}),
		filesystem.NewOSFileSystem(),
	)
	check.ErrorIs(t, err, configuration.ErrConfigValidation)
	check.ErrorContains(t, err, "min_signoff_count")
}
