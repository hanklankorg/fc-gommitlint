#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
#
# SPDX-License-Identifier: CC0-1.0

# Updates pinned container image digests in Containerfile, Containerfile.alpine, Containerfile.sandbox, and justfile
# Usage: ./scripts/dev/upgrade-container-digests.sh (run from project root)
# Dependencies: skopeo, cosign
#
# A new digest is only written after its signature verifies against the
# publisher's documented signing identity. Images without one are updated
# with a warning.

set -euo pipefail

for tool in skopeo cosign; do
  command -v "$tool" >/dev/null 2>&1 || {
    printf "\033[1;31m✗\033[0m %s not found\n" "$tool" >&2
    exit 1
  }
done

# Verifies the signature of image@digest. Returns 0 when verified or when the
# publisher documents no signing identity (logged), 1 when verification fails.
verify_signature() {
  local image=$1 digest=$2
  case "$image" in
  gcr.io/distroless/*)
    # Keyless identity from the distroless README (image signing section).
    if ! cosign verify \
      --certificate-identity keyless@distroless.iam.gserviceaccount.com \
      --certificate-oidc-issuer https://accounts.google.com \
      "${image}@${digest}" >/dev/null 2>&1; then
      printf "  \033[1;31m✗ %s@%s signature verification failed\033[0m\n" "$image" "$digest" >&2
      return 1
    fi
    printf "  \033[32m✓ %s@%s signature verified\033[0m\n" "$image" "$digest"
    ;;
  *)
    printf "  \033[33m⚠ %s has no documented signing identity; signature not verified\033[0m\n" "$image"
    ;;
  esac
}

updated=0
failed=0

for file in Containerfile Containerfile.alpine Containerfile.sandbox justfile; do
  [[ -f "$file" ]] || continue

  # Match lines with real pinned images (digest must be 64+ hex chars)
  while IFS= read -r line; do
    if [[ "$line" =~ ([a-zA-Z0-9._/-]+):([a-zA-Z0-9._-]+)@(sha256:[a-f0-9]{64}) ]]; then
      image="${BASH_REMATCH[1]}"
      tag="${BASH_REMATCH[2]}"
      old_digest="${BASH_REMATCH[3]}"

      # Get latest digest for this tag
      new_digest=$(skopeo inspect --format '{{.Digest}}' "docker://${image}:${tag}" 2>/dev/null) || {
        printf "  \033[33m⚠ Could not inspect %s:%s\033[0m\n" "$image" "$tag"
        continue
      }

      if [[ "$new_digest" != "$old_digest" ]]; then
        if ! verify_signature "$image" "$new_digest"; then
          failed=$((failed + 1))
          continue
        fi
        sed -i.bak "s|${image}:${tag}@${old_digest}|${image}:${tag}@${new_digest}|g" "$file"
        rm -f "${file}.bak"
        printf "  \033[32m✓ %s:%s\033[0m digest updated in %s\n" "$image" "$tag" "$file"
        updated=$((updated + 1))
      else
        printf "  \033[2m· %s:%s up to date\033[0m\n" "$image" "$tag"
      fi
    fi
  done < <(grep -n '@sha256:' "$file")
done

if [[ $updated -eq 0 ]]; then
  printf "\033[2m  All container images are up to date\033[0m\n"
else
  printf "\033[32m✓ Updated %d container image(s)\033[0m\n" "$updated"
fi

if [[ $failed -gt 0 ]]; then
  printf "\033[1;31m✗ %d digest(s) left unchanged: signature verification failed\033[0m\n" "$failed" >&2
  exit 1
fi
