#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0

# Native GitLab build commands, called only by the independently pinned release
# policy. The job receives its own registry token, never a signing/publication key.
set -euo pipefail
umask 077

: "${RELEASE_TAG:?}" "${RELEASE_VERSION:?}" "${RELEASE_SHA:?}"
: "${CI_REGISTRY:?}" "${CI_REGISTRY_IMAGE:?}" "${CI_REGISTRY_USER:?}" "${CI_REGISTRY_PASSWORD:?}"
test "$(git rev-parse HEAD)" = "${RELEASE_SHA}"
work=$(mktemp -d /tmp/gommitlint-release.XXXXXXXX)
readonly work
trap 'rm -rf -- "${work}"' EXIT

go run ./scripts/releaseplan --output "${work}/release-plan"
plan="${work}/release-plan/release-plan.json"
jq -c '.images | map_values(map(.platform))' "${plan}"

export SOURCE_DATE_EPOCH
SOURCE_DATE_EPOCH=$(git log -1 --format=%ct)

# Use the same generators and GoReleaser configuration as the Forgejo release.
go build -o bin/gommitlint ./cmd/gommitlint
bash scripts/docs/completions.sh
bash scripts/docs/manpage.sh
bin/gommitlint translations compile
mkdir -p generated
gzip -9 -n -c CHANGELOG.md >generated/changelog.gz
forge-ci release notes --changelog-file CHANGELOG.md \
  --release-version "${RELEASE_TAG}" --target-file release-notes.md
GORELEASER_CURRENT_TAG="${RELEASE_TAG}" GITHUB_TOKEN='' GITLAB_TOKEN='' \
  goreleaser release --clean --verbose --skip=publish --release-notes "${PWD}/release-notes.md" \
  --config "${work}/release-plan/goreleaser.yaml"
cp -- release-notes.md dist/release-notes.md

platform="$(go env GOOS)/$(go env GOARCH)"
binary_dir=$(jq -er --arg platform "$platform" '.images.distroless[] | select(.platform == $platform) | ."build-args"[] | select(startswith("DIRPATH=")) | ltrimstr("DIRPATH=")' "${plan}")
reported=$("${binary_dir}gommitlint" --version)
case "${reported}" in
"gommitlint version ${RELEASE_VERSION} (Build date: "*")") ;;
*)
  printf 'ERROR: release binary version differs from %s\n' "${RELEASE_VERSION}" >&2
  exit 1
  ;;
esac

# The Alpine image uses musl PIE executables, with the same source stamps.
while IFS=$'\t' read -r platform interpreter output; do
  mkdir -p "$(dirname "$output")"
  CGO_ENABLED=0 GOOS="${platform%/*}" GOARCH="${platform#*/}" \
    go build -buildmode=pie -trimpath -buildvcs=false -tags 'netgo osusergo' \
    -ldflags "all=-w -buildid= -X main.version=${RELEASE_VERSION} -X main.commit=${RELEASE_SHA} -X main.date=${SOURCE_DATE_EPOCH} -I ${interpreter}" \
    -o "$output" ./cmd/gommitlint
done < <(jq -r '.musl[] | [.platform, .interpreter, .output] | @tsv' "${plan}")

# All storage is new and pod-local. The engine probes the storage backend; the
# explicit paths let this shell use its result without sourcing an output file.
mkdir -m 0700 "${work}/buildah"
export CONTAINERS_STORAGE_CONF="${work}/buildah/storage.conf"
export TMPDIR="${work}/buildah/tmp"
forge-ci container setup-buildah --install-packages=false \
  --temp-dir "${work}/buildah" --storage-conf "${CONTAINERS_STORAGE_CONF}" \
  --storage-root "${work}/buildah/storage" --tmp-dir "${TMPDIR}" \
  --env-file "${work}/buildah.env"
auth_file="${work}/registry-auth.json"
printf '%s' "${CI_REGISTRY_PASSWORD}" | forge-ci container login \
  --registry "${CI_REGISTRY}" --registry-username "${CI_REGISTRY_USER}" \
  --registry-password-file - --auth-file "${auth_file}"

build_image() {
  local role=$1 tag=$2 containerfile=$3 builds=$4 digest
  local platform
  local -a platform_args=()
  while IFS= read -r platform; do
    platform_args+=(--platform "$platform")
  done < <(jq -r '.[].platform' <<<"$builds")
  # The source project's registry holds staging tags only. The trusted signer
  # imports the recorded digest into the separate publication project.
  forge-ci container build-push-oci-image --image "${CI_REGISTRY_IMAGE}" \
    --tag "staging-${tag}" --containerfile "${containerfile}" --builds-json "${builds}" \
    --auth-file "${auth_file}" --server-url "${CI_SERVER_URL}" --repository "${CI_PROJECT_PATH}" \
    --title gommitlint --description 'Git commit message linter and validator' \
    --licenses 'EUPL-1.2 OR GPL-3.0-or-later' --vendor Itiquette \
    --authors 'Josef Andersson <janderssonse@proton.me>' \
    --ref-name "${tag}" --version "${RELEASE_TAG}" --revision "${RELEASE_SHA}"
  digest=$(forge-ci container manifest digest --ref "${CI_REGISTRY_IMAGE}:staging-${tag}" --auth-file "${auth_file}")
  forge-ci container image-evidence --registry-ref "${CI_REGISTRY_IMAGE}" --digest "${digest}" \
    "${platform_args[@]}" --auth-file "${auth_file}" \
    --trivy-output-template "dist/trivy-results-${role}-{arch}.json"
  forge-ci container ledger add --ledger dist/release-images.json --role "${role}" \
    --image-name "${CI_REGISTRY_IMAGE}" --digest "${digest}" --final-tag-name "${tag}" \
    --derive-candidate-tag --default-sbom --tag "${RELEASE_TAG}"
}

build_image distroless "${RELEASE_TAG}-distroless" Containerfile \
  "$(jq -c '.images.distroless' "${plan}")"
build_image alpine "${RELEASE_TAG}" Containerfile.alpine \
  "$(jq -c '.images.alpine' "${plan}")"

forge-ci release files import-goreleaser --dist-dir dist --manifest dist/release-files.json
forge-ci release prepare-dist --path dist --prune-dirs true
printf 'Release handoff files (bytes, path relative to dist):\n'
find dist -type f -printf '%s\t%P\n' | LC_ALL=C sort -k2
printf 'Release handoff apparent size (bytes, including directory metadata):\n'
du --apparent-size --block-size=1 --summarize dist
