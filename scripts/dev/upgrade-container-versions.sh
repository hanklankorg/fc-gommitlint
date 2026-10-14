#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Bumps container image version tags (e.g., fedora:41 → 43, alpine:3.21 → 3.23)
# Updates both the tag and digest in Containerfile, Containerfile.alpine, Containerfile.sandbox, and justfile
# Usage: ./scripts/dev/upgrade-container-versions.sh (run from project root)
# Dependencies: skopeo, jq

set -euo pipefail

command -v skopeo >/dev/null 2>&1 || {
  printf "\033[1;31m✗\033[0m skopeo not found\n"
  exit 1
}
command -v jq >/dev/null 2>&1 || {
  printf "\033[1;31m✗\033[0m jq not found\n"
  exit 1
}

updated=0

for file in Containerfile Containerfile.alpine Containerfile.sandbox justfile; do
  [[ -f "$file" ]] || continue

  while IFS= read -r line; do
    if [[ "$line" =~ ([a-zA-Z0-9._/-]+):([a-zA-Z0-9._-]+)@(sha256:[a-f0-9]{64}) ]]; then
      image="${BASH_REMATCH[1]}"
      tag="${BASH_REMATCH[2]}"
      old_digest="${BASH_REMATCH[3]}"

      # Skip non-version tags (codenames like "trixie", "nonroot")
      if ! [[ "$tag" =~ ^[0-9] ]]; then
        printf "  \033[2m· %s:%s (codename, skip)\033[0m\n" "$image" "$tag"
        continue
      fi

      # Get available tags from registry
      tags_json=$(skopeo list-tags "docker://${image}" 2>/dev/null) || {
        printf "  \033[33m⚠ Could not list tags for %s\033[0m\n" "$image"
        continue
      }

      # Find latest version matching the same pattern
      if [[ "$tag" =~ ^([0-9]+)\.([0-9]+)$ ]]; then
        major="${BASH_REMATCH[1]}"
        latest=$(echo "$tags_json" | jq -r '.Tags[]' | grep -E "^${major}\.[0-9]+$" | sort -V | tail -1)
      elif [[ "$tag" =~ ^[0-9]+$ ]]; then
        latest=$(echo "$tags_json" | jq -r '.Tags[]' | grep -E '^[0-9]+$' | sort -V | tail -1)
      else
        printf "  \033[2m· %s:%s (non-standard version, skip)\033[0m\n" "$image" "$tag"
        continue
      fi

      if [[ -n "$latest" ]] && [[ "$latest" != "$tag" ]] && [[ "$(printf '%s\n%s\n' "$tag" "$latest" | sort -V | tail -1)" == "$latest" ]]; then
        # Get new digest for the latest tag
        new_digest=$(skopeo inspect --format '{{.Digest}}' "docker://${image}:${latest}" 2>/dev/null) || {
          printf "  \033[33m⚠ Could not inspect %s:%s\033[0m\n" "$image" "$latest"
          continue
        }

        sed -i "s|${image}:${tag}@${old_digest}|${image}:${latest}@${new_digest}|g" "$file"
        printf "  \033[32m✓ %s\033[0m %s → %s in %s\n" "$image" "$tag" "$latest" "$file"
        updated=$((updated + 1))
      else
        printf "  \033[2m· %s:%s up to date\033[0m\n" "$image" "$tag"
      fi
    fi
  done < <(grep '@sha256:' "$file")
done

if [[ $updated -eq 0 ]]; then
  printf "\033[2m  All container image versions are up to date\033[0m\n"
else
  printf "\033[32m✓ Updated %d container image version(s)\033[0m\n" "$updated"
fi
