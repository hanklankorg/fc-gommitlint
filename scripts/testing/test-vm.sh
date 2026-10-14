#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Contract checks with inert hypervisors/transports. Never starts a real guest.
set -eEuo pipefail
trap 'printf "VM controller contract failed at %s:%s\n" "${BASH_SOURCE[0]}" "$LINENO" >&2
  if [[ -n "${work:-}" && -f "$work/output" ]]; then cat "$work/output" >&2; fi' ERR

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  else
    shasum -a 256 "$@"
  fi
}

write_mocks() {
  cat >"$work/bin/id" <<'SH'
#!/bin/sh
echo "${MOCK_UID:-1000}"
SH
  cat >"$work/bin/uname" <<'SH'
#!/bin/sh
case "$1" in
-s) echo "$MOCK_HOST" ;;
-m) echo "$MOCK_ARCH" ;;
esac
SH
  cat >"$work/bin/sw_vers" <<'SH'
#!/bin/sh
echo 26.0
SH
  cat >"$work/bin/qemu-img" <<'SH'
#!/usr/bin/env bash
printf 'qemu-img %s\n' "$*" >>"$MOCK_TRACE"
case "$1" in
info) printf '{"format":"qcow2"}\n' ;;
create | convert) printf 'image\n' >"${!#}" ;;
resize) ;;
*) exit 2 ;;
esac
SH
  cat >"$work/bin/qemu-system-x86_64" <<'SH'
#!/usr/bin/env bash
printf 'qemu %s\n' "$*" >>"$MOCK_TRACE"
while (($#)); do
  case "$1" in
  -pidfile)
    printf '99999999' >"$2"
    printf '%s' "${2%/pid}/known_hosts" >"$MOCK_KNOWN"
    ;;
  -serial)
    printf '%s' "${2#file:}" >"$MOCK_CONSOLE"
    printf '\nITIQUETTE_BOOTSTRAP_READY\n' >"${2#file:}"
    ;;
  esac
  shift
done
SH
  cat >"$work/bin/ssh" <<'SH'
#!/usr/bin/env bash
printf 'ssh %s\n' "$*" >>"$MOCK_TRACE"
for argument in "$@"; do
  case "$argument" in
  UserKnownHostsFile=*)
    parsed=$("$REAL_SSH" -G -F /dev/null -o "$argument" example.invalid 2>/dev/null | sed -n 's/^userknownhostsfile //p')
    [[ "$parsed" == "$(<"$MOCK_KNOWN")" ]] || exit 89
    ;;
  esac
done
case "$*" in
*' true' | *'uname -s' | *'uname -m')
  # Probes must not consume terminal input, even when the caller supplies it.
  if IFS= read -r unexpected; then exit 88; fi
  ;;
esac
case "$*" in
*'uname -s'*) echo "$MOCK_GUEST" ;;
*'uname -m'*) echo x86_64 ;;
*'tar -xf'*) [[ $(wc -c) -gt 0 ]] || exit 88 ;;
*nohup*)
  [[ "$MOCK_RESULT" != missing ]] || exit 0
  printf 'mock guest output\000\033[31m café\n' >>"$(<"$MOCK_CONSOLE")"
  if [[ "${MOCK_DELAY:-0}" != 0 ]]; then
    printf 'split guest ' >>"$(<"$MOCK_CONSOLE")"
    sleep 2
    printf 'line\n' >>"$(<"$MOCK_CONSOLE")"
    # A verbose suite must stream before the quiet-time heartbeat, not sit
    # behind a fixed number-of-lines-per-poll throttle until guest shutdown.
    for ((index=1; index<=2000; index++)); do
      printf 'verbose test %s\n' "$index"
    done >>"$(<"$MOCK_CONSOLE")"
  fi
  sleep "${MOCK_DELAY:-0}"
  [[ "$*" =~ (test-vm\.[[:alnum:]]+) ]]
  printf 'TEST_VM_RESULT %s %s\r\n' "${BASH_REMATCH[1]}" "$MOCK_RESULT" >>"$(<"$MOCK_CONSOLE")"
  ;;
esac
SH
  cat >"$work/bin/tart" <<'SH'
