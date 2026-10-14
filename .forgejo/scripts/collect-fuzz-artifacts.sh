#!/usr/bin/env bash

# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0

set -euo pipefail
readonly max_artifact_files=64
readonly max_corpus_files=$((max_artifact_files - 2))
readonly max_file_bytes=$((1024 * 1024))
readonly max_corpus_bytes=$((7 * 1024 * 1024))
readonly max_artifact_bytes=$((8 * 1024 * 1024))

repo=${1:?repository name required}
package=${2:?package path required}
target=${3:?fuzz target required}
source_root=${4:?writable package root required}
cache_root=${5:?fresh fuzz cache root required}
output=${6:?artifact output directory required}

[[ "$repo" =~ ^[A-Za-z0-9._/-]+$ ]] || {
  printf 'fuzz artifacts: invalid repository %q\n' "$repo" >&2
  exit 2
}
[[ "$target" =~ ^Fuzz[A-Za-z0-9_]+$ ]] || {
  printf 'fuzz artifacts: invalid target %q\n' "$target" >&2
  exit 2
}
[[ "$package" =~ ^\./[A-Za-z0-9_./-]+$ && "$package" != *..* && "$package" != *//* ]] || {
  printf 'fuzz artifacts: invalid package %q\n' "$package" >&2
  exit 2
}
[[ -d "$source_root" && ! -L "$source_root" && -d "$cache_root" && ! -L "$cache_root" ]] || {
  printf 'fuzz artifacts: campaign roots must be non-symlink directories\n' >&2
  exit 2
}

mkdir -p -- "$output"
[[ -d "$output" && ! -L "$output" ]] || {
  printf 'fuzz artifacts: unsafe output directory\n' >&2
  exit 2
}
chmod 0700 "$output"

manifest=$output/manifest.tsv
campaigns=$output/campaigns.tsv
[[ -e "$manifest" ]] || printf 'kind\tpackage\ttarget\tseed\tsha256\tbytes\trestore_path\treproduction\n' >"$manifest"
[[ -e "$campaigns" ]] || printf 'repository\tpackage\ttarget\tcampaign_seed\tkept_files\tcorpus_files\tcorpus_bytes\trejected_secret_files\trejected_bounded_files\n' >"$campaigns"

contains_secret() {
  LC_ALL=C grep -aEiq -- '-----BEGIN ([A-Z0-9 ]+ )?PRIVATE KEY-----|(^|[^A-Za-z0-9])(github_pat_|gh[pousr]_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}|AKIA[A-Z0-9]{16}|xox[baprs]-[A-Za-z0-9-]{10,})([^A-Za-z0-9]|$)|[Aa]uthorization:[[:space:]]*(Bearer|Basic)[[:space:]]+[A-Za-z0-9._~+/-]{12,}|https?://[^/@[:space:]]+:[^/@[:space:]]+@' "$1"
}

validate_artifacts() {
  local file name size files=0 bytes=0
  while IFS= read -r -d '' file; do
    [[ -f "$file" && ! -L "$file" ]] || {
      printf 'fuzz artifacts: unexpected output entry %q\n' "$file" >&2
      return 1
    }
    name=$(basename -- "$file")
    case "$name" in
    manifest.tsv | campaigns.tsv) ;;
    *.corpus) [[ "$name" =~ ^Fuzz[A-Za-z0-9_]+-[a-f0-9]{64}[.]corpus$ ]] || {
      printf 'fuzz artifacts: invalid corpus name %q\n' "$name" >&2
      return 1
    } ;;
    *)
      printf 'fuzz artifacts: unexpected output file %q\n' "$name" >&2
      return 1
      ;;
    esac
    size=$(stat -c '%s' -- "$file")
    ((size <= max_file_bytes)) || {
      printf 'fuzz artifacts: output file exceeds size cap: %s\n' "$name" >&2
      return 1
    }
    contains_secret "$file" && {
      printf 'fuzz artifacts: secret pattern found in retained output: %s\n' "$name" >&2
      return 1
    }
    files=$((files + 1))
    bytes=$((bytes + size))
  done < <(find "$output" -mindepth 1 -maxdepth 1 -print0 | sort -z)
  ((files <= max_artifact_files && bytes <= max_artifact_bytes)) || {
    printf 'fuzz artifacts: retained output exceeds artifact caps\n' >&2
    return 1
  }
}

