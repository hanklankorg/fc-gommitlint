#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Project-agnostic Go development tools installer
# Usage: ./scripts/dev/install-go-dev-tools.sh [tools_dir]
# Default: tools_dir=tools

set -euo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly CYAN=$'\033[1;36m'
readonly NC=$'\033[0m' # No Color

# Parse arguments with defaults
readonly TOOLS_DIR="${1:-${TOOLS_DIR:-tools}}"

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

# Efficient command finder
find_command() {
  local cmd="$1"
  shift
  local search_paths=("$@" "$cmd")

  for path in "${search_paths[@]}"; do
    if command -v "$path" >/dev/null 2>&1; then
      echo "$path"
      return 0
    fi
  done
  return 1
}

check_go() {
  local minimum_version=1.27.2
  local go_cmd
  go_cmd=$(find_command go "$HOME/go/bin/go" "/usr/local/go/bin/go") || {
    fail "Go not found. Please install Go ${minimum_version} or later"
  }

  local go_version major minor patch required_major required_minor required_patch
  go_version=$("$go_cmd" env GOVERSION | cut -c3-)
  IFS=. read -r major minor patch <<<"$go_version"
  IFS=. read -r required_major required_minor required_patch <<<"$minimum_version"
  patch=${patch%%[^0-9]*}
  if [[ ! "$major" =~ ^[0-9]+$ || ! "$minor" =~ ^[0-9]+$ || ! "$patch" =~ ^[0-9]+$ ||
    ! "$required_major" =~ ^[0-9]+$ || ! "$required_minor" =~ ^[0-9]+$ || ! "$required_patch" =~ ^[0-9]+$ ]]; then
    fail "Invalid Go version: required ${minimum_version}, found ${go_version}"
  fi

  if ((10#$major < 10#$required_major)) ||
    ((10#$major == 10#$required_major && 10#$minor < 10#$required_minor)) ||
    ((10#$major == 10#$required_major && 10#$minor == 10#$required_minor && 10#$patch < 10#$required_patch)); then
    fail "Go ${minimum_version} or later is required (found Go ${go_version} at ${go_cmd})"
  fi
  success "Go $go_version found at $go_cmd" >&2
  echo "$go_cmd"
}

install_go_tools() {
  local go_cmd="$1"

  if [[ ! -d "$TOOLS_DIR" ]] || [[ ! -f "$TOOLS_DIR/go.mod" ]]; then
    fail "Tools module not found at $TOOLS_DIR/go.mod"
  fi

  log "Installing Go development tools from $TOOLS_DIR"

  # Install tools from tools module
  (
    cd "$TOOLS_DIR"

    # Extract tools using go tool command (Go 1.24+)
    # Go 1.27 prints "name (import/path)"; earlier releases print the bare path.
    # Unwrap the parenthesised form, then filter to module paths only
    # (contain '/'), which excludes built-in tools such as asm and vet.
    local tool_output tools
    tool_output=$("$go_cmd" tool -n 2>/dev/null) || fail "Failed to read $TOOLS_DIR/go.mod tool directives"
    tools=$(sed -E 's/^[^[:space:]]+ \((.+)\)$/\1/' <<<"$tool_output" | grep '/' || true)

    if [[ -z "$tools" ]]; then
      fail "No tool directives found in $TOOLS_DIR/go.mod"
    fi

    while IFS= read -r tool; do
      if [[ -n "$tool" ]]; then
        log "Installing $tool"
        "$go_cmd" install -v "$tool" || fail "Failed to install $tool"
      fi
    done <<<"$tools"
  )

  # Report where the binaries actually landed. `go install` honours GOBIN and
  # falls back to $GOPATH/bin, so the destination is not always ~/go/bin and is
  # worth naming rather than leaving the caller to guess.
  local install_dir
  install_dir=$("$go_cmd" env GOBIN)
  if [[ -z "$install_dir" ]]; then
    install_dir="$("$go_cmd" env GOPATH)/bin"
  fi

  success "Go tools installation completed"
  printf "  Installed to: %s\n" "$install_dir"

  case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf "! %s is not on PATH; the installed tools will not be found\n" "$install_dir" ;;
  esac
}

check_external_tools() {
  log "Checking for recommended external tools..."

  local external_tools=(
    "goreleaser" "cosign" "scorecard" "syft"
    "actionlint" "gitleaks" "shellcheck" "shfmt"
    "yamlfmt" "hadolint" "trivy" "rumdl"
  )

  local missing=()
  for tool in "${external_tools[@]}"; do
    if command -v "$tool" >/dev/null 2>&1; then
      continue
    fi
    if command -v mise >/dev/null 2>&1; then
      if mise exec -- sh -c "command -v $tool" >/dev/null 2>&1; then
        continue
      fi
    fi
    missing+=("$tool")
  done

  if ((${#missing[@]} > 0)); then
    printf "%sMissing external tools:%s %s\n" "$YELLOW" "$NC" "${missing[*]}"
    printf "Recommended: Install with mise (mise install) or via your package manager\n"
    printf "See .mise.toml for tool versions and installation details\n"
  else
    success "All recommended external tools found"
  fi
}

main() {
  printf "%s→%s Setting up Go development tools\n\n" "$CYAN" "$NC"

  local go_cmd
  go_cmd=$(check_go)

  install_go_tools "$go_cmd"
  check_external_tools

  printf "\n"
  success "Development tools setup completed"
}

main "$@"