#!/usr/bin/env bash
printf 'tart %s\n' "$*" >>"$MOCK_TRACE"
if [[ "$1" == get && "${MOCK_TART_MISSING:-0}" == 1 ]]; then exit 1; fi
if [[ "$1" == exec ]]; then
  case "$*" in
  *'uname -s'*) echo "$MOCK_GUEST" ;;
  *'uname -m'*) echo arm64 ;;
  *'tar -xf'*) cat >/dev/null ;;
  *'controller.sh blackbox'*) printf 'mock Tart test output\n'; exit 7 ;;
  esac
fi
SH
  chmod +x "$work/bin/id" "$work/bin/uname" "$work/bin/sw_vers" "$work/bin/qemu-img" \
    "$work/bin/qemu-system-x86_64" "$work/bin/ssh" "$work/bin/tart"
}

record_images() {
  local image_hash
  cache="$work/repo/.test-vms/freebsd-amd64"
  mkdir -p "$cache"
  cp "$work/base.qcow2" "$cache/disk.qcow2"
  (cd "$cache" && digest disk.qcow2 >disk.sha256)
  touch "$cache/ready"
  image_hash=$(digest "$work/base.qcow2")
  jq -n --arg image "$work/base.qcow2" --arg key "$work/key" --arg sha256 "${image_hash%% *}" \
    '{image:$image,sha256:$sha256,ssh_user:"root",ssh_key:$key,ssh_host_key:"ssh-ed25519 AAAA"}' >"$cache/profile.json"
  cp "$cache/profile.json" "$work/profile.json"
  mkdir -p "$work/repo/.test-vms/macos-arm64"
  touch "$work/repo/.test-vms/macos-arm64/ready"
  printf '{"image":"ghcr.io/example/macos@sha256:%064d"}\n' 0 >"$work/repo/.test-vms/macos-arm64/profile.json"
}

run_vm() {
  local expected=$1 host=$2 architecture=$3 guest=$4 target=$5 status=0
  local action=${6:-run} selection=${7-blackbox}
  set -- "$action" "$target"
  if [[ "$action" == clean-downloads ]]; then
    set -- "$action"
  elif [[ "$action" != clean ]]; then
    set -- "$@" "$selection"
  fi
  : >"$work/trace"
  { printf 'unused controller input\n' || true; } | "$timer" 40s env -i PATH="$work/bin:$PATH" HOME="$work/home" TMPDIR="$test_tmp" \
    MOCK_HOST="$host" MOCK_ARCH="$architecture" MOCK_GUEST="$guest" MOCK_RESULT="${guest_result:-9}" \
    MOCK_DELAY="${guest_delay:-0}" \
    MOCK_TART_MISSING="${tart_missing:-0}" \
    MOCK_TRACE="$work/trace" MOCK_CONSOLE="$work/console-path" MOCK_KNOWN="$work/known-path" REAL_SSH="$real_ssh" \
    bash "$work/repo/scripts/testing/vm.sh" "$@" >"$work/output" 2>&1 || status=$?
  if [[ "$status" != "$expected" ]]; then
    cat "$work/output" >&2
    printf 'expected %s, got %s\n' "$expected" "$status" >&2
    exit 1
  fi
  [[ ! -d "$work/repo/.test-vms/$target.lock" ]]
  if compgen -G "$test_tmp/test-vm.*" >/dev/null; then
    printf 'VM controller left temporary run state in %s\n' "$test_tmp" >&2
    exit 1
  fi
}

test_controller_refusals() {
  local marker status
  # Refusal precedes state creation, even for empty CI markers.
  for marker in CI CONTINUOUS_INTEGRATION GITHUB_ACTIONS GITEA_ACTIONS FORGEJO_ACTIONS GITLAB_CI \
    BUILDKITE CIRCLECI JENKINS_URL TEAMCITY_VERSION TF_BUILD DRONE WOODPECKER CODEBUILD_BUILD_ID TRAVIS; do
    status=0
    env -i PATH="$work/bin:$PATH" "$marker=" bash "$work/repo/scripts/testing/vm.sh" clean freebsd-amd64 >"$work/output" 2>&1 || status=$?
    [[ "$status" == 2 && ! -e "$work/repo/.test-vms" ]]
  done
  status=0
  env -i PATH="$work/bin:$PATH" MOCK_UID=0 bash "$work/repo/scripts/testing/vm.sh" clean freebsd-amd64 >"$work/output" 2>&1 || status=$?
  [[ "$status" == 2 && ! -e "$work/repo/.test-vms" ]]
}

