// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVMRecipeHelpPrecedesControllerExecution(t *testing.T) {
	t.Parallel()

	just, err := exec.LookPath("just")
	if err != nil {
		t.Skip("Just is required to exercise recipe argument handling")
	}

	dir := t.TempDir()

	justfile := filepath.Join(dir, "justfile")
	if err := os.WriteFile(justfile, readRepositoryFile(t, "justfile"), 0o600); err != nil {
		t.Fatal(err)
	}

	scripts := filepath.Join(dir, "scripts", "testing")
	if err := os.MkdirAll(scripts, 0o700); err != nil {
		t.Fatal(err)
	}
	// The only possible backend is this inert recorder, including valid cases.
	stub := "#!/bin/sh\nprintf '%s\\000' \"$@\" >\"$RECIPE_PROBE\"\nexit 19\n"
	if err := os.WriteFile(filepath.Join(scripts, "vm.sh"), []byte(stub), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args []string
		code int
		text string
		argv string
	}{
		{"missing prepare target", []string{"test-vm-prepare"}, 2, "Missing VM target", ""},
		{"missing cleanup target", []string{"test-vm-clean"}, 2, "Missing VM target", ""},
		{"missing unit target", []string{"test-unit-vm"}, 2, "Missing VM target", ""},
		{"missing race target", []string{"test-race-vm"}, 2, "Missing VM target", ""},
		{"missing integration target", []string{"test-integration-vm"}, 2, "Missing VM target", ""},
		{"missing blackbox target", []string{"test-blackbox-vm"}, 2, "Missing VM target", ""},
		{"missing combined target", []string{"test-combined-vm"}, 2, "Missing VM target", ""},
		{"empty target", []string{"test-vm-prepare", ""}, 2, "Missing VM target", ""},
		{"unknown target", []string{"test-combined-vm", "unknown-os"}, 1, "pattern", ""},
		{"unknown distribution", []string{"test-matrix-container-on-macos", "unknown-distro"}, 1, "pattern", ""},
		{"explicit help", []string{"help", "test-vm-prepare"}, 0, "freebsd-amd64", ""},
		{"native selection", []string{"test-combined-vm", "openbsd-amd64"}, 19, "", "run\x00openbsd-amd64\x00combined\x00"},
		{"fixed Mac guest", []string{"test-matrix-container-on-macos", "debian-13"}, 19, "", "run\x00linux-arm64\x00container-matrix\x00debian-13\x00"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			marker := filepath.Join(t.TempDir(), "called")

			args := append([]string{"--justfile", justfile}, test.args...)
			cmd := exec.CommandContext(t.Context(), just, args...)

			cmd.Env = append(os.Environ(), "RECIPE_PROBE="+marker)
			output, err := cmd.CombinedOutput()
			code := 0

			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}

				code = exit.ExitCode()
			}

			if code != test.code || !strings.Contains(string(output), test.text) {
				t.Fatalf("want exit %d and %q, got exit %d:\n%s", test.code, test.text, code, output)
			}

			data, readErr := os.ReadFile(marker)
			if test.argv == "" {
				if !errors.Is(readErr, os.ErrNotExist) {
					t.Fatalf("help or invalid input reached the controller: %q (%v)", data, readErr)
				}
			} else if readErr != nil || string(data) != test.argv {
				t.Fatalf("controller arguments changed: %q (%v)", data, readErr)
			}
		})
	}
}
