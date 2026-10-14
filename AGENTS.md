# AGENTS.md

Coding-agent instructions for working in this repository.

## What this project is

`gommitlint` is a commit-message and commit-history linter for Git, written in
Go 1.27. It runs as a CLI, Git hook, or CI step. Stack: urfave/cli v3, go-git v5,
koanf v2 (config), zerolog, gotext (i18n), `just` as task runner, nanolinter for
repo linting, mise for tool pinning, Forgejo Actions CI in `.forgejo/workflows/`.

The product boundary is: a focused commit linter — hexagonal, functional,
idiomatic, don't overengineer.

## Critical rules

**Never run these git commands without explicit user approval:** `git commit`,
`git push`, `git reset`, `git rebase`, `git merge`, `git tag`. Present changes and
let the user commit them.

## Read these first

- `README.adoc` — positioning and quick start
- `docs/modules/ROOT/pages/architecture.adoc` — layers, boundary rules, design decisions
- `docs/modules/ROOT/pages/cli-conventions.adoc` — CLI flag/output/exit-code conventions
- `docs/modules/ROOT/pages/reference/rules.adoc` — rule catalog reference
- `CONTRIBUTING.md` — DCO sign-off, commit standard, translation workflow

## Commands

- `just verify` — read-only gate: lint + unit tests
- `just verify-fix` — mutating loop: lint-fix, then verify with lint in allow-dirty mode
- `just test` — in-process tests and fuzz seed corpora
- `just test-fuzz-native` — replay committed seeds; `just test-fuzz-container 14s` runs the
  separate all-target generated campaign
- `just test-integration-container`, `just test-blackbox-container`, `just test-combined-container` — exact
  local-only entry points for tagged external suites in rootless Podman
  with no network. No active CI workflow runs them. `just test-blackbox-container` runs
  the whole black-box contract, the GPG, SSH, PTY, locale, namespace and
  resource-limit rows included. `just test-integration-container` forwards extra
  arguments to the `go test` inside the sandbox (`just test-integration-container -v`,
  `just test-integration-container -run TestSigning`); they travel as positional
  parameters and are never spliced into a command string. The other two take
  no arguments and refuse rather than ignore them.
- `just test-combined-container-matrix [name...]` — the
  distribution matrix: the integration and black-box suites rerun once per
  distribution in `test/distribution/images` (Ubuntu, Debian, Fedora, openSUSE
  Leap and Tumbleweed, Arch), each with its own packaged git, GnuPG and
  OpenSSH. It is the same launcher's `distribution` mode with the same
  guarantees, local-only and not part of `just verify`. Record the images
  first with `just test-container-prepare-distributions [name...]` (network);
  `just test-container-clean-distributions` removes those recordings.
- `just test-unit-native` likewise forwards extra arguments to `go test`
  (`just test-unit-native -v`). `just test-unit-container` runs the untagged
  packages and committed seeds with the race detector in the prepared Go image,
  without building or executing the shipped CLI. There are no verbose recipes:
  `-v` is an argument.
- Single test: `just test-run-native <filter>` or `just test-pkg-native ./internal/path/`
  (raw: `go test -count=1 -race -run 'TestName' ./internal/path/`)
- `just lint` — justfile formatting, the repository workflow guard, and the
  nanolinter suite (`nanolinter verify`, plan in `nanolinter.toml`); granular
  recipes exist per language: `just lint-go`, `lint-yaml`, `lint-markdown`,
  `lint-shell`, ... and per-language fixes: `just lint-fix` (all
  safe fixes) or `lint-go-fix`, `lint-yaml-fix`, `lint-markdown-fix`,
  `lint-shell-fix`, `lint-shell-format-fix`. Keep these granular recipes — do
  not collapse them into one `nanolinter fix`. The `links` check is advisory
  there and in pull requests (`[policy] warn` in `nanolinter.toml`): it
  reports and never fails the run. `just lint-links` is the blocking run and
  the link gate before a release.
- `just security` — every scan (`security-secrets`, `security-sast`,
  `security-iac`, `security-govulncheck`, `security-osv`, `security-trivy-fs`,
  `security-container`) plus OpenSSF Scorecard, which needs credentials
