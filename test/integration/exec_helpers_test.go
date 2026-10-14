//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSetIsolatedEnvironment_ExcludesHostConfiguration(t *testing.T) {
	hostConfig := filepath.Join(t.TempDir(), "host-config")
	t.Setenv("XDG_CONFIG_HOME", hostConfig)
	t.Setenv("GOMMITLINT_HOST_SENTINEL", "must-not-leak")

	cmd := exec.CommandContext(t.Context(), "git", "--version")
	SetIsolatedEnvironment(t, cmd)
	env := environmentMap(cmd.Env)

	check.NotEqual(t, hostConfig, env["XDG_CONFIG_HOME"])
	check.Equal(t, filepath.Join(env["HOME"], ".config"), env["XDG_CONFIG_HOME"])
	check.Equal(t, "1", env["GIT_CONFIG_NOSYSTEM"])
	check.Equal(t, "0", env["GIT_TERMINAL_PROMPT"])
	_, leaked := env["GOMMITLINT_HOST_SENTINEL"]
	check.False(t, leaked)
}

func environmentMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}

	return values
}
