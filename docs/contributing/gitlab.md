# GitLab merge-request acceptance

The [merge-request configuration](../../examples/gitlab/merge-request.yml)
runs Gommitlint's checks through Forge CI's pinned GitLab `check` component.
It keeps the lint/security floor, schema and translation checks, version-stamp
comparison, existing Forgejo release-policy checks, race tests and tagged
compile checks in one clean job.

## Run against Forge Lab

Use a clean throwaway checkout. Copy the example to `.gitlab-ci.yml` and commit
that preparation, then run the engine's documented recipe:

```text
just test-live-gitlab-consumer /absolute/gommitlint-trial /absolute/job-image.tar
```

The prepared Docker image archive must provide Go, Nanolinter and its check
tools, GCC/libc headers, gettext, Git and OpenSSH. The existing
`Containerfile.ci` describes Gommitlint's image additions. The trial replaces
only the component coordinates and the `job-image: $CI_JOB_IMAGE` marker with
its reviewed engine commit and uploaded image digest. It preserves the source
tree and check commands and creates an ephemeral signed MR change.

For SHA-256 repositories, the lab administrator must enable GitLab's
`support_sha256_repositories` feature for the test account before creation.
The job declares `CHECKOUT_OBJECT_FORMAT=sha256` explicitly. Cold compiler and
scanner caches need more than two GiB of scratch space: the example requests
four GiB and limits usage to eight. The runner must admit those native storage
overrides; this does not require privileged jobs.

## Adopt on another GitLab instance

Host the reviewed engine component on that instance and publish its recorded
binary. Set `engine-project` and the component coordinate to that project,
keep a full commit pin, and replace the image marker with a published,
digest-pinned image providing the same tools. The marker is a trial input,
not a published image reference.

Require successful pipelines in project settings, disallow merging on skipped
pipelines, and protect the target branch. Keep release credentials protected
and scoped to their signing environment. Included components are merged into
the caller's configuration, so independent release policy and signing jobs
remain separate integration work.

The isolation command in the example checks the existing **Forgejo** release
workflow. It does not establish GitLab release isolation. Native GitLab release
and signing acceptance has not been completed.

## Recorded result

This historical result predates the Forge CI naming migration. It is not
acceptance evidence for the current local engine candidate.

On 2026-10-08, the complete trial passed in **953.909 seconds** against local
GitLab, using Gommitlint source `9f69975` with the prepared MR configuration
and the engine at `b5f496b7`. The job was unprivileged, with one CPU, one GiB
RAM and eight GiB scratch storage.

- A signing-key canary available to the check job failed before checkout;
  GitLab blocked the MR on failed pipeline **2423**.
- After protecting and scoping the canary to `release-signing`, pipeline
  **2424** passed the complete gate on the same SHA-256 MR source commit.
- The harness checked both the MR event/source identity and the project's
  successful-pipeline-required / skipped-pipeline-refusal settings.

The retained engine evidence is `20261008T204325Z-2476096/live-run.log` in the
original run's state directory (30-day retention). New Forge CI runs use
`$XDG_STATE_HOME/forge-ci/live-runs`.
