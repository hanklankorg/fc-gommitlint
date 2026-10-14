// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSecureConfigPathValidation(t *testing.T) {
	t.Parallel()
	// Skip permission tests when running as root or in environments where
	// permission validation might not work reliably (e.g., some CI environments)
	if os.Getuid() == 0 {
		t.Skip("Skipping permission tests when running as root")
	}

	tmpDir := t.TempDir()

	// Create a valid config file
	validConfigFile := "valid-settings.yaml"
	validConfigFilePath := filepath.Join(tmpDir, validConfigFile)
	configContent := testConfigContent
	err := os.WriteFile(validConfigFilePath, []byte(configContent), 0o600)
	check.NoError(t, err)

	// Create an insecure config file
	insecureConfigFile := "insecure-settings.yaml"
	insecureConfigFilePath := filepath.Join(tmpDir, insecureConfigFile)
	err = os.WriteFile(insecureConfigFilePath, []byte(configContent), 0o666)
	check.NoError(t, err)

	// Explicitly set the file mode to override umask effects
	err = os.Chmod(insecureConfigFilePath, 0o666) // #nosec G302 - intentionally testing insecure permissions
	check.NoError(t, err)

	tests := []struct {
		name        string
		configPath  string
		expectError bool
		expectedErr error
		description string
	}{
		{
			name:        "valid secure config",
			configPath:  validConfigFile,
			expectError: false,
			description: "should accept valid secure config files",
		},
		{
			name:        "insecure permissions",
			configPath:  insecureConfigFile,
			expectError: true,
			expectedErr: configuration.ErrConfigPathWorldWritable,
			description: "should reject files with insecure permissions",
		},
		{
			name:        "non-existent file",
			configPath:  "nonexistent.yaml",
			expectError: true,
			expectedErr: ErrConfigFileNotFound,
			description: "should reject non-existent files",
		},
		{
			name:        testNamePathTraversal,
			configPath:  testPathTraversal,
			expectError: true,
			expectedErr: domain.ErrPathTraversal,
			description: testRejectPathTraversalDescription,
		},
		{
			name:        testNameEmptyPath,
			configPath:  "",
			expectError: true,
			description: "should reject empty paths",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := secureConfigPathValidationWithWorkingDir(testCase.configPath, tmpDir)

			if testCase.expectError {
				check.Error(t, err, testCase.description)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr, testCase.description)
				}
			} else {
				check.NoError(t, err, testCase.description)
			}
		})
	}
}

func TestConfigPathValidationError(t *testing.T) {
	t.Parallel()

	err := configPathValidationError{
		Path:   testPathTraversal,
		Reason: "path traversal detected",
	}

	expected := "invalid config path ../../../etc/passwd: path traversal detected"
	check.Equal(t, expected, err.Error())
}
