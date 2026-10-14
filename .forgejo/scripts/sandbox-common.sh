#!/usr/bin/env bash

# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# The sandbox image the tiers run in. prepare-sandbox-image.sh builds it from
# Containerfile.sandbox into a private store and exports a checksummed OCI
# archive; run-external-tests-sandbox.sh loads that archive and asserts the tag
# is in it. Both need the same name, and neither may reach a registry for it,
# so it is a local tag rather than a repository reference: the archive and its
# sha256 sidecar are what bind the two scripts together, not a registry digest.
# shellcheck disable=SC2034 # read by the scripts that source this file
SANDBOX_IMAGE_REF='localhost/gommitlint-sandbox:local'
readonly SANDBOX_IMAGE_REF

# Where `just test-container-prepare` records that archive, relative to the
# repository root. It is under the ignored build-output directory on purpose:
# the tiers snapshot the worktree, and git's ignore rules keep a 400 MB archive
# out of the snapshot they hand the sandbox. Recording it once is what lets the
# tiers run with no registry reachable at all -- they load this and nothing
# else, and refuse when it is missing or stale rather than build one.
SANDBOX_IMAGE_DIR='bin/sandbox-image'
readonly SANDBOX_IMAGE_DIR

# The distribution matrix: one image per line of the list, each recorded like
# the sandbox image above, in a directory of its own under the second path, and
# tagged under the third name. The list and the Containerfile are relative to
# the tree being tested, the recordings to the live checkout.
SANDBOX_DISTRIBUTION_LIST='test/distribution/images'
SANDBOX_DISTRIBUTION_DIR='bin/distribution-images'
SANDBOX_DISTRIBUTION_IMAGE_REPO='localhost/gommitlint-distribution'
readonly SANDBOX_DISTRIBUTION_LIST SANDBOX_DISTRIBUTION_DIR SANDBOX_DISTRIBUTION_IMAGE_REPO

# Leave a minute for startup/minimization inside each 2m fuzz harness deadline.
# Limit digits before arithmetic so oversized inputs cannot overflow the bound.
sandbox_fuzz_seconds() {
  local sandbox_duration=$1 sandbox_milliseconds
  [[ "$sandbox_duration" =~ ^([1-9][0-9]{0,4})(ms|s|m)$ ]] || {
    printf 'external-test sandbox: fuzz time must be positive integer ms/s/m, at most 60s: %q\n' "$sandbox_duration" >&2
    return 2
  }
  sandbox_milliseconds=${BASH_REMATCH[1]}
  case "${BASH_REMATCH[2]}" in
  s) sandbox_milliseconds=$((sandbox_milliseconds * 1000)) ;;
  m) sandbox_milliseconds=$((sandbox_milliseconds * 60000)) ;;
  esac
  ((sandbox_milliseconds <= 60000)) || {
    printf 'external-test sandbox: fuzz time must be at most 60s: %q\n' "$sandbox_duration" >&2
    return 2
  }
  printf '%s\n' "$(((sandbox_milliseconds + 999) / 1000))"
}

# Locals here are prefixed because every sourcing script runs these functions in
# its own global scope. `local x` is a fatal error when the caller has already
# declared `readonly x`, so an unprefixed name silently couples this file to the
# variable names its callers happen to choose. That is not hypothetical: this
# function used `local suite`, which made every recipe routed through
# run-external-tests-local.sh (`readonly suite`) abort before running a single
# test, while the CI path stayed green because run-external-tests-sandbox.sh
# calls this before declaring its own `suite`.
sandbox_refuse_ci_product_mode() {
  local sandbox_suite=$1 sandbox_marker
  [[ "$sandbox_suite" == fuzz ]] && return

  for sandbox_marker in CI CONTINUOUS_INTEGRATION GITHUB_ACTIONS GITEA_ACTIONS FORGEJO_ACTIONS GITLAB_CI \
    BUILDKITE CIRCLECI JENKINS_URL TEAMCITY_VERSION TF_BUILD DRONE WOODPECKER CODEBUILD_BUILD_ID TRAVIS; do
    if [[ -v "$sandbox_marker" ]]; then
      printf 'external-test sandbox: refusing product mode %s while CI marker %s is set\n' "$sandbox_suite" "$sandbox_marker" >&2
      return 1
    fi
  done
}

