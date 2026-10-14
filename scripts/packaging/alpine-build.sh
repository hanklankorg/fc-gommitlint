#!/bin/sh
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Alpine APK build script (runs inside Alpine container)
# Called by: just package-native-alpine (via podman)

set -eu

: "${PKG_DIR:?PKG_DIR not set}"
: "${KEYS_DIR:?KEYS_DIR not set}"
: "${OUT_DIR:?OUT_DIR not set}"
: "${GO_VERSION:?GO_VERSION not set}"

# Install build dependencies (bash needed by scripts/docs/*.sh, ssh-keygen by
# the SSH signature test in the package check)
apk add --no-cache alpine-sdk go go-md2man gettext sudo bash curl openssh-keygen

# Install upstream Go (distro version may be too old for go.mod requirement)
# Replace system go with upstream so abuild (which runs as a separate user
# with a login shell that resets PATH) picks it up automatically.
# install-go.sh is mounted into /work by the packaging container, so it is
# only visible at container runtime — disable shellcheck's static follow.
# shellcheck disable=SC1091
. /work/install-go.sh

# Create non-root builder user (abuild requires this)
if ! id builder >/dev/null 2>&1; then
  adduser -D builder
fi

addgroup builder abuild >/dev/null 2>&1 || true

# Allow passwordless sudo for specific commands only (abuild needs to install signing keys)
if ! grep -q "^builder ALL=(ALL) NOPASSWD: /bin/cp" /etc/sudoers 2>/dev/null; then
  echo "builder ALL=(ALL) NOPASSWD: /bin/cp" >>/etc/sudoers
fi

# Pre-seed distfiles cache with local source tarball (avoids download from upstream)
mkdir -p /var/cache/distfiles
for f in "$PKG_DIR"/*.tar.gz; do
  [ -e "$f" ] && cp "$f" /var/cache/distfiles/
done
chown -R builder:builder /var/cache/distfiles

# Grant builder access to mounted directories
chown -R builder:builder "$PKG_DIR" "$KEYS_DIR"

# Run build as builder user
BUILD_RC=0
su - builder <<BUILDSCRIPT || BUILD_RC=$?
set -eu

mkdir -p "\$HOME/.abuild"

# Create the signing key once and persist it to the mounted directory. The
# key is usable only when abuild.conf names a private key that is present: a
# directory left with a public key or a conf but no private key cannot sign,
# so a new pair is generated and its conf replaces the stale one. Generation
# runs before anything is linked, so the copies never meet their own source.
conf_key=\$(sed -n 's/^PACKAGER_PRIVKEY="\(.*\)"\$/\1/p' "$KEYS_DIR/abuild.conf" 2>/dev/null || true)
if [ -z "\$conf_key" ] || [ ! -f "$KEYS_DIR/\$(basename "\$conf_key")" ]; then
    abuild-keygen -a -n
    cp "\$HOME/.abuild"/*.rsa "\$HOME/.abuild"/*.rsa.pub "\$HOME/.abuild/abuild.conf" "$KEYS_DIR"/
fi

# Use the persisted keys and conf.
for f in "$KEYS_DIR"/*; do
    [ -e "\$f" ] && ln -snf "\$f" "\$HOME/.abuild/\$(basename "\$f")"
done

# Install the public key for package verification
sudo cp "$KEYS_DIR"/*.rsa.pub /etc/apk/keys/

cd "$PKG_DIR"
abuild checksum
abuild -r
BUILDSCRIPT

# Copy packages as root (container root maps to host user, avoiding UID namespace issues)
mkdir -p "$OUT_DIR"
if [ -d /home/builder/packages ]; then
  find /home/builder/packages -name "*.apk" -type f -exec cp {} "$OUT_DIR"/ \;
fi

# Restore root ownership on mounted PKG_DIR so host user can traverse it
# (was chowned to builder earlier, which maps to a sub-UID in rootless podman)
chown -R root:root "$PKG_DIR"

# Propagate build failure
if [ "$BUILD_RC" -ne 0 ]; then
  echo "Build failed with exit code: $BUILD_RC" >&2
  exit "$BUILD_RC"
fi
