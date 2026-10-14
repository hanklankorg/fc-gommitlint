#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Updates the repository's Go-version authorities to the latest stable release.
# Usage: ./scripts/dev/upgrade-go-version.sh [--check]
# Dependencies: curl, jq, sed, grep, skopeo, and mise
# Output: Updated module, tool, script, workflow, container, and AsciiDoc pins

set -euo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly CYAN=$'\033[0;36m'
readonly NC=$'\033[0m' # No Color

# Logging utilities
log() { printf "%s\n" "$1"; }
success() { printf "${GREEN}✓${NC} %s\n" "$1"; }
fail() { printf "${RED}✗${NC} %s\n" "$1" >&2; }
info() { printf "${CYAN}→${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}!${NC} %s\n" "$1"; }

# Get the latest stable Go version from official API
get_latest_go_version() {
  local version
  version=$(curl -sL "https://go.dev/dl/?mode=json" | jq -r '.[0].version' | sed 's/^go//')

  if [[ -z "$version" ]]; then
    fail "Failed to fetch latest Go version"
    exit 1
  fi

  echo "$version"
}

# Get current Go version from go.mod
get_current_go_version() {
  if [[ ! -f "go.mod" ]]; then
    fail "go.mod not found"
    exit 1
  fi

  sed -n 's/^go //p' go.mod
}

# Verify that the explicit Go-version authorities agree with go.mod.
check_version_consistency() {
  local expected=$1
  local file expected_text

  while IFS='|' read -r file expected_text; do
    if [[ ! -f "$file" ]]; then
      fail "Version authority not found: $file"
      return 1
    fi

    if ! grep -Fq "$expected_text" "$file"; then
      fail "$file does not contain the expected Go version: $expected_text"
      return 1
    fi
  done <<EOF
tools/go.mod|go ${expected}
.mise.toml|go = "${expected}"
scripts/dev/install-go-dev-tools.sh|local minimum_version=${expected}
docs/modules/ROOT/pages/architecture.adoc|Go ${expected}+
Containerfile.sandbox|golang:${expected}-
EOF

  success "Go version authorities agree on ${expected}"
}

# Apply one sed expression to a file and report only actual changes.
update_file() {
  local file=$1
  local pattern=$2

  if [[ ! -f "$file" ]]; then
    warn "File not found: $file (skipping)"
    return
  fi

  local temp_file
  temp_file=$(mktemp)

  if sed "$pattern" "$file" >"$temp_file"; then
    if ! cmp -s "$file" "$temp_file"; then
      cp "$temp_file" "$file"
      success "Updated $file"
    fi
  else
    rm -f "$temp_file"
    fail "Failed to update $file"
    return 1
  fi

  rm -f "$temp_file"
}

