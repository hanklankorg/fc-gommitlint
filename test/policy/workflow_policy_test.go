// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package policy pins structural invariants of the hosted Forgejo
// workflows on the parsed YAML, so a workflow can be reworded, reordered,
// or gain a job without touching this file.
//
// What must stay true:
//
//  1. every job runs on the repository-selected runner, or is a
//     reusable-workflow call that receives that runner label;
//  2. pull-request jobs run in the repository-selected immutable image,
//     never a literal registry tag, without setup-go, and with
//     GOTOOLCHAIN=local;
//  3. the fuzz workflow drives the sandbox in fuzz mode and no hosted
//     workflow reaches a shipped-product or local-only test mode;
//  4. the pull-request workflow keeps the gettext catalogs gated;
//  5. Pages deployment derives its authenticated remote from validated
//     Forgejo context rather than a hardcoded host;
//  6. forge-ci references match the audited contract at one pin, and no
//     release action or secret is admitted before independent authentication;
//  7. release preparation is forge-ci's pinned prepare workflow, behind a
//     token preflight of its own, and the SSH release key enters nothing else;
//  8. release builds use explicit job-local registry credentials and leave
//     signing and publication to the forge-ci signer.
package policy_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	runnerLabel     = "${{ vars.CI_RUNNER_LABEL }}"
	jobImage        = "${{ vars.CI_JOB_IMAGE }}"
	forgeCIPin      = "0c7c4354acc7b215da895d36204f9099dcff8406"
	forgeCITrust    = "forge-ci-trust"
	forgeCIVariable = "${{ vars.FORGE_CI_AUTHENTICATED_COMMIT }}"
	// One action and one reusable workflow of the forge-ci Forgejo layer,
	// each written out as the whole coordinate the workflows use. Every other
	// reference must share their repository and pin. They are whole literals
	// on purpose: a lab run that serves the layer from a disposable repository
	// rewrites the workflows' coordinates wherever they are quoted, and this
	// file then follows the workflows it checks.
	layerSetupAction     = "https://codefloe.com/itiquette/forge-ci/.forgejo/actions/setup-forge-ci@0c7c4354acc7b215da895d36204f9099dcff8406"
	layerPrepareWorkflow = "itiquette/forge-ci/.forgejo/workflows/prepare-release.yml@0c7c4354acc7b215da895d36204f9099dcff8406"
	// The trust inputs of the signer call. The key is public and comes from a
	// variable; the image and the key digest are literals, so neither moves
	// without a reviewed commit.
	signerImage       = "localhost/forge-ci-signer@sha256:d6cd6c2d40df8e7c82649879ff99c4bd33e023e3b3bf576697774eb63774e082"
	trustRootVariable = "${{ vars.COSIGN_PUBLIC_KEY }}"
	trustRootDigest   = "0a118a651aac6f751385aeaeb4242996ecc5b1b30c272d8da4a75369abd6b630"
)

// productPattern matches the shipped-product and local-only test surface
// that hosted workflows must not reach; see the local/sandbox launchers. The
// distribution matrix is part of that surface: its recipes, its image list and
// the launcher options that select it.
//
// The build-tag alternative is anchored to `go test` rather than matching a
// tag selector wherever it appears. What this guard forbids is a workflow
// *executing* the tagged tiers: they need a built product, a git fixture and
// the sandbox. `go vet -tags integration ./...` executes none of that -- it
// type-checks the tagged packages and stops -- and it is the only thing that
// notices when a tier no workflow runs stops compiling, so the pull-request
// workflow is allowed to run it. The anchor follows a shell line
// continuation, so splitting the invocation across lines does not slip past.
var productPattern = regexp.MustCompile(`just[[:space:]]+test-[[:alnum:]-]*(container|vm)\b|scripts/testing/(environment|vm|vm-guest|vm-tests|vm-trust-ca|vm-bootstrap|vm-bootstrap-guest)[.]sh|run-external-tests-local[.]sh|GOMMITLINT_(BLACKBOX|EXTERNAL)|test/blackbox|test/integration|go[[:space:]]+test([^\n]|\\\n)*tags[=[:space:]]*("[^"]*|'[^']*|[[:alnum:]_,]*)(blackbox|integration)|just[[:space:]]+test-(integration|blackbox|external)|--source-(bin|root|manifest|manifest-checksum)|test/distribution|just[[:space:]]+(container-test|build-distribution-images)|--distribution\b|--harness-(root|manifest)`)

type workflow struct {
	name string
	doc  map[string]any
}

