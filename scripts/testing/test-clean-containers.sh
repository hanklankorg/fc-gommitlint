#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0
# Exercise the actual recipe against disposable fixtures, never real images.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
mkdir "$work/repo" "$work/outside"
sed -n '/^test-container-clean-distributions:/,/^$/p' "$root/justfile" >"$work/repo/justfile"
printf '\n_job name details:\n    @true\n' >>"$work/repo/justfile"
cd "$work/repo"

expect_status() {
  local expected=$1 status=0
  just test-container-clean-distributions >"$work/output" 2>&1 || status=$?
  if [[ "$status" != "$expected" ]]; then
    cat "$work/output" >&2
    exit 1
  fi
}

mkdir "$work/outside/distribution-images"
printf keep >"$work/outside/distribution-images/canary"
ln -s "$work/outside" bin
expect_status 2
[[ -f "$work/outside/distribution-images/canary" ]]
rm bin
mkdir bin
ln -s "$work/outside/distribution-images" bin/distribution-images
expect_status 2
[[ -f "$work/outside/distribution-images/canary" ]]
rm bin/distribution-images
mkdir bin/distribution-images
printf archive >bin/distribution-images/image.oci.tar
printf keep >bin/other-build-output
expect_status 0
[[ ! -e bin/distribution-images && -f bin/other-build-output ]]
expect_status 0
printf 'Container cleanup contracts passed (no engine).\n'
