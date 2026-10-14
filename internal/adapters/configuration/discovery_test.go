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
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestGetConfigPaths_NoRepoPath_ReturnsCurrentDirConfigs(t *testing.T) {
	t.Parallel()

	t.Run("current directory paths", func(t *testing.T) {
		t.Parallel()

		paths := configuration.GetConfigPaths("", "", configuration.NewMapEnvironment(map[string]string{}))
		expected := []string{
			".gommitlint.yaml",
			".gommitlint.yml",
			".gommitlint.toml",
		}
		check.Equal(t, expected, paths)
	})
}

func TestGetConfigPaths_WithRepoPath_ReturnsRepoSpecificConfigs(t *testing.T) {
	t.Parallel()

	t.Run("absolute repo path", func(t *testing.T) {
		t.Parallel()

		testDir := filepath.Join(t.TempDir(), "test-repo")
		paths := configuration.GetConfigPaths(testDir, "", configuration.OSEnvironment{})
		check.Contains(t, paths[0], filepath.Join(testDir, ".gommitlint.yaml"))
	})

	t.Run("suspicious path falls back to current directory", func(t *testing.T) {
		t.Parallel()

		paths := configuration.GetConfigPaths("../../../etc", "", configuration.OSEnvironment{})
		check.Contains(t, paths[0], ".gommitlint.yaml")
		check.NotContains(t, paths[0], "../")
	})
}

func TestGetConfigPaths_WithXDGVariables_IncludesXDGPaths(t *testing.T) {
	t.Run("without XDG_CONFIG_HOME or HOME", func(t *testing.T) {
		env := configuration.NewMapEnvironment(map[string]string{})

		paths := configuration.GetConfigPaths("", "", env)
		check.Len(t, paths, 3)

		for _, path := range paths {
			check.NotContains(t, path, "XDG")
			check.NotContains(t, path, ".config")
		}
	})

	t.Run("with HOME but without XDG_CONFIG_HOME", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir
		homeDir := filepath.Join(tmpDir, "home")
		configDir := filepath.Join(homeDir, ".config")
		gommitlintDir := filepath.Join(configDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)

		env := configuration.NewMapEnvironment(map[string]string{
			"HOME": homeDir,
		})

		paths := configuration.GetConfigPaths("", "", env)
		check.Greater(t, len(paths), 3)

		found := false

		for _, path := range paths {
			if strings.Contains(path, filepath.Join(homeDir, ".config", "gommitlint")) {
				found = true

				break
			}
		}

		check.True(t, found, "HOME/.config/gommitlint paths should be included when XDG_CONFIG_HOME is not set")
	})

	t.Run("XDG_CONFIG_HOME takes precedence over HOME", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir

		homeDir := filepath.Join(tmpDir, "home")
		homeConfigDir := filepath.Join(homeDir, ".config")
		homeGommitlintDir := filepath.Join(homeConfigDir, "gommitlint")
		testEnv.FS.CreateDir(homeGommitlintDir)

		xdgConfigDir := filepath.Join(tmpDir, "xdg-config")
		xdgGommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(xdgGommitlintDir)

		env := configuration.NewMapEnvironment(map[string]string{
			"HOME":               homeDir,
			testEnvXDGConfigHome: xdgConfigDir,
		})

		paths := configuration.GetConfigPaths("", "", env)
		check.Greater(t, len(paths), 3)

		foundXDG := false
		foundHome := false

		for _, path := range paths {
			if strings.Contains(path, xdgConfigDir) {
				foundXDG = true
			}

			if strings.Contains(path, filepath.Join(homeDir, ".config")) {
				foundHome = true
			}
		}

		check.True(t, foundXDG, "XDG_CONFIG_HOME paths should be included")
		check.False(t, foundHome, "HOME/.config paths should NOT be included when XDG_CONFIG_HOME is set")
	})

	t.Run("with invalid XDG_CONFIG_HOME", func(t *testing.T) {
		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: "../dangerous/path",
		})

		paths := configuration.GetConfigPaths("", "", env)
		check.Len(t, paths, 3)
	})

	t.Run("with XDG_CONFIG_HOME containing double dots in name", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir
		xdgConfigDir := filepath.Join(tmpDir, "config..dir")
		gommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		paths := configuration.GetConfigPaths("", "", env)
		check.Greater(t, len(paths), 3)
	})

	t.Run("with valid XDG_CONFIG_HOME", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir
		xdgConfigDir := filepath.Join(tmpDir, "xdg-config")
		gommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		paths := configuration.GetConfigPaths("", "", env)
		check.Greater(t, len(paths), 3)

		found := false

		for _, path := range paths {
			if strings.Contains(path, "xdg-config") {
				found = true

				break
			}
		}

		check.True(t, found, "XDG config paths should be included")
	})
}