func hostedWorkflows(t *testing.T) []workflow {
	t.Helper()

	pattern := filepath.Join(repoRoot(t), ".forgejo", "workflows", "*.yml")

	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) == 0 {
		t.Fatalf("no hosted workflows found at %s (err: %v)", pattern, err)
	}

	workflows := make([]workflow, 0, len(paths))

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s is not valid YAML: %v", path, err)
		}

		workflows = append(workflows, workflow{name: filepath.Base(path), doc: doc})
	}

	return workflows
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}

		dir = parent
	}
}

func jobs(doc map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}

	jobsNode, _ := doc["jobs"].(map[string]any)
	for name, spec := range jobsNode {
		if m, ok := spec.(map[string]any); ok {
			out[name] = m
		}
	}

	return out
}

func steps(job map[string]any) []map[string]any {
	list, _ := job["steps"].([]any)
	out := make([]map[string]any, 0, len(list))

	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}

	return out
}

func namedStep(t *testing.T, job map[string]any, name string) map[string]any {
	t.Helper()

	for _, step := range steps(job) {
		if step["name"] == name {
			return step
		}
	}

	t.Fatalf("job has no %q step", name)

	return nil
}

func readRepositoryFile(t *testing.T, path ...string) []byte {
	t.Helper()

	parts := append([]string{repoRoot(t)}, path...)

	raw, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

// scalars collects every string value under a node, so policies can scan
// run bodies, env values, with inputs and expressions alike.
func scalars(node any, out *[]string) {
	switch value := node.(type) {
	case string:
		*out = append(*out, value)
	case []any:
		for _, item := range value {
			scalars(item, out)
		}
	case map[string]any:
		for _, item := range value {
			scalars(item, out)
		}
	}
}

func runBodies(doc map[string]any) []string {
	var out []string

	for _, job := range jobs(doc) {
		for _, step := range steps(job) {
			if run, ok := step["run"].(string); ok {
				out = append(out, run)
			}
		}
	}

	return out
}

// hostedWorkflow returns the named workflow and fails when it is missing, so a
// rename cannot turn its policy tests into ones that check nothing.
func hostedWorkflow(t *testing.T, name string) workflow {
	t.Helper()

	for _, wf := range hostedWorkflows(t) {
		if wf.name == name {
			return wf
		}
	}

	t.Fatalf("hosted workflow %s not found", name)

	return workflow{}
}

func releaseWorkflow(t *testing.T) workflow {
	t.Helper()

	return hostedWorkflow(t, "release.yml")
}

// forgeCIReference splits a reference to the forge-ci Forgejo layer into
// its path below .forgejo/ and its pin. Actions are fetched by URL, reusable
// workflows by owner/repository path on the same instance; the two prefixes
// are those of the anchor coordinates above.
func forgeCIReference(uses string) (string, string, bool) {
	for _, prefix := range []string{
		strings.TrimSuffix(layerSetupAction, "actions/setup-forge-ci@"+forgeCIPin),
		strings.TrimSuffix(layerPrepareWorkflow, "workflows/prepare-release.yml@"+forgeCIPin),
	} {
		rest, ok := strings.CutPrefix(uses, prefix)
		if !ok {
			continue
		}

		subject, pin, ok := strings.Cut(rest, "@")

		return subject, pin, ok
	}

	return "", "", false
}

func jobNeeds(job map[string]any) []string {
	switch value := job["needs"].(type) {
	case string:
		return []string{value}
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if name, ok := item.(string); ok {
				out = append(out, name)
			}
		}

		return out
	default:
		return nil
	}
}

func dependsOn(jobMap map[string]map[string]any, jobName, dependency string, seen map[string]bool) bool {
	for _, needed := range jobNeeds(jobMap[jobName]) {
		if needed == dependency {
			return true
		}

		if seen[needed] {
			continue
		}

		seen[needed] = true
		if dependsOn(jobMap, needed, dependency, seen) {
			return true
		}
	}

	return false
}

func jobUsesForgeCI(job map[string]any) bool {
	if uses, ok := job["uses"].(string); ok {
		_, _, isForgeCI := forgeCIReference(uses)
		if isForgeCI {
			return true
		}
	}

	for _, step := range steps(job) {
		if uses, ok := step["uses"].(string); ok {
			_, _, isForgeCI := forgeCIReference(uses)
			if isForgeCI {
				return true
			}
		}
	}

	return false
}

// secretExpression matches the secrets context anywhere inside a workflow
// expression: ${{ secrets.X }}, the unspaced ${{secrets.X}}, or toJSON(secrets).
var secretExpression = regexp.MustCompile(`\$\{\{[^}]*\bsecrets\b`)

func referencesSecret(node any) bool {
	var values []string
	scalars(node, &values)

	return slices.ContainsFunc(values, secretExpression.MatchString)
}

func definesForgeCIVariable(node map[string]any) bool {
	env, _ := node["env"].(map[string]any)
	_, defined := env["FORGE_CI_AUTHENTICATED_COMMIT"]

	return defined
}