test_download_cleanup() {
  local store="$work/repo/.test-vms"
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 clean
  run_vm 0 Linux x86_64 FreeBSD downloads clean-downloads
  [[ ! -e "$store" ]]
  mkdir -p "$store/downloads" "$store/results" "$store/netbsd-amd64"
  printf media >"$store/downloads/media"
  printf evidence >"$store/results/log"
  printf image >"$store/netbsd-amd64/disk.qcow2"
  mkdir "$store/openbsd-arm64.lock"
  run_vm 2 Linux x86_64 FreeBSD downloads clean-downloads
  [[ -f "$store/downloads/media" && -d "$store/openbsd-arm64.lock" ]]
  [[ ! -e "$store/freebsd-amd64.lock" && ! -e "$store/freebsd-arm64.lock" && ! -e "$store/openbsd-amd64.lock" ]]
  rmdir "$store/openbsd-arm64.lock"
  run_vm 0 Linux x86_64 FreeBSD downloads clean-downloads
  [[ ! -e "$store/downloads" && -f "$store/results/log" && -f "$store/netbsd-amd64/disk.qcow2" ]]
  if compgen -G "$store/*.lock" >/dev/null; then exit 1; fi
  ln -s "$store/results" "$store/downloads"
  run_vm 2 Linux x86_64 FreeBSD downloads clean-downloads
  [[ -f "$store/results/log" ]]
  rm -rf -- "$store"
}

test_qemu_lifecycle() {
  guest_result=missing
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64
  grep -Fq 'VM exited without a test result' "$work/output"
  guest_result=9
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64
  grep -Fq '[console] mock guest output[31m café' "$work/output"
  if grep -q $'\033' "$work/output"; then exit 1; fi
  grep -Fq 'suites=black-box | environment=vm | target=FreeBSD/amd64 | backend=qemu' "$work/output"
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64 run combined
  grep -Fq 'suites=integration + black-box | environment=vm | target=FreeBSD/amd64' "$work/output"
  grep -q -- '-accel kvm' "$work/trace"
  grep -q -- 'restrict=on' "$work/trace"
  grep -Fq 'stty 115200 </dev/ttyu0' "$work/trace"
  if grep -q -- 'qemu-img convert' "$work/trace"; then
    printf 'A test run unexpectedly prepared an image.\n' >&2
    exit 1
  fi
  mkdir "$work/space ~root"
  test_tmp="$work/space ~root"
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64
  test_tmp=$work
  run_vm 2 Linux x86_64 Darwin freebsd-amd64
  if grep -q -- 'tar -xf' "$work/trace"; then
    printf 'Source was sent to a mismatched guest.\n' >&2
    exit 1
  fi

  run_vm 2 Linux x86_64 OpenBSD openbsd-arm64
  grep -Fq 'this controller is Linux/amd64' "$work/output"
  grep -Fq 'just test-vm-prepare openbsd-amd64' "$work/output"
  grep -Fq 'just test-blackbox-vm openbsd-amd64' "$work/output"
  [[ ! -s "$work/trace" ]]

  printf 'changed image\n' >>"$cache/disk.qcow2"
  run_vm 1 Linux x86_64 FreeBSD freebsd-amd64
  [[ ! -s "$work/trace" ]]
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 clean
  [[ ! -e "$cache" ]]
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64
  grep -Fq 'First-time setup for freebsd-amd64' "$work/output"
  grep -Fq 'scripts/testing/profiles.json' "$work/output"
  grep -Fq 'just test-vm-prepare freebsd-amd64' "$work/output"
  grep -Fq 'just test-blackbox-vm freebsd-amd64' "$work/output"
  [[ ! -s "$work/trace" ]]
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64 prepare "$work/profile.json"
  [[ ! -e "$cache" ]]
  guest_result=0
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 prepare "$work/profile.json"
  [[ -f "$cache/ready" && -f "$cache/disk.sha256" ]]
  grep -q -- 'qemu-img convert' "$work/trace"
  grep -q -- 'restrict=off' "$work/trace"
  guest_result=9
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64
  if grep -q -- 'qemu-img convert' "$work/trace"; then
    printf 'A test run unexpectedly prepared an image.\n' >&2
    exit 1
  fi
}

