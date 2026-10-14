// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The distribution matrix reruns the tagged tiers in one image per
// distribution. It is the launcher's `distribution` mode, not a runner of its
// own, so the guards the other tiers have hold for it too; these tests pin
// what is specific to it: the pinned image list, the recording key that ties
// each archive to that list, and the options that select the mode.

// distributionList is the matrix's image list, relative to the repository.
const distributionList = "test/distribution/images"

// TestDistributionImages_List_PinsEveryImageAndRenovatesIt reads the list the
// way sandbox-common.sh does and the way renovate does, and requires the two
// to agree on every entry. An entry the launcher accepts but renovate's regex
// misses would sit at its digest forever; one renovate updates in a shape the
// launcher refuses would break every run at the next bump.
func TestDistributionImages_List_PinsEveryImageAndRenovatesIt(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := sandboxFunction(t, `sandbox_distribution_names "$2"`, filepath.Join(repoRoot(t), distributionList))
	if code != 0 {
		t.Fatalf("sandbox_distribution_names refused the repository's own list with %d: %s", code, stderr)
	}

	names := strings.Fields(stdout)
	if len(names) == 0 {
		t.Fatal("the distribution list names no distribution")
	}

	var config struct {
		CustomManagers []struct {
			ManagerFilePatterns []string `json:"managerFilePatterns"`
			MatchStrings        []string `json:"matchStrings"`
		} `json:"customManagers"`
	}

	if err := json.Unmarshal(readRepositoryFile(t, "renovate.json"), &config); err != nil {
		t.Fatal(err)
	}

	var matchStrings []string

	for _, manager := range config.CustomManagers {
		for _, pattern := range manager.ManagerFilePatterns {
			if strings.Contains(pattern, "test/distribution/images") {
				matchStrings = append(matchStrings, manager.MatchStrings...)
			}
		}
	}

	if len(matchStrings) != 1 {
		t.Fatalf("renovate.json must carry exactly one matchString for %s, found %d", distributionList, len(matchStrings))
	}

	list := string(readRepositoryFile(t, "test", "distribution", "images"))
	matches := regexp.MustCompile(matchStrings[0]).FindAllStringSubmatch(list, -1)

	if len(matches) != len(names) {
		t.Fatalf("renovate matches %d entries of %s, the launcher reads %d", len(matches), distributionList, len(names))
	}

	for index, match := range matches {
		depName, currentValue := match[1], match[2]
		want := names[index] + "=" + depName + ":" + currentValue + "@"

		if !strings.Contains(list, "\n"+want) {
			t.Errorf("renovate entry %d (%s:%s) does not update the line for %s", index, depName, currentValue, names[index])
		}
	}
}

// TestDistributionImages_Names_RefusesEveryUnpinnedOrMalformedList feeds the
// list reader the shapes it exists to stop. The list is data read by every
// launcher step, so a bad line must fail all of them rather than drop an
// entry from the matrix without a word.
func TestDistributionImages_Names_RefusesEveryUnpinnedOrMalformedList(t *testing.T) {
	t.Parallel()

	const digest = "@sha256:9cc080028c43b27d2074d63a5f9caf7166d731494965616c1a6d2827a004585c"

	for _, testCase := range []struct {
		name    string
		list    string
		refusal string
	}{
		{name: "tag without digest", list: "debian-13=docker.io/library/debian:13.7\n", refusal: "digest-pinned"},
		{name: "digest without tag", list: "debian-13=docker.io/library/debian" + digest + "\n", refusal: "digest-pinned"},
		{name: "latest tag", list: "debian-13=docker.io/library/debian:latest" + digest + "\n", refusal: "digest-pinned"},
		{name: "unsafe name", list: "../debian=docker.io/library/debian:13.7" + digest + "\n", refusal: "digest-pinned"},
		{name: "quote in repository", list: "debian-13=docker.io/library/de\"bian:13.7" + digest + "\n", refusal: "digest-pinned"},
		{
			name:    "listed twice",
			list:    "debian-13=docker.io/library/debian:13.7" + digest + "\ndebian-13=docker.io/library/debian:13.6" + digest + "\n",
			refusal: "listed twice",
		},
		{name: "empty", list: "# only a comment\n\n", refusal: "names no distribution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			list := filepath.Join(t.TempDir(), "images")
			if err := os.WriteFile(list, []byte(testCase.list), 0o600); err != nil {
				t.Fatal(err)
			}

			code, _, stderr := sandboxFunction(t, `sandbox_distribution_names "$2"`, list)
			if code != 1 {
				t.Fatalf("want refusal status 1, got %d (stderr %q)", code, stderr)
			}

			if !strings.Contains(stderr, testCase.refusal) {
				t.Errorf("refusal %q does not say %q", stderr, testCase.refusal)
			}
		})
	}
}

