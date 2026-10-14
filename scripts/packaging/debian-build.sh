#!/bin/bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Debian package build script (runs inside Debian container)
# Called by: just package-native-debian (via podman)

set -euo pipefail

: "${PKG_DIR:?PKG_DIR not set}"
: "${OUT_DIR:?OUT_DIR not set}"
: "${VERSION:?VERSION not set}"
: "${GO_VERSION:?GO_VERSION not set}"

# Install build dependencies (distro Go used only to bootstrap upstream Go)
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  dpkg-dev debhelper golang-go go-md2man gettext bash git ca-certificates curl

# Install upstream Go (distro version may be too old for go.mod requirement)
# install-go.sh is mounted into /work by the packaging container, so it is
# only visible at container runtime — disable shellcheck's static follow.
# shellcheck disable=SC1091
. /work/install-go.sh

# Extract source (tarball contains gommitlint/ prefix)
cd /tmp
tar xzf "${PKG_DIR}"/*.tar.gz
cd gommitlint

# Build binary package (unsigned, skip dep check since we installed deps above)
dpkg-buildpackage -us -uc -d -b

# Copy artifacts to output
mkdir -p "${OUT_DIR}"
cp /tmp/gommitlint*.deb "${OUT_DIR}"/ 2>/dev/null || true
cp /tmp/gommitlint*.changes "${OUT_DIR}"/ 2>/dev/null || true
cp /tmp/gommitlint*.buildinfo "${OUT_DIR}"/ 2>/dev/null || true
