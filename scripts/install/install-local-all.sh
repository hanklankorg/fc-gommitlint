#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Installs binary, documentation, completions, and translations
# Usage: ./scripts/install/install-local-all.sh [--brief] [bin_dir] [executable_name]
# Dependencies: install-binary.sh, manpage.sh, and the installed gommitlint binary

set -euo pipefail

# Colors for output
readonly RED=$'\033[0;31m'
readonly GREEN=$'\033[0;32m'
readonly YELLOW=$'\033[1;33m'
readonly CYAN=$'\033[1;36m'
readonly NC=$'\033[0m' # No Color

readonly PROJECT_MARKERS=("go.mod" "cmd/gommitlint/main.go" ".gommitlint.yaml")

# Parse arguments
BRIEF_MODE=false
BIN_DIR=""
EXECUTABLE=""

while [[ $# -gt 0 ]]; do
  case $1 in
  --brief)
    BRIEF_MODE=true
    shift
    ;;
  *)
    if [[ -z "$BIN_DIR" ]]; then
      BIN_DIR="$1"
    elif [[ -z "$EXECUTABLE" ]]; then
      EXECUTABLE="$1"
    fi
    shift
    ;;
  esac
done

# Set defaults
BIN_DIR="${BIN_DIR:-./bin}"
EXECUTABLE="${EXECUTABLE:-gommitlint}"

# Resolved the way install-binary.sh does, so messages name the real destination.
export INSTALL_DIR="${INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"

log() {
  if [[ "$BRIEF_MODE" != "true" ]]; then
    printf "${YELLOW}▸${NC} %s\n" "$1"
  fi
}

success() {
  printf "${GREEN}✓${NC} %s\n" "$1"
}

fail() {
  printf "${RED}✗${NC} %s\n" "$1" >&2
}

