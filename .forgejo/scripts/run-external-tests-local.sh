#!/usr/bin/env bash

# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

set -euo pipefail
readonly suite=${1:-source}
(($# == 0)) || shift
fuzztime=''
if [[ "$suite" == fuzz && $# -eq 1 ]]; then
  fuzztime=$1
  shift
fi
# The unit and integration tiers forward their arguments to the `go test` that runs
# in the sandbox, which is what replaced the old integration-verbose suite: `-v`
# is an argument, not a tier. The arguments stay argv the whole way -- positional
# parameters here, after `--` to the sandbox launcher, and after the `_`
# placeholder of its `bash -c '<command> "$@"' _ ...` inside the container -- so
# nothing ever splices them into a command string for a shell to reparse.
#
# Accepting them here is a deliberate relaxation of a guard that refused every
# extra argument, so that a stray one could not quietly change what a
# destructive tier runs. It is relaxed for these tiers only: source, blackbox and
# fuzz run fixed commands, and an argument they would silently drop still stops
# the run. Unit mode snapshots source and never builds a shipped CLI artifact.
test_args=()
if [[ "$suite" == integration || "$suite" == unit ]]; then
  test_args=("$@")
  set --
fi
# The distribution matrix takes distribution names, not go test arguments:
# each must be a line of test/distribution/images, and none means all of them.
distributions=()
if [[ "$suite" == distribution ]]; then
  distributions=("$@")
  set --
fi
(($# == 0)) || {
  printf 'local external tests: unexpected arguments for %s\n' "$suite" >&2
  exit 2
}
case "$suite" in
source | unit | integration | blackbox | distribution | fuzz) ;;
*)
  printf 'usage: %s [source|unit [go test args...]|integration [go test args...]|blackbox|distribution [name...]|fuzz [time]]\n' "$0" >&2
  exit 2
  ;;
esac

# shellcheck source=.forgejo/scripts/sandbox-common.sh
source "$(dirname "$0")/sandbox-common.sh"
if [[ "$suite" == fuzz ]]; then sandbox_fuzz_seconds "$fuzztime" >/dev/null; fi
if [[ "$suite" == distribution ]]; then
  distribution_list=$(realpath -e -- "$(dirname "$0")/../..")/$SANDBOX_DISTRIBUTION_LIST
  known_distributions=$(sandbox_distribution_names "$distribution_list") || exit 2
  ((${#distributions[@]} > 0)) || mapfile -t distributions <<<"$known_distributions"
  for distribution in "${distributions[@]}"; do
    grep -Fxq -- "$distribution" <<<"$known_distributions" || {
      printf 'local external tests: unknown distribution %q; known: %s\n' "$distribution" "${known_distributions//$'\n'/ }" >&2
      exit 2
    }
  done
  test_args=("${distributions[@]}")
fi
sandbox_refuse_ci_product_mode "$suite"
sandbox_clear_ambient_container_env
if [[ "$suite" == fuzz ]]; then sandbox_reexec_clean "$suite" "$fuzztime"; else sandbox_reexec_clean "$suite" "${test_args[@]}"; fi
sandbox_clear_ambient_container_env

umask 077
repo_root=$(realpath -e -- "$(git rev-parse --show-toplevel)")
# The tier loads the archive `just test-container-prepare` recorded and never
# builds one, so no part of a run from here on can reach a registry. Checked
# first because it is the one prerequisite an operator has to act on, and
# because everything after it costs minutes.
recorded=$repo_root/$SANDBOX_IMAGE_DIR
readonly recorded
printf 'local external tests: checking prepared image recordings\n' >&2
if [[ "$suite" == distribution ]]; then
  for distribution in "${distributions[@]}"; do
    sandbox_require_recorded_image "$repo_root/$SANDBOX_DISTRIBUTION_DIR/$distribution" \
      "$repo_root/test/distribution/Containerfile" "$distribution"
  done
else
  sandbox_require_recorded_image "$recorded" "$repo_root/Containerfile.sandbox"
fi
# Short on purpose: this directory becomes RUNNER_TEMP for the sandbox scripts,
# whose runroot must stay within Podman's 50-character limit.
state=$(mktemp -d "${TMPDIR:-/tmp}/gl-local.XXXXXX")
cleanup_local_state() {
  local result=$?
  trap - EXIT
  printf 'local external tests: cleaning temporary source and build files\n' >&2
  sandbox_remove_private_tree "$state" || result=1
  printf 'local external tests: finished (exit %s)\n' "$result" >&2
  exit "$result"
}
trap cleanup_local_state EXIT
[[ "$state" != "$repo_root/"* ]] || {
  printf 'local external tests: private state cannot be inside the checkout\n' >&2
  exit 1
}
mkdir -p "$state"/{build-cache,go-mod,go-path,home,product,snapshot-state}

if [[ "$suite" == fuzz ]]; then
  artifact_output=$(mktemp -d "${TMPDIR:-/tmp}/gommitlint-fuzz-artifacts.XXXXXX")
  set +e
  RUNNER_TEMP="$state" bash .forgejo/scripts/run-external-tests-sandbox.sh fuzz \
    --artifact-output "$artifact_output" \
    --fuzztime "$fuzztime" \
    --sandbox-image "$recorded/sandbox-image.oci.tar" \
    --sandbox-image-checksum "$recorded/sandbox-image.sha256"
  result=$?
  set -e
  printf 'local fuzz artifacts: %s\n' "$artifact_output" >&2
  exit "$result"
fi

source_root=$state/source
source_manifest=$state/snapshot-state/source.manifest
source_manifest_checksum=$state/snapshot-state/source.manifest.sha256
printf 'local external tests: snapshotting current source\n' >&2
sandbox_snapshot_worktree "$repo_root" "$source_root" "$source_manifest" "$source_manifest_checksum"
snapshot_hash=$(sha256sum "$source_manifest")
snapshot_hash=${snapshot_hash%% *}
printf 'local external tests: source snapshot sha256:%s\n' "$snapshot_hash" >&2

if [[ "$suite" == unit ]]; then
  RUNNER_TEMP="$state" bash "$source_root/.forgejo/scripts/run-external-tests-sandbox.sh" unit \
    --source-root "$source_root" \
    --source-manifest "$source_manifest" \
    --source-manifest-checksum "$source_manifest_checksum" \
    --sandbox-image "$recorded/sandbox-image.oci.tar" \
    --sandbox-image-checksum "$recorded/sandbox-image.sha256" \
    -- "${test_args[@]}"
  exit "$?"
fi

printf 'local external tests: compiling Gommitlint from the snapshot (timeout 5m)\n' >&2
(
  cd "$source_root"
  timeout 5m env -i PATH="$PATH" CGO_ENABLED=0 GOCACHE="$state/build-cache" GOENV=off \
    GOMODCACHE="$state/go-mod" GOPATH="$state/go-path" GOTOOLCHAIN=local HOME="$state/home" \
    XDG_CACHE_HOME="$state/home/cache" "${SANDBOX_MODULE_ENV[@]}" go build -trimpath -buildvcs=false -o "$state/product/gommitlint" ./cmd/gommitlint
)
printf 'local external tests: verifying source and checksumming compiled Gommitlint\n' >&2
sandbox_verify_snapshot "$source_root" "$source_manifest" "$source_manifest_checksum" || {
  printf 'local external tests: source snapshot changed during product compilation\n' >&2
  exit 1
}
(cd "$state/product" && sha256sum gommitlint >gommitlint.sha256)

if [[ "$suite" == distribution ]]; then
  # The distribution images carry no Go toolchain, so the tagged harnesses are
  # compiled here, from the same snapshot and with the same clean environment
  # as the product, and are never executed here. Each lands at its package's
  # path under the harness tree, so the container runs it from the matching
  # directory of the read-only source. The tree gets a manifest of its own,
  # in the snapshot's format, and the sandbox launcher stages and verifies it
  # exactly as it does the source.
  harness_root=$state/harness
  harness_manifest=$state/snapshot-state/harness.manifest
  harness_manifest_checksum=$state/snapshot-state/harness.manifest.sha256
  go_env=(env -i PATH="$PATH" CGO_ENABLED=0 GOCACHE="$state/build-cache" GOENV=off
    GOMODCACHE="$state/go-mod" GOPATH="$state/go-path" GOTOOLCHAIN=local HOME="$state/home"
    XDG_CACHE_HOME="$state/home/cache" "${SANDBOX_MODULE_ENV[@]}")
  (
    cd "$source_root"
    printf 'local external tests: listing distribution test packages\n' >&2
    mapfile -t packages < <(timeout 5m "${go_env[@]}" go list -tags=integration \
      -f '{{if or .TestGoFiles .XTestGoFiles}}{{.Dir}}{{end}}' ./test/integration/...)
    harnesses=()
    for package in "${packages[@]}"; do
      [[ -n "$package" ]] || continue
      package=${package#"$PWD"/}
      sandbox_safe_snapshot_path "$package"
      harnesses+=("integration $package")
    done
    ((${#harnesses[@]} > 0))
    harnesses+=("blackbox test/blackbox")
    for harness in "${harnesses[@]}"; do
      tags=${harness%% *}
      package=${harness#* }
      mkdir -p "$harness_root/$package"
      printf 'local external tests: compiling %s tests in %s (timeout 5m)\n' "$tags" "$package" >&2
      timeout 5m "${go_env[@]}" go test -c -tags="$tags" -trimpath -buildvcs=false \
        -o "$harness_root/$package/$tags.test" "./$package"
    done
  )
  : >"$harness_manifest"
  while IFS= read -r -d '' harness; do
    chmod 0555 "$harness"
    hash=$(sha256sum -- "$harness")
    printf '%s 0555 %s\n' "${hash%% *}" "${harness#"$harness_root"/}" >>"$harness_manifest"
  done < <(find "$harness_root" -type f -print0 | sort -z)
  while IFS= read -r -d '' harness; do chmod 0555 "$harness"; done < <(find "$harness_root" -type d -print0)
  (cd "$state/snapshot-state" && sha256sum harness.manifest >harness.manifest.sha256)
  sandbox_verify_snapshot "$harness_root" "$harness_manifest" "$harness_manifest_checksum" || {
    printf 'local external tests: harness tree does not match its manifest\n' >&2
    exit 1
  }
  sandbox_verify_snapshot "$source_root" "$source_manifest" "$source_manifest_checksum" || {
    printf 'local external tests: source snapshot changed during harness compilation\n' >&2
    exit 1
  }

  # One sandbox per distribution, each from its own recorded image and in its
  # own private store, all against the same verified product and harnesses.
  # A failing distribution does not stop the rest; the summary names every
  # result and any failure fails the run.
  declare -A results=()
  failed=0
  for distribution in "${distributions[@]}"; do
    printf '\n######## %s\n' "$distribution" >&2
    results[$distribution]=FAILED
    recorded_distribution=$repo_root/$SANDBOX_DISTRIBUTION_DIR/$distribution
    if sandbox_require_recorded_image "$recorded_distribution" "$source_root/test/distribution/Containerfile" "$distribution" &&
      RUNNER_TEMP="$state" bash "$source_root/.forgejo/scripts/run-external-tests-sandbox.sh" distribution \
        --distribution "$distribution" \
        --source-root "$source_root" \
        --source-manifest "$source_manifest" \
        --source-manifest-checksum "$source_manifest_checksum" \
        --source-bin "$state/product/gommitlint" \
        --source-checksum "$state/product/gommitlint.sha256" \
        --harness-root "$harness_root" \
        --harness-manifest "$harness_manifest" \
        --harness-manifest-checksum "$harness_manifest_checksum" \
        --sandbox-image "$recorded_distribution/sandbox-image.oci.tar" \
        --sandbox-image-checksum "$recorded_distribution/sandbox-image.sha256"; then
      results[$distribution]=passed
    else
      failed=1
    fi
  done
  printf '\n== distribution matrix, source snapshot sha256:%s\n' "$snapshot_hash" >&2
  for distribution in "${distributions[@]}"; do
    printf '  %-22s %s\n' "$distribution" "${results[$distribution]}" >&2
  done
  exit "$failed"
fi
# Again, now against the snapshot that is about to be tested rather than the
# live checkout: the image has to match the Containerfile in the tree the tier
# actually runs, and the two can drift while the product compiles.
printf 'local external tests: rechecking the prepared image against the source snapshot\n' >&2
sandbox_require_recorded_image "$recorded" "$source_root/Containerfile.sandbox"
RUNNER_TEMP="$state" bash "$source_root/.forgejo/scripts/run-external-tests-sandbox.sh" "$suite" \
  --source-root "$source_root" \
  --source-manifest "$source_manifest" \
  --source-manifest-checksum "$source_manifest_checksum" \
  --source-bin "$state/product/gommitlint" \
  --source-checksum "$state/product/gommitlint.sha256" \
  --sandbox-image "$recorded/sandbox-image.oci.tar" \
  --sandbox-image-checksum "$recorded/sandbox-image.sha256" \
  -- "${test_args[@]}"
