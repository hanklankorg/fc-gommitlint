//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testkit

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func WriteRepoConfig(t *testing.T, repoPath, name, content string) string {
	t.Helper()

	configFile := filepath.Join(repoPath, name)
	check.NoError(t, os.WriteFile(configFile, []byte(content), 0o600))

	return configFile
}

func SignoffDisabledYAML() string {
	return "gommitlint:\n  rules:\n    disabled:\n      - signoff\n"
}

// InMemoryConfigPath writes the sign-off-disabled config an in-memory scenario
// needs and returns its path.
//
// It lives beside SignoffDisabledYAML because multiple integration scenarios
// use it.
func InMemoryConfigPath(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".gommitlint.yaml")
	if err := os.WriteFile(path, []byte(SignoffDisabledYAML()), 0o600); err != nil {
		t.Fatalf("write in-memory test config: %v", err)
	}

	return path
}
