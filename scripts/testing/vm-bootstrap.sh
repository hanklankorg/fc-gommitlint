#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Itiquette Authors
# SPDX-License-Identifier: CC0-1.0

# Sourced by vm.sh only during explicit automatic preparation.
# The controller owns the lock, temporary directory, processes and cleanup.
# shellcheck disable=SC2154,SC2034 # Controller state and outputs shared with vm.sh.

bootstrap_requirements() {
  need curl
  if [[ "$backend" == qemu ]]; then
    need ssh-keygen
    need ssh-keyscan
    case "$bootstrap" in
    freebsd-cloudinit)
      need xz
      need xorriso
      ;;
    openbsd-autoinstall)
      need xorriso
      need python3
      ;;
    netbsd-cloud)
      need gzip
      need python3
      if ! command -v sha512sum >/dev/null 2>&1; then need shasum; fi
      ;;
    *) fail "unknown bootstrap method: $bootstrap" ;;
    esac
  fi
}

bootstrap_checksum() {
  local bits=$1 file=$2 actual
  if command -v "sha${bits}sum" >/dev/null 2>&1; then
    actual=$("sha${bits}sum" "$file")
  else
    actual=$(shasum -a "$bits" "$file")
  fi
  printf '%s\n' "${actual%% *}"
}

bootstrap_download() {
  local expected bits=256 cached downloaded="$scratch/download" unpacked
  expected=$(jq -er '.sha256 // .sha512' "$profile")
  if [[ "$expected" =~ ^[0-9a-f]{128}$ ]]; then
    bits=512
  else
    [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || fail 'profile needs a SHA-256 or SHA-512 image checksum'
  fi
  [[ "$image" == https://* ]] || fail 'automatic QEMU images must use HTTPS'
  image_format=$(jq -er '.format | select(. == "iso" or . == "raw.gz" or . == "qcow2.xz")' "$profile")
  unpacked=$(jq -r '.checksum_unpacked // false' "$profile")
  [[ "$unpacked" == false || "$image_format" == raw.gz ]] || fail 'unpacked checksums require raw.gz'
  [[ ! -L "$store/downloads" ]] || fail 'download cache must not be a symlink'
  mkdir -p "$store/downloads"
  cached="$store/downloads/$expected"
  [[ ! -L "$cached" ]] || fail 'cached image must not be a symlink'
  if [[ ! -f "$cached" ]] || [[ $(bootstrap_checksum "$bits" "$cached") != "$expected" ]]; then
    printf 'Downloading pinned image: %s\n' "$image"
    curl --fail --location --proto '=https' --proto-redir '=https' --retry 2 \
      --connect-timeout 15 --max-time 1800 --output "$downloaded" "$image"
    if [[ "$unpacked" == true ]]; then
      gzip -dc "$downloaded" >"$scratch/unpacked"
      downloaded="$scratch/unpacked"
    fi
    [[ $(bootstrap_checksum "$bits" "$downloaded") == "$expected" ]] || fail 'downloaded image checksum differs from the repository profile'
    # Publish on the cache filesystem only after verification; interrupted
    # copies remain in the preparation directory owned by the cleanup trap.
    cp "$downloaded" "$cache/.download-ready"
    chmod 0444 "$cache/.download-ready"
    mv -f "$cache/.download-ready" "$cached"
  fi
  base_image="$scratch/base-image"
  case "$image_format:$unpacked" in
  iso:*) base_image=$cached ;;
  raw.gz:false) gzip -dc "$cached" >"$base_image" ;;
  raw.gz:true) cp "$cached" "$base_image" ;;
  qcow2.xz:*) xz -dc "$cached" >"$base_image" ;;
  esac
}

bootstrap_firmware() {
  local candidate prefix
  [[ "$guest_arch" == arm64 ]] || return 0
  prefix=$(dirname "$(dirname "$(command -v "$qemu")")")
  for candidate in "$prefix/share/qemu/edk2-aarch64-code.fd" \
    /opt/homebrew/share/qemu/edk2-aarch64-code.fd \
    /usr/share/AAVMF/AAVMF_CODE.fd /usr/share/qemu-efi-aarch64/QEMU_EFI.fd; do
    if [[ -f "$candidate" ]]; then
      cp "$candidate" "$cache/firmware"
      return
    fi
  done
  fail 'ARM QEMU firmware is missing. Install qemu-efi-aarch64 on Linux or QEMU with Homebrew on macOS.'
}

