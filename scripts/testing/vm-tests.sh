#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0

# Project-owned commands for the VM adapter. No engine or host setup lives here.
set -euo pipefail

prepare_toolchain() (
  # Install the exact go.mod version into the disposable test user's home.
  # HTTPS metadata supplies the checksum for that version/platform, never latest.
  local version os filename checksum stage
  version=$(sed -n 's/^go //p' go.mod)
  if command -v go >/dev/null 2>&1 && [[ $(GOTOOLCHAIN=local go env GOVERSION) == "go$version" ]]; then
    return
  fi
  [[ "$version" =~ ^[0-9]+[.][0-9]+[.][0-9]+$ ]] || exit 2
  os=$(printf '%s' "$VM_KERNEL" | tr '[:upper:]' '[:lower:]')
  filename="go$version.$os-$VM_ARCH.tar.gz"
  stage=$(mktemp -d "${TMPDIR:?}/go-toolchain.XXXXXXXX")
  trap 'rm -rf -- "$stage"' EXIT
  curl --fail --location --proto '=https' --proto-redir '=https' --max-time 120 \
    'https://go.dev/dl/?mode=json&include=all' >"$stage/releases.json"
  checksum=$(jq -er --arg file "$filename" \
    '[.[].files[] | select(.filename == $file) | .sha256] | select(length == 1) | .[0] | select(test("^[0-9a-f]{64}$"))' "$stage/releases.json")
  curl --fail --location --proto '=https' --proto-redir '=https' --retry 2 --max-time 600 \
    -o "$stage/go.tar.gz" "https://go.dev/dl/$filename"
  printf '%s  %s\n' "$checksum" "$stage/go.tar.gz" | sha256sum -c -
  tar -xzf "$stage/go.tar.gz" -C "$stage"
  mkdir -p "$HOME/.local"
  mv "$stage/go" "$HOME/.local/go-$version"
)

check_toolchain() {
  local required
  export GOTOOLCHAIN=local GOENV=off GOFLAGS=-mod=readonly GOMAXPROCS=2
  required=$(sed -n 's/^go //p' go.mod)
  [[ $(go env GOVERSION) == "go$required" ]] || {
    printf 'VM needs Go %s.\n' "$required" >&2
    exit 2
  }
  [[ $(go env GOHOSTOS) == "$(printf '%s' "$VM_KERNEL" | tr '[:upper:]' '[:lower:]')" && $(go env GOHOSTARCH) == "$VM_ARCH" ]] || {
    printf 'Go toolchain does not match the native guest.\n' >&2
    exit 2
  }
}

prepare_dependencies() {
  local argument just_command
  go mod download
  if [[ "$VM_KERNEL" == Linux ]]; then
    git init -q
    git add --all
    mise trust .mise.toml
    mise install --locked just
    just_command=$(mise which just)
    "$just_command" test-container-prepare
    set --
    while IFS= read -r -d '' argument; do
      set -- "$@" "$argument"
    done <"${VM_ARGS:?}"
    (($# > 0)) || exit 2
    "$just_command" test-container-prepare-distributions "$@"
    mkdir -p "$HOME/vm-container-state"
    cp "$VM_ARGS" "$HOME/vm-container-state/distributions"
    cp -R bin/sandbox-image bin/distribution-images "$HOME/vm-container-state/"
  fi
}

restore_container_state() {
  mkdir -p bin
  cp -R "$HOME/vm-container-state/sandbox-image" "$HOME/vm-container-state/distribution-images" bin/
  git init -q
  git add --all
}

run_containers() {
  local argument module_cache just_command
  just_command=$(mise which just)
  set --
  while IFS= read -r -d '' argument; do
    set -- "$@" "$argument"
  done <"${VM_ARGS:?}"
  if [[ "$tier" == container-matrix && $# == 0 ]]; then
    while IFS= read -r -d '' argument; do
      set -- "$@" "$argument"
    done <"$HOME/vm-container-state/distributions"
  fi
  module_cache=$(go env GOMODCACHE)
  export GOMMITLINT_MODULE_PROXY="file://$module_cache/cache/download"
  case "$tier" in
  container-unit) "$just_command" test-unit-container "$@" ;;
  container-blackbox) "$just_command" test-blackbox-container "$@" ;;
  container-combined) "$just_command" test-combined-container "$@" ;;
  container-matrix) "$just_command" test-combined-container-matrix "$@" ;;
  *) exit 2 ;;
  esac
}