func TestJobsRunOnTheSelectedRunnerOrPassItToReusableWorkflows(t *testing.T) {
	t.Parallel()

	for _, wf := range hostedWorkflows(t) {
		for name, job := range jobs(wf.doc) {
			if uses, ok := job["uses"].(string); ok {
				with, _ := job["with"].(map[string]any)
				if strings.Contains(uses, "forge-ci") && with["runner-label"] != runnerLabel {
					t.Errorf("%s: job %s must pass runner-label %q to %s", wf.name, name, runnerLabel, uses)
				}

				continue
			}

			if job["runs-on"] != runnerLabel {
				t.Errorf("%s: job %s must select the repository-configured runner, got %v", wf.name, name, job["runs-on"])
			}
		}
	}
}

func TestPullRequestJobsUseTheSelectedImageAndToolchain(t *testing.T) {
	t.Parallel()

	wf := hostedWorkflow(t, "pullrequest.yml")

	jobMap := jobs(wf.doc)
	if len(jobMap) == 0 {
		t.Fatal("pullrequest.yml has no jobs")
	}

	for name, job := range jobMap {
		image := job["container"]
		if m, ok := image.(map[string]any); ok {
			image = m["image"]
		}

		if image != jobImage {
			t.Errorf("pullrequest.yml: job %s must run in %q, got %v", name, jobImage, image)
		}

		for _, step := range steps(job) {
			if uses, ok := step["uses"].(string); ok && strings.Contains(uses, "actions/setup-go@") {
				t.Errorf("pullrequest.yml: job %s must use the Go toolchain from the job image, not %s", name, uses)
			}
		}
	}

	env, _ := wf.doc["env"].(map[string]any)
	if env["GOTOOLCHAIN"] != "local" {
		t.Error("pullrequest.yml must set GOTOOLCHAIN: local to refuse automatic toolchain downloads")
	}
}

func TestHostedWorkflowsStayClearOfShippedProductTestModes(t *testing.T) {
	t.Parallel()

	for _, wf := range hostedWorkflows(t) {
		var values []string

		scalars(wf.doc, &values)

		if wf.name == "fuzz.yml" {
			fuzzMode := regexp.MustCompile(`run-external-tests-sandbox[.]sh[\s\\]+fuzz([\s\\]|$)`)
			productMode := regexp.MustCompile(`run-external-tests-sandbox[.]sh[\s\\]+(source|integration|blackbox|distribution)`)
			sawFuzz := false

			for _, value := range values {
				if fuzzMode.MatchString(value) {
					sawFuzz = true
				}

				if productMode.MatchString(value) {
					t.Errorf("fuzz.yml invokes a shipped-product sandbox mode: %q", value)
				}

				if productPattern.MatchString(value) || regexp.MustCompile(`go\s+build\b`).MatchString(value) {
					t.Errorf("fuzz.yml adds shipped-product inputs to the product-free fuzz workflow: %q", value)
				}
			}

			if !sawFuzz {
				t.Error("fuzz.yml must invoke run-external-tests-sandbox.sh in fuzz mode")
			}

			continue
		}

		for _, value := range values {
			if productPattern.MatchString(value) ||
				strings.Contains(value, "run-external-tests-sandbox.sh") ||
				strings.Contains(value, "prepare-sandbox-image.sh") {
				t.Errorf("%s references a local external-product or fuzz launcher: %q", wf.name, value)
			}
		}
	}
}

// TestFuzzHarnessIsBuiltWithCoverageInstrumentation pins the one build flag
// the campaign cannot report for itself. A harness compiled without
// -d=libfuzzer still runs, still exits 0 and still finds nothing beyond the
// seed corpus, so dropping the flag looks exactly like a clean campaign and
// only a guard catches it.
func TestFuzzHarnessIsBuiltWithCoverageInstrumentation(t *testing.T) {
	t.Parallel()

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "run-external-tests-sandbox.sh"))

	// The launcher compiles the harness exactly once; a second build site
	// would be one this test does not cover, so it has to fail here too.
	builds := regexp.MustCompile(`(?s)go test -c.*?-o "\$campaign_root/harness"`).FindAllString(script, -1)
	if len(builds) != 1 {
		t.Fatalf("sandbox launcher must precompile the campaign harness exactly once, found %d builds", len(builds))
	}

	if !strings.Contains(builds[0], "-gcflags=all=-d=libfuzzer") {
		t.Errorf("campaign harness build must carry -gcflags=all=-d=libfuzzer, got %q", builds[0])
	}
}

