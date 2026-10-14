#!/bin/sh
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Installs, from the distribution's own repositories, what the integration and
# black-box suites ask of their environment -- the same list
# Containerfile.sandbox carries, with the test that needs each tool cited
# there -- and generates the sv_SE.UTF-8 locale the locale row sets. Runs as
# root during the image build, the only step of the matrix with a network.
#
# git, GnuPG and OpenSSH are the point: they are the distribution's packages,
# at the distribution's versions, so the matrix tests gommitlint against what
# a user of that distribution actually has. Nothing is pinned for the same
# reason; the base image is.

set -eu

# shellcheck source=/dev/null
. /etc/os-release

generate_locale() {
  localedef -c -i sv_SE -f UTF-8 sv_SE.UTF-8 || [ "$?" -eq 1 ]
}

case "$ID" in
ubuntu | debian)
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    bash bsdutils coreutils gettext git gnupg gpg-agent grep jq locales openssh-client util-linux
  rm -rf /var/lib/apt/lists/*
  generate_locale
  ;;
fedora)
  dnf install -y --setopt=install_weak_deps=False \
    bash coreutils gettext git-core glibc-langpack-sv gnupg2 grep jq openssh-clients util-linux util-linux-script
  dnf clean all
  ;;
opensuse-leap | opensuse-tumbleweed)
  zypper --non-interactive install --no-recommends \
    bash coreutils gettext-tools git-core glibc-i18ndata gpg2 grep jq openssh-clients util-linux
  zypper clean --all
  generate_locale
  ;;
arch)
  # The Arch base image excludes locale sources from extraction; let glibc
  # unpack the one the locale row needs.
  printf 'NoExtract = !usr/share/i18n/locales/sv_SE\n' >>/etc/pacman.conf
  pacman -Syu --noconfirm --needed \
    bash coreutils gettext git gnupg grep jq openssh util-linux
  pacman -S --noconfirm glibc
  pacman -Scc --noconfirm
  generate_locale
  ;;
*)
  printf 'install-deps: unsupported distribution: %s\n' "$ID" >&2
  exit 1
  ;;
esac

# Assert rather than trust the package names: a rename or split in a later
# release fails here, loudly, instead of deep inside a tier.
for tool in bash git grep gpg gpg-agent ssh ssh-keygen script stty prlimit unshare sh msgfmt jq locale; do
  command -v "$tool" >/dev/null || {
    printf 'install-deps: %s provides no %s\n' "$ID" "$tool" >&2
    exit 1
  }
done
locale -a >/tmp/locales
grep -Eqx 'C\.(utf8|UTF-8)' /tmp/locales || {
  printf 'install-deps: %s has no C.UTF-8 locale\n' "$ID" >&2
  exit 1
}
grep -Eqx 'sv_SE\.(utf8|UTF-8)' /tmp/locales || {
  printf 'install-deps: %s has no sv_SE.UTF-8 locale\n' "$ID" >&2
  exit 1
}
rm -f /tmp/locales
