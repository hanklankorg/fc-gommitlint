#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Controller-installed guest entry point, never a host-side test command.
set -euo pipefail

check_guest() {
  local actual_arch
  [[ $EUID == 0 && ${BASH_SOURCE[0]} == /var/run/itiquette-vm/controller.sh ]] || {
    printf 'Use the Just VM recipes; this entry point runs only inside their guest.\n' >&2
    exit 2
  }
  tier=$1 expected_os=$2 expected_arch=$3 run_id=$4 transport=${5:-tart}
  [[ $(uname -s) == "$expected_os" ]] || exit 2
  case "$(uname -m)" in
  x86_64 | amd64) actual_arch=amd64 ;;
  aarch64 | arm64) actual_arch=arm64 ;;
  *) exit 2 ;;
  esac
  [[ "$actual_arch" == "$expected_arch" ]] || exit 2
  case "$tier" in
  prepare | unit | race | integration | blackbox | combined | \
    container-unit | container-blackbox | container-combined | container-matrix) ;;
  *) exit 2 ;;
  esac
}

finish() {
  local status=$?
  trap - EXIT
  sync
  printf '\nTEST_VM_RESULT %s %s\n' "$run_id" "$status"
  if [[ "$transport" == serial ]]; then
    /sbin/shutdown -p now
  fi
  exit "$status"
}

prepare_user() {
  ((BASH_VERSINFO[0] >= 4)) || {
    printf 'The guest needs Bash 4+ for repository shell tests.\n' >&2
    exit 2
  }
  command -v sudo >/dev/null || {
    printf 'The base image must provide sudo.\n' >&2
    exit 2
  }
  mkdir -p "$home"
  if ! id tester >/dev/null 2>&1; then
    [[ "$tier" == prepare ]] || {
      printf 'Prepare this VM first.\n' >&2
      exit 2
    }
    case "$expected_os" in
    Darwin)
      id -nu 4242 >/dev/null 2>&1 && exit 2
      dscl . -create /Users/tester
      dscl . -create /Users/tester UniqueID 4242
      dscl . -create /Users/tester PrimaryGroupID 20
      dscl . -create /Users/tester UserShell /bin/bash
      dscl . -create /Users/tester NFSHomeDirectory "$home"
      ;;
    FreeBSD) pw useradd tester -u 4242 -d "$home" -s "$(command -v bash)" ;;
    *) useradd -u 4242 -d "$home" -s "$(command -v bash)" tester ;;
    esac
  fi
  [[ $(id -u tester) != 0 ]] || exit 2
  case " $(id -Gn tester) " in
  *' wheel '* | *' admin '* | *' sudo '*)
    printf 'tester must be an unprivileged account.\n' >&2
    exit 2
    ;;
  esac
  if sudo -n -u tester sudo -n true 2>/dev/null; then
    printf 'tester must not have unrestricted passwordless sudo.\n' >&2
    exit 2
  fi
}

prepare_workspace() {
  local tool path
  mkdir -p "$home" "$artifacts"
  home=$(cd "$home" && pwd -P)
  artifacts=$(cd "$artifacts" && pwd -P)
  chown -R tester "$home" "$artifacts" "$source_dir"

  # Short enough for nested Podman, and canonical through macOS's /tmp symlink.
  mkdir -p /tmp/itq
  chown tester /tmp/itq
  temporary=$(cd /tmp/itq && pwd -P)
  export PATH="/opt/homebrew/bin:/opt/homebrew/opt/gettext/bin:/opt/homebrew/opt/rustup/bin:/usr/local/go/bin:/usr/local/cargo/bin:/usr/local/bin:/usr/pkg/bin:/usr/pkg/sbin:/usr/bin:/bin:/usr/sbin:/sbin"

  # Shell fixtures need GNU options; product executables and syscalls stay native.
  mkdir -p /var/run/itiquette-vm/tools
  for tool in stat sort date readlink realpath sha256sum find sed tar timeout cp mv rm mkdir chmod install head tail mktemp basename dirname du wc; do
    if path=$(command -v "g$tool"); then
      ln -sf "$path" "/var/run/itiquette-vm/tools/$tool"
    fi
  done
  # Some BSD packages install GnuPG 2 under its versioned executable name.
  if ! command -v gpg >/dev/null 2>&1 && path=$(command -v gpg2); then
    ln -sf "$path" /var/run/itiquette-vm/tools/gpg
  fi
  PATH="/var/run/itiquette-vm/tools:$PATH"
}