test_progress_while_waiting() {
  local burst_line waiting_line
  guest_delay=16
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64
  grep -Fq '] Waiting: Compiling and running blackbox' "$work/output"
  grep -Fq '[console] split guest line' "$work/output"
  grep -Fq '[console] mock guest output[31m café' "$work/output"
  burst_line=$(grep -nFx '[console] verbose test 2000' "$work/output" | cut -d: -f1)
  waiting_line=$(grep -n -m1 -F '] Waiting: Compiling and running blackbox' "$work/output" | cut -d: -f1)
  ((burst_line < waiting_line))
  guest_delay=0
}

test_tart_lifecycle() {
  run_vm 7 Darwin arm64 Darwin macos-arm64
  grep -Fq '[guest] mock Tart test output' "$work/output"
  grep -q -- '--net-host' "$work/trace"
  grep -q -- 'tart clone base test-vm.' "$work/trace"
  grep -q -- 'tart delete test-vm.' "$work/trace"
  tart_missing=1
  run_vm 2 Darwin arm64 Darwin macos-arm64
  grep -Fq 'prepared Tart image is missing or unreadable' "$work/output"
  if grep -q 'tart clone' "$work/trace"; then exit 1; fi
  tart_missing=0
  run_vm 2 Linux x86_64 Darwin macos-arm64
  [[ ! -s "$work/trace" ]]
  run_vm 2 Darwin arm64 Linux linux-arm64 run container-matrix
  grep -Fxq '  just test-matrix-container-on-macos' "$work/output"
  mkdir -p "$work/repo/.test-vms/linux-arm64"
  cp "$work/repo/.test-vms/macos-arm64/profile.json" "$work/repo/.test-vms/linux-arm64/profile.json"
  touch "$work/repo/.test-vms/linux-arm64/ready"
  run_vm 2 Darwin arm64 Linux linux-arm64
  [[ ! -s "$work/trace" ]]
  jq '.distributions = ["debian-13"]' "$work/repo/.test-vms/macos-arm64/profile.json" >"$work/repo/.test-vms/linux-arm64/profile.json"
  run_vm 7 Darwin arm64 Linux linux-arm64
  grep -q -- '--net-host' "$work/trace"
  run_vm 0 Darwin arm64 Linux linux-arm64 run container-unit
  grep -Fq 'suites=unit | environment=container-in-vm | target=Linux/arm64 | backend=tart' "$work/output"
}

test_profile_errors() {
  local result
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64 prepare "$work/absent.json"
  grep -Fq 'custom profile is missing' "$work/output"
  if grep -Fq 'First-time setup' "$work/output"; then exit 1; fi
  printf '{}\n' >"$work/incomplete.json"
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64 prepare "$work/incomplete.json"
  grep -Fq 'needs a nonempty image string' "$work/output"
  [[ ! -s "$work/trace" ]]
  result="$(sed -n 's/^Logs: //p' "$work/output")/result.txt"
  grep -Fxq 'action=prepare' "$result"
  grep -Fxq 'tier=prepare' "$result"
  grep -Fxq 'controller_exit_code=2' "$result"
  printf '{"image":"https://example.invalid/image","bootstrap":"openbsd-autoinstall"}\n' >"$work/wrong-method.json"
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64 prepare "$work/wrong-method.json"
  grep -Fq "bootstrap method 'openbsd-autoinstall' does not match freebsd-amd64" "$work/output"
  [[ ! -s "$work/trace" ]]
}

