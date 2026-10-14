# Maintainer Release Guide

This guide is for Gommitlint maintainers operating the production release
workflow.
It does not authorize contributors to publish artifacts and is not an end-user
verification guide.

## Authorities and Boundary

The production contract is split across these sources:

| Source | Authority |
| --- | --- |
| `.forgejo/workflows/release.yml` | Gommitlint trigger, build, image, hand-off, and caller-side input and secret wiring |
| Pinned `itiquette/forge-ci` reusable workflows (`.forgejo/workflows/`) | Release preparation; signer inputs, required secret interface, signing, image promotion, and publication |
| `itiquette/ci-policy` | The organization Cosign public key and its digest (`keys/`), and the published signer image |
| `.goreleaser.yaml` | Binary, archive, package, checksum, and SBOM build configuration |
| `docs/contributing/packaging.md` | Local and distribution packaging procedures |
| `docs/modules/ROOT/pages/security-posture.adoc` | Release threat model and SLSA isolation design |
| `docs/modules/ROOT/pages/verify.adoc` | Verification procedure for published artifacts |

There is no local publication recipe.
`just release-snapshot` creates an unsigned local snapshot for development only.
It is not a minimal release and its output must never be published as a
production release.
Production releases require the complete signing and publication contract. No
unsigned fallback and no optional-signing mode are supported.

## Trust Preconditions

The current working-tree migration uses a locally verified Forge CI binary and
an amd64 signer candidate at `localhost/forge-ci-signer`. The organization
hosting coordinates are staged, not published. Replace the local image reference
with the verified registry-served digest after publication; a multi-architecture
index may have a different digest. Local checks do not authorize release.

> [!IMPORTANT]
> The Gommitlint caller is aligned with the currently pinned forge-ci source,
> but that revision is not carried by an independently authenticated signed
> forge-ci release. The workflow therefore stays disabled unless the protected
> `FORGE_CI_AUTHENTICATED_COMMIT` variable exactly matches the pin. Do not set
> it until an operator has independently authenticated that revision and the
> pull-request isolation gate passes against it.

Before creating a release request:

1. Make sure that `main` is protected and the request's source commit passed the required pull-request gates.
2. Make sure that the working tree is clean and matches the reviewed source commit on `main`.
3. Run `just verify-full` from the repository root.
   Use a private checkout without production credentials and retain its results.
   This local gate builds artifacts but does not publish them or replace the PR isolation evidence.
   Then run `just lint-links`. It is the blocking link gate: `just lint` and the pull-request workflow only report broken links, and the release workflow does not lint.
4. Make sure that every forge-ci reference uses one exact SHA from an independently authenticated signed forge-ci release.
   This includes actions, both reusable workflows, and their `forge-ci-sha` inputs.
5. Make sure that a trusted operator set the protected
   `FORGE_CI_AUTHENTICATED_COMMIT` variable to that exact SHA. Repository
   automation must never create, default, or update this variable.
6. Make sure that the other required repository variables match values supplied by a
   trusted Forgejo operator.
7. Compare caller inputs and the explicit `secrets:` blocks with the reusable
   workflows at that exact pin. Compare the `signer-image` digest with the one
   ci-policy published for that pin, and `cosign-public-key-sha256` with
   ci-policy's `keys/cosign.pub.sha256`.
8. Require the `Verify SLSA L3 isolation invariants` step and its containing pull-request job to pass.
   The pinned `forge-ci/.forgejo/actions/check-l3-isolation` action is the
   supported validator and invokes the layer's checksum-verified
   `forge-ci validate isolation` command.
   There is no supported local shell validator. `just verify-l3-isolation`
   fails to make that missing evidence explicit.
9. Examine the gate output for the reviewed source commit and the same forge-ci pin used by the release workflow.
10. Make sure that all required repository or organization secrets are configured
   before pushing a tag.

Never use `@main`, a floating tag, mixed forge-ci pins, or `secrets: inherit`.
Do not weaken the digest hand-off or expose signing keys to the consumer build
job.

The local gate can write build output. Recheck source status afterward and
review unexpected changes instead of discarding them to obtain a clean result.
A passing policy validator is required evidence, not external SLSA certification
or proof that the blocked forge-ci revision is production-ready.

## Required Secrets