bootstrap_http() {
  # A loopback-only, temporary file server for the guest's installer/metadata.
  python3 -u -c '
import functools, http.server, sys
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=sys.argv[1])
server = http.server.HTTPServer(("127.0.0.1", 0), handler)
print(server.server_port, flush=True)
server.serve_forever()
' "$scratch/http" >"$scratch/http-port" 2>"$logs/bootstrap-http.log" &
  http_pid=$!
  local deadline=$((SECONDS + 10))
  until [[ -s "$scratch/http-port" ]]; do
    kill -0 "$http_pid" 2>/dev/null || fail 'bootstrap HTTP server exited; inspect bootstrap-http.log'
    ((SECONDS < deadline)) || fail 'bootstrap HTTP server did not become ready'
    sleep 1
  done
  http_port=$(<"$scratch/http-port")
  [[ "$http_port" =~ ^[0-9]+$ ]] || fail 'invalid bootstrap HTTP port'
}

bootstrap_attach_cd() {
  local block
  block=$(jq -nc --arg path "$1" \
    '{driver:"raw","read-only":true,file:{driver:"file",filename:$path},"node-name":"bootstrap-cd"}')
  # Explicit SCSI works on both q35 and ARM virt; -cdrom assumes a machine's
  # default drive interface. JSON also preserves commas/spaces in file paths.
  qemu_extra+=(-device 'virtio-scsi-pci,id=bootstrap-scsi' -blockdev "$block"
    -device 'scsi-cd,bus=bootstrap-scsi.0,drive=bootstrap-cd')
}

bootstrap_openbsd_iso() {
  # Keep the upstream installer kernel path, including EFI boot.conf settings.
  xorriso -osirrox on -indev "$base_image" -extract /etc/boot.conf "$scratch/boot.conf.base" >>"$logs/seed.log" 2>&1
  printf 'stty com0 115200\nset tty com0\nset timeout 1\n' >"$scratch/boot.conf"
  cat "$scratch/boot.conf.base" >>"$scratch/boot.conf"
  # cdbr reads ECMA-119 names, not Rock Ridge. Without this rule xorriso
  # renames 7.9 to 7_9, hiding cdboot before serial output can start.
  xorriso -indev "$base_image" -outdev "$scratch/install.iso" \
    -compliance allow_dir_id_ext -map "$scratch/boot.conf" /etc/boot.conf \
    -boot_image any replay >>"$logs/seed.log" 2>&1
}

bootstrap_seed() {
  local public_key release proxy
  public_key=$(<"$identity.pub")
  case "$bootstrap" in
  freebsd-cloudinit)
    mkdir "$scratch/seed"
    printf 'instance-id: %s\nlocal-hostname: itiquette-test\n' "$name" >"$scratch/seed/meta-data"
    printf '#cloud-config\n' >"$scratch/seed/user-data"
    jq -n --arg key "$public_key" \
      '{disable_root:false,ssh_pwauth:false,users:[{name:"root",ssh_authorized_keys:[$key]}],
        write_files:[{path:"/etc/ssh/sshd_config",owner:"root:wheel",permissions:"0600",
          content:"HostKey /etc/ssh/ssh_host_ed25519_key\nPermitRootLogin prohibit-password\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM yes\nUseDNS no\nAuthorizedKeysFile .ssh/authorized_keys\n"},
          {path:"/etc/rc.conf.d/firstboot_pkg_upgrade",owner:"root:wheel",permissions:"0644",
          content:"firstboot_pkg_upgrade_enable=\"NO\"\n"}],
        # FreeBSD nuageinit accepts command strings, not cloud-init argv lists.
        # It runs before normal sshd startup, so generate the host keys first.
        runcmd:["ssh-keygen -A && /usr/sbin/sshd -t && printf \"\\n%s\\n\" ITIQUETTE_BOOTSTRAP_READY > /dev/ttyu0 || { printf \"\\n%s\\n\" ITIQUETTE_BOOTSTRAP_FAILED > /dev/ttyu0; exit 1; }"]
      }' >>"$scratch/seed/user-data"
    xorriso -as mkisofs -quiet -V cidata -J -r -o "$scratch/seed.iso" "$scratch/seed" >"$logs/seed.log" 2>&1
    bootstrap_attach_cd "$scratch/seed.iso"
    ;;
  openbsd-autoinstall)
    release=$(jq -er '.release | select(test("^[0-9]+[.][0-9]+$"))' "$profile")
    printf 'Preparing OpenBSD installation media; details: %s/seed.log\n' "$logs"
    mkdir -p "$scratch/http/sets"
    xorriso -osirrox on -indev "$base_image" -extract "/$release/$guest_arch" "$scratch/http/sets" >"$logs/seed.log" 2>&1
    # The ISO includes SHA256, but HTTP installation requires SHA256.sig.
    # The installer verifies this list with the release key bundled in the ISO.
    printf 'Fetching the signed OpenBSD set list\n'
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
      --connect-timeout 15 --max-time 120 --retry 2 --retry-max-time 300 --max-filesize 1048576 \
      --output "$scratch/http/sets/SHA256.sig" "${image%/*}/SHA256.sig"
    printf '/ 1G-* 100%%\n' >"$scratch/http/disklabel"
    bootstrap_http
    cat >"$scratch/http/install.conf" <<EOF
