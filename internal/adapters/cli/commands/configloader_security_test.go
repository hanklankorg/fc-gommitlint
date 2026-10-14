// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Test constants for string literals.
const (
	testConfigFileName = "settings.yaml"
	testConfigContent  = `gommitlint:
  message:
    subject:
      max_length: 50`
)

// TestLoadConfigFromCommand_RejectsDangerousConfigPaths covers path validation of the --config flag.
func TestLoadConfigFromCommand_RejectsDangerousConfigPaths(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		configPath  string
		expectedErr error
		description string
	}{
		{
			name:        testNameValidRelativePath,
			configPath:  testConfigFileName,
			expectedErr: ErrConfigFileNotFound,
			description: "should handle valid relative paths",
		},
		{
			name:        "path traversal attack",
			configPath:  testPathTraversal,
			expectedErr: domain.ErrPathTraversal,
			description: testRejectPathTraversalDescription,
		},
		{
			name:        "absolute path to file",
			configPath:  filepath.Join(t.TempDir(), testConfigFileName),
			expectedErr: ErrConfigFileNotFound,
			description: "should allow absolute paths (validation passes, file not found)",
		},
		{
			name:        testNameNullByteInjection,
			configPath:  "settings.yaml\x00.txt",
			expectedErr: domain.ErrPathDangerous,
			description: testRejectNullBytesDescription,
		},
		{
			name:        testNameVeryLongPath,
			configPath:  strings.Repeat("a", 1025), // Exceeds 1024 limit
			expectedErr: domain.ErrParameterTooLong,
			description: testRejectLongPathsDescription,
		},
		{
			name:        "path with newlines",
			configPath:  testPathWithNewline,
			expectedErr: ErrConfigFileNotFound,
			description: "should allow newlines (validation passes, file not found)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create CLI app context
			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: flagNameConfig},
					&cli.BoolFlag{Name: flagNameIgnoreConfig},
				},
			}

			// Use the config path as-is (this is what the security test is designed to test)
			err := app.Set(flagNameConfig, testCase.configPath)
			check.NoError(t, err)

			// Test config loading using explicit working directory
			_, err = LoadConfigFromCommandWithWorkingDir(app, tmpDir)

			check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
		})
	}
}

// TestLoadConfigFromCommand_ValidatesFilePermissions tests file permission validation.
func TestLoadConfigFromCommand_ValidatesFilePermissions(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	tests := []struct {
		name           string
		fileMode       os.FileMode
		expectedErr    error // nil means the load must succeed
		expectAdvisory bool  // load succeeds but warns (e.g. group-writable)
		description    string
	}{
		{
			name:        "readable config file",
			fileMode:    0o600,
			description: "should accept readable config files",
		},
		{
			name:        "world-writable config file",
			fileMode:    0o666,
			expectedErr: configuration.ErrConfigPathWorldWritable,
			description: "should reject world-writable config files",
		},
		{
			name:           "group-writable config file",
			fileMode:       0o620,
			expectAdvisory: true,
			description:    "should load group-writable config files with WarnConfigInsecurePerms",
		},
		{
			name:        "executable config file",
			fileMode:    0o700,
			expectedErr: configuration.ErrConfigPathExecutable,
			description: "should reject executable config files",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create config file with specific permissions using relative path
			configFileName := "test-config-" + strings.ReplaceAll(testCase.name, " ", "-") + ".yaml"
			configContent := testConfigContent
			configFilePath := filepath.Join(tmpDir, configFileName)

			err := os.WriteFile(configFilePath, []byte(configContent), testCase.fileMode)
			check.NoError(t, err)

			// Explicitly set the file mode to override umask effects
			err = os.Chmod(configFilePath, testCase.fileMode)
			check.NoError(t, err)

			// Create CLI app context
			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: flagNameConfig},
					&cli.BoolFlag{Name: flagNameIgnoreConfig},
				},
			}

			// Use relative path for --config (this is what the test is designed to test)
			err = app.Set(flagNameConfig, configFileName)
			check.NoError(t, err)

			// Test config loading using explicit working directory
			result, err := LoadConfigFromCommandWithWorkingDir(app, tmpDir)

			if testCase.expectedErr != nil {
				check.ErrorIs(t, err, testCase.expectedErr, testCase.description)

				return
			}

			check.NoError(t, err, testCase.description)

			if testCase.expectAdvisory {
				var found bool

				for _, w := range result.Warnings {
					if w.Code == domain.WarnConfigInsecurePerms {
						found = true

						break
					}
				}

				check.True(t, found,
					"%s: expected WarnConfigInsecurePerms among warnings, got %v",
					testCase.description, result.Warnings)
			}
		})
	}
}