The pinned prepare workflow checks that the release and signing secrets below
are configured before it checks out consumer-controlled
repository content. It reads the two tokens as `RELEASE_TOKEN` and
`RELEASE_REGISTRY_TOKEN`; the `secrets:` blocks of `release.yml` map this
repository's names to them.
`DEPENDENCY_GITHUB_TOKEN` and `DEPENDENCY_FORGE_TOKEN` are optional and only
the build job's tool setup reads them. The build
job installs its tools from `mise.lock`, by locked URL and checksum, so it does
not depend on the token. The prepare workflow takes no rate-limit token and
needs none: it installs its changelog renderer from a pinned archive that it
verifies against a checksum, without a call to the GitHub API.

The six signing secrets carry the names the delivery layer reads, `RELEASE_<what>`.
They were renamed with the layer: a repository or organization that still
stores `SSH_SIGNING_KEY_RELEASE`, `GPG_SIGNING_KEY`, `GPG_SIGNING_FINGERPRINT`,
`GPG_SIGNING_PASSWORD`, `COSIGN_SIGNING_KEY` or `COSIGN_SIGNING_PASSWORD` must
store the same values under the names below before a release. The prepare
workflow stops a release that finds one missing.
The check is for presence. It does not prove that signing keys and passwords
are valid; a separate preflight job verifies that `PACKAGE_PUBLISH_TOKEN`
can read the repository.
Review secret names and operator records without retrieving or logging values.

| Secret | Purpose |
| --- | --- |
| `RELEASE_API_TOKEN` | Forgejo release creation and asset upload |
| `PACKAGE_PUBLISH_TOKEN` | Registry authentication and signed image and package publication |
| `DEPENDENCY_GITHUB_TOKEN` | Optional: GitHub-backed tool resolution in the build job |
| `DEPENDENCY_FORGE_TOKEN` | Optional: Forgejo-backed tool resolution in the build job; separate from the package-publishing credential |
| `RELEASE_SSH_SIGNING_KEY` | Signed changelog commit and release tag |
| `RELEASE_GPG_PRIVATE_KEY` | Distribution package signatures |
| `RELEASE_GPG_FINGERPRINT` | Expected package-signing key identity |
| `RELEASE_GPG_PASSPHRASE` | Package-signing key decryption |
| `RELEASE_COSIGN_PRIVATE_KEY` | Checksum, SBOM, provenance, and OCI signatures and attestations |
| `RELEASE_COSIGN_PASSPHRASE` | Cosign key decryption |

Store values only in the Forgejo secret store.
Before deploying these workflow bindings, migrate `CODEFLOE_TOKEN_RELEASE_API`
to `RELEASE_API_TOKEN`, `CODEFLOE_TOKEN_PACKAGES` to `PACKAGE_PUBLISH_TOKEN`,
and `GH_RATE_LIMIT_TOKEN` to `DEPENDENCY_GITHUB_TOKEN`. Configure a separate
dependency credential as `DEPENDENCY_FORGE_TOKEN` if authenticated tool
resolution is needed; do not reuse the package-publishing credential.
The Pages workflow separately reads `PAGES_DEPLOY_TOKEN` instead of
`CODEFLOE_TOKEN_PAGES`.
Do not place values in this repository, local release notes, command lines,
issues, logs, or this guide.
Key creation, custody, rotation, and revocation are operator responsibilities,
not a repository bootstrap procedure.

## Required Variables

| Variable | Purpose |
| --- | --- |
| `CI_RUNNER_LABEL` | Exact trusted runner label provisioned for this repository (`ci-builder` in production) |
| `CI_JOB_IMAGE` | Full immutable digest reference to the consumer-owned pull-request job image |
| `FORGE_CI_AUTHENTICATED_COMMIT` | Protected, operator-supplied exact forge-ci SHA admitted by the action-free release trust gate |
| `COSIGN_PUBLIC_KEY` | PEM text of ci-policy's `keys/cosign.pub`; the signer accepts it only if it hashes to the digest written in `release.yml` |
| `RELEASE_GIT_URL` | Exact operator-supplied `ssh://git@host[:port]/itiquette/gommitlint.git` write destination |
| `RELEASE_GIT_HOST_KEY_FINGERPRINT` | Independently supplied ed25519 host-key fingerprint for that SSH endpoint |

`RELEASE_PLATFORMS` is an optional, comma-separated subset of the container
platforms declared in `packaging/release-platforms.json`. Leave it empty for
the ordinary full release matrix. A focused lab trial may set it to
`linux/amd64`; the consumer's `scripts/releaseplan` command then selects the
matching binaries, packages, musl builds, image builds and image-scan platforms
together. It writes a temporary GoReleaser configuration without editing
`.goreleaser.yaml`. Unknown or duplicate selections fail before the build.
Production releases should retain the full declared matrix unless a different
release scope was explicitly selected. See
[`scripts/releaseplan/README.md`](../../scripts/releaseplan/README.md).

