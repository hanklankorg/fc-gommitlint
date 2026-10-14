#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

set -euo pipefail

readonly GETTEXT_VERSION="0.23.1"
readonly PREFIX=${PREFIX:-"$HOME/.local"}

cleanup() {
  rm -rf -- "$work_dir"
}

check_dependencies() {
  local tool
  for tool in make gcc; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      printf 'Missing dependency: %s\n' "$tool" >&2
      exit 1
    fi
  done
  if ! command -v wget >/dev/null 2>&1 && ! command -v curl >/dev/null 2>&1; then
    printf 'Missing dependency: wget or curl\n' >&2
    exit 1
  fi
}

main() {
  local archive="gettext-${GETTEXT_VERSION}.tar.gz" url work_parent
  check_dependencies
  # WORK_DIR selects a parent, never a directory we may recursively remove.
  work_parent=$(cd "${WORK_DIR:-${TMPDIR:-/tmp}}" && pwd -P)
  work_dir=$(mktemp -d "$work_parent/gommitlint-gettext.XXXXXXXX")
  trap cleanup EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  url="https://ftp.gnu.org/gnu/gettext/${archive}"

  printf 'Downloading gettext %s\n' "$GETTEXT_VERSION"
  if command -v wget >/dev/null 2>&1; then
    wget -q -O "$work_dir/$archive" "$url"
  else
    curl -fsSL -o "$work_dir/$archive" "$url"
  fi

  printf 'Extracting\n'
  tar -xzf "$work_dir/$archive" -C "$work_dir"

  printf 'Building\n'
  cd "$work_dir/gettext-${GETTEXT_VERSION}"
  ./configure --prefix="$PREFIX"
  make
  make install

  printf 'Installed gettext %s to %s\n' "$GETTEXT_VERSION" "$PREFIX"
  printf 'Ensure %s/bin is on your PATH\n' "$PREFIX"
  printf "Note: Go extraction is handled by Gommitlint's built-in extractor.\n"
  printf 'This install is for msgmerge/msgen/msgattrib/msgfmt only.\n'
}

main "$@"
