// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGettextInstallerOnlyRemovesItsOwnTemporaryDirectory(t *testing.T) {
	t.Parallel()

	for _, failTemp := range []bool{false, true} {
		t.Run(map[bool]string{false: "download failure", true: "temporary directory failure"}[failTemp], func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			bin := filepath.Join(root, "bin")

			parent := filepath.Join(root, "caller work")
			for _, dir := range []string{bin, parent} {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			keep := filepath.Join(parent, "keep.txt")
			if err := os.WriteFile(keep, []byte("caller data"), 0o600); err != nil {
				t.Fatal(err)
			}

			for _, tool := range []string{"make", "gcc", "wget", "curl"} {
				if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nexit 23\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			want := 23
			if failTemp {
				want = 69

				if err := os.WriteFile(filepath.Join(bin, "mktemp"), []byte("#!/bin/sh\nexit 69\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}

			cmd := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot(t), "scripts/dev/install-gettext.sh"))
			cmd.Env = []string{"HOME=" + root, "WORK_DIR=" + parent, "PATH=" + bin + ":" + os.Getenv("PATH")}
			output, err := cmd.CombinedOutput()

			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != want {
				t.Fatalf("want exit %d, got %v: %s", want, err, output)
			}

			data, err := os.ReadFile(keep)
			if err != nil || string(data) != "caller data" {
				t.Fatalf("caller-owned work directory changed: %q, %v", data, err)
			}

			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary state left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestOutputWrapperPreservesArgumentsAndExitStatus(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	probe := filepath.Join(dir, "probe with spaces")
	log := filepath.Join(dir, "argv")

	if err := os.WriteFile(probe, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" >\"$PROBE_LOG\"\nexit 7\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	argument := `literal ' " $(exit 99); *`
	justfile := string(readRepositoryFile(t, "justfile"))

	recipe := regexp.MustCompile(`(?m)^_run_with_output[^\n]*:\n((?:[ \t]+[^\n]*\n)+)`).FindStringSubmatch(justfile)
	if recipe == nil {
		t.Fatal("missing output-wrapper recipe")
	}

	body := strings.NewReplacer("{{ DIM }}", "", "{{ NC }}", "", "{{ RED }}", "", "{{ GREEN }}", "").Replace(recipe[1])
	cmd := exec.CommandContext(t.Context(), "bash", "-c", body,
		"output-wrapper", "fixture command", probe, argument, "second argument")

	cmd.Env = append(os.Environ(), "PROBE_LOG="+log)
	output, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("want exit 7, got %v: %s", err, output)
	}

	data, err := os.ReadFile(log)
	if err != nil || string(data) != strings.Join([]string{argument, "second argument", ""}, "\n") {
		t.Fatalf("command arguments changed: %q, %v", data, err)
	}
}

func TestDistributionRecipeChoicesMatchImageList(t *testing.T) {
	t.Parallel()

	justfile := string(readRepositoryFile(t, "justfile"))

	match := regexp.MustCompile(`(?m)^test_distributions := '([^']+)'$`).FindStringSubmatch(justfile)
	if match == nil {
		t.Fatal("missing distribution choices")
	}

	var names []string

	for line := range strings.SplitSeq(string(readRepositoryFile(t, "test/distribution/images")), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, _, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("invalid distribution entry: %s", line)
		}

		names = append(names, regexp.QuoteMeta(name))
	}

	if want := strings.Join(names, "|"); match[1] != want {
		t.Fatalf("recipe choices differ from image list: got %q, want %q", match[1], want)
	}
}
