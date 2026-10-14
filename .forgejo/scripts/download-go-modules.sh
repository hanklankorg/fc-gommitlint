#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0

set -euo pipefail

max_attempts=3

for ((attempt = 1; attempt <= max_attempts; attempt++)); do
  if GOTOOLCHAIN=local go mod download; then
    exit 0
  fi

  if ((attempt == max_attempts)); then
    printf 'ERROR: go mod download failed after %d attempts\n' "${max_attempts}" >&2
    exit 1
  fi

  printf 'go mod download attempt %d failed; retrying\n' "${attempt}" >&2
  sleep $((attempt * 2))
done
