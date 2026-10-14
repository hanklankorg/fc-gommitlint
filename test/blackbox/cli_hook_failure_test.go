//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitMsgHook_RendersValidationFailure(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	repo := newEmptyGitRepo(t)

	installResult := harness.run(repo.Path, nil, "hook", "install", "--repo-path", repo.Path)
	if installResult.code != 0 {
		t.Fatalf("hook installation failed\nstdout:\n%s\nstderr:\n%s", installResult.stdout, installResult.stderr)
	}

	const invalidMessage = "not a conventional hook message"
	messagePath := filepath.Join(repo.Path, "COMMIT_EDITMSG")
	if err := os.WriteFile(messagePath, []byte(invalidMessage+"\n"), 0o600); err != nil {
		t.Fatalf("write commit message: %v", err)
	}

	pathEnv := filepath.Dir(harness.bin)
	if inheritedPath := os.Getenv("PATH"); inheritedPath != "" {
		pathEnv += string(os.PathListSeparator) + inheritedPath
	}

	hookPath := filepath.Join(repo.Path, ".git", "hooks", "commit-msg")
	// The hook is a different program from the product, so it runs through
	// runProgram rather than run -- same isolated environment, different binary.
	hookResult := harness.runProgram(hookPath, repo.Path, []string{"PATH=" + pathEnv}, messagePath)

	if hookResult.code != 1 {
		t.Fatalf("validation failure exit code changed\nstdout:\n%s\nstderr:\n%s", hookResult.stdout, hookResult.stderr)
	}
	if hookResult.stdout != "" {
		t.Fatalf("hook diagnostics must stay on stderr, got stdout:\n%s", hookResult.stdout)
	}
	for _, expected := range []string{"[gommitlint] commit-msg failed", invalidMessage, "Fix the issues above and try again"} {
		if !strings.Contains(hookResult.stderr, expected) {
			t.Fatalf("hook stderr missing %q:\n%s", expected, hookResult.stderr)
		}
	}
}
