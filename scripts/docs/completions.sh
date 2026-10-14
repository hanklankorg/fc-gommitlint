#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Generate package completion assets from a binary named exactly gommitlint.

set -euo pipefail

readonly BINARY_NAME="gommitlint"
readonly BINARY_PATH="${BINARY_PATH:-./bin/gommitlint}"
readonly OUTPUT_DIR="${OUTPUT_DIR:-generated/completions}"

stage=""

cleanup() {
  if [[ -n "$stage" ]]; then
    rm -rf -- "$stage"
  fi
}

trap cleanup EXIT

fail() {
  printf "Completion generation failed: %s\n" "$1" >&2
  exit 1
}

if (($# != 0)); then
  fail "this packaging command does not accept arguments"
fi

if [[ "$(basename -- "$BINARY_PATH")" != "$BINARY_NAME" ]]; then
  fail "BINARY_PATH must use the final gommitlint basename: $BINARY_PATH"
fi

if [[ ! -x "$BINARY_PATH" ]]; then
  fail "executable not found: $BINARY_PATH"
fi

mkdir -p -- "$OUTPUT_DIR"
stage=$(mktemp -d "$OUTPUT_DIR/.staging.XXXXXX") || fail "could not create staging directory"

for shell in bash zsh fish; do
  output_file="$stage/$BINARY_NAME.$shell"
  if ! LC_ALL=C LANG=C LANGUAGE=C "$BINARY_PATH" completion --stdout "$shell" >"$output_file"; then
    fail "$shell renderer returned a failure"
  fi

  if [[ ! -s "$output_file" ]]; then
    fail "$shell renderer returned empty output"
  fi

  chmod 0644 "$output_file"
done

# Every renderer succeeded before any published asset is replaced. Each rename
# is atomic because staging lives in the destination directory.
for shell in bash zsh fish; do
  mv -f -- "$stage/$BINARY_NAME.$shell" "$OUTPUT_DIR/$BINARY_NAME.$shell"
done

printf "Generated bash, zsh, and fish package completions in %s\n" "$OUTPUT_DIR"
