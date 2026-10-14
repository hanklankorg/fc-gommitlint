#!/usr/bin/env just --justfile

# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# nanolinter integration. Installed via mise (.mise.toml pins the version).
# Override with NANOLINTER=/path/to/binary for local dev against a build.
# just's default shell is `sh -cu` -- no pipefail, so a failing producer in a
# piped one-liner is silently swallowed. Run every recipe line under bash with
# the full strict triad instead.
set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments
set lazy

nanolinter_cmd := env("NANOLINTER", "nanolinter")

# Just validates these choices before running recipe dependencies.
vm_targets := 'freebsd-amd64|freebsd-arm64|openbsd-amd64|openbsd-arm64|netbsd-amd64|netbsd-arm64|macos-arm64|linux-arm64'
# Empty means help only; _require-vm-target refuses it before any VM work.
vm_target_argument := '(' + vm_targets + ')?'
test_distributions := 'ubuntu-26\.04|debian-13|fedora-44|opensuse-leap-16|opensuse-tumbleweed|arch'
tool_args_help := 'Extra arguments passed to the underlying tool; use --help for its options'

# Single source of truth for packaging version: derived from the latest git
# tag (with leading "v" stripped). Falls back to the .mise.toml pin when no
# tag exists yet (fresh clone of a shallow archive).
package_version := `if v=$(git describe --tags --abbrev=0 2>/dev/null) && [ -n "$v" ]; then printf '%s' "${v#v}"; else grep -m1 '"forgejo:itiquette/gommitlint"' .mise.toml | sed -E 's/.*"([^"]+)".*$/\1/'; fi`

# Build paths
bin := "./bin" # Build output directory
dist := "./dist" # Release output directory (goreleaser)
executable := "gommitlint" # Binary name
image_archive := bin + "/" + executable + "-image.tar" # build-image export

# Bold ANSI colour codes. Use {{CYAN}}…{{NC}} in recipe bodies instead of
# inlining escape sequences, so theme/contrast adjustments live in one place.
CYAN := "\\033[1;36m"
GREEN := "\\033[0;32m"
RED := "\\033[1;31m"
DIM := "\\033[2m"
NC := "\\033[0m"

# ==================================================================================== #
# DEFAULT - Show available recipes
# ==================================================================================== #

[private]
default:
    @printf ' {{ CYAN }}Gommitlint recipes{{ NC }}\n\n Quick start: {{ GREEN }}mise trust · just install-dev-tools · just verify-fix{{ NC }}\n Argument help: just --usage <recipe>\n ▪ main job of its group\n\n'
    @"{{ just_executable() }}" --justfile "{{ justfile() }}" --list --unsorted --list-heading ''

# ==================================================================================== #
# HELPER FUNCTIONS
# ==================================================================================== #

# Print cyan header with optional command in dim
_header text cmd="":
    #!/usr/bin/env bash
    text=$1
    cmd=${2:-}
    if [[ -n "$cmd" ]]; then
        printf "{{ CYAN }}%s{{ NC }}\n" "$text"
        printf " {{ DIM }}%s{{ NC }}\n" "$cmd"
    else
        printf "{{ CYAN }}%s{{ NC }}\n" "$text"
    fi

# Announce a job before its work or dependencies begin.
_job name details:
    #!/usr/bin/env bash
    printf '== Job: %s | host=%s/%s\n   %s\n' "$1" "$(uname -s)" "$(uname -m)" "$2" >&2

[arg('recipe', help='Recipe name; omit to list recipes. Prints help without running the selected job.')]
[doc('Show recipe argument help')]
[group('help')]
help recipe="":
    #!/usr/bin/env bash
    if [[ -n "$1" ]]; then
        {{ quote(just_executable()) }} --justfile {{ quote(justfile()) }} --usage="$1"
    else
        {{ quote(just_executable()) }} --justfile {{ quote(justfile()) }} --list --unsorted
    fi

[no-exit-message]
_require-vm-target target recipe:
    #!/usr/bin/env bash
    if [[ -z "$1" ]]; then
        printf 'Missing VM target for %s. Choose a target explicitly.\n\n' "$2" >&2
        {{ quote(just_executable()) }} --justfile {{ quote(justfile()) }} --usage="$2" >&2
        exit 2
    fi

# Show captured command output only on failure
_run_with_output desc +args:
    #!/usr/bin/env bash
    set -euo pipefail
    desc=$1
    shift
    printf " {{ DIM }}"
    printf '%q ' "$@"
    printf "{{ NC }}\n"
    output=$("$@" 2>&1) || {
        code=$?
        printf "{{ RED }}✗{{ NC }} %s failed\n" "$desc"
        printf "%s\n" "$output"
        exit "$code"
    }
    printf "{{ GREEN }}✓{{ NC }} %s completed\n" "$desc"

# Require an external tool and show an install hint
# Example prerequisite: `(_require-tool "podman")`.
_require-tool tool hint="":
    #!/usr/bin/env bash
    tool=$1
    hint=${2:-}
    if command -v "$tool" >/dev/null 2>&1; then
        exit 0
    fi
    printf "{{ RED }}✗ %s not found{{ NC }}\n" "$tool" >&2
    if [[ -n "$hint" ]]; then
        printf "  %s\n" "$hint" >&2
    fi
    exit 1

# Print the host Go architecture or reject the host
_host-goarch:
    #!/usr/bin/env bash
    case "$(uname -m)" in
        x86_64) printf "amd64" ;;
        aarch64|arm64) printf "arm64" ;;
        *) printf "{{ RED }}✗ Unsupported host architecture: %s{{ NC }}\n" "$(uname -m)" >&2; exit 1 ;;
    esac

# Build Linux for the host architecture
_build-linux-host:
    @"{{ just_executable() }}" _build_binary linux "$("{{ just_executable() }}" _host-goarch)"

# Internal cross-compilation helper
_build_binary goos goarch:
    @./scripts/build/build-cross-platform.sh "$1" "$2" {{ quote(bin) }} {{ quote(executable) }}

# ==================================================================================== #
# SETUP - Development environment setup
# ==================================================================================== #

# ▪ Install the pinned development tools
[doc('▪ Install pinned tools')]
[group('setup')]
install-dev-tools:
    @"{{ just_executable() }}" _job "install-dev-tools" "tools=.mise.toml + tools/go.mod; network=allowed; scope=user tool installations"
    @TOOLS=$(awk '/^\[tools\]/{t=1; next} /^\[/{t=0} t && /^[^#[:space:]]/ && /=/ { sub(/[[:space:]]*=.*/, ""); gsub(/"/, ""); print }' .mise.toml | tr '\n' ' '); \
     if [[ -n "$TOOLS" ]]; then mise install --locked $TOOLS; else printf "No mise tools found\n"; fi
    @"{{ just_executable() }}" install-dev-tools-go

# Install Go development tools
[group('setup')]
install-dev-tools-go:
    @"{{ just_executable() }}" _header "Install Go development tools" "./scripts/dev/install-go-dev-tools.sh"
    ./scripts/dev/install-go-dev-tools.sh

# Reports per-check policy + which tool each requires + missing-tool install hints.
[doc('Show the lint plan and its tools')]
[group('setup')]
doctor:
    @{{ quote(nanolinter_cmd) }} doctor

# ==================================================================================== #
# DEVELOPMENT - Development workflow
# ==================================================================================== #

# Non-fixing gate; tools can write caches/temp files and access the network.
# The mutating fix-then-check loop is `just verify-fix`.
# Includes lint, tagged-tier compilation and in-process tests.
[doc('▪ Run local checks')]
[group('development')]
verify: (_job "verify" "environment=native; mode=non-fixing; checks=lint + tagged compile-checks + unit/fuzz seeds") lint _test-tags-compile test

# Run verify plus drift and release checks
[doc('Add drift and release checks')]
[group('development')]
verify-full: (_job "verify-full" "checks=verify + release/drift checks + binary matrix + repeat builds; publication=none") verify release-check check-alpine-pins check-schema check-translations check-gettext-catalogs check-localized-manpages build-all verify-reproducible

# Mutating on purpose: the local before-you-commit loop -- fix, then gate.
# Lint runs with --allow-dirty so the fixes themselves do not fail it.
[doc('Fix, then run local checks')]
[group('development')]
verify-fix: (_job "verify-fix" "environment=native; mode=apply fixes, then check; checks=lint + tagged compile-checks + unit/fuzz seeds") lint-fix (lint "--allow-dirty") _test-tags-compile test

# ==================================================================================== #
# TEST SETUP - Environment checks, image preparation and cleanup
# ==================================================================================== #

# Report/check the explicitly selected environment without launching tests.
[arg('mode', pattern='native|container', help='Valid arguments: native, container')]
[doc('Check test environment')]
[group('test-setup')]
test-environment mode="native":
    @bash scripts/testing/environment.sh "$1"

# Record the sandbox OCI archive, checksum and Containerfile key separately
# from test execution. Rebuild after changing Containerfile.sandbox or cleaning
# bin/. Test runs load this recording with --pull=never; host-side Go module
# preparation can still use the network.
[doc('Build image; network')]
[group('test-setup')]
test-container-prepare: (test-environment "container")
    @bash .forgejo/scripts/prepare-sandbox-image.sh

