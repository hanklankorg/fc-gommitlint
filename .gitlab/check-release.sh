#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0

# Native release-candidate checks run in the non-root, key-free check job.
set -euo pipefail
go mod verify
go test -race -count=1 ./...
