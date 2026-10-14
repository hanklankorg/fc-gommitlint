#!/usr/bin/env bash

# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

set -euo pipefail
# shellcheck source=.forgejo/scripts/sandbox-common.sh
source "$(dirname "$0")/sandbox-common.sh"
sandbox_refuse_ci_product_mode "${1:-source}"
sandbox_clear_ambient_container_env
sandbox_reexec_clean "$@"
sandbox_clear_ambient_container_env

# Bounds for the harness container and for go test inside it. Slow runners
# can raise both without editing this file, e.g.
#   GOMMITLINT_EXTERNAL_RUN_TIMEOUT=30m GOMMITLINT_EXTERNAL_TEST_TIMEOUT=20m
readonly run_timeout=${GOMMITLINT_EXTERNAL_RUN_TIMEOUT:-15m}
readonly test_timeout=${GOMMITLINT_EXTERNAL_TEST_TIMEOUT:-10m}
[[ "$run_timeout" =~ ^[1-9][0-9]*(s|m|h)$ && "$test_timeout" =~ ^[1-9][0-9]*(s|m|h)$ ]] || {
  printf 'external-test sandbox: invalid timeout override %q or %q\n' "$run_timeout" "$test_timeout" >&2
  exit 2
}
readonly suite=${1:-source}
(($# == 0)) || shift

source_root=''
source_manifest=''
source_manifest_checksum=''
source_bin=''
source_checksum=''
sandbox_image=''
sandbox_image_checksum=''
large_history_budget=''
artifact_output=''
distribution=''
harness_root=''
harness_manifest=''
harness_manifest_checksum=''
fuzztime='10s'
# Everything after `--` is handed to `go test` in the container as positional
# parameters, never as text inside the command string.
test_args=()
while (($# > 0)); do
  case "$1" in
  --)
    shift
    test_args=("$@")
    break
    ;;
  --source-root | --source-manifest | --source-manifest-checksum | --source-bin | --source-checksum | --sandbox-image | --sandbox-image-checksum | --large-history-budget | --artifact-output | --fuzztime | --distribution | --harness-root | --harness-manifest | --harness-manifest-checksum)
    (($# >= 2)) && [[ -n "$2" ]] || {
      printf 'external-test sandbox: %s requires a value\n' "$1" >&2
      exit 2
    }
    case "$1" in
    --source-root) source_root=$2 ;;
    --source-manifest) source_manifest=$2 ;;
    --source-manifest-checksum) source_manifest_checksum=$2 ;;
    --source-bin) source_bin=$2 ;;
    --source-checksum) source_checksum=$2 ;;
    --sandbox-image) sandbox_image=$2 ;;
    --sandbox-image-checksum) sandbox_image_checksum=$2 ;;
    --large-history-budget) large_history_budget=$2 ;;
    --artifact-output) artifact_output=$2 ;;
    --fuzztime) fuzztime=$2 ;;
    --distribution) distribution=$2 ;;
    --harness-root) harness_root=$2 ;;
    --harness-manifest) harness_manifest=$2 ;;
    --harness-manifest-checksum) harness_manifest_checksum=$2 ;;
    esac
    shift 2
    ;;
  *)
    printf 'external-test sandbox: unknown option %s\n' "$1" >&2
    exit 2
    ;;
  esac
done

# Unit and integration commands consume "$@" as literal arguments. The
# other commands would drop extras silently, which is how a stray argument turns
# into a run that is not the one that was asked for.
((${#test_args[@]} == 0)) || [[ "$suite" == integration || "$suite" == unit ]] || {
  printf 'external-test sandbox: extra test arguments are only accepted by unit or integration, not %s\n' "$suite" >&2
  exit 2
}
[[ -z "$large_history_budget" || "$large_history_budget" =~ ^[0-9]+(ms|s|m|h)$ ]] || {
  printf 'external-test sandbox: invalid large-history budget %q\n' "$large_history_budget" >&2
  exit 2
}
[[ -n "$sandbox_image" && -n "$sandbox_image_checksum" ]] || {
  printf 'external-test sandbox: sandbox image artifacts are required\n' >&2
  exit 2
}

# The test commands run inside the sandbox shell, which expands TEST_TIMEOUT.
# Announce compilation before Go's first test output, then show names/results.
# shellcheck disable=SC2016
case "$suite" in
unit)
  test_command='
printf "== unit tests: compiling and running with the race detector (cold compilation can take several minutes)\n" >&2
go test -v -count=1 -race -buildvcs=false -timeout="$TEST_TIMEOUT" "$@" $(go list ./... | grep -v generated)
'
  ;;
source)
  test_command='