// TestSandboxTmpfsMountsCarryNoOwnershipOptions keeps the external tiers
// runnable on Podman 4. Only 5.x accepts uid= and gid= as tmpfs options;
// 4.9.3 -- what the current Debian and Ubuntu LTS ship -- refuses the whole
// run with `unknown mount option`, minutes into a tier, so a reintroduced
// option would read as an unrelated failure on exactly the hosts least able
// to diagnose it. The container runs a single unprivileged principal under
// --network=none, --read-only, --cap-drop=ALL and private ipc/uts, so the
// default 1777 these mounts take instead excludes nobody.
func TestSandboxTmpfsMountsCarryNoOwnershipOptions(t *testing.T) {
	t.Parallel()

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "run-external-tests-sandbox.sh"))

	// Comments discuss the option by name; only the argv is the contract.
	var code []string

	for line := range strings.SplitSeq(script, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			code = append(code, line)
		}
	}

	mounts := regexp.MustCompile(`--tmpfs[ =]"?([^"\s\\]+)`).FindAllStringSubmatch(strings.Join(code, "\n"), -1)
	if len(mounts) == 0 {
		t.Fatal("sandbox launcher has no --tmpfs mounts; this guard no longer reads the argv it was written for")
	}

	home := false

	for _, mount := range mounts {
		spec := mount[1]

		path, options, _ := strings.Cut(spec, ":")
		if path == "/home/sandbox" {
			home = true
		}

		for option := range strings.SplitSeq(options, ",") {
			name, _, _ := strings.Cut(option, "=")
			if name == "uid" || name == "gid" || name == "mode" {
				t.Errorf("--tmpfs %s carries %q; Podman 4.x rejects uid=/gid=, and mode= without them leaves the mount owned by a uid the container is not", path, option)
			}
		}
	}

	// The HOME mount is the one that had the options, so a rename that
	// escapes this loop has to fail rather than pass by absence.
	if !home {
		t.Error("sandbox launcher no longer mounts a /home/sandbox tmpfs; re-point this guard at whatever carries HOME")
	}
}

func TestProductPatternRefusesEveryLocalOnlyTierSpelling(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		value   string
		refused bool
	}{
		{value: "go test -tags=blackbox ./test/...", refused: true},
		{value: "go test -tags=integration ./...", refused: true},
		{value: "GOFLAGS=-tags=integration go test ./test/integration/...", refused: true},
		{value: "go test -tags=integration,blackbox ./...", refused: true},
		{value: `go test -tags "netgo integration" ./...`, refused: true},
		{value: "go test \\\n  -tags=blackbox ./...", refused: true},
		{value: "just test-integration", refused: true},
		// The tier now takes go test arguments; the refusal must not care
		// what follows the recipe name.
		{value: "just test-integration -run TestX -v", refused: true},
		{value: "just test-blackbox", refused: true},
		{value: "just test-blackbox-container", refused: true},
		{value: "just test-combined-container", refused: true},
		{value: "just test-combined-container-matrix fedora-44 arch", refused: true},
		{value: "just test-unit-container", refused: true},
		{value: "just test-unit-vm freebsd-amd64", refused: true},
		{value: "just test-race-vm macos-arm64", refused: true},
		{value: "just test-vm-prepare linux-arm64 profile.json", refused: true},
		{value: "just test-unit-container-on-macos", refused: true},
		{value: "just test-matrix-container-on-macos debian-13", refused: true},
		{value: "bash scripts/testing/vm-guest.sh", refused: true},
		{value: "bash scripts/testing/vm-tests.sh", refused: true},
		{value: "bash scripts/testing/test-vm.sh", refused: false},
		{value: "just test-container-prepare", refused: true},
		{value: "just  test-external", refused: true},
		{value: "just container-tests-full", refused: true},
		{value: "just container-test fedora-44 arch", refused: true},
		{value: "just build-distribution-images", refused: true},
		{value: "bash .forgejo/scripts/prepare-sandbox-image.sh --distribution arch", refused: true},
		{value: "run-external-tests-sandbox.sh distribution --harness-root x", refused: true},
		{value: "cat test/distribution/images", refused: true},
		{value: `bash .forgejo/scripts/prepare-sandbox-image.sh "${RUNNER_TEMP}/sandbox-image"`, refused: false},
		{value: "go list ./... | grep -v generated | xargs go test -count=1 -race -buildvcs=false", refused: false},
		{value: "go test ./test/policy", refused: false},
		{value: `go build -tags "netgo osusergo" ./cmd/gommitlint`, refused: false},
		// Compile-only: vet builds the tagged packages and runs nothing.
		{value: "go vet -tags integration ./...", refused: false},
		{value: "go vet -tags blackbox ./...", refused: false},
		{value: `tag_ref="refs/tags/${RELEASE_REQUEST}"`, refused: false},
		{value: "just test-fuzz", refused: false},
		{value: "just lint", refused: false},
	} {
		if got := productPattern.MatchString(testCase.value); got != testCase.refused {
			t.Errorf("productPattern.MatchString(%q) = %v, want %v", testCase.value, got, testCase.refused)
		}
	}
}

