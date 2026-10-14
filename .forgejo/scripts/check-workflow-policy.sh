#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Repository policy gates that do not concern workflow YAML. The workflow
# invariants (runner selection, job image, shipped-product test modes,
# gettext gating) live in test/policy as a Go test over the parsed
# YAML; this script keeps the checks on other files.

set -euo pipefail

repo_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)

fail() {
  printf 'ERROR: repository policy: %s\n' "$1" >&2
  exit 1
}

ci_containerfile="$repo_root/Containerfile.ci"
[[ -f "$ci_containerfile" && ! -L "$ci_containerfile" ]] ||
  fail 'Containerfile.ci must be a regular file'
grep -Fq 'ARG NANOLINTER_GO_IMAGE' "$ci_containerfile" ||
  fail 'Containerfile.ci must require an explicit Nanolinter Go base image'
for ci_package in gcc gettext libc6-dev openssh-client; do
  grep -Eq "^[[:space:]]+$ci_package=[^[:space:]\\\\]+([[:space:]\\\\]|$)" "$ci_containerfile" ||
    fail "Containerfile.ci must pin $ci_package"
done

if grep -R -n -E '^//go:build[[:space:]]+unit([[:space:]]|$)' "$repo_root/internal" "$repo_root/test/policy" "$repo_root/test/integration" --include='*_test.go' >/dev/null; then
  fail 'ordinary unit tests must not use the unit build tag'
fi

if grep -R -n -E '"codefloe.com/itiquette/gommitlint/(internal/|test/(integration|policy)(/|"))' "$repo_root/test/blackbox" --include='*.go' >/dev/null; then
  fail 'black-box tests must not import product internals or other test suites'
fi

if grep -R -n -E '^//go:build.*blackbox' "$repo_root/internal" "$repo_root/test/policy" "$repo_root/test/integration" --include='*.go' >/dev/null; then
  fail 'black-box build tags must stay out of internal, policy, and integration packages'
fi

verify_line=$(grep -E '^verify:' "$repo_root/justfile")
[[ "$verify_line" != *clean* ]] || fail 'just verify must remain read-only'
[[ "$verify_line" != *container* && "$verify_line" != *-vm* && "$verify_line" != *blackbox* && "$verify_line" != *distribution* ]] ||
  fail 'container, VM and black-box tiers must stay out of just verify'

printf 'OK: repository policy gates hold\n'
