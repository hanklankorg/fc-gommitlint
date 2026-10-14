// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package policy_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// `just test-integration -v` replaced a recipe of its own, so caller
// arguments now travel from the justfile through two launchers and into a
// container shell. The sandbox launcher hands that shell a command string, and
// a string is exactly where forwarded arguments stop being data: spliced in,
// `$(...)`, a backtick or a `;` in one would run as source text with the
// tier's privileges. They are passed as positional parameters instead, and
// these tests are the reason that has to stay true.

// integrationCommand returns the integration tier's command string exactly as
// the sandbox launcher composes it, read from the launcher itself so the test
// cannot drift into checking a copy.
func integrationCommand(t *testing.T) string {
	t.Helper()

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "run-external-tests-sandbox.sh"))

	assignment := regexp.MustCompile(`(?m)^integration\)\s+(test_command='[^']*')\s+;;$`).FindStringSubmatch(script)
	if assignment == nil {
		t.Fatal("run-external-tests-sandbox.sh has no single-quoted integration test_command assignment")
	}

	return assignment[1]
}

// TestIntegrationTierRunsCallerArgumentsAsLiteralArgv drives the launcher's
// own command string with the calling convention the launcher uses, against a
// `go` that records its argv. A hostile argument must arrive at `go test` as
// one literal argument, and the three command substitutions inside it must
// leave no trace, because nothing ever asked a shell to read them.
//
// This is the half of the sandbox tier that can be proved without a container:
// the argv the inner shell builds. Everything podman does with that argv is
// unchanged by argument forwarding.
func TestIntegrationTierRunsCallerArgumentsAsLiteralArgv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")

	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}

	argvFile := filepath.Join(dir, "argv")
	stub := "#!/bin/sh\nprintf '%s\\0' \"$@\" >\"$GOMMITLINT_TEST_ARGV\"\n"

	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	// Every character a shell acts on, and three ways to run a command. The
	// markers are how the test knows nothing ran: a shell that reads this
	// argument creates them, and one that treats it as data cannot.
	backtickMarker := filepath.Join(dir, "ran-backtick")
	dollarMarker := filepath.Join(dir, "ran-dollar")
	semicolonMarker := filepath.Join(dir, "ran-semicolon")
	hostile := `-run=Subject 'quoted' "quoted" ` +
		"`touch " + backtickMarker + "`" +
		`$(touch ` + dollarMarker + `);touch ` + semicolonMarker + `;$(id)`

	// The launcher's own convention: the command string, then `_` as $0, then
	// the caller's arguments as positional parameters for its trailing "$@".
	runner := integrationCommand(t) + "\nexec bash -euo pipefail -c \"$test_command\" _ \"$@\""
	argv := append([]string{"-euo", "pipefail", "-c", runner, "argument-forwarding"}, "-v=false", hostile)

	cmd := exec.CommandContext(t.Context(), "bash", argv...)
	cmd.Env = []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TEST_TIMEOUT=10m",
		"GOMMITLINT_TEST_ARGV=" + argvFile,
	}

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("running the integration command: %v (stderr %q)", err, stderr.String())
	}

	recorded, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}

	got := strings.Split(string(recorded), "\x00")
	got = got[:len(got)-1]

	want := []string{
		"test", "-v", "-tags=integration", "-count=1", "-buildvcs=false", "-timeout=10m",
		"./test/integration/...", "-v=false", hostile,
	}

	if !slices.Equal(got, want) {
		t.Errorf("go test argv\n got %q\nwant %q", got, want)
	}

	for _, marker := range []string{backtickMarker, dollarMarker, semicolonMarker} {
		if _, err := os.Stat(marker); err == nil {
			t.Errorf("the forwarded argument executed a command: %s exists", marker)
		}
	}
}

// TestSandboxLauncherKeepsForwardedArgumentsOutOfTheCommandString pins the
// property the previous test demonstrates. A future edit that builds the
// command string from the arguments would still pass a run, and would still
// be the injection this tier must not have, so the shape is a rule rather
// than an observation.
func TestSandboxLauncherKeepsForwardedArgumentsOutOfTheCommandString(t *testing.T) {
	t.Parallel()

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "run-external-tests-sandbox.sh"))

	for _, assignment := range regexp.MustCompile(`(?m)^\s*(?:\w+\) )?test_command=.*$`).FindAllString(script, -1) {
		if strings.Contains(assignment, "test_args") {
			t.Errorf("a test_command is built from the forwarded arguments: %q", assignment)
		}
	}

	if !strings.Contains(script, `./test/integration/... "$@"`) {
		t.Error("the integration command must end in \"$@\" so forwarded arguments reach go test")
	}

	if !strings.Contains(script, `-c "$test_command" _ "${test_args[@]}"`) {
		t.Error(`the container shell must be invoked as -c "$test_command" _ "${test_args[@]}"`)
	}
}

