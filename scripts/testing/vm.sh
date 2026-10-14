#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Shared QEMU/Tart adapter. The project owns vm-tests.sh; this file owns only
# image preparation, transport, the disposable machine and its exit status.
set -euo pipefail
umask 077
# All uploads below explicitly redirect a file. An SSH probe must not inherit
# terminal input: timeout's process group would stop it with SIGTTIN.
exec </dev/null

fail() {
  printf 'test VM: %s\n' "$*" >&2
  exit 2
}

need() {
  if command -v "$1" >/dev/null 2>&1; then
    return
  fi
  case "$1" in
  qemu-img | qemu-system-*) fail "missing controller tool: $1. Install QEMU (system emulator and image utilities), then retry. See docs/testing/test-environments.adoc." ;;
  tart) fail 'missing controller tool: tart. Install Tart on the Apple Silicon Mac, then retry. See docs/testing/test-environments.adoc.' ;;
  gtimeout) fail 'missing controller tool: GNU timeout. Install coreutils (gtimeout on macOS), then retry.' ;;
  ssh) fail 'missing controller tool: ssh. Install the OpenSSH client, then retry.' ;;
  *) fail "missing controller tool: $1. Install it using your package manager, then retry." ;;
  esac
}

arch() {
  case "$1" in
  x86_64 | amd64) printf amd64 ;;
  aarch64 | arm64) printf arm64 ;;
  *) fail "unsupported architecture: $1" ;;
  esac
}

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  else
    shasum -a 256 "$@"
  fi
}

parse_arguments() {
  request_args=("$@")
  action=${1:-} target=${2:-} selection=${3:-}
  case "$action" in
  prepare) [[ $# == 2 || $# == 3 ]] || fail 'usage: vm.sh prepare TARGET [PROFILE.json]' ;;
  run) [[ $# -ge 3 ]] || fail 'usage: vm.sh run TARGET TIER [names...]' ;;
  clean) [[ $# == 2 ]] || fail 'usage: vm.sh clean TARGET' ;;
  clean-downloads)
    [[ $# == 1 ]] || fail 'usage: vm.sh clean-downloads'
    target=downloads
    return
    ;;
  *) fail 'select prepare, run, clean or clean-downloads explicitly' ;;
  esac
  case "$target" in
  freebsd-amd64 | freebsd-arm64)
    kernel=FreeBSD
    backend=qemu
    ;;
  openbsd-amd64 | openbsd-arm64)
    kernel=OpenBSD
    backend=qemu
    ;;
  netbsd-amd64 | netbsd-arm64)
    kernel=NetBSD
    backend=qemu
    ;;
  macos-arm64)
    kernel=Darwin
    backend=tart
    ;;
  linux-arm64)
    kernel=Linux
    backend=tart
    ;;
  *) fail "unsupported target: $target" ;;
  esac
  if [[ "$action" == run ]]; then
    case "$selection" in
    unit | race | integration | blackbox | combined)
      [[ $# == 3 ]] || fail 'native VM tiers do not accept extra arguments'
      ;;
    container-unit | container-blackbox | container-combined | container-matrix)
      [[ "$kernel" == Linux ]] || fail 'container tiers require a Linux guest'
      ;;
    *) fail "unsupported tier: $selection" ;;
    esac
  fi
}

check_controller() {
  local marker
  for marker in CI CONTINUOUS_INTEGRATION GITHUB_ACTIONS GITEA_ACTIONS FORGEJO_ACTIONS GITLAB_CI \
    BUILDKITE CIRCLECI JENKINS_URL TEAMCITY_VERSION TF_BUILD DRONE WOODPECKER CODEBUILD_BUILD_ID TRAVIS; do
    if declare -p "$marker" >/dev/null 2>&1; then
      fail "local VM recipes refuse CI marker $marker"
    fi
  done
  [[ $(id -u) != 0 ]] || fail 'run the controller as an ordinary user'
}

set_phase() {
  printf '%s\n' "$1" >"$logs/phase.txt"
  printf '== VM phase: %s\n' "$1" >&2
}

