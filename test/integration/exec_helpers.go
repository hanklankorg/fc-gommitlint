//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"os/exec"
	"testing"

	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
)

// SetIsolatedEnvironment configures an exec.Command with isolated environment variables.
// This prevents tests from being affected by the host system's environment.
func SetIsolatedEnvironment(t *testing.T, cmd *exec.Cmd) {
	t.Helper()

	testkit.SetIsolatedEnvironment(t, cmd)
}