# Validates that required project marker files exist in current directory
validate_project_directory() {
  local missing=()
  for marker in "${PROJECT_MARKERS[@]}"; do
    if [[ ! -f "$marker" ]]; then
      missing+=("$marker")
    fi
  done
  if ((${#missing[@]} > 0)); then
    fail "Missing project markers: ${missing[*]}"
    return 1
  fi
  return 0
}

# Installs binary component using dedicated script
install_binary_component() {
  log "Installing binary..."
  if _binary_output=$(./scripts/install/install-binary.sh "$BIN_DIR" "$EXECUTABLE" 2>&1); then
    printf "✓ Binary installed: %s/%s\n" "$INSTALL_DIR" "$EXECUTABLE"
    return 0
  else
    printf "✗ Binary installation failed\n"
    if [[ "$BRIEF_MODE" != "true" ]]; then
      fail "Binary installation failed"
    fi
    return 1
  fi
}

# Generates and installs man page component
install_manpage_component() {
  log "Generating man page..."
  if _manpage_output=$(./scripts/docs/manpage.sh 2>&1); then
    log "Installing man page..."
    if mkdir -p ~/.local/share/man/man1; then
      if cp "generated/manpages/$EXECUTABLE.1.gz" ~/.local/share/man/man1/; then
        local locale_dir locale dest_dir has_locales=false
        for locale_dir in generated/manpages/*/man1; do
          if [[ ! -d "$locale_dir" ]]; then
            continue
          fi

          locale=$(basename "$(dirname "$locale_dir")")
          dest_dir="$HOME/.local/share/man/$locale/man1"
          mkdir -p "$dest_dir"

          if [[ -f "$locale_dir/$EXECUTABLE.1.gz" ]]; then
            cp "$locale_dir/$EXECUTABLE.1.gz" "$dest_dir/"
            has_locales=true
          fi
        done

        if [[ "$has_locales" == "true" ]]; then
          printf "✓ Man pages installed: ~/.local/share/man/{man1,<lang>/man1}/%s.1.gz\n" "$EXECUTABLE"
        else
          printf "✓ Man page installed: ~/.local/share/man/man1/%s.1.gz\n" "$EXECUTABLE"
        fi
        return 0
      else
        printf "✗ Man page copy failed\n"
        return 1
      fi
    else
      printf "✗ Man page directory creation failed\n"
      return 1
    fi
  else
    printf "✗ Man page generation failed\n"
    return 1
  fi
}

# Installs shell completions through the product-owned installer
install_completions_component() {
  log "Installing completions..."
  local installed_binary="$INSTALL_DIR/$EXECUTABLE"

  if _completions_output=$("$installed_binary" completion --all 2>&1); then
    printf "✓ Completions installed via %s completion --all\n" "$EXECUTABLE"
    return 0
  else
    printf "✗ Completion installation failed\n"
    return 1
  fi
}

# Generates and installs translation catalogs component
install_translations_component() {
  log "Generating translations..."
  if _translations_output=$(go run ./cmd/gommitlint translations compile 2>&1); then
    log "Installing translations..."
    local locale_dir locale dest_dir has_locales=false
    for locale_dir in generated/locale/*/LC_MESSAGES; do
      if [[ ! -d "$locale_dir" ]]; then
        continue
      fi

      locale=$(basename "$(dirname "$locale_dir")")
      dest_dir="$HOME/.local/share/locale/$locale/LC_MESSAGES"
      mkdir -p "$dest_dir"

      if [[ -f "$locale_dir/$EXECUTABLE.mo" ]]; then
        cp "$locale_dir/$EXECUTABLE.mo" "$dest_dir/"
        has_locales=true
      fi
    done

    if [[ "$has_locales" == "true" ]]; then
      printf "✓ Translations installed: ~/.local/share/locale/<lang>/LC_MESSAGES/%s.mo\n" "$EXECUTABLE"
    else
      printf "! No translation catalogs installed\n"
    fi
    return 0
  else
    printf "✗ Translation generation failed\n"
    return 1
  fi
}

# Displays unified installation summary with next steps
display_summary() {
  local results=("$@")
  local success_count=0
  local has_failures=false

  if [[ "$BRIEF_MODE" != "true" ]]; then
    printf "\n%s→%s Installation Summary:\n\n" "$CYAN" "$NC"
  fi

  # Show results and count successes
  for result in "${results[@]}"; do
    if [[ "$BRIEF_MODE" != "true" ]]; then
      # Handle multi-line results properly
      while IFS= read -r line; do
        printf "  %s\n" "$line"
        # Count first line that starts with ✓ as success
        if [[ "$line" == ✓* ]]; then
          ((success_count++)) || true
        elif [[ "$line" == ✗* ]]; then
          has_failures=true
        fi
      done <<<"$result"
    else
      # For brief mode, just count successes from first lines
      local first_line
      first_line=$(echo "$result" | head -1)
      if [[ "$first_line" == ✓* ]]; then
        ((success_count++)) || true
      elif [[ "$first_line" == ✗* ]]; then
        has_failures=true
      fi
    fi
  done

  if [[ "$BRIEF_MODE" != "true" ]]; then
    printf "\n"
  fi

  # Display final status and next steps
  if [[ $success_count -gt 0 ]]; then
    if [[ "$BRIEF_MODE" != "true" ]]; then
      success "Installation completed with $success_count/4 components successful"
      printf "\n%sNext steps:%s\n" "$YELLOW" "$NC"
      printf "  • Add to PATH: %sexport PATH=\"%s:\$PATH\"%s\n" "$GREEN" "$INSTALL_DIR" "$NC"
      printf "  • Test binary: %s%s --version%s\n" "$GREEN" "$EXECUTABLE" "$NC"
      printf "  • View manual: %sman %s%s\n" "$GREEN" "$EXECUTABLE" "$NC"
      printf "  • Restart shell to activate completions\n"
    else
      printf "INSTALL_STATUS=success:%s/4_components\n" "$success_count"
    fi

    if [[ "$has_failures" == "true" ]]; then
      return 1
    fi
    return 0
  else
    if [[ "$BRIEF_MODE" != "true" ]]; then
      fail "Installation failed - no components were successfully installed"
    else
      printf "INSTALL_STATUS=failed:0/4_components\n"
    fi
    return 1
  fi
}

main() {
  if [[ "$BRIEF_MODE" != "true" ]]; then
    printf "%s→%s Installing complete local development environment...\n\n" "$CYAN" "$NC"
  fi

  # Validate environment
  validate_project_directory || exit 1

  # Collect results from each component
  declare -a results

  # Install binary component
  if binary_result=$(install_binary_component); then
    results+=("$binary_result")
  else
    results+=("$binary_result")
    exit 1 # Binary is critical - fail immediately
  fi

  # Install man page component
  if manpage_result=$(install_manpage_component); then
    results+=("$manpage_result")
  else
    results+=("$manpage_result")
  fi

  # Install completions component
  if completions_result=$(install_completions_component); then
    results+=("$completions_result")
  else
    results+=("$completions_result")
  fi

  # Install translations component
  if translations_result=$(install_translations_component); then
    results+=("$translations_result")
  else
    results+=("$translations_result")
  fi

  # Display unified summary
  if ! display_summary "${results[@]}"; then
    exit 1
  fi
}

main "$@"
