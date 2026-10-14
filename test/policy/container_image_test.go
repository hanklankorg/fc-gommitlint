// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultImageTrustsTheRepoMount pins what makes the default container
// image usable on a bind-mounted repository: git refuses a repository owned by
// another user, and a SHA-256 repository can only be read through git. Without
// the trusted path and the working directory, `podman run -v "$PWD:/repo:ro"
// <image> validate` fails for every SHA-256 repository.
func TestDefaultImageTrustsTheRepoMount(t *testing.T) {
	t.Parallel()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), "Containerfile.alpine"))
	if err != nil {
		t.Fatalf("read Containerfile.alpine: %v", err)
	}

	for _, want := range []string{
		"git config --system --add safe.directory /repo",
		"WORKDIR /repo",
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("Containerfile.alpine no longer contains %q", want)
		}
	}
}
