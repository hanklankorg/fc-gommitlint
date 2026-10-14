// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fake only the reported platform and the command under test. No engine or VM
// runs: a refused selection must never get as far as the command's marker.
func TestEnvironmentSelectionIsExplicit(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, system, machine, mode string
		ci                          bool
		want                        int
	}{
		{"native Linux", "Linux", "x86_64", "native", false, 7},
		{"native macOS", "Darwin", "arm64", "native", false, 7},
		{"native BSD", "FreeBSD", "amd64", "native", false, 7},
		{"native ARM Linux", "Linux", "aarch64", "native", false, 7},
		{"native CI", "Linux", "x86_64", "native", true, 7},
		{"no automatic Linux VM", "Darwin", "arm64", "container", false, 2},
		{"no BSD container substitute", "FreeBSD", "amd64", "container", false, 2},
		{"CI refuses even an empty marker", "Linux", "x86_64", "container", true, 2},
		{"no automatic mode", "Linux", "x86_64", "auto", false, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()

			uname := "#!/bin/sh\ncase \"$1\" in -s) printf '%s\\n' \"$TEST_SYSTEM\";; -m) printf '%s\\n' \"$TEST_MACHINE\";; esac\n"
			if err := os.WriteFile(filepath.Join(dir, "uname"), []byte(uname), 0o700); err != nil {
				t.Fatal(err)
			}

			marker := filepath.Join(dir, "called")
			argument := `quotes ' " $(touch SHOULD_NOT_EXIST); *`
			suite := "unit + fuzz seeds " + argument
			cmd := exec.CommandContext(t.Context(), "bash",
				filepath.Join(repoRoot(t), "scripts", "testing", "environment.sh"),
				test.mode, "--suite", suite, "--", "/bin/sh", "-c", `printf '%s' "$1" >"$TEST_MARKER"; exit 7`, "probe", argument)
			cmd.Dir = dir

			cmd.Env = []string{
				"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TEST_SYSTEM=" + test.system, "TEST_MACHINE=" + test.machine, "TEST_MARKER=" + marker,
			}
			if test.ci {
				cmd.Env = append(cmd.Env, "CI=")
			}

			var stderr bytes.Buffer

			cmd.Stderr = &stderr
			err := cmd.Run()

			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != test.want {
				t.Fatalf("want exit %d, got %v; %s", test.want, err, stderr.String())
			}

			data, readErr := os.ReadFile(marker)
			if test.want == 7 {
				if readErr != nil || string(data) != argument {
					t.Fatalf("arguments changed: %q; %v", data, readErr)
				}

				architecture := "amd64"
				if test.machine == "arm64" || test.machine == "aarch64" {
					architecture = "arm64"
				}

				banner := "== Test job: suites=" + suite + " | environment=" + test.mode + " | host=" + test.system + "/" + architecture
				if !strings.Contains(stderr.String(), banner) {
					t.Fatalf("missing runtime report: %s", stderr.String())
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("refused selection executed a command: %q; %v", data, readErr)
			}

			if _, err := os.Stat(filepath.Join(dir, "SHOULD_NOT_EXIST")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("suite label or forwarded argument was executed: %v", err)
			}
		})
	}
}

func TestVMControllerContracts(t *testing.T) {
	t.Parallel()

	// Container snapshots make source files read-only. Exercise that contract
	// here too: mutable fixtures must be writable copies, not source edits.
	dir := t.TempDir()

	scripts := filepath.Join(dir, "source", "scripts", "testing")
	if err := os.MkdirAll(scripts, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"test-vm.sh", "vm.sh", "vm-bootstrap.sh", "vm-bootstrap-guest.sh", "vm-guest.sh"} {
		if err := os.WriteFile(filepath.Join(scripts, name), readRepositoryFile(t, "scripts", "testing", name), 0o444); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(scripts, "test-vm.sh"))

	cmd.Env = append(os.Environ(), "TMPDIR="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("inert VM controller contracts: %v\n%s", err, output)
	}
}