The native GitLab entry points are `.gitlab/check-release.sh` and
`.gitlab/build-release.sh`, called by an independently controlled, pinned
Forge CI release policy. They use the same release plan and keep signing and
final publication outside the build job. The consumer's MR example and
builder-image instructions are under [`examples/gitlab`](../../examples/gitlab/).

`release.yml` names the repository that serves the delivery layer itself, as
`forge-ci-repository: itiquette/forge-ci` beside each `forge-ci-sha`.
The `FORGE_CI_REPOSITORY` variable is not read and may be removed.

Treat runner selection as part of the release trust boundary.
Do not derive Git connection values from the Forgejo HTTPS URL.
Do not obtain the expected SSH fingerprint from the release SSH endpoint during the release job.
Leave `FORGE_CI_AUTHENTICATED_COMMIT` unset until the
pinned revision has been independently authenticated. A missing or mismatched
value disables the release before any action or release secret is used.

## Trigger

The current Gommitlint workflow accepts only an immutable
`release-request/vMAJOR.MINOR.PATCH` tag.
Forgejo's tag glob is deliberately broad. After the trust gate and the token
preflight, the prepare workflow applies the strict release-request and
stable-SemVer checks.
Pre-release and extra-component tags are rejected.

Only after the trust preconditions pass, create the request from reviewed `main`.
Set `RELEASE_COMMIT` to the exact source commit approved for this request.

Before pushing, compare `origin`'s push destination with the independently
approved Gommitlint repository URL. Do not print a URL that contains credentials.
Coordinate requests and reruns so that only one release operation targets this
repository at a time. Immutable tags do not serialize different release versions.

The Bash block below stops before pushing if any local check or tag creation
fails:

```bash
(
  set -euo pipefail
  VERSION=1.2.3
  RELEASE_COMMIT='<reviewed full source commit ID>'
  [[ "$VERSION" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
  BRANCH=$(git symbolic-ref --short HEAD)
  test "$BRANCH" = main
  HEAD_COMMIT=$(git rev-parse --verify 'HEAD^{commit}')
  test "$HEAD_COMMIT" = "$RELEASE_COMMIT"
  STATUS=$(git status --porcelain)
  test -z "$STATUS"
  REQUEST_TAG="release-request/v${VERSION}"
  git tag --annotate "$REQUEST_TAG" "$RELEASE_COMMIT" -m "Request release v${VERSION}"
  git push origin "refs/tags/${REQUEST_TAG}:refs/tags/${REQUEST_TAG}"
)
```

If the local request tag already exists, tag creation fails and this block does
not push it. Do not replace or delete that tag to retry the block.
If pushing fails after tag creation, retain the tag and inspect the remote/run
state before an approved retry. A push can reach the server even when the
client loses the response.

The workflow requires an annotated tag, but it does not cryptographically verify
a human request-tag signature because no request-tag allow-signers policy is
configured. It peels the tag and requires that commit to match the triggering
event and to be the head of the freshly fetched `origin/main`: preparation
pushes the release commit to `main`, so a request on any other commit is
refused. The block above therefore fails in the workflow when `main` has moved
since `RELEASE_COMMIT` was reviewed.
The request tag is immutable authorization input. The trusted prepare workflow
derives and creates the final `vX.Y.Z` tag. Never create or move that final tag
manually. The head-of-`main` rule is the workflow's source check, not proof
that the commit received human review.

## Workflow Choreography

The workflow has five caller-visible stages.

### Authenticate Forge CI

The first job runs only an inline comparison and receives no release secret.
It compares `FORGE_CI_AUTHENTICATED_COMMIT` with the expected SHA embedded in
that job. It does not fetch or cryptographically authenticate a forge-ci
publication at runtime. The protected value records an operator's prior trust
decision.

All later stages depend on this gate. Consistency between the embedded SHA,
action references, reusable workflows, and their `forge-ci-sha` inputs must
also pass source review and the PR policy checks. The inline comparison alone does not scan all
those references.

### Preflight

A job of this workflow verifies `PACKAGE_PUBLISH_TOKEN` with the pinned
`verify-forgejo-token` action before anything is prepared. It is separate
because no token may enter the job that holds the SSH release key.

### Prepare

