# Native GitLab checks and releases

`merge-request.yml` is the reviewed MR gate. Copy it to `.gitlab-ci.yml` in a
clean trial checkout. The check component is pinned, and its hosted use waits
for the engine's authenticated publication. The Kubernetes CPU/storage variables
request capacity for the full cold-cache gate; the runner must allow those
bounded overrides. Other executors use their own resource configuration.

Releases use an independently controlled Forge CI policy with these key-free
consumer commands:

```yaml
checks:
  - bash .gitlab/check-release.sh
build:
  - bash .gitlab/build-release.sh
```

Build the release job image separately, from a reviewed digest-pinned
`Containerfile.ci` image, using `Containerfile.release` and its dedicated
`release.containerignore`. Its `CHECK_IMAGE` build argument is required. This
image contains the native packaging/build tools; signing tools and credentials
remain in the policy's separate signer image and environments.

Both release entry points consume the normal repository configuration. Optional
`RELEASE_PLATFORMS` selects a declared subset through `scripts/releaseplan`; an
empty setting preserves the full release. For an amd64-only disposable trial,
run the engine's recipe:

```sh
just test-live-gitlab-consumer-release /absolute/consumer \
  /absolute/check.tar /absolute/build.tar /absolute/signer.tar \
  RELEASE_PLATFORMS=linux/amd64
```

Use the supported full-release wrapper when the selected lab runner requires it
for nested image builds. Keep the consumer build free of signing keys and final
publication credentials. The normal maintainer release prerequisites still
apply; a lab result is not production publication approval.
