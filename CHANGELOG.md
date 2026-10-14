# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- Keep `just upgrade-go`'s Mise lock and installed toolchain in step with its
  source pins. Allow bounded ShellCheck headroom observed in the real PR job,
  and recognize the verbatim OpenBSD `(I)nstall` test prompt in Typos.

### Added
- `test-vm-prepare TARGET` now uses checked-in, checksum/digest-pinned profiles to obtain and bootstrap the guest automatically. It generates test-only access, installs guest tools and the exact Go toolchain, and reuses a prepared image only while its input key matches. A custom development-image profile remains an optional second argument. Bootstrap downloads are cached separately; native guest boot results still require verification on the appropriate controller.
- Explicit native VM test recipes: QEMU for BSD, Tart for macOS, and Linux Podman suites inside a Tart Linux guest. `test-vm-prepare TARGET PROFILE.json` prepares a pinned base separately from offline test execution; runs use disposable guests, unprivileged tests, read-only native artifacts, bounded resources, and retained logs. Native PTY, process, resource and macOS fixture-CA handling use the guest platform. These opt-in recipes stay outside default verification and hosted CI; mocked controller checks do not establish native platform support.

### Changed
- **BREAKING (release operators):** stage the Forge CI migration at local commit `0c7c4354acc7b215da895d36204f9099dcff8406`. Workflows, CLI calls and policy guards use the new engine names and `FORGE_CI_AUTHENTICATED_COMMIT`. The signer is a digest-pinned local amd64 candidate; replace its localhost reference with the verified registry-served digest after publication. Hosting and independent release authentication remain pending.
- Native panic-report fixtures use physical temporary roots, preserving the no-follow policy on macOS. Local artifact comparison uses portable array readers and cleanup avoids GNU `realpath`. The published Nanolinter mise tool is Linux-only; macOS contributor setup documents Bash 5+ from MacPorts, mise-pinned tools and the native source build.
- VM commands stream guest logs and report controller phases and elapsed quiet periods while running. SSH probes disconnect unused terminal input and have forced-stop deadlines, avoiding terminal job-control stalls.
- **BREAKING (development):** Mac-hosted Linux container recipes are named `test-*-container-on-macos` and use their fixed Linux ARM64 guest without a target argument. Native BSD/macOS VM recipes still require an explicit target; omitting it prints argument help without starting a VM. `just help RECIPE` exposes argument help directly.
- VM setup reports log paths before slow work, gives field-specific profile errors, and verifies cached Tart images before reuse. ARM firmware is checked before image downloads; bootstrap CDs use explicit virtio-SCSI attachment on both architectures.
- Development recipes document arguments through `just --usage` and reject invalid fixed-choice values before running dependencies. The output-capturing helper passes commands as literal arguments instead of evaluating shell text. Gettext installation uses a fresh temporary child directory and preserves a caller-supplied `WORK_DIR`.
- **BREAKING (development):** test recipes name their execution environment: `test-unit-native`, `test-integration-container`, `test-blackbox-container`, `test-combined-container`, `test-fuzz-native`, `test-fuzz-container`, and `test-combined-container-matrix`. The combined recipes run integration and black-box tests together. Image preparation is explicit through `test-container-prepare` and `test-container-prepare-distributions`. The new `test-unit-container` runs the untagged unit/race tests without a shipped CLI. Default `test`, `verify` and `verify-fix` remain current-environment checks; the Bash preflight validates the requested mode without selecting another backend.
- The delivery layer is pinned to `1aa232423e30f4d4ac2f88def3e1549946ebd3e1`, including the shared release-note/input-contract checks and anonymous signer-image availability before release preparation. Preparation and signing receive the same digest-pinned image
- Release notes extract the signed changelog's exact version section through Reusable CI, preserving the project's title and installation/verification links. The isolation action checks input declarations from its pinned checkout instead of this repository maintaining a second API list
- Follow `itiquette/reusable-ci` to `e21a412d72523ff4744c1cde2e4641bf7643fc81`: release preparation renders project configuration in a separate job from SSH signing, and the engine includes the reviewed registry-auth and ledger fixes. Release-key and signer-publication documentation now names `itiquette/ci-policy` (formerly `release-policy` and `release-ci`)
- **BREAKING (release operators)** the signer image is named `reusable-ci-signer`: `release.yml` and the workflow policy test name `codefloe.com/itiquette/reusable-ci-signer` where they named `codefloe.com/itiquette/release-ci-signer`. The image is built from the engine alone and was not yet published under the old name. The pinned digest is unchanged and is replaced after the first publication
- the `links` check is advisory in `just lint` and in the pull-request workflow: `nanolinter.toml` lists it under `[policy] warn`, so it still runs and prints every broken link but cannot fail the run. It fetches URLs from the internet, and a lint result must depend only on the code under review. `just lint-links` is the blocking run and the link gate before a release
- the body line-length check (`GL305`, `message.body.max_line_length`) no longer counts an identity trailer: `Signed-off-by`, `Co-authored-by`, `Reviewed-by`, `Acked-by`, `Tested-by` or `Reported-by`, followed by a name and one address in angle brackets and nothing else. Git writes that line from the author's name and address, so an author with a long name could neither wrap nor shorten it. Text after the address, another trailer key, or an indented line is still ordinary body text and still checked; whether a sign-off is valid stays the `signoff` rule's decision
- **BREAKING (release operators): rename six signing secrets before the next release.** The delivery layer now reads its secrets as `RELEASE_<what>`, and this repository's signing secrets follow: `SSH_SIGNING_KEY_RELEASE` is `RELEASE_SSH_SIGNING_KEY`, `GPG_SIGNING_KEY` is `RELEASE_GPG_PRIVATE_KEY`, `GPG_SIGNING_PASSWORD` is `RELEASE_GPG_PASSPHRASE`, `GPG_SIGNING_FINGERPRINT` is `RELEASE_GPG_FINGERPRINT`, `COSIGN_SIGNING_KEY` is `RELEASE_COSIGN_PRIVATE_KEY` and `COSIGN_SIGNING_PASSWORD` is `RELEASE_COSIGN_PASSPHRASE`. Store the same values under the new names in the repository or organization; the old names are no longer read, and the prepare workflow stops a release that finds one missing. Token-secret migrations are listed in `docs/contributing/releasing.md`; the layer's token inputs remain `RELEASE_TOKEN` and `RELEASE_REGISTRY_TOKEN`.
- the workflows pin `itiquette/reusable-ci` at `68dffe33ae2cea4f1f2f03ae93382b6657fb2fe5`, which `REUSABLE_CI_AUTHENTICATED_COMMIT` must now name. From this commit the layer downloads its binary from the generic packages of the engine repository's owner and runs it only when it has the SHA-256 the commit records, so the binary must be published on the instance before a job runs, and every job image needs `curl` and `sha256sum`. Each call of a layer workflow passes `reusable-ci-repository: itiquette/reusable-ci`, because the layer no longer has a default repository; the `REUSABLE_CI_REPOSITORY` variable is not read and may be removed. The prepare workflow installs its changelog renderer from a pinned, checksum-verified archive, with no call to the GitHub API. `check-l3-isolation` now also requires the signer image and the Cosign key digest as literals, which they already are
- **BREAKING (release operators)** the pull-request and release workflows take their delivery layer from `itiquette/reusable-ci` (`.forgejo/workflows/` and `.forgejo/actions/`) instead of `itiquette/release-ci`, which no longer ships it. The protected approval variable is renamed from `RELEASE_CI_AUTHENTICATED_COMMIT` to `REUSABLE_CI_AUTHENTICATED_COMMIT`: it authenticates the commit of the delivery layer, and that is now a reusable-ci commit. One variable is new: `COSIGN_PUBLIC_KEY`, the PEM text of release-ci's `keys/cosign.pub`, which the signer accepts only against the digest written in `release.yml`. Release preparation is now reusable-ci's `prepare-release.yml` workflow rather than a job composed here: it admits a request only at the head of `main`, where the composed job accepted any commit reachable from it, and it takes no rate-limit token. The Codefloe token check that job made is a `preflight` job of its own. Each container image is built, scanned and recorded by one `build-release-image` step; the Trivy results in the `build-dist` run artifact are now named `trivy-results-<role>-<arch>.json`, and no published file changes name. The build declares its release files with `reusable-ci release files import-goreleaser`, which the signer requires.
- **BREAKING (unreleased 0.10.0)** the container image under the plain release tag is now the Alpine image, which includes Git and validates SHA-1 and SHA-256 repositories. The distroless image, which has no Git and validates SHA-1 repositories only, moves to `<version>-distroless`. The `<version>-alpine` tag is no longer published.
- the default container image trusts a repository mounted at `/repo`, its working directory, so `podman run -v "$PWD:/repo:ro" <image> validate` works without `--user` on a repository owned by another user.
- a SHA-256 repository that Git cannot read (Git is not installed, or it refuses the repository's owner) now reports why Git failed and what to do, instead of go-git's `core.repositoryformatversion does not support extension: objectformat`.
- the Alpine image moves to Alpine 3.24 and the test sandbox to the Debian 13 Go image. `.containerignore` admits only the binaries the images copy, which keeps the recorded sandbox and distribution archives under `bin/` out of the build context.
- The release does not require the `DEPENDENCY_GITHUB_TOKEN` secret. It only buys headroom against GitHub's API for tool resolution and gates nothing, and a release that needs a live third-party credential cannot be rebuilt by a fork, an auditor or an isolated lab. Without it the lookups run anonymously, as Nanolinter's release already does
- **BREAKING** `just test-verbose` and `just test-integration-verbose` are gone, and the launcher's `integration-verbose` suite with them. `just test-unit` and `just test-integration` now take caller arguments and hand them to `go test`, so `just test-unit -v`, `just test-integration -v` and `just test-integration -run TestSigning` all work and a recipe per flag is no longer needed. The local launcher used to refuse every extra argument, so that a stray one could not quietly change what a destructive tier runs; that refusal is relaxed for the integration tier alone, and `source`, `blackbox` and `fuzz` still refuse rather than silently drop what they cannot use. Arguments stay argv from `just` to `go test`: positional parameters through both launchers, and positional parameters after the `_` placeholder of the container shell's `bash -c '<command> "$@"' _ ...`, so no shell ever reparses them and an argument carrying quotes, `$(...)`, backticks or a semicolon arrives as one literal argument
- the external test tiers load a sandbox image instead of building one. `just build-sandbox-image` builds it from `Containerfile.sandbox` and records the OCI archive, its checksum, and a key naming the digest-pinned base, the installed packages and a hash of the Containerfile under `bin/sandbox-image`; `just test-integration`, `just test-blackbox`, `just test-external` and `just test-fuzz-all` load that recording. Each of them used to rebuild the image first, paying a base pull from docker.io, an `apt-get` from deb.debian.org and about two minutes before its first test, on every single run. A tier whose recording is missing, fails its checksum, or was built from a different Containerfile now refuses and names the recipe that records it: it never rebuilds the image on your behalf, because a tier that may pull cannot claim it reaches no network, and it never runs against a stale image, because that would test an environment the checkout no longer describes
- the sandbox tiers no longer carry the `network` tag on the `just --list` page, only `container`: with the image recorded ahead of time they reach no registry at all. `just build-sandbox-image` carries `container, network` instead, because it is now the one step that does
- the external test tiers run on Podman 4 as well as 5, and the Podman 5 floor they briefly carried (also unreleased) is gone with it. That floor was project policy rather than necessity: the only thing 4.x rejected was `uid=`/`gid=` on the `--tmpfs` carrying the container's `HOME`. The policy is no longer needed, because the mount no longer needs those options — `/home/sandbox` is now a plain tmpfs at the default `1777`, the same shape `/tmp` and `/run` have always had here. Nothing is given away by it: the container runs a single unprivileged principal under `--network=none`, `--read-only`, `--cap-drop=ALL`, `--security-opt=no-new-privileges` and private ipc/uts, so there is no second principal inside for `0700` to exclude, and no other isolation flag changed. Podman 4.9.3 is what the current Debian and Ubuntu LTS ship, so the floor had made these tiers unrunnable on a stock LTS host; `internal/cipolicy` now asserts that no `--tmpfs` option in the launcher regains `uid=` or `gid=`
- **BREAKING** `just test-blackbox-platform` is gone and `just test-blackbox` runs the whole black-box contract, the locale, PTY, resource-limit, zero-egress, GPG and SSH rows included. The two tiers were split because those rows fail rather than skip when `gpg`, `ssh-keygen` or `script(1)` is missing, which is a bad bargain on an arbitrary developer machine — but they never run on one: they run in the sandbox image this repository builds and controls, which carries every tool they need. The launcher's `blackbox-extended` suite goes with the recipe.
- the release build runs the binary it just produced and fails when its `--version` does not report the release tag. GoReleaser derives `-X main.version` from the tag through its own templating, so a stamp that stops reaching the linker, or a `{{.Version}}` that stops tracking the tag, used to ship a binary reporting something else with nothing in the pipeline asking it. This is the one thing the deleted release-parity tier could catch, at one step instead of a sandbox
- **BREAKING** the `release-parity` tier and `TestArtifactParity_BlackBox` are gone. The test compared `--version` and one failing validation across a source build, a release binary, a package binary and a digest-pinned container image, and it had never run: no recipe reached the tier, the repository's own workflow-policy guard forbids CI from touching it, and a manual run needed four staged artefacts, a three-entry checksum manifest, `--strict` and a separate approval step. A control that expensive, run zero times, is coverage on paper only. The sandbox launcher loses `--strict`, `--release-bin`, `--package-bin`, `--oci-archive`, `--oci-image`, `--checksums` and its outer OCI controller with it, and the `blackbox` and `source` tiers now simply run the whole black-box package instead of skipping one function in it
- `just test-harness` is gone and `just test-unit` covers every in-process package again. The pair existed so helper coverage would stop flattering the product number, but `test-coverage` profiled both together and the pull-request workflow ran them as one command, so the split reported nothing it promised and paid for a second run.
- `just test-tags-compile` is now the private `_test-tags-compile`. It is a step of `verify` and `verify-fix`, not a tier anyone picks from the page.
- the pull-request workflow compile-checks the `integration` and `blackbox` build tags with `go vet`. Neither tier runs in any workflow, so their tagged code could stop compiling with nothing going red, and once did. The `productPattern` policy guard used to refuse any tag selector in a workflow, which caught the vet along with the real thing; it now anchors that rule to `go test`, so executing the tagged tiers in CI is still refused while compiling them is allowed.
- the `just --list` descriptions of `verify`, `verify-fix` and `verify-full` now name the steps they run (`Lint and test, like CI does`, `Apply safe fixes, then lint and test`, `Run verify plus drift and release checks`) instead of calling them a gate, which named a role rather than a job; the old `verify-full` text also claimed "slower local tiers", which is not what it runs
- rename `just dev` to `just verify-fix`: `dev` did not say what it does, and the fixing variant of `verify` now follows the same `X` / `X-fix` pairing the repository already uses for `lint` / `lint-fix`
- rename `just install` to `just install-dev-tools` and `just install-go-dev-tools` to `just install-dev-tools-go`: `install` set up this working copy while the `install-local*` recipes install the built product for the user, so the setup recipes now say so
- `just install` no longer runs `mise trust`: trusting the reviewed `.mise.toml` is an explicit developer step with plain `mise trust`, and mise itself refuses an untrusted config
- remove process-wide memory locking; coredump suppression and scoped secret-residue controls remain, while swap-at-rest protection belongs to the host
- the hosted-workflow invariants (runner selection, job image, shipped-product test modes, gettext gating) moved from grep-and-count checks in check-workflow-policy.sh into `internal/cipolicy`, a test over the parsed workflow YAML: a reformat or an added job no longer breaks the gate, and a violated property names the job instead of a line count. The script keeps the non-workflow gates (Containerfile.ci pins, build-tag and justfile rules)
- **BREAKING (unreleased 0.10.0)** one binary for both Git object formats: `gommitlint` detects the repository's object format at runtime and uses go-git for SHA-1 and system Git for SHA-256 repositories, as both builds already did; the separate `gommitlint-sha256` binary, archive, package files and `just build-*-sha256` recipes are gone. A SHA-256 repository now works with the standard binary wherever `git` is on `PATH`; the only change for users is that there is nothing extra to download
- local `hook install`/`remove` resolve the hooks directory the way git does: linked worktrees install into the main repository's common hooks directory instead of failing, and a repository whose hooks are redirected by `core.hooksPath` (local, global, or system) is refused with guidance instead of receiving an inert file under `.git/hooks`
- generated hooks fail closed when `gommitlint` is missing: they resolve `PATH`, then the absolute path of the installing binary (baked in at install time), and otherwise refuse the commit/push with guidance; `GOMMITLINT_HOOK_MISSING=skip` restores the old pass-through for one operation
- `hook install --global` refuses to overwrite a `core.hooksPath` that names another directory unless `--force` is given, and records the replaced value; `hook remove --global` restores that value, keeps a dispatcher other tools still plug into, and only unsets `core.hooksPath` when it names the gommitlint directory
- align release CI with immutable request tags, prepared release identity, and the current signer contract
- **BREAKING**: release API and package publication use `RELEASE_API_TOKEN` and `PACKAGE_PUBLISH_TOKEN`; tool resolution uses separate optional `DEPENDENCY_GITHUB_TOKEN` and `DEPENDENCY_FORGE_TOKEN` credentials. Pages uses `PAGES_DEPLOY_TOKEN`. Provision the new stored-secret keys with the workflow update as documented in `docs/contributing/releasing.md`.
- select hosted runners through required repository variable `CI_RUNNER_LABEL`
- run all pull-request jobs in a consumer-owned image selected by immutable `CI_JOB_IMAGE`
- give Go Vet enough cold-cache headroom on the bounded one-CPU PR runner
- `jirareference` only accepts a key that stands on its own (`PROJ-123`, `[PROJ-123]`, `(PROJ-123)`; no longer `xPROJ-123` or `PROJ-123abc`), and once `jira.project_prefixes` is set only a key with a listed prefix counts as a reference: `Support SHA-256 hashing` or a lone `OTHER-123` now reports `missing_jira` instead of passing or reporting `invalid_project`; `jira.ignore_ticket_patterns` still excludes keys first
- `message.body.min_length` applies to any non-empty body on its own (0 disables it) instead of only when `message.body.required` is also set; a missing body is still governed by `required` alone
- a conventional scope with an empty segment (`feat(api,): x`, `feat(,api): x`, `feat(,): x`) is refused as `invalid_multi_scope` in both multi-scope modes; with `allow_multi_scope` off it used to pass as a single scope

### Fixed
- Prepared SDKs and dependency caches now live outside OpenBSD's boot-cleared temporary directory. Native OpenBSD black-box tests use its supported PTY flags and memory sysctl.
- OpenBSD VM preparation preserves the ISO bootloader's dotted release directory and upstream kernel path, supplies the official signed set list, and selects unambiguous package variants. QEMU guest output reaches the serial log through `nohup`; installer failures and VMs exiting without a result fail promptly.
- The Alpine release image pins `ca-certificates` 20260909-r0. Alpine 3.23 no longer serves 20260611-r0, so `apk add` refused the pinned version and every release failed while building the Alpine image
- `just test-integration`, `just test-blackbox`, `just test-external` and `just test-fuzz-all` no longer fail on a developer machine with Podman's `the specified runroot is longer than 50 characters`: the local launcher and the sandbox scripts use short state-directory names, and the sandbox now reports the offending path and its length when a long `TMPDIR` or `RUNNER_TEMP` still exceeds the limit
- `signoff` compares sign-off emails case-insensitively when it looks for duplicate signers, as the `identity` rule already compares emails, so `<Dev@Example.com>` and `<dev@example.com>` no longer count as two signers; names are still not compared
- `--range` and the `branchahead` rule no longer fail with `git traversal depth exceeded maximum: 10000` in a SHA-1 repository whose history is more than 10,000 commits deep: the go-git adapter walks only the requested range and its boundary, newest first as `git rev-list from..to` does, and still refuses a walk that would load more than 100,000 commits
- untrusted commit text no longer reaches the terminal or CI log as raw control sequences: subjects, diagnostics, verbose context and help in the text, GitLab, GitHub, table and job-summary formats escape ESC, C1 and other control characters as the text COMMIT header already did, and line breaks in commit text become spaces, so a crafted commit can no longer retitle the terminal, forge an OSC 8 link or start a GitLab `section_start` line (JSON, SARIF and the plain CSV format are unchanged)
- a second termination signal arriving while the signal handler shuts down (for example a quick double Ctrl+C) no longer crashes gommitlint with a Go `panic: send on closed channel` crash report
- `jirareference` reports a disallowed project key in a non-conventional subject once when `jira.require_in_subject` and `jira.project_prefixes` are both set, instead of two identical `invalid_project` errors, and no longer flags a key there that `jira.ignore_ticket_patterns` excludes (conventional subjects already behaved this way)
- "commits ahead" counts against the branch when a tag has the same name: in a SHA-256 repository, where the system-Git adapter is used, `git` resolved a bare `main` to the tag first, so `branchahead` and `--base-branch` counted from the tag instead of the branch and gave a different number than the same repository in SHA-1 format. Both adapters now resolve the name once, branches before tags before remotes, and count against the resolved reference
- a rule that panics during validation now fails the same way whatever the batch size: with 50 or more commits the panic was turned into a `GL902 validation_failed` error on every commit and exit 1, hiding the bug, while a smaller batch produced exit 70 and a private diagnostic report. The panic now always reaches the crash reporter
- `message.body.max_line_length` counts characters, not bytes: a body line of Swedish or other non-ASCII text was measured in UTF-8 bytes and flagged at roughly half the documented limit, while the subject rule already counted characters
- a GPG key whose self-signature carries an explicit key-expiration time of zero is no longer reported as expired at its creation time: RFC 4880 defines zero, like an absent value, as "never expires", and the identity and signature checks now treat it that way for primary keys and signing subkeys
- `hook install`, `hook status`, `hook run pre-push` and validation in a SHA-256 repository act on the repository named by `--repo-path` (or the working directory) even when run from inside a git hook: git exports `GIT_DIR`, `GIT_WORK_TREE` and `GIT_INDEX_FILE` to its hooks, and the product's own `git` calls inherited them, so a hook in one repository could read another repository's `core.hooksPath` and refuse or permit an install for the wrong reason, or read the wrong repository's object format and commits; configuration variables such as `GIT_CONFIG_GLOBAL` are still honoured
- a commit signed for both object formats (a `gpgsig` and a `gpgsig-sha256` header, as git writes with a SHA-1 compatibility mapping) is read in a SHA-256 repository with the SHA-256 signature alone, the one git verifies, instead of both armored blocks concatenated into one signature that could not be verified

## [0.9.12] - 2026-06-09

### Added
- improve web page
- add landing pages
- walk-up repo-path discovery and output polish
- expand 'config init' with safer defaults
- self-applying managed sources and trusted absolute extends
- publish JSON Schema and add 'config schema' subcommand
- authenticate managed-sources headers and scrub secrets
- schema warnings for typos and shape mismatches
- mlock + coredump suppression at startup
- default to dual-locale "both"
- add snowball stemmer adapter

### Changed
- upload dist hand-off with v3-node20 artifact action
- slim dist/ hand-off by dropping raw binary dirs
- bump forgejo-ci pins to cache-key fix
- add trivy to release toolchain for image scans
- expose release tools into ~/.local/bin in the build job
- bump forgejo-ci pins to tool-bin symlink fix
- pin forgejo-ci actions to setup-toolchain PATH fix
- fix md2man
- fix login
- fix regex
- fix timing
- bump nanolinter
- secret fix
- avoid SC2046 in test job package list
- drop unused mise.lock
- run security in the flavour and test on setup-go (no compile)
- move schema-drift and L3 gates out of the linter flavour
- pass object-format sha256 to checkout-consumer (no curl needed)
- check out via forgejo-ci checkout-consumer action
- lint in nanolinter go flavour, checkout via forgejo-ci git helper
- pin the nanolinter go-flavour image by tag and digest
- run the lint job in nanolinter's go-flavour image
- declare uv by bare registry name for locked-mode check
- let the signer publish the release, GoReleaser skips publish
- build container images with forgejo-ci setup-buildah
- use forgejo-ci scan-container-image and prepare-release actions
- reference forgejo-ci composite actions by full URL
- adopt forgejo-ci ledger action and lock mise for PRs
- remove ally check from web ci
- rework justfile groups and onboard nanolinter
- drop build-tool tests, wire missing lint tools
- migrate linting and translation tooling
- cross-repo signing via itiquette/release-ci + L3 invariant check
- harden Alpine apk pins and Renovate digests
- respect XDG_BIN_HOME and overwrite stale binaries
- centralize linter configs under development/
- add schemagen tool with invopop/jsonschema dep
- prune unused exports
- dual license GPL and EUPL
- satisfy goconst, cyclop, gofmt, and typos
- remove web gopher
- relocate shared-config guidance to reference and standardize license
- document managed source auto-apply and absolute extends
- extract output formats, shell completion, uninstall to dedicated pages
- prepare for antora
- surface man page in --help and add a drift test
- open with project overview and onboarding pointers
- supply-chain posture, verification, and k8s admission
- tighten config field comments for editor tooltips
- add usage examples section
- reorganise cli, adapters, output, app services, and packaging
- cut duplication in rules, config, signing, and adapters
- align output formatters and surface catches
- clean command and config wiring
- accept preset config paths and validate --log-level upfront
- polish validate + config-show surface and error routing
- extract report rendering into dispatch table
- split main.go into completion, help, suggestion modules
- consolidate config presentation and drop unused helpers
- split config_show into orchestration and text render
- drop the single-valued managed_sources strategy field
- move lifecycle policy into app services
- drop Log() from domain.Logger
- simplify config and render stderr as prose
- drop unused TextOptions.Verbose field
- consolidate localization and test helpers
- polish formatters and warnings catalogue
- make adapters explicit
- consolidate metadata and settings contracts
- normalize ref-not-found and skip imperative on unparseable subject
- make signature handling explicit
- clearer imperative + case feedback
- move orchestration into app services
- migrate suite from testify to internal check helper
- parallel commit validation with bounded worker pool

### Fixed
- silence group-writable warning on empty stub config
- route warnings to stderr only
- tighten imperative suggestions and conventional-context resolution
- add no-op hardening fallback for unsupported GOOS

## [0.9.11] - 2026-04-16

### Added
- add top-level panic recovery boundary
- adopt BSD sysexits.h for environmental failures

### Changed
- add stream-contract end-to-end tests
- promote magic numbers to named constants
- thread cmd.Writer/cmd.ErrWriter through command hotspots

### Fixed
- skip merge commits when LinearHistory rule is disabled
- surface config-load warnings on stderr consistently

## [0.9.10] - 2026-04-13

### Added
- add Alpine container image variant for musl environments

### Changed
- fix broken config table

## [0.9.9] - 2026-04-07

### Added
- improve localization support
- allow multiple args to base-branch flag

### Changed

## [0.9.8] - 2026-04-02

### Changed
- update translations

### Fixed
- improve spell lint output

## [0.9.7] - 2026-03-29

### Added
- autodetect ci with pr basebranch

### Changed

## [0.9.6] - 2026-03-27

### Changed
- add container version upgrade tooling
- add upgrade containers just job
- pin checkout action to digest with full semver

### Fixed
- version corrected in container build

## [0.9.5] - 2026-03-22

### Added
- improve -v rule output

### Changed
- remove unused MaxGitRefLength constant
- update translation pot
- updated deps
- align struct fields and extract ssh-rsa constant
- remove crypto imports from domain and signing re-export wrappers
- split config test files to match source split
- split config.go into subcommand-focused files
- extract fileops package from signing adapter
- document SignedIdentity intent and remove dead rule helpers
- rename NewDefault/NewDefaultConfig

### Fixed
- correct MinSignoffCount default in doc and config show
- merge LimitsConfig during config layering
- default MinSignoffCount to 1
- make disabled list take precedence
- correct case sentence word extraction

## [0.9.4] - 2026-03-10

### Added
- add presets function
- add remote config and sync func
- improve managed source troubleshooting UX
- add config provenance and source graph
- add shared config sync and health checks
- add sha256 validation backend

### Changed
- add missing license header
- refresh toolchain and fix lint compatibility
- update trivy to 0.69.3
- lint fix
- fix test for branch
- update branch-ahead default expectation
- document shared config workflows

### Fixed
- set branchahead default to 10
- cleanup sha256 feat residue
- cleanup sha256 feat residue
- improve branch detection

## [0.9.3] - 2026-02-26

### Added
- replace xgettext with xgotext for translation

### Changed
- add REUSE license annotations for locale files
- regenerate translation catalogues with xgotext
- fix lint error
- add better sha256 support for failing test
- fix gets main
- fix lint and ci fetch depth
- add ripgrep to install
- add test coverage for i18n localizer package
- extract repeated helpers for validation and report building
- localise rule help templates
- enforce registry completeness and align git error returns
- move output format/theme defaults from domain to adapter layer
- use output.DefaultTheme in CLI/adapter layer
- move ruleConfigKeys from domain/settings to cli adapter
- move output format constants and theme normalisation to adapter layer
- remove Help field from domain, centralise help text in output layer
- move output format/theme registry
- remove "Passed" display string from domain reporting
- remove progress Info logging from domain validation
- move DisplayName to output layer, delete domain.ReportOptions
- split commit readers and select git cli for sha256
- parse ignore SHAs and clean up CLI test helpers
- add DerefPositiveInt, collapse 5 repeated deref+guard blocks
- centralise error and rule help registries
- consolidate hash truncation and jira validators
- add validation runner and compliance headers
- move check-translations out of lint to documentation group
- replace FormatReport switch with map-based dispatch
- bind run logic onto ValidationTarget, remove type-tag switch
- bundle BuildReport tail parameters into ReportInput
- single construction path for ValidationContext
- replace ValidationContext in BuildReport
- collapse RepositoryRuleWithInfo into RepositoryRule
- remove pass-through metadata from ValidationContext
- introduce TableOptions, separate from TextOptions
- expose TextPlain, remove Plain field from TextOptions
- format translation scripts with shfmt -i 2

### Fixed
- improve translation handling
- localise RecoveryError structural strings at output boundary
- add lint-shell-fmt-fix alias to match just-check error message
- validate --rule and --exclude-rule flag values against known rule names
- improve single source truth, fix sha256 fallback

## [0.9.2] - 2026-02-20

### Added
- enforce exclusive verbose flags
- expose warnings and sources in reports
- add JSON error code field
- show config sources in summaries
- include structured warning details

### Changed
- lint fixes
- migrate config renovate.json
- remove unused context value
- drop stale benchmark todo
- add compress changelog
- remove orphaned ErrUnsupportedSignatureType error code
- remove dead legacy code from hook removal
- consolidate duplicate conventional commit detection
- use value receiver on stateless ImperativeValidator
- pass localizer through call chain in hook removal
- remove dead ParseMultipleScopesFromRaw function
- replace magic hook-type strings with constants
- remove orphaned conventional commit helper functions
- replace confusing loop-return with utf8.DecodeRuneInString
- merge identical CaseUpper/CaseSentence branches and remove unused params
- flatten 4-level nesting in validateRefsLineFormat
- rename shouldSkipRealRules to hasOnlyPseudoRuleErrors
- deduplicate error joining in buildRepositoryRuleReport
- unexport internal imperative helper functions
- standardize error creation in signoff.go to chaining style
- normalise config path reasons
- add UnknownRuleError
- drop unused rule filters
- localise failure messaging
- localise helper errors
- unwrap config path errors
- align output errors
- improve recovery errors
- centralise help text
- unify format helpers
- add recovery errors for args
- chain recovery error causes
- standardise output errors
- decouple filesystem access
- add dependency recovery hints
- add typed validation errors
- validate output format
- reuse output format constants
- centralise config path validation
- add repository sentinel errors
- add commit constructor with payload
- map verification codes
- enforce typed error codes and fix lint
- unify filesystem adapters
- return ErrReferenceNotFound
- add hook existence errors
- align invalid type code
- stabilise error formatting
- normalise rule display names
- align tests and defaults
- add legacy guidance
- restore legacy messages
- normalise codes and naming
- drop legacy error codes
- localise context keys
- centralise enablement logic
- stabilise key provider context
- use typed subject parsing errors
- centralise defaults and filtering
- centralise defaults and codes
- centralise rule defaults
- centralise output formats
- stabilise Jira regex errors
- add config format helpers
- extend config validation errors
- prefer sentinel errors in tests
- use typed no-key errors
- standardise key directory errors
- align error codes and defaults
- add recovery errors
- reuse config format constants
- use output format constants
- assert typed helper errors
- reuse format constants
- reuse minimal YAML helper
- reuse minimal config helper
- centralise output YAML strings
- assert typed config errors
- centralise log level strings
- assert subject and range errors
- use format constants
- reuse log level constants
- replace error level literal
- improve building rpm and deb

### Fixed
- restore amend regression
- remove duplicate godoc comment in imperative validator
- use domain error constants in jira validation rule
- support --flag=value parsing
- preserve error causes
- default to text format
- drop amend fixup detection

## [0.9.1] - 2026-02-20

### Added
- improve completion support
- add theme support

### Changed
- update release flow to handle moved ref
- add test pipx install
- add test token support

## [0.9.0] - 2026-02-20

### Added
- add prel l18n support
- add verbose flag to hook status command

### Changed
- misc clean up
- remove duplicate security validation
- use value semantics for ValidationError returns
- add assertions to imperative edge case tests
- add rapid context cancellation tests
- remove vim artifacts from README

### Fixed
- harden validation safety and cli errors
- warn on file close failures in validation output
- generate available commands list dynamically
- handle writer errors in command output
- respect NO_COLOR in GitLab CI formatter
- collect all scope validation errors before returning
- use strconv.Itoa for integer-to-string conversion in test
- return nil for empty slice in sortedCopy
- clarify unimplemented afero adapter methods
- warn on GPG key load failures
- extract actual format from SSH signature blob
- prevent TOCTOU race in hook installation
- correct method reference bug in IsPathWritable
- reject invalid spell check locales

## [0.8.11] - 2026-02-20

### Added
- align config, reporting, and output behavior
- lint fix
- surface config warnings in reports

### Changed
- centralize rule help metadata
- split GitHub/GitLab formatters into helpers
- derive format list from domain settings
- derive format list from domain settings
- assert ignore_commits warnings surface
- dedupe special-character formatter cases

### Fixed
- parse environment options in config adapter
- improve integration tests
- improve integration tests
- warn on invalid_commit entries
- remove auto detect base branch
- map missing config to usage exit code
- normalize rule-help names to canonical settings
- remove auto detect base branch residue
- map git reference errors to domain codes
- allow empty list overrides in merges
- allow clearing string fields in merges
- normalize rule priority during merges
- correct various docs
- correct various docs
- correct more docs
- add context_cancelled message mapping
- clarify table summary counts
- report validated and ignored commit counts
- improve header output
- respect context cancellation during verification

## [0.8.10] - 2026-02-20

### Added
- implement real signature verification payloads
- add configurable limits for commits and key files
- make commit limits configurable

### Changed

### Fixed
- improve misc minor docs
- broken json time now more deterministic
- improve single vs plural output
- improve file reading
- improve traverse handling
- ignore private keys
- improve key check paths
- set default subject length 100
- validate all rules
- use csv output for plain

## [0.8.9] - 2026-02-20

### Added
- improve commit sort

### Changed

### Fixed
- normalise rule names
- only active branchead rule on base-branch flag
- preserve enabled rule order
- throw error on invalid config file
- support hiearchial config loading
- add fuller config merging
- cap stdin size and reject --count 0
- add valid repo check
- secure auto-discovered config path
- tighten parser selection and improve CLI format help
- improve global dir check
- only overwrite global tag with force
- improve text summary and rule help consistency

## [0.8.8] - 2026-02-20

### Changed
- add guards for duplicate relese

## [0.8.7] - 2026-02-20

### Added
- no def. rule linearhistory, improve help, mv ci scripts
- add github format output improvements

### Changed
- add gommitlint image
- lint fix
- use gommitlint 0.8.6
- move scorecard to full sec check
- fix govuln
- fix redundant checks already in lint

## [0.8.6] - 2026-02-20

### Added
- add sha256 expr build, misc docs

### Changed
- minor install docchange

## [0.8.5] - 2026-02-20

### Added
- improve ci cache, use base renovate

### Changed

## [0.8.4] - 2026-02-20

### Changed
- misc version bumps
- update readme
- set ssh multiplex to avoid ratelimit

### Fixed
- fix mise upgrade job

## [0.8.3] - 2026-02-20

### Changed
- change links for pub keys, changelog script

## [0.8.2] - 2026-02-20

### Changed
- use gommitlint etc

## [0.8.1] - 2026-02-20

### Changed
- add unsupported darwin build

## [0.8.0] - 2026-02-20

### Changed
- initial commit

[Unreleased]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.12...HEAD
[0.9.12]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.11...v0.9.12
[0.9.11]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.10...v0.9.11
[0.9.10]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.9...v0.9.10
[0.9.9]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.8...v0.9.9
[0.9.8]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.7...v0.9.8
[0.9.7]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.6...v0.9.7
[0.9.6]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.5...v0.9.6
[0.9.5]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.4...v0.9.5
[0.9.4]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.3...v0.9.4
[0.9.3]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.2...v0.9.3
[0.9.2]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.1...v0.9.2
[0.9.1]: https://codefloe.com/itiquette/gommitlint/compare/v0.9.0...v0.9.1
[0.9.0]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.11...v0.9.0
[0.8.11]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.10...v0.8.11
[0.8.10]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.9...v0.8.10
[0.8.9]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.8...v0.8.9
[0.8.8]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.7...v0.8.8
[0.8.7]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.6...v0.8.7
[0.8.6]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.5...v0.8.6
[0.8.5]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.4...v0.8.5
[0.8.4]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.3...v0.8.4
[0.8.3]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.2...v0.8.3
[0.8.2]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.1...v0.8.2
[0.8.1]: https://codefloe.com/itiquette/gommitlint/compare/v0.8.0...v0.8.1
[0.8.0]: https://codefloe.com/itiquette/gommitlint/releases/tag/v0.8.0
