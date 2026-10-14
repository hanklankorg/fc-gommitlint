//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"strings"
	"testing"
)

// TestStreams_NoANSIEscapesWhenPiped is the binary-level half of the no-ANSI-when-piped
// conformance contract: with output captured (never a TTY), neither `--help`
// nor a real validate run may emit a single ANSI escape (0x1b) on either
// stream. The colour-decision function itself is unit-tested across the
// flag × NO_COLOR × isTTY matrix in internal/adapters/cli. The isolated test
// environment forces LC_ALL=C, so this holds regardless of the host locale.
//
// The harness base environment sets NO_COLOR=1 and TERM=dumb, which suppress
// colour before the TTY decision is ever reached; the cases here clear both so
// the only thing standing between the product and an escape is the pipe.
//
//nolint:tparallel // builds a shared binary; subtests run in parallel
func TestStreams_NoANSIEscapesWhenPiped(t *testing.T) {
	harness := newHarness(t)
	colorEnv := []string{"NO_COLOR=", "TERM=xterm-256color"}

	tests := []struct {
		name     string
		setupDir func(t *testing.T) string
		args     []string
	}{
		{
			name:     "--help",
			setupDir: func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:     []string{"--help"},
		},
		{
			name:     "bare invocation concise help",
			setupDir: func(t *testing.T) string { return gitRepoPathWithCommits(t, "feat: valid") },
			args:     []string{},
		},
		{
			// A failing validate run (default rules fail on the plain message)
			// exercises the coloured result path with --color auto on a
			// non-TTY: it must degrade to plain text.
			name:     "validate run with --color auto",
			setupDir: func(t *testing.T) string { return gitRepoPathWithCommits(t, "invalid commit format") },
			args:     []string{"validate", "--color", "auto"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := testCase.setupDir(t)
			result := harness.run(dir, colorEnv, testCase.args...)

			if strings.Contains(result.stdout, "\x1b") {
				t.Fatalf("stdout must contain zero ANSI escapes when piped\n%s", result.stdout)
			}
			if strings.Contains(result.stderr, "\x1b") {
				t.Fatalf("stderr must contain zero ANSI escapes when piped\n%s", result.stderr)
			}
		})
	}
}