printf "== integration tests: compiling and running\n" >&2
go test -v -tags=integration -count=1 -buildvcs=false -timeout="$TEST_TIMEOUT" ./test/integration/...
printf "== black-box tests: compiling and running\n" >&2
go test -v -tags=blackbox -count=1 -buildvcs=false -timeout="$TEST_TIMEOUT" ./test/blackbox
'
  ;;
integration)
  test_command='
printf "== integration tests: compiling and running\n" >&2
go test -v -tags=integration -count=1 -buildvcs=false -timeout="$TEST_TIMEOUT" ./test/integration/... "$@"
'
  ;;
blackbox)
  test_command='
printf "== black-box tests: compiling and running\n" >&2
go test -v -tags=blackbox -count=1 -buildvcs=false -timeout="$TEST_TIMEOUT" ./test/blackbox
'
  ;;
distribution)
  # The image has no Go: the harnesses arrive precompiled under
  # /artifacts/harness at their package paths, and each runs from that
  # package's directory of the read-only source, as go test would run it.
  # Every harness runs even after one fails, so a distribution reports all of
  # its breakage at once; any failure fails the tier.
  test_command='
printf "== %s\n" "$distribution"
git --version
IFS= read -r gpg_version < <(gpg --version)
printf "%s\n" "$gpg_version"
ssh -V
shopt -s globstar nullglob
harnesses=(/artifacts/harness/**/*.test)
test "${#harnesses[@]}" -gt 0
status=0
for harness in "${harnesses[@]}"; do
    package=${harness#/artifacts/harness/}
    package=${package%/*}
    printf "\n== %s\n" "$package"
    (cd "/workspace/$package" && "$harness" -test.v -test.count=1 -test.timeout="$TEST_TIMEOUT") || status=1
done
exit "$status"
'
  ;;
fuzz)
  # Expanded by the inner sandbox shell, never by this host-side launcher.
  # shellcheck disable=SC2016
  test_command='
mapfile -t files < <(grep -rl "^func Fuzz" --include="*_test.go" . | sort)
test "${#files[@]}" -gt 0
counter=0
for file in "${files[@]}"; do
    package=$(dirname "$file")
    mapfile -t targets < <(grep -hoE "^func (Fuzz[A-Za-z0-9_]+)" "$file" | sed "s/^func //")
    test "${#targets[@]}" -gt 0
    for target in "${targets[@]}"; do
        printf "Fuzzing %s in %s\n" "$target" "$package"
        counter=$((counter + 1))
        campaign=$(printf "campaign-%04d" "$counter")
        campaign_root=/fuzz-campaigns/$campaign
        mkdir -p "$campaign_root/cache" "$campaign_root/work"
        printf "%s\t%s\t%s\n" "$package" "$target" "$campaign" >>/fuzz-campaigns/index.tsv
        cp -a "$package/." "$campaign_root/work/"
        # -d=libfuzzer is what "go test -fuzz" adds for itself and a
        # precompiled harness does not get: without it Go warns that the
        # binary carries no coverage instrumentation, the campaign mutates
        # blindly, and it reaches nothing the seed corpus already reached.
        # "all=" instruments the dependencies the target calls, not only the
        # package under test.
        go test -c -trimpath -buildvcs=false \
            -gcflags=all=-d=libfuzzer -ldflags="-buildid=" \
            -o "$campaign_root/harness" "$package"
        if (
            cd "$campaign_root/work"
            "$campaign_root/harness" -test.run="^$" -test.fuzz="^${target}$" \
                -test.fuzztime="$FUZZTIME" -test.fuzzcachedir="$campaign_root/cache" \
                -test.parallel=2 -test.timeout=2m
        ); then
            :
        else
            result=$?
            exit "$result"
        fi
    done
done
'
  ;;
*)
  printf 'usage: %s [source|unit|integration|blackbox|distribution|fuzz] [options] [-- go test args...]\n' "$0" >&2
  exit 2
  ;;
esac
# Report the actual container platform, separately from the controller banner.
# shellcheck disable=SC2016
test_command='
distribution=$(uname -s)
if [[ -r /etc/os-release ]]; then
    . /etc/os-release
    distribution=${PRETTY_NAME:-${ID:-unknown}}
fi
printf "== Test runtime: environment=container | os=%s | arch=%s | distribution=%s\n" "$(uname -s)" "$(uname -m)" "$distribution" >&2
'"$test_command"
readonly test_command

# A distribution run names a listed distribution and brings the precompiled
# harness tree; every other tier builds its harness in the Go sandbox image and
# takes neither.
image=$SANDBOX_IMAGE_REF
if [[ "$suite" == distribution ]]; then
  [[ -n "$distribution" && -n "$harness_root" && -n "$harness_manifest" && -n "$harness_manifest_checksum" ]] || {
    printf 'external-test sandbox: the distribution tier requires --distribution and the harness options\n' >&2
    exit 2
  }
  sandbox_distribution_base "$(dirname "$0")/../../$SANDBOX_DISTRIBUTION_LIST" "$distribution" >/dev/null || exit 2
  image=$SANDBOX_DISTRIBUTION_IMAGE_REPO:$distribution
else
  [[ -z "$distribution$harness_root$harness_manifest$harness_manifest_checksum" ]] || {
    printf 'external-test sandbox: distribution and harness options are reserved for the distribution tier\n' >&2
    exit 2
  }
fi
readonly image

if [[ "$suite" == fuzz ]]; then
  [[ -z "$source_root$source_manifest$source_manifest_checksum$source_bin$source_checksum" ]] || {
    printf 'external-test sandbox: product options are not valid for fuzz\n' >&2
    exit 2
  }
  [[ -n "$artifact_output" && "$artifact_output" == /* ]] || {
    printf 'external-test sandbox: fuzz artifact output must be an absolute path\n' >&2
    exit 2
  }
  fuzz_seconds=$(sandbox_fuzz_seconds "$fuzztime")
elif [[ "$suite" == unit ]]; then
  [[ -z "$source_bin$source_checksum$artifact_output$large_history_budget" &&
    -n "$source_root" && -n "$source_manifest" && -n "$source_manifest_checksum" ]] || {
    printf 'external-test sandbox: unit tests require a source snapshot and accept no product artifacts\n' >&2
    exit 2
  }
else
  [[ -z "$artifact_output" ]] || {
    printf 'external-test sandbox: artifact output is reserved for fuzz\n' >&2
    exit 2
  }
  [[ -n "$source_bin" && -n "$source_checksum" ]] || {
    printf 'external-test sandbox: this tier requires a checksummed source binary\n' >&2
    exit 2
  }
  if [[ -n "$source_root$source_manifest$source_manifest_checksum" ]]; then
    [[ -n "$source_root" && -n "$source_manifest" && -n "$source_manifest_checksum" ]] || {
      printf 'external-test sandbox: source snapshot options must be supplied together\n' >&2
      exit 2
    }
  fi
fi

((EUID != 0)) || {
  printf 'external-test sandbox: refusing to run Podman as root\n' >&2
  exit 1
}
command -v podman >/dev/null 2>&1 || {
  printf 'external-test sandbox: rootless Podman is required\n' >&2
  exit 1
}
repo_root=$(realpath -e -- "$(git rev-parse --show-toplevel)")
artifact_roots=("$repo_root")
runner_root=''
if [[ -n "${RUNNER_TEMP:-}" ]]; then
  runner_root=$(realpath -e -- "$RUNNER_TEMP")
  artifact_roots+=("$runner_root")
fi
if [[ "$suite" == fuzz ]]; then
  if [[ -e "$artifact_output" || -L "$artifact_output" ]]; then
    [[ -d "$artifact_output" && ! -L "$artifact_output" && -O "$artifact_output" ]] || {
      printf 'external-test sandbox: unsafe fuzz artifact output %s\n' "$artifact_output" >&2
      exit 1
    }
  else
    [[ -d "$(dirname -- "$artifact_output")" ]] || {
      printf 'external-test sandbox: fuzz artifact output parent is missing\n' >&2
      exit 1
    }
    mkdir -m 0700 -- "$artifact_output"
  fi
  artifact_output=$(realpath -e -- "$artifact_output")
  [[ "$artifact_output" != "$repo_root" && "$artifact_output" != "$repo_root/"* ]] || {
    printf 'external-test sandbox: fuzz artifact output must be outside the checkout\n' >&2
    exit 1
  }
fi
require_staged_file() {
  local source=$1 resolved root
  [[ -f "$source" && ! -L "$source" ]] || {
    printf 'external-test sandbox: artifact must be a regular non-symlink file: %s\n' "$source" >&2
    return 1
  }
  resolved=$(realpath -e -- "$source")
  for root in "${artifact_roots[@]}"; do
    if [[ "$resolved" == "$root/"* ]]; then return 0; fi
  done
  printf 'external-test sandbox: artifact is outside the checkout and RUNNER_TEMP: %s\n' "$source" >&2
  return 1
}
require_private_source_root() {
  local source=$1 resolved
  [[ -n "$runner_root" && -d "$source" && ! -L "$source" ]] || {
    printf 'external-test sandbox: source snapshot must be a non-symlink directory under RUNNER_TEMP\n' >&2
    return 1
  }
  resolved=$(realpath -e -- "$source")
  [[ "$resolved" == "$runner_root/"* ]] || {
    printf 'external-test sandbox: source snapshot is outside RUNNER_TEMP: %s\n' "$source" >&2
    return 1
  }
}
for artifact in "$sandbox_image" "$sandbox_image_checksum" "$source_manifest" "$source_manifest_checksum" "$source_bin" "$source_checksum" "$harness_manifest" "$harness_manifest_checksum"; do
  if [[ -n "$artifact" ]]; then require_staged_file "$artifact"; fi
done
if [[ -n "$source_root" ]]; then require_private_source_root "$source_root"; fi
if [[ -n "$harness_root" ]]; then require_private_source_root "$harness_root"; fi
sandbox_init gl-ext
readonly state=$SANDBOX_STATE
[[ "$state" != "$repo_root/"* ]] || {
  printf 'external-test sandbox: private state cannot be inside the checkout\n' >&2
  exit 1
}
mkdir -p "$state"/{artifacts,fuzz-campaigns,go-build,go-mod,go-path,inputs,prefetch-home,snapshot-state}

collect_fuzz_campaigns() {
  local package target campaign
  [[ -f "$state/fuzz-campaigns/index.tsv" ]] || return 0
  while IFS=$'\t' read -r package target campaign; do
    [[ "$campaign" =~ ^campaign-[0-9]{4}$ ]] || {
      printf 'external-test sandbox: invalid campaign index entry %q\n' "$campaign" >&2
      return 1
    }
    (
      cd "$state/source"
      bash .forgejo/scripts/collect-fuzz-artifacts.sh gommitlint "$package" "$target" \
        "$state/fuzz-campaigns/$campaign/work" "$state/fuzz-campaigns/$campaign/cache" \
        "$artifact_output"
    )
  done <"$state/fuzz-campaigns/index.tsv"
}
cleanup_fuzz() {
  local result=$? collect_result=0
  trap - EXIT
  collect_fuzz_campaigns || collect_result=$?
  ((collect_result == 0)) || result=$collect_result
  set +e
  (exit "$result")
  sandbox_cleanup
}
if [[ "$suite" == fuzz ]]; then trap cleanup_fuzz EXIT; else trap sandbox_cleanup EXIT; fi

stage_input() {
  local source=$1 destination=$2 mode=$3 target
  target=$state/$destination
  mkdir -p "$(dirname "$target")"
  install -m "$mode" -- "$(realpath -e -- "$source")" "$target"
  [[ -f "$target" && ! -L "$target" ]]
}

printf 'external-test sandbox: copying and verifying the prepared image archive\n' >&2
stage_input "$sandbox_image" inputs/sandbox-image.oci.tar 0444
stage_input "$sandbox_image_checksum" inputs/sandbox-image.sha256 0444
sandbox_verify_one "$state/inputs/sandbox-image.oci.tar" "$state/inputs/sandbox-image.sha256" sandbox-image.oci.tar || {
  printf 'external-test sandbox: invalid sandbox image artifact\n' >&2
  exit 1
}
[[ $(sandbox_podman info --format '{{.Host.Security.Rootless}}') == true ]] || {
  printf 'external-test sandbox: Podman is not rootless\n' >&2
  exit 1
}
printf 'external-test sandbox: loading the local image into private Podman storage (no rebuild; timeout 5m)\n' >&2
sandbox_podman_timeout 5m load --input "$state/inputs/sandbox-image.oci.tar" >/dev/null
sandbox_podman image inspect "$image" >/dev/null || {
  printf 'external-test sandbox: archive does not contain the pinned sandbox image\n' >&2
  exit 1
}

printf 'external-test sandbox: staging and verifying the source snapshot\n' >&2
if [[ -z "$source_root" ]]; then
  sandbox_snapshot_worktree "$repo_root" "$state/source" "$state/snapshot-state/source.manifest" "$state/snapshot-state/source.manifest.sha256"
else
  stage_input "$source_manifest" snapshot-state/source.manifest 0444
  stage_input "$source_manifest_checksum" snapshot-state/source.manifest.sha256 0444
  sandbox_stage_snapshot "$source_root" "$state/snapshot-state/source.manifest" \
    "$state/snapshot-state/source.manifest.sha256" "$state/source" || {
    printf 'external-test sandbox: invalid or unstable source snapshot\n' >&2
    exit 1
  }
fi
snapshot_hash=$(sha256sum "$state/snapshot-state/source.manifest")
snapshot_hash=${snapshot_hash%% *}
printf 'external-test sandbox: source snapshot sha256:%s\n' "$snapshot_hash" >&2

sandbox_artifact_args=()
sandbox_env_args=()
sandbox_fuzz_args=()
container_timeout=$run_timeout
if [[ "$suite" == fuzz ]]; then
  # The outer deadline covers the whole matrix, not just one target. Keep the
  # original 15m build/startup margin and add the requested campaign time.
  # An explicit RUN_TIMEOUT remains the operator's total wall-clock budget.
  if [[ -z "${GOMMITLINT_EXTERNAL_RUN_TIMEOUT:-}" ]]; then
    target_count=$(grep -rhE '^func Fuzz[A-Za-z0-9_]+' --include='*_test.go' "$state/source" | wc -l)
    container_timeout=$((900 + target_count * fuzz_seconds))s
  fi
  sandbox_fuzz_args+=(--volume "$state/fuzz-campaigns:/fuzz-campaigns:rw" --env "FUZZTIME=$fuzztime")
fi
if [[ "$suite" != fuzz && "$suite" != unit ]]; then
  printf 'external-test sandbox: staging and verifying compiled Gommitlint\n' >&2
  stage_input "$source_bin" artifacts/source/gommitlint 0555
  stage_input "$source_checksum" inputs/source-gommitlint.sha256 0444
  sandbox_verify_one "$state/artifacts/source/gommitlint" "$state/inputs/source-gommitlint.sha256" gommitlint || {
    printf 'external-test sandbox: invalid source product artifact\n' >&2
    exit 1
  }
  sandbox_env_args+=(--env GOMMITLINT_BLACKBOX_BIN=/artifacts/source/gommitlint)
fi
if [[ "$suite" == distribution ]]; then
  printf 'external-test sandbox: staging and verifying compiled test harnesses\n' >&2
  stage_input "$harness_manifest" snapshot-state/harness.manifest 0444
  stage_input "$harness_manifest_checksum" snapshot-state/harness.manifest.sha256 0444
  sandbox_stage_snapshot "$harness_root" "$state/snapshot-state/harness.manifest" \
    "$state/snapshot-state/harness.manifest.sha256" "$state/artifacts/harness" || {
    printf 'external-test sandbox: invalid or unstable harness tree\n' >&2
    exit 1
  }
fi
if [[ -n "$large_history_budget" ]]; then sandbox_env_args+=(--env "GOMMITLINT_BLACKBOX_LARGE_HISTORY_BUDGET=$large_history_budget"); fi

if ((${#sandbox_env_args[@]} > 0)); then sandbox_artifact_args+=(--volume "$state/artifacts:/artifacts:ro"); fi

# The Go sandbox compiles its harness from these modules; a distribution
# image compiles nothing, so it gets neither the modules nor the Go
# environment, only the PATH its own packages live on.
sandbox_go_args=()
container_path=/usr/local/bin:/usr/bin:/bin
if [[ "$suite" != distribution ]]; then
  printf 'external-test sandbox: preparing Go module dependencies on the controller (timeout 5m)\n' >&2
  (
    cd "$state/source"
    timeout 5m env -i PATH="$PATH" GOCACHE="$state/go-build" GOENV=off GOMODCACHE="$state/go-mod" \
      GOPATH="$state/go-path" GOTOOLCHAIN=local HOME="$state/prefetch-home" XDG_CACHE_HOME="$state/prefetch-home/cache" "${SANDBOX_MODULE_ENV[@]}" go mod download
  )
  sandbox_go_args=(--volume "$state/go-mod:/gomodcache:ro" --volume "$state/go-build:/gocache:rw"
    --env CGO_ENABLED=0 --env GOCACHE=/gocache --env GOENV=off
    --env GOMODCACHE=/gomodcache --env GOPATH=/go --env GOPROXY=off
    --env GOROOT=/usr/local/go --env GOSUMDB=off --env GOTOOLCHAIN=local)
  container_path=/usr/local/go/bin:$container_path
  # The unit lane keeps the native recipe's race detector. The Go image carries
  # the C compiler; external and fuzz lanes retain their existing static builds.
  if [[ "$suite" == unit ]]; then sandbox_go_args+=(--env CGO_ENABLED=1); fi
fi
printf 'external-test sandbox: verifying the final source snapshot\n' >&2
sandbox_verify_snapshot "$state/source" "$state/snapshot-state/source.manifest" "$state/snapshot-state/source.manifest.sha256" || {
  printf 'external-test sandbox: private source snapshot changed before harness compilation\n' >&2
  exit 1
}

# --init gives the harness container an init process (catatonit) as PID 1 that
# reaps reparented children. Without it, bash exec-optimises a single-command
# tier and the `go` tool becomes PID 1, which never wait()s on the orphans the
# kernel reparents to it: every killed grandchild lingers as a zombie, which is
# what made the bounded-command self-test read a dead sleeper as alive. It is a
# self-contained addition — podman bind-mounts its own static init read only and
# catatonit needs no capability — so --read-only, --cap-drop=ALL,
# --userns=keep-id and the noexec /run tmpfs are all unaffected.
#
# The `_` after -c is bash's $0 for the command string; every word after it is a
# positional parameter the inner `"$@"` expands one-to-one. Caller arguments
# therefore never become shell source text, so a semicolon, a backtick or a
# $(...) in one reaches `go test` as a literal character.
#
# The /home/sandbox mount carries no uid=, gid= or mode=, which is what lets
# these tiers run on podman 4 as well as 5: 4.x rejects uid=/gid= as tmpfs
# options and fails the whole run with `unknown mount option`, and 4.9.3 is
# what the current Debian and Ubuntu LTS ship. mode=0700 cannot be kept on its
# own either -- without uid=/gid= the mount belongs to a uid this container is
# not, and HOME becomes unwritable. So it takes the default 1777, the same
# shape /tmp and /run have always had here. That gives nothing away: the
# container runs a single unprivileged principal under --network=none,
# --read-only, --cap-drop=ALL, --security-opt=no-new-privileges and private
# ipc/uts, so there is no second principal inside for 0700 to exclude.
# test/policy asserts that no --tmpfs option below regains uid= or gid=.
printf 'external-test sandbox: starting test container (timeout %s)\n' "$container_timeout" >&2
# Ctrl-C must stop the run at once. A shell runs its exit trap only between
# commands, and timeout hides the terminal's interrupt from podman, so the
# command runs in the background and the shell waits for it.
timeout --signal=TERM --kill-after=15s "$container_timeout" podman "${SANDBOX_PODMAN_ARGS[@]}" \
  run --rm --init --cidfile "$state/cid-harness" --pull=never \
  --network=none --read-only --userns=keep-id --user "$(id -u):$(id -g)" \
  --cap-drop=ALL --security-opt=no-new-privileges --ipc=private --uts=private \
  --cpus=2 --memory=4g --memory-swap=4g --pids-limit=512 \
  --ulimit=nofile=1024:1024 --ulimit=nproc=512:512 --http-proxy=false \
  --volume "$state/source:/workspace:ro" "${sandbox_go_args[@]}" "${sandbox_artifact_args[@]}" \
  "${sandbox_fuzz_args[@]}" \
  --tmpfs /tmp:rw,nodev,nosuid,size=2g --tmpfs /run:rw,nodev,nosuid,noexec,size=16m \
  --tmpfs /home/sandbox:rw,nodev,nosuid,size=128m \
  --workdir /workspace --entrypoint /bin/bash \
  --env HOME=/home/sandbox --env HTTP_PROXY= --env HTTPS_PROXY= --env ALL_PROXY= --env NO_PROXY= \
  --env http_proxy= --env https_proxy= --env all_proxy= --env no_proxy= \
  --env "PATH=$container_path" --env TMPDIR=/tmp \
  --env "TEST_TIMEOUT=$test_timeout" \
  --env XDG_CACHE_HOME=/home/sandbox/cache --env XDG_CONFIG_HOME=/home/sandbox/config \
  --env XDG_DATA_HOME=/home/sandbox/data --env XDG_STATE_HOME=/home/sandbox/state \
  "${sandbox_env_args[@]}" "$image" -euo pipefail -c "$test_command" _ "${test_args[@]}" &
wait "$!"
