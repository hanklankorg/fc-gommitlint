# Release platform selection

`RELEASE_PLATFORMS` is an optional comma-separated subset of the platforms in
`packaging/release-platforms.json`, for example `linux/amd64` or
`linux/arm64`. Empty preserves the normal release: the original GoReleaser
configuration, both declared container platforms, and both musl builds.

The release job runs:

```sh
go run ./scripts/releaseplan --output "$RUNNER_TEMP/release-plan"
```

The resulting `goreleaser.yaml` and `release-plan.json` select binaries,
packages, container builds, musl binaries, and image-scan platforms together.
The original configuration is not edited. Unknown, duplicate, or unsupported
selections fail before any build. The version check executes the selected
host-native binary; run an ARM-only trial on an ARM runner.

Forgejo supplies the optional repository variable. GitLab supplies it to its
key-free build job. The lab passes it explicitly for the selected trial:

```sh
just test-live-consumer /absolute/consumer release \
  FORGE_CI_AUTHENTICATED_COMMIT=@layer-commit RELEASE_PLATFORMS=linux/amd64
```

The container build arguments and musl loader paths belong to the consumer's
declaration. Adding a supported platform updates that declaration and the
GoReleaser matrix; the selector contains no preferred host or architecture.
