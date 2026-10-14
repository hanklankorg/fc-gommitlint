#!/bin/bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# RPM build script (runs inside Fedora container)
# Called by: just package-native-rpm (via podman)

set -euo pipefail

: "${PKG_DIR:?PKG_DIR not set}"
: "${OUT_DIR:?OUT_DIR not set}"
: "${VERSION:?VERSION not set}"
: "${COMMIT:?COMMIT not set}"
: "${GO_VERSION:?GO_VERSION not set}"

# Install build dependencies (distro Go used only to bootstrap upstream Go)
dnf install -y rpm-build golang go-md2man gettext git bash curl

# Install upstream Go (distro version may be too old for go.mod requirement)
# install-go.sh is mounted into /work by the packaging container, so it is
# only visible at container runtime — disable shellcheck's static follow.
# shellcheck disable=SC1091
. /work/install-go.sh

# Setup rpmbuild tree
RPMBUILD_DIR="/root/rpmbuild"
mkdir -p "${RPMBUILD_DIR}"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Copy source tarball and spec
cp "${PKG_DIR}"/*.tar.gz "${RPMBUILD_DIR}/SOURCES/"
cp "${PKG_DIR}/gommitlint.spec" "${RPMBUILD_DIR}/SPECS/"

# Build RPM
rpmbuild -ba "${RPMBUILD_DIR}/SPECS/gommitlint.spec" \
  --define "version ${VERSION}" \
  --define "commit ${COMMIT}"

# Copy artifacts to output
mkdir -p "${OUT_DIR}"
find "${RPMBUILD_DIR}/RPMS" -name "*.rpm" -exec cp {} "${OUT_DIR}"/ \;
find "${RPMBUILD_DIR}/SRPMS" -name "*.rpm" -exec cp {} "${OUT_DIR}"/ \;
