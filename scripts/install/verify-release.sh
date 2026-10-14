#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Verify gommitlint release artifacts.
# Downloads and verifies checksums and optionally a Cosign signature with an
# independently supplied public key.
#
# Usage: ./scripts/install/verify-release.sh <version> [--full]
#
# Examples:
#   ./scripts/install/verify-release.sh 0.9.12  # Integrity check only
#   # Use --full only when the release's tagged signer matches the trusted key.

set -euo pipefail

VERSION="${1:-}"
FULL_VERIFY="${2:-}"

if (($# > 2)); then
  echo "Usage: $0 <version> [--full]" >&2
  exit 1
fi

if [[ -z "$VERSION" ]]; then
  echo "Usage: $0 <version> [--full]" >&2
  echo "Example: $0 0.9.12" >&2
  exit 1
fi

if [[ ! "$VERSION" =~ ^[0-9]+[.][0-9]+[.][0-9]+$ ]]; then
  echo "Version must be MAJOR.MINOR.PATCH without a leading v" >&2
  exit 1
fi

if [[ -n "$FULL_VERIFY" && "$FULL_VERIFY" != "--full" ]]; then
  echo "Unknown option: $FULL_VERIFY" >&2
  exit 1
fi

# Detect architecture
MACHINE=$(uname -m)
case "$MACHINE" in
x86_64 | amd64) ARCH="x86_64" ;;
aarch64 | arm64) ARCH="arm64" ;;
riscv64) ARCH="riscv64" ;;
*)
  echo "Unsupported architecture: $MACHINE" >&2
  exit 1
  ;;
esac

SYSTEM=$(uname -s)
case "$SYSTEM" in
Linux) OS="linux" ;;
Darwin) OS="darwin" ;;
FreeBSD) OS="freebsd" ;;
NetBSD) OS="netbsd" ;;
OpenBSD) OS="openbsd" ;;
*)
  echo "Unsupported operating system: $SYSTEM" >&2
  exit 1
  ;;
esac

case "${OS}/${ARCH}" in
linux/x86_64 | linux/arm64 | linux/riscv64 | darwin/arm64 | \
  freebsd/x86_64 | freebsd/arm64 | netbsd/x86_64 | netbsd/arm64 | \
  openbsd/x86_64 | openbsd/arm64 | openbsd/riscv64) ;;
*)
  echo "No release archive is published for ${OS}/${ARCH}" >&2
  exit 1
  ;;
esac

BASE_URL="https://codefloe.com/itiquette/gommitlint/releases/download/v${VERSION}"
ARCHIVE="gommitlint-${VERSION}-${OS}-${ARCH}.tar.gz"
CHECKSUMS="gommitlint_${VERSION}_checksums.txt"

echo "=== Verifying gommitlint ${VERSION} for ${OS}/${ARCH} ==="
echo ""

# Download archive and checksums
echo "Downloading ${ARCHIVE}..."
curl -sSfL "${BASE_URL}/${ARCHIVE}" -o "${ARCHIVE}"

echo "Downloading checksums..."
curl -sSfL "${BASE_URL}/${CHECKSUMS}" -o "${CHECKSUMS}"

# Verify checksum
echo ""
echo "=== Checksum Verification ==="
archive_digest=""
while read -r digest filename; do
  if [[ "$filename" == "$ARCHIVE" ]]; then
    if [[ -n "$archive_digest" ]]; then
      echo "Duplicate checksum entry for ${ARCHIVE}" >&2
      exit 1
    fi
    if [[ ! "$digest" =~ ^[0-9a-f]{64}$ ]]; then
      echo "Malformed checksum entry for ${ARCHIVE}" >&2
      exit 1
    fi
    archive_digest=$digest
  fi
done <"${CHECKSUMS}"

if [[ -z "$archive_digest" ]]; then
  echo "Checksum manifest has no entry for ${ARCHIVE}" >&2
  exit 1
fi

if command -v sha256sum &>/dev/null; then
  actual_digest=$(sha256sum "${ARCHIVE}")
  actual_digest=${actual_digest%% *}
elif command -v shasum &>/dev/null; then
  actual_digest=$(shasum -a 256 "${ARCHIVE}")
  actual_digest=${actual_digest%% *}
elif command -v sha256 &>/dev/null; then
  actual_digest=$(sha256 -q "${ARCHIVE}")
else
  echo "No SHA-256 checksum tool found" >&2
  exit 1
fi

if [[ "$actual_digest" == "$archive_digest" ]]; then
  echo "✓ Checksum verified"
else
  echo "✗ Checksum verification failed" >&2
  exit 1
fi

# Full verification with cosign
if [[ "$FULL_VERIFY" == "--full" ]]; then
  echo ""
  echo "=== Cosign Signature Verification ==="

  if ! command -v cosign &>/dev/null; then
    echo "cosign not found. Install with: brew install cosign" >&2
    exit 1
  fi

  if [[ -z "${COSIGN_PUBLIC_KEY:-}" || ! -f "${COSIGN_PUBLIC_KEY}" ]]; then
    echo "COSIGN_PUBLIC_KEY must name an independently trusted public-key file" >&2
    exit 1
  fi

  echo "Downloading signature bundle..."
  curl -sSfL "${BASE_URL}/${CHECKSUMS}.bundle" -o "${CHECKSUMS}.bundle"

  if cosign verify-blob --key "${COSIGN_PUBLIC_KEY}" --bundle "${CHECKSUMS}.bundle" "${CHECKSUMS}" 2>/dev/null; then
    echo "✓ Cosign signature verified"
  else
    echo "✗ Cosign signature verification failed" >&2
    exit 1
  fi
else
  echo "WARNING: the checksum manifest was not signature-authenticated." >&2
  echo "Do not trust or run the archive unless you independently trust the release origin." >&2
fi

echo ""
if [[ "$FULL_VERIFY" == "--full" ]]; then
  echo "=== Extracting authenticated archive ==="
  tar -xzf "${ARCHIVE}"
  echo "✓ Extracted to current directory"
  echo ""
  echo "=== Authenticated verification complete ==="
  echo "Run: ./${ARCHIVE%.tar.gz}/gommitlint --version"
else
  echo "=== Integrity check complete (not release authentication) ==="
  echo "Authenticate before running: COSIGN_PUBLIC_KEY=/trusted/cosign.pub $0 ${VERSION} --full"
fi