main() {
  if (($# > 1)); then
    fail "Usage: $0 [--check]"
    return 1
  fi

  if [[ ${1:-} == "--check" ]]; then
    check_version_consistency "$(get_current_go_version)"
    return
  fi

  if (($# == 1)); then
    fail "Unknown option: $1"
    return 1
  fi

  command -v mise >/dev/null 2>&1 || {
    fail "mise is needed to update the locked Go toolchain"
    return 1
  }

  info "Checking for latest stable Go version..."

  # Get versions
  local current_version
  current_version=$(get_current_go_version)

  local latest_version
  latest_version=$(get_latest_go_version)

  log ""
  info "Current Go version: ${current_version}"
  info "Latest Go version: ${latest_version}"

  local current_normalized="${current_version}"
  if [[ ! "$current_version" =~ \. ]]; then
    current_normalized="${current_version}.0"
  elif [[ "$current_version" =~ ^[0-9]+\.[0-9]+$ ]]; then
    current_normalized="${current_version}.0"
  fi

  if [[ "$current_normalized" == "$latest_version" ]]; then
    check_version_consistency "$current_version"
    success "Already using the latest Go version"
    exit 0
  fi

  log ""
  info "Updating Go version from ${current_version} to ${latest_version}..."
  log ""

  # Update go.mod files - use full version with patch
  info "Updating go.mod files..."
  update_file "go.mod" "s/^go .*/go ${latest_version}/"
  update_file "tools/go.mod" "s/^go .*/go ${latest_version}/"
  update_file ".mise.toml" "s/^go = \"[0-9.]*\"/go = \"${latest_version}\"/"
  update_file "scripts/dev/install-go-dev-tools.sh" \
    "s/^  local minimum_version=.*/  local minimum_version=${latest_version}/"

  # Update Forgejo workflow files
  info "Updating Forgejo workflow files..."
  for workflow in .forgejo/workflows/*.yml .forgejo/workflows/*.yaml; do
    if [[ -f "$workflow" ]]; then
      update_file "$workflow" \
        "s/go-version: ['\"]\\?[0-9]\\+\\.[0-9]\\+\\(\\.[0-9]\\+\\)\\?['\"]\\?/go-version: '${latest_version}'/g"
    fi
  done

  # Update maintained AsciiDoc sources; generated manpages and Markdown are not authorities.
  info "Updating AsciiDoc version references..."

  local current_major_minor
  current_major_minor=$(echo "$current_version" | cut -d. -f1,2)
  local current_pattern=${current_version//./\\.}
  local current_major_minor_pattern=${current_major_minor//./\\.}

  shopt -s globstar nullglob
  for doc in README.adoc docs/**/*.adoc; do
    update_file "$doc" \
      "s/Go ${current_pattern}/Go ${latest_version}/g; s/go${current_pattern}/go${latest_version}/g; s/Go ${current_major_minor_pattern}\\([^0-9.]\\|$\\)/Go ${latest_version}\\1/g; s/go${current_major_minor_pattern}\\([^0-9.]\\|$\\)/go${latest_version}\\1/g"
  done

  # The sandbox image pins the Go toolchain image by tag and digest. A new tag
  # needs its own digest, so both are resolved together; the tiers run with
  # GOTOOLCHAIN=local and would fail on a toolchain older than go.mod's.
  info "Updating the sandbox Go image..."
  local sandbox_file=Containerfile.sandbox
  local sandbox_pin sandbox_suffix sandbox_digest
  sandbox_pin=$(sed -n 's/^ARG BASE_IMAGE=\(docker\.io\/library\/golang:[^@]*\)@sha256:[a-f0-9]\{64\}$/\1/p' "$sandbox_file")
  if [[ -z "$sandbox_pin" ]]; then
    fail "$sandbox_file has no digest-pinned golang ARG BASE_IMAGE line"
    return 1
  fi
  sandbox_suffix=${sandbox_pin#*:"${current_version}"}
  if [[ "$sandbox_suffix" == "$sandbox_pin" ]]; then
    fail "$sandbox_file pins ${sandbox_pin#*:}, not Go ${current_version}"
    return 1
  fi
  command -v skopeo >/dev/null 2>&1 || {
    fail "skopeo is needed to resolve the digest of golang:${latest_version}${sandbox_suffix}"
    return 1
  }
  sandbox_digest=$(skopeo inspect --format '{{.Digest}}' "docker://docker.io/library/golang:${latest_version}${sandbox_suffix}") || {
    fail "docker.io/library/golang:${latest_version}${sandbox_suffix} is not published yet"
    return 1
  }
  update_file "$sandbox_file" \
    "s|^ARG BASE_IMAGE=docker\.io/library/golang:.*\$|ARG BASE_IMAGE=docker.io/library/golang:${latest_version}${sandbox_suffix}@${sandbox_digest}|"
  update_file "$sandbox_file" "s/${current_pattern}/${latest_version}/g"
  warn "Rebuild the sandbox image: just test-container-prepare"

  # Update .tool-versions if it exists (asdf)
  if [[ -f ".tool-versions" ]]; then
    info "Updating .tool-versions..."
    update_file ".tool-versions" \
      "s/golang [0-9]\\+\\.[0-9]\\+\\.[0-9]\\+/golang ${latest_version}/g"
  fi

  # The release workflow installs with mise-locked=true. Refresh the Go lock
  # together with its specification, and run tidy with that exact toolchain.
  info "Locking and installing the selected Go toolchain..."
  mise lock go
  mise install --locked go

  # Run go mod tidy on both modules
  info "Running go mod tidy..."
  mise exec -- go mod tidy
  success "Main module tidied"

  if [[ -d "tools" ]]; then
    (cd tools && mise exec -- go mod tidy)
    success "Tools module tidied"
  fi

  check_version_consistency "$latest_version"

  log ""
  success "Go version updated successfully!"
  info "Updated from ${current_version} to ${latest_version}"
  log ""
  warn "Please review the changes and test the build before committing"
  info "Recommended next steps:"
  log "  1. Run 'just test' to verify tests pass"
  log "  2. Run 'just lint' to check for any issues"
  log "  3. Commit the changes with: git commit -m \"build: upgrade Go to ${latest_version}\""
}

main "$@"
