# Development Guide

This is the canonical setup and daily-workflow guide for people changing
Gommitlint source, tests, or repository documentation.
Run all commands from the repository root unless a command says otherwise.
Use a dedicated Bash session for the sequences below and stop when a prerequisite fails.
Bootstrap, generated-file updates, product execution, and release operations are separate activities, not implicit documentation-review steps.

## Scope and Ownership

Gommitlint documentation has two audiences and therefore two formats.
The published Antora site under `docs/modules/ROOT/` is for people using
Gommitlint.
The repository documents (`README.adoc`, `CONTRIBUTING.md`, and the Markdown
files under `docs/contributing/`) are for people working on it; they are read in
the forge or an editor and are not published to the site.

This guide owns contributor setup, source layout, local checks, focused tests,
and generated-file workflows.
Other concerns have separate owners:

| Concern | Authoritative document |
| --- | --- |
| Contribution policy, issues, pull requests, DCO, and commit signing | [CONTRIBUTING.md](../../CONTRIBUTING.md) |
| Product use and installation | [README.adoc](../../README.adoc) and the [published Antora pages](../modules/ROOT/pages/index.adoc) |
| Architecture and dependency rules | [Architecture](../modules/ROOT/pages/architecture.adoc) |
| Reproducible-build guarantees and caveats | [Reproducible Builds](../modules/ROOT/pages/reproducible-builds.adoc) |
| Packaging and distribution formats | [Packaging Guide](packaging.md) |
| External-binary test contracts | [CLI Black-Box Testing Requirements](../testing/cli-black-box.adoc) |
| Release verification by users | [Verify Releases](../modules/ROOT/pages/verify.adoc) |
| Maintainer release operations | [Maintainer Release Guide](releasing.md) |

`AGENTS.md` contains automation-specific permissions and prohibitions.
It is not the human contributor policy and is not a substitute for this guide
or `CONTRIBUTING.md`.

## Toolchain and Trust

The recommended setup needs Git, Bash, `mise`, and `just`.
The repository supports host development on the architectures accepted by
`just build` (`amd64` and `arm64`).
Container, external-test, and native-package tasks have additional requirements
documented with those tasks.

