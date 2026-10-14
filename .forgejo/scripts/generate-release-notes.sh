#!/usr/bin/env bash
# SPDX-FileCopyrightText: The Gommitlint Authors
# SPDX-License-Identifier: CC0-1.0
#
# Generate release notes for a given version tag.
# Usage: ./.forgejo/scripts/generate-release-notes.sh <version-tag>
# Example: ./.forgejo/scripts/generate-release-notes.sh v0.5.1
#
# Outputs: release-notes.md in current directory

set -euo pipefail

VERSION="${1:-}"
FORGEJO_SERVER="${FORGEJO_SERVER:?Forgejo server required}"
FORGEJO_REPO="${FORGEJO_REPO:?Forgejo repository required}"

if [[ -z "$VERSION" ]]; then
  echo "Usage: $0 <version-tag>" >&2
  echo "Example: $0 v0.5.1" >&2
  exit 1
fi

VERSION_NUM="${VERSION#v}" # Remove 'v' prefix for display
SERVER_URL=$(
  node <<'NODE'
const value = process.env.FORGEJO_SERVER ?? "";
let parsed;
try {
  parsed = new URL(value);
} catch {
  console.error("ERROR: forgejo.server_url must be a canonical HTTPS origin");
  process.exit(64);
}
if (parsed.protocol !== "https:" || parsed.origin !== value) {
  console.error("ERROR: forgejo.server_url must be a canonical HTTPS origin");
  process.exit(64);
}
process.stdout.write(parsed.origin);
NODE
)
if [[ ! "$FORGEJO_REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "ERROR: forgejo.repository must be a nonempty owner/repository pair" >&2
  exit 64
fi
REPOSITORY_OWNER=${FORGEJO_REPO%%/*}
REPOSITORY_NAME=${FORGEJO_REPO#*/}
if [[ "$REPOSITORY_OWNER" == . || "$REPOSITORY_OWNER" == .. ||
  "$REPOSITORY_NAME" == . || "$REPOSITORY_NAME" == .. ]]; then
  echo "ERROR: forgejo.repository must be a nonempty owner/repository pair" >&2
  exit 64
fi
PROJECT_URL="${SERVER_URL}/${FORGEJO_REPO}"

# The prepare job already rendered and signed this version's changelog.
NOTES=$(forge-ci release notes --changelog-file CHANGELOG.md \
  --release-version "$VERSION" --target-file -)

# Build release notes file
{
  printf '%s\n\n' "## gommitlint ${VERSION_NUM}"
  printf '%s\n' "$NOTES"
  printf '\n%s\n\n' "---"
  printf '%s\n' "**Installation:** See [README](${PROJECT_URL}#installation)"
  printf '%s\n' "**Verification:** See [the verification guide](${PROJECT_URL}/src/tag/${VERSION}/docs/modules/ROOT/pages/verify.adoc)"
  printf '%s\n' "**Contributing:** See [CONTRIBUTING.md](${PROJECT_URL}/src/branch/main/CONTRIBUTING.md)"
  printf '%s\n' "**Issues:** ${PROJECT_URL}/issues"
} >release-notes.md

echo "Generated release-notes.md ($(wc -l <release-notes.md) lines)"
