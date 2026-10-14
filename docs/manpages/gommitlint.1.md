# GOMMITLINT 1 "User Commands"

## NAME

gommitlint - git commit linter

## SYNOPSIS

**gommitlint** [*OPTION*...] *COMMAND* [*args*...]  
**gommitlint** [*OPTION*...] **validate** [*OPTION*...]  
**gommitlint** [*OPTION*...] **doctor**
**gommitlint** [*OPTION*...] **config** {**show**|**check**|**init**|**schema**|**sync**}  
**gommitlint** [*OPTION*...] **hook** {**install**|**remove**|**status**} [*OPTION*...]

## DESCRIPTION

**gommitlint** validates Git commit messages against configurable rules. It supports conventional commits, subject/body checks, JIRA references, spell checking, author-email policy, and signature presence, armor, type, and key-inventory policy. It does not cryptographically verify or authorize a commit signer.

Without another effective target, **gommitlint validate** selects HEAD.
CI detection can supply a base branch and change that selection.
Use **--no-ci-detect** when an explicit ref, range, or count must control the target independently of CI.

A *FILE* of "-" means standard input for **--message-file**.
Target precedence is message file, effective base branch, range, ref, count, then HEAD.
A base comparison selects commits reachable from HEAD but not from the selected base, not a chronological interval.

By default, configuration merges built-in defaults, global settings under *$XDG_CONFIG_HOME/gommitlint/settings.{yaml,yml,toml}*, and a project *.gommitlint.{yaml,yml,toml}*. **--config** selects a separate explicit-file mode. **--ignore-config** uses defaults only. Environment variables affect discovery and dedicated runtime behavior rather than providing a general configuration overlay.

Discovery selects at most one user file and one project file, in YAML, YML, then TOML priority.
Project configuration can override user settings. User configuration is not an enforced policy floor.

**gommitlint** validates commits using 10 configurable validation rules: **branchahead**, **commitbody**, **conventional**, **cryptosignature**, **identity**, **jirareference**, **linearhistory**, **signoff**, **spell**, and **subject**. By default, 7 rules are enabled (disabled: identity, jirareference, linearhistory).

**gommitlint** has four main commands:

- **validate** - Check commit messages against validation rules
- **doctor** - Inspect configuration, hook state, and effective policy
- **config** - Manage configuration (show effective config, generate templates)  
- **hook** - Manage Git hooks (install, remove, check status)

Validation results are written to standard output. Logs, errors, and diagnostics go to standard error. See **STANDARD OUTPUT AND STANDARD ERROR** and **EXIT STATUS** for details.

## GLOBAL OPTIONS

**--help**, **-h**
: Show help message and exit.

**--version**
: Output version information and exit.

**--config** *FILE*
: Load built-in defaults plus *FILE* and its imports. Do not load discovered global or project configuration files.

**--ignore-config**
: Use built-in defaults and ignore all configuration files.

**--repo-path** *PATH*
: Operate on git repository at *PATH* (default: current directory).

Repository selection does not restrict user-wide **hook --global** operations.
Relative input/output paths still use their documented command-specific bases.

**--color** *MODE*
: Control colored output: auto, always, never (default: auto). Nonempty **NO_COLOR** and **TERM=dumb** override **always**.

**--theme** *NAME*
: Set output theme. Root help lists supported values. **config show** reports effective configuration, not a catalogue of every theme.

**--quiet**, **-q**
: Suppress successful reports, including JSON and SARIF, and reduce logging to errors. An empty stdout stream is therefore possible on success.

**--log-level** *LEVEL*
: Set logging level: error, warn, info, debug, trace (default: info).

**--allow-coredumps**
: Skip Gommitlint's attempts to suppress core dumps. Normal startup attempts RLIMIT_CORE=0 and, on Linux, PR_SET_DUMPABLE=0. Failure is nonfatal and reported at debug level. These defenses do not guarantee that sensitive memory cannot reach disk. Host policy can still prohibit core dumps when this flag is set. Also available through **GOMMITLINT_ALLOW_COREDUMPS=1**.