validate_artifacts
kept=0
stored_files=0
total_bytes=0
rejected_secret=0
rejected_bounds=0
declare -A seen=()
shopt -s nullglob
for existing in "$output"/*.corpus; do
  digest=$(sha256sum -- "$existing")
  digest=${digest%% *}
  [[ "$(basename -- "$existing")" == *"-$digest.corpus" ]] || {
    printf 'fuzz artifacts: corpus digest does not match its name\n' >&2
    exit 1
  }
  seen[$(basename -- "$existing")]=1
  stored_files=$((stored_files + 1))
  total_bytes=$((total_bytes + $(stat -c '%s' -- "$existing")))
done
shopt -u nullglob

collect_one() {
  local kind=$1 file=$2 seed size digest checkout_seed checkout_digest destination destination_name restore reproduction
  [[ -f "$file" && ! -L "$file" ]] || return 0
  seed=$(basename -- "$file")
  [[ "$seed" =~ ^[A-Za-z0-9_-]{1,128}$ ]] || return 0
  size=$(stat -c '%s' -- "$file")
  if ((size > max_file_bytes)); then
    rejected_bounds=$((rejected_bounds + 1))
    return 0
  fi
  if contains_secret "$file"; then
    rejected_secret=$((rejected_secret + 1))
    return 0
  fi
  digest=$(sha256sum -- "$file")
  digest=${digest%% *}
  destination_name=${target}-${digest}.corpus
  [[ -z "${seen[$destination_name]:-}" ]] || return 0
  if [[ "$kind" == minimized-failure ]]; then
    checkout_seed=${package#./}/testdata/fuzz/$target/$seed
    if [[ -f "$checkout_seed" && ! -L "$checkout_seed" ]]; then
      checkout_digest=$(sha256sum -- "$checkout_seed")
      checkout_digest=${checkout_digest%% *}
      [[ "$checkout_digest" != "$digest" ]] || return 0
    fi
  fi
  if ((stored_files >= max_corpus_files || total_bytes + size > max_corpus_bytes)); then
    rejected_bounds=$((rejected_bounds + 1))
    return 0
  fi
  destination=$output/$destination_name
  install -m 0600 -- "$file" "$destination"
  restore=${package#./}/testdata/fuzz/$target/$seed
  reproduction="GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -run='^${target}/${seed}$' $package"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$kind" "$package" "$target" "$seed" "$digest" "$size" "$restore" "$reproduction" >>"$manifest"
  seen[$destination_name]=1
  kept=$((kept + 1))
  stored_files=$((stored_files + 1))
  total_bytes=$((total_bytes + size))
}
crasher_root=$source_root/testdata/fuzz/$target
if [[ -d "$crasher_root" && ! -L "$crasher_root" ]]; then
  while IFS= read -r -d '' file; do collect_one minimized-failure "$file"; done < <(find "$crasher_root" -type f -print0 | sort -z)
fi
while IFS= read -r -d '' file; do collect_one generated-corpus "$file"; done < <(find "$cache_root" -type f -print0 | sort -z)

campaign_seed='committed seed corpus; fresh Go native fuzz cache; Go exposes no RNG seed flag'
printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
  "$repo" "$package" "$target" "$campaign_seed" "$kept" "$stored_files" "$total_bytes" "$rejected_secret" "$rejected_bounds" >>"$campaigns"
chmod 0600 "$manifest" "$campaigns"
while IFS= read -r -d '' file; do chmod 0600 "$file"; done < <(find "$output" -maxdepth 1 -type f -name '*.corpus' -print0)
validate_artifacts
