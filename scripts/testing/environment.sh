#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Shared launcher contract, kept identical in gommitlint and nanolinter.
# Selection comes from the recipe; detection only validates that selection.
# Compatible with Bash 3.2 so unsupported hosts receive a useful refusal.
set -euo pipefail

fail() {
  printf 'test environment: %s\n' "$*" >&2
  exit 2
}

check_container() {
  local system=$1 architecture=$2 marker
  [[ "$system" == Linux ]] || fail 'this Podman test launcher needs a Linux controller; select a Linux VM explicitly, never substitute Linux for native macOS/BSD tests'
  [[ "$architecture" == amd64 || "$architecture" == arm64 ]] || fail "container tests do not support $architecture"
  ((BASH_VERSINFO[0] >= 4)) || fail 'the existing Linux container launchers require Bash 4 or newer'
  for marker in CI CONTINUOUS_INTEGRATION GITHUB_ACTIONS GITEA_ACTIONS FORGEJO_ACTIONS GITLAB_CI \
    BUILDKITE CIRCLECI JENKINS_URL TEAMCITY_VERSION TF_BUILD DRONE WOODPECKER CODEBUILD_BUILD_ID TRAVIS; do
    if declare -p "$marker" >/dev/null 2>&1; then
      fail "local container tests refuse CI marker $marker"
    fi
  done
  ((EUID != 0)) || fail 'container tests require an unprivileged user'
  command -v podman >/dev/null 2>&1 || fail 'podman is not installed on this Linux controller. Install and configure rootless Podman, then prepare the test images; see docs/testing/test-environments.adoc.'
}

main() {
  local mode=${1:-} suite='' system machine architecture context=current-environment
  case "$mode" in
  native | container) ;;
  *) fail 'expected native or container' ;;
  esac
  shift
  if [[ "${1:-}" == --suite ]]; then
    (($# >= 2)) && [[ -n "$2" && "$2" != -- ]] || fail 'expected a suite label after --suite'
    suite=$2
    shift 2
  fi
  if (($# > 0)); then
    [[ "$1" == -- ]] || fail 'expected -- before the command'
    shift
    (($# > 0)) || fail 'missing command after --'
  fi

  system=$(uname -s)
  machine=$(uname -m)
  case "$system" in
  Linux | Darwin | FreeBSD | OpenBSD | NetBSD) ;;
  *) fail "unsupported operating system: $system" ;;
  esac
  case "$machine" in
  x86_64 | amd64) architecture=amd64 ;;
  aarch64 | arm64) architecture=arm64 ;;
  riscv64) architecture=riscv64 ;;
  *) fail "unsupported architecture: $machine" ;;
  esac

  if [[ -f /run/.containerenv || -f /.dockerenv ]]; then
    context=container
  fi
  if [[ -n "$suite" ]]; then
    printf '== Test job: suites=%s | environment=%s | host=%s/%s | context=%s\n' "$suite" "$mode" "$system" "$architecture" "$context" >&2
  else
    printf 'test environment: selected=%s current=%s/%s context=%s\n' "$mode" "$system" "$architecture" "$context" >&2
  fi
  if [[ "$mode" == container ]]; then
    check_container "$system" "$architecture"
  fi

  # The project launcher owns image, rootless, resource and isolation checks.
  if (($# > 0)); then
    exec "$@"
  fi
}

main "$@"
