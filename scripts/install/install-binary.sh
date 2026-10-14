#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Project-agnostic binary installer with automatic architecture detection
# Usage: ./scripts/install-binary.sh [binary_name] [bin_dir] [install_dir]
# Defaults: binary_name=gommitlint, bin_dir=./bin, install_dir=$HOME/.local/bin

set -euo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly NC=$'\033[0m' # No Color

# Parse arguments with defaults
arg_one="${1:-}"
arg_two="${2:-}"
arg_three="${3:-}"

if [[ -n "$arg_one" && -n "$arg_two" && -d "$arg_one" ]]; then
  BIN_DIR="$arg_one"
  BINARY_NAME="$arg_two"
else
  BINARY_NAME="${arg_one:-${BINARY_NAME:-gommitlint}}"
  BIN_DIR="${arg_two:-${BIN_DIR:-./bin}}"
fi

# Install destination, resolved in order of precedence:
#   1. third positional argument
#   2. INSTALL_DIR env var (explicit override)
#   3. XDG_BIN_HOME env var (de-facto XDG convention for user binaries)
#   4. $HOME/.local/bin (sensible default — already on PATH for most setups)
INSTALL_DIR="${arg_three:-${INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}}"

readonly BINARY_NAME
readonly BIN_DIR
readonly INSTALL_DIR

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

# Detect system architecture
detect_architecture() {
  local arch
  arch=$(uname -m)

  case $arch in
  x86_64)
    echo "amd64"
    ;;
  aarch64 | arm64)
    echo "arm64"
    ;;
  *)
    fail "Unsupported architecture: $arch"
    ;;
  esac
}

main() {
  # Detect architecture
  local arch
  arch=$(detect_architecture)
  local os="${OS:-linux}"

  # Find source binary - try arch-specific first, then generic
  local source_binary=""
  local candidates=(
    "${BIN_DIR}/${BINARY_NAME}-${os}-${arch}"
    "${BIN_DIR}/${BINARY_NAME}"
  )

  for candidate in "${candidates[@]}"; do
    if [[ -f "$candidate" ]]; then
      source_binary="$candidate"
      break
    fi
  done

  if [[ -z "$source_binary" ]]; then
    fail "Binary not found: tried ${BINARY_NAME}-${os}-${arch} and ${BINARY_NAME} in ${BIN_DIR}"
  fi

  # Create install directory if it doesn't exist
  if [[ ! -d "$INSTALL_DIR" ]]; then
    log "Creating directory: $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR" || fail "Failed to create directory: $INSTALL_DIR"
  fi

  # Install binary
  local dest="${INSTALL_DIR}/${BINARY_NAME}"
  log "Installing ${BINARY_NAME} (${arch}) to ${dest}"

  # -f overwrites read-only files and replaces an existing symlink target
  # cleanly; without it a stale install with locked permissions would error.
  cp -f "$source_binary" "$dest" || fail "Failed to copy binary"
  chmod +x "$dest" || fail "Failed to make binary executable"

  success "Successfully installed ${BINARY_NAME} to ${dest}"

  if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    printf "%sNote:%s %s is not in your PATH\n" "$YELLOW" "$NC" "$INSTALL_DIR"
    printf "Add it with: export PATH=\"\$PATH:%s\"\n" "$INSTALL_DIR"
  fi
}

main "$@"