- `bash .forgejo/scripts/check-workflow-policy.sh` — repository policy gates
  outside workflow YAML (Containerfile.ci pins, build-tag, black-box import,
  and justfile rules);
  the workflow invariants themselves are `test/policy`, a Go test over
  the parsed YAML that runs with the ordinary unit tests
- `GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -c -tags=blackbox
  -o /tmp/gommitlint-blackbox.test ./test/blackbox` — compile the tagged
  black-box harness without running tests or a shipped binary
- `just build` — build for current arch into `bin/`
- `just verify-full` — non-publishing local pre-release gate: drift checks,
  release builds/config, and repeat-build verification. L3 isolation remains a
  separate pull-request gate.
- `just install-dev-tools` — install locked mise tools and required Go tools
- `just doctor` / `just install-dev-tools-go` — check/install required tooling

See the `justfile` for the full recipe list.

Execution is opt-in by recipe name: `test`, `verify`, and `verify-fix` stay in
the current environment; they never select a container or VM. The Bash
`scripts/testing/environment.sh` preflight reports OS/architecture and validates
the requested mode, without preparing images or falling back to another mode.
Current Podman launchers require a Linux controller. Native macOS/BSD black-box
coverage is not implied by Linux container results.

Native VM execution is explicit: `test-vm-prepare TARGET` reads the pinned target
in `scripts/testing/profiles.json`, bootstraps the guest and records its project
dependencies. An optional JSON argument imports a custom development image.
`test-unit-vm`, `test-race-vm`, `test-integration-vm`,
`test-blackbox-vm` and `test-combined-vm` select their native suites. BSD uses
QEMU; macOS uses Tart. `test-*-container-on-macos` selects Linux containers
inside a Tart Linux guest. Read `docs/testing/test-environments.adoc` for image
prerequisites, targets and the controller/guest boundary. These recipes never
belong in default verification or hosted CI. Do not run `vm-guest.sh`, its project
adapter, `vm-bootstrap-guest.sh`, or the VM trust helper directly on a host; do not manufacture their
guest records. Native tagged-tier execution is allowed only through the VM recipes,
with host/guest checks, unprivileged execution and network denial intact.

The exact agent-safe verification sequence is:

```bash
bash .forgejo/scripts/check-workflow-policy.sh
nanolinter verify --offline --allow-dirty
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -run='^(Test|Example)' -count=1 -race -buildvcs=false $(GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go list './...' | grep -v generated)
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -run='^Fuzz' -count=1 -race -buildvcs=false $(GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go list './...' | grep -v generated)
tmp=$(mktemp -d)
trap 'rm -rf -- "$tmp"' EXIT
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -c -tags=integration -o "$tmp/" ./test/integration/...
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off go test -c -tags=blackbox -o "$tmp/gommitlint-blackbox.test" ./test/blackbox
```

The last two commands compile tagged harnesses only. They do not execute
`TestMain`, an external process, Podman, or a shipped binary.

## Architecture

Hexagonal; dependencies point inward (adapters → app → domain, never reverse):

- `internal/domain/` — semantic models and validation decisions: `rules/`,
  `rulecatalog/`, `settings/`, `diagnostics/`. No adapter or application imports.
  Supplied capabilities can perform effects; host-dependent helpers and
  package-level error/pattern values are described in the architecture page.
- `internal/app/` — use-case coordination (`validation/`, `hooks/`).
- `internal/adapters/` — everything external: `cli/`, `git/`, `configuration/`,
  `output/`, `signing/`, `logging/`, ...

Configuration is never stored in `context.Context` — pass it as explicit
parameters; context is for cancellation and logging only.

Rule enablement is three-level: `disabled` (vetoes) → `enabled` → catalog default.
Default-disabled rules: `jirareference`, `identity`, `linearhistory`
(`internal/domain/rulecatalog/catalog.go`).

## Conventions

- Value semantics: value receivers that return new instances, never mutating
  pointer receivers. Pure functions with explicit dependencies.
