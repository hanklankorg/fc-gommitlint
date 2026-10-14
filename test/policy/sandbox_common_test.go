// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSandboxRefusesEveryCIMarkerForProductModes runs the launcher's
// sandbox_refuse_ci_product_mode in bash with one CI marker set at a time.
// Every product mode must be refused with the marker named, the fuzz mode
// (which never touches a shipped product) must pass, and a clean
// environment must pass. The marker list here is the contract; the script
// must carry at least these.
func TestSandboxRefusesEveryCIMarkerForProductModes(t *testing.T) {
	t.Parallel()

	markers := []string{
		"CI", "CONTINUOUS_INTEGRATION", "GITHUB_ACTIONS", "GITEA_ACTIONS", "FORGEJO_ACTIONS", "GITLAB_CI",
		"BUILDKITE", "CIRCLECI", "JENKINS_URL", "TEAMCITY_VERSION", "TF_BUILD", "DRONE", "WOODPECKER",
		"CODEBUILD_BUILD_ID", "TRAVIS",
	}
	productModes := []string{"source", "integration", "blackbox", "distribution"}
	script := filepath.Join(repoRoot(t), ".forgejo", "scripts", "sandbox-common.sh")

	refusal := func(t *testing.T, mode string, env ...string) (int, string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "bash", "-c", `source "$1" && sandbox_refuse_ci_product_mode "$2"`, "sandbox", script, mode)

		cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)

		var stderr bytes.Buffer

		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			return 0, stderr.String()
		}

		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("running %s: %v", script, err)
		}

		return exitError.ExitCode(), stderr.String()
	}

	for _, marker := range markers {
		for _, mode := range productModes {
			t.Run(marker+"/"+mode, func(t *testing.T) {
				t.Parallel()

				code, stderr := refusal(t, mode, marker+"=1")
				if code != 1 {
					t.Errorf("mode %s with %s set: want status 1, got %d (stderr %q)", mode, marker, code, stderr)
				}

				want := "refusing product mode " + mode + " while CI marker " + marker + " is set"
				if !strings.Contains(stderr, want) {
					t.Errorf("mode %s with %s set: stderr %q does not name the refusal %q", mode, marker, stderr, want)
				}
			})
		}

		t.Run(marker+"/fuzz", func(t *testing.T) {
			t.Parallel()

			if code, stderr := refusal(t, "fuzz", marker+"=1"); code != 0 {
				t.Errorf("fuzz mode with %s set must pass, got status %d (stderr %q)", marker, code, stderr)
			}
		})
	}

	t.Run("no marker/source", func(t *testing.T) {
		t.Parallel()

		if code, stderr := refusal(t, "source"); code != 0 {
			t.Errorf("source mode without a marker must pass, got status %d (stderr %q)", code, stderr)
		}
	})
}

