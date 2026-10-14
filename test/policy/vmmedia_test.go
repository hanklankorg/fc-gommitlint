// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenBSDInstallerPreservesBootPaths(t *testing.T) {
	t.Parallel()

	xorriso, err := exec.LookPath("xorriso")
	if err != nil {
		t.Skip("xorriso is an optional VM controller prerequisite")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source")

	scratch := filepath.Join(dir, "scratch")
	for _, path := range []string{filepath.Join(source, "etc"), filepath.Join(source, "7.9", "amd64"), scratch} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	const kernel = "set image /7.9/amd64/bsd.rd\n"
	for name, contents := range map[string][]byte{
		"etc/boot.conf":    []byte(kernel),
		"7.9/amd64/cdbr":   make([]byte, 2048),
		"7.9/amd64/cdboot": []byte("inert loader fixture"),
		"7.9/amd64/bsd.rd": []byte("inert kernel fixture"),
	} {
		if err := os.WriteFile(filepath.Join(source, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runISO := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), xorriso, args...)

		cmd.Env = append(os.Environ(), "HOME="+dir)

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xorriso: %v\n%s", err, output)
		}

		return string(output)
	}
	base := filepath.Join(dir, "base.iso")
	// Model the upstream CD's dotted ISO directory and inert El Torito entry.
	runISO("-outdev", base, "-compliance", "allow_dir_id_ext", "-map", source, "/",
		"-boot_image", "any", "bin_path=/7.9/amd64/cdbr",
		"-boot_image", "any", "cat_path=/7.9/amd64/boot.catalog")

	cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c",
		`source "$1"; bootstrap_openbsd_iso`, "remaster",
		filepath.Join(repoRoot(t), "scripts", "testing", "vm-bootstrap.sh"))

	cmd.Env = append(os.Environ(), "HOME="+dir, "base_image="+base, "scratch="+scratch, "logs="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		log, _ := os.ReadFile(filepath.Join(dir, "seed.log"))
		t.Fatalf("remaster: %v\n%s\n%s", err, output, log)
	}

	image := filepath.Join(scratch, "install.iso")
	// cdbr cannot see Rock Ridge names. Its actual lookup must still succeed.
	listing := runISO("-read_fs", "ecma119", "-indev", image, "-ls", "/7.9/AMD64")
	if !strings.Contains(listing, "CDBOOT") {
		t.Fatalf("loader missing from the ISO boot path: %s", listing)
	}

	config := filepath.Join(dir, "boot.conf")
	runISO("-osirrox", "on", "-indev", image, "-extract", "/etc/boot.conf", config)

	data, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), kernel) || !strings.Contains(string(data), "set tty com0\n") {
		t.Fatalf("installer kernel or serial console setting lost: %s", data)
	}
}

func TestOpenBSDInstallerFailureIsReported(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "console.log"), []byte("\x00failed; check /tmp/ai/ai.log\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", `
source "$1"
fail() { printf '%s\n' "$*" >&2; exit 2; }
bootstrap=openbsd-autoinstall
logs=$2
pid=$$
bootstrap_wait_console CONGRATULATIONS 2
`, "installer-failure", filepath.Join(repoRoot(t), "scripts", "testing", "vm-bootstrap.sh"), dir)

	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "OpenBSD unattended installation failed") {
		t.Fatalf("installer failure was not reported: %v; %s", err, output)
	}
}
