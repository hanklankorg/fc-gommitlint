#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0
#
# Translation drift gate. Mirrors what `gommitlint translations update` would
# do and checks the result matches the checked-in locale/ tree. Fails if a
# developer added a new tr.T(...) call without rerunning the update.
#
# Implemented as a script rather than a Go test because the underlying
# built-in extraction pipeline walks every Go file in the repo.
# That's a build-tooling check, not a unit test — running it under `go test`
# slowed down every test invocation by an order of magnitude.

set -euo pipefail

readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly NC=$'\033[0m'

log() {
  printf "${YELLOW}▸${NC} %s\n" "$1"
}

success() {
  printf "${GREEN}✓${NC} %s\n" "$1"
}

fail() {
  printf "${RED}✗${NC} %s\n" "$1" >&2
  exit 1
}

# Required tools — same set the `translations update` pipeline shells out to.
for tool in msgmerge msgen msgattrib go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    fail "$tool not on PATH. Install GNU gettext (apt install gettext / brew install gettext)."
  fi
done

repo_root="$(git rev-parse --show-toplevel 2>/dev/null)" || fail "must be run inside the gommitlint git repo"
src_locale="$repo_root/locale"
[[ -d "$src_locale" ]] || fail "locale dir not found at $src_locale"

tmp_root="$(mktemp -d)"
trap 'rm -rf "$tmp_root"' EXIT
tmp_locale="$tmp_root/locale"

log "Mirroring locale/ to tmpdir"
cp -R "$src_locale" "$tmp_locale"

log "Running translations update against tmpdir"
(cd "$repo_root" && go run ./cmd/gommitlint translations update --locale-dir "$tmp_locale")

# Normalize each .po before comparison. `#:` source-location comments and the
# POT-Creation-Date / PO-Revision-Date headers vary per-machine and per-run.
normalize() {
  grep -v -E '^#:|^"POT-Creation-Date|^"PO-Revision-Date' "$1"
}

drifted=0
catalogs=("$src_locale/gommitlint.pot" "$src_locale"/*/gommitlint.po)
for src_catalog in "${catalogs[@]}"; do
  [[ -f "$src_catalog" ]] || continue
  rel="${src_catalog#"$src_locale/"}"
  tmp_catalog="$tmp_locale/$rel"
  if [[ ! -f "$tmp_catalog" ]]; then
    printf "${RED}✗${NC} %s was not regenerated\n" "$rel" >&2
    drifted=1
    continue
  fi
  if ! diff -u <(normalize "$src_catalog") <(normalize "$tmp_catalog") >/dev/null; then
    printf "${RED}✗${NC} %s is out of date:\n" "$rel" >&2
    diff -u <(normalize "$src_catalog") <(normalize "$tmp_catalog") | head -50 >&2
    drifted=1
  fi
done

if [[ $drifted -ne 0 ]]; then
  fail "Translation catalogs drifted. Run 'just update-translations' and commit the changes."
fi

success "Translation catalogs are up to date"