# Record selected distributions, or all names from test/distribution/images.
# Rebuild after changing test/distribution/ or cleaning bin/.
[arg('names', pattern=test_distributions, help='Distribution names; omit for all. Valid arguments are listed in the pattern')]
[doc('Build images; network')]
[group('test-setup')]
test-container-prepare-distributions *names: (test-environment "container")
    #!/usr/bin/env bash
    set -euo pipefail
    source .forgejo/scripts/sandbox-common.sh
    names=("$@")
    ((${#names[@]} > 0)) || mapfile -t names < <(sandbox_distribution_names "$SANDBOX_DISTRIBUTION_LIST")
    for name in "${names[@]}"; do
        bash .forgejo/scripts/prepare-sandbox-image.sh --distribution "$name"
    done

# Each test run removes its private Podman store; only the recordings remain.
[doc('Remove recorded images')]
[group('test-setup')]
test-container-clean-distributions:
    #!/usr/bin/env bash
    set -euo pipefail
    for path in bin bin/distribution-images; do
        if [[ -L "$path" ]]; then
            printf 'Refusing symlink cleanup path: %s\n' "$path" >&2
            exit 2
        fi
    done
    "{{ just_executable() }}" _job "test-container-clean-distributions" "remove=bin/distribution-images (recorded test images)"
    rm -rf -- bin/distribution-images

[arg('profile', help='Optional custom development-image JSON; defaults to the checked-in target profile')]
[arg('target', pattern=vm_target_argument, help='Required to prepare a guest; omit for help. Example: just test-vm-prepare freebsd-amd64. Choices are listed in the pattern.')]
[doc('Prepare VM; network')]
[group('test-setup')]
test-vm-prepare target="" profile="": (_require-vm-target target "test-vm-prepare")
    @bash scripts/testing/vm.sh prepare "$1" "$2"

[arg('target', pattern=vm_target_argument, help='Required to remove a prepared VM; omit for help. Example: just test-vm-clean freebsd-amd64. Choices are listed in the pattern.')]
[doc('Remove prepared VM')]
[group('test-setup')]
test-vm-clean target="": (_require-vm-target target "test-vm-clean")
    @bash scripts/testing/vm.sh clean "$1"

[doc('Remove downloaded VM media; refuses active targets and preserves results')]
[group('test-setup')]
test-vm-clean-downloads:
    @bash scripts/testing/vm.sh clean-downloads

# ==================================================================================== #
# TEST - Testing and coverage
# ==================================================================================== #

# The everyday tier, and what CI runs on every pull request: in-process Go
# tests only, with no container, no network and no git fixture.
# ▪ Run default tests
[group('test')]
test: test-unit-native

# Everything in process, under the race detector, generated code aside: the
# product, shared test helpers, and repository checks under test/policy.
# The two ran separately for a while so that helper coverage would stop
# flattering the product number, but neither `test-coverage` nor the
# pull-request workflow ever kept them apart, so the split reported nothing it
# promised and cost a second run.
# Extra arguments go to `go test`, which is why there is no verbose twin:
#
#   just test-unit-native -v
#   just test-unit-native -run TestSubjectRule -failfast
#
# Run unit and seed tests
[arg('args', help='Arguments passed to go test, for example -v or -run TestName')]
[group('test')]
test-unit-native *args:
    @bash scripts/testing/environment.sh native --suite "unit + fuzz seeds (race)"
    @"{{ just_executable() }}" _header "Run unit tests" "go test -count=1 -race <all packages>"
    go test -count=1 -race -buildvcs=false "$@" $(go list './...' | grep -v generated)

# The same untagged packages and seed corpora, explicitly in the Go container.
[arg('args', help='Arguments passed to go test, for example -v or -run TestName')]
[doc('Test units; network prep')]
[group('test')]
test-unit-container *args:
    @bash scripts/testing/environment.sh container --suite "unit + fuzz seeds (race)" -- bash .forgejo/scripts/run-external-tests-local.sh unit "$@"

# One package, once the failure is located:
#
#   just test-pkg-native ./internal/domain/rules/
#
# Run one package
[arg('pkg', help='Go package path, for example ./internal/domain/rules')]
[group('test')]
test-pkg-native pkg:
    @bash scripts/testing/environment.sh native --suite "unit + fuzz seeds (package, race)"
    @"{{ just_executable() }}" _header "Run package tests" "go test -count=1 -race <package>"
    go test -count=1 -race -buildvcs=false "$1"

# One test, or a family of them, by name across every package:
#
#   just test-run-native TestSubjectRule
#
# Run matching tests
[arg('filter', help='Go test-name regular expression, for example ^TestSubject')]
[group('test')]
test-run-native filter:
    @bash scripts/testing/environment.sh native --suite "unit + fuzz seeds (filtered, race)"
    @"{{ just_executable() }}" _header "Run tests by filter" "go test -run=<filter> ./..."
    go test -count=1 -race -buildvcs=false -run="$1" $(go list './...' | grep -v generated)

# The sandbox tiers, all through .forgejo/scripts/run-external-tests-local.sh:
# test-integration-container runs ./test/integration/...; test-blackbox-container runs the
# whole black-box package, locale, PTY, resource-limit, zero-egress, GPG and
# SSH rows included; test-combined-container runs integration and black-box in one
# sandbox, in one pass rather than two.
#
# Each of them loads the image `just test-container-prepare` recorded, and
# refuses with that recipe named when the recording is missing or no longer
# matches Containerfile.sandbox. None of them builds an image or reaches a
# registry. Host-side builds and module preparation do use the network;
# only execution inside the container is offline.
#
# Those rows once had a tier of their own, because a missing gpg, ssh-keygen or
# script(1) fails them rather than skipping them, and that is a bad bargain on
# an arbitrary host. They never run on one: they run in the sandbox image this
# repository builds, which carries the tools.
#
# test-integration-container forwards extra arguments to the `go test` that runs inside
# the sandbox. The other sandbox tiers run fixed commands and refuse extra
# arguments rather than ignore them.

# gommitlint driven against real git repositories and real keys rather than
# fakes, which is where wiring bugs the unit tests cannot see turn up.
#
# Extra arguments go to `go test` in the sandbox, hence no verbose twin:
#
#   just test-integration-container -v
#   just test-integration-container -run TestSigning
#
# Run integration tests; container, network preparation
[arg('args', help='Arguments passed to go test, for example -v or -run TestName')]
[doc('Run integration; network prep')]
[group('test')]
test-integration-container *args:
    @bash scripts/testing/environment.sh container --suite "integration" -- bash .forgejo/scripts/run-external-tests-local.sh integration "$@"

# Compile-check the integration and black-box tags.
#
# go build, go vet, and `just test` all walk past a tagged file as if it were
# not there, and no workflow runs the tiers those tags select, so a tier can
# stop compiling with nothing going red. Something already had: the ec_tests
# tier stopped building when the function it covered was folded into
# HasMinimumGPGKeyStrength, and stayed broken because no gate compiled the tag.
# The pull-request workflow now runs the same two vets, so this is the local
# half of one gate; keep the tag lists on both sides the same.
#
# The combinations are the point, not the tag names: add the pairing a source
# file actually asks for, not the tag on its own.
#
# Vet rather than test: this answers "does it still build", in seconds, without
# a shipped binary, a sandbox, or a git repository.
# Private: a gate step, not a tier anyone picks from the page. The gates
# below are the only callers.
_test-tags-compile:
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Compile-check tagged tiers" "go vet -tags <integration|blackbox> ./..."
    for tags in integration blackbox; do
        printf 'vet -tags %s\n' "$tags"
        go vet -tags "$tags" ./...
    done

# Replays the seeds committed under testdata/fuzz. Deterministic and quick,
# and by design it finds nothing new: it keeps past crashers fixed.
# Replay committed fuzz seed corpora
[doc('Replay fuzz seeds')]
[group('test')]
test-fuzz-native:
    @bash scripts/testing/environment.sh native --suite "fuzz seeds (race)"
    @"{{ just_executable() }}" _header "Replay fuzz seeds" "go test -count=1 -race -run='^Fuzz'"
    GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -count=1 -race -buildvcs=false -run='^Fuzz' $(GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go list './...' | grep -v generated)

# The opposite of test-fuzz-native: every target is fuzzed for `time` hunting inputs
# nobody has tried, so a run is nondeterministic and a new crasher is a find.
# Time accepts positive integer ms/s/m up to 60s per target, leaving room in
# the 2m harness timeout for startup and minimization. The container's default
# wall limit adds the requested campaign time to a 15m preparation margin;
# GOMMITLINT_EXTERNAL_RUN_TIMEOUT can set an explicit total limit.
# Fuzz targets on new inputs (up to 60s each); container, network preparation
[arg('time', pattern='[1-9][0-9]*(ms|s|m)', help='Positive duration up to 60s per target, for example 14s or 500ms')]
[doc('Fuzz inputs; network prep')]
[group('test')]
test-fuzz-container time="14s":
    @bash scripts/testing/environment.sh container --suite "generated fuzz campaign" -- bash .forgejo/scripts/run-external-tests-local.sh fuzz "$1"

# Drives the built binary the way a user would, over the whole black-box
# contract. It fails rather than skips when a tool a row needs is missing; the
# sandbox image carries them all, so nothing is optional.
# Test the shipped binary; container, network preparation
[doc('Test CLI; network prep')]
[group('test')]
test-blackbox-container:
    @bash scripts/testing/environment.sh container --suite "black-box" -- bash .forgejo/scripts/run-external-tests-local.sh blackbox

# Integration and black-box together in one sandbox, for one image load rather
# than two; the usual pre-pull-request run when a change touches the CLI.
# Run both sandbox tiers; container, network preparation
[doc('Integration/blackbox; network')]
[group('test')]
test-combined-container:
    @bash scripts/testing/environment.sh container --suite "integration + black-box" -- bash .forgejo/scripts/run-external-tests-local.sh source

# The distribution matrix: the integration and black-box suites again, once in
# each distribution of test/distribution/images, against that distribution's
# own packaged git, GnuPG and OpenSSH -- the tools gommitlint shells out to or
# is tested with, whose behaviour moves between releases. Same launcher, same
# guarantees: a checksummed snapshot, the product and the harnesses built from
# it outside the sandbox, CI markers refused, and every run --network=none in
# rootless Podman. Each distribution image is recorded beforehand by
# `just test-container-prepare-distributions`; a missing or stale one is refused with
# that recipe named. A failing distribution does not stop the others, and the
# summary names each result. Local only, and not part of `just verify`.
#
#   just test-combined-container-matrix
#   just test-combined-container-matrix fedora-44 arch
#
# Run both sandbox tiers in every distribution; container, network preparation
[arg('names', pattern=test_distributions, help='Prepared distribution names; omit for all. Valid arguments are listed in the pattern')]
[doc('Integration/blackbox; network')]
[group('test')]
test-combined-container-matrix *names:
    @bash scripts/testing/environment.sh container --suite "integration + black-box (distribution matrix)" -- bash .forgejo/scripts/run-external-tests-local.sh distribution "$@"

[arg('target', pattern=vm_target_argument, help='Required to run; omit for help. Prepared guest OS/architecture, for example freebsd-amd64. Choices are listed in the pattern.')]
[doc('Run VM unit tests')]
[group('test')]
test-unit-vm target="": (_require-vm-target target "test-unit-vm")
    @bash scripts/testing/vm.sh run "$1" unit

[arg('target', pattern=vm_target_argument, help='Required to run; omit for help. Prepared guest with Go race support, for example macos-arm64. Choices are listed in the pattern.')]
[doc('Run VM race tests')]
[group('test')]
test-race-vm target="": (_require-vm-target target "test-race-vm")
    @bash scripts/testing/vm.sh run "$1" race

[arg('target', pattern=vm_target_argument, help='Required to run; omit for help. Prepared guest OS/architecture, for example freebsd-amd64. Choices are listed in the pattern.')]
[doc('Run VM integration tests')]
[group('test')]
test-integration-vm target="": (_require-vm-target target "test-integration-vm")
    @bash scripts/testing/vm.sh run "$1" integration

[arg('target', pattern=vm_target_argument, help='Required to run; omit for help. Prepared guest OS/architecture, for example freebsd-amd64. Choices are listed in the pattern.')]
[doc('Run VM CLI tests')]
[group('test')]
test-blackbox-vm target="": (_require-vm-target target "test-blackbox-vm")
    @bash scripts/testing/vm.sh run "$1" blackbox

[arg('target', pattern=vm_target_argument, help='Required to run; omit for help. Prepared guest OS/architecture, for example freebsd-amd64. Choices are listed in the pattern.')]
[doc('Integration + blackbox')]
[group('test')]
test-combined-vm target="": (_require-vm-target target "test-combined-vm")
    @bash scripts/testing/vm.sh run "$1" combined

[arg('args', help='Arguments passed to go test, for example -v or -run TestName')]
[doc('Linux units; Apple Silicon')]
[group('test')]
test-unit-container-on-macos *args:
    @bash scripts/testing/vm.sh run linux-arm64 container-unit "$@"

[doc('Linux CLI; Apple Silicon')]
[group('test')]
test-blackbox-container-on-macos:
    @bash scripts/testing/vm.sh run linux-arm64 container-blackbox

[doc('Linux combined; Apple Silicon')]
[group('test')]
test-combined-container-on-macos:
    @bash scripts/testing/vm.sh run linux-arm64 container-combined

[arg('names', pattern=test_distributions, help='Prepared distribution names; omit for the VM profile selection')]
[doc('Linux matrix; Apple Silicon')]
[group('test')]
test-matrix-container-on-macos *names:
    @bash scripts/testing/vm.sh run linux-arm64 container-matrix "$@"

# The unit coverage profile plus an HTML rendering of it under bin/, for
# reading in a browser. It is a report, not a gate: nothing fails on a number.
# Write HTML coverage
[group('test')]
test-coverage-native:
    @bash scripts/testing/environment.sh native --suite "unit + fuzz seeds (coverage, race)"
    @mkdir -p {{ bin }}
    @"{{ just_executable() }}" _header "Run unit tests with coverage" "go test -coverprofile"
    go test -v -count=1 -race -buildvcs=false -coverprofile={{ bin }}/coverage-unit.out $(go list './...' | grep -v generated)
    @"{{ just_executable() }}" _header "Generate HTML report"
    @"{{ just_executable() }}" _run_with_output "Coverage report generation" go tool cover -html {{ quote(bin + "/coverage-unit.out") }} -o {{ quote(bin + "/coverage.html") }}

# ==================================================================================== #
# LINT - Code quality and repository policy
# ==================================================================================== #
#
# All leaf lint/security recipes forward extra positional arguments to
# nanolinter. Use this to disable a check on a one-off run, narrow scope,
# or pass `--fix`:
#
#   just lint --disable-check sast
#   just lint-commits --base-branch develop
#   just lint-markdown --fix
#
# Aggregate recipes (lint-go-fix, lint-fix, verify) do not forward args —
# their sub-recipes accept incompatible flag sets. (lint-go is a single
# nanolinter call now and does forward args.)

# ▪ Run the full lint suite
[arg('args', help=tool_args_help)]
[group('lint')]
lint *args:
    @"{{ just_executable() }}" --justfile "{{ justfile() }}" --fmt --check
    @bash .forgejo/scripts/check-workflow-policy.sh
    @{{ quote(nanolinter_cmd) }} verify "$@"

# Run the Go lint suite
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go *args:
    @{{ quote(nanolinter_cmd) }} lint go "$@"

# Run go vet
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go-vet *args:
    @{{ quote(nanolinter_cmd) }} lint go-vet "$@"

# Run staticcheck
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go-staticcheck *args:
    @{{ quote(nanolinter_cmd) }} lint go-staticcheck "$@"

# Run golangci-lint
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go-golangci *args:
    @{{ quote(nanolinter_cmd) }} lint go-golangci "$@"

# Check Go whitespace
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go-wsl *args:
    @{{ quote(nanolinter_cmd) }} lint go-wsl "$@"

# Check Go formatting
[arg('args', help=tool_args_help)]
[group('lint')]
lint-go-format *args:
    @{{ quote(nanolinter_cmd) }} lint go-format "$@"

# Check CI workflows
[arg('args', help=tool_args_help)]
[group('lint')]
lint-actions *args:
    @{{ quote(nanolinter_cmd) }} lint actions "$@"

# Check commit messages
[arg('args', help=tool_args_help)]
[group('lint')]
lint-commits *args:
    @{{ quote(nanolinter_cmd) }} lint commits "$@"

# Check container definitions
[arg('args', help=tool_args_help)]
[group('lint')]
lint-container *args:
    @{{ quote(nanolinter_cmd) }} lint container "$@"

# Check REUSE compliance
[arg('args', help=tool_args_help)]
[group('lint')]
lint-license *args:
    @{{ quote(nanolinter_cmd) }} lint license "$@"

# The blocking link gate: `just lint` reports broken links without failing
# (`[policy] warn` in nanolinter.toml), this recipe fails on them. Run it
# before a release.
# Check links; network
[arg('args', help=tool_args_help)]
[group('lint')]
lint-links *args:
    @{{ quote(nanolinter_cmd) }} lint links "$@"

# Check Markdown files
[arg('args', help=tool_args_help)]
[group('lint')]
lint-markdown *args:
    @{{ quote(nanolinter_cmd) }} lint markdown "$@"

# Check shell scripts
[arg('args', help=tool_args_help)]
[group('lint')]
lint-shell *args:
    @{{ quote(nanolinter_cmd) }} lint shell "$@"

# Check shell formatting
[arg('args', help=tool_args_help)]
[group('lint')]
lint-shell-format *args:
    @{{ quote(nanolinter_cmd) }} lint shell-format "$@"

# Check TOML files
[arg('args', help=tool_args_help)]
[group('lint')]
lint-toml *args:
    @{{ quote(nanolinter_cmd) }} lint toml "$@"

# Check spelling
[arg('args', help=tool_args_help)]
[group('lint')]
lint-typos *args:
    @{{ quote(nanolinter_cmd) }} lint typos "$@"

# Check XML files
[arg('args', help=tool_args_help)]
[group('lint')]
lint-xml *args:
    @{{ quote(nanolinter_cmd) }} lint xml "$@"

# Check YAML files
[arg('args', help=tool_args_help)]
[group('lint')]
lint-yaml *args:
    @{{ quote(nanolinter_cmd) }} lint yaml "$@"

# Alpine package pins name the release they came from, and Renovate bumps the
# base image on its own: the two must not drift apart.
[doc('Check Alpine image pins')]
[group('lint')]
check-alpine-pins:
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Check Alpine pin coupling" "BASE_IMAGE release == depName=alpine_X_Y/*"
    file=Containerfile.alpine
    base=$(sed -nE 's/^ARG BASE_IMAGE=alpine:([0-9]+)\.([0-9]+)@sha256:[0-9a-f]{64}$/alpine_\1_\2/p' "$file")
    if [ -z "$base" ]; then
        printf "{{ RED }}✗ $file has no digest-pinned 'ARG BASE_IMAGE=alpine:X.Y@sha256:...' to read the release from{{ NC }}\n" >&2
        exit 1
    fi
    mismatched=$(grep -oE 'depName=alpine_[0-9]+_[0-9]+/[^ ]+' "$file" | grep -v "^depName=${base}/" || true)
    if [ -n "$mismatched" ]; then
        printf "{{ RED }}✗ $file pins packages from another Alpine release than its base image{{ NC }}\n" >&2
        printf "  base image is %s, but these annotations disagree:\n" "$base" >&2
        printf "    %s\n" $mismatched >&2
        printf "  Renovate cannot derive one from the other, so a base-image bump leaves\n" >&2
        printf "  these behind and the mismatch first appears in a release build.\n" >&2
        exit 1
    fi
    annotated=$(grep -cE 'depName=alpine_[0-9]+_[0-9]+/' "$file" || true)
    if [ "$annotated" -eq 0 ]; then
        printf "{{ RED }}✗ $file pins no Alpine packages; this check is asserting nothing{{ NC }}\n" >&2
        exit 1
    fi
    printf "{{ GREEN }}✓ %s package pins all name %s{{ NC }}\n" "$annotated" "$base"

# ==================================================================================== #
# LINT-FIX - Auto-fix linting violations
# ==================================================================================== #

# ▪ Apply supported autofixes
[group('lint-fix')]
lint-fix: lint-go-fix lint-shell-fix lint-shell-format-fix lint-markdown-fix lint-yaml-fix
    @printf "\033[0;32m✓ All auto-fixes completed\033[0m\n"

# Format Go and tidy go.mod
[group('lint-fix')]
tidy:
    @"{{ just_executable() }}" _header "Format Go code"
    @"{{ just_executable() }}" _run_with_output "Go fmt" go fmt ./...
    @"{{ just_executable() }}" _header "Clean dependencies verbose"
    @"{{ just_executable() }}" _run_with_output "Go mod tidy" go mod tidy -v

# Fix Go with tidy, gofmt, golangci and wsl
[doc('Tidy, format and fix Go')]
[group('lint-fix')]
lint-go-fix: tidy
    @{{ quote(nanolinter_cmd) }} lint go-format --fix
    @{{ quote(nanolinter_cmd) }} lint go-golangci --fix
    @{{ quote(nanolinter_cmd) }} lint go-wsl --fix

# Fix Go formatting
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-go-format-fix *args:
    @{{ quote(nanolinter_cmd) }} lint go-format --fix "$@"

# Fix golangci-lint findings
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-go-golangci-fix *args:
    @{{ quote(nanolinter_cmd) }} lint go-golangci --fix "$@"

# Fix Go whitespace
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-go-wsl-fix *args:
    @{{ quote(nanolinter_cmd) }} lint go-wsl --fix "$@"

# Fix Markdown files
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-markdown-fix *args:
    @{{ quote(nanolinter_cmd) }} lint markdown --fix "$@"

# Fix shell scripts
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-shell-fix *args:
    @{{ quote(nanolinter_cmd) }} lint shell --fix "$@"

# Fix shell formatting
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-shell-format-fix *args:
    @{{ quote(nanolinter_cmd) }} lint shell-format --fix "$@"

# Fix YAML files
[arg('args', help=tool_args_help)]
[group('lint-fix')]
lint-yaml-fix *args:
    @{{ quote(nanolinter_cmd) }} lint yaml --fix "$@"

# ==================================================================================== #
# SECURITY - Vulnerability and secret scanning
# ==================================================================================== #

# ▪ Run all scans; container, credentials
[doc('▪ Scan; container, credentials')]
[group('security')]
security: security-secrets security-sast security-iac security-govulncheck security-osv security-trivy-fs security-container security-scorecard
    @printf "{{ GREEN }}✓ Security scans completed{{ NC }}\n"

# Scan for committed secrets
[arg('args', help=tool_args_help)]
[group('security')]
security-secrets *args:
    @{{ quote(nanolinter_cmd) }} lint secrets "$@"

# Scan source code (SAST); network
[arg('args', help=tool_args_help)]
[group('security')]
security-sast *args:
    @{{ quote(nanolinter_cmd) }} lint sast "$@"

# Scan infrastructure as code
[arg('args', help=tool_args_help)]
[group('security')]
security-iac *args:
    @{{ quote(nanolinter_cmd) }} lint iac "$@"

# Scan Go vulnerabilities; network
[arg('args', help=tool_args_help)]
[group('security')]
security-govulncheck *args:
    @{{ quote(nanolinter_cmd) }} lint go-vulncheck "$@"

# Scan dependencies (OSV); network
[arg('args', help=tool_args_help)]
[group('security')]
security-osv *args:
    @{{ quote(nanolinter_cmd) }} lint osv "$@"

# Scan files with Trivy; network
[arg('args', help=tool_args_help)]
[group('security')]
security-trivy-fs *args:
    @{{ quote(nanolinter_cmd) }} lint trivy-fs "$@"

# Scans the archive that build-image exports with Trivy and removes it after.
# Fails on fixable HIGH/CRITICAL vulnerabilities, secrets and image-config
# misconfigurations; accepted findings live in .trivyignore.yaml.
[arg('args', help='Extra Trivy image-scan options; this recipe builds the image first')]
[doc('Build/scan image; network')]
[group('security')]
security-container *args: (_require-tool "trivy" "Install trivy with mise") build-image
    #!/usr/bin/env bash
    set -euo pipefail
    archive={{ quote(image_archive) }}
    trap 'rm -f -- "$archive"' EXIT
    trivy image --input "$archive" \
        --scanners vuln,secret --image-config-scanners misconfig,secret \
        --severity HIGH,CRITICAL --ignore-unfixed \
        --ignorefile .trivyignore.yaml --exit-code 1 "$@"

# Run OpenSSF Scorecard; credentials
[arg('args', help=tool_args_help)]
[doc('Run Scorecard; credentials')]
[group('security')]
security-scorecard *args:
    @{{ quote(nanolinter_cmd) }} security scorecard "$@"

# The authoritative validator is forge-ci's check-l3-isolation action, which
# invokes its checksum-verified engine. There is no supported local
# shell entry point in this repository.
[doc('Local L3 check unavailable')]
[group('security')]
verify-l3-isolation:
    @printf '%s\n' "ERROR: local L3 validation is unavailable; the deleted shell validator has no safe local replacement." "Use the pinned forge-ci check-l3-isolation action in the pull-request workflow." >&2; exit 1

# ==================================================================================== #
# BUILD - Compilation and images
# ==================================================================================== #

# ▪ Build the host binary
[group('build')]
build: (_job "build" ("target=host; output=" + bin))
    #!/usr/bin/env bash
    set -euo pipefail
    GOOS=$(go env GOHOSTOS)
    GOARCH=$("{{ just_executable() }}" _host-goarch)
    printf 'Build target: %s/%s | output=%s\n' "$GOOS" "$GOARCH" "{{ bin }}/{{ executable }}-$GOOS-$GOARCH" >&2
    if "{{ just_executable() }}" _build_binary "$GOOS" "$GOARCH" >/dev/null 2>&1; then
        printf "{{ GREEN }}✓{{ NC }} Host binary built successfully (%s)\n" "$GOARCH"
    else
        printf "{{ RED }}✗{{ NC }} Host binary build failed (%s)\n" "$GOARCH"
        "{{ just_executable() }}" _build_binary "$GOOS" "$GOARCH"
        exit 1
    fi

# Run verify, the binary matrix and multi-arch image build; container, network
[doc('Verify/build; container, network')]
[group('build')]
build-full: (_job "build-full" "steps=verify + binary matrix + linux/amd64,linux/arm64 container images; engine=podman; network=allowed") (_require-tool "podman" "Install podman with your package manager") verify build-all build-image-multi

# Build every released binary
[group('build')]
build-all: (_job "build-all" ("targets=all configured binary targets (reported per build); output=" + bin)) build-linux-amd64 build-linux-arm64 build-linux-riscv64 build-darwin-arm64 build-freebsd-amd64 build-freebsd-arm64 build-netbsd-amd64 build-netbsd-arm64 build-openbsd-amd64 build-openbsd-arm64 build-openbsd-riscv64

# Build the linux/amd64 binary
[group('build')]
build-linux-amd64:
    @"{{ just_executable() }}" _job "build-linux-amd64" {{ quote("target=linux/amd64; output=" + bin + "/" + executable + "-linux-amd64") }}
    @"{{ just_executable() }}" _run_with_output "AMD64 binary built successfully" "{{ just_executable() }}" _build_binary linux amd64

# Build the linux/arm64 binary
[group('build')]
build-linux-arm64:
    @"{{ just_executable() }}" _job "build-linux-arm64" {{ quote("target=linux/arm64; output=" + bin + "/" + executable + "-linux-arm64") }}
    @"{{ just_executable() }}" _run_with_output "ARM64 binary built successfully" "{{ just_executable() }}" _build_binary linux arm64

# Build the linux/riscv64 binary
[group('build')]
build-linux-riscv64:
    @"{{ just_executable() }}" _build_binary linux riscv64

# Build the darwin/arm64 binary
[group('build')]
build-darwin-arm64:
    @"{{ just_executable() }}" _build_binary darwin arm64

# Build the freebsd/amd64 binary
[group('build')]
build-freebsd-amd64:
    @"{{ just_executable() }}" _build_binary freebsd amd64

# Build the freebsd/arm64 binary
[group('build')]
build-freebsd-arm64:
    @"{{ just_executable() }}" _build_binary freebsd arm64

# Build the netbsd/amd64 binary
[group('build')]
build-netbsd-amd64:
    @"{{ just_executable() }}" _build_binary netbsd amd64

# Build the netbsd/arm64 binary
[group('build')]
build-netbsd-arm64:
    @"{{ just_executable() }}" _build_binary netbsd arm64

# Build the openbsd/amd64 binary
[group('build')]
build-openbsd-amd64:
    @"{{ just_executable() }}" _build_binary openbsd amd64

# Build the openbsd/arm64 binary
[group('build')]
build-openbsd-arm64:
    @"{{ just_executable() }}" _build_binary openbsd arm64

# Build the openbsd/riscv64 binary
[group('build')]
build-openbsd-riscv64:
    @"{{ just_executable() }}" _build_binary openbsd riscv64

# Build GoReleaser snapshots twice and compare top-level .tar.gz, .deb, .rpm
# and .apk files. This is same-environment repeatability of those artifacts.
#
# This used to build one host binary twice through
# scripts/build/build-cross-platform.sh, which verifies something other than
# what ships: it sets GOAMD64=v2 where the release builds the default v1, its
# ldflags lack the `all=` prefix the release uses, and it takes the version
# from `git describe` rather than the tag. It also never touched an archive,
# a package or a checksum byte.
#
# This is the argument-free check `verify-full` runs. For parity with a
# published release, use `just verify-rebuild <version>` on the tag instead;
# that is the stronger result, because it also covers the release
# environment. See docs/modules/ROOT/pages/reproducible-builds.adoc.
#
# SBOMs are skipped: syft stamps every document with a fresh serial number and
# timestamp, so they can never match and would drown out real differences.
# Signing and publishing are skipped because Release CI owns both.
# Compares snapshot archives and packages across two builds.
[doc('Compare archive/package builds')]
[group('build')]
verify-reproducible: (_job "verify-reproducible" "steps=generate + two snapshot builds + archive/package comparison; publication=none") (_require-tool "goreleaser" "Install goreleaser with mise") generate
    #!/usr/bin/env bash
    set -euo pipefail
    first="{{ bin }}/.repro-first"
    second="{{ bin }}/.repro-second"
    rm -rf -- "$first" "$second"
    "{{ just_executable() }}" _header "Reproducible release verification" \
        "goreleaser snapshot twice; compare .tar.gz/.deb/.rpm/.apk"
    goreleaser release --clean --snapshot --skip=publish,sign,sbom
    mv -- {{ dist }} "$first"
    goreleaser release --clean --snapshot --skip=publish,sign,sbom
    mv -- {{ dist }} "$second"
    first_files=()
    while IFS= read -r artifact; do [[ -z "$artifact" ]] || first_files+=("$artifact"); done < <(cd "$first" && shopt -s nullglob && printf '%s\n' *.tar.gz *.deb *.rpm *.apk | sort)
    second_files=()
    while IFS= read -r artifact; do [[ -z "$artifact" ]] || second_files+=("$artifact"); done < <(cd "$second" && shopt -s nullglob && printf '%s\n' *.tar.gz *.deb *.rpm *.apk | sort)
    if [[ "${#first_files[@]}" -eq 0 ]]; then
        printf "{{ RED }}✗ the snapshot produced no comparable files{{ NC }}\n" >&2
        exit 1
    fi
    status=0
    if [[ "${first_files[*]}" != "${second_files[*]}" ]]; then
        printf "{{ RED }}✗ the two builds produced different file sets{{ NC }}\n" >&2
        status=1
    fi
    for artifact in "${first_files[@]}"; do
        if [[ ! -f "$second/$artifact" ]]; then
            printf "{{ RED }}✗ missing from the second build: %s{{ NC }}\n" "$artifact" >&2
            status=1
        elif ! cmp -s -- "$first/$artifact" "$second/$artifact"; then
            printf "{{ RED }}✗ differs between builds: %s{{ NC }}\n" "$artifact" >&2
            status=1
        fi
    done
    if [[ "$status" -ne 0 ]]; then
        printf "  Preserved for diffoscope: %s and %s\n" "$first" "$second" >&2
        exit 1
    fi
    rm -rf -- "$first" "$second"
    printf "{{ GREEN }}✓ %s release files identical across two builds{{ NC }}\n" "${#first_files[@]}"

# Build a host image; container, network
[doc('Build host image; network')]
[group('build')]
build-image: (_job "build-image" ("engine=podman; target=linux/host-architecture; image=" + executable + ":dev; archive=" + image_archive + "; network=allowed")) (_require-tool "podman" "Install podman with your package manager") _build-linux-host
    #!/usr/bin/env bash
    set -euo pipefail
    GOARCH=$("{{ just_executable() }}" _host-goarch)
    stage=$(mktemp -d "{{ bin }}/.image.XXXXXXXX")
    trap 'rm -f -- "$stage/{{ executable }}"; rmdir -- "$stage"' EXIT
    cp "{{ bin }}/{{ executable }}-linux-$GOARCH" "$stage/{{ executable }}"
    podman build --platform="linux/$GOARCH" \
        --timestamp "$(git log -1 --format=%ct)" \
        --build-arg "DIRPATH=${stage#./}/" \
        -t {{ executable }}:dev -f Containerfile .
    podman save --format docker-archive -o {{ quote(image_archive) }} {{ executable }}:dev

# SOURCE_DATE_EPOCH pinned to the latest commit so layer mtimes and the OCI
# `created` field are derived from git state, not wall-clock. Same pattern
# as the release workflow — see docs/modules/ROOT/pages/reproducible-builds.adoc.
[doc('Build multi-arch image; network')]
[group('build')]
build-image-multi: (_job "build-image-multi" "engine=podman; targets=linux/amd64,linux/arm64; manifest=gommitlint:dev; network=allowed") (_require-tool "podman" "Install podman with your package manager") build-all
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Multi-arch container" "podman buildx build"

    SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)
    stage=$(mktemp -d "{{ bin }}/.image.XXXXXXXX")
    trap 'rm -f -- "$stage/{{ executable }}"; rmdir -- "$stage"' EXIT

    # Create manifest list
    podman manifest rm gommitlint:dev 2>/dev/null || true
    podman rmi gommitlint:dev 2>/dev/null || true
    podman manifest create gommitlint:dev

    # Build and add AMD64 image to manifest
    printf "Building AMD64 image...\n"
    cp "{{ bin }}/{{ executable }}-linux-amd64" "$stage/{{ executable }}"
    podman buildx build --platform=linux/amd64 --timestamp="${SOURCE_DATE_EPOCH}" --manifest=gommitlint:dev --build-arg "DIRPATH=${stage#./}/" -f Containerfile .

    # Build and add ARM64 image to manifest
    printf "Building ARM64 image...\n"
    cp "{{ bin }}/{{ executable }}-linux-arm64" "$stage/{{ executable }}"
    podman buildx build --platform=linux/arm64 --timestamp="${SOURCE_DATE_EPOCH}" --manifest=gommitlint:dev --build-arg "DIRPATH=${stage#./}/" -f Containerfile .

    printf "\033[0;32m✓ Multi-arch container manifest created\033[0m\n"

# ==================================================================================== #
# DOCUMENTATION - Documentation and code generation
# ==================================================================================== #

# ▪ Generate docs and release assets
[doc('▪ Generate docs/release assets')]
[group('documentation')]
generate: generate-manpage generate-completion generate-translations generate-schema generate-packaging

# Generate compressed manpages
[group('documentation')]
generate-manpage:
    @"{{ just_executable() }}" _header "Generate man page" "./scripts/docs/manpage.sh"
    ./scripts/docs/manpage.sh

# Generate shell completions
[group('documentation')]
generate-completion: build
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Generate shell completions" "./scripts/docs/completions.sh"
    stage=$(mktemp -d)
    trap 'rm -rf -- "$stage"' EXIT
    cp -- "{{ bin }}/{{ executable }}-$(go env GOHOSTOS)-$("{{ just_executable() }}" _host-goarch)" "$stage/{{ executable }}"
    BINARY_PATH="$stage/{{ executable }}" ./scripts/docs/completions.sh

# Compile translation catalogs
[group('documentation')]
generate-translations:
    @"{{ just_executable() }}" _header "Generate translations" "gommitlint translations compile"
    @go run ./cmd/gommitlint translations compile

# Regenerate schemas/gommitlint.schema.json from internal/domain/settings.
# Run after touching settings types or their doc comments; CI's check-schema
# enforces drift-free output.
[doc('Generate the config JSON Schema')]
[group('documentation')]
generate-schema:
    @"{{ just_executable() }}" _header "Generate config JSON Schema" "go run ./internal/tools/schemagen"
    @go run ./internal/tools/schemagen

# nFPM changelogs (packaging/nfpm/changelog.yaml) are hand-edited and describe
# PACKAGING changes, not software changes — software CHANGELOG.md is separate.
[doc('Generate Debian metadata')]
[group('documentation')]
generate-packaging: generate-debian-copyright generate-changelog-gz

# Generate Debian copyright from LICENSES/
[doc('Generate Debian copyright')]
[group('documentation')]
generate-debian-copyright:
    @"{{ just_executable() }}" _header "Generate Debian copyright" "./scripts/packaging/generate-debian-copyright.sh"
    ./scripts/packaging/generate-debian-copyright.sh

# Required by Debian Policy §12.7.
# -n: skip name and mtime in the gzip header so local and CI builds produce
# the same bytes. Without it gzip embeds the source file's mtime and any
# repo touch (git pull, file regen) would change changelog.gz.
[doc('Generate compressed changelog')]
[group('documentation')]
generate-changelog-gz:
    @"{{ just_executable() }}" _header "Generate compressed changelog" "gzip -9 -n CHANGELOG.md → generated/changelog.gz"
    @mkdir -p generated
    @gzip -9 -n -c CHANGELOG.md > generated/changelog.gz

# Refresh translation catalogs
[group('documentation')]
update-translations:
    @"{{ just_executable() }}" _header "Update translation catalogs" "gommitlint translations update"
    @go run ./cmd/gommitlint translations update

# Check the config schema for drift
[group('documentation')]
check-schema:
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Check config schema drift" "go run ./internal/tools/schemagen → diff"
    tmp=$(mktemp)
    trap 'rm -f "$tmp"' EXIT
    go run ./internal/tools/schemagen "$tmp"
    if ! diff -u schemas/gommitlint.schema.json "$tmp"; then
        printf "{{ RED }}✗ schemas/gommitlint.schema.json is stale — run 'just generate-schema'{{ NC }}\n" >&2
        exit 1
    fi
    printf "{{ GREEN }}✓ Config schema is up to date{{ NC }}\n"

# Drift gate; ~60s, run on demand.
[doc('Check translation catalogs')]
[group('documentation')]
check-translations:
    @"{{ just_executable() }}" _header "Check translation drift" "./scripts/lint/translation-drift.sh"
    ./scripts/lint/translation-drift.sh

# Check gettext catalog syntax
[group('documentation')]
check-gettext-catalogs:
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Check gettext catalogs" "msgfmt -c locale/gommitlint.pot locale/*/gommitlint.po"
    for catalog in locale/gommitlint.pot locale/*/gommitlint.po; do
        [[ -f "$catalog" ]] || continue
        msgfmt -c -o /dev/null "$catalog"
    done
    printf "{{ GREEN }}✓ Gettext catalogs are syntactically valid{{ NC }}\n"

# Check every declared manpage locale has a source.
[doc('Check declared manpage locales')]
[group('documentation')]
check-localized-manpages:
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Check localized manpages" "docs/manpages/locale/<lang>/gommitlint.1.md"
    locales_dir="docs/manpages/locale"
    if [[ ! -d "$locales_dir" ]]; then
        printf "{{ GREEN }}✓ No localized manpages declared{{ NC }}\n"
        exit 0
    fi
    failed=0
    for locale_dir in "$locales_dir"/*; do
        [[ -d "$locale_dir" ]] || continue
        locale=$(basename "$locale_dir")
        src="$locale_dir/gommitlint.1.md"
        if [[ ! -f "$src" ]]; then
            printf "{{ RED }}✗{{ NC }} %s: missing %s\n" "$locale" "$src" >&2
            failed=1
        else
            printf "{{ GREEN }}✓{{ NC }} %s: %s\n" "$locale" "$src"
        fi
    done
    [[ $failed -eq 0 ]] || exit 1

# Use in a shell rc: `eval "$(just completion zsh)"`.
[arg('shell', pattern='bash|zsh|fish', help='Valid arguments: bash, zsh, fish')]
[doc('Print shell completions')]
[group('documentation')]
completion shell="bash":
    @go run ./cmd/gommitlint completion "$1" --stdout

# ==================================================================================== #
# INSTALL - Install built artefacts onto the host
# ==================================================================================== #

# ▪ Install binary and user docs
[group('install')]
install-local: build
    @"{{ just_executable() }}" _header "Install all components locally" "./scripts/install/install-local-all.sh {{ bin }} {{ executable }}"
    BINARY_PATH="{{ bin }}/{{ executable }}-$(go env GOHOSTOS)-$("{{ just_executable() }}" _host-goarch)" OS="$(go env GOHOSTOS)" ./scripts/install/install-local-all.sh {{ bin }} {{ executable }}

# Install the binary for the user
[group('install')]
install-local-binary: build
    @"{{ just_executable() }}" _header "Install binary locally" "./scripts/install/install-binary.sh {{ bin }} {{ executable }}"
    OS="$(go env GOHOSTOS)" ./scripts/install/install-binary.sh {{ bin }} {{ executable }}

# Install the manpages for the user
[group('install')]
install-local-man: generate-manpage
    @"{{ just_executable() }}" _header "Local man page installation" "cp to ~/.local/share/man/man1/"
    # Check if ~/.local/share/man/man1 directory exists
    if [ ! -d ~/.local/share/man/man1 ]; then \
        printf "\033[1;31m✗ Directory ~/.local/share/man/man1 does not exist\033[0m\n"; \
        printf "  Please create it first: mkdir -p ~/.local/share/man/man1\n"; \
        printf "  Or ensure your system follows XDG Base Directory specification\n"; \
        exit 1; \
    fi
    cp generated/manpages/{{ executable }}.1.gz ~/.local/share/man/man1/
    printf "\033[0;32m✓ Installed man page to ~/.local/share/man/man1/\033[0m\n"
    printf "  Run 'man {{ executable }}' to view\n"

# Install the completions for the user
[doc('Install user completions')]
[group('install')]
install-local-completion: install-local-binary
    @"{{ just_executable() }}" _header "Install shell completions" "{{ executable }} completion --all"
    "${INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}/{{ executable }}" completion --all

# ==================================================================================== #
# PACKAGING - Package building (nFPM convenience + native distro)
# ==================================================================================== #

# ▪ Build nFPM packages
[group('packaging')]
package-nfpm: release-check build generate
    @"{{ just_executable() }}" _header "Build nFPM packages (deb/rpm/apk)" "goreleaser release --snapshot --clean --skip publish,announce,validate,sign"
    goreleaser release --snapshot --clean --skip publish,announce,validate,sign

# Check nFPM packages
[group('packaging')]
package-nfpm-lint: (_require-tool "lintian" "Install lintian") (_require-tool "rpmlint" "Install rpmlint")
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    debs=({{ dist }}/*.deb)
    rpms=({{ dist }}/*.rpm)
    ((${#debs[@]} > 0)) || { printf "No Debian packages found\n" >&2; exit 1; }
    ((${#rpms[@]} > 0)) || { printf "No RPM packages found\n" >&2; exit 1; }
    "{{ just_executable() }}" _header "Lint Debian packages" "lintian -v -i -I --pedantic dist/*.deb"
    lintian -v -i -I --pedantic "${debs[@]}"
    "{{ just_executable() }}" _header "Lint RPM packages" "rpmlint dist/*.rpm"
    rpmlint "${rpms[@]}"

# Alpine APKs are signed by abuild inside the container regardless; .deb and
# .rpm outputs are produced without a signing step. The "signed" qualifier
# was misleading and the `-unsigned` variant was identical, so it's gone.
# Native source archives come from committed HEAD; packaging helpers and
# version pins are read from the working tree.
[doc('Build HEAD; container, network')]
[group('packaging')]
package-native-all: package-native-debian package-native-rpm package-native-alpine
    @printf "{{ GREEN }}✓ All native packages built in dist/native/{{ NC }}\n"

# Built via podman for distro-repo compatibility.
[doc('Build HEAD; container, network')]
[group('packaging')]
package-native-debian: (_require-tool "podman" "Install: sudo apt install podman")
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Build native Debian package" "podman + dpkg-buildpackage"

    VERSION="{{ package_version }}"
    GO_VERSION=$(grep '^go ' go.mod | awk '{print $2}')

    # Prepare build directory
    BUILD_DIR=$(mktemp -d)
    BUILD_CONTAINER="gommitlint-package-$$"
    cleanup_build_dir() {
        local status=$?
        trap - EXIT
        podman rm --force --time 0 --ignore "$BUILD_CONTAINER" >/dev/null 2>&1 || true
        podman unshare rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        exit "$status"
    }
    trap cleanup_build_dir EXIT

    # Create source tarball
    printf "\033[1;34m▸ Creating source tarball...\033[0m\n"
    git archive --format=tar.gz --prefix=gommitlint/ -o "${BUILD_DIR}/gommitlint-${VERSION}.tar.gz" HEAD

    # renovate: datasource=docker depName=debian
    DEBIAN_IMAGE="debian:trixie@sha256:f324c7ff54321e8d9c588493a20244965938ce0aa50bbd1022d38010e9ffc4b1"

    # Build inside Debian container
    printf "\033[1;34m▸ Building Debian package in container (Go %s)...\033[0m\n" "${GO_VERSION}"
    # Ctrl-C must stop the build. The build script is PID 1 in the container
    # and ignores the interrupt, and the shell runs its exit trap only between
    # commands, so the run is in the background and cleanup removes the container.
    podman run --rm --name "${BUILD_CONTAINER}" \
        -v "${BUILD_DIR}:/work/pkg" \
        -v "{{ justfile_directory() }}/scripts/packaging/install-go.sh:/work/install-go.sh:ro" \
        -v "{{ justfile_directory() }}/scripts/packaging/debian-build.sh:/work/debian-build.sh:ro" \
        -e "PKG_DIR=/work/pkg" \
        -e "OUT_DIR=/work/pkg/output" \
        -e "VERSION=${VERSION}" \
        -e "GO_VERSION=${GO_VERSION}" \
        "${DEBIAN_IMAGE}" \
        bash /work/debian-build.sh &
    wait "$!"

    # Move artifacts
    shopt -s nullglob
    debs=("$BUILD_DIR"/output/*.deb)
    changes=("$BUILD_DIR"/output/*.changes)
    buildinfo=("$BUILD_DIR"/output/*.buildinfo)
    ((${#debs[@]} > 0)) || { printf "No native Debian package was produced\n" >&2; exit 1; }
    ((${#changes[@]} > 0)) || { printf "No Debian changes file was produced\n" >&2; exit 1; }
    ((${#buildinfo[@]} > 0)) || { printf "No Debian buildinfo file was produced\n" >&2; exit 1; }
    native_dist="{{ justfile_directory() }}/dist/native"
    mkdir -p "$native_dist"
    cp -- "${debs[@]}" "${changes[@]}" "${buildinfo[@]}" "$native_dist/"

    printf "\033[0;32m✓ Native Debian package built in dist/native/\033[0m\n"

# Built via podman for distro-repo compatibility.
[doc('Build HEAD; container, network')]
[group('packaging')]
package-native-rpm: (_require-tool "podman" "Install: sudo apt install podman")
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Build native RPM package" "podman + rpmbuild"

    VERSION="{{ package_version }}"
    COMMIT=$(git rev-parse --short HEAD)
    GO_VERSION=$(grep '^go ' go.mod | awk '{print $2}')

    # Prepare build directory
    BUILD_DIR=$(mktemp -d)
    BUILD_CONTAINER="gommitlint-package-$$"
    cleanup_build_dir() {
        local status=$?
        trap - EXIT
        podman rm --force --time 0 --ignore "$BUILD_CONTAINER" >/dev/null 2>&1 || true
        podman unshare rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        exit "$status"
    }
    trap cleanup_build_dir EXIT

    # Create source tarball and copy spec
    printf "\033[1;34m▸ Creating source tarball...\033[0m\n"
    git archive --format=tar.gz --prefix=gommitlint/ -o "${BUILD_DIR}/gommitlint-${VERSION}.tar.gz" HEAD
    cp packaging/native/rpm/gommitlint.spec "${BUILD_DIR}/"

    # renovate: datasource=docker depName=fedora
    FEDORA_IMAGE="fedora:45@sha256:aacbc26b38361ba0cb3d10f268da40b61b36aed6ed6fe5831a35871636bedb06"

    # Build inside Fedora container
    printf "\033[1;34m▸ Building RPM package in container (Go %s)...\033[0m\n" "${GO_VERSION}"
    # Ctrl-C must stop the build. The build script is PID 1 in the container
    # and ignores the interrupt, and the shell runs its exit trap only between
    # commands, so the run is in the background and cleanup removes the container.
    podman run --rm --name "${BUILD_CONTAINER}" \
        -v "${BUILD_DIR}:/work/pkg" \
        -v "{{ justfile_directory() }}/scripts/packaging/install-go.sh:/work/install-go.sh:ro" \
        -v "{{ justfile_directory() }}/scripts/packaging/rpm-build.sh:/work/rpm-build.sh:ro" \
        -e "PKG_DIR=/work/pkg" \
        -e "OUT_DIR=/work/pkg/output" \
        -e "VERSION=${VERSION}" \
        -e "COMMIT=${COMMIT}" \
        -e "GO_VERSION=${GO_VERSION}" \
        "${FEDORA_IMAGE}" \
        bash /work/rpm-build.sh &
    wait "$!"

    # Move artifacts
    shopt -s nullglob
    rpms=("$BUILD_DIR"/output/*.rpm)
    ((${#rpms[@]} > 0)) || { printf "No native RPM package was produced\n" >&2; exit 1; }
    native_dist="{{ justfile_directory() }}/dist/native"
    mkdir -p "$native_dist"
    cp -- "${rpms[@]}" "$native_dist/"

    printf "\033[0;32m✓ Native RPM package built in dist/native/\033[0m\n"

# Built via podman for distro-repo compatibility.
[doc('Build HEAD; container, network')]
[group('packaging')]
package-native-alpine: (_require-tool "podman" "Install: sudo apt install podman")
    #!/usr/bin/env bash
    set -euo pipefail
    "{{ just_executable() }}" _header "Build native Alpine package" "podman + abuild"

    VERSION="{{ package_version }}"
    GO_VERSION=$(grep '^go ' go.mod | awk '{print $2}')

    # Prepare build directory (podman creates root-owned files, use podman unshare for cleanup)
    BUILD_DIR=$(mktemp -d)
    BUILD_CONTAINER="gommitlint-package-$$"
    cleanup_build_dir() {
        local status=$?
        trap - EXIT
        podman rm --force --time 0 --ignore "$BUILD_CONTAINER" >/dev/null 2>&1 || true
        podman unshare rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        rm -rf -- "$BUILD_DIR" 2>/dev/null || true
        exit "$status"
    }
    trap cleanup_build_dir EXIT

    # Copy APKBUILD
    cp packaging/native/alpine/APKBUILD "${BUILD_DIR}/"

    # Update version in APKBUILD
    sed -i "s/^pkgver=.*/pkgver=${VERSION}/" "${BUILD_DIR}/APKBUILD"

    # Create source tarball
    printf "\033[1;34m▸ Creating source tarball...\033[0m\n"
    git archive --format=tar.gz --prefix=gommitlint/ -o "${BUILD_DIR}/gommitlint-${VERSION}.tar.gz" HEAD

    # Prepare persistent signing keys and output directories
    KEYS_DIR="{{ justfile_directory() }}/.abuild-keys"
    OUT_DIR="${BUILD_DIR}/packages"
    mkdir -p "${KEYS_DIR}" "${OUT_DIR}"

    # renovate: datasource=docker depName=alpine
    ALPINE_IMAGE="alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6"

    # Build APK inside Alpine container
    printf "\033[1;34m▸ Building Alpine package in container (Go %s)...\033[0m\n" "${GO_VERSION}"
    # Ctrl-C must stop the build. The build script is PID 1 in the container
    # and ignores the interrupt, and the shell runs its exit trap only between
    # commands, so the run is in the background and cleanup removes the container.
    podman run --rm --name "${BUILD_CONTAINER}" \
        -v "${BUILD_DIR}:/work/pkg" \
        -v "${KEYS_DIR}:/work/.abuild-keys" \
        -v "{{ justfile_directory() }}/scripts/packaging/install-go.sh:/work/install-go.sh:ro" \
        -v "{{ justfile_directory() }}/scripts/packaging/alpine-build.sh:/work/alpine-build.sh:ro" \
        -e "PKG_DIR=/work/pkg" \
        -e "KEYS_DIR=/work/.abuild-keys" \
        -e "OUT_DIR=/work/pkg/packages" \
        -e "GO_VERSION=${GO_VERSION}" \
        "${ALPINE_IMAGE}" \
        sh -eu /work/alpine-build.sh &
    wait "$!"

    # Move artifacts
    shopt -s nullglob
    apks=("$OUT_DIR"/*.apk)
    ((${#apks[@]} > 0)) || { printf "No native Alpine package was produced\n" >&2; exit 1; }
    native_dist="{{ justfile_directory() }}/dist/native"
    mkdir -p "$native_dist"
    cp -- "${apks[@]}" "$native_dist/"

    printf "\033[0;32m✓ Native Alpine package built in dist/native/\033[0m\n"

# Check native packages
[group('packaging')]
package-native-lint: (_require-tool "lintian" "Install lintian") (_require-tool "rpmlint" "Install rpmlint")
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    debs=(dist/native/*.deb)
    rpms=(dist/native/*.rpm)
    ((${#debs[@]} > 0)) || { printf "No native Debian packages found\n" >&2; exit 1; }
    ((${#rpms[@]} > 0)) || { printf "No native RPM packages found\n" >&2; exit 1; }
    "{{ just_executable() }}" _header "Lint native Debian packages" "lintian dist/native/*.deb"
    lintian -v -i -I --pedantic "${debs[@]}"
    "{{ just_executable() }}" _header "Lint native RPM packages" "rpmlint -c rpmlintrc.toml dist/native/*.rpm"
    rpmlint -c "{{ justfile_directory() }}/packaging/native/rpm/rpmlintrc.toml" "${rpms[@]}"
    "{{ just_executable() }}" _header "Lint Alpine APKBUILD" "apkbuild-lint packaging/native/alpine/APKBUILD"
    go tool -modfile={{ justfile_directory() }}/tools/go.mod apkbuild-lint "{{ justfile_directory() }}/packaging/native/alpine/APKBUILD"

# ==================================================================================== #
# RELEASE - Local release builds (nothing is published)
# ==================================================================================== #

# ▪ Build a local release snapshot
[group('release')]
release-snapshot: (_job "release-snapshot" "steps=release-check + verify + build + generate + unsigned snapshot; targets=.goreleaser.yaml; output=dist/; publication=none") release-check verify build generate
    @"{{ just_executable() }}" _header "Build unsigned snapshot" "goreleaser release --clean --snapshot --skip publish,sign"
    goreleaser release --clean --snapshot --skip publish,sign

# Validate the release configuration
[doc('Validate release config')]
[group('release')]
release-check: (_require-tool "goreleaser" "Install goreleaser with mise")
    @"{{ just_executable() }}" _header "Validate config" "goreleaser check"
    goreleaser check

# Rebuild a released tag with GoReleaser and compare top-level .tar.gz, .deb,
# .rpm and .apk files against that release's checksum manifest.
#
# This replaces `verify-reproducible`, which verified something other than
# what ships. That recipe built one host binary twice through
# scripts/build/build-cross-platform.sh, which sets GOAMD64=v2 where the
# release builds the default v1, whose ldflags lack the `all=` prefix the
# release uses, and which takes the version from `git describe` rather than
# the tag. It also never touched an archive, a package or a checksum byte.
#
# The downloaded manifest is NOT authenticated, so a match is a
# reproducibility result and not an authenticity one. Pass an independently
# verified manifest as the second argument to skip the download; see
# docs/modules/ROOT/pages/verify.adoc for the signer trust route. A supplied
# manifest is copied before generation or GoReleaser can overwrite dist/.
#
# Pass ver without the leading v (e.g. `just verify-rebuild 0.9.12`).
# HEAD must be the tag. SBOMs are excluded because syft stamps every document
# with a fresh serial number and timestamp, so they can never match; signing
# and publishing are skipped because Release CI owns both.
#
# Tags released before .goreleaser.yaml gained its archive and package mtime
# keys cannot match: those files carry the release runner's wall-clock
# timestamps, which no rebuild can reproduce.
# ver excludes the leading v; compares archive/package checksums.
[arg('ver', help='Release version without v, for example 0.9.12; HEAD must be that release tag')]
[arg('sums', help='Optional path to an independently verified checksum manifest')]
[doc('Compare tag rebuild; network')]
[group('release')]
verify-rebuild ver sums="":
    #!/usr/bin/env bash
    set -euo pipefail
    version="$1"
    checksums_path="${2:-}"
    if [[ -z "$version" || "$version" == v* ]] || ! git check-ref-format "refs/tags/v${version}"; then
        printf "{{ RED }}✗ ver must name a release version without the leading v{{ NC }}\n" >&2
        exit 2
    fi
    expected_tag="v${version}"
    actual_tag="$(git describe --tags --exact-match 2>/dev/null || true)"
    if [[ "$actual_tag" != "$expected_tag" ]]; then
        printf "{{ RED }}✗ HEAD is not the %s tag (got: %s){{ NC }}\n" \
            "$expected_tag" "${actual_tag:-<none>}" >&2
        printf "  Run {{ CYAN }}git checkout %s{{ NC }} first.\n" "$expected_tag" >&2
        exit 2
    fi
    manifest_copy=""
    cleanup() {
        [[ -z "$manifest_copy" ]] || rm -f -- "$manifest_copy"
    }
    trap cleanup EXIT
    if [[ -n "$checksums_path" ]]; then
        if [[ ! -f "$checksums_path" ]]; then
            printf "{{ RED }}✗ Checksum manifest is not a regular file: %s{{ NC }}\n" \
                "$checksums_path" >&2
            exit 2
        fi
        manifest_copy="$(mktemp)"
        cp -- "$checksums_path" "$manifest_copy"
    fi
    "{{ just_executable() }}" release-check generate
    "{{ just_executable() }}" _header "Rebuild ${expected_tag} from source" \
        "goreleaser release --clean --skip=publish,sign,sbom"
    goreleaser release --clean --skip=publish,sign,sbom
    if [[ -n "$manifest_copy" ]]; then
        "{{ just_executable() }}" _header "Compare with the supplied checksums" "$checksums_path"
        cp -- "$manifest_copy" {{ dist }}/published_checksums.txt
    else
        published_url="https://codefloe.com/itiquette/gommitlint/releases/download/${expected_tag}/gommitlint_${version}_checksums.txt"
        "{{ just_executable() }}" _header "Compare with the unauthenticated published checksums" \
            "$published_url"
        curl -sSfL "$published_url" -o {{ dist }}/published_checksums.txt
        printf "{{ RED }}! The downloaded manifest is not authenticated; this is a reproducibility comparison only.{{ NC }}\n" >&2
    fi
    cd {{ dist }}
    rebuilt=()
    while IFS= read -r artifact; do [[ -z "$artifact" ]] || rebuilt+=("$artifact"); done < <(shopt -s nullglob && printf '%s\n' *.tar.gz *.deb *.rpm *.apk | sort)
    if [[ "${#rebuilt[@]}" -eq 0 ]]; then
        printf "{{ RED }}✗ The rebuild produced no comparable files{{ NC }}\n" >&2
        exit 1
    fi
    : > comparison_checksums.txt
    for artifact in "${rebuilt[@]}"; do
        match_count=0
        selected_digest=""
        while read -r digest filename extra; do
            if [[ "${filename#\*}" == "$artifact" ]]; then
                match_count=$((match_count + 1))
                selected_digest="$digest"
                if [[ -n "${extra:-}" ]]; then
                    printf "{{ RED }}✗ Malformed checksum row for %s{{ NC }}\n" "$artifact" >&2
                    exit 1
                fi
            fi
        done < published_checksums.txt
        if [[ "$match_count" -ne 1 ]]; then
            printf "{{ RED }}✗ Expected one published checksum row for %s, found %s{{ NC }}\n" \
                "$artifact" "$match_count" >&2
            exit 1
        fi
        printf '%s  %s\n' "$selected_digest" "$artifact" >> comparison_checksums.txt
    done
    sha256sum --strict -c comparison_checksums.txt
    printf "{{ GREEN }}✓ %s rebuilt files match the checksum manifest for %s{{ NC }}\n" \
        "${#rebuilt[@]}" "$expected_tag"

# ==================================================================================== #
# DEPENDENCIES - Dependency management
# ==================================================================================== #

# Does not bump Go (use upgrade-go), Python or the held OpenGrep version.
[confirm("Bulk-upgrade dependencies, tools and container digests?")]
[doc('▪ Upgrade deps/tools; network')]
[group('dependencies')]
upgrade: upgrade-deps upgrade-go-dev-tools upgrade-mise upgrade-containers

# Upgrade Go; network
[group('dependencies')]
upgrade-go:
    @"{{ just_executable() }}" _header "Upgrade Go to latest stable version" "./scripts/dev/upgrade-go-version.sh"
    ./scripts/dev/upgrade-go-version.sh

# Upgrade Go dependencies; network
[group('dependencies')]
upgrade-deps:
    @"{{ just_executable() }}" _header "Upgrade all dependencies" "go get -u -t ./..."
    go get -u -t ./...
    @"{{ just_executable() }}" _header "Clean dependencies" "go mod tidy"
    go mod tidy

# Upgrade major versions; network
[group('dependencies')]
upgrade-major: (_require-tool "go-mod-upgrade" "Install: go install github.com/oligot/go-mod-upgrade@latest")
    @"{{ just_executable() }}" _header "Check major version upgrades" "go-mod-upgrade"
    go-mod-upgrade

# Upgrade Go dev tools; network
[group('dependencies')]
upgrade-go-dev-tools:
    @"{{ just_executable() }}" _header "Upgrade Go development tools" "./scripts/dev/upgrade-go-dev-tools.sh"
    ./scripts/dev/upgrade-go-dev-tools.sh

# Skips Go (upgrade-go coordinates its pins) and Python (chosen for uv tools).
[doc('Upgrade mise tools; network')]
[group('dependencies')]
upgrade-mise:
    @"{{ just_executable() }}" _header "Upgrade mise-managed tools" "mise upgrade --bump (keeping Go and Python pins)"
    @TOOLS=$(awk '/^\[tools\]/{t=1; next} /^\[/{t=0} t && /^[^#[:space:]]/ && /=/ { sub(/[[:space:]]*=.*/, ""); gsub(/"/, ""); if ($0 != "go" && $0 != "python") print }' .mise.toml | tr '\n' ' '); \
     if [[ -n "$TOOLS" ]]; then mise upgrade --bump $TOOLS; else printf "No mise tools to upgrade\n"; fi

# Upgrade image digests; network
[group('dependencies')]
upgrade-containers: (_require-tool "skopeo" "Install skopeo with your distro package manager") (_require-tool "cosign" "Install cosign with mise")
    @"{{ just_executable() }}" _header "Upgrade container image digests" "./scripts/dev/upgrade-container-digests.sh"
    ./scripts/dev/upgrade-container-digests.sh

# Upgrade image tags and digests; network
[doc('Upgrade image pins; network')]
[group('dependencies')]
upgrade-container-versions:
    @"{{ just_executable() }}" _header "Upgrade container image versions" "./scripts/dev/upgrade-container-versions.sh"
    ./scripts/dev/upgrade-container-versions.sh

# List available updates; network
[group('dependencies')]
upgrade-list:
    @"{{ just_executable() }}" _header "Check available updates" "./scripts/dev/list-updates.sh"
    ./scripts/dev/list-updates.sh

# ==================================================================================== #
# MAINTENANCE - Cache cleanup and utilities
# ==================================================================================== #

# ▪ Remove build and release outputs
[doc('▪ Remove build/release outputs')]
[group('maintenance')]
clean:
    #!/usr/bin/env bash
    set -euo pipefail
    root=$(CDPATH='' cd -- {{ quote(justfile_directory()) }} && pwd -P)
    "{{ just_executable() }}" _header "Remove compiled binaries"
    for name in bin dist; do
        target="$root/$name"
        case "$target" in
            "$root/bin"|"$root/dist") ;;
            *) printf "Refusing unsafe cleanup path: %s\n" "$target" >&2; exit 1 ;;
        esac
        if [[ -L "$target" ]]; then
            printf "Refusing to remove symlink: %s\n" "$target" >&2
            exit 1
        fi
        if [[ -e "$target" && ! -d "$target" ]]; then
            printf "Refusing to remove non-directory: %s\n" "$target" >&2
            exit 1
        fi
        rm -rf -- "$target"
    done
    printf "{{ GREEN }}✓{{ NC }} Build artifacts clean\n"

# Remove Go and linter caches
[confirm("Wipe Go build/module/test caches? Next builds re-download everything.")]
[group('maintenance')]
clean-caches:
    @"{{ just_executable() }}" _header "Clear all Go caches"
    @"{{ just_executable() }}" _run_with_output "Go cache clean" go clean -cache -modcache -testcache -fuzzcache
    @"{{ just_executable() }}" _header "Clear linter cache"
    @"{{ just_executable() }}" _run_with_output "Golangci-lint cache clean" golangci-lint cache clean