## VALIDATE OPTIONS

**--message-file** *FILE*, **-f** *FILE*
: Read commit message from *FILE* instead of git commit (use - for stdin).

**--base-branch** *BRANCH*, **-b** *BRANCH*
: Validate commits reachable from HEAD but not from *BRANCH*. A comma-separated fallback list selects the first existing candidate. If an explicit required base has no match, validation fails with **66**. An unresolved optional base can instead warn and skip branch-dependent checks.

**--range** *RANGE*
: Validate commits in git *RANGE* (e.g., main..feature).

**--ref** *REF*, **-r** *REF*
: Validate specific git *REF* (default: HEAD).

**--count** *N*, **-n** *N*
: Validate last *N* commits from HEAD (default: 1).

**--format** *FORMAT*
: Select text, plain, json, sarif, github, gitlab, forgejo, or table. Explicit format wins over CI detection, followed by the command's text default. The configuration **output** field does not select the validation format.

**--verbose**, **-v**
: Increase report detail, up to two occurrences in total. **-v** and **--verbose** contribute to the same count. More than two is misuse. Quiet mode suppresses additional report detail.

**--rule** *RULES*
: Validate only specific *RULES* (comma-separated, e.g., conventional,signoff).

**--exclude-rule** *RULES*
: Exclude specific *RULES* from validation (comma-separated, e.g., cryptosignature).

**--ignore-repo-rules**
: Only run commit message rules, ignore all repository rules.

**--no-ci-detect**
: Disable automatic CI environment detection. By default, **gommitlint** detects GitHub Actions, Forgejo/Gitea Actions, and GitLab CI environments to automatically set **--base-branch** and **--format**. This flag disables that behavior. Can also be set via **GOMMITLINT_NO_CI_DETECT** environment variable.

**--rule-help** *RULE*
: Show detailed help for specific *RULE* and exit.

**--report-file** *FILE*
: Write validation results to *FILE* instead of stdout (use - for stdout). Setup can create parent directories and truncate an existing file before validation completes. Failure can leave an empty or partial report. Select an unused path when replacement is not intended.

## HOOK OPTIONS

**--force**, **-f**
: Replace existing Gommitlint-managed hooks during installation. Unrelated custom hooks remain protected even with **--force** and require separate manual handling.