- Validation errors use the fluent builder (real example from
  `internal/domain/rules/branchahead.go`):

  ```go
  return domain.New(r.Name(), diagnostics.ReasonTooManyCommits, message).
      WithContextMap(diagnostics.Context{
          diagnostics.ContextActual:   strconv.Itoa(commitsAhead),
          diagnostics.ContextExpected: fmt.Sprintf("max %d", r.maxCommitsAhead),
      })
  ```

- Linter configs live in tool-owned directories under `development/`
  (`golangci-lint/golangci.yml`, `typos/typos.toml`,
  `gitleaks/gitleaks.toml`, `osv-scanner/osv-scanner.toml`, `actionlint/actionlint.yaml`,
  `trivy/trivy.yaml`) plus root `nanolinter.toml`.
- User-facing strings must be translatable (localizer.Get/GetN); `just check-translations` verifies the catalogs match the extracted strings.

## Testing

Pull requests run static checks and untagged tests, including fuzz seeds, under
the race detector on the runner selected by repository variable
`CI_RUNNER_LABEL` (`ci-builder` in production). Generated-input fuzzing, tagged integration
packages, black-box packages, artifact parity, and all shipped-binary execution
are excluded from pull requests.

- Assertions outside `test/blackbox` use the in-repo
  `internal/testhelpers/check` package (go-cmp based, testify-style API:
  `check.NoError(t, err)`, `check.Equal(t, want, got)`). Black-box tests use
  `testing.T` directly and must not import product internal packages. Do **not**
  import testify — it was removed.
- Table-driven tests; names follow `Test<Component>_<Method>_<Scenario>`,
  subtests snake_case. Test data in adjacent `testdata/`.
- Build tags name what a test needs; the directory names which tier it is in,
  and the two agree. None for unit tests; `//go:build integration` for
  in-process scenarios and their scaffolding; `//go:build blackbox` for
  scenarios that drive the shipped binary.
- Black-box scenarios live in `test/blackbox/` and run the product through the
  one harness there. `test/integration/` keeps only what runs in
  process.
- Tag by what a scenario needs, at the granularity of a scenario. Tagging a
  whole file for one subtest that needs a binary moves the rest with it: that
  is how four in-process subtests once ended up in a tier requiring a built
  product and a sandbox. The check is `-tags=integration` passing with no
  binary in the environment at all.
- A helper shared by in-process integration scenarios belongs in a subpackage —
  `gitrepo` for repositories, `testkit` for fixtures — never in one scenario's
  `_test.go`. Black-box-only Git and config fixtures stay under `test/blackbox`
  and use direct dependencies or the standard library, never product internals
  or helpers from `test/integration`.
- The supported recipes route through
  `.forgejo/scripts/run-external-tests-local.sh`, which builds and checksums the
  product before the fail-closed sandbox launcher.
- The local launcher snapshots current tracked content and untracked non-ignored
  files into private state, rejecting unsafe/special/symlink paths and excluding
  ambient caches and credential paths. It hashes the manifest, builds and
  checksums the source product from that snapshot, and mounts only a second
  verified private copy for tagged harness compilation. The harness must use
  `GOMMITLINT_BLACKBOX_BIN` and fail rather than rebuilding when that binary
  path is absent. One variable names the artifact under test for the black-box
  harness.
- Product launchers refuse CI marker environments before clearing their
  environment. Black-box runs have a required tool contract and never silently
  skip; the sandbox image supplies every tool the platform/tool scenarios need,
  so no row is optional.
- Scheduled/manual generated fuzzing uses the launcher's `fuzz` mode and never
  executes the shipped product; pull requests execute committed fuzz seeds only.
- The static workflow guard permits the generic sandbox launcher only from the
  exact `.forgejo/workflows/fuzz.yml` path and only in `fuzz` mode without
  product binaries, product options, integration packages, or black-box tags.
  Every product mode and every launcher reference from another active workflow
  fails the guard.
- There is no live-provider tier in this repository. Managed-source network
  scenarios are outside the hermetic suites and have no Actions workflow.
