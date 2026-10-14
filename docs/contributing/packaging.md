# Packaging Guide

Guide for building, packaging, and distributing gommitlint across platforms and architectures.

Commands assume Bash at the repository root. Examples are alternatives, not one
sequence to run in full. Use a disposable, reviewed checkout with the required
tools available. Builds execute repository code, including the built product
when generating completions and translations.

Preserve needed `bin/`, `dist/`, generated assets, and local signing keys before
building. Recipes can replace outputs, download tools and dependencies, and
leave partial state after failure. A successful local build is not published
release evidence or distribution acceptance.

## Quick Reference

```text
debian/                        # Debian source package (at repo root, required by Debian tools)
packaging/
├── nfpm/
│   └── changelog.yaml         # nFPM package changelog (YAML format)
└── native/
    ├── rpm/
    │   └── gommitlint.spec    # RPM spec file
    └── alpine/
        └── APKBUILD           # Alpine build recipe
```

```bash
# Rebuild binaries/assets and create unsigned nFPM snapshot packages
just package-nfpm

# Native packages (from source)
just package-native-debian
just package-native-rpm
just package-native-alpine
```

## Table of Contents

- [Overview](#overview)
- [Architecture Support](#architecture-support)
  - [Go Binary Builds](#go-binary-builds)
  - [Package Architectures](#package-architectures)
- [Building from Source](#building-from-source)
  - [Host Architecture](#host-architecture)
  - [Cross-Compilation](#cross-compilation)
  - [Container Images](#container-images)
- [Generated Assets](#generated-assets)
- [nFPM Convenience Packages](#nfpm-convenience-packages)
  - [Building nFPM Packages](#building-nfpm-packages)
  - [Linting nFPM Packages](#linting-nfpm-packages)
  - [nFPM Configuration](#nfpm-configuration)
  - [nFPM Known Limitations](#nfpm-known-limitations)
- [Native Packages](#native-packages)
  - [Native Debian Package](#native-debian-package)
    - [How it works](#how-it-works)
    - [Using Debian tooling directly](#using-debian-tooling-directly)
    - [Debian files](#debian-files)
  - [Native RPM Package](#native-rpm-package)
    - [How it works](#how-it-works-1)
  - [Native Alpine APK Package](#native-alpine-apk-package)
    - [How it works](#how-it-works-2)
    - [Alpine subpackages](#alpine-subpackages)
    - [Signing keys](#signing-keys)
  - [Building All Native Packages](#building-all-native-packages)
  - [Linting Native Packages](#linting-native-packages)
- [Packaging File Locations](#packaging-file-locations)
- [Package Changelogs](#package-changelogs)
- [Signing](#signing)
  - [Release Package Signing](#release-package-signing)
  - [Native Alpine APK Signing](#native-alpine-apk-signing)
  - [Native Debian Package Signing](#native-debian-package-signing)
  - [Native RPM Package Signing](#native-rpm-package-signing)
- [Distribution packaging readiness](#distribution-packaging-readiness)
  - [Debian archive readiness](#debian-archive-readiness)
  - [Fedora archive readiness](#fedora-archive-readiness)

## Overview

Gommitlint uses two packaging approaches:

| Approach | Purpose | Builds from | Output |
|----------|---------|-------------|--------|
| **nFPM** (convenience) | Direct downloads from Codefloe releases | Pre-compiled binaries | `dist/*.{deb,rpm,apk}` |
| **Native** (source-built) | Local candidates for distribution packaging work | Source code | `dist/native/*.{deb,rpm,apk}` |

**nFPM packages** are fast to build and automated via GoReleaser. They are suitable for users downloading directly from the project's release page.

**Native packages** use distribution tooling (`dpkg-buildpackage`, `rpmbuild`,
`abuild`). The current recipes are development builds, not proof of distribution
policy compliance. See [Distribution packaging readiness](#distribution-packaging-readiness)
for remaining requirements.

## Architecture Support

### Go Binary Builds

The standard release configuration selects these OS/architecture combinations:

| OS | Architecture | Build Mode | Notes |
|----|-------------|------------|-------|
| GNU/Linux | amd64 | PIE | Primary target |
| GNU/Linux | arm64 | PIE | ARM servers, Raspberry Pi 4+ |
| GNU/Linux | riscv64 | exe | Configured non-PIE build |
| macOS | arm64 | exe | Apple Silicon; cross-compiled, untested/unsupported |
| FreeBSD | amd64, arm64 | exe | Configured non-PIE builds |
| NetBSD | amd64, arm64 | exe | Go has no riscv64 port |
| OpenBSD | amd64, arm64, riscv64 | exe | Configured non-PIE builds |

Standard GoReleaser builds use `CGO_ENABLED=0` and the `netgo`/`osusergo` tags.
This does not establish identical linking behavior on every operating system.
The release workflow's separate musl PIE build is outside that standard matrix.

PIE (Position Independent Executable) is used where supported for ASLR security hardening.

### Package Architectures

| Package Format | Architectures | Notes |
|---------------|---------------|-------|
| nFPM `.deb` | amd64, arm64, riscv64 | Configured GoReleaser Linux builds |
| nFPM `.rpm` | amd64 (x86_64), arm64 (aarch64), riscv64 | Configured GoReleaser Linux builds |
| nFPM `.apk` | amd64 (x86_64), arm64 (aarch64), riscv64 | Standard GoReleaser Linux binaries, not the separate musl PIE build |
| Native `.deb` | Container/host architecture | Current recipe has no cross-build selector; `debian/control` declares `any` |
| Native `.rpm` | Container/host architecture | Current spec requests PIE; not every Go target supports this build mode |
| Native `.apk` | x86_64, aarch64 | Architectures declared by APKBUILD; recipe uses the container's architecture |

## Building from Source

### Host Architecture

Build a binary for the current machine:

```bash
just build
```

This uses the host OS and an amd64 or arm64 architecture and writes
`bin/gommitlint-<os>-<arch>`.

### Cross-Compilation

Build for specific architectures:

```bash
just build-linux-amd64    # GNU/Linux amd64
just build-linux-arm64    # GNU/Linux arm64
just build-linux-riscv64  # GNU/Linux riscv64
just build-all            # Complete standard release matrix
```

The cross-compilation script (`scripts/build/build-cross-platform.sh`) supports
the valid release-matrix combinations directly:

```bash
# Direct usage for other targets
./scripts/build/build-cross-platform.sh freebsd amd64
./scripts/build/build-cross-platform.sh linux riscv64
```

Build flags applied to all targets:

- `-trimpath` — Remove local filesystem paths from the binary
- `-buildvcs=false` — Don't embed VCS info (injected via ldflags instead)
- `-tags "netgo,osusergo"` — Pure Go networking and user resolution (no glibc)
- `-ldflags "-w"` — Strip debug information
- Version, commit, and date injected via `-X main.version=...`

### Container Images

The release workflow defines two container image variants:

- **`gommitlint:<version>`** — Distroless Debian base (`Containerfile`), minimal attack surface
- **`gommitlint:<version>-alpine`** — Alpine base (`Containerfile.alpine`), musl-native PIE binary, for Alpine/musl environments (e.g., MegaLinter)

The configured release image flow targets amd64 and arm64 and delegates signing
to Forge CI, using ci-policy's key. No published Gommitlint release
provides the current signer's evidence. Publication remains blocked until the
pinned Forge CI revision is
independently authenticated; see [Release Verification](../modules/ROOT/pages/verify.adoc).
Local development images are unsigned and do not establish release parity.

Build the dev image locally:

```bash
just build-image-multi    # Builds amd64 + arm64 manifest
```

This recipe requires Podman, executes builds, can pull images, and replaces the
local `gommitlint:dev` manifest. It does not publish a release.

#### Proxy Configuration

Host builds and GoReleaser follow the Go toolchain's `GOPROXY` and `GOSUMDB`
environment variables:

```bash
GOPROXY=https://athens.corp.example.com just build
```

**Release workflow** (for forks on self-hosted Forgejo/Gitea):

The release workflow derives the container registry host and repository path
from Forgejo context. The native package recipes use digest-pinned Debian,
Fedora, and Alpine images and do not currently expose host proxy or mirror
variables inside those containers. Add any such interface to the justfile
explicitly rather than assuming Podman forwards it.

## Generated Assets

Several assets are generated at build time and used by both nFPM and native packaging:

| Asset | Output Location | Generator Script |
|-------|----------------|-----------------|
| Man pages | `generated/manpages/` | `scripts/docs/manpage.sh` |
| Shell completions | `generated/completions/` | `scripts/docs/completions.sh` |
| Translations | `generated/locale/` | `gommitlint translations compile` |
| Debian copyright | `debian/copyright` | `scripts/packaging/generate-debian-copyright.sh` |

`scripts/docs/completions.sh` is packaging-only.
It requires an executable supplied as `BINARY_PATH` whose basename is exactly
`gommitlint`, renders Bash, Zsh, and Fish through the repository-independent
`completion --stdout` mode under the `C` locale, and publishes nothing
unless all three renderers succeed with non-empty output.
User-level installation is owned by `gommitlint completion`, not a packaging
script.

These recipes overwrite their generated outputs and can modify tracked
`debian/copyright` and schema files. Completion and translation generation can
execute the newly built binary. Review changes afterward.

Select all assets or a narrower generation recipe:

```bash
just generate                  # All assets (man, completions, translations, packaging)
just generate-manpage          # Man pages only
just generate-completion       # Shell completions only
just generate-translations     # Locale files only
just generate-packaging        # Debian copyright and deterministic changelog.gz
```

The generated assets include:

- **Man pages**: English and Swedish, gzipped
- **Completions**: Bash, Zsh, and Fish shell completions
- **Translations**: English and Swedish `.mo` files compiled from `.po` sources via `msgfmt`

## nFPM Convenience Packages

### Building nFPM Packages

nFPM consumes built binaries, but this recipe does not merely package existing
files. It runs `release-check`, `build`, and `generate`, then starts a GoReleaser
snapshot build with `--clean`. This replaces `dist/`, including any needed
`dist/native/` outputs left there.

```bash
just package-nfpm    # Build unsigned convenience packages
```

Output appears in `dist/` alongside other GoReleaser artifacts.
Publication, announcements, validation, and signing are explicitly skipped in
the snapshot invocation. Release signatures belong to the separately gated
signer, not this local recipe. Snapshot versions are not release versions.

### Linting nFPM Packages

```bash
just package-nfpm-lint    # Run lintian (deb) and rpmlint (rpm)
```

Both tools and at least one `.deb` and `.rpm` must exist. Lintian failure stops
the recipe before rpmlint. This recipe does not validate `.apk` contents,
authenticate packages, or prove distribution acceptance.

### nFPM Configuration

nFPM is configured in the `nfpms:` section of `.goreleaser.yaml`. Key settings:

- **Changelog**: `packaging/nfpm/changelog.yaml` (nfpm/chglog YAML format)
- **Copyright**: `debian/copyright` (DEP-5 format, shared with native Debian packaging)
- **Formats**: deb, rpm, apk
- **Overrides**: Per-format configuration for lintian overrides, RPM groups, etc.

nFPM packages include:

- `gommitlint` binary
- Man pages (English and Swedish)
- Shell completions (Bash, Zsh, Fish)
- Locale translations (English, Swedish)
- License and documentation files

### nFPM Known Limitations

1. **`type: doc` bug** — Files marked with `type: doc` are excluded from deb packages instead of being treated as normal files. Workaround: don't use `type: doc`.

2. **No `%lang()` support** — Locale files cannot be marked with RPM's `%lang()` directive. rpmlint will warn about `file-not-in-%lang`.

## Native Packages

Native recipes build from source using distribution tooling. They run
network-enabled containers, not the repository's zero-network test sandbox.
Package-manager commands install tools inside those containers, and build
programs can write through the mounted paths.

The tarball comes from `git archive HEAD`, excluding uncommitted source changes.
The justfile, mounted helper scripts, Go version read from `go.mod`, and copied
RPM spec or APKBUILD come from the working tree. Use a clean reviewed checkout
to avoid unintentionally mixing revisions.

The recipe's package-version variable uses the nearest tag or a local mise pin.
Debian and RPM metadata also contain version declarations. Inspect the actual
package metadata; filenames and a successful build do not prove a release identity.
Outputs accumulate in `dist/native/`, overwriting matching filenames but retaining
other versions. The aggregate recipe is not transactional.

### Native Debian Package

**Host requirements:** Podman, Git, and `just`.

```bash
just package-native-debian
```

The recipe archives the current `HEAD`, starts its digest-pinned Debian
container, and runs `scripts/packaging/debian-build.sh` there. The container
installs Debian packaging tools and the Go version from `go.mod`; those tools do
not need to be installed on the host.

Output includes `.deb`, `.changes`, and `.buildinfo` files under `dist/native/`.

#### How it works

Inside the container, `debian/rules` orchestrates the build:

1. **Build**: Compiles `gommitlint` with PIE, `CGO_ENABLED=0`, and version injection
2. **Generate**: Runs `scripts/docs/manpage.sh`, `scripts/docs/completions.sh`, and `gommitlint translations compile`
3. **Test**: Runs `go test -v ./...`
4. **Install**: Places binaries, man pages, completions, translations, and docs into the package tree

#### Using Debian tooling directly

The `debian/` directory remains at the repository root for standard Debian
tools. Maintainers with a complete host toolchain can bypass the supported
container recipe and invoke those tools directly:

Direct builds execute packaging code on the host and write into the checkout
and its parent directory. `gbp buildpackage` also depends on the branches and
pristine-tar state declared in `debian/gbp.conf`. `uscan --no-download` still
contacts upstream, while `dch` changes `debian/changelog`.
Select only the needed operation and replace example versions/filenames first.

```bash
# Build binary package (unsigned)
dpkg-buildpackage -us -uc -b

# Build with git-buildpackage
gbp buildpackage

# Check package quality
lintian -v -i -I --pedantic "../gommitlint_<version>_<arch>.deb"

# Check for new upstream releases
uscan --no-download --verbose

# Update changelog
dch -v 1.0.0-1 "New upstream release 1.0.0"
```

#### Debian files

| File | Purpose |
|------|---------|
| `debian/changelog` | Package changelog (Debian format, maintained with `dch`) |
| `debian/control` | Package metadata, build and runtime dependencies |
| `debian/copyright` | DEP-5 machine-readable copyright |
| `debian/rules` | Build rules (dh-based with Go overrides) |
| `debian/source/format` | Source format (`3.0 (quilt)`) |
| `debian/watch` | uscan configuration for detecting new upstream releases on Codefloe |
| `debian/upstream/metadata` | DEP-12 upstream metadata |
| `debian/gbp.conf` | git-buildpackage configuration |
| `debian/gommitlint.lintian-overrides` | Lintian warning overrides for Go binaries |

### Native RPM Package

The current spec's limitations are recorded under [Fedora archive readiness](#fedora-archive-readiness).
The [distribution-readiness section](#distribution-packaging-readiness) links to the official-repository submission procedures.

**Host requirements:** Podman, Git, and `just`.

```bash
just package-native-rpm
```

The recipe archives the current `HEAD`, copies
`packaging/native/rpm/gommitlint.spec`, and runs
`scripts/packaging/rpm-build.sh` in a digest-pinned Fedora container. The
container installs RPM tooling and the Go version from `go.mod`.

Binary and source RPMs are copied into `dist/native/`; names include the package
version, release, and architecture. Inspect the produced inventory rather than
assuming one RPM file.

#### How it works

The spec file (`packaging/native/rpm/gommitlint.spec`) provides these local build stages:

1. **Prep**: Extracts source tarball
2. **Build**: Compiles binaries with PIE, generates man pages, completions, and translations
3. **Check**: Runs `go test -v ./...`
4. **Install**: Installs all files to `%{buildroot}` using RPM macros (`%{_bindir}`, `%{_mandir}`, etc.)
5. **Locale**: `%find_lang` auto-discovers translation files

The `%files` section uses `%license`, `%doc`, and `%find_lang` for proper RPM metadata.

### Native Alpine APK Package

**Host requirements:** Podman, Git, and `just`.

```bash
just package-native-alpine
```

This builds the APK in a digest-pinned Alpine container, avoiding an Alpine
toolchain on the host.

Output: `dist/native/gommitlint-<version>-r<pkgrel>.apk` plus subpackages.

#### How it works

1. The justfile archives `HEAD` and copies the working-tree APKBUILD into a temporary directory.
2. Podman mounts that directory, helper scripts, and the persistent `.abuild-keys/` directory.
3. The container installs build dependencies and downloads the Go version selected from `go.mod`.
4. The script creates a non-root `builder` user and generates a development key if none exists.
5. The builder runs `abuild checksum` and `abuild -r`.
6. The recipe copies generated APKs back to `dist/native/` if the container build succeeds.

`abuild checksum` records the staged source bytes; it does not independently
authenticate their publisher. The Go installer likewise compares a tarball with
a checksum downloaded from the same configured server. That is not an
independent publisher-trust check.

#### Alpine subpackages

Alpine convention splits packages into subpackages:

| Subpackage | Contents |
|------------|----------|
| `gommitlint` | Binary (`gommitlint`) |
| `gommitlint-doc` | Man pages, README, CHANGELOG, SECURITY |
| `gommitlint-bash-completion` | Bash completions |
| `gommitlint-zsh-completion` | Zsh completions |
| `gommitlint-fish-completion` | Fish completions |
| `gommitlint-lang` | Locale translations |

#### Signing keys

The local Alpine recipe signs with an `abuild` development key generated under
`.abuild-keys/` (gitignored). This key is unrelated to the production release
signing identity.

The container has write access to this key directory and changes its ownership
for the builder user. Gitignore does not protect key confidentiality or make
the directory disposable. Keep production keys out of it and restrict access
to retained development keys. Cleanup is best effort and can leave temporary
paths after a failure.

### Building All Native Packages

```bash
just package-native-all    # Debian, RPM, and Alpine
```

The local Debian and RPM outputs have no signing step. Alpine's `abuild` signs
its APK with the local development key described above.

### Linting Native Packages

```bash
just package-native-lint    # Run lintian on .deb, rpmlint on .rpm
```

This requires both package types and both lint tools. It then runs
`apkbuild-lint` through `tools/go.mod` against the working-tree APKBUILD, not the
APK payload. Any earlier failure stops later checks.

## Packaging File Locations

```text
debian/                           # Debian packaging (at repo root, required by Debian tools)
├── changelog                     # Debian package changelog
├── control                       # Build-Depends, Depends, description
├── copyright                     # DEP-5 copyright (shared with nFPM)
├── gbp.conf                      # git-buildpackage config
├── gommitlint.lintian-overrides  # Lintian warning overrides
├── rules                         # Build rules (dh + Go overrides)
├── source/format                 # 3.0 (quilt)
├── upstream/metadata             # DEP-12 upstream metadata
└── watch                         # uscan watch file (Codefloe)

packaging/
├── nfpm/
│   └── changelog.yaml            # nFPM package changelog (YAML format)
└── native/
    ├── rpm/
    │   └── gommitlint.spec       # RPM spec file
    └── alpine/
        └── APKBUILD              # Alpine build recipe
```

The `debian/` directory belongs at the source root for Debian source-package
tools. Lintian can inspect an explicitly named package elsewhere. This
repository keeps RPM and Alpine definitions under `packaging/native/`.

## Package Changelogs

There are two separate package changelogs for different purposes:

| File | Format | Used by |
|------|--------|---------|
| `debian/changelog` | Debian changelog format | `dpkg-buildpackage`, `gbp`, `dch` |
| `packaging/nfpm/changelog.yaml` | nfpm/chglog YAML | GoReleaser nFPM (convenience deb/rpm) |

Both describe **packaging changes** (e.g., "New upstream release 0.9.0", "Add Swedish man page translation"), not software changes. For software changes, users should refer to `CHANGELOG.md`.

**Updating the Debian changelog:**

```bash
dch -v 1.0.0-1 "New upstream release 1.0.0"
```

**Updating the nFPM changelog:**

Edit `packaging/nfpm/changelog.yaml` following the existing YAML structure.

## Signing

### Release Package Signing

There is no local nFPM signing recipe and `.goreleaser.yaml` intentionally has
no signing configuration. The configured release-request workflow would build
unsigned artifacts and transfer them to the isolated Forge CI signer after
the trust gate succeeds. That configured flow is not current published evidence.
Do not enable `FORGE_CI_AUTHENTICATED_COMMIT` merely to make packaging succeed.

### Native Alpine APK Signing

The local Alpine build signs automatically with the development key under
`.abuild-keys/`. The public key is installed into the temporary build
container's `/etc/apk/keys/`; this does not establish production trust.

### Native Debian Package Signing

For unsigned builds (local development):

```bash
dpkg-buildpackage -us -uc -b
```

For an independently approved Debian signing workflow, first confirm the
package metadata and signing identity. The command below attempts to sign
build metadata such as `.changes` and `.buildinfo`; it does not embed a
signature in each `.deb` or upload packages:

```bash
dpkg-buildpackage -b    # Requires the configured signing identity
```

### Native RPM Package Signing

RPM signing is configured separately via `~/.rpmmacros`. The current build recipe produces unsigned packages.

## Distribution packaging readiness

The native recipes produce local source-built packages, not archive-acceptance
evidence. Debian `sbuild` and Fedora `mock`/Koji builds run without network
access, so all Go dependencies and sandbox-safe tests must be declared and
available before submission.

The current Debian and RPM recipes still invoke ordinary Go module builds and
are not ready for those builders. The complete submission procedures,
maintainer setup, command examples, and policy references live in the general
development notes:

- [Debian packaging for Go applications](https://codefloe.com/itiquette/docs/src/branch/main/development/notes/debian-go-packaging.md)
- [Fedora packaging for Go applications](https://codefloe.com/itiquette/docs/src/branch/main/development/notes/fedora-go-packaging.md)

Keep release-specific dependency inventories and submission records with the
Gommitlint packaging work, rather than turning the general notes into project
status ledgers.

Refresh dependency inventories, versions, license data, and distribution policy
against the exact proposed release. Local container success does not replace an
offline build in the distribution's clean build environment.

### Debian archive readiness

Gommitlint's current Debian packaging has these gaps:

- The compiler dependency in `debian/control` permits versions older than `go.mod` requires.
- `debian/control` lacks the archive-packaged Go dependency list needed for a non-vendored build.
- `debian/rules` invokes `go build` and unfiltered `go test -v ./...` directly and generates documentation, completions, and translations.
- Completion and translation generation executes the newly built product.
  Inspect those commands and their tools before defining the archive build; disabling tests alone does not make the recipe product-free.

The selected dependency strategy must be reflected in both `debian/control` and `debian/rules`.
The release-specific inventory must cover intended architectures, build tags, generators, and selected tests; `debian/changelog`, `Standards-Version`, copyright data, and upstream metadata need a final refresh.
Authenticate the selected upstream source using [release verification](../modules/ROOT/pages/verify.adoc), including its current evidence limits.
The upstream verification key and the maintainer upload key have different trust roles.

Keep source format `3.0 (quilt)` so Debian revisions and patches remain separate from upstream source, and keep `debian/` at the repository root.
`debian/changelog` records package history; software changes belong in `CHANGELOG.md`.
The archive build must be offline and reproducible from declared source and build dependencies.

### Fedora archive readiness

Gommitlint's hand-written `packaging/native/rpm/gommitlint.spec` is a local source-build recipe with these gaps:

- It has no generated bundled-module metadata or complete vendored-license inventory.
- It permits a compiler older than `go.mod` requires and contains release-specific version data.
- Its `go build` does not enforce network denial, so missing dependencies can trigger module downloads.
- Its `%check` runs unfiltered `go test -v ./...`.
- Its build executes the newly built product for completions and translations; audit those generators separately from test selection.

A generated vendor-profile spec must be reconciled with this spec's install layout: binary, man pages, completions, translations, and `%find_lang` output.
The package name remains `gommitlint`; `golang-*` names are for Go library source packages.
Vendored modules need generated `bundled(golang(<import-path>))` provides from the selected vendor tree.
Install the vendored modules' license files and include their licenses in the package's cumulative SPDX expression.
Package build/test phases must remain offline, using source-package inputs or declared Fedora build dependencies.
Convenience nFPM packages remain distinct from the source RPM reviewed by Fedora.
A successful build or clean lint result is not evidence of acceptance, sponsorship, or availability in Fedora.

## References

Consult distribution policy over third-party tutorials; the Debian and Fedora notes linked under [distribution readiness](#distribution-packaging-readiness) maintain the full upstream packaging bibliographies.

- [Maintainer release guide](releasing.md) — Gommitlint's signing and publication procedure.
- [Release verification](../modules/ROOT/pages/verify.adoc) — artifact authentication and current signer limitations.