// sandboxFunction runs one function from the launcher's shared library in
// bash and reports its status and stderr.
func sandboxFunction(t *testing.T, call string, args ...string) (int, string, string) {
	t.Helper()

	script := filepath.Join(repoRoot(t), ".forgejo", "scripts", "sandbox-common.sh")
	argv := append([]string{"sandbox", script}, args...)

	cmd := exec.CommandContext(t.Context(), "bash", append([]string{"-c", `source "$1" && ` + call}, argv...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("running %s: %v", script, err)
	}

	return exitError.ExitCode(), stdout.String(), stderr.String()
}

func TestSandboxOfflineModuleProxyRequiresLocalCache(t *testing.T) {
	t.Parallel()

	cache := t.TempDir()
	for _, proxy := range []string{"file://" + cache, "https://proxy.example.invalid", "file://relative", "file://" + filepath.Join(cache, "missing")} {
		t.Run(proxy, func(t *testing.T) {
			t.Parallel()

			code, output, stderr := sandboxFunction(t,
				`GOMMITLINT_MODULE_PROXY="$2" SANDBOX_CLEAN_ENV=1 sandbox_reexec_clean && printf '%s\n' "${SANDBOX_MODULE_ENV[@]}"`, proxy)
			if proxy == "file://"+cache {
				if code != 0 || output != "GOPROXY="+proxy+"\nGOSUMDB=off\n" {
					t.Fatalf("local cache: code=%d output=%q stderr=%q", code, output, stderr)
				}
			} else if code != 2 || !strings.Contains(stderr, "prepared local cache") {
				t.Fatalf("invalid proxy accepted: code=%d output=%q stderr=%q", code, output, stderr)
			}
		})
	}
}

// TestSandboxRefusesEveryRecordingButACurrentOne drives the launcher's
// sandbox_require_recorded_image. The external tiers load the archive that
// `just build-sandbox-image` recorded and never rebuild it themselves, which
// keeps registries out of their execution path. That property survives exactly
// as long as this guard refuses everything except a
// present, intact and current recording: rebuilding on the operator's behalf
// would put a registry back in the tier's path, and accepting a stale archive
// would test an environment the checkout no longer describes.
//
// The archive here is a stand-in file. The guard checksums it and reads the
// key beside it; loading it is the sandbox launcher's job, under Podman.
func TestSandboxRefusesEveryRecordingButACurrentOne(t *testing.T) {
	t.Parallel()

	containerfile := filepath.Join(repoRoot(t), "Containerfile.sandbox")

	code, key, stderr := sandboxFunction(t, `sandbox_image_key "$2"`, containerfile)
	if code != 0 {
		t.Fatalf("sandbox_image_key on the repository's own Containerfile failed with %d: %s", code, stderr)
	}

	for _, want := range []string{"sandbox-image-key 1", "@sha256:", "packages ", "containerfile sha256:"} {
		if !strings.Contains(key, want) {
			t.Errorf("the recorded key %q does not carry %q", key, want)
		}
	}

	record := func(t *testing.T, archive, checksum, recordedKey string) string {
		t.Helper()

		dir := t.TempDir()

		for name, content := range map[string]string{
			"sandbox-image.oci.tar": archive,
			"sandbox-image.sha256":  checksum,
			"sandbox-image.key":     recordedKey,
		} {
			if content == "" {
				continue
			}

			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o444); err != nil {
				t.Fatal(err)
			}
		}

		return dir
	}

	const archive = "stand-in for the exported OCI archive\n"

	sum := sha256.Sum256([]byte(archive))
	checksum := hex.EncodeToString(sum[:]) + "  sandbox-image.oci.tar\n"

	for _, testCase := range []struct {
		name     string
		archive  string
		checksum string
		key      string
		refusal  string
	}{
		{name: "current recording", archive: archive, checksum: checksum, key: key},
		{name: "nothing recorded", refusal: "no recorded sandbox image"},
		{name: "archive missing", checksum: checksum, key: key, refusal: "sandbox-image.oci.tar is missing"},
		{name: "checksum missing", archive: archive, key: key, refusal: "sandbox-image.sha256 is missing"},
		{name: "key missing", archive: archive, checksum: checksum, refusal: "sandbox-image.key is missing"},
		{
			name: "archive does not match its checksum", archive: "a different archive\n",
			checksum: checksum, key: key, refusal: "does not match its checksum",
		},
		{
			name: "key records another Containerfile", archive: archive, checksum: checksum,
			key:     strings.Replace(key, "containerfile sha256:", "containerfile sha256:0", 1),
			refusal: "is stale",
		},
		{
			name: "key records another base image", archive: archive, checksum: checksum,
			key:     strings.Replace(key, "\nbase ", "\nbase other/", 1),
			refusal: "is stale",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := record(t, testCase.archive, testCase.checksum, testCase.key)
			code, _, stderr := sandboxFunction(t, `sandbox_require_recorded_image "$2" "$3"`, dir, containerfile)

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

			if !strings.Contains(stderr, "just test-container-prepare") {
				t.Errorf("refusal %q does not name the recipe that records the image", stderr)
			}
		})
	}
}

func TestSandboxFuzzTimeLeavesHarnessTimeoutMargin(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		duration string
		seconds  string
	}{
		{"1ms", "1\n"}, {"1001ms", "2\n"}, {"14s", "14\n"},
		{"45s", "45\n"}, {"60s", "60\n"}, {"60000ms", "60\n"}, {"1m", "60\n"},
		{"60001ms", ""}, {"61s", ""}, {"2m", ""}, {"0s", ""},
		{"999999999999999999999999s", ""}, {"1h", ""}, {"1s; exit 0", ""},
	} {
		t.Run(testCase.duration, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := sandboxFunction(t, `sandbox_fuzz_seconds "$2"`, testCase.duration)
			if testCase.seconds == "" {
				if code != 2 || !strings.Contains(stderr, "at most 60s") {
					t.Fatalf("want bounded-duration refusal; status=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
			} else if code != 0 || stdout != testCase.seconds {
				t.Fatalf("want %q seconds; status=%d stdout=%q stderr=%q", testCase.seconds, code, stdout, stderr)
			}
		})
	}
}
