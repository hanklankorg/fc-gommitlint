#!/bin/sh
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Install upstream Go toolchain, replacing the distro-provided version.
# Designed to be sourced by packaging build scripts (alpine-build.sh, etc.).
#
# Required environment variables:
#   GO_VERSION  - Go version to install (e.g., "1.26.1")
#
# Optional environment variables:
#   GO_DOWNLOAD_BASE - Base URL for Go downloads (default: https://dl.google.com/go)
#                      Enterprise users can point this to an internal mirror.
#   GO_INSTALL_DIR   - Where to extract Go (default: /usr/local)

set -eu

: "${GO_VERSION:?GO_VERSION not set}"

GO_DOWNLOAD_BASE="${GO_DOWNLOAD_BASE:-https://dl.google.com/go}"
GO_INSTALL_DIR="${GO_INSTALL_DIR:-/usr/local}"

# Detect host architecture and map to Go's naming convention
detect_goarch() {
  case "$(uname -m)" in
  x86_64) echo "amd64" ;;
  aarch64) echo "arm64" ;;
  armv7l) echo "armv6l" ;;
  s390x) echo "s390x" ;;
  ppc64le) echo "ppc64le" ;;
  riscv64) echo "riscv64" ;;
  *)
    echo "FATAL: unsupported architecture: $(uname -m)" >&2
    return 1
    ;;
  esac
}

# Build the download URL for a Go tarball or checksum
go_download_url() {
  echo "${GO_DOWNLOAD_BASE}/go${1}.${2}-${3}.tar.gz${4:-}"
}

GOARCH=$(detect_goarch)
TARBALL_URL=$(go_download_url "${GO_VERSION}" linux "${GOARCH}")
CHECKSUM_URL=$(go_download_url "${GO_VERSION}" linux "${GOARCH}" .sha256)
GO_TARBALL="/tmp/go${GO_VERSION}.linux-${GOARCH}.tar.gz"

echo "Downloading Go ${GO_VERSION} for linux/${GOARCH}..."
echo "  Tarball:  ${TARBALL_URL}"
echo "  Checksum: ${CHECKSUM_URL}"

curl --proto =https -fsSL "${TARBALL_URL}" -o "${GO_TARBALL}"
EXPECTED_SHA256=$(curl --proto =https -fsSL "${CHECKSUM_URL}")
ACTUAL_SHA256=$(sha256sum "${GO_TARBALL}" | awk '{print $1}')

if [ "${ACTUAL_SHA256}" != "${EXPECTED_SHA256}" ]; then
  echo "FATAL: Go tarball checksum mismatch!" >&2
  echo "  Expected: ${EXPECTED_SHA256}" >&2
  echo "  Actual:   ${ACTUAL_SHA256}" >&2
  exit 1
fi

tar -C "${GO_INSTALL_DIR}" -xzf "${GO_TARBALL}"
rm -f "${GO_TARBALL}"
ln -sf "${GO_INSTALL_DIR}/go/bin/go" /usr/bin/go
ln -sf "${GO_INSTALL_DIR}/go/bin/gofmt" /usr/bin/gofmt
export GOTOOLCHAIN=local
echo "Using Go: $(go version)"