sandbox_clear_ambient_container_env() {
  unset HTTP_PROXY HTTPS_PROXY ALL_PROXY NO_PROXY http_proxy https_proxy all_proxy no_proxy
  unset CONTAINER_HOST DOCKER_HOST BUILDAH_HOST DOCKER_CONFIG CONTAINERS_AUTH_FILE
  unset CONTAINERS_CONF CONTAINERS_MOUNTS_CONF CONTAINERS_POLICY CONTAINERS_REGISTRIES_CONF CONTAINERS_STORAGE_CONF
  unset REGISTRIES_CONFIG_PATH REGISTRY_AUTH_FILE STORAGE_DRIVER STORAGE_OPTS
  unset GOMMITLINT_BLACKBOX_BIN GOMMITLINT_BLACKBOX_LARGE_HISTORY_BUDGET
}

sandbox_reexec_clean() {
  # A Linux VM controller has no egress during execution. It supplies only a
  # prepared local module proxy; ordinary host runs retain their existing prep.
  SANDBOX_MODULE_ENV=()
  if [[ -n "${GOMMITLINT_MODULE_PROXY:-}" ]]; then
    [[ "$GOMMITLINT_MODULE_PROXY" == file:///* && -d "${GOMMITLINT_MODULE_PROXY#file://}" ]] || {
      printf 'external-test sandbox: module proxy must name a prepared local cache\n' >&2
      return 2
    }
    SANDBOX_MODULE_ENV=("GOPROXY=$GOMMITLINT_MODULE_PROXY" GOSUMDB=off)
  fi
  if [[ "${SANDBOX_CLEAN_ENV:-}" == '1' ]]; then return; fi
  local -a clean=(env -i GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 HOME=/nonexistent PATH="$PATH" SANDBOX_CLEAN_ENV=1)
  [[ -z "${RUNNER_TEMP:-}" ]] || clean+=(RUNNER_TEMP="$RUNNER_TEMP")
  [[ -z "${TMPDIR:-}" ]] || clean+=(TMPDIR="$TMPDIR")
  [[ -z "${GOMMITLINT_EXTERNAL_RUN_TIMEOUT:-}" ]] || clean+=(GOMMITLINT_EXTERNAL_RUN_TIMEOUT="$GOMMITLINT_EXTERNAL_RUN_TIMEOUT")
  [[ -z "${GOMMITLINT_EXTERNAL_TEST_TIMEOUT:-}" ]] || clean+=(GOMMITLINT_EXTERNAL_TEST_TIMEOUT="$GOMMITLINT_EXTERNAL_TEST_TIMEOUT")
  [[ -z "${GOMMITLINT_MODULE_PROXY:-}" ]] || clean+=(GOMMITLINT_MODULE_PROXY="$GOMMITLINT_MODULE_PROXY")
  exec "${clean[@]}" bash "$0" "$@"
}

sandbox_safe_snapshot_path() {
  local path=$1 LC_ALL=C
  [[ -n "$path" && "$path" != /* && "$path" != -* && "$path" != . && "$path" != .. ]] || return 1
  [[ "$path" != ./* && "$path" != */./* && "$path" != */. && "$path" != ../* && "$path" != */../* && "$path" != */.. ]] || return 1
  [[ "$path" != *//* && "$path" != *\\* && "$path" != .git && "$path" != .git/* ]] || return 1
  [[ ! "$path" =~ [[:cntrl:]] ]]
}

sandbox_excluded_snapshot_path() {
  local path=$1 base=${1##*/} lower
  lower=${base,,}

  case "/$path/" in
  */.git/* | */.cache/* | */cache/* | */.rumdl-cache/* | */node_modules/* | */__pycache__/* | */.pytest_cache/* | */.mypy_cache/* | */.tox/* | */.nox/* | */.gradle/* | */.direnv/* | */.mise/* | */.ssh/* | */.gnupg/* | */.aws/* | */.azure/* | */.kube/* | */.docker/* | */.config/containers/*)
    return 0
    ;;
  esac

  case "$lower" in
  .env | .env.* | .netrc | .npmrc | .pypirc | auth.json | credentials | credentials.* | secrets.json | token | token.* | id_rsa | id_dsa | id_ecdsa | id_ed25519 | *.key | *.p12 | *.pfx)
    return 0
    ;;
  esac

  return 1
}

sandbox_require_regular_source_path() {
  local root=$1 path=$2 source=$1/$2 resolved partial segment
  local -a segments=()
  [[ ! -L "$source" && -f "$source" ]] || return 1

  partial=$root
  IFS=/ read -r -a segments <<<"$path"
  for segment in "${segments[@]}"; do
    partial=$partial/$segment
    [[ ! -L "$partial" ]] || return 1
  done

  resolved=$(realpath -e -- "$source") || return 1
  [[ "$resolved" == "$root/"* ]]
}

sandbox_snapshot_one() {
  local root=$1 destination=$2 manifest=$3 path=$4 origin=$5 source target mode hash source_hash
  sandbox_safe_snapshot_path "$path" || {
    printf 'external-test snapshot: refusing unsafe path %q\n' "$path" >&2
    return 1
  }
  if sandbox_excluded_snapshot_path "$path"; then
    printf 'external-test snapshot: excluded cache or credential path %q\n' "$path" >&2
    return
  fi

  source=$root/$path
  if [[ ! -e "$source" && ! -L "$source" ]]; then
    [[ "$origin" == tracked ]] && return
    printf 'external-test snapshot: untracked path disappeared: %s\n' "$path" >&2
    return 1
  fi
  sandbox_require_regular_source_path "$root" "$path" || {
    printf 'external-test snapshot: path is not a regular non-symlink file: %s\n' "$path" >&2
    return 1
  }

  mode=0444
  [[ -x "$source" ]] && mode=0555
  target=$destination/$path
  mkdir -p "$(dirname "$target")"
  install -m "$mode" -- "$source" "$target"
  hash=$(sha256sum -- "$target")
  hash=${hash%% *}
  source_hash=$(sha256sum -- "$source")
  source_hash=${source_hash%% *}
  [[ "$hash" == "$source_hash" ]] || {
    printf 'external-test snapshot: source changed while copying: %s\n' "$path" >&2
    return 1
  }
  printf '%s %s %s\n' "$hash" "$mode" "$path" >>"$manifest"
}

sandbox_validate_worktree_entries() {
  local root=$1 list_dir=$2 path entry
  local -A ignored=()

  : >"$list_dir/worktree-paths.list"
  while IFS= read -r -d '' entry; do
    printf '%s\0' "${entry#"$root"/}" >>"$list_dir/worktree-paths.list"
  done < <(find "$root" -mindepth 1 -path "$root/.git" -prune -o -print0)
  git -C "$root" check-ignore --stdin -z <"$list_dir/worktree-paths.list" >"$list_dir/ignored-paths.list" || {
    [[ $? -eq 1 ]] || return 1
  }
  while IFS= read -r -d '' path; do ignored[$path]=1; done <"$list_dir/ignored-paths.list"

  while IFS= read -r -d '' path; do
    [[ -z "${ignored[$path]:-}" ]] || continue
    sandbox_excluded_snapshot_path "$path" && continue
    sandbox_safe_snapshot_path "$path" || {
      printf 'external-test snapshot: refusing unsafe worktree path %q\n' "$path" >&2
      return 1
    }
    entry=$root/$path
    if [[ -L "$entry" || (! -f "$entry" && ! -d "$entry") ]]; then
      printf 'external-test snapshot: refusing symlink or special file %q\n' "$path" >&2
      return 1
    fi
  done <"$list_dir/worktree-paths.list"

  rm -f -- "$list_dir/worktree-paths.list" "$list_dir/ignored-paths.list"
}

sandbox_snapshot_worktree() {
  local root=$1 destination=$2 manifest=$3 checksum=$4 path list_dir destination_parent manifest_parent
  root=$(realpath -e -- "$root")
  [[ -d "$root/.git" || -f "$root/.git" ]] || {
    printf 'external-test snapshot: source root is not a Git worktree: %s\n' "$root" >&2
    return 1
  }
  [[ ! -e "$destination" ]] || {
    printf 'external-test snapshot: destination already exists: %s\n' "$destination" >&2
    return 1
  }
  destination_parent=$(realpath -e -- "$(dirname "$destination")")
  manifest_parent=$(realpath -e -- "$(dirname "$manifest")")
  if [[ "$destination_parent" == "$root" || "$destination_parent" == "$root/"* || "$manifest_parent" == "$root" || "$manifest_parent" == "$root/"* ]]; then
    printf 'external-test snapshot: private snapshot state cannot be inside the checkout\n' >&2
    return 1
  fi

  list_dir=$(dirname "$manifest")
  sandbox_validate_worktree_entries "$root" "$list_dir"
  mkdir -p "$destination"
  : >"$manifest"
  git -C "$root" ls-files --cached -z >"$list_dir/tracked-files.list"
  git -C "$root" ls-files --others --exclude-standard -z >"$list_dir/untracked-files.list"

  while IFS= read -r -d '' path; do
    sandbox_snapshot_one "$root" "$destination" "$manifest" "$path" tracked
  done <"$list_dir/tracked-files.list"
  while IFS= read -r -d '' path; do
    sandbox_snapshot_one "$root" "$destination" "$manifest" "$path" untracked
  done <"$list_dir/untracked-files.list"

  rm -f -- "$list_dir/tracked-files.list" "$list_dir/untracked-files.list"
  while IFS= read -r -d '' path; do chmod 0555 "$path"; done < <(find "$destination" -type d -print0)
  (cd "$(dirname "$manifest")" && sha256sum "$(basename "$manifest")" >"$(basename "$checksum")")
  sandbox_verify_snapshot "$destination" "$manifest" "$checksum"
}

sandbox_verify_snapshot() {
  local root=$1 manifest=$2 checksum=$3 line regex hash mode path actual actual_mode entry relative
  local -A seen=()
  [[ -d "$root" && ! -L "$root" ]] || return 1
  sandbox_verify_one "$manifest" "$checksum" "${manifest##*/}" || return 1
  regex='^([A-Fa-f0-9]{64}) (0444|0555) (.+)$'

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ $regex ]] || return 1
    hash=${BASH_REMATCH[1],,}
    mode=${BASH_REMATCH[2]}
    path=${BASH_REMATCH[3]}
    sandbox_safe_snapshot_path "$path" || return 1
    [[ -z "${seen[$path]:-}" ]] || return 1
    seen[$path]=1
    sandbox_require_regular_source_path "$root" "$path" || return 1
    actual_mode=$(stat -c '%a' -- "$root/$path") || return 1
    [[ "0$actual_mode" == "$mode" ]] || return 1
    actual=$(sha256sum -- "$root/$path")
    actual=${actual%% *}
    [[ "${actual,,}" == "$hash" ]] || return 1
  done <"$manifest"

  while IFS= read -r -d '' entry; do
    relative=${entry#"$root"/}
    if [[ -d "$entry" && ! -L "$entry" ]]; then continue; fi
    [[ -f "$entry" && ! -L "$entry" && -n "${seen[$relative]:-}" ]] || return 1
  done < <(find "$root" -mindepth 1 -print0)
  [[ ${#seen[@]} -gt 0 ]]
}

sandbox_stage_snapshot() {
  local source=$1 manifest=$2 checksum=$3 destination=$4 line regex mode path target
  sandbox_verify_snapshot "$source" "$manifest" "$checksum" || return 1
  [[ ! -e "$destination" ]] || return 1
  mkdir -p "$destination"
  regex='^[A-Fa-f0-9]{64} (0444|0555) (.+)$'

  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ "$line" =~ $regex ]] || return 1
    mode=${BASH_REMATCH[1]}
    path=${BASH_REMATCH[2]}
    target=$destination/$path
    mkdir -p "$(dirname "$target")"
    install -m "$mode" -- "$source/$path" "$target"
  done <"$manifest"

  while IFS= read -r -d '' path; do chmod 0555 "$path"; done < <(find "$destination" -type d -print0)
  sandbox_verify_snapshot "$destination" "$manifest" "$checksum"
}

sandbox_remove_private_tree() {
  local tree=$1
  [[ -n "$tree" && -d "$tree" && ! -L "$tree" ]] || return 1
  chmod -R u+w -- "$tree"
  rm -rf -- "$tree"
}

sandbox_init() {
  local prefix=$1 docker_repository=${2:-docker.io/library/golang} base="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
  [[ "$docker_repository" =~ ^[a-z0-9]+([.-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)+$ ]] || {
    printf 'sandbox: refusing registry repository %q\n' "$docker_repository" >&2
    return 1
  }
  umask 077
  SANDBOX_STATE=$(mktemp -d "${base%/}/${prefix}.XXXXXX")
  export SANDBOX_STATE
  # Podman refuses a runroot longer than 50 characters and names neither the
  # path nor the limit. The local launcher nests its own state directory as
  # RUNNER_TEMP, so that directory, this prefix and "/runroot" share the budget;
  # the prefixes are abbreviations for that reason.
  local runroot="$SANDBOX_STATE/runroot"
  ((${#runroot} <= 50)) || {
    printf 'sandbox: runroot is %d characters, over Podman'\''s 50: %s (use a shorter RUNNER_TEMP or TMPDIR)\n' "${#runroot}" "$runroot" >&2
    return 1
  }
  local config_dir="$SANDBOX_STATE/home/.config/containers"
  mkdir -p "$SANDBOX_STATE"/{auth,cache,certs,data,graphroot,hooks,networks,runroot,runtime,tmp} "$config_dir"
  printf '{}\n' >"$SANDBOX_STATE/auth/auth.json"
  printf '[engine]\nevents_logger = "file"\nhooks_dir = ["%s"]\ntmp_dir = "%s"\n' "$SANDBOX_STATE/hooks" "$SANDBOX_STATE/tmp" >"$config_dir/containers.conf"
  : >"$config_dir/mounts.conf"
  : >"$config_dir/registries.conf"
  printf '[storage]\ndriver = "vfs"\ngraphroot = "%s"\nrunroot = "%s"\n' "$SANDBOX_STATE/graphroot" "$SANDBOX_STATE/runroot" >"$config_dir/storage.conf"
  # The only registry any tier may reach is the one the sandbox image is built
  # from: docker.io/library/golang, pulled by prepare-sandbox-image.sh, or for
  # a distribution image the one repository its pinned base names. Every
  # other docker reference is rejected; oci-archive stays open because that is
  # how the built sandbox image enters the private store.
  # containers-storage is open because exporting the built image reads it back
  # out of this run's own store, which only holds what the two rules above
  # just admitted -- the store is created empty and destroyed on exit.
  printf '%s\n' '{"default":[{"type":"reject"}],"transports":{"containers-storage":{"":[{"type":"insecureAcceptAnything"}]},"docker":{"'"$docker_repository"'":[{"type":"insecureAcceptAnything"}]},"oci-archive":{"":[{"type":"insecureAcceptAnything"}]}}}' >"$config_dir/policy.json"
  export CONTAINERS_CONF="$config_dir/containers.conf" CONTAINERS_MOUNTS_CONF="$config_dir/mounts.conf"
  export CONTAINERS_POLICY="$config_dir/policy.json" CONTAINERS_REGISTRIES_CONF="$config_dir/registries.conf" CONTAINERS_STORAGE_CONF="$config_dir/storage.conf"
  export HOME="$SANDBOX_STATE/home" REGISTRIES_CONFIG_PATH="$config_dir/registries.conf"
  export REGISTRY_AUTH_FILE="$SANDBOX_STATE/auth/auth.json" TMPDIR="$SANDBOX_STATE/tmp"
  export XDG_CACHE_HOME="$SANDBOX_STATE/cache" XDG_CONFIG_HOME="$SANDBOX_STATE/home/.config" XDG_DATA_HOME="$SANDBOX_STATE/data"
  export XDG_RUNTIME_DIR="$SANDBOX_STATE/runtime"
  unset CONTAINER_HOST DBUS_SESSION_BUS_ADDRESS DOCKER_HOST
  SANDBOX_PODMAN_ARGS=(--root "$SANDBOX_STATE/graphroot" --runroot "$SANDBOX_STATE/runroot" --storage-driver vfs --hooks-dir "$SANDBOX_STATE/hooks" --network-config-dir "$SANDBOX_STATE/networks" --tmpdir "$SANDBOX_STATE/tmp" --transient-store --events-backend file)
}

sandbox_podman() {
  podman "${SANDBOX_PODMAN_ARGS[@]}" "$@"
}

# Ctrl-C must stop an engine command at once. A shell runs its exit trap only
# between commands, and timeout hides the terminal's interrupt from podman, so
# the command runs in the background and the shell waits for it.
SANDBOX_JOB=''
sandbox_podman_timeout() {
  local duration=$1
  shift
  timeout --signal=TERM --kill-after=15s "$duration" podman "${SANDBOX_PODMAN_ARGS[@]}" "$@" &
  SANDBOX_JOB=$!
  wait "$SANDBOX_JOB"
  SANDBOX_JOB=''
}

sandbox_cleanup() {
  local result=$? cidfile
  trap - EXIT
  printf 'sandbox: cleaning temporary containers and image storage (exit %s)\n' "$result" >&2
  # An interrupted build, save or load is still running; removal below ends a run.
  [[ -z "$SANDBOX_JOB" ]] || kill "$SANDBOX_JOB" 2>/dev/null || :
  for cidfile in "$SANDBOX_STATE"/cid*; do
    [[ -s "$cidfile" ]] || continue
    IFS= read -r SANDBOX_CID <"$cidfile" || true
    [[ -z "${SANDBOX_CID:-}" ]] || sandbox_podman_timeout 30s rm --force --time 0 --ignore "$SANDBOX_CID" >/dev/null 2>&1 || result=1
  done
  # Image layers extracted into the private store carry container uids (apt's
  # _apt, for one) that map to sub-uids this user can neither chmod nor read,
  # so remove the store from inside the user namespace that wrote it. The
  # plain removal below then has only our own files left to delete. Failure
  # is not reported here: it shows up as the removal failing.
  sandbox_podman_timeout 2m unshare rm -rf "${SANDBOX_STATE:?}/graphroot" >/dev/null 2>&1 || :
  sandbox_remove_private_tree "${SANDBOX_STATE:?}" || result=1
  printf 'sandbox: finished (exit %s)\n' "$result" >&2
  exit "$result"
}

# What a recorded archive is good for. prepare-sandbox-image.sh writes this key
# beside the archive it exported, and the launchers refuse an archive whose key
# no longer matches the checkout, so a Containerfile edit can never be tested
# against the image built before it.
#
# The Containerfile hash alone would answer the question; the base image and the
# package list are read out of the file as well so the key says in words what
# changed, and so a Containerfile that stops having the shape those two are read
# from fails here instead of quietly recording less than it claims.
#
# With a distribution name the key describes that distribution's image
# instead: the base comes from the list beside the Containerfile, and the
# install script the builder receives is hashed with the Containerfile, since
# that script is the package list.
sandbox_image_key() {
  local sandbox_containerfile=$1 sandbox_distribution=${2:-} sandbox_base sandbox_packages sandbox_hash sandbox_install
  [[ -f "$sandbox_containerfile" && ! -L "$sandbox_containerfile" ]] || {
    printf 'sandbox image: %s is not a regular file\n' "$sandbox_containerfile" >&2
    return 1
  }
  if [[ -n "$sandbox_distribution" ]]; then
    sandbox_base=$(sandbox_distribution_base "${sandbox_containerfile%/*}/images" "$sandbox_distribution") || return 1
    sandbox_install=${sandbox_containerfile%/*}/install-deps.sh
    [[ -f "$sandbox_install" && ! -L "$sandbox_install" ]] || {
      printf 'sandbox image: %s is not a regular file\n' "$sandbox_install" >&2
      return 1
    }
    sandbox_hash=$(sha256sum -- "$sandbox_containerfile")
    sandbox_install=$(sha256sum -- "$sandbox_install")
    printf 'sandbox-image-key 1\nimage %s:%s\nbase %s\ninstall-deps sha256:%s\ncontainerfile sha256:%s\n' \
      "$SANDBOX_DISTRIBUTION_IMAGE_REPO" "$sandbox_distribution" "$sandbox_base" \
      "${sandbox_install%% *}" "${sandbox_hash%% *}"
    return
  fi
  sandbox_base=$(sed -n 's/^ARG BASE_IMAGE=\([^[:space:]]*@sha256:[A-Fa-f0-9]\{64\}\)$/\1/p' "$sandbox_containerfile")
  [[ -n "$sandbox_base" && "$sandbox_base" != *$'\n'* ]] || {
    printf 'sandbox image: %s has no single digest-pinned ARG BASE_IMAGE line\n' "$sandbox_containerfile" >&2
    return 1
  }
  sandbox_packages=$(awk '
    /apt-get install / { collecting = 1; next }
    collecting {
      gsub(/^[[:space:]]+|[[:space:]]*\\$/, "", $0)
      if ($0 ~ /;$/) { collecting = 0; sub(/;$/, "", $0) }
      if ($0 != "") { list = list (list == "" ? "" : " ") $0 }
    }
    END { print list }
  ' "$sandbox_containerfile")
  [[ -n "$sandbox_packages" ]] || {
    printf 'sandbox image: %s installs no packages the key can name\n' "$sandbox_containerfile" >&2
    return 1
  }
  sandbox_hash=$(sha256sum -- "$sandbox_containerfile")
  printf 'sandbox-image-key 1\nimage %s\nbase %s\npackages %s\ncontainerfile sha256:%s\n' \
    "$SANDBOX_IMAGE_REF" "$sandbox_base" "$sandbox_packages" "${sandbox_hash%% *}"
}

# The tiers load an image; they never build one. Either the recorded archive is
# there, intact and current, or the tier stops here and says which recipe
# records it. Rebuilding on the operator's behalf would put a registry back in
# the tier's path -- the network tag comes off these tiers only because they
# cannot reach one -- and reusing a stale image would test an environment the
# checkout no longer describes. Refusing does neither.
#
# A third argument names a distribution: the recording is then that
# distribution's, checked against its key, and the hint names the recipe that
# records distribution images.
sandbox_require_recorded_image() {
  local sandbox_dir=$1 sandbox_containerfile=$2 sandbox_distribution=${3:-} sandbox_name sandbox_expected sandbox_recorded
  local sandbox_hint="run 'just test-container-prepare' to record it"
  [[ -z "$sandbox_distribution" ]] || sandbox_hint="run 'just test-container-prepare-distributions $sandbox_distribution' to record it"
  for sandbox_name in sandbox-image.oci.tar sandbox-image.sha256 sandbox-image.key; do
    [[ -f "$sandbox_dir/$sandbox_name" && ! -L "$sandbox_dir/$sandbox_name" ]] || {
      printf 'external-test sandbox: no recorded sandbox image in %s (%s is missing); %s\n' \
        "$sandbox_dir" "$sandbox_name" "$sandbox_hint" >&2
      return 1
    }
  done
  sandbox_verify_one "$sandbox_dir/sandbox-image.oci.tar" "$sandbox_dir/sandbox-image.sha256" sandbox-image.oci.tar || {
    printf 'external-test sandbox: the recorded sandbox image does not match its checksum; %s\n' "$sandbox_hint" >&2
    return 1
  }
  sandbox_expected=$(sandbox_image_key "$sandbox_containerfile" "$sandbox_distribution") || return 1
  sandbox_recorded=$(<"$sandbox_dir/sandbox-image.key")
  [[ "$sandbox_recorded" == "$sandbox_expected" ]] || {
    printf 'external-test sandbox: the recorded sandbox image is stale; %s\n' "$sandbox_hint" >&2
    printf 'recorded:\n%s\nexpected from %s:\n%s\n' "$sandbox_recorded" "$sandbox_containerfile" "$sandbox_expected" >&2
    return 1
  }
}

sandbox_verify_one() {
  local file=$1 manifest=$2 expected=$3 line regex hash actual
  [[ -f "$file" && ! -L "$file" && -f "$manifest" && ! -L "$manifest" ]] || return 1
  IFS= read -r line <"$manifest"
  [[ $(wc -l <"$manifest") -eq 1 ]] || return 1
  regex='^([A-Fa-f0-9]{64})[[:space:]][ *](.+)$'
  [[ "$line" =~ $regex ]] || return 1
  [[ "${BASH_REMATCH[2]}" == "$expected" ]] || return 1
  hash=${BASH_REMATCH[1],,}
  actual=$(sha256sum "$file")
  actual=${actual%% *}
  [[ "${actual,,}" == "$hash" ]]
}

# The distribution list is data, never sourced. Every line that is not blank
# or a comment must be `name=repository:tag@sha256:digest`, so a malformed or
# unpinned entry stops every caller rather than being skipped. Prints the names
# in list order.
sandbox_distribution_names() {
  local sandbox_list=$1 sandbox_line sandbox_count=0
  local sandbox_regex='^([a-z0-9][a-z0-9.-]*)=([a-z0-9]+([.-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)+):([A-Za-z0-9_][A-Za-z0-9._-]*)@sha256:[a-f0-9]{64}$'
  local -A sandbox_seen=()
  [[ -f "$sandbox_list" && ! -L "$sandbox_list" ]] || {
    printf 'distribution list: %s is not a regular file\n' "$sandbox_list" >&2
    return 1
  }
  while IFS= read -r sandbox_line || [[ -n "$sandbox_line" ]]; do
    [[ -z "$sandbox_line" || "$sandbox_line" == '#'* ]] && continue
    [[ "$sandbox_line" =~ $sandbox_regex && "${BASH_REMATCH[6]}" != latest ]] || {
      printf 'distribution list: not a version-tagged, digest-pinned entry: %q\n' "$sandbox_line" >&2
      return 1
    }
    [[ -z "${sandbox_seen[${BASH_REMATCH[1]}]:-}" ]] || {
      printf 'distribution list: %s is listed twice\n' "${BASH_REMATCH[1]}" >&2
      return 1
    }
    sandbox_seen[${BASH_REMATCH[1]}]=1
    printf '%s\n' "${BASH_REMATCH[1]}"
    sandbox_count=$((sandbox_count + 1))
  done <"$sandbox_list"
  ((sandbox_count > 0)) || {
    printf 'distribution list: %s names no distribution\n' "$sandbox_list" >&2
    return 1
  }
}

# Prints the pinned base reference for one listed distribution, and refuses a
# name the list does not carry.
sandbox_distribution_base() {
  local sandbox_list=$1 sandbox_name=$2 sandbox_line sandbox_names
  sandbox_names=$(sandbox_distribution_names "$sandbox_list") || return 1
  grep -Fxq -- "$sandbox_name" <<<"$sandbox_names" || {
    printf 'unknown distribution %q; known: %s\n' "$sandbox_name" "${sandbox_names//$'\n'/ }" >&2
    return 1
  }
  while IFS= read -r sandbox_line || [[ -n "$sandbox_line" ]]; do
    [[ "$sandbox_line" == "$sandbox_name="* ]] || continue
    printf '%s\n' "${sandbox_line#*=}"
    return
  done <"$sandbox_list"
}
