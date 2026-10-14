//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package testkit

import (
	"maps"
	"os"
	"os/exec"
	"slices"
	"testing"

	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
)

func SetIsolatedEnvironment(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	cmd.Env = isolatedEnvironment(t, nil)
}

func isolatedEnvironment(t *testing.T, overrides map[string]string) []string {
	t.Helper()

	isoEnv := gitrepo.NewIsolatedEnvironment(t.TempDir())
	envMap := isoEnv.ToMap()

	// Child processes need only executable lookup and Windows process basics
	// from the host. Test-specific values must be passed as explicit overrides.
	for _, key := range []string{"PATH", "PATHEXT", "SYSTEMROOT", "WINDIR", "COMSPEC"} {
		if value, ok := os.LookupEnv(key); ok {
			envMap[key] = value
		}
	}

	maps.Copy(envMap, overrides)

	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+envMap[key])
	}

	return env
}