**--global**
: Install/remove hooks under *$XDG_CONFIG_HOME/git/git-hooks/*, defaulting to *~/.config/git/git-hooks/* when **XDG_CONFIG_HOME** is unset, and manage global git config core.hooksPath. Install refuses an existing different value unless **--force** records and replaces it; removal restores that value when safe.

Global installation/removal changes user-wide Git configuration, regardless of **--repo-path**, **--config**, or **--ignore-config**.
These operations do not prompt for confirmation and are not one transaction across all files.
Record existing hooks and settings before approving changes. Global removal has no dry-run mode.

## CONFIG OPTIONS

**--format** *FORMAT*
: Select text, json, or yaml for **config show** or **config check** (default: text). **config sync** has text output and no **--format** option.

**--view** *VIEW*
: Select the text output detail level for **config show** and **config check**: compact, verbose (default: verbose).

**--source** *ID*
: For **config sync**, sync only the managed source with *ID*.

**--dry-run**
: For **config sync**, fetch and verify selected managed sources without replacing vendored files or their metadata. Fetches can send configured credentials.

**--check**
: For **config sync**, fetch selected sources and fail if vendored copies are missing or stale, without refreshing them. This is not an offline cache-only check.

**--timeout** *DURATION*
: For **config sync**, bound each managed-source download with a positive Go duration (default 10s). This is not a whole-command timeout.

**--compact**
: For **config schema**, emit single-line JSON instead of indented (default: indented).

**--preset** *PRESET*
: For **config init**, write a small config extending a built-in preset: minimal, conventional, ambitious, or secure.

## STANDARD OUTPUT AND STANDARD ERROR

**gommitlint** separates results from diagnostics so output can be piped and parsed reliably.

*Standard output* carries command results, including validation reports, configuration exports, and help.
With **validate --format=json**, a completed, unsuppressed report is one JSON document.
Use **--report-file** *FILE* for a file destination, or **--report-file -** for stdout.

*Standard error* carries logs, warnings, usage errors, and command diagnostics.
Validation findings remain in the selected report rather than all moving to stderr.
Diagnostic wording and formatting can change between releases.

Startup, configuration, cancellation, or output failures can prevent a complete JSON report.
Capture the producer's exit status separately from any parser's status. A successful parser does not prove successful validation.
Retain stderr for failure diagnosis rather than discarding it unconditionally.

**--quiet** can suppress successful machine reports and forces error-level logging.
**--log-level** controls diagnostics separately from report verbosity.
Validation JSON contains **timestamp**, **summary**, **metadata**, **warnings**, **commits**, and **repository**, without a numeric schema version.
Plain output is six-column headerless RFC-4180 CSV. Parse CSV records rather than assume one physical line per record.

## EXIT STATUS

**gommitlint** follows POSIX conventions for `0`, `1`, `2`, and signal-derived codes. It uses BSD *sysexits.h* conventions (range 64–78) for classes of failure where a specific category exists. CI scripts that only care about pass/fail should check `exit == 0` or `exit != 0`. Scripts that want to distinguish "this commit is wrong" from "gommitlint could not run" should treat `1` specially and leave the rest as infrastructure problems.

**0**
: No reported fatal error. Validation can include ignored commits or skipped rules, so success is not proof that every rule checked every commit.

**1**
: Validation rule violations found in commit messages or repository.

**2**
: Command misuse — bad flags, unknown subcommand, or cancellation without an associated signal. Reserved for situations that don't fit a *sysexits.h* category.

**64**
: Usage error (*EX_USAGE*) for a bare **gommitlint** invocation with no subcommand. A concise usage card is written to standard error.

**65**
: Malformed input (*EX_DATAERR*). The user supplied a reference, range, or count that is syntactically invalid or out of bounds (invalid commit range, --count exceeds the walk limit).

**66**
: Missing input (*EX_NOINPUT*). A ref, range, or config path the user named on the CLI cannot be found (commit SHA unknown, --config points at a nonexistent file).

**69**
: External service unavailable (*EX_UNAVAILABLE*). The git environment gommitlint depends on is missing or failed (working directory is not a git repository, git subprocess error).

**70**
: Internal software error (*EX_SOFTWARE*), including report-output failures. The entry-point boundary can recover panics that unwind through its own goroutine, not every goroutine panic or fatal runtime failure. It attempts a mode-0600 report under **$XDG_STATE_HOME/gommitlint/crash**, normally **~/.local/state/gommitlint/crash**. Report creation and stderr notices are best effort.

**77**
: Permission or key-policy failure (*EX_NOPERM*). Signing-key setup is incomplete — key directory missing or unreadable, no eligible keys configured, key load failure.

**78**
: Configuration error (*EX_CONFIG*). Invalid or malformed configuration file; repository rules configured but not running inside a git repository.

**130**
: Interrupted by SIGINT (Ctrl-C). `128 + 2` per POSIX convention.

**131**
: Interrupted by SIGQUIT (Ctrl-\). `128 + 3`.

**143**
: Interrupted by SIGTERM (kill command). `128 + 15`.

The panic report can contain a bounded panic value and stack with sensitive data. Review it before sharing.
The documented codes follow typed-error precedence, not numeric severity.
A valid child-process status can be preserved, including codes outside the named categories above.
A status such as **130** alone does not prove that Gommitlint itself received a signal.

## ENVIRONMENT

**NO_COLOR**
: Disable colored output when nonempty. An empty value does not disable color.

**GOMMITLINT_DEBUG**
: Enable debug logging in Git hooks when nonempty (sets --log-level=debug).

**GOMMITLINT_NO_CI_DETECT**
: Disable automatic CI environment detection when nonempty, including values such as **0**. Use **--no-ci-detect** for an explicit command-line choice.

**GOMMITLINT_ALLOW_COREDUMPS**
: Set to **1** to skip Gommitlint's core-dump suppression attempts, equivalent to **--allow-coredumps**. This does not override host policy or guarantee a core file.

**XDG_CONFIG_HOME**
: Directory for user configuration files (default: ~/.config). Configuration files are searched at $XDG_CONFIG_HOME/gommitlint/settings.yaml, settings.yml, and settings.toml.

## FILES

**.gommitlint.yaml**
: Default configuration file in current directory.

**.gommitlint.yml**
: Alternative configuration file name.

**.gommitlint.toml**
: TOML format configuration file.

**$XDG_CONFIG_HOME/gommitlint/settings.yaml**
: User configuration file (XDG Base Directory specification).

**$XDG_CONFIG_HOME/gommitlint/settings.yml**
: User configuration file (YAML).

**$XDG_CONFIG_HOME/gommitlint/settings.toml**
: User configuration file (TOML).

**<git-common-dir>/hooks/commit-msg**
: Git hook for message validation, installed by **hook install**. This is *.git/hooks/commit-msg* in a normal repository and the shared common hooks directory for linked worktrees. Local installation refuses an effective core.hooksPath redirect.

**<git-common-dir>/hooks/pre-push**
: Git hook for the configured signature policy, with the same path resolution as commit-msg.

## EXAMPLES

Validate HEAD independently of CI detection:

    gommitlint validate --ref=HEAD --no-ci-detect

Validate all commits in the current feature branch against main:

    gommitlint validate --base-branch=main

Validate a specific commit:

    gommitlint validate --ref=HEAD~1 --no-ci-detect

Validate a commit message from a file:

    gommitlint validate --message-file=commit-msg.txt

Validate a range of commits:

    gommitlint validate --range=main..feature --no-ci-detect

Validate the last 5 commits with verbose output:

    gommitlint validate --count=5 --verbose --no-ci-detect

Validate only conventional commit format:

    gommitlint validate --rule=conventional

Exclude signature metadata and key-inventory checks:

    gommitlint validate --exclude-rule=cryptosignature

After separately approving local hook changes, install commit-msg and pre-push hooks:

    gommitlint hook install

As an alternative, after reviewing user-wide effects and approving them, install global hooks:

    gommitlint hook install --global

Preview a starter without writing a configuration file:

    gommitlint config init --stdout

After review, choose one file-writing alternative. Each command below is independent:

    gommitlint config init

To create a minimal preset configuration instead:

    gommitlint config init --preset minimal

To select a different output filename:

    gommitlint config init --output custom.yaml

Retain existing configuration before explicitly approving replacement:

    gommitlint config init --force

Shell redirection from **--stdout** would bypass the command's overwrite refusal. Do not use it to replace an unreviewed file.

Show current effective configuration:

    gommitlint config show --format=yaml
    gommitlint config show --view=compact

Check configuration health:

    gommitlint config check
    gommitlint config check --format=json
    gommitlint config check --view=compact

After reviewing managed-source locations, credentials, and local destinations, refresh the selected configuration:

    gommitlint config sync

Alternatively, fetch and compare without refreshing vendored files:

    gommitlint config sync --check

Inspect current setup:

    gommitlint doctor

Print the built binary's JSON Schema without redirecting over an existing file:

    gommitlint config schema

For compact output instead:

    gommitlint config schema --compact

Check current hook installation status:

    gommitlint hook status

Output in JSON for CI/CD pipelines:

    gommitlint validate --format=json

Validate in CI with auto-detection (detects platform, format, and PR base branch):

    gommitlint validate

Output in GitHub Actions format:

    gommitlint validate --format=github

## CONFIGURATION

**gommitlint** configuration files use YAML or TOML with a "gommitlint" root key. Loading has three modes.

Example configuration (minimal):

    gommitlint:
      message:
        subject:
          max_length: 100
      rules:
        disabled: ["jirareference"]

The **branchahead** rule requires an effective base, which can come from **--base-branch** or CI detection.
Rule/check configuration and target selection are separate decisions.
An explicit parent-rule disable wins. Enabling a check can activate its parent, while disabling a check filters its diagnostics after execution.
Allowed fixup/squash/amend commits, or disabling GL901, can skip all commit rules, including identity and signature policy.

**Discovery mode merge order** (later sources override earlier):

1. Built-in defaults
2. Global config: *$XDG_CONFIG_HOME/gommitlint/settings.yaml* (or *.yml*, *.toml*)
3. Project config: *.gommitlint.yaml* in the selected repository (or *.yml*, *.toml*)

When both global and project configs exist, they are merged. Project settings override global settings. Use **gommitlint config show** to see the effective merged configuration and its sources.

**Explicit-file mode:** **--config** loads built-in defaults plus the named file and its imports. It bypasses discovered global and project files.

**Defaults-only mode:** **--ignore-config** loads built-in defaults and no configuration files. It cannot be combined with **--config**.

The canonical defaults and configuration reference live in `docs/modules/ROOT/pages/reference/configuration.adoc`.

Local configuration files may extend other local config files with `extends`. Extended files are
loaded first, and the current file overrides them.

Remote team configuration can be declared with `managed_sources` and synced explicitly with
**gommitlint config sync**. Managed sources are fetched over HTTPS, verified with a configured
SHA-256 digest, and vendored into a local file. A matching digest is not independent publisher authentication.
Ordinary configuration loading uses local copies. This is not OS-enforced network isolation for the whole process or its system-Git adapter.
Managed source URLs may be written directly as HTTPS URLs or as `${ENV_VAR}` references that are
resolved only during **gommitlint config sync**.

To troubleshoot shared config:

1. Inspect the effective config with **gommitlint config show
   --format=json**.
2. Run **gommitlint config check**.
3. After separately approving source access, use **gommitlint config sync --check** to compare vendored copies with fetched content.

To refresh one source directly, run
**gommitlint config sync --source <id>**.

**gommitlint config show --format=json** uses a version-1 inspection envelope with a source graph and selected value provenance.
Its **config** field contains settings directly, without another **gommitlint** wrapper.
YAML output instead exports sanitized settings under **gommitlint:**. It has no versioned inspection envelope or provenance fields.
The sanitized YAML export is not a credential-preserving backup.

JSON inspection metadata explains where effective settings came from, including explicit rule overrides. Managed source
inspection reports whether a source is direct or environment-based. When a setting overrides an
earlier value, provenance also shows the overridden source and value. For environment-based managed
sources, output also includes the configured source string and whether the last sync resolved it
from the environment. Provenance also explains why key values and rule states are active.
Last-sync metadata does not prove the current environment or upstream source state.

**gommitlint config check** includes grouped issue details and a managed-source summary block.
These make multi-source shared policy setups easier to troubleshoot.
When a recommended next command is available, the output includes it.
Machine-readable output includes the same recommendation as
`recommended_next_command`.

## BUGS

### Reporting Bugs

Report bugs at: <https://codefloe.com/itiquette/gommitlint/issues>

For suspected vulnerabilities, follow the private reporting procedure in **SECURITY.md** instead of opening a public issue.
Remove secrets and private repository information from shared diagnostics.

### Known Issues

Large repositories with many commits may experience slower validation times. Use **--count** or **--range** to limit the scope of validation for better performance.

## SEE ALSO

**git**(1), **git-commit**(1), **git-log**(1), **git-hook**(5)

Full documentation: <https://codefloe.com/itiquette/gommitlint>

Conventional Commits specification: <https://conventionalcommits.org>

## COPYRIGHT

Copyright © 2025 The Gommitlint Authors. Dual-licensed: choose either EUPL-1.2 <https://eupl.eu/1.2/en/> or GPL-3.0-or-later <https://www.gnu.org/licenses/gpl-3.0.html>.

This is free software: you are free to change and redistribute it. There is NO WARRANTY, to the extent permitted by law.