func TestPullRequestWorkflowGatesTheGettextCatalogs(t *testing.T) {
	t.Parallel()

	runs := strings.Join(runBodies(hostedWorkflow(t, "pullrequest.yml").doc), "\n")

	for _, required := range []string{
		"./scripts/lint/translation-drift.sh",
		"command -v msgmerge",
		"command -v msgen",
		"command -v msgattrib",
		"command -v msgfmt",
	} {
		if !strings.Contains(runs, required) {
			t.Errorf("pullrequest.yml must run %q", required)
		}
	}

	if !regexp.MustCompile(`(?m)^\s*msgfmt(\s.*)?locale/gommitlint[.]pot(\s|$)`).MatchString(runs) {
		t.Error("pullrequest.yml must validate the POT catalog with msgfmt")
	}

	if !regexp.MustCompile(`(?m)^\s*for\s+[^;]+\s+in\s+locale/[*]/gommitlint[.]po([;\s]|$)`).MatchString(runs) {
		t.Error("pullrequest.yml must validate every PO catalog")
	}
}

func TestPagesDeployDerivesValidatedRemoteFromForgejoContext(t *testing.T) {
	t.Parallel()

	for _, wf := range hostedWorkflows(t) {
		if wf.name != "pages-deploy.yml" {
			continue
		}

		deploy := jobs(wf.doc)["deploy"]
		for _, step := range steps(deploy) {
			if step["name"] != "Deploy to pages branch" {
				continue
			}

			env, _ := step["env"].(map[string]any)
			if env["FORGEJO_SERVER"] != "${{ forgejo.server_url }}" {
				t.Errorf("pages deploy must source FORGEJO_SERVER from forgejo.server_url, got %v", env["FORGEJO_SERVER"])
			}

			if env["REPO"] != "${{ forgejo.repository }}" {
				t.Errorf("pages deploy must source REPO from forgejo.repository, got %v", env["REPO"])
			}

			run, _ := step["run"].(string)
			for _, required := range []string{
				"const url = new URL(rawServer)",
				`rawServer === origin || rawServer === origin + "/"`,
				`url.protocol !== "https:"`,
				`url.username !== ""`,
				`url.password !== ""`,
				`url.pathname !== "/"`,
				`url.search !== ""`,
				`url.hash !== ""`,
				"parts.length === 2",
				"part.length > 0",
				"process.stdout.write(origin)",
				`remote="${server}/${REPO}.git"`,
				`git remote add origin "${remote}"`,
			} {
				if !strings.Contains(run, required) {
					t.Errorf("pages deploy must retain remote validation contract %q", required)
				}
			}

			if strings.Contains(run, "codefloe.com") {
				t.Error("pages deploy must not hardcode the Forgejo server host")
			}

			return
		}

		t.Fatal("pages-deploy.yml has no Deploy to pages branch step")
	}

	t.Fatal("pages-deploy.yml is not part of the hosted workflow set")
}

// layerReference recognizes shared delivery references by their path shape,
// so a foreign repository cannot evade the reviewed-coordinate check.
func layerReference(uses string) bool {
	return strings.Contains(uses, "/.forgejo/actions/") || strings.Contains(uses, "/.forgejo/workflows/")
}

// Input declarations are checked by the pinned engine's isolation action.
// This repository owns which revision it reviewed, not a copy of that API.
func TestForgeCIReferencesUseTheReviewedPin(t *testing.T) {
	t.Parallel()

	references := 0
	checkReference := func(location string, node map[string]any) {
		uses, ok := node["uses"].(string)
		if !ok {
			return
		}

		subject, pin, isForgeCI := forgeCIReference(uses)
		if !isForgeCI {
			if layerReference(uses) {
				t.Errorf("%s has an unsupported delivery-layer reference: %s", location, uses)
			}

			return
		}

		references++

		if pin != forgeCIPin {
			t.Errorf("%s must pin forge-ci to %s, got %s", location, forgeCIPin, pin)
		}

		with, _ := node["with"].(map[string]any)

		// A called workflow checks its helper files out at forge-ci-sha, so
		// the commit is written out a second time and must be the same one.
		if strings.HasPrefix(subject, "workflows/") && with["forge-ci-sha"] != forgeCIPin {
			t.Errorf("%s must pass forge-ci-sha %s, got %v", location, forgeCIPin, with["forge-ci-sha"])
		}
	}

	for _, wf := range hostedWorkflows(t) {
		for jobName, job := range jobs(wf.doc) {
			checkReference(wf.name+":"+jobName, job)

			for _, step := range steps(job) {
				stepName, _ := step["name"].(string)

				checkReference(wf.name+":"+jobName+":"+stepName, step)
			}
		}
	}

	if references == 0 {
		t.Fatal("no forge-ci references found in hosted workflows")
	}
}

