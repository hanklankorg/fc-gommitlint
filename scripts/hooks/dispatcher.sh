#!/bin/sh
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later
#
# HOOK_TYPE hook dispatcher for gommitlint
# Runs all executable scripts in HOOK_TYPE.d/ directory
#
# This dispatcher pattern allows multiple tools to hook into the same
# Git hook phase. Scripts are executed in sorted order.
#
# NOTE: HOOK_TYPE will be replaced at runtime with the actual hook type
# (commit-msg, pre-push, etc.)

set -e

stdin_dir=""
stdin_file=""

# shellcheck disable=SC2329 # Invoked by the EXIT and signal traps below.
cleanup() {
  if [ -n "$stdin_dir" ]; then
    rm -rf "$stdin_dir"
  fi
}

trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# Determine script directory
script_dir="$(cd "$(dirname "$0")" >/dev/null 2>&1 && pwd -P)"
hooks_dir="$script_dir/HOOK_TYPE.d"

# Exit if plugin directory doesn't exist
if [ ! -d "$hooks_dir" ]; then
  exit 0
fi

# Git gives pre-push updates on stdin. Snapshot them once so every plugin sees
# the complete, byte-identical protocol stream instead of only the first plugin.
# shellcheck disable=SC2050 # HOOK_TYPE is replaced when the dispatcher is installed.
if [ "HOOK_TYPE" = "pre-push" ]; then
  umask 077

  tmp_root=${TMPDIR:-/tmp}
  case "$tmp_root" in
  /*) ;;
  *) tmp_root="./$tmp_root" ;;
  esac

  attempt=0
  while [ "$attempt" -lt 10 ]; do
    stdin_dir="$tmp_root/gommitlint-pre-push.$$.${attempt}"
    if mkdir "$stdin_dir" 2>/dev/null; then
      break
    fi

    stdin_dir=""
    attempt=$((attempt + 1))
  done

  if [ -z "$stdin_dir" ]; then
    printf "Hook dispatcher could not create secure pre-push input storage\n" >&2
    exit 1
  fi

  stdin_file="$stdin_dir/input"

  if ! cat >"$stdin_file"; then
    printf "Hook dispatcher could not capture pre-push input\n" >&2
    exit 1
  fi
fi

# Run all executable .sh files in sorted order
for script in "$hooks_dir"/*.sh; do
  # Skip if no .sh files exist (glob didn't match)
  [ -e "$script" ] || continue

  # Skip if not executable
  [ -x "$script" ] || continue

  # Run the script, replaying pre-push stdin and preserving its failure status.
  if [ -n "$stdin_file" ]; then
    if "$script" "$@" <"$stdin_file"; then
      status=0
    else
      status=$?
    fi
  elif "$script" "$@"; then
    status=0
  else
    status=$?
  fi

  if [ "$status" -ne 0 ]; then
    printf "Hook failed: %s (status %d)\n" "$(basename "$script")" "$status" >&2
    exit "$status"
  fi
done

exit 0