func TestFindAllExistingConfigFiles(t *testing.T) {
	t.Run("finds only project config when no global config", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		projectDir := testEnv.TempDir

		testEnv.FS.CreateDir(projectDir)
		testEnv.FS.WriteFile(filepath.Join(projectDir, ".gommitlint.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatText)))

		env := configuration.NewMapEnvironment(map[string]string{})

		found := configuration.FindAllExistingConfigFilesWithEnv(projectDir, "", env)

		check.Empty(t, found.Global, "should have no global config")
		check.Equal(t, filepath.Join(projectDir, ".gommitlint.yaml"), found.Project)
	})

	t.Run("finds only global config when no project config", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir

		// Create global config only
		xdgConfigDir := filepath.Join(tmpDir, "xdg-config")
		gommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)
		testEnv.FS.WriteFile(filepath.Join(gommitlintDir, "settings.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatJSON)))

		// Project dir exists but has no config
		projectDir := filepath.Join(tmpDir, "project")
		testEnv.FS.CreateDir(projectDir)

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		found := configuration.FindAllExistingConfigFilesWithEnv(projectDir, "", env)

		check.Equal(t, filepath.Join(gommitlintDir, "settings.yaml"), found.Global)
		check.Empty(t, found.Project, "should have no project config")
	})

	t.Run("finds both global and project configs", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir

		// Create global config
		xdgConfigDir := filepath.Join(tmpDir, "xdg-config")
		gommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)
		testEnv.FS.WriteFile(filepath.Join(gommitlintDir, "settings.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatJSON)))

		// Create project config
		projectDir := filepath.Join(tmpDir, "project")
		testEnv.FS.CreateDir(projectDir)
		testEnv.FS.WriteFile(filepath.Join(projectDir, ".gommitlint.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatText)))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		found := configuration.FindAllExistingConfigFilesWithEnv(projectDir, "", env)

		check.Equal(t, filepath.Join(gommitlintDir, "settings.yaml"), found.Global)
		check.Equal(t, filepath.Join(projectDir, ".gommitlint.yaml"), found.Project)
	})

	t.Run("finds no configs when none exist", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		projectDir := testEnv.TempDir

		// Empty environment
		env := configuration.NewMapEnvironment(map[string]string{})

		found := configuration.FindAllExistingConfigFilesWithEnv(projectDir, "", env)

		check.Empty(t, found.Global)
		check.Empty(t, found.Project)
	})

	// A relative --repo-path is resolved from the caller's working directory,
	// the way a hook run from a subdirectory sees it, not from the process cwd.
	t.Run("relative repo path resolves from the working directory", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		workingDir := testEnv.TempDir

		projectDir := filepath.Join(workingDir, "sub")
		testEnv.FS.CreateDir(projectDir)
		testEnv.FS.WriteFile(filepath.Join(projectDir, ".gommitlint.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatText)))

		env := configuration.NewMapEnvironment(map[string]string{})

		found := configuration.FindAllExistingConfigFilesWithEnv("sub", workingDir, env)

		check.Equal(t, filepath.Join(projectDir, ".gommitlint.yaml"), found.Project)
		check.Empty(t, found.Global)

		// The control: without a working directory the same relative path is
		// anchored at the process cwd, where that config does not exist.
		cwd, err := os.Getwd()
		check.NoError(t, err)
		check.Equal(t, filepath.Join(cwd, "sub", ".gommitlint.yaml"), configuration.GetConfigPaths("sub", "", env)[0])
		check.Empty(t, configuration.FindAllExistingConfigFilesWithEnv("sub", "", env).Project)
	})

	t.Run("Sources returns correct ConfigSource entries", func(t *testing.T) {
		testEnv := testhelpers.NewRealFSTestEnv(t)
		tmpDir := testEnv.TempDir

		// Create both configs
		xdgConfigDir := filepath.Join(tmpDir, "xdg-config")
		gommitlintDir := filepath.Join(xdgConfigDir, "gommitlint")
		testEnv.FS.CreateDir(gommitlintDir)
		testEnv.FS.WriteFile(filepath.Join(gommitlintDir, "settings.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatJSON)))

		projectDir := filepath.Join(tmpDir, "project")
		testEnv.FS.CreateDir(projectDir)
		testEnv.FS.WriteFile(filepath.Join(projectDir, ".gommitlint.yaml"), []byte(testhelpers.MinimalConfigYAML(output.FormatText)))

		env := configuration.NewMapEnvironment(map[string]string{
			testEnvXDGConfigHome: xdgConfigDir,
		})

		found := configuration.FindAllExistingConfigFilesWithEnv(projectDir, "", env)
		sources := found.Sources()

		check.Len(t, sources, 2)
		check.Equal(t, configuration.SourceTypeGlobal, sources[0].Type)
		check.Equal(t, configuration.SourceTypeProject, sources[1].Type)
	})
}
