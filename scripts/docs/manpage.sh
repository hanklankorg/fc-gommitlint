#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Project-agnostic man page generator
# Usage: ./scripts/docs/manpage.sh [binary_name] [source_doc] [output_dir]
# Defaults: binary_name=gommitlint, source_doc=docs/manpages/gommitlint.1.md, output_dir=generated/manpages

set -uo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly NC=$'\033[0m' # No Color

# Parse arguments with defaults
readonly BINARY_NAME="${1:-${BINARY_NAME:-gommitlint}}"
readonly SOURCE_DOC="${2:-${SOURCE_DOC:-docs/manpages/${BINARY_NAME}.1.md}}"
readonly OUTPUT_DIR="${3:-${OUTPUT_DIR:-generated/manpages}}"
readonly LOCALES_DIR="${LOCALES_DIR:-docs/manpages/locale}"

log() {
  printf "${YELLOW}▸${NC} %s\n" "$1" 2>/dev/null || true
}

success() {
  printf "${GREEN}✓${NC} %s\n" "$1" 2>/dev/null || true
}

fail() {
  printf "${RED}✗${NC} %s\n" "$1" >&2 2>/dev/null || true
  exit 1
}

check_prerequisites() {
  local missing=()

  if ! command -v go-md2man >/dev/null 2>&1; then
    missing+=("go-md2man")
  fi

  if ! command -v gzip >/dev/null 2>&1; then
    missing+=("gzip")
  fi

  if ((${#missing[@]} > 0)); then
    fail "Missing dependencies: ${missing[*]}"
  fi

  if [[ ! -f "$SOURCE_DOC" ]]; then
    fail "Source document not found: $SOURCE_DOC"
  fi

  if [[ ! -r "$SOURCE_DOC" ]]; then
    fail "Source document not readable: $SOURCE_DOC"
  fi
}

generate_manpage_for_source() {
  local source_doc="$1"
  local output_dir="$2"
  local locale_label="$3"

  mkdir -p "$output_dir" || fail "Failed to create output directory: $output_dir"

  local man_file="$output_dir/${BINARY_NAME}.1"
  local gz_file="${man_file}.gz"

  if [[ -n "$locale_label" ]]; then
    log "Generating man page ($locale_label) from $source_doc"
  else
    log "Generating man page from $source_doc"
  fi

  go-md2man -in "$source_doc" -out "$man_file" || fail "Failed to generate man page from $source_doc"

  log "Compressing man page"
  gzip -9 -n -f "$man_file" || fail "Failed to compress man page: $man_file"

  if [[ ! -f "$gz_file" ]]; then
    fail "Man page generation failed: $gz_file not created"
  fi

  local size
  size=$(stat -f%z "$gz_file" 2>/dev/null || stat -c%s "$gz_file" 2>/dev/null || echo "unknown")
  success "Generated man page: $gz_file (size: $size bytes)"
}

generate_manpages() {
  generate_manpage_for_source "$SOURCE_DOC" "$OUTPUT_DIR" ""

  if [[ ! -d "$LOCALES_DIR" ]]; then
    return 0
  fi

  local locale_dir locale source_doc
  for locale_dir in "$LOCALES_DIR"/*; do
    if [[ ! -d "$locale_dir" ]]; then
      continue
    fi

    locale=$(basename "$locale_dir")
    source_doc="$locale_dir/${BINARY_NAME}.1.md"
    if [[ -f "$source_doc" ]]; then
      generate_manpage_for_source "$source_doc" "$OUTPUT_DIR/$locale/man1" "$locale"
    fi
  done
}

show_installation_help() {
  {
    printf "\n%sMan Page Installation:%s\n" "$YELLOW" "$NC"
    printf "  System-wide: %ssudo cp %s/%s.1.gz /usr/share/man/man1/%s\n" "$GREEN" "$OUTPUT_DIR" "$BINARY_NAME" "$NC"
    printf "  User-local:  %scp %s/%s.1.gz ~/.local/share/man/man1/%s\n" "$GREEN" "$OUTPUT_DIR" "$BINARY_NAME" "$NC"
    printf "  View with:   %sman %s%s\n" "$GREEN" "$BINARY_NAME" "$NC"
    if [[ -d "$OUTPUT_DIR" ]]; then
      printf "  Locales:     %s<lang>/man1/%s.1.gz%s\n" "$GREEN" "$BINARY_NAME" "$NC"
    fi
  } 2>/dev/null || true
}

main() {
  log "Starting man page generation for $BINARY_NAME"

  check_prerequisites
  generate_manpages
  show_installation_help

  success "Man page generation completed"
}

main "$@"