watch_progress() (
  # This observer must never inherit the controller's VM cleanup traps.
  trap - EXIT HUP INT TERM
  local controller=$1 fd label line current detail chunks request complete stopping=false
  local -a pending=([5]='' [6]='' [7]='' [8]='')
  local last_output=-1 next_notice=$((SECONDS + 15))
  exec 5<"$logs/console.log" 6<"$logs/bootstrap.log" 7<"$logs/test.log" 8<"$logs/bootstrap-http.log"
  while kill -0 "$controller" 2>/dev/null; do
    for fd in 5 6 7 8; do
      case "$fd" in
      5) label=console ;;
      6) label=packages ;;
      7) label=guest ;;
      8) label=metadata-http ;;
      esac
      chunks=0
      # Budget by characters, not lines: verbose suites can emit thousands of
      # short test-result lines per second. A 64-line cap fell far behind them.
      while [[ "$stopping" == true || "$chunks" -lt 262144 ]]; do
        line=''
        complete=false
        if IFS= read -r -n 4096 -u "$fd" line; then
          complete=true
        elif [[ -z "$line" ]]; then
          break
        fi
        last_output=$SECONDS
        chunks=$((chunks + ${#line} + 1))
        line=${pending[fd]}$line
        # Keep raw logs intact; don't let guest control bytes affect the terminal.
        # Remove overwritten spinner characters together with their backspaces.
        line=${line//?$'\b'/}
        line=${line//$'\r'/}
        line=${line//[$'\001'-$'\010'$'\013'$'\014'$'\016'-$'\037'$'\177']/}
        if [[ "$complete" == true || ${#line} -ge 4096 ]]; then
          printf '[%s] %s\n' "$label" "$line" >&2
          pending[fd]=''
        else
          # EOF in a growing file may be the middle of a line, not its end.
          pending[fd]=$line
          break
        fi
      done
      if [[ "$stopping" == true && -n "${pending[fd]}" ]]; then
        printf '[%s] %s\n' "$label" "${pending[fd]}" >&2
        pending[fd]=''
      fi
    done
    [[ "$stopping" == false ]] || break
    if ((SECONDS >= next_notice)); then
      current=$(<"$logs/phase.txt")
      if ((last_output < 0)); then
        detail='no guest log output yet'
      else
        detail="last guest log output $((SECONDS - last_output))s ago"
      fi
      printf '[VM +%ss] Waiting: %s (%s)\n' "$((SECONDS - progress_started))" "${current:-current step}" "$detail" >&2
      next_notice=$((SECONDS + 15))
    fi
    # A timed builtin read polls logs without spawning orphanable sleep children.
    if IFS= read -r -t 1 -u 4 request; then
      [[ "$request" != stop ]] || stopping=true
    fi
  done
)

start_progress() {
  : >"$logs/console.log"
  : >"$logs/bootstrap.log"
  : >"$logs/test.log"
  : >"$logs/bootstrap-http.log"
  mkfifo "$scratch/progress-control"
  exec 4<>"$scratch/progress-control"
  progress_started=$SECONDS
  watch_progress "$$" &
  progress_pid=$!
}

stop_progress() {
  [[ -n "$progress_pid" ]] || return 0
  if jobs -pr | grep -Fxq "$progress_pid"; then
    printf 'stop\n' >&4
  fi
  wait "$progress_pid" 2>/dev/null || true
  progress_pid=''
  exec 4>&-
}

# Lifecycle state stays global so EXIT can clean it after main returns.
# Per-step scratch variables are local to their functions.
cleanup() {
  local result=$? command child lock
  trap - EXIT HUP INT TERM
  if [[ -n "$logs" ]]; then set_phase 'Cleaning up the run' || true; fi
  if [[ -z "$pid" && -n "$scratch" && -f "$scratch/pid" ]]; then
    pid=$(<"$scratch/pid") || pid=''
  fi
  [[ -z "$pid" || "$pid" =~ ^[1-9][0-9]*$ ]] || pid=''
  if [[ -n "$pid" ]]; then
    command=$(ps -ww -p "$pid" -o command= 2>/dev/null || true)
    if [[ "$command" == *"$name"* ]]; then
      kill -TERM "$pid" 2>/dev/null || true
      sleep 1
      command=$(ps -ww -p "$pid" -o command= 2>/dev/null || true)
      [[ "$command" != *"$name"* ]] || kill -KILL "$pid" 2>/dev/null || true
    fi
    wait "$pid" 2>/dev/null || true
  fi
  if [[ "$cloned" == true ]]; then
    "$timer" --kill-after=5s 30s tart delete "$name" >/dev/null 2>&1 || result=1
  fi
  for child in "$http_pid" "$console_pid"; do
    if [[ -n "$child" ]] && jobs -pr | grep -Fxq "$child"; then
      kill -TERM "$child" 2>/dev/null || true
      wait "$child" 2>/dev/null || true
    fi
  done
  stop_progress
  if [[ -n "$scratch" ]]; then
    rm -rf -- "$scratch" || result=1
  fi
  if [[ "$preparing" == true && "$result" != 0 ]]; then
    rm -rf -- "$cache" || result=1
  fi
  for lock in ${download_locks[@]+"${download_locks[@]}"}; do
    rmdir "$lock" || result=1
  done
  rmdir "$cache.lock" || result=1
  if [[ -n "$logs" ]]; then
    printf 'controller_exit_code=%s\n' "$result" >>"$logs/result.txt" || result=1
    if [[ "$result" != 0 ]]; then
      printf 'test VM: failed (exit %s); logs: %s\n' "$result" "$logs" >&2
    fi
  fi
  exit "$result"
}

profile_help() {
  local recipe
  printf 'First-time setup for %s:\n' "$target"
  printf '  just test-vm-prepare %s\n' "$target"
  printf 'Preparation reads scripts/testing/profiles.json, downloads verified media,\n'
  printf 'sets up the guest and installs project dependencies. This step needs network access.\n'
  printf 'Host prerequisites and custom images: docs/testing/test-environments.adoc\n'
  if [[ "$action" == run ]]; then
    case "$selection" in
    container-*) recipe="test-${selection#container-}-container-on-macos" ;;
    *) recipe="test-$selection-vm" ;;
    esac
    printf 'Then retry:\n  just %s' "$recipe"
    if [[ "$selection" != container-* ]]; then printf ' %s' "$target"; fi
    if ((${#request_args[@]} > 3)); then
      printf ' %q' "${request_args[@]:3}"
    fi
    printf '\n'
  fi
}

load_profile() {
  local host host_arch compatible version
  guest_arch=${target##*-}
  host=$(uname -s)
  host_arch=$(arch "$(uname -m)")
  if [[ "$backend" == tart && ("$host" != Darwin || "$host_arch" != arm64) ]]; then
    fail 'Tart guests require an Apple Silicon Mac controller. On Linux use the direct *-container recipes; see just help and docs/testing/test-environments.adoc.'
  fi
  need jq
  need git
  need tar
  need ps
  if ! command -v sha256sum >/dev/null 2>&1; then need shasum; fi
  if command -v timeout >/dev/null 2>&1; then
    timer=timeout
  else
    need gtimeout
    timer=gtimeout
  fi
  if [[ "$host_arch" != "$guest_arch" ]]; then
    printf 'test VM: requested %s, but this controller is %s/%s.\n' "$target" "$host" "$host_arch" >&2
    if [[ "$backend" == qemu ]]; then
      compatible="${target%-*}-$host_arch"
      printf 'For this controller, prepare the matching target:\n' >&2
      printf '  just test-vm-prepare %s\n' "$compatible" >&2
      if [[ "$action" == run ]]; then
        printf 'Then run:\n  just test-%s-vm %s\n' "$selection" "$compatible" >&2
      fi
      fail "To test $target specifically, use an $guest_arch Linux controller or a matching Mac. These recipes do not support CPU emulation. See docs/testing/test-environments.adoc."
    fi
    fail 'Tart targets require an Apple Silicon Mac controller; run this recipe there. See docs/testing/test-environments.adoc.'
  fi
  profile=$cache/profile.json
  if [[ "$action" == prepare ]]; then
    if [[ -n "$selection" ]]; then
      profile=$selection
    else
      profile="$scratch/input-profile.json"
      jq -e --arg target "$target" '.targets[$target] | select(type == "object")' \
        "$repo/scripts/testing/profiles.json" >"$profile" ||
        fail "no valid profile for $target in scripts/testing/profiles.json. Check the repository profile data."
    fi
  fi
  if [[ ! -f "$profile" || -L "$profile" ]]; then
    if [[ "$action" == prepare ]]; then
      fail "custom profile is missing or not a regular non-symlink file: $profile. Correct the file path and retry the same command. See the custom-image examples in docs/testing/test-environments.adoc."
    fi
    fail "no prepared VM for $target."$'\n'"$(profile_help)"
  fi
  image=$(jq -er '.image | select(type == "string" and length > 0)' "$profile") ||
    fail "profile $profile needs a nonempty image string. See docs/testing/test-environments.adoc."
  bootstrap=$(jq -er '(.bootstrap // "") | select(type == "string")' "$profile") ||
    fail "profile $profile has an invalid bootstrap value; it must be a string."
  case "$kernel:$bootstrap" in
  *:) ;;
  FreeBSD:freebsd-cloudinit | OpenBSD:openbsd-autoinstall | NetBSD:netbsd-cloud | Darwin:tart | Linux:tart) ;;
  *) fail "bootstrap method '$bootstrap' does not match $target. Check scripts/testing/profiles.json or your custom profile." ;;
  esac
  if [[ "$action" == prepare && -n "$bootstrap" ]]; then
    # shellcheck source=scripts/testing/vm-bootstrap.sh
    source "$repo/scripts/testing/vm-bootstrap.sh"
  fi
  if [[ "$kernel" == Linux ]]; then
    jq -e '.distributions | type == "array" and length > 0 and all(.[]; type == "string" and test("^[a-z0-9][a-z0-9-]*$"))' "$profile" >/dev/null ||
      fail 'Linux profiles must select a nonempty distributions array of ARM64 image names'
  fi

  if [[ "$backend" == qemu ]]; then
    need qemu-img
    need ssh
    case "$host" in
    Linux) accelerator=kvm ;;
    Darwin) accelerator=hvf ;;
    *) fail "unsupported QEMU controller: $host" ;;
    esac
    if [[ "$guest_arch" == amd64 ]]; then
      qemu='qemu-system-x86_64'
    else
      qemu='qemu-system-aarch64'
    fi
    need "$qemu"
    if [[ "$action" != prepare || -z "$bootstrap" ]]; then
      login=$(jq -er '.ssh_user | select(type == "string" and test("^[a-z_][a-z0-9_-]*$"))' "$profile") ||
        fail "profile $profile needs a valid ssh_user (root or the guest control account)."
      identity=$(jq -er '.ssh_key | select(type == "string")' "$profile") ||
        fail "profile $profile needs ssh_key pointing to the dedicated test identity."
      hostkey=$(jq -er '.ssh_host_key | select(type == "string" and test("^ssh-ed25519 [A-Za-z0-9+/=]+$"))' "$profile") ||
        fail "profile $profile needs ssh_host_key in 'ssh-ed25519 BASE64' format."
      [[ "$identity" == /* && -f "$identity" ]] || fail 'ssh_key must name a dedicated absolute test-key path'
    fi
  else
    [[ "$host" == Darwin ]] || fail 'Tart requires an Apple Silicon Mac'
    need tart
    version=$(sw_vers -productVersion)
    ((${version%%.*} >= 26)) || fail "controller runs macOS $version; Tart tests need macOS 26+ for host-only networking. Update this Mac or use a Mac running macOS 26 or later."
    [[ "$image" =~ @sha256:[0-9a-f]{64}$ ]] || fail 'Tart images must be pinned by OCI digest'
    export TART_HOME="$cache/tart" TART_NO_AUTO_PRUNE=1
  fi
}

check_prepared_image() {
  set_phase 'Verifying the prepared image'
  if [[ "$backend" == qemu ]]; then
    (cd "$cache" && digest -c disk.sha256) || {
      printf 'test VM: prepared disk verification failed. Run just test-vm-clean %s, then prepare it again.\n' "$target" >&2
      return 1
    }
  else
    "$timer" --kill-after=5s 15s tart get base >/dev/null ||
      fail "prepared Tart image is missing or unreadable. Run just test-vm-clean $target, then just test-vm-prepare $target."
  fi
}

prepare_image() {
  local expected actual firmware
  set_phase 'Checking image preparation inputs'
  input_key=$(preparation_key)
  if [[ -f "$cache/ready" && -f "$cache/input.sha256" && $(<"$cache/input.sha256") == "$input_key" ]]; then
    check_prepared_image
    printf 'reused=true\n' >>"$logs/result.txt"
    printf 'Prepared VM is current: %s\n' "$target"
    reused=true
    return
  fi
  [[ ! -e "$cache" ]] || fail "target exists; use just test-vm-clean $target before replacing it"
  if [[ -n "$bootstrap" ]]; then bootstrap_requirements; fi
  mkdir "$cache"
  preparing=true
  if [[ -n "$bootstrap" ]]; then
    set_phase 'Preparing the base image and guest access'
    bootstrap_image
  elif [[ "$backend" == qemu ]]; then
    [[ "$image" == /* && -f "$image" && ! -L "$image" ]] || fail 'image must name an absolute QCOW2 file'
    expected=$(jq -er '.sha256 | select(test("^[0-9a-f]{64}$"))' "$profile")
    actual=$(digest "$image")
    [[ "${actual%% *}" == "$expected" ]] || fail 'base image checksum mismatch'
    qemu-img info -f qcow2 --output=json "$image" |
      jq -e '(."backing-filename" // "") == "" and (."format-specific".data."data-file" // "") == ""' >/dev/null ||
      fail 'use a standalone QCOW2 base'
    qemu-img convert -f qcow2 -O qcow2 "$image" "$cache/disk.qcow2"
    if [[ "$guest_arch" == arm64 ]]; then
      firmware=$(jq -er .firmware "$profile")
      expected=$(jq -er '.firmware_sha256 | select(test("^[0-9a-f]{64}$"))' "$profile")
      [[ "$firmware" == /* && -f "$firmware" ]] || fail 'ARM QEMU requires a UEFI firmware file'
      actual=$(digest "$firmware")
      [[ "${actual%% *}" == "$expected" ]] || fail 'firmware checksum mismatch'
      cp "$firmware" "$cache/firmware"
    fi
  else
    "$timer" --kill-after=15s 30m tart clone "$image" base
  fi
  jq -S . "$profile" >"$cache/profile.json"
  if [[ -z "$selection" ]]; then
    jq '. + {repository_profile:true}' "$cache/profile.json" >"$scratch/profile-origin.json"
    mv "$scratch/profile-origin.json" "$cache/profile.json"
  fi
}

preparation_key() {
  local file input=${1:-$profile}
  {
    jq -Sc 'if .bootstrap then del(.ssh_user,.ssh_key,.ssh_host_key,.repository_profile) else . end' "$input"
    for file in scripts/testing/vm-bootstrap.sh scripts/testing/vm-bootstrap-guest.sh \
      scripts/testing/vm-guest.sh scripts/testing/vm-tests.sh scripts/testing/vm-metadata-proxy.py \
      go.mod go.sum rust-toolchain.toml Cargo.lock .mise.toml mise.lock; do
      if [[ -f "$repo/$file" ]]; then digest <"$repo/$file"; fi
    done
    if [[ "$kernel" == Linux ]]; then
      for file in tools/go.mod tools/go.sum Containerfile.sandbox \
        test/distribution/Containerfile test/distribution/images test/distribution/install-deps.sh \
        packaging/container/Containerfile.blackbox tests/container/Containerfile tests/container/images tests/container/install-deps.sh; do
        if [[ -f "$repo/$file" ]]; then digest <"$repo/$file"; fi
      done
    fi
  } | digest | cut -d ' ' -f 1
}

create_run_directory() {
  local tier=$selection suites=$selection environment=vm
  [[ "$action" != prepare ]] || tier=prepare
  scratch=$(mktemp -d "${TMPDIR:-/tmp}/test-vm.XXXXXXXX")
  scratch=$(cd "$scratch" && pwd -P)
  name=${scratch##*/}
  logs=$store/results/$name
  mkdir -p "$logs" "$scratch/source"
  serial_backend="file:$logs/console.log"
  printf 'target=%s\nbackend=%s\naction=%s\ntier=%s\n' "$target" "$backend" "$action" "$tier" >"$logs/result.txt"
  if [[ "$action" == run ]]; then
    if [[ "$suites" == container-* ]]; then
      environment=container-in-vm
      suites=${suites#container-}
    fi
    case "$suites" in
    combined) suites='integration + black-box' ;;
    blackbox) suites='black-box' ;;
    race) suites='unit (race)' ;;
    matrix) suites='distribution matrix' ;;
    esac
    printf '== Test job: suites=%s | environment=%s | target=%s/%s | backend=%s\n' "$suites" "$environment" "$kernel" "${target##*-}" "$backend" >&2
  else
    printf '== VM preparation: target=%s/%s | backend=%s | profile=%s | cache=%s | network=allowed\n' \
      "$kernel" "${target##*-}" "$backend" "${selection:-scripts/testing/profiles.json}" "$cache" >&2
  fi
  printf 'VM %s: %s\nLogs: %s\n' "$action" "$target" "$logs"
}

create_run_state() {
  local path parent source_hash
  set_phase 'Snapshotting the current source'
  : >"$scratch/args"
  if (($# > 0)); then
    printf '%s\0' "$@" >"$scratch/args"
  fi
  if [[ "$action" == prepare && "$kernel" == Linux ]]; then
    jq -j '.distributions[] | . + "\u0000"' "$profile" >"$scratch/args"
  fi

  # Include new source files, excluding credentials, images and Git state.
  git -C "$repo" ls-files --cached --others --exclude-standard -z >"$scratch/paths"
  while IFS= read -r -d '' path; do
    case "/$path/" in
    */.git/* | */.test-vms/* | */.ssh/* | */.gnupg/*) continue ;;
    esac
    case "${path##*/}" in
    .env | .env.* | id_rsa | id_ed25519 | *.key | *.p12 | *.pfx | credentials) continue ;;
    esac
    [[ -e "$repo/$path" || -L "$repo/$path" ]] || continue
    [[ -f "$repo/$path" && ! -L "$repo/$path" ]] || fail "unsupported source entry: $path"
    parent=$(cd "$(dirname "$repo/$path")" && pwd -P)
    [[ "$parent" == "$repo" || "$parent" == "$repo/"* ]] || fail "source escapes checkout: $path"
    mkdir -p "$(dirname "$scratch/source/$path")"
    cp -p "$repo/$path" "$scratch/source/$path"
  done <"$scratch/paths"
  tar -C "$scratch/source" -cf "$scratch/source.tar" .
  source_hash=$(digest "$scratch/source.tar")
  printf 'source_sha256=%s\nimage=%s\n' "${source_hash%% *}" "$image" >>"$logs/result.txt"
}

start_qemu() {
  local disk=$cache/disk.qcow2 block restrict=on
  local -a machine=(-machine q35)
  set_phase "Starting QEMU for $kernel/$guest_arch"
  if [[ "$action" == run ]]; then
    qemu-img create -q -f qcow2 -F qcow2 -b "$disk" "$scratch/disk.qcow2"
    disk=$scratch/disk.qcow2
  else
    restrict=off
  fi
  block=$(jq -nc --arg path "$disk" '{driver:"qcow2",file:{driver:"file",filename:$path},"node-name":"disk"}')
  ssh_port=$((22000 + RANDOM % 30000))
  if [[ "$guest_arch" == arm64 ]]; then
    machine=(-machine virt -bios "$cache/firmware")
  fi
  "$timer" --kill-after=5s 30s "$qemu" -name "$name" -daemonize -pidfile "$scratch/pid" -accel "$accelerator" -cpu host \
    "${machine[@]}" -m 4096 -smp 2 -blockdev "$block" -device virtio-blk-pci,drive=disk \
    -netdev "user,id=net,${bootstrap_net}restrict=$restrict,hostfwd=tcp:127.0.0.1:$ssh_port-:22" -device virtio-net-pci,netdev=net \
    -device virtio-rng-pci "${qemu_extra[@]}" -display none -monitor none -serial "$serial_backend"
  pid=$(<"$scratch/pid")
  [[ "$pid" =~ ^[1-9][0-9]*$ ]] || {
    pid=''
    fail 'invalid QEMU PID'
  }
  if [[ "$action" != prepare || -z "$bootstrap" ]]; then configure_ssh; fi
}

configure_ssh() {
  local hosts_file
  printf 'test-vm %s\n' "$hostkey" >"$scratch/known_hosts"
  hosts_file=${scratch//\\/\\\\}/known_hosts
  hosts_file=${hosts_file//\"/\\\"}
  hosts_file=${hosts_file//%/%%}
  remote=(ssh -F /dev/null -p "$ssh_port" -i "$identity" -o BatchMode=yes -o IdentitiesOnly=yes -o IdentityAgent=none
    -o ForwardAgent=no -o ClearAllForwardings=yes -o StrictHostKeyChecking=yes -o HostKeyAlias=test-vm
    -o "UserKnownHostsFile=\"$hosts_file\"" -o GlobalKnownHostsFile=/dev/null -o ConnectTimeout=3 "$login@127.0.0.1")
  root=(env)
  if [[ "$login" != root ]]; then
    root=(sudo -n)
  fi
}

start_tart() {
  local -a tart_args
  set_phase "Starting Tart for $kernel/$guest_arch"
  if [[ "$action" == prepare ]]; then
    name=base
  else
    cloned=true
    "$timer" --kill-after=15s 2m tart clone base "$name"
  fi
  tart set "$name" --cpu 2 --memory 4096
  tart_args=(run "$name" --no-graphics --no-audio --no-clipboard --no-usb-accessories)
  if [[ "$action" != prepare ]]; then
    tart_args+=(--net-host)
  fi
  tart "${tart_args[@]}" >"$logs/console.log" 2>&1 &
  pid=$!
  remote=(tart exec -i "$name")
  root=(sudo -n)
}

wait_for_transport() {
  local deadline=$((SECONDS + 180))
  until "$timer" --kill-after=5s 10s "${remote[@]}" true 2>"$logs/transport.log"; do
    kill -0 "$pid" 2>/dev/null || fail "VM exited; inspect $logs/console.log"
    ((SECONDS < deadline)) || fail 'VM did not become ready'
    sleep 1
  done
}

wait_for_guest() {
  local actual_os actual_arch
  set_phase 'Guest access and platform check (limit 3m)'
  wait_for_transport
  actual_os=$("$timer" --kill-after=5s 15s "${remote[@]}" uname -s)
  actual_arch=$("$timer" --kill-after=5s 15s "${remote[@]}" uname -m)
  [[ "$actual_os" == "$kernel" && "$(arch "$actual_arch")" == "$guest_arch" ]] ||
    fail "guest mismatch: $actual_os/$actual_arch; expected $kernel/$guest_arch. Check the profile, run just test-vm-clean $target, then repeat just test-vm-prepare $target."
  printf 'guest=%s/%s\n' "$actual_os" "$guest_arch" >>"$logs/result.txt"
}

remote_shell() {
  local text=$1
  if [[ "$backend" == qemu ]]; then
    text=${text//\'/\'\\\'\'}
    "$timer" --kill-after=5s 30s "${remote[@]}" "${root[@]}" sh -c "'$text'"
  else
    "$timer" --kill-after=5s 30s "${remote[@]}" "${root[@]}" sh -c "$text"
  fi
}

stage_guest() {
  set_phase 'Transferring the source snapshot to the guest'
  # shellcheck disable=SC2016 # Check the guest UID, not the controller UID.
  remote_shell 'test "$(id -u)" = 0 && command -v bash >/dev/null && rm -rf /var/tmp/itiquette-vm-source && mkdir -p /var/tmp/itiquette-vm-source'
  "$timer" --kill-after=5s 120s "${remote[@]}" "${root[@]}" tar -xf - -C /var/tmp/itiquette-vm-source <"$scratch/source.tar"
  remote_shell 'install -d -m 0755 /var/run/itiquette-vm; install -m 0555 /var/tmp/itiquette-vm-source/scripts/testing/vm-guest.sh /var/run/itiquette-vm/controller.sh'
  "$timer" --kill-after=5s 30s "${remote[@]}" "${root[@]}" tee /var/run/itiquette-vm/args <"$scratch/args" >/dev/null
}

run_guest() {
  local tier=$selection guest=/var/run/itiquette-vm/controller.sh serial deadline result_pattern
  if [[ "$action" == prepare ]]; then
    tier=prepare
    set_phase 'Preparing the guest toolchain and project dependencies (limit 30m)'
  else
    set_phase "Compiling and running $selection in the guest (limit 30m; cold compilation can be quiet)"
  fi
  status=0
  if [[ "$backend" == qemu ]]; then
    serial=$(jq -r '.serial // "/dev/ttyu0"' "$cache/profile.json")
    [[ "$serial" =~ ^/dev/tty[A-Za-z0-9._-]+$ ]] || fail 'invalid guest serial device'
    # Verbose suites need more than the boot console's default baud rate.
    # Open the serial TTY after nohup; otherwise it redirects to nohup.out.
    remote_shell "test -c $serial && stty 115200 <$serial && { nohup /bin/sh -c 'exec env PATH=/usr/local/bin:/usr/pkg/bin:/usr/pkg/sbin:/usr/bin:/bin:/usr/sbin:/sbin bash $guest $tier $kernel $guest_arch $name serial >$serial 2>&1' </dev/null >/dev/null 2>&1 & }"
    deadline=$((SECONDS + 1800))
    result_pattern="^TEST_VM_RESULT $name [0-9]+[[:space:]]*$"
    until grep -Eq "$result_pattern" "$logs/console.log"; do
      if ! ps -ww -p "$pid" -o command= 2>/dev/null | grep -Fq "$name"; then
        # A pipe reader may still be flushing the VM's final serial output.
        if [[ -n "$console_pid" ]]; then wait "$console_pid" 2>/dev/null || true; fi
        grep -Eq "$result_pattern" "$logs/console.log" || fail "VM exited without a test result; inspect $logs/console.log"
        break
      fi
      if ((SECONDS >= deadline)); then
        status=124
        break
      fi
      sleep 1
    done
    if [[ "$status" != 124 ]]; then
      status=$(sed -n "s/^TEST_VM_RESULT $name //p" "$logs/console.log" | tr -d '\r')
    fi
  else
    "$timer" --kill-after=15s 30m "${remote[@]}" "${root[@]}" env PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin \
      bash "$guest" "$tier" "$kernel" "$guest_arch" "$name" >"$logs/test.log" 2>&1 || status=$?
  fi
  [[ "$status" =~ ^[0-9]+$ ]] || fail 'missing or ambiguous guest result'
  printf 'exit_code=%s\n' "$status" >>"$logs/result.txt"
}

finish_preparation() {
  local deadline
  set_phase 'Finalizing and checksumming the prepared image'
  if [[ "$backend" == qemu ]]; then
    deadline=$((SECONDS + 60))
    while ps -ww -p "$pid" -o command= 2>/dev/null | grep -Fq "$name"; do
      ((SECONDS < deadline)) || fail 'guest did not shut down after preparation'
      sleep 1
    done
    pid=''
    (
      cd "$cache"
      digest disk.qcow2 >disk.sha256
      if [[ -f firmware ]]; then
        digest firmware >>disk.sha256
      fi
    )
  else
    "$timer" --kill-after=5s 30s tart stop base
    deadline=$((SECONDS + 30))
    while kill -0 "$pid" 2>/dev/null; do
      ((SECONDS < deadline)) || fail 'Tart did not stop after preparation'
      sleep 1
    done
    wait "$pid" || true
    pid=''
  fi
  printf '%s\n' "$input_key" >"$cache/input.sha256"
  printf '%s\n' "$target" >"$cache/ready"
}

main() {
  local locked_target
  parse_arguments "$@"
  check_controller
  repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
  store=$repo/.test-vms
  cache=$store/$target
  [[ ! -L "$store" && ! -L "$cache" ]] || fail 'VM state must not be a symlink'
  if [[ ("$action" == clean || "$action" == clean-downloads) && ! -e "$store" ]]; then
    printf 'No VM state to clean: %s\n' "$cache"
    return
  fi
  mkdir -p "$store"
  mkdir "$cache.lock" 2>/dev/null || fail "cannot lock target $target ($cache.lock). Wait for an active run to finish. After an interrupted run, follow the stale-lock cleanup instructions in docs/testing/test-environments.adoc."
  scratch='' name='' pid='' logs='' cloned=false preparing=false reused=false
  http_pid='' console_pid='' progress_pid='' bootstrap_net=''
  download_locks=()
  qemu_extra=(-no-user-config)
  trap cleanup EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  if [[ "$action" == clean-downloads ]]; then
    # Media is shared by every target. Reuse their locks so preparation cannot
    # start between an active-job check and deletion. Release partial acquisition
    # through the normal cleanup trap if any target is already locked.
    for locked_target in freebsd-amd64 freebsd-arm64 openbsd-amd64 openbsd-arm64 \
      netbsd-amd64 netbsd-arm64 macos-arm64 linux-arm64; do
      mkdir "$store/$locked_target.lock" 2>/dev/null || fail "cannot clean downloads while target $locked_target is locked"
      download_locks+=("$store/$locked_target.lock")
    done
    printf '== VM download cleanup: remove=%s | retain=prepared-images,results\n' "$cache" >&2
    rm -rf -- "$cache"
    return
  fi
  if [[ "$action" == clean ]]; then
    printf '== VM cleanup: target=%s/%s | backend=%s | remove=%s\n' "$kernel" "${target##*-}" "$backend" "$cache" >&2
    rm -rf -- "$cache"
    return
  fi

  create_run_directory
  set_phase 'Checking the controller and target profile'
  start_progress
  load_profile
  if [[ "$action" == prepare ]]; then
    prepare_image
    [[ "$reused" != true ]] || return 0
  else
    [[ -f "$cache/ready" ]] || fail "VM preparation did not finish for $target. Inspect .test-vms/results, then run just test-vm-clean $target and repeat just test-vm-prepare $target."
    if [[ -f "$cache/input.sha256" ]]; then
      local current_profile=$profile
      if [[ $(jq -r '.repository_profile // false' "$profile") == true ]]; then
        current_profile="$scratch/current-profile.json"
        jq -e --arg target "$target" '.targets[$target]' "$repo/scripts/testing/profiles.json" >"$current_profile"
      fi
      [[ $(preparation_key "$current_profile") == "$(<"$cache/input.sha256")" ]] ||
        fail "prepared VM inputs changed. Run just test-vm-clean $target, then just test-vm-prepare $target."
    fi
    check_prepared_image
  fi
  create_run_state "${@:4}"
  case "$backend" in
  qemu) start_qemu ;;
  tart) start_tart ;;
  esac
  if [[ "$action" == prepare && -n "$bootstrap" ]]; then
    set_phase 'Bootstrapping the guest operating system'
    bootstrap_connect
  fi
  wait_for_guest
  if [[ "$action" == prepare && -n "$bootstrap" ]]; then
    set_phase 'Installing guest base tools (see packages output)'
    bootstrap_guest_tools
    if [[ "$backend" == qemu ]]; then
      # Guest setup may reload sshd; NetBSD also switches to the root account.
      set_phase 'Checking SSH after guest setup (limit 3m)'
      wait_for_transport
      jq --arg login "$login" --arg identity "$identity" --arg hostkey "$hostkey" \
        '. + {ssh_user:$login,ssh_key:$identity,ssh_host_key:$hostkey}' "$cache/profile.json" >"$scratch/resolved-profile.json"
      mv "$scratch/resolved-profile.json" "$cache/profile.json"
    fi
  fi
  stage_guest
  run_guest
  if [[ "$action" == prepare && "$status" == 0 ]]; then
    finish_preparation
  fi
  printf 'VM result: %s (logs in %s)\n' "$status" "$logs"
  return "$status"
}

main "$@"
