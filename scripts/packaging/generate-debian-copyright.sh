#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Generate Debian machine-readable copyright file from LICENSES directory

set -euo pipefail

OUTPUT_FILE="debian/copyright"

# Function to convert license text to Debian format (prefix with space, empty lines become " .")
format_license() {
  sed 's/^$/ ./; s/^/ /'
}

cat >"$OUTPUT_FILE" <<'HEADER'
Format: https://www.debian.org/doc/packaging-manuals/copyright-format/1.0/
Upstream-Name: gommitlint
Upstream-Contact: Josef Andersson <janderssonse@proton.me>
Source: https://codefloe.com/itiquette/gommitlint
Comment: This package is REUSE compliant. See https://reuse.software/

Files: *
Copyright: 2024 The Gommitlint Authors
License: EUPL-1.2 or GPL-3.0+
Comment: Dual-licensed. Recipient may choose either the EUPL-1.2 or the GNU GPL version 3 or any later version.

Files: .forgejo/* scripts/* justfile .goreleaser.yaml development/golangci-lint/golangci.yml
Copyright: 2024 The Gommitlint Authors
License: CC0-1.0

HEADER

{
  # Add EUPL-1.2 full text
  echo "License: EUPL-1.2"
  format_license <"LICENSES/EUPL-1.2.txt"
  echo ""

  # Add GPL-3.0+ full text (Debian short form for GPL-3.0-or-later)
  echo "License: GPL-3.0+"
  format_license <"LICENSES/GPL-3.0-or-later.txt"
  echo ""

  # Add CC0-1.0 full text
  echo "License: CC0-1.0"
  format_license <"LICENSES/CC0-1.0.txt"
} >>"$OUTPUT_FILE"

echo "Generated ${OUTPUT_FILE}"