On macOS, use Bash 5+ from MacPorts (`sudo port install bash`) and mise for the
repository-pinned tools. Put `/opt/local/bin` before `/bin` on `PATH`, start
`/opt/local/bin/bash`, and confirm `bash --version` reports 5 or newer.
Installing it does not replace Apple's `/bin/bash`. Use MacPorts for gettext
and the GNU utilities required by the recipes you select.
Nanolinter's published archives are Linux-only, so mise skips that tool on
macOS. Install a reviewed
[native Nanolinter source build](https://docs.itiquette.org/nanolinter/install.html#macos-source-build)
on `PATH` before `just doctor` or the lint gate. No verification step is skipped.
Native fixtures use physical temporary roots; custom crash-report state roots
must still be absolute and symlink-free.

Do not copy version numbers into documentation as another source of truth.
Use these files:

| File | Authority |
| --- | --- |
| `go.mod` | Main module and Go language/toolchain baseline used by CI |
| `.mise.toml` | Exact development Go version, external tool declarations, and install policy |
| `mise.lock` | Generated locked URLs and checksums used by locked mise installs |
| `tools/go.mod` | Go command tools and their module versions, with checksums in `tools/go.sum` |
| `justfile` | Supported task behavior and command composition |

The Go versions in `go.mod`, `.mise.toml`, and `tools/go.mod` are intentionally
kept aligned.
Use `just upgrade-go` for a coordinated Go version change.
That is a networked upgrade procedure, not a repair for an unavailable pinned toolchain during review.

Review `.mise.toml`, `mise.lock`, `tools/go.mod`, `tools/go.sum`, `justfile`, and `scripts/dev/install-go-dev-tools.sh` before bootstrap, especially
when they changed in the branch you checked out.
Trusting the checked-out configuration is a separate decision you make with `mise trust`; `just install-dev-tools` does not trust it, and mise refuses an untrusted configuration.
`just install-dev-tools` is a networked, user-environment-mutating operation that:

1. Installs every declared mise tool with `mise install --locked`.
2. Installs each Go tool declared by `tools/go.mod`.

Go-tool installation follows the tools module, not `mise.lock`, and writes to Go's selected installation directory.
The installer searches `$HOME/go/bin/go`, then `/usr/local/go/bin/go`, then PATH, and enforces a minimum version rather than exact equality.
Confirm which Go executable it selects. Activating mise alone does not override those earlier search paths.

Installation is sequential, not transactional. A later failure does not undo tools already installed.
The installer's recommended-external-tool check can warn without failing, and `just doctor` runs Nanolinter's doctor, not Gommitlint's command.

The lock file constrains resolved artifacts and checksums.
The mise configuration also applies a seven-day minimum release age where the
selected backend supports release-age metadata.
The configured exclusions exempt Gommitlint and Nanolinter's own releases from that age delay.
Neither mechanism makes an unreviewed repository configuration safe to trust.
If you do not trust the checkout, do not run the bootstrap.

## Quick Start

Clone the repository and enter its root:

```bash
set -euo pipefail
git clone https://codefloe.com/itiquette/gommitlint.git
cd gommitlint
```

Read the complete bootstrap inputs, including unchanged files. Use both diffs to inspect local changes:

```bash
git diff -- .mise.toml mise.lock tools/go.mod tools/go.sum justfile scripts/dev/install-go-dev-tools.sh
git diff --cached -- .mise.toml mise.lock tools/go.mod tools/go.sum justfile scripts/dev/install-go-dev-tools.sh
```

An empty diff does not authenticate the checkout or approve its bootstrap.
Only after review and approval, trust the configuration, install the tools and inspect their health:

```bash
set -euo pipefail
mise trust
just install-dev-tools
just doctor
```

After setup, verify and build the host binary:

```bash
set -euo pipefail
just verify
just build
```

If product execution is separately approved, inspect the exact host artifact:

```bash
set -euo pipefail
host_os=$(go env GOHOSTOS)
host_arch=$(just _host-goarch)
"./bin/gommitlint-${host_os}-${host_arch}" --version
```

`just verify` is intended to avoid source fixes, but it is not a read-only filesystem or network sandbox.
Its lint plan runs external tools, and its Go checks can compile code and populate caches.
The recipe does not force offline mode. Review the effective lint configuration and environment before selecting it as a gate.
The `links` check is advisory in `just lint` and in the pull-request workflow: it fetches URLs from the internet, so `nanolinter.toml` lists it under `[policy] warn`, and it reports broken links without failing the run.
`just lint-links` is the blocking run, and the link gate before a release.
`just build` writes the host binary under `bin/`.

## Source Map

| Path | Responsibility |
| --- | --- |
| `cmd/gommitlint/` | Small composition root and executable metadata |
| `internal/app/validation/` | Validation use-case coordination |
| `internal/app/hooks/` | Git hook lifecycle coordination |
| `internal/domain/` | I/O-free core types, validation, rules, diagnostics, catalogs, and settings |
| `internal/adapters/` | CLI, Git, config, filesystem, output, signing, i18n, and other external boundaries |
| `internal/testhelpers/` | Shared in-process test support and assertions |
| `internal/tools/schemagen/` | Config JSON Schema generator |
| `test/policy/` | Untagged repository workflow, launcher, and distribution-policy tests |
| `test/integration/` | Build-tagged in-process integration scenarios, helpers, and fixtures |
| `test/blackbox/` | Build-tagged shipped-CLI harness |
| `scripts/` | Build, docs, install, packaging, lint, and guarded test entry points |
| `docs/modules/ROOT/` | Published Antora component pages, assets, and navigation |
| `docs/contributing/` | Human repository-development and packaging guides |
| `web/` | Hugo landing-page sources; see [Landing-page maintenance](#landing-page-maintenance) |
| `generated/` | Regenerated packaging, manpage, completion, and locale output |

Dependencies point inward: adapters may use application and domain packages,
application packages may use domain packages, and domain packages use neither
application nor adapters.
Define small interfaces where they are consumed and pass dependencies
explicitly.
See the [Architecture](../modules/ROOT/pages/architecture.adoc) page before
changing package boundaries.

Keep package-level tests beside the code they verify.
Repository-wide contracts and cross-package scenarios belong under `test/`, organized by responsibility and execution tier rather than hexagonal layers.
The directory name does not select a tier: policy tests run with the ordinary tests, while integration and black-box suites retain their build tags and guarded launchers.
Shared `internal/testhelpers/` stays accessible to both colocated tests and these suites; moving it to `test/internal/` would prevent colocated tests from importing it.

For package API documentation, use `go doc` or, after separate server approval, run `pkgsite -http=127.0.0.1:6060` with the installed tool.
The loopback address avoids publishing local package documentation on every network interface.
This starts a server and can use dependency caches or network access. It is not a static documentation check.

## Daily Workflow

Verification and build jobs print a startup summary with the host platform and
selected checks or build targets. `verify-fix` announces that it applies fixes;
image preparation names the image, output location and network use. Binary and
image targets are reported separately from the machine running the build.
`engine-default` means the recipe leaves image architecture selection to the
configured container engine.

Use the smallest focused test while editing, then run the repository gate.

```bash
set -euo pipefail
just test-pkg-native ./internal/domain
just test-run-native '^TestValidateCommit_'
just verify
```

The development commands differ deliberately:

| Command | Source-tree behavior | Purpose |
| --- | --- | --- |
| `just lint-fix` | Mutating | Applies supported formatters and fixes; Go fixes also run `go mod tidy` |
| `just verify-fix` | Mutating | Runs `lint-fix`, then `verify` with lint in allow-dirty mode |
| `just verify` | Non-fixing intent, with build/cache writes | Runs lint, tagged compile/vet checks, and in-process tests with committed fuzz seeds |
| `just verify-full` | Non-fixing intent, with build and temporary output | Runs the non-publishing local pre-release gate, including product-executing translation drift checks |

Always inspect `git diff` after `just lint-fix` or `just verify-fix`.
Do not use either mutating command merely to answer whether a tree is green.

`just verify-full` adds Alpine pin coupling, schema and translation drift
checks, localized-manpage checks, the complete binary matrix, repeat-build
verification, and GoReleaser configuration validation.
It does not publish a release and does not claim L3 isolation evidence.
The translation drift script runs `go run ./cmd/gommitlint translations update` against a temporary locale copy.
That check executes the product even though it is intended to preserve the source catalogs.
Repeat-build success compares local outputs, not an independently authenticated published release or universal cross-environment reproducibility.

The shell validator was deleted after validation moved into the shared engine's
pinned `check-l3-isolation` action and engine. There is no supported
local replacement. The pull-request action is the required isolation gate;
directly invoking `just verify-l3-isolation` still fails closed so no caller can
mistake missing local evidence for a pass.

Common build commands are:

```bash
just build              # Current host OS and architecture
just build-all          # Complete release matrix
```

Use the [Packaging Guide](packaging.md) for containers and package builds.

## Testing in different environments

Use a checkout containing the changes you want to test; a fresh clone only
contains committed changes. Run these commands from its root as your normal user.
The sequence is local tests, Linux containers, then any native VM targets you
want to cover. `just verify` adds local quality checks; it does not select the
container or VM suites.

### Linux: local tests and containers

Complete the [tool setup](#quick-start) first. Container tests also require
rootless Podman with cgroup v2. Then open a project-tool shell:

```bash
mise exec -- bash
```

This opens a child Bash shell on the same Linux host with the project's tools
available on `PATH`. It is not a container or VM. Run the following block in that
shell; `exit` returns to your previous shell. Environment changes are scoped to
the child shell, while tools installed during setup remain installed.

For a first run on an x86-64 Linux host:

```bash
(
  set -euo pipefail
  just test

  # Prepare the sandbox and distribution images; these steps use the network.
  just test-container-prepare
  just test-container-prepare-distributions

  just test-unit-container
  just test-combined-container
  just test-combined-container-matrix
)
```

`combined` runs integration and black-box tests together. The matrix selects all
configured distributions when no names are given. On ARM64 Linux, select
ARM64-capable distributions explicitly, for example `debian-13`, in both the
distribution preparation and matrix commands.

### macOS: native tests and Linux containers through VMs

This route needs an Apple Silicon Mac running macOS 26 or later, plus the
[controller prerequisites](../testing/test-environments.adoc). Run these commands
in Bash from the checkout root on the Mac. Preparation installs the compilers
and test dependencies inside the guests; the Mac needs the controller tools.

```bash
(
  set -euo pipefail
  just test-vm-prepare macos-arm64
  just test-unit-vm macos-arm64
  just test-race-vm macos-arm64
  just test-combined-vm macos-arm64

  just test-vm-prepare linux-arm64
  just test-unit-container-on-macos
  just test-combined-container-on-macos
  just test-matrix-container-on-macos
)
```

The macOS guest provides native macOS coverage. The Linux guest provides Linux
container coverage. Its checked-in profile currently prepares Debian only, so
its matrix result covers that selection, not all six distributions.

### Add native BSD coverage

With QEMU and the other controller prerequisites installed, this block covers
the three BSD targets from an x86-64 Linux controller:

```bash
(
  set -euo pipefail
  for target in freebsd-amd64 openbsd-amd64 netbsd-amd64; do
    just test-vm-prepare "$target"
    just test-unit-vm "$target"
    just test-combined-vm "$target"
  done
)
```

On Apple Silicon or ARM64 Linux, use `freebsd-arm64 openbsd-arm64 netbsd-arm64`
in that loop. The controller and guest architectures must match.

### Setup, results, and reruns

Host tool installation is the explicit setup step above. VM preparation
configures the guest and writes images, keys and logs under ignored `.test-vms/`;
it does not install host packages or change persistent host system settings.
Container preparation also writes image recordings and container storage.

The blocks stop at the first failure. Subsequent test runs use the recorded
container images; prepare them again when their inputs change. VM preparation
reuses a current ready image; follow its cleanup/rebuild instruction when inputs
have changed. Check the printed log path, and record each selected environment
as passed, failed or blocked. Real guest execution is required to establish a
platform pass.

For argument help, use `just --usage test-vm-prepare` or `just --usage` with any
other recipe name. The [test environments guide](../testing/test-environments.adoc)
owns image profiles, prerequisites, cache behavior, isolation and troubleshooting.
Fuzz campaigns, coverage reports and release/package checks remain separate
opt-in recipes.

## Test Tiers

The [test environments guide](../testing/test-environments.adoc) covers the
explicit QEMU/Tart recipes, prepared image profiles and native platform scope.

Use `just test-pkg-native <package>` and `just test-run-native <filter>` for focused work.
Both retain the repository's race-detector and no-VCS-build settings.
The name filter can match no tests and still return success. Confirm the selected names and executed test count.
These recipes do not independently force an offline toolchain or dependency policy.
Use `just test` before submitting; it runs all untagged packages and committed
fuzz seeds under the race detector.
Use `just test-coverage-native` to write the HTML coverage report under `bin/`.

`just test-unit-native`, `just test-unit-container` and `just test-integration-container` pass any extra arguments through
to `go test`, so `just test-unit-native -v` and `just test-integration-container -run TestX`
need no recipe of their own. Arguments travel as positional parameters the
whole way, including into the sandbox container, so a value containing spaces,
quotes or shell metacharacters reaches `go test` as one literal argument. The
other sandbox tiers run fixed commands and refuse extra arguments rather than
ignore them.

| Tier | Supported entry point | Where it runs | Detailed contract |
| --- | --- | --- | --- |
| Unit and fuzz seeds | `just test` or `just test-fuzz-native` | Current environment; pull requests and local development | This guide |
| Container unit and fuzz seeds | `just test-unit-container` | Explicit local Podman run; no product executable | This guide |
| Generated fuzz inputs | `just test-fuzz-container <duration>` | Explicit local container; scheduled/manual CI uses its dedicated launcher | [Black-box test document](../testing/cli-black-box.adoc) |
| External integration | `just test-integration-container` | Explicit local Podman run | [Black-box test document](../testing/cli-black-box.adoc) |
| Shipped CLI black box | `just test-blackbox-container` | Explicit local Podman run | [Black-box test document](../testing/cli-black-box.adoc) |
| Distribution matrix | `just test-combined-container-matrix [name...]` | Explicit local Podman runs | [Black-box test document](../testing/cli-black-box.adoc) |

The sandbox tiers load an image rather than build one. Run
`just test-container-prepare` once, and again whenever `Containerfile.sandbox`
changes or `just clean` has removed `bin/`; it is the only step that
reaches a registry. A tier whose recorded image is missing, corrupt, or built
from a different Containerfile refuses and names that recipe, instead of
rebuilding the image or running against a stale one.

`just test-combined-container` combines the integration and black-box source
tiers in one sandbox.
`just test-blackbox-container` runs the whole black-box contract, including the
platform- and tool-dependent cases; the sandbox image provides their tools, so
a missing one fails the tier instead of skipping a row.
The distribution matrix reruns the integration and black-box suites once per
distribution listed in `test/distribution/images` (Ubuntu 26.04, Debian 13,
Fedora 44, openSUSE Leap 16 and Tumbleweed, Arch), each image carrying that
distribution's own packaged git, GnuPG and OpenSSH, so a change in how one of
them behaves shows up against the release that ships it.
Record the images first with `just test-container-prepare-distributions` (all of them, or
the names given); like `just test-container-prepare` it is the only step that
reaches a registry, and it needs rerunning when anything under
`test/distribution/` changes.
`just test-combined-container-matrix` then runs every distribution and
`just test-combined-container-matrix fedora-44 arch` the named ones; a failing distribution does
not stop the rest, and the summary lists each result.
`just test-container-clean-distributions` removes the recordings under
`bin/distribution-images`. The matrix is local-only and not part of
`just verify`.

The recipe chooses the tier and execution environment independently. `test`,
`verify` and `verify-fix` do not launch containers or VMs. `test-environment`
reports the current OS and architecture; container recipes explicitly validate
a Linux controller before reaching the existing guarded launcher. Detection
never switches a native run to containers, emulation or a VM. Container unit
tests retain `-race` and run no integration/black-box tags or shipped CLI.

Do not replace supported recipes with hand-written launcher invocations; the
detailed document owns isolation, fixture, artifact, and parity requirements.

In-process tests use `internal/testhelpers/check` for assertions.
Keep fixtures in adjacent `testdata/` directories and keep tests isolated with
`t.TempDir()` and `t.Setenv()`.
Do not add `t.Parallel()` to tests that share urfave/cli process state.

## Pull-request job image

The pull-request jobs run in the image named by the repository variable
`CI_JOB_IMAGE`. `Containerfile.ci` is its recipe: a Nanolinter Go image plus
the four Debian packages the gates need. No workflow builds it. An operator
builds it from a reviewed commit, with an explicit digest-pinned base:

```bash
podman build -f Containerfile.ci \
  --build-arg NANOLINTER_GO_IMAGE=<registry>/nanolinter@sha256:<digest> \
  -t localhost/gommitlint-ci:candidate .
```

Publishing the image and changing `CI_JOB_IMAGE` to its registry digest are
separate operator steps; see the [Maintainer Release Guide](releasing.md).
The package versions are pinned. Debian retires old versions at point
releases, so a rebuild can fail until the pins in `Containerfile.ci` are bumped.

## Documentation and Generation

Documentation ownership follows its audience:

- Root `README.adoc` is the repository front door: a short product overview
  and quick start that points users to the site and contributors to these guides.
- `docs/modules/ROOT/pages/` contains user-facing Antora pages.
- `docs/contributing/` contains repository-development documents.
- `docs/testing/` contains detailed internal test contracts.
- `docs/manpages/` contains hand-edited Markdown manpage sources.

For a new published page, add the AsciiDoc file under
`docs/modules/ROOT/pages/` and add its `xref:` to
`docs/modules/ROOT/nav.adoc`.
`docs/antora.yml` is the unversioned component descriptor (`version: ~`).
The shared documentation hub owns site assembly and publication; this
repository has no local Antora publication recipe.
Only `docs/modules/ROOT/` participates in the published component.
The Markdown manpage sources and the `docs/contributing/`, `docs/testing/`, and
`docs/internal/` trees remain repository documentation outside Antora
navigation.

Never hand-edit files under `generated/`.
Regenerate the complete set with:

```bash
just generate
```

`just generate` creates manpages, shell completions, compiled translations, the
config schema, and packaging support files.
Completion generation first builds the host binary, then executes its completion commands.
Translation generation and updates execute the product through `go run`.
Schema generation runs a separate Go generator. Packaging generation can replace `debian/copyright` outside `generated/`.
These steps can partially complete before a later failure. Inspect all affected paths rather than assuming generation is one transaction.
Use narrower commands when only one source changed:

| Source | Generated output | Commands |
| --- | --- | --- |
| `docs/manpages/gommitlint.1.md` and localized sources | `generated/manpages/` | `just generate-manpage`; `just check-localized-manpages` |
| Live CLI completion definitions | `generated/completions/` | `just generate-completion` |
| `internal/domain/settings/` types and doc comments | `schemas/gommitlint.schema.json` | `just generate-schema`; `just check-schema` |
| `locale/<lang>/gommitlint.po` | `generated/locale/` | `just generate-translations`; `just check-gettext-catalogs` |
| `CHANGELOG.md` and licensing inputs | `generated/changelog.gz`, `debian/copyright` | `just generate-packaging` |

### Config JSON Schema

`schemas/gommitlint.schema.json` is generated from `internal/domain/settings` by `internal/tools/schemagen` and committed outside `generated/`; do not hand-edit it.
Each binary embeds the schema bytes available when it was built.
Regeneration does not change an already built or installed binary, so an editor using another revision can disagree with the selected binary's `config schema` output.
Field doc comments become schema `description` strings; write them for readers of editor tooltips.

After changing settings types or their doc comments, regenerate the schema:

```bash
just generate-schema
```

Review the generated diff, then validate source/schema consistency:

```bash
just check-schema
```

`check-schema` runs the generator into a temporary file and compares it with the checked-in schema.
It does not rebuild installed binaries or validate an external editor's schema selection.

#### Publishing to SchemaStore

SchemaStore can provide schema associations to supporting editor integrations.
Availability depends on catalog acceptance, client support, configuration, and caching; the example below is not evidence that an association is published.
For a separately approved submission, propose or update this entry in `src/api/json/catalog.json` at [SchemaStore/schemastore](https://github.com/SchemaStore/schemastore):

```json
{
  "name": ".gommitlint.yaml",
  "description": "Gommitlint configuration",
  "fileMatch": [
    ".gommitlint.yaml",
    ".gommitlint.yml",
    ".gommitlint.toml"
  ],
  "url": "https://codefloe.com/itiquette/gommitlint/raw/branch/main/schemas/gommitlint.schema.json"
}
```

The URL selects mutable `main`, not necessarily the latest stable release or the schema embedded in an installed binary.
Consumers can retain cached bytes; inspect the selected revision when investigating editor/runtime differences.
Update the catalog entry when its fields change, not merely when the referenced schema content changes.

### Translations

Gommitlint uses gettext-style source catalogs under `locale/<lang>/gommitlint.po`.
Compilation writes `.mo` files under `generated/locale/<lang>/LC_MESSAGES/`; installing them into a user or system locale directory is separate.
This follows the common PO workflow used by projects such as systemd and Vim.

To add a language:

1. Copy `locale/en/gommitlint.po` to `locale/<lang>/gommitlint.po`, retaining the `msgid` values.
2. Set the header's `Language:` field to the locale code, such as `sv` or `pt_BR`.
3. Translate `msgstr` values while preserving placeholders such as `%s`, `%d`, and backticks.

For an existing language, edit only the target PO file and keep unrelated locale changes out of the same PR.
Translate values still matching English where appropriate; leave no empty `msgstr` except the header and avoid `#, fuzzy` entries.
User-facing CLI/output strings must use `localizer.Get` or `localizer.GetN`; `just lint` enforces this in those packages.

Use preinstalled gettext tools: `msgmerge`, `msgen`, `msgattrib`, and `msgfmt`.
Tool installation is a separate setup activity.
After changing user-facing strings and selecting the intended catalog update, run:

```bash
set -euo pipefail
just update-translations
```

Review the extracted catalog changes and complete the translations before compiling and checking them:

```bash
set -euo pipefail
just generate-translations
just check-translations
just check-gettext-catalogs
```

`just update-translations` extracts Go message IDs into `locale/gommitlint.pot`; `msgmerge` updates each locale's PO file, `msgattrib` removes obsolete entries, and `msgen` generates the English catalog.
Both update and compile recipes use `go run ./cmd/gommitlint`, so they execute the product's developer commands.
The drift check also runs `translations update` against a temporary copy of `locale/` and compares normalized catalogs rather than byte-identical files; temporary/build writes still occur.
Do not hand-edit compiled `.mo` files.
`check-gettext-catalogs` checks syntax, and `check-localized-manpages` checks source-file presence.
Neither proves translation accuracy, complete translated content, or successful rendering of every localized manual.
For a syntax check that does not create `messages.mo`, select an explicit output sink and replace `sv` with the intended locale:

```bash
msgfmt --check --output-file=/dev/null locale/sv/gommitlint.po
```

General contribution and licensing policy remains in [CONTRIBUTING.md](../../CONTRIBUTING.md).

### Localized manual pages

Localized manpage sources live at
`docs/manpages/locale/<lang>/gommitlint.1.md` and use
`docs/manpages/gommitlint.1.md` as the English source.
To add a language:

1. Create that locale directory.
2. Copy the English source into that directory.
3. Translate the copy.
4. Run `just generate-manpage`.
5. Run `just check-localized-manpages`.

After manpage or command-reference changes, run
`just test-pkg-native ./cmd/gommitlint` to exercise documentation synchronization tests.

## Landing-page maintenance

`web/` contains the single-page Hugo source for `gommitlint.itiquette.org`.
The page links to the full [Antora documentation](https://docs.itiquette.org/gommitlint/).

### Sources and local preview

| Path | Responsibility |
| --- | --- |
| `web/hugo.toml` | Site configuration and URL parameters |
| `web/content/_index.md` | Homepage front matter; the body lives in the template |
| `web/layouts/home.html`, `web/layouts/partials/{head,footer}.html` | Hand-written templates, without a theme dependency |
| `web/assets/style.css` | Hand-written, fingerprinted stylesheet |
| `web/scripts/check-contrast.mjs` | Dependency-free WCAG contrast check |
| `web/static/fonts/` | Self-hosted Atkinson Hyperlegible and its OFL notice |
| `web/static/.domains` | Codefloe Pages custom-domain configuration |

Logos are copied from `assets/` during the build, rather than committed again
under `web/static/`. On GNU/Linux amd64, use the same pinned Hugo Extended
installer as CI. From the repository root, with Bash, curl, sha256sum, tar and
install available:

```bash
(
  set -eu
  install -d "$HOME/.local/bin"
  bash .forgejo/scripts/install-hugo.sh "$HOME/.local/bin"
  install -m 0644 assets/logo.png web/static/logo.png
  install -m 0644 assets/logo-animated.svg web/static/logo-animated.svg
  install -m 0644 assets/apple-touch-icon.png web/static/apple-touch-icon.png
  install -m 0644 assets/favicon-32.png web/static/favicon-32.png
  "$HOME/.local/bin/hugo" server --source web
)
```

`.forgejo/scripts/install-hugo.sh` owns the Hugo version and GNU/Linux amd64
checksum. Hugo is not declared in `.mise.toml`; another platform needs its
matching Hugo Extended release installed separately.

### Build and deployment

`.forgejo/workflows/pages-deploy.yml` handles pushes to `main` affecting the
page inputs. It installs Hugo Extended, mirrors the logo assets, runs
`node web/scripts/check-contrast.mjs`, builds with `hugo --minify`, and checks
rendered HTML using `web/.htmlvalidate.json`. Stylesheet fingerprints put a
content hash in the filename for long-lived caching. The workflow replaces the
rendered `public/` content on the `pages` branch; Codefloe Pages uses `.domains`
to map it to `gommitlint.itiquette.org`.

The workflow uses repository variable `CI_RUNNER_LABEL` (`ci-builder` in
production). Forks must choose an available compatible runner label.

The `pa11y-ci` WCAG 2.1 AA crawl is local-only because its headless Chromium
dependencies are substantial. Before publishing significant template changes,
check a locally served, freshly built `public/` tree:

```sh
npx -y http-server public -p 3000 -s & npx -y pa11y-ci --config web/.pa11yci.json; kill %1
```

The configuration uses `--no-sandbox`; this procedure is for reviewed local
output, not third-party HTML. Contrast and HTML structure are checked by CI;
the broader accessibility crawl is a separate local check.

### Presentation and localization

The page has no external fonts, analytics, trackers or client-side JavaScript.
It uses a self-only Content-Security-Policy without inline content, semantic
`header`/`main`/`footer` landmarks, a skip link and descriptive link text.
CI checks WCAG AA contrast in light and dark schemes. `prefers-reduced-motion`
selects a static logo, `prefers-color-scheme` chooses light/dark, and
`prefers-contrast: more` replaces muted text with full-contrast text.

The site currently uses `languageCode = "en-us"`. To add a language, create
translated `web/content.<lang>/_index.md` front matter, add `[languages.<lang>]` to
`web/hugo.toml`, and wire a switcher in `web/layouts/partials/head.html`. Preserve and
translate `.Params.lead`, `.Params.status_note`, and each feature's `title` and
`body`, which the existing feature templates consume.

### Landing-page attribution

`logo.png` and `logo-animated.svg` contain an original rendering of the Go
gopher by [Renée French](https://reneefrench.blogspot.com/), used under
[CC-BY-3.0](https://creativecommons.org/licenses/by/3.0/). The derivative
illustration is © The Gommitlint Authors under the same license.

[Atkinson Hyperlegible](https://brailleinstitute.org/freefont), by the Braille
Institute of America, is self-hosted for low-vision readability under the
[SIL Open Font License 1.1](../../web/static/fonts/OFL.txt).
The site's stylesheet, templates and other content are CC0-1.0.

## Before Submission

- Keep the change focused and update tests and user documentation together.
- Regenerate only outputs owned by the source you changed.
- Run focused tests during development and `just verify` before submission.
- Run `just verify-full` for release, toolchain, build-matrix, or packaging changes.
- Check `git diff --check` and inspect every generated or autofixed change.
- Follow [CONTRIBUTING.md](../../CONTRIBUTING.md) for issue discussion, pull
  requests, Conventional Commits, DCO sign-off, and cryptographic commit signing.
- Maintainers preparing a release must also follow the [Maintainer Release Guide](releasing.md).