build_suite() {
  local suite=$1 tags='' packages package directory relative executable
  local -a flags=(-trimpath -buildvcs=false)
  case "$suite" in
  unit)
    packages=$(go list ./... | grep -v generated)
    [[ "$tier" != race ]] || flags+=(-race)
    ;;
  integration)
    tags=integration
    flags+=(-tags=integration)
    packages=$(go list -tags=integration -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./test/integration/...)
    ;;
  blackbox)
    tags=blackbox
    flags+=(-tags=blackbox)
    packages=$(go list -tags=blackbox ./test/blackbox)
    ;;
  esac
  while IFS= read -r package; do
    [[ -n "$package" ]] || continue
    directory=$(go list -tags="$tags" -f '{{.Dir}}' "$package")
    relative=${directory#"$VM_SOURCE"/}
    executable="${suite}-${relative//\//-}.test"
    printf '\n== Compiling %s test harness: %s\n' "$suite" "$package"
    go test -v -c "${flags[@]}" -o "$artifacts/$executable" "$package"
    [[ -f "$artifacts/$executable" ]] || continue
    printf '%s\t%s\n' "$directory" "$executable" >>"$artifacts/tests.tsv"
    (cd "$artifacts" && sha256sum "$executable" >>SHA256SUMS)
  done <<<"$packages"
}

build_tests() {
  local suite
  local -a suites=()
  mkdir -p "$artifacts"
  : >"$artifacts/tests.tsv"
  : >"$artifacts/SHA256SUMS"
  if [[ "$tier" == blackbox || "$tier" == combined ]]; then
    printf '\n== Compiling gommitlint (dependency packages follow on a cold cache)\n'
    go build -v -trimpath -buildvcs=false -o "$artifacts/gommitlint" ./cmd/gommitlint
    (cd "$artifacts" && sha256sum gommitlint >>SHA256SUMS)
  fi
  case "$tier" in
  unit | race) suites=(unit) ;;
  integration) suites=(integration) ;;
  blackbox) suites=(blackbox) ;;
  combined) suites=(integration blackbox) ;;
  esac
  for suite in "${suites[@]}"; do
    build_suite "$suite"
  done
  [[ -s "$artifacts/tests.tsv" ]]
}

run_tests() {
  local directory executable status=0
  ulimit -c 0
  ulimit -n 1024
  ulimit -u 512
  (cd "$artifacts" && sha256sum -c SHA256SUMS)
  export GOMMITLINT_BLACKBOX_BIN="$artifacts/gommitlint"
  while IFS=$'\t' read -r directory executable; do
    printf '\n== %s\n' "$directory"
    # Match go test's non-terminal streams while forwarding results live.
    (cd "$directory" && "$artifacts/$executable" -test.v -test.count=1 -test.timeout=10m) 2>&1 | cat || status=1
  done <"$artifacts/tests.tsv"
  return "$status"
}

main() {
  local go_version
  [[ $(id -un) == tester && -f /var/run/itiquette-vm/controller.sh ]] || exit 2
  phase=$1 tier=${2:-unit}
  cd "${VM_SOURCE:?}"
  artifacts=${VM_ARTIFACTS:?}
  go_version=$(sed -n 's/^go //p' go.mod)
  export PATH="$HOME/.local/go-$go_version/bin:$PATH"
  if [[ "$phase" == prepare ]]; then prepare_toolchain; fi
  check_toolchain

  if [[ "$phase" == prepare ]]; then
    prepare_dependencies
    return
  fi
  export GOPROXY=off GOSUMDB=off MISE_OFFLINE=1
  if [[ "$tier" == container-* ]]; then
    [[ "$VM_KERNEL" == Linux ]] || exit 2
    case "$phase" in
    build) restore_container_state ;;
    run) run_containers ;;
    *) exit 2 ;;
    esac
    return
  fi

  case "$tier" in
  unit | race | integration | blackbox | combined) ;;
  *)
    printf 'Unsupported Gommitlint VM tier: %s\n' "$tier" >&2
    exit 2
    ;;
  esac
  case "$phase" in
  build) build_tests ;;
  run) run_tests ;;
  *) exit 2 ;;
  esac
}

main "$@"
