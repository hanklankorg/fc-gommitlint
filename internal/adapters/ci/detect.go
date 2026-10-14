// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package ci provides automatic detection of CI/CD environments.
// It reads well-known environment variables to determine the CI platform,
// whether a pull/merge request is active, and the target branch.
package ci

const (
	envTrue = "true"

	// envGitHubBaseRef is the env var GitHub Actions exports for the PR target
	// branch. Forgejo and Gitea Actions re-export the same name for GitHub
	// workflow compatibility, so all three platforms read it here.
	envGitHubBaseRef = "GITHUB_BASE_REF"
)

// Environment holds detected CI context.
type Environment struct {
	// Platform is the output format name matching the detected CI system.
	// One of "github", "forgejo", "gitlab", or empty if not in CI.
	Platform string

	// BaseBranch is the PR/MR target branch (e.g., "main").
	// Empty when not in a PR context or when the platform doesn't provide it.
	BaseBranch string
}

// Detected reports whether a CI environment was found.
func (e Environment) Detected() bool {
	return e.Platform != ""
}

// Detect identifies the current CI environment from environment variables.
//
// Detection order matters: Forgejo is checked before GitHub because Forgejo
// sets GITHUB_ACTIONS=true for compatibility. Gitea is checked before GitHub
// for the same reason.
//
// The getEnv parameter allows dependency injection for testing.
func Detect(getEnv func(string) string) Environment {
	if getEnv("GOMMITLINT_NO_CI_DETECT") != "" {
		return Environment{}
	}

	switch {
	case getEnv("FORGEJO_ACTIONS") == envTrue:
		return Environment{
			Platform:   "forgejo",
			BaseBranch: getEnv(envGitHubBaseRef),
		}

	case getEnv("GITEA_ACTIONS") == envTrue:
		// Gitea Actions uses the same annotation format as Forgejo.
		return Environment{
			Platform:   "forgejo",
			BaseBranch: getEnv(envGitHubBaseRef),
		}

	case getEnv("GITHUB_ACTIONS") == envTrue:
		return Environment{
			Platform:   "github",
			BaseBranch: getEnv(envGitHubBaseRef),
		}

	case getEnv("GITLAB_CI") == envTrue:
		baseBranch := getEnv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME")
		if baseBranch == "" {
			baseBranch = getEnv("CI_EXTERNAL_PULL_REQUEST_TARGET_BRANCH_NAME")
		}

		return Environment{
			Platform:   "gitlab",
			BaseBranch: baseBranch,
		}
	}

	return Environment{}
}