// TestLoadConfigFromCommand_RejectsSymlinkedConfigs tests symlink attack prevention.
func TestLoadConfigFromCommand_RejectsSymlinkedConfigs(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("Skipping symlink tests when running as root")
	}

	tmpDir := t.TempDir()

	tests := []struct {
		name         string
		setupSymlink func(t *testing.T) string
		expectedErr  error
		description  string
	}{
		{
			name: "symlink to sensitive file",
			setupSymlink: func(t *testing.T) string {
				t.Helper()

				symlinkName := "symlink-to-passwd-settings.yaml"
				symlinkPath := filepath.Join(tmpDir, symlinkName)
				_ = os.Symlink("/etc/passwd", symlinkPath)

				return symlinkName
			},
			expectedErr: configuration.ErrConfigPathSymlink,
			description: "should reject symlinks to sensitive files",
		},
		{
			name: "symlink to valid config in temp dir",
			setupSymlink: func(t *testing.T) string {
				t.Helper()

				configPath := filepath.Join(tmpDir, "valid-config.yaml")
				err := os.WriteFile(configPath, []byte(testConfigContent), 0o600)
				check.NoError(t, err)

				symlinkName := "symlink-to-valid-config.yaml"
				symlinkPath := filepath.Join(tmpDir, symlinkName)
				err = os.Symlink(configPath, symlinkPath)
				check.NoError(t, err)

				return symlinkName
			},
			expectedErr: configuration.ErrConfigPathSymlink,
			description: "should reject symlinks even to valid config",
		},
		{
			name: "symlink to missing file",
			setupSymlink: func(t *testing.T) string {
				t.Helper()

				symlinkName := "symlink-to-missing.yaml"
				symlinkPath := filepath.Join(tmpDir, symlinkName)
				err := os.Symlink(filepath.Join(tmpDir, "missing.yaml"), symlinkPath)
				check.NoError(t, err)

				return symlinkName
			},
			expectedErr: configuration.ErrConfigPathSymlink,
			description: "should reject symlinks to missing files",
		},
		{
			name: "broken symlink",
			setupSymlink: func(t *testing.T) string {
				t.Helper()

				symlinkName := "broken-symlink-settings.yaml"
				symlinkPath := filepath.Join(tmpDir, symlinkName)
				_ = os.Symlink("nonexistent-file.yaml", symlinkPath)

				return symlinkName
			},
			expectedErr: configuration.ErrConfigPathSymlink,
			description: "should reject broken symlinks",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			configPath := testCase.setupSymlink(t)
			// No need for manual cleanup - t.TempDir() handles it

			// Create CLI app context
			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: flagNameConfig},
				},
			}

			err := app.Set(flagNameConfig, configPath)
			check.NoError(t, err)

			// Test config loading using explicit working directory
			_, err = LoadConfigFromCommandWithWorkingDir(app, tmpDir)

			check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
		})
	}
}

// TestLoadConfigFromCommand_PreventsDirectoryTraversal tests directory traversal scenarios.
func TestLoadConfigFromCommand_PreventsDirectoryTraversal(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	var err error

	// Create nested directory structure
	nestedDir := filepath.Join(tmpDir, "nested", "dir")
	err = os.MkdirAll(nestedDir, 0o750)
	check.NoError(t, err)

	// Create config file in parent directory
	parentConfig := filepath.Join(tmpDir, "parent-settings.yaml")
	configContent := testConfigContent
	err = os.WriteFile(parentConfig, []byte(configContent), 0o600)
	check.NoError(t, err)

	tests := []struct {
		name        string
		configPath  string
		expectedErr error
		description string
	}{
		{
			name:        "access parent directory",
			configPath:  "../../parent-settings.yaml",
			expectedErr: domain.ErrPathTraversal,
			description: "should prevent access to parent directories",
		},
		{
			name:        "complex traversal pattern",
			configPath:  "../../../some-dir/../" + filepath.Base(tmpDir) + "/parent-settings.yaml",
			expectedErr: domain.ErrPathTraversal,
			description: "should prevent complex traversal patterns",
		},
		{
			name:        "current directory access",
			configPath:  "./local-settings.yaml",
			expectedErr: ErrConfigFileNotFound,
			description: "should allow current directory access",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create CLI app context
			app := &cli.Command{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: flagNameConfig},
				},
			}

			err := app.Set(flagNameConfig, testCase.configPath)
			check.NoError(t, err)

			// Test config loading using explicit working directory (nested directory)
			_, err = LoadConfigFromCommandWithWorkingDir(app, nestedDir)

			check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
		})
	}
}
