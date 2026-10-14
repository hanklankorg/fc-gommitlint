#!/bin/sh
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Initial package setup in a freshly created guest. Never run this on a host.
set -eu
[ "$(id -u)" = 0 ] && [ "$0" = /var/run/itiquette-vm/bootstrap.sh ] || exit 2
[ "$(uname -s)" = "$1" ] || exit 2
project=$2
case "$project" in gommitlint | nanolinter) ;; *) exit 2 ;; esac
export PATH=/opt/homebrew/bin:/usr/local/bin:/usr/pkg/bin:/usr/pkg/sbin:/usr/bin:/bin:/usr/sbin:/sbin

case "$1" in
FreeBSD)
  export ASSUME_ALWAYS_YES=yes
  pkg bootstrap -y
  pkg install -y bash sudo git jq coreutils findutils gsed gtar curl gnupg gettext-tools gmake pkgconf
  if [ "$project" = nanolinter ]; then pkg install -y rust gettext-runtime; fi
  # Subsequent disposable runs keep this prepared identity without a seed CD.
  sysrc nuageinit_enable=NO
  ;;
OpenBSD)
  printf 'https://cdn.openbsd.org/pub/OpenBSD\n' >/etc/installurl
  # Select the unflavoured packages explicitly; ambiguous names may be skipped.
  pkg_add -I bash sudo-- git jq coreutils findutils gsed gtar-- curl gnupg gettext-tools gmake
  if [ "$project" = nanolinter ]; then pkg_add rust; fi
  ;;
NetBSD)
  # sshd may start before certctl_init. Wait for rc's completion marker rather
  # than racing a second rehash against the image's first-boot trust setup.
  printf 'Waiting for NetBSD boot tasks (limit 3m).\n'
  attempt=0
  until grep -Fq '[/etc/rc exiting with status 0]' /var/run/rc.log 2>/dev/null; do
    [ "$attempt" -lt 180 ] || {
      printf 'NetBSD boot did not finish; inspect /var/run/rc.log.\n' >&2
      exit 2
    }
    sleep 1
    attempt=$((attempt + 1))
  done
  # Materialize the trust anchors shipped in the verified image before the
  # first HTTPS package request; SSH readiness alone does not establish this.
  printf 'Preparing NetBSD TLS trust anchors.\n'
  certctl rehash
  machine=$(uname -p)
  export PKG_PATH="https://cdn.NetBSD.org/pub/pkgsrc/packages/NetBSD/$machine/11.0/All"
  pkg_add -v bash sudo git-base jq coreutils findutils gsed gtar curl gnupg2 gettext-tools gmake pkgconf
  if [ "$project" = nanolinter ]; then pkg_add -v rust; fi
  mkdir -p /root/.ssh
  chmod 0700 /root/.ssh
  cp /home/opc/.ssh/authorized_keys /root/.ssh/authorized_keys
  chmod 0600 /root/.ssh/authorized_keys
  sed '/^[[:space:]]*PermitRootLogin[[:space:]]/d; /^[[:space:]]*PasswordAuthentication[[:space:]]/d; /^[[:space:]]*KbdInteractiveAuthentication[[:space:]]/d' \
    /etc/ssh/sshd_config >/etc/ssh/sshd_config.itiquette
  printf '\nPermitRootLogin prohibit-password\nPasswordAuthentication no\nKbdInteractiveAuthentication no\n' >>/etc/ssh/sshd_config.itiquette
  mv /etc/ssh/sshd_config.itiquette /etc/ssh/sshd_config
  printf '\nec2_init=NO\n' >>/etc/rc.conf
  # The amd64 live image initially boots on VGA. Subsequent prepared boots
  # should report their kernel/startup progress through the controller console.
  if [ "$(uname -m)" = amd64 ]; then
    if [ -f /boot.cfg ]; then sed '/^consdev=/d' /boot.cfg >/boot.cfg.itiquette; else : >/boot.cfg.itiquette; fi
    printf 'consdev=com0,115200\n' >>/boot.cfg.itiquette
    mv /boot.cfg.itiquette /boot.cfg
  fi
  /etc/rc.d/sshd reload
  ;;
Darwin)
  # The upstream Tart base image provides Homebrew and the admin control user.
  sudo -H -u admin /opt/homebrew/bin/brew install bash git jq coreutils findutils gnu-sed gnu-tar gettext gnupg pkgconf
  if [ "$project" = nanolinter ]; then sudo -H -u admin /opt/homebrew/bin/brew install rustup; fi
  ;;
Linux)
  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y bash sudo git jq build-essential pkg-config gettext curl gnupg \
    openssh-client locales podman uidmap slirp4netns fuse-overlayfs skopeo
  if [ "$project" = nanolinter ]; then apt-get install -y rustup; fi
  locale-gen en_US.UTF-8 sv_SE.UTF-8
  # Pinned bootstrap tool; the project locks the tools subsequently managed by mise.
  curl --fail --location --proto '=https' --proto-redir '=https' --retry 2 --max-time 300 \
    -o /var/run/itiquette-vm/mise \
    https://github.com/jdx/mise/releases/download/v2026.10.1/mise-v2026.10.1-linux-arm64
  printf 'd4785456a86d1c836f09ccfe00aa6b044b18d29d79aae46a1820c96024cf9c38  /var/run/itiquette-vm/mise\n' | sha256sum -c -
  install -m 0755 /var/run/itiquette-vm/mise /usr/local/bin/mise
  rm -f /var/run/itiquette-vm/mise
  ;;
*) exit 2 ;;
esac
for tool in bash sudo git jq curl; do
  command -v "$tool" >/dev/null 2>&1 || {
    printf 'Guest package setup did not provide required tool: %s\n' "$tool" >&2
    exit 2
  }
done
printf 'Guest base tools installed for %s on %s.\n' "$project" "$1"
