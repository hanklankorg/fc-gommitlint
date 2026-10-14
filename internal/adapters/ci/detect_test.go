// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package ci_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/ci"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const (
	envGithubActions = "GITHUB_ACTIONS"
	envGithubBaseRef = "GITHUB_BASE_REF"
	envGitlabCI      = "GITLAB_CI"
	envValueTrue     = "true"

	branchDevelop = "develop"
	branchMain    = "main"

	platformForgejo = "forgejo"
	platformGitlab  = "gitlab"
)

func TestDetect_IdentifiesPlatformBranchAndOptOut(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		env          map[string]string
		wantPlatform string
		wantBranch   string
		wantDetected bool
	}{
		{
			name:         "local_environment_no_ci",
			env:          map[string]string{},
			wantPlatform: "",
			wantBranch:   "",
			wantDetected: false,
		},
		{
			name: "github_actions_pull_request",
			env: map[string]string{
				envGithubActions: envValueTrue,
				envGithubBaseRef: branchMain,
			},
			wantPlatform: "github",
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			name: "github_actions_push_event",
			env: map[string]string{
				envGithubActions: envValueTrue,
				envGithubBaseRef: "",
			},
			wantPlatform: "github",
			wantBranch:   "",
			wantDetected: true,
		},
		{
			name: "forgejo_actions_pull_request",
			env: map[string]string{
				"FORGEJO_ACTIONS": envValueTrue,
				envGithubActions:  envValueTrue,
				envGithubBaseRef:  branchDevelop,
			},
			wantPlatform: platformForgejo,
			wantBranch:   branchDevelop,
			wantDetected: true,
		},
		{
			name: "gitea_actions_pull_request",
			env: map[string]string{
				"GITEA_ACTIONS":  envValueTrue,
				envGithubActions: envValueTrue,
				envGithubBaseRef: branchMain,
			},
			wantPlatform: platformForgejo,
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			name: "gitlab_ci_merge_request",
			env: map[string]string{
				envGitlabCI:                           envValueTrue,
				"CI_MERGE_REQUEST_TARGET_BRANCH_NAME": branchMain,
			},
			wantPlatform: platformGitlab,
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			name: "gitlab_ci_external_pull_request",
			env: map[string]string{
				envGitlabCI: envValueTrue,
				"CI_EXTERNAL_PULL_REQUEST_TARGET_BRANCH_NAME": branchMain,
			},
			wantPlatform: platformGitlab,
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			name: "gitlab_ci_push_event",
			env: map[string]string{
				envGitlabCI: envValueTrue,
			},
			wantPlatform: platformGitlab,
			wantBranch:   "",
			wantDetected: true,
		},
		{
			name: "opt_out_env_var",
			env: map[string]string{
				"GOMMITLINT_NO_CI_DETECT": envValueTrue,
				envGithubActions:          envValueTrue,
				envGithubBaseRef:          branchMain,
			},
			wantPlatform: "",
			wantBranch:   "",
			wantDetected: false,
		},
		{
			name: "opt_out_any_value",
			env: map[string]string{
				"GOMMITLINT_NO_CI_DETECT": "1",
				envGithubActions:          envValueTrue,
			},
			wantPlatform: "",
			wantBranch:   "",
			wantDetected: false,
		},
		{
			// Cross-contamination. Nothing checked that a platform reads only
			// its OWN branch variable, so a GitLab branch lookup that fell
			// back to GITHUB_BASE_REF — or a GitHub one that reached for
			// GitLab's — would have gone unnoticed. Forgejo and Gitea are
			// excluded from this claim on purpose: they re-export
			// GITHUB_BASE_REF deliberately, which the product documents.
			name: "gitlab_ignores_the_github_base_ref",
			env: map[string]string{
				envGitlabCI:      envValueTrue,
				envGithubBaseRef: branchDevelop,
			},
			wantPlatform: platformGitlab,
			wantBranch:   "",
			wantDetected: true,
		},
		{
			name: "github_ignores_the_gitlab_target_branch",
			env: map[string]string{
				envGithubActions:                      envValueTrue,
				"CI_MERGE_REQUEST_TARGET_BRANCH_NAME": branchDevelop,
			},
			wantPlatform: "github",
			wantBranch:   "",
			wantDetected: true,
		},
		{
			// The last untested pair in the precedence chain. Forgejo and
			// Gitea are already pinned above GitHub; GitHub above GitLab was
			// only implied by the order of the switch.
			name: "github_preferred_over_gitlab",
			env: map[string]string{
				envGithubActions: envValueTrue,
				envGitlabCI:      envValueTrue,
				envGithubBaseRef: branchMain,
			},
			wantPlatform: "github",
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			// The opt-out boundary. getEnv cannot distinguish unset from
			// empty, so empty MUST mean "not opted out" — otherwise detection
			// would be off for everyone. "opt_out_any_value" pins that a
			// non-"true" value still opts out; this pins the other end.
			name: "empty_opt_out_value_does_not_opt_out",
			env: map[string]string{
				"GOMMITLINT_NO_CI_DETECT": "",
				envGithubActions:          envValueTrue,
				envGithubBaseRef:          branchMain,
			},
			wantPlatform: "github",
			wantBranch:   branchMain,
			wantDetected: true,
		},
		{
			// Platform flags are matched against "true" exactly, unlike the
			// opt-out which takes any non-empty value. The two rules sit three
			// lines apart in the product and nothing distinguished them.
			name: "a_platform_flag_that_is_not_true_is_not_ci",
			env: map[string]string{
				envGithubActions: "1",
				envGitlabCI:      "yes",
			},
			wantPlatform: "",
			wantBranch:   "",
			wantDetected: false,
		},
		{
			name: "gitlab_mr_preferred_over_external",
			env: map[string]string{
				envGitlabCI:                                   envValueTrue,
				"CI_MERGE_REQUEST_TARGET_BRANCH_NAME":         branchMain,
				"CI_EXTERNAL_PULL_REQUEST_TARGET_BRANCH_NAME": branchDevelop,
			},
			wantPlatform: platformGitlab,
			wantBranch:   branchMain,
			wantDetected: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			env := testCase.env
			getEnv := func(key string) string {
				return env[key]
			}

			result := ci.Detect(getEnv)

			check.Equal(t, testCase.wantPlatform, result.Platform, "platform mismatch")
			check.Equal(t, testCase.wantBranch, result.BaseBranch, "base branch mismatch")
			check.Equal(t, testCase.wantDetected, result.Detected(), "detected mismatch")
		})
	}
}

func TestDetect_DetectedPlatformsAreValidOutputFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "github", env: map[string]string{envGithubActions: envValueTrue}},
		{name: "forgejo", env: map[string]string{"FORGEJO_ACTIONS": envValueTrue}},
		{name: "gitlab", env: map[string]string{envGitlabCI: envValueTrue}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detected := ci.Detect(func(key string) string { return testCase.env[key] })
			check.True(t, output.IsValidFormat(detected.Platform),
				"detected CI platform %q must select a supported output format", detected.Platform)
		})
	}
}