func TestUnitContainerSelectsUntaggedRaceTests(t *testing.T) {
	t.Parallel()

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "run-external-tests-sandbox.sh"))

	assignment := regexp.MustCompile(`(?m)^unit\)\s+(test_command='[^']*')\s+;;$`).FindStringSubmatch(script)
	if assignment == nil {
		t.Fatal("missing unit command")
	}

	dir := t.TempDir()

	stub := "#!/bin/sh\nif [ \"$1\" = list ]; then printf './cmd/example\\n./internal/example\\ngenerated/catalog\\n'; else printf '%s\\n' \"$@\" >\"$GOMMITLINT_TEST_ARGV\"; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}

	argument := `-run=Unit 'quoted' "literal" $(id); false`
	runner := assignment[1] + "\nexec bash -euo pipefail -c \"$test_command\" _ \"$@\""
	cmd := exec.CommandContext(t.Context(), "bash", "-euo", "pipefail", "-c", runner, "unit-command", "-v=false", argument)
	argvFile := filepath.Join(dir, "argv")

	cmd.Env = []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"TEST_TIMEOUT=10m", "GOMMITLINT_TEST_ARGV=" + argvFile,
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("unit command failed: %v; %s", err, output)
	}

	data, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"test", "-v", "-count=1", "-race", "-buildvcs=false", "-timeout=10m", "-v=false", argument,
		"./cmd/example", "./internal/example", "",
	}, "\n")
	if string(data) != want {
		t.Fatalf("unit selection or arguments changed:\n got %q\nwant %q", data, want)
	}
}

// TestSandboxLauncherRefusesTestArgumentsForEveryOtherTier drives the sandbox
// launcher's own `--` parsing. The refusal sits among the argument checks, all
// of which run before the launcher touches podman, git or the worktree, and
// every case here is missing the sandbox image the launcher requires, so a run
// that gets past the argument checks still stops two lines later, before it
// reaches podman.
func TestSandboxLauncherRefusesTestArgumentsForEveryOtherTier(t *testing.T) {
	t.Parallel()

	script := filepath.Join(repoRoot(t), ".forgejo", "scripts", "run-external-tests-sandbox.sh")

	run := func(t *testing.T, args ...string) (int, string) {
		t.Helper()

		cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}

		var stderr bytes.Buffer

		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			t.Fatalf("the launcher must not succeed without a sandbox image (stderr %q)", stderr.String())
		}

		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("running %s: %v", script, err)
		}

		return exitError.ExitCode(), stderr.String()
	}

	for _, suite := range []string{"source", "blackbox", "distribution", "fuzz"} {
		t.Run(suite, func(t *testing.T) {
			t.Parallel()

			code, stderr := run(t, suite, "--", "-v")
			if code != 2 || !strings.Contains(stderr, "only accepted by unit or integration, not "+suite) {
				t.Errorf("%s must refuse extra test arguments, got status %d (stderr %q)", suite, code, stderr)
			}
		})
	}

	t.Run("integration", func(t *testing.T) {
		t.Parallel()

		code, stderr := run(t, "integration", "--", "-v", "-run=A B;id")
		if strings.Contains(stderr, "extra test arguments") {
			t.Errorf("integration must accept extra test arguments, got status %d (stderr %q)", code, stderr)
		}
	})
}

// localLauncher runs the local launcher with a CI marker set, so a run that
// gets past argument parsing stops at the very next line -- the CI-marker
// refusal -- instead of snapshotting the worktree and standing a container up.
// The refusals this file is about all exit before that line, so the marker
// separates "the arguments were accepted" from "the arguments were refused"
// without depending on anything about the host.
func localLauncher(t *testing.T, args ...string) (int, string) {
	t.Helper()

	script := filepath.Join(repoRoot(t), ".forgejo", "scripts", "run-external-tests-local.sh")

	cmd := exec.CommandContext(t.Context(), "bash", append([]string{script}, args...)...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "CI=1"}

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

// TestLocalLauncherForwardsArgumentsForTheIntegrationTierOnly holds the
// relaxation to one tier. The launcher used to refuse every extra argument so
// that a stray one could not quietly change what a destructive tier runs;
// integration now forwards them to go test, and the others must still refuse
// rather than drop them.
func TestLocalLauncherForwardsArgumentsForTheIntegrationTierOnly(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		args   []string
		code   int
		stderr string
	}{
		{
			name: "integration takes go test arguments", code: 1, stderr: "refusing product mode integration",
			args: []string{"integration", "-v", "-run=A B'\"`id`$(id);id"},
		},
		{name: "integration alone", args: []string{"integration"}, code: 1, stderr: "refusing product mode integration"},
		{name: "blackbox alone", args: []string{"blackbox"}, code: 1, stderr: "refusing product mode blackbox"},
		{name: "blackbox refuses", args: []string{"blackbox", "-v"}, code: 2, stderr: "unexpected arguments for blackbox"},
		{name: "source refuses", args: []string{"source", "-v"}, code: 2, stderr: "unexpected arguments for source"},
		{name: "fuzz refuses", args: []string{"fuzz", "10s", "-v"}, code: 2, stderr: "unexpected arguments for fuzz"},
		{name: "the verbose suite is gone", args: []string{"integration-verbose"}, code: 2, stderr: "usage:"},
		{name: "distribution alone means all", args: []string{"distribution"}, code: 1, stderr: "refusing product mode distribution"},
		{
			name: "distribution takes listed names", code: 1, stderr: "refusing product mode distribution",
			args: []string{"distribution", "fedora-44", "arch"},
		},
		{
			name: "distribution refuses an unlisted name", code: 2, stderr: "unknown distribution",
			args: []string{"distribution", "arch", "gentoo"},
		},
		{name: "distribution refuses go test arguments", args: []string{"distribution", "-v"}, code: 2, stderr: "unknown distribution"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			code, stderr := localLauncher(t, testCase.args...)
			if code != testCase.code {
				t.Errorf("want status %d, got %d (stderr %q)", testCase.code, code, stderr)
			}

			if !strings.Contains(stderr, testCase.stderr) {
				t.Errorf("stderr %q does not say %q", stderr, testCase.stderr)
			}
		})
	}
}
