# Hosted GitHub trial fixtures

These files are what Forge CI's hosted GitHub consumer trial runs for
Gommitlint. They are fixtures: nothing here is an active workflow, and this
repository starts no live run.

| File | Copied to | Purpose |
|---|---|---|
| `workflows/pullrequest.yml` | `.github/workflows/pullrequest.yml` | Pull-request gate through Forge CI's orchestrator, plus the caller's own tests |
| `workflows/release.yml` | `.github/workflows/release.yml` | Reduced release: amd64 binary, one image, keyless signatures, GitHub release |
| `artifacts.yml` | read in place | What the release builds |
| `image/Containerfile` | read in place | The release image; `image/` is its build context |

The trial is started from a Forge CI checkout, inside Forge Lab's hosted
organization session. It copies the two workflows into a disposable
repository of a test organization and replaces `diggsweden/forge-ci`, the
`v0.90.0` pin and the runtime images with the engine under test.

The release is deliberately smaller than `.forgejo/workflows/release.yml`.
Forge CI's GitHub orchestrator builds plain binaries and images; archives,
distribution packages, the distroless image and key-based Cosign belong to the
Forgejo release and are not reproduced here.

Hosted trial pull request.
