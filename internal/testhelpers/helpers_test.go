// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testhelpers_test

import (
	"os"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"gopkg.in/yaml.v3"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestWriteFile_CreatesParentDirectories(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	filePath := "/test/subdir/test.txt"

	// Write file (parent directory should be created automatically)
	env.FS.WriteFile(filePath, []byte("test content"))

	// Verify file exists and has correct content in memory filesystem
	check.True(t, env.FS.FileExists(filePath))
	content := env.FS.ReadFile(filePath)
	check.Equal(t, "test content", content)
}

// TestCreateTemp_RefusesOnTheMemoryFilesystem pins the refusal that the memory
// adapter is built around.
//
// CreateTemp has to hand back an *os.File, which the in-memory filesystem has
// no way to produce, so the adapter fails closed with
// ErrCreateTempNotSupported. That sentinel is the contract callers branch on --
// returning a nil file with a nil error instead would hand every caller a
// dereference.
func TestCreateTemp_RefusesOnTheMemoryFilesystem(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	file, err := env.SigningFS.CreateTemp("/tmp", "gommitlint-*")

	check.ErrorIs(t, err, testhelpers.ErrCreateTempNotSupported,
		"callers branch on this sentinel to fall back to a real filesystem")
	check.Nil(t, file, "a refusal must not also return a file")
}

func TestCreateDir_MakesDirectoryVisible(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	dir := "/isolated/my-test"
	env.FS.CreateDir(dir)

	check.True(t, env.FS.DirExists(dir))
}

func TestCopyFile_DuplicatesContent(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	srcPath := "/test/copy/source.txt"
	dstPath := "/test/copy/dest.txt"

	// Create source file in memory filesystem
	env.FS.WriteFile(srcPath, []byte("source content"))

	// Copy file
	env.FS.CopyFile(srcPath, dstPath)

	// Verify destination has same content in memory filesystem
	dstContent := env.FS.ReadFile(dstPath)
	check.Equal(t, "source content", dstContent)
}

// WriteConfig takes a rules map (rule name to an enabled bool or a settings
// map) and nests it under gommitlint.rules; decode the file to pin that shape.
func TestWriteConfig_NestsRuleSettingsUnderGommitlintRules(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	configPath := "/test/config.yaml"

	env.FS.WriteConfig(configPath, map[string]any{
		"subject": true,
		"body":    map[string]any{"min_length": 10},
	})

	var got map[string]any

	check.NoError(t, yaml.Unmarshal([]byte(env.FS.ReadFile(configPath)), &got))
	check.Equal(t, map[string]any{
		"gommitlint": map[string]any{
			"rules": map[string]any{
				"subject": map[string]any{"enabled": true},
				"body":    map[string]any{"min_length": 10},
			},
		},
	}, got)
}

func TestWriteExecutable_WritesContent(t *testing.T) {
	env := testhelpers.NewTestEnv(t)
	scriptPath := "/test/script.sh"

	scriptContent := `#!/bin/sh
echo "Hello, World!"
`

	env.FS.WriteExecutable(scriptPath, []byte(scriptContent))

	// Verify file exists and has content in memory filesystem
	content := env.FS.ReadFile(scriptPath)
	check.Equal(t, scriptContent, content)
}

func TestNewTestEnv_SetsRequiredEnvironmentVariables(t *testing.T) {
	// Note: This test is now handled by NewTestEnv which sets up the environment
	// NewTestEnv sets NO_COLOR=1, GOMMITLINT_TEST=1, and other env vars
	env := testhelpers.NewTestEnv(t)
	check.NotNil(t, env)

	// Verify environment was set
	check.Equal(t, "1", os.Getenv("NO_COLOR"))
	check.Equal(t, "1", os.Getenv("GOMMITLINT_TEST"))
}

// TestNewTestEnv_RedirectsEveryPerUserLocation guards the isolation the 42
// callers of NewTestEnv rely on. HOME alone is not enough: configuration
// discovery prefers XDG_CONFIG_HOME, and git and gpg read GIT_CONFIG_GLOBAL and
// GNUPGHOME, so each is asserted to have moved away from the host value set
// before the helper runs, and to sit under the temporary HOME.
func TestNewTestEnv_RedirectsEveryPerUserLocation(t *testing.T) {
	const hostSentinel = "/nonexistent/host-sentinel"

	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GNUPGHOME"} {
		t.Setenv(name, hostSentinel)
	}

	testhelpers.NewTestEnv(t)

	home := os.Getenv("HOME")
	check.NotEqual(t, hostSentinel, home, "HOME must not stay on the host value")

	for _, name := range []string{"XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GNUPGHOME"} {
		value := os.Getenv(name)
		check.NotEqual(t, hostSentinel, value, name+" must not stay on the host value")
		check.True(t, strings.HasPrefix(value, home+string(os.PathSeparator)), name+" must live under the temporary HOME, got "+value)
	}
}

func TestNewTestEnv_SetsRepoStructure(t *testing.T) {
	env := testhelpers.NewTestEnv(t)

	check.True(t, env.FS.DirExists(env.RepoPath), "repo path should exist")
	check.True(t, env.FS.DirExists(env.GitDir()), "git dir should exist")
	check.True(t, env.FS.DirExists(env.HooksDir()), "hooks dir should exist")
}