func TestReleaseSignerReceivesTheOrganizationTrustRootAsReviewedLiterals(t *testing.T) {
	t.Parallel()

	signer := jobs(releaseWorkflow(t).doc)["sign-and-publish"]
	with, _ := signer["with"].(map[string]any)

	// That the image and the key digest are literals of the right shape is
	// check-l3-isolation's to hold. This test holds which ones they are.
	if with["signer-image"] != signerImage {
		t.Errorf("signer image must be the reviewed digest-pinned literal %s, got %v", signerImage, with["signer-image"])
	}

	if with["cosign-public-key"] != trustRootVariable {
		t.Errorf("signer must read the Cosign public key from %s, got %v", trustRootVariable, with["cosign-public-key"])
	}

	if with["cosign-public-key-sha256"] != trustRootDigest {
		t.Errorf("signer must bind the Cosign public key to the literal digest %s, got %v", trustRootDigest, with["cosign-public-key-sha256"])
	}

	if with["release-request-sha"] != "${{ forgejo.sha }}" {
		t.Errorf("signer must take the request commit from the event context, got %v", with["release-request-sha"])
	}

	if signer["secrets"] == "inherit" {
		t.Error("signer secrets must be enumerated, never inherited")
	}
}

func assertActionFreeForgeCITrust(t *testing.T, trust map[string]any) map[string]any {
	t.Helper()

	if _, ok := trust["uses"]; ok {
		t.Errorf("%s must be an inline job, not a reusable-workflow call", forgeCITrust)
	}

	if _, ok := trust["continue-on-error"]; ok {
		t.Errorf("%s must not permit authentication failure", forgeCITrust)
	}

	trustSteps := steps(trust)

	if len(trustSteps) != 1 {
		t.Fatalf("%s must contain exactly one inline authentication step, got %d", forgeCITrust, len(trustSteps))
	}

	step := trustSteps[0]

	if _, ok := step["uses"]; ok {
		t.Errorf("%s must not execute action code", forgeCITrust)
	}

	if _, ok := step["continue-on-error"]; ok {
		t.Errorf("%s authentication step must not permit failure", forgeCITrust)
	}

	if referencesSecret(trust) {
		t.Errorf("%s must not receive release secrets", forgeCITrust)
	}

	return step
}

func assertForgeCIComparison(t *testing.T, step map[string]any) {
	t.Helper()

	env, _ := step["env"].(map[string]any)

	if env["FORGE_CI_AUTHENTICATED_COMMIT"] != forgeCIVariable {
		t.Errorf("%s must read the protected authentication variable directly, got %v", forgeCITrust, env["FORGE_CI_AUTHENTICATED_COMMIT"])
	}

	run, _ := step["run"].(string)

	for _, required := range []string{
		"expected='" + forgeCIPin + "'",
		`if [ "${FORGE_CI_AUTHENTICATED_COMMIT}" != "${expected}" ]; then`,
		"exit 1",
	} {
		if !strings.Contains(run, required) {
			t.Errorf("%s must retain fail-closed authentication contract %q", forgeCITrust, required)
		}
	}

	if regexp.MustCompile(`(?m)^\s*(export\s+)?FORGE_CI_AUTHENTICATED_COMMIT=`).MatchString(run) {
		t.Errorf("%s must not manufacture the protected authentication variable", forgeCITrust)
	}
}

func assertSingleForgeCIVariableSource(t *testing.T, release workflow) {
	t.Helper()

	if definesForgeCIVariable(release.doc) {
		t.Error("release.yml must not default the protected authentication variable at workflow scope")
	}

	for name, job := range jobs(release.doc) {
		if definesForgeCIVariable(job) {
			t.Errorf("release.yml: job %s must not default the protected authentication variable", name)
		}

		if name == forgeCITrust {
			continue
		}

		for _, step := range steps(job) {
			if definesForgeCIVariable(step) {
				t.Errorf("release.yml: only %s may source the protected authentication variable", forgeCITrust)
			}
		}
	}

	var values []string

	scalars(release.doc, &values)

	variableSources := 0

	for _, value := range values {
		if value == forgeCIVariable {
			variableSources++
		}

		if strings.Contains(value, "vars.FORGE_CI_AUTHENTICATED_COMMIT") && value != forgeCIVariable {
			t.Errorf("release.yml must not transform or default the protected authentication variable: %q", value)
		}
	}

	if variableSources != 1 {
		t.Errorf("release.yml must source the protected authentication variable exactly once, got %d", variableSources)
	}
}