System hostname = itiquette-test
Change the default console to com0 = yes
Which speed should com0 use = 115200
Password for root = *************
Public ssh key for root account = $public_key
Start sshd(8) by default = yes
Allow root ssh login = prohibit-password
Setup a user = no
Network interface to configure = vio0
IPv4 address for vio0 = dhcp
IPv6 address for vio0 = none
Which disk is the root disk = sd0
Use (W)hole disk = whole
Use (A)uto layout = auto
URL to autopartitioning template for disklabel = http://10.0.2.2:$http_port/disklabel
Location of sets = http
HTTP Server = 10.0.2.2:$http_port
Server directory = sets
Set name(s) = -game* -x* +xbase*
What timezone are you in = UTC
EOF
    bootstrap_openbsd_iso
    bootstrap_attach_cd "$scratch/install.iso"
    qemu_extra+=(-boot once=d)
    mkfifo "$scratch/serial.in" "$scratch/serial.out"
    # Keep the input FIFO open; only constant installer answers are sent to it.
    exec 3<>"$scratch/serial.in"
    cat "$scratch/serial.out" >"$logs/console.log" &
    console_pid=$!
    serial_backend="pipe:$scratch/serial"
    ;;
  netbsd-cloud)
    # Upstream NetBSD images already contain ec2_init. Feed its OCI variant
    # from local files; this does not connect to a real cloud metadata service.
    mkdir -p "$scratch/http/opc/v1/instance/metadata" "$scratch/http/openstack/latest"
    printf 'itiquette-test\n' >"$scratch/http/opc/v1/instance/hostname"
    printf '%s\n' "$public_key" >"$scratch/http/opc/v1/instance/metadata/ssh_authorized_keys"
    printf '{}\n' >"$scratch/http/openstack/latest/meta_data.json"
    bootstrap_http
    # DHCP stays outside 169.254/16, while the metadata address is in the /8.
    # A fresh relay per connection propagates HTTP EOF to NetBSD ftp. A TCP
    # chardev or netcat keeps the stream open and makes metadata fetches stall.
    printf -v proxy 'python3 %q %s' "$repo/scripts/testing/vm-metadata-proxy.py" "$http_port"
    # QEMU's key/value option syntax escapes literal commas by doubling them.
    # Metadata uses IPv4. Avoid advertising unusable IPv6 through user-mode
    # networking: pkg_add otherwise stalls on IPv6 when the host has no route.
    bootstrap_net="ipv6=off,net=169.0.0.0/8,host=169.0.2.2,dhcpstart=169.0.2.15,guestfwd=tcp:169.254.169.254:80-cmd:${proxy//,/,,},"
    qemu_extra+=(-smbios 'type=1,manufacturer=OpenStack' -smbios 'type=3,asset=OracleCloud')
    login=opc
    ;;
  esac
}

bootstrap_image() {
  if [[ "$backend" == tart ]]; then
    "$timer" --kill-after=15s 30m tart clone "$image" base
    tart set base --disk-size 64
    return
  fi
  bootstrap_firmware
  bootstrap_download
  identity="$cache/id_ed25519"
  ssh-keygen -q -t ed25519 -N '' -C itiquette-test -f "$identity"
  login=root
  if [[ "$image_format" == iso ]]; then
    qemu-img create -q -f qcow2 "$cache/disk.qcow2" 64G
  else
    local format=raw
    [[ "$image_format" != qcow2.xz ]] || format=qcow2
    qemu-img convert -f "$format" -O qcow2 "$base_image" "$cache/disk.qcow2"
    qemu-img resize "$cache/disk.qcow2" 64G
  fi
  bootstrap_seed
}