// TestSandboxRecording_RequireRecordedImage_BindsEachDistributionToItsKey is
// the distribution half of TestSandboxRefusesEveryRecordingButACurrentOne.
// A distribution's recording is accepted only under the key for that very
// distribution, so an archive recorded for one base can never be run as
// another, and a refusal names the recipe that records the right one.
func TestSandboxRecording_RequireRecordedImage_BindsEachDistributionToItsKey(t *testing.T) {
	t.Parallel()

	containerfile := filepath.Join(repoRoot(t), "test", "distribution", "Containerfile")

	key := func(t *testing.T, name string) string {
		t.Helper()

		code, stdout, stderr := sandboxFunction(t, `sandbox_image_key "$2" "$3"`, containerfile, name)
		if code != 0 {
			t.Fatalf("sandbox_image_key for %s failed with %d: %s", name, code, stderr)
		}

		return stdout
	}

	arch := key(t, "arch")
	for _, want := range []string{"image localhost/gommitlint-distribution:arch\n", "@sha256:", "install-deps sha256:", "containerfile sha256:"} {
		if !strings.Contains(arch, want) {
			t.Errorf("the distribution key %q does not carry %q", arch, want)
		}
	}

	const archive = "stand-in for the exported OCI archive\n"

	sum := sha256.Sum256([]byte(archive))

	record := func(t *testing.T, recordedKey string) string {
		t.Helper()

		dir := t.TempDir()

		for name, content := range map[string]string{
			"sandbox-image.oci.tar": archive,
			"sandbox-image.sha256":  hex.EncodeToString(sum[:]) + "  sandbox-image.oci.tar\n",
			"sandbox-image.key":     recordedKey,
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o444); err != nil {
				t.Fatal(err)
			}
		}

		return dir
	}

	for _, testCase := range []struct {
		name         string
		recorded     string
		distribution string
		refusal      string
	}{
		{name: "current recording", recorded: arch, distribution: "arch"},
		{name: "another distribution's recording", recorded: key(t, "debian-13"), distribution: "arch", refusal: "is stale"},
		{name: "the Go sandbox's recording", recorded: "sandbox-image-key 1\n", distribution: "arch", refusal: "is stale"},
		{name: "unlisted distribution", recorded: arch, distribution: "gentoo", refusal: "unknown distribution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := record(t, testCase.recorded)
			code, _, stderr := sandboxFunction(t, `sandbox_require_recorded_image "$2" "$3" "$4"`, dir, containerfile, testCase.distribution)

			if testCase.refusal == "" {
				if code != 0 {
					t.Fatalf("a current recording must be accepted, got status %d (stderr %q)", code, stderr)
				}

				return
			}

			if code != 1 {
				t.Fatalf("want refusal status 1, got %d (stderr %q)", code, stderr)
			}

			if !strings.Contains(stderr, testCase.refusal) {
				t.Errorf("refusal %q does not say %q", stderr, testCase.refusal)
			}

			if testCase.refusal == "is stale" && !strings.Contains(stderr, "just test-container-prepare-distributions "+testCase.distribution) {
				t.Errorf("refusal %q does not name the recipe that records the image", stderr)
			}
		})
	}
}

// TestSandboxLauncher_Distribution_RefusesOptionsOutsideTheirTier drives the
// sandbox launcher's option checks for the distribution mode. They run before
// the launcher stages anything or touches Podman, and every case here fails
// one of them, so no case reaches a container.
func TestSandboxLauncher_Distribution_RefusesOptionsOutsideTheirTier(t *testing.T) {
	t.Parallel()

	script := filepath.Join(repoRoot(t), ".forgejo", "scripts", "run-external-tests-sandbox.sh")
	image := []string{"--sandbox-image", "absent.oci.tar", "--sandbox-image-checksum", "absent.sha256"}
	harness := []string{"--harness-root", "absent", "--harness-manifest", "absent", "--harness-manifest-checksum", "absent"}

	for _, testCase := range []struct {
		name    string
		args    []string
		refusal string
	}{
		{
			name: "distribution without a name", args: append(append([]string{"distribution"}, image...), harness...),
			refusal: "requires --distribution and the harness options",
		},
		{
			name: "distribution without harnesses", args: append([]string{"distribution", "--distribution", "arch"}, image...),
			refusal: "requires --distribution and the harness options",
		},
		{
			name:    "unlisted distribution",
			args:    append(append([]string{"distribution", "--distribution", "gentoo"}, image...), harness...),
			refusal: "unknown distribution",
		},
		{
			name: "blackbox with a distribution", args: append([]string{"blackbox", "--distribution", "arch"}, image...),
			refusal: "reserved for the distribution tier",
		},
		{
			name: "fuzz with harnesses", args: append(append([]string{"fuzz"}, image...), harness...),
			refusal: "reserved for the distribution tier",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, testCase.args...)...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

			var stderr bytes.Buffer

			cmd.Stderr = &stderr

			err := cmd.Run()

			var exitError *exec.ExitError
			if !errors.As(err, &exitError) {
				t.Fatalf("the launcher must refuse, got %v (stderr %q)", err, stderr.String())
			}

			if exitError.ExitCode() != 2 || !strings.Contains(stderr.String(), testCase.refusal) {
				t.Errorf("want status 2 saying %q, got %d (stderr %q)", testCase.refusal, exitError.ExitCode(), stderr.String())
			}
		})
	}
}