install_ca_helper() {
  if [[ "$expected_os" == Darwin && -f "$source_dir/scripts/testing/vm-trust-ca.sh" ]]; then
    install -d -m 0755 /usr/local/libexec /etc/sudoers.d
    install -m 0555 "$source_dir/scripts/testing/vm-trust-ca.sh" /usr/local/libexec/itiquette-test-trust-ca
    printf 'tester ALL=(root) NOPASSWD: /usr/local/libexec/itiquette-test-trust-ca ""\n' >/etc/sudoers.d/itiquette-test
    chmod 0440 /etc/sudoers.d/itiquette-test
    visudo -cf /etc/sudoers.d/itiquette-test
  fi
}

run_user() (
  # The SSH control user's home may be inaccessible after dropping privileges.
  cd "$source_dir"
  sudo -n -u tester env -i HOME="$home" PATH="$home/.cargo/bin:$PATH" TMPDIR="$temporary" \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0="$source_dir" \
    VM_ARTIFACTS="$artifacts" VM_SOURCE="$source_dir" VM_KERNEL="$expected_os" VM_ARCH="$expected_arch" \
    VM_ARGS=/var/run/itiquette-vm/args XDG_RUNTIME_DIR="/run/user/$(id -u tester)" \
    bash "$source_dir/scripts/testing/vm-tests.sh" "$@"
)

bsd_interfaces() {
  local listing interfaces
  listing=$(ifconfig -a) || return
  interfaces=$(printf '%s\n' "$listing" | awk -F: '/^[^[:space:]:]+: flags=/ {print $1}')
  [[ -n "$interfaces" ]] || return 2
  printf '%s\n' "$interfaces"
}

disable_network() {
  local service path interface interfaces
  # Tart uses virtio for control; QEMU reports through its serial console.
  case "$expected_os" in
  Darwin)
    networksetup -listallnetworkservices | tail -n +2 | while IFS= read -r service; do
      networksetup -setnetworkserviceenabled "${service#\*}" off
    done
    ;;
  Linux)
    for service in NetworkManager systemd-networkd; do
      if [[ $(systemctl show -p LoadState --value "$service") == loaded ]]; then
        systemctl stop "$service"
      fi
    done
    for path in /sys/class/net/*; do
      interface=${path##*/}
      [[ "$interface" == lo ]] || ip link set "$interface" down
    done
    ;;
  *)
    pkill -x dhclient || true
    pkill -x dhcpcd || true
    pkill -x dhcpleased || true
    pkill -x slaacd || true
    ;;
  esac
  if [[ "$expected_os" != Linux ]]; then
    interfaces=$(bsd_interfaces)
    for interface in $interfaces; do
      [[ "$interface" == lo* ]] || ifconfig "$interface" down
    done
  fi
  printf 'kernel=%s\narch=%s\nnetwork=none\nmemory_mib=4096\n' \
    "$expected_os" "$expected_arch" >/var/run/itiquette-vm/offline
  chmod 0444 /var/run/itiquette-vm/offline
}

main() {
  check_guest "$@"
  source_dir=$(cd /var/tmp/itiquette-vm-source && pwd -P)
  # OpenBSD links /var/tmp to /tmp and clears it at boot. SDKs and dependency
  # caches must survive the prepared image's next boot.
  home=/var/itiquette-vm/home
  artifacts=/var/tmp/itiquette-vm-artifacts
  trap finish EXIT
  prepare_user
  prepare_workspace
  install_ca_helper

  if [[ "$tier" == prepare ]]; then
    rm -f /var/run/itiquette-vm/offline
    if [[ "$expected_os" == Linux ]]; then
      loginctl enable-linger tester
      systemctl start "user@$(id -u tester).service"
    fi
    run_user prepare
    return
  fi

  disable_network
  printf 'Guest: %s %s, %s; tier: %s\n' "$expected_os" "$expected_arch" "$(uname -r)" "$tier"
  run_user build "$tier"
  # Fixtures/caches stay writable; native source and executables are sealed.
  # Container tiers keep their existing launchers' snapshot/mount boundaries.
  if [[ "$tier" != container-* ]]; then
    chown -R root "$source_dir" "$artifacts"
    chmod -R a+rX,a-w "$source_dir" "$artifacts"
  fi
  run_user run "$tier"
}

main "$@"