The prepare job is the pinned `prepare-release.yml` workflow; it runs no step
of this repository. It checks every required secret for presence before any
checkout, and its no-node checkout fetches all refs.
It validates the stable release request, requires the request commit at the
head of `origin/main`, and requires an annotated request tag whose peeled
commit is the event's.
Only then does its prepare action receive the SSH release key.

That action generates `CHANGELOG.md`.
When the changelog changes, preparation creates a signed commit.
An unchanged rerun need not create another commit. Preparation creates and signs
the final release tag once and returns the prepared release tag and SHA.

The prepared SHA can differ from the request's source SHA because preparation
can add a changelog commit. Downstream jobs must use the returned identity,
not reconstruct it from the request tag.
Do not hand-edit a release changelog after this stage or move the resulting tag
manually.

### Build and Hand Off

The build job checks out the prepared release SHA.
Tool setup installs only required build tools with its cache disabled.
The job generates release documentation and runs the build-only GoReleaser
configuration with publication disabled at invocation.
It also builds both OCI image variants with the pinned `build-release-image`
action, which pushes each manifest list, scans every pushed platform into
`dist/trivy-results-<role>-<arch>.json`, and records the image in the
digest-first ledger. `forge-ci release files import-goreleaser` then writes
`dist/release-files.json`, the manifest that declares the public files, and
the pinned hand-off action uploads `dist/`.
The hand-off emits the canonical digest consumed by the signer.

Registry login uses the pinned action to create a mode-0600 auth file under the
job's temporary directory. Both image builds receive that path explicitly.
No shared registry-auth location is used, and runner job cleanup owns the
temporary file.

This job may use scoped package and rate-limit tokens needed to build and push
images. It must never receive SSH, Cosign, or GPG signing keys or the release-API
token.
Unsigned files in `dist/` are intermediate build output, not a release.

### Sign and Publish

The cross-repository reusable workflow checks out the exact pinned forge-ci
helpers and the prepared Gommitlint release SHA. Its privileged jobs run in
the signer image `release.yml` names by digest.
It verifies the release tag, that the release commit is the single child of
the request commit, the unsigned `dist/` digest and the declared release files
before signing.
Its internal privilege split signs assets and images, promotes image tags with
registry-only credentials, then verifies the signed hand-off and publishes with
release-API-only credentials.
It also generates and signs SLSA provenance.

At the current pin, the reusable workflow has a secret-free `coordinates`
preflight and `sign`, `promote`, `publish`, and `cleanup` jobs. Later
forge-ci revisions can have a different layout.
Use the workflow and runbook from the exact pin, not the current `main` branch,
when deciding how to recover a failed stage.

Each `secrets:` call-site block is the complete privilege hand-off to its
workflow. Keep them explicit and synchronized with the pinned workflow's
`workflow-call-secrets-contract` header.

## Completion and Failure

Treat the release as complete only when every prepare, build, sign, image, and
publish job is green.
Then compare the final tag's commit with the returned prepared release SHA.
Follow [Verify Releases](../modules/ROOT/pages/verify.adoc) against the published release.
Compare the published artifact set and image digests with the release's recorded
checksums and image ledger. A green upload job alone does not authenticate the
files obtained by a consumer.

Retain the run URL, request tag/source SHA, forge-ci pin, prepared tag/SHA,
hand-off digests, and final verification results. Do not include secret values
in those records.

On failure, preserve the failed run.
Before you retry, identify the failed privilege stage.
Failure does not mean that nothing was published. Image signatures, promoted
tags, or some release assets can already exist.
Do not force-move the final tag, bypass the signer, upload unsigned artifacts,
or publish a partial release manually.
Use the runbook from the exact pinned forge-ci revision for rerun and recovery
semantics.

## Contract Updates

Update the forge-ci pin only through a reviewed pull request.
In one change:

1. Select an exact commit from a trusted signed forge-ci release.
2. Update every action reference, reusable-workflow reference, `forge-ci-sha`
   input, and the expected SHA of the trust gate together, and the
   `signer-image` digest to the image ci-policy published for that commit.
3. Re-read the pinned workflows' required inputs and secret-contract headers.
4. Update Gommitlint's caller inputs and explicit secret mapping to match.
5. Run the workflow policy and L3 isolation guards.
6. Require review of both the caller diff and the pinned upstream diff.

The current workflow and pinned upstream contract outrank this explanatory
guide if they disagree.
Resolve any disagreement before cutting a release. Do not document around a
failing trust boundary.
