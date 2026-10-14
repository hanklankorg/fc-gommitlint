#!/usr/bin/env bash

# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Builds the sandbox the external test tiers run in, from this repository's
# own Containerfile.sandbox, and exports it as a read-only checksummed OCI
# archive with a key saying what it was built from. The tiers used to pull a
# linter image from the forge registry: an image built for another purpose,
# whose capabilities nobody guaranteed, and one the forge had to serve on every
# run. The project now ships the sandbox its own tests need.
#
# This script is the only part of the test setup that reaches a network: it
# fetches the digest-pinned base from docker.io and the packages from Debian.
# It used to run inside every tier, which is why those tiers were tagged as
# reaching the network and why each of them paid two minutes before its first
# test. It is now its own step -- `just test-container-prepare` -- run when the
# Containerfile changes; the tiers load what it recorded and refuse when the
# recording is missing or no longer matches.
#
# With no argument it records into the repository's own ignored build-output
# path, which is where the tiers look. CI passes a directory of its own,
# because its jobs hand the archive between each other as an artifact.
#
# `--distribution NAME` records one image of the local-only distribution
# matrix instead: test/distribution/Containerfile over the base
# test/distribution/images pins for NAME, into
# bin/distribution-images/NAME. The same rules hold -- the only registry
# repository admitted is that base's, and the tier that loads the recording
# never builds one. No workflow passes this option.

set -euo pipefail
# shellcheck source=.forgejo/scripts/sandbox-common.sh
source "$(dirname "$0")/sandbox-common.sh"
sandbox_clear_ambient_container_env
sandbox_reexec_clean "$@"
sandbox_clear_ambient_container_env
repo_root=$(realpath -e -- "$(dirname "$0")/../..")
readonly repo_root
distribution=''
if [[ "${1:-}" == --distribution ]]; then
  [[ $# -ge 2 && -n "$2" ]] || {
    printf 'sandbox image: --distribution requires a name\n' >&2
    exit 2
  }
  distribution=$2
  shift 2
fi
readonly distribution
(($# <= 1)) || {
  printf 'usage: %s [--distribution NAME] [output-directory]\n' "$0" >&2
  exit 2
}
build_args=()
if [[ -n "$distribution" ]]; then
  base=$(sandbox_distribution_base "$repo_root/$SANDBOX_DISTRIBUTION_LIST" "$distribution")
  [[ "$base" =~ ^(.+):[^:/@]+@sha256:[a-f0-9]{64}$ ]]
  repository=${BASH_REMATCH[1]}
  image_ref=$SANDBOX_DISTRIBUTION_IMAGE_REPO:$distribution
  output=${1:-$repo_root/$SANDBOX_DISTRIBUTION_DIR/$distribution}
  containerfile=$repo_root/test/distribution/Containerfile
  build_args=(--build-arg "BASE_IMAGE=$base")
else
  repository=docker.io/library/golang
  image_ref=$SANDBOX_IMAGE_REF
  output=${1:-$repo_root/$SANDBOX_IMAGE_DIR}
  containerfile=$repo_root/Containerfile.sandbox
fi
readonly repository image_ref output containerfile build_args
# Read before the build, written after it, so the recorded key describes the
# Containerfile the archive was actually built from even if the file is edited
# while the build runs. A later edit then reads as stale, which it is.
key=$(sandbox_image_key "$containerfile" "$distribution")
readonly key
printf '== Image preparation: engine=podman | host=%s/%s | target=Linux/engine-default\n   image=%s | distribution=%s | output=%s | network=allowed\n' \
  "$(uname -s)" "$(uname -m)" "$image_ref" "${distribution:-Go sandbox}" "$output" >&2
sandbox_init gl-img "$repository"
trap sandbox_cleanup EXIT
test "$(sandbox_podman info --format '{{.Host.Security.Rootless}}')" = true
mkdir -p "$output"
# Empty build context: Containerfile.sandbox has no COPY, so no part of the
# checkout is handed to the builder. A distribution image copies exactly one
# file, its install script, and the context holds that and nothing else.
readonly context="$SANDBOX_STATE/context"
mkdir -p "$context"
if [[ -n "$distribution" ]]; then
  install -m 0444 -- "$repo_root/test/distribution/install-deps.sh" "$context/install-deps.sh"
fi
# A build has to pull a base and install packages, so it gets a longer bound
# than the pull it replaces; every step stays bounded.
#
# The cgroup manager is named because the sandbox runs with no session bus on
# purpose, and crun cannot ask systemd for a scope without one. Nothing is
# given up: the build sets no resource limits. The limits that matter are on
# the test container in run-external-tests-sandbox.sh, which keeps Podman's
# own choice of manager.
sandbox_podman_timeout 20m --cgroup-manager=cgroupfs build --pull=always --timestamp 0 \
  --cert-dir "$SANDBOX_STATE/certs" --file "$containerfile" "${build_args[@]}" \
  --tag "$image_ref" "$context" >/dev/null
sandbox_podman image inspect "$image_ref" >/dev/null
# Recording is a replacement, and what is already there is read-only, so the
# three files are removed rather than written through. Only after the build
# succeeded: a build that fails leaves the previous recording alone, and if
# that recording no longer matches the Containerfile the tiers say so.
# Anything at these paths that is not a plain file we wrote is a refusal.
for name in sandbox-image.oci.tar sandbox-image.sha256 sandbox-image.key; do
  target=$output/$name
  [[ ! -L "$target" && (! -e "$target" || -f "$target") ]] || {
    printf 'sandbox image: refusing to replace %s: not a regular file\n' "$target" >&2
    exit 1
  }
  rm -f -- "$target"
done
printf 'sandbox image: exporting OCI archive and recording checksum (timeout 5m)\n' >&2
sandbox_podman_timeout 5m save --format oci-archive --output "$output/sandbox-image.oci.tar" "$image_ref"
(cd "$output" && sha256sum sandbox-image.oci.tar >sandbox-image.sha256)
# Last, so a half-written recording has no key and the tiers refuse it.
printf '%s\n' "$key" >"$output/sandbox-image.key"
chmod 0444 "$output/sandbox-image.oci.tar" "$output/sandbox-image.sha256" "$output/sandbox-image.key"
printf 'sandbox image recorded in %s\n%s\n' "$output" "$key"