func assertForgeCITrustDependencies(t *testing.T, jobMap map[string]map[string]any) {
	t.Helper()

	guardedJobs := 0

	for name, job := range jobMap {
		if name == forgeCITrust || (!jobUsesForgeCI(job) && !referencesSecret(job)) {
			continue
		}

		guardedJobs++

		if !dependsOn(jobMap, name, forgeCITrust, map[string]bool{name: true}) {
			t.Errorf("release.yml: job %s uses forge-ci or receives a secret but does not depend transitively on %s", name, forgeCITrust)
		}
	}

	if guardedJobs == 0 {
		t.Fatal("release.yml has no forge-ci or secret-bearing jobs guarded by authentication")
	}
}

func TestReleaseWorkflowAuthenticatesForgeCIBeforeActionsOrSecrets(t *testing.T) {
	t.Parallel()

	release := releaseWorkflow(t)
	jobMap := jobs(release.doc)
	trust, ok := jobMap[forgeCITrust]

	if !ok {
		t.Fatalf("release.yml must define the %s job", forgeCITrust)
	}

	step := assertActionFreeForgeCITrust(t, trust)

	assertForgeCIComparison(t, step)
	assertSingleForgeCIVariableSource(t, release)
	assertForgeCITrustDependencies(t, jobMap)
}

func TestReferencesSecretMatchesEveryExpressionSpelling(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		value string
		want  bool
	}{
		{value: "${{ secrets.RELEASE_GPG_PRIVATE_KEY }}", want: true},
		{value: "${{secrets.RELEASE_GPG_PRIVATE_KEY}}", want: true},
		{value: `echo "${{ toJSON(secrets) }}"`, want: true},
		{value: "${{ vars.CI_RUNNER_LABEL }}", want: false},
		{value: "echo no secrets here", want: false},
	} {
		if got := referencesSecret(testCase.value); got != testCase.want {
			t.Errorf("referencesSecret(%q) = %v, want %v", testCase.value, got, testCase.want)
		}
	}
}

// TestReleasePrepareIsThePinnedWorkflowBehindATokenPreflight holds the prepare
// job to forge-ci's prepare workflow. That workflow owns what this
// repository used to compose by hand: the secret presence check before any
// checkout, the annotated request tag bound to the event commit, and the
// request admitted only at the head of main before the SSH key is loaded. A
// prepare job with steps of its own would run repository code beside that key.
func TestReleasePrepareIsThePinnedWorkflowBehindATokenPreflight(t *testing.T) {
	t.Parallel()

	jobMap := jobs(releaseWorkflow(t).doc)
	prepare := jobMap["prepare"]

	prepareUses, _ := prepare["uses"].(string)
	if prepareUses != layerPrepareWorkflow {
		t.Errorf("release preparation must be the pinned prepare-release workflow, got %q", prepareUses)
	}

	if len(steps(prepare)) != 0 {
		t.Error("the prepare job must not run steps of its own beside the SSH release key")
	}

	secrets, _ := prepare["secrets"].(map[string]any)
	if secrets["RELEASE_SSH_SIGNING_KEY"] != "${{ secrets.RELEASE_SSH_SIGNING_KEY }}" {
		t.Errorf("prepare-release must receive only the declared SSH release key, got %v", secrets["RELEASE_SSH_SIGNING_KEY"])
	}

	with, _ := prepare["with"].(map[string]any)
	for _, unsupported := range []string{"verify-release-request-signature", "release-request-allowed-signers"} {
		if _, ok := with[unsupported]; ok {
			t.Errorf("prepare-release must not claim an unavailable request-tag signature policy through %s", unsupported)
		}
	}

	// The prepare workflow verifies no token: no token may enter the job that
	// holds the SSH key. The check is a job of this workflow that prepare
	// waits for.
	verify := namedStep(t, jobMap["preflight"], "Verify Forgejo package token")

	verifyUses, _ := verify["uses"].(string)
	if !strings.HasSuffix(verifyUses, "/.forgejo/actions/verify-forgejo-token@"+forgeCIPin) {
		t.Errorf("the token preflight must use the pinned verify-forgejo-token action, got %q", verifyUses)
	}

	if !slices.Contains(jobNeeds(prepare), "preflight") {
		t.Error("the prepare job must wait for the token preflight")
	}

	for name, job := range jobMap {
		if name == "prepare" {
			continue
		}

		var values []string

		scalars(job, &values)

		for _, value := range values {
			if strings.Contains(value, "RELEASE_SSH_SIGNING_KEY") {
				t.Errorf("release.yml: job %s must not reference the SSH release key", name)
			}
		}
	}
}

