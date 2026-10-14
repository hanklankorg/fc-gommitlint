#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Gommitlint cross-platform binary builder.
#
# This script mirrors gommitlint's build tags, PIE split, CPU baselines, and
# main-package metadata. It discovers the main package from these layouts:
#   - ./cmd/<binary-name>/     (standard multi-binary layout)
#   - ./cmd/<binary-name>/main.go  (single file in cmd)
#   - .                        (go.mod root as package)
#   - ./main.go                (simple single-file project)
# Usage: ./scripts/build/build-cross-platform.sh [goos] [goarch] [bin_dir] [binary_name] [main_path]
# Defaults: goos=linux, goarch=amd64, bin_dir=./bin, binary_name=<project>, main_path=auto-discovered

set -euo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly NC=$'\033[0m'

# Parse arguments with defaults
TARGET_OS="${1:-${GOOS:-linux}}"
TARGET_ARCH="${2:-${GOARCH:-amd64}}"
readonly BIN_DIR="${3:-${BIN_DIR:-./bin}}"
readonly BINARY_NAME="${4:-${BINARY_NAME:-$(basename "$(pwd)")}}"
readonly MAIN_PATH="${5:-${MAIN_PATH:-}}"

# Build configuration for pure-Go binaries on the selected OS and architecture.
# Supported targets depend on Go's static internal-linking and PIE matrices;
# callers must choose a target matching the destination system.
readonly BUILD_TAGS="netgo,osusergo" # Pure Go networking and user resolution (no glibc deps)
readonly BUILDMODE_DEFAULT="pie"     # Position Independent Executable for ASLR security
export CGO_ENABLED="0"               # Disable cgo; static linkage remains target-dependent

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

# Find the main package from gommitlint's supported repository layouts.
# Returns the first valid location found, preferring directories (packages) over files
find_main_package() {
  local binary_name="$1"
  local custom_path="$2"

  # If custom path provided, use it (allows override of auto-discovery)
  if [[ -n "$custom_path" ]]; then
    if [[ -f "$custom_path" ]] || [[ -d "$custom_path" ]]; then
      echo "$custom_path"
      return 0
    fi
  fi

  # Check common Go project layouts in order of preference
  # Directories are preferred over files to ensure all package files are included
  local locations=(
    "./cmd/${binary_name}"
    "cmd/${binary_name}"
    "."
    "./cmd/${binary_name}/main.go"
    "cmd/${binary_name}/main.go"
    "main.go"
  )

  for loc in "${locations[@]}"; do
    # Check directories first (for package builds)
    if [[ -d "$loc" ]]; then
      echo "$loc"
      return 0
    fi
  done

  # If no directory found, check for single file builds
  for loc in "${locations[@]}"; do
    if [[ -f "$loc" ]]; then
      echo "$loc"
      return 0
    fi
  done

  return 1
}

# Match the PIE/non-PIE split in .goreleaser.yaml.
get_build_mode() {
  local os="$1"
  local arch="$2"

  case "$os/$arch" in
  linux/amd64 | linux/arm64)
    echo "$BUILDMODE_DEFAULT"
    ;;
  *)
    echo "exe"
    ;;
  esac
}

# Generate build metadata. main.date is a unix epoch, the same shape as
# .goreleaser.yaml ({{ .CommitTimestamp }}) and release.yml (git log %ct).
generate_build_metadata() {
  local version commit build_date

  # Try git for version info
  if command -v git >/dev/null 2>&1 && git rev-parse --git-dir >/dev/null 2>&1; then
    version=$(git describe --tags --dirty --always 2>/dev/null || echo "dev")
    commit=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
    build_date=${SOURCE_DATE_EPOCH:-$(git log -1 --format=%ct 2>/dev/null || date -u +%s)}
  else
    version="dev"
    commit="unknown"
    build_date=${SOURCE_DATE_EPOCH:-$(date -u +%s)}
  fi

  echo "$version|$commit|$build_date"
}

# Execute build
execute_build() {
  local output_binary="$BIN_DIR/$BINARY_NAME-$TARGET_OS-$TARGET_ARCH"
  [[ "$TARGET_OS" == "windows" ]] && output_binary="${output_binary}.exe"

  local main_pkg
  main_pkg=$(find_main_package "$BINARY_NAME" "$MAIN_PATH") || {
    fail "Cannot find main package for $BINARY_NAME"
  }

  local buildmode
  buildmode=$(get_build_mode "$TARGET_OS" "$TARGET_ARCH")

  # Get metadata
  IFS='|' read -r version commit build_date <<<"$(generate_build_metadata)"

  # Inject gommitlint's main-package version metadata.
  local ldflags="-w -buildid=" # -w strips debug info, -buildid= makes builds reproducible
  ldflags="$ldflags -X main.version=$version"
  ldflags="$ldflags -X main.commit=$commit"
  ldflags="$ldflags -X main.date=$build_date"

  log "Building $BINARY_NAME for $TARGET_OS/$TARGET_ARCH"
  log "Main package: $main_pkg"
  log "Build tags: $BUILD_TAGS"
  log "Output: $output_binary"

  # Apply CPU optimizations
  case "$TARGET_ARCH" in
  amd64)
    export GOAMD64="${GOAMD64:-v2}"
    log "AMD64 optimization: $GOAMD64"
    ;;
  arm64)
    export GOARM64="${GOARM64:-v8.0}"
    log "ARM64 optimization: $GOARM64"
    ;;
  esac

  # Build
  GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED="$CGO_ENABLED" \
    go build \
    -buildmode="$buildmode" \
    -trimpath \
    -buildvcs=false \
    -tags="$BUILD_TAGS" \
    -ldflags="$ldflags" \
    -o "$output_binary" \
    "$main_pkg" || fail "Build failed"

  success "Built: $(basename "$output_binary")"
}

main() {
  printf '== Build: product=%s | host=%s/%s | target=%s/%s | output=%s\n' \
    "$BINARY_NAME" "$(uname -s)" "$(uname -m)" "$TARGET_OS" "$TARGET_ARCH" "$BIN_DIR" >&2

  # Check Go is available
  command -v go >/dev/null 2>&1 || fail "Go not found"

  # Create output directory
  mkdir -p "$BIN_DIR"

  # Execute build
  execute_build

  success "Build completed"
}

main "$@"
