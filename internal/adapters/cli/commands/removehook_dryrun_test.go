// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestRemoveHookDryRun_ShowsPlannedChanges(t *testing.T) {
	tests := []struct {
		name            string
		hookType        string
		setupHook       bool
		hookContent     string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:      "no_existing_hook",
			hookType:  hookTypeCommitMsg,
			setupHook: false,
			wantContains: []string{
				"* Dry-run mode: showing what would be removed",
				"✗ No commit-msg hook found",
				"Nothing to remove",
			},
			wantNotContains: []string{
				"Would remove:",
				testOutputHookExists,
			},
		},
		{
			name:        "existing_managed_commit_msg_hook",
			hookType:    hookTypeCommitMsg,
			setupHook:   true,
			hookContent: createCommitMsgHookScript(),
			wantContains: []string{
				"* Dry-run mode",
				testOutputHookExists,
				testOutputTypeHook,
				"Action: Would remove",
				"Impact: Commit messages will no longer be automatically validated",
				"Proceed with removal: gommitlint hook remove",
			},
			wantNotContains: []string{
				"Warning: This is not a gommitlint hook",
				"Backup current hook:",
				"Impact: Configured signature validation will no longer run automatically before pushes",
			},
		},
		{
			name:        "existing_managed_pre_push_hook",
			hookType:    hookTypePrePush,
			setupHook:   true,
			hookContent: createPrePushHookScript(),
			wantContains: []string{
				"* Dry-run mode",
				testOutputHookExists,
				testOutputTypeHook,
				"Action: Would remove",
				"Impact: Configured signature validation will no longer run automatically before pushes",
				"Proceed with removal: gommitlint hook remove",
			},
			wantNotContains: []string{
				"Warning: This is not a gommitlint hook",
				"Backup current hook:",
				"Impact: Commit messages will no longer be automatically validated",
			},
		},
		{
			name:        "existing_custom_hook",
			hookType:    hookTypeCommitMsg,
			setupHook:   true,
			hookContent: "#!/bin/sh\necho 'custom validation'\necho 'more custom stuff'",
			wantContains: []string{
				"* Dry-run mode",
				testOutputHookExists,
				"Type: custom hook (not installed by gommitlint)",
				"Action: Would refuse removal",
				"Remove manually if needed: rm",
			},
			wantNotContains: []string{
				testOutputTypeHook,
				"Proceed with removal: gommitlint hook remove",
				"Would remove:",
				"Custom hook behavior will be lost",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("LC_ALL", "C")
			t.Setenv("LANG", "C")

			// Create temporary repository

			tmpDir := t.TempDir()
			gitDir := filepath.Join(tmpDir, ".git")
			hooksDir := filepath.Join(gitDir, "hooks")
			check.NoError(t, os.MkdirAll(hooksDir, 0o750))
			hookPath := filepath.Join(hooksDir, testCase.hookType)

			// Create minimal git config
			configFile := filepath.Join(gitDir, "config")
			configContent := gitConfigContent
			check.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o600))

			// Setup existing hook if needed
			if testCase.setupHook {
				check.NoError(t, os.WriteFile(hookPath, []byte(testCase.hookContent), 0o600))
			}

			var hookMode os.FileMode

			if testCase.setupHook {
				hookInfo, statErr := os.Stat(hookPath)
				check.NoError(t, statErr)

				hookMode = hookInfo.Mode()
			}

			// Create CLI command
			var buf bytes.Buffer

			cmd := &cli.Command{
				Writer: &buf,
			}

			// Execute dry-run
			err := executeRemoveHookDryRun(t.Context(), cmd, tmpDir, output.DefaultTheme, nil)
			check.NoError(t, err)

			outputStr := buf.String()

			// Check expected content
			for _, expected := range testCase.wantContains {
				check.Contains(t, outputStr, expected, "Output should contain: %s", expected)
			}

			// Check unexpected content
			for _, unexpected := range testCase.wantNotContains {
				check.NotContains(t, outputStr, unexpected, "Output should not contain: %s", unexpected)
			}

			// Verify dry-run left the hook unchanged if it was setup.
			if testCase.setupHook {
				hookContent, readErr := os.ReadFile(hookPath)
				check.NoError(t, readErr, "Hook should still exist after dry-run")
				check.Equal(t, []byte(testCase.hookContent), hookContent)

				hookInfo, statErr := os.Stat(hookPath)
				check.NoError(t, statErr)
				check.Equal(t, hookMode, hookInfo.Mode())
			}
		})
	}
}

func TestFormatManagedRemoveDryRun_DistinguishesHookOrigins(t *testing.T) {
	t.Parallel()

	inspections := map[string]apphooks.RemovalInspection{
		hookTypeCommitMsg: {
			RepoPath:     testHookRepoPath,
			HookPath:     testHookCommitMsgPath,
			HookType:     hookTypeCommitMsg,
			Exists:       true,
			Size:         1024,
			IsGommitlint: true,
		},
		hookTypePrePush: {
			RepoPath: testHookRepoPath,
			HookPath: testHookRepoPath + "/.git/hooks/pre-push",
			HookType: hookTypePrePush,
			Exists:   true,
			Size:     512,
			Content:  "#!/bin/sh\necho custom",
		},
	}

	formatted := formatManagedRemoveDryRun(inspections, false, output.DefaultTheme, nil)

	for _, expected := range []string{
		testOutputRepoPath,
		"Hook type: commit-msg",
		"Target path: /test/repo/.git/hooks/commit-msg",
		"Hook exists: /test/repo/.git/hooks/commit-msg (1024 bytes)",
		testOutputTypeHook,
		"Action: Would remove",
		"Hook type: pre-push",
		"Type: custom hook (not installed by gommitlint)",
		"Action: Would refuse removal",
		"Remove manually if needed: rm",
	} {
		check.Contains(t, formatted, expected)
	}
}

func TestFormatManagedRemoveDryRun_PluralizesCounts(t *testing.T) {
	t.Parallel()

	inspections := map[string]apphooks.RemovalInspection{
		hookTypeCommitMsg: {
			RepoPath: testHookRepoPath,
			HookPath: testHookCommitMsgPath,
			HookType: hookTypeCommitMsg,
			Exists:   true,
			Size:     1,
			Content:  "one line",
		},
		hookTypePrePush: {
			RepoPath: testHookRepoPath,
			HookPath: testHookRepoPath + "/.git/hooks/pre-push",
			HookType: hookTypePrePush,
			Exists:   true,
			Size:     2,
			Content:  "one\ntwo\nthree\nfour\nfive\nsix",
		},
	}

	formatted := formatManagedRemoveDryRun(inspections, false, output.DefaultTheme, nil)

	check.Contains(t, formatted, "(1 byte)")
	check.Contains(t, formatted, "first 1 line")
	check.Contains(t, formatted, "(2 bytes)")
	check.Contains(t, formatted, "first 5 lines")
	check.Contains(t, formatted, "(1 more line)")
}
