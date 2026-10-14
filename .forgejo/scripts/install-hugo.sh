#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0
#
# Install a pinned Hugo (extended) binary with checksum verification.
#
# Hugo is downloaded by URL in the web workflows (pages-deploy, web-preview).
# This script is the single source of truth for the pinned version and digest
# so both workflows install the same verified binary. Ported from
# nanolinter's .forgejo/scripts/install-hugo.sh — keep the two in step when
# bumping.
#
# To bump Hugo:
#   1. Set HUGO_VERSION below.
#   2. curl -fsSL \
#        "https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_${HUGO_VERSION}_checksums.txt" \
#        | grep hugo_extended_${HUGO_VERSION}_linux-amd64.tar.gz
#      and copy the digest into HUGO_LINUX_AMD64_TAR_GZ_SHA256.

set -euo pipefail

HUGO_VERSION="0.160.1"
HUGO_LINUX_AMD64_TAR_GZ_SHA256="133adf4932c1b626b6fe6aa28d56791555abe3ceff167e03be534e8324c9ed39"

DEST="${1:-/usr/local/bin}"

for cmd in curl sha256sum tar install; do
  command -v "${cmd}" >/dev/null 2>&1 || {
    echo "ERROR: required command not found: ${cmd}" >&2
    exit 1
  }
done

archive="$(mktemp /tmp/hugo.XXXXXX.tar.gz)"
extract_dir="$(mktemp -d /tmp/hugo.XXXXXX)"
trap 'rm -rf "${archive}" "${extract_dir}"' EXIT

url="https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_extended_${HUGO_VERSION}_linux-amd64.tar.gz"

echo "Installing Hugo (extended) ${HUGO_VERSION}..."
curl -fSL --connect-timeout 15 --retry 3 --retry-delay 2 --retry-connrefused "${url}" -o "${archive}"
printf '%s  %s\n' "${HUGO_LINUX_AMD64_TAR_GZ_SHA256}" "${archive}" | sha256sum -c -
tar -xzf "${archive}" -C "${extract_dir}" hugo
install -m 0755 "${extract_dir}/hugo" "${DEST}/hugo"
"${DEST}/hugo" version