- Repository-wide workflow and launcher contracts live in `test/policy/` and
  run with ordinary untagged tests. Keep package-local tests beside their code
  and shared assertions in `internal/testhelpers/`; tests do not mirror hexagonal layers.
- Integration tests build repos with `test/integration/gitrepo` helpers
  and must stay isolated (`t.TempDir()`, `t.Setenv()`, no network).
- Tests that construct urfave/cli apps run serially (`cliExecutionMutex` in
  `test/integration/`) — don't add `t.Parallel()` to them.

Agents may run static checks, untagged unit/race/fuzz-seed tests, the
compile-only tagged harness commands above, generated fuzz campaigns, and the
tagged external tiers. Container integration/black-box execution is allowed **only**
through `just test-integration-container`, `just test-blackbox-container`,
`just test-combined-container`, and `just test-combined-container-matrix`, which route through
`.forgejo/scripts/run-external-tests-local.sh`: it snapshots and checksums the
worktree, builds the product outside the sandbox, and runs those exact artifacts
in the rootless zero-network Podman sandbox, which runs on Podman 4 or
newer. The distribution matrix (`distribution` mode) additionally compiles the
tagged harnesses outside the sandbox from the same snapshot, records them in a
checksummed manifest, and only executes them inside each distribution image,
because those images carry no Go toolchain. Never run a `-tags=integration` or
`-tags=blackbox` binary outside that launcher or the explicit VM recipes above,
never invoke `podman` by hand to
stand a suite up, and never bypass the launcher's CI-marker refusal — the
refusal is the reason these tiers are safe to allow at all.

Generated fuzz campaigns run through `just test-fuzz-container <time>`, always
with a bounded time (the recipe defaults to 14s). That is the launcher's
`fuzz` mode: product-free, zero-network, and never executing a shipped binary.
A campaign that finds a crasher is a finding: commit the reproduced
`testdata/fuzz/` entry and fix the code, do not retune the target to make it
pass.

Container work is allowed, and one step of it reaches the network:
`just test-container-prepare` runs `.forgejo/scripts/prepare-sandbox-image.sh`,
which builds the sandbox from `Containerfile.sandbox` — pulling the
digest-pinned base from docker.io — and records the OCI archive, its checksum
and a key describing what it was built from under `bin/sandbox-image`. The
tiers only load that recording, with `--pull=never`, and refuse when it is
missing, corrupt, or keyed to a different Containerfile; they never rebuild it
and never reach a registry. Run the recipe when the Containerfile changes.
Execution itself stays `--network=none`. `just test-container-prepare-distributions` is the
matrix's equivalent: `prepare-sandbox-image.sh --distribution NAME` builds
`test/distribution/Containerfile` over the base pinned (version tag and digest)
in `test/distribution/images`, admits only that base's registry repository, and
records into `bin/distribution-images/NAME`, keyed to the list entry, the
Containerfile and `test/distribution/install-deps.sh`. Run it when anything
under `test/distribution/` changes.

Still not allowed: there is no live-provider tier in this repository; if one is
ever added it is excluded too. Do not add any such tier to an active workflow.

## Boundaries

- `generated/` (manpages, completions, locale catalogs, changelog.gz) — never
  hand-edit; regenerate with `just generate`.
- `schemas/gommitlint.schema.json` — regenerate with `just generate-schema`;
  drift-checked by `just check-schema`.
- `locale/**` gettext catalogs — update via `just update-translations` /
  `just generate-translations`.
- `bin/`, `dist/`, `web/static/fonts/` (third-party OFL-1.1 fonts) — don't touch.
- Licensing is REUSE-compliant: code `EUPL-1.2 OR GPL-3.0-or-later`, docs/config
  `CC0-1.0`, copyright "The Gommitlint Authors". New files need SPDX headers
  unless a `REUSE.toml` annotation covers them (`**/*.md` and `**/*.adoc` do).

## Git

- Brief Conventional Commits; the repo lints its own history (`.gommitlint.yaml`).
- Always `git commit -s` (DCO Signed-off-by required).
- Never add Co-Authored-By or other AI co-author trailers.
