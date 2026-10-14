//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"testing"
)

// TestShippedBinary_RunsValidateAndHelp exercises the built product as a
// subprocess, so it belongs in the black-box tier.
func TestShippedBinary_RunsValidateAndHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary execution test in short mode")
	}

	harness := newHarness(t)

	// A real filesystem path: this runs the product as a subprocess, so the
	// repository has to exist somewhere it can open.
	repoDir := gitRepoPathWithCommits(t, "feat: binary execution test")

	// validate is the path a hook or a pipeline actually takes.
	result := harness.run(repoDir, nil, "validate", "--repo-path", repoDir)
	if result.code != 0 {
		t.Fatalf("binary execution should succeed\nstdout:\n%s\nstderr:\n%s", result.stdout, result.stderr)
	}

	help := harness.run(repoDir, nil, "--help")
	if help.code != 0 {
		t.Fatalf("help should work\nstdout:\n%s\nstderr:\n%s", help.stdout, help.stderr)
	}
}