bootstrap_wait_console() {
  local pattern=$1 deadline=$((SECONDS + ${2:-1200}))
  until grep -Eq "$pattern" "$logs/console.log"; do
    if [[ "$bootstrap" == freebsd-cloudinit ]] && grep -Eq '/usr/libexec/flua: /usr/libexec/nuageinit:[0-9]+:|^ITIQUETTE_BOOTSTRAP_FAILED' "$logs/console.log"; then
      fail "FreeBSD initialization failed; inspect $logs/console.log"
    fi
    if [[ "$bootstrap" == openbsd-autoinstall ]] && grep -Fq 'failed; check /tmp/ai/ai.log' "$logs/console.log"; then
      fail "OpenBSD unattended installation failed; inspect $logs/console.log"
    fi
    kill -0 "$pid" 2>/dev/null || fail 'installer VM exited; inspect console.log'
    ((SECONDS < deadline)) || fail "installer did not reach '$pattern'; inspect console.log"
    sleep 1
  done
}

bootstrap_connect() {
  if [[ "$backend" != qemu ]]; then
    return
  fi
  if [[ "$bootstrap" == freebsd-cloudinit ]]; then
    # Cloud-init may replace host keys. Pin the key only after its final stage
    # has enabled key-only root access and restarted sshd.
    bootstrap_wait_console '^ITIQUETTE_BOOTSTRAP_READY'
  elif [[ "$bootstrap" == openbsd-autoinstall ]]; then
    printf 'Waiting for the OpenBSD installer (timeout 3m); console: %s/console.log\n' "$logs"
    bootstrap_wait_console '\(A\)utoinstall' 180
    printf 'A\n' >&3
    bootstrap_wait_console 'Response file location'
    printf 'http://10.0.2.2:%s/install.conf\n' "$http_port" >&3
    printf 'Installing OpenBSD (timeout 20m); console: %s/console.log\n' "$logs"
    bootstrap_wait_console 'CONGRATULATIONS|Installation complete'
    # Successful autoinstall reboots automatically; no extra keystrokes.
  fi
  # First contact is with the fresh VM we just booted on this loopback forward.
  # Record its host key locally; every actual SSH command uses strict checking.
  local deadline=$((SECONDS + 1200)) keys
  until keys=$(ssh-keyscan -T 3 -p "$ssh_port" -t ed25519 127.0.0.1 2>/dev/null) && [[ -n "$keys" ]]; do
    kill -0 "$pid" 2>/dev/null || fail 'bootstrap VM exited; inspect console.log'
    ((SECONDS < deadline)) || fail 'guest SSH did not become ready; inspect console.log'
    sleep 2
  done
  hostkey=$(printf '%s\n' "$keys" | awk '$2 == "ssh-ed25519" {print $2 " " $3}')
  [[ "$hostkey" =~ ^ssh-ed25519\ [A-Za-z0-9+/=]+$ ]] || fail 'guest returned an ambiguous SSH host key'
  configure_ssh
}

bootstrap_guest_tools() {
  # POSIX sh is available before Bash/sudo. Install only in the verified guest.
  local installer=/var/run/itiquette-vm/bootstrap.sh project=gommitlint command
  [[ ! -f "$repo/rust-toolchain.toml" ]] || project=nanolinter
  command="sh $installer $kernel $project"
  if [[ "$bootstrap" == netbsd-cloud ]]; then
    # Only on first boot of the verified upstream image: its cloud user is in
    # wheel and root has an empty password. Switch to key-only root SSH below.
    "$timer" 30s "${remote[@]}" su -m root -c \
      "'mkdir -p /var/run/itiquette-vm; cat > $installer; chmod 0500 $installer'" <"$repo/scripts/testing/vm-bootstrap-guest.sh"
    "$timer" --kill-after=15s 30m "${remote[@]}" su -m root -c "'$command'" >"$logs/bootstrap.log" 2>&1
    login=root
    configure_ssh
    return
  fi
  remote_shell 'mkdir -p /var/run/itiquette-vm; cat > /var/run/itiquette-vm/bootstrap.sh; chmod 0500 /var/run/itiquette-vm/bootstrap.sh' \
    <"$repo/scripts/testing/vm-bootstrap-guest.sh"
  if [[ "$backend" == qemu ]]; then
    "$timer" --kill-after=15s 30m "${remote[@]}" "${root[@]}" sh -c "'$command'" >"$logs/bootstrap.log" 2>&1
  else
    "$timer" --kill-after=15s 30m "${remote[@]}" "${root[@]}" sh -c "$command" >"$logs/bootstrap.log" 2>&1
  fi
}