func TestReleaseBuildUsesExplicitJobLocalRegistryCredentials(t *testing.T) {
	t.Parallel()

	release := releaseWorkflow(t)

	workflowPermissions, _ := release.doc["permissions"].(map[string]any)
	if _, ok := workflowPermissions["packages"]; ok {
		t.Error("release workflow must not grant ambient package-write permission")
	}

	build := jobs(release.doc)["build-and-release"]

	permissions, _ := build["permissions"].(map[string]any)
	if _, ok := permissions["packages"]; ok {
		t.Error("release build must not request ambient package-write permission")
	}

	login := namedStep(t, build, "Login to container registry")
	if login["id"] != "registry-login" {
		t.Errorf("registry login must expose the pinned auth-file output through id registry-login, got %v", login["id"])
	}

	loginWith, _ := login["with"].(map[string]any)
	if loginWith["create-auth-file"] != "true" {
		t.Errorf("registry login must create a job-local auth file, got %v", loginWith["create-auth-file"])
	}

	if _, ok := loginWith["export-env"]; ok {
		t.Error("registry login must not export ambient registry credentials")
	}

	buildActions := 0

	for _, step := range steps(build) {
		uses, _ := step["uses"].(string)
		if !strings.Contains(uses, "/.forgejo/actions/build-release-image@"+forgeCIPin) {
			continue
		}

		buildActions++

		with, _ := step["with"].(map[string]any)
		if with["auth-file"] != "${{ steps.registry-login.outputs.auth-file }}" {
			t.Errorf("image build %q must receive the registry-login auth file explicitly, got %v", step["name"], with["auth-file"])
		}
	}

	if buildActions != 2 {
		t.Errorf("release workflow must retain two release image builds, got %d", buildActions)
	}
}

func TestReleaseBuildConfigurationHasNoPublisher(t *testing.T) {
	t.Parallel()

	var config map[string]any
	if err := yaml.Unmarshal(readRepositoryFile(t, ".goreleaser.yaml"), &config); err != nil {
		t.Fatalf(".goreleaser.yaml is not valid YAML: %v", err)
	}

	for _, forbidden := range []string{"gitea_urls", "release"} {
		if _, ok := config[forbidden]; ok {
			t.Errorf(".goreleaser.yaml must leave %s configuration to the forge-ci signer", forbidden)
		}
	}

	for _, required := range []string{
		"builds", "source", "archives", "nfpms", "checksum", "metadata", "sboms", "report_sizes",
	} {
		if _, ok := config[required]; !ok {
			t.Errorf(".goreleaser.yaml must retain artifact configuration %s", required)
		}
	}

	build := jobs(releaseWorkflow(t).doc)["build-and-release"]
	goreleaser := namedStep(t, build, "Run GoReleaser")

	run, _ := goreleaser["run"].(string)
	if !strings.Contains(run, "goreleaser release --clean --verbose --skip=publish") {
		t.Error("release workflow must keep GoReleaser publication disabled at invocation")
	}

	// The signer publishes what the producer's manifest declares and refuses
	// a hand-off without one.
	declare := namedStep(t, build, "Declare the release files")

	declareRun, _ := declare["run"].(string)
	if !strings.Contains(declareRun, "forge-ci release files import-goreleaser") {
		t.Error("release build must declare its release files from GoReleaser's metadata")
	}
}

func TestReleaseNotesDeriveValidatedForgejoProjectURL(t *testing.T) {
	t.Parallel()

	releaseEnv, _ := releaseWorkflow(t).doc["env"].(map[string]any)
	if releaseEnv["FORGEJO_SERVER"] != "${{ forgejo.server_url }}" {
		t.Errorf("release notes must source FORGEJO_SERVER from forgejo.server_url, got %v", releaseEnv["FORGEJO_SERVER"])
	}

	if releaseEnv["FORGEJO_REPO"] != "${{ forgejo.repository }}" {
		t.Errorf("release notes must source FORGEJO_REPO from forgejo.repository, got %v", releaseEnv["FORGEJO_REPO"])
	}

	script := string(readRepositoryFile(t, ".forgejo", "scripts", "generate-release-notes.sh"))
	for _, required := range []string{
		`${FORGEJO_SERVER:?Forgejo server required}`,
		`${FORGEJO_REPO:?Forgejo repository required}`,
		`parsed = new URL(value)`,
		`parsed.protocol !== "https:"`,
		`parsed.origin !== value`,
		`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`,
		`REPOSITORY_OWNER=${FORGEJO_REPO%%/*}`,
		`REPOSITORY_NAME=${FORGEJO_REPO#*/}`,
		`PROJECT_URL="${SERVER_URL}/${FORGEJO_REPO}"`,
		`${PROJECT_URL}/src/tag/${VERSION}/docs/modules/ROOT/pages/verify.adoc`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("release notes must retain validated Forgejo URL contract %q", required)
		}
	}

	if strings.Contains(script, "codefloe.com") {
		t.Error("release notes must not hardcode the Codefloe server or repository")
	}
}