test_automatic_preparation() {
  local hash
  cp "$root/scripts/testing/vm-bootstrap.sh" "$root/scripts/testing/vm-bootstrap-guest.sh" "$work/repo/scripts/testing/"
  # The stale-input check edits this private copy, including from read-only snapshots.
  chmod u+w "$work/repo/scripts/testing/vm-bootstrap.sh"
  cat >"$work/bin/curl" <<'SH'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$MOCK_TRACE"
while (($#)); do
  if [[ "$1" == --output ]]; then printf 'image\n' >"$2"; exit 0; fi
  shift
done
exit 92
SH
  cat >"$work/bin/xz" <<'SH'
#!/bin/sh
cat "$2"
SH
  cat >"$work/bin/ssh-keygen" <<'SH'
#!/usr/bin/env bash
printf 'fixture private key\n' >"${!#}"
printf 'ssh-ed25519 AAAA\n' >"${!#}.pub"
SH
  cat >"$work/bin/ssh-keyscan" <<'SH'
#!/bin/sh
printf '127.0.0.1 ssh-ed25519 AAAA\n'
SH
  cat >"$work/bin/xorriso" <<'SH'
#!/usr/bin/env bash
printf 'xorriso %s\n' "$*" >>"$MOCK_TRACE"
while (($#)); do
  if [[ "$1" == -o ]]; then
    sed '1d' "${2%/*}/seed/user-data" >"$MOCK_TRACE.seed.json"
    printf 'seed\n' >"$2"
    exit 0
  fi
  shift
done
exit 93
SH
  chmod +x "$work/bin/curl" "$work/bin/xz" "$work/bin/ssh-keygen" "$work/bin/ssh-keyscan" "$work/bin/xorriso"
  hash=$(digest "$work/base.qcow2")
  jq -n --arg hash "${hash%% *}" '{targets:{"freebsd-amd64":{
    bootstrap:"freebsd-cloudinit",image:"https://example.invalid/image.xz",
    sha256:$hash,format:"qcow2.xz",serial:"/dev/ttyu0"
  }}}' >"$work/repo/scripts/testing/profiles.json"
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 clean
  guest_result=0
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 prepare ""
  grep -q '^curl ' "$work/trace"
  grep -q '^xorriso ' "$work/trace"
  grep -Fq 'virtio-scsi-pci,id=bootstrap-scsi' "$work/trace"
  grep -Fq 'scsi-cd,bus=bootstrap-scsi.0,drive=bootstrap-cd' "$work/trace"
  jq -e '.write_files[0].content | contains("PermitRootLogin prohibit-password") and contains("PasswordAuthentication no")' "$work/trace.seed.json" >/dev/null
  jq -e '.runcmd[0] | type == "string" and startswith("ssh-keygen -A &&") and contains("sshd -t &&") and contains("ITIQUETTE_BOOTSTRAP_READY") and contains("ITIQUETTE_BOOTSTRAP_FAILED")' "$work/trace.seed.json" >/dev/null
  jq -e '.write_files[] | select(.path == "/etc/rc.conf.d/firstboot_pkg_upgrade") | .content == "firstboot_pkg_upgrade_enable=\"NO\"\n"' "$work/trace.seed.json" >/dev/null
  [[ -f "$cache/ready" && -f "$cache/input.sha256" && -f "$cache/id_ed25519" ]]
  jq -e '.repository_profile and .ssh_user == "root" and .ssh_host_key == "ssh-ed25519 AAAA"' "$cache/profile.json" >/dev/null
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 prepare ""
  [[ ! -s "$work/trace" ]]
  guest_result=9
  run_vm 9 Linux x86_64 FreeBSD freebsd-amd64
  if grep -q '^curl ' "$work/trace"; then exit 1; fi
  # Changed preparation inputs refuse execution without fetching or booting.
  printf '\n# changed fixture\n' >>"$work/repo/scripts/testing/vm-bootstrap.sh"
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64
  [[ ! -s "$work/trace" ]]
  # A bad download never becomes an image or a ready recording.
  run_vm 0 Linux x86_64 FreeBSD freebsd-amd64 clean
  jq '.targets["freebsd-amd64"].sha256 = ("0" * 64)' "$work/repo/scripts/testing/profiles.json" >"$work/bad-profile.json"
  mv "$work/bad-profile.json" "$work/repo/scripts/testing/profiles.json"
  run_vm 2 Linux x86_64 FreeBSD freebsd-amd64 prepare ""
  [[ ! -e "$cache" ]]
  if grep -q '^qemu ' "$work/trace"; then exit 1; fi
}

# shellcheck disable=SC2030,SC2031 # Handoff fixtures isolate controller globals.
test_openbsd_console_handoff() (
  # Replay the installer prompts, without booting or executing a guest helper.
  # shellcheck source=scripts/testing/vm-bootstrap.sh
  source "$root/scripts/testing/vm-bootstrap.sh"
  bootstrap=openbsd-autoinstall backend=qemu http_port=12345 ssh_port=2222 logs=$work
  printf '(I)nstall, (U)pgrade, (A)utoinstall or (S)hell?\nResponse file location?\nCONGRATULATIONS! Your OpenBSD install has been successfully completed!\n' >"$work/openbsd-console"
  bootstrap_wait_console() { grep -Eq "$1" "$work/openbsd-console" || exit 71; }
  configure_ssh() { :; }
  exec 3>"$work/openbsd-input"
  PATH="$work/bin:$PATH" bootstrap_connect
  [[ $(<"$work/openbsd-input") == $'A\nhttp://10.0.2.2:12345/install.conf' ]]
)

# shellcheck disable=SC2030,SC2031 # Handoff fixtures isolate controller globals.
test_freebsd_init_failure() (
  # A nuageinit parser failure must not become a twenty-minute readiness wait.
  # shellcheck source=scripts/testing/vm-bootstrap.sh
  source "$root/scripts/testing/vm-bootstrap.sh"
  bootstrap=freebsd-cloudinit logs=$work pid=$$ status=0
  printf '/usr/libexec/flua: /usr/libexec/nuageinit:514: attempt to concatenate a table value\n' >"$work/console.log"
  fail() {
    printf '%s\n' "$*" >&2
    exit 2
  }
  (bootstrap_wait_console '^ITIQUETTE_BOOTSTRAP_READY' 1) >"$work/output" 2>&1 || status=$?
  [[ "$status" == 2 ]]
  grep -Fq 'FreeBSD initialization failed' "$work/output"
)

test_bsd_interfaces() {
  local network_function interfaces status=0
  network_function=$(sed -n '/^bsd_interfaces() {$/,/^}/p' "$root/scripts/testing/vm-guest.sh")
  [[ -n "$network_function" ]]
  cat >"$work/bin/ifconfig" <<'SH'
#!/bin/sh
test "$1" = -a || exit 44
test "${MOCK_IFCONFIG_FAIL:-}" != 1 || exit 73
printf 'lo0: flags=8049<UP,LOOPBACK,RUNNING>\n\tinet 127.0.0.1\nvio0: flags=8843<UP,BROADCAST,RUNNING>\n'
SH
  chmod +x "$work/bin/ifconfig"
  interfaces=$(PATH="$work/bin:$PATH" bash -euo pipefail -c "$network_function; bsd_interfaces")
  [[ "$interfaces" == $'lo0\nvio0' ]]
  PATH="$work/bin:$PATH" MOCK_IFCONFIG_FAIL=1 bash -euo pipefail -c "$network_function; bsd_interfaces" || status=$?
  [[ "$status" == 73 ]]
}

main() {
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
  work=$(mktemp -d "${TMPDIR:-/tmp}/vm-contract.XXXXXXXX")
  trap 'rm -rf -- "$work"' EXIT
  real_ssh=$(command -v ssh) || {
    printf 'VM controller contract tests require OpenSSH ssh for offline argument validation.\n' >&2
    exit 2
  }
  test_tmp=$work
  if command -v timeout >/dev/null 2>&1; then
    timer=timeout
  else
    timer=gtimeout
  fi
  mkdir -p "$work/repo/scripts/testing" "$work/bin" "$work/home"
  cp "$root/scripts/testing/vm.sh" "$work/repo/scripts/testing/vm.sh"
  git -C "$work/repo" init -q
  printf 'fixture\n' >"$work/repo/source.txt"
  printf 'image\n' >"$work/base.qcow2"
  printf 'fixture identity\n' >"$work/key"
  write_mocks
  test_controller_refusals
  test_download_cleanup
  record_images
  test_qemu_lifecycle
  test_progress_while_waiting
  test_tart_lifecycle
  test_profile_errors
  test_automatic_preparation
  test_openbsd_console_handoff
  test_freebsd_init_failure
  test_bsd_interfaces
  printf 'VM controller contracts passed (no real guests).\n'
}

main "$@"
