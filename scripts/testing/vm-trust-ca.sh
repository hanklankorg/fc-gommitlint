#!/bin/sh
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# The sole privileged operation exposed to macOS TLS fixtures. The controller
# installs this root-owned helper only inside its disposable guest.
set -eu
[ "$#" -eq 0 ] && [ "$(id -u)" -eq 0 ] && [ "$(uname -s)" = Darwin ] || exit 2
case "$(sysctl -n hw.model)" in VirtualMac*) ;; *) exit 2 ;; esac
test -f /var/run/itiquette-vm/offline
umask 077
certificate=$(mktemp /private/tmp/itiquette-ca.XXXXXXXX)
trap 'rm -f "$certificate"' EXIT
head -c 65537 >"$certificate"
test "$(wc -c <"$certificate")" -le 65536
security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$certificate"
