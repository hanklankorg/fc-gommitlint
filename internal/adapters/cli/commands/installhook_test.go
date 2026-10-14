// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
)

func TestExecuteInstallHook_QuietSuppressesSuccessOutput(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	isolateGlobalGitConfig(t)

	var stdout bytes.Buffer

	root := hookCommandTestRoot(repo, &stdout)

	err := root.Run(t.Context(), []string{"gommitlint", "--quiet", commandNameHook, commandNameHookInstall})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.FileExists(t, filepath.Join(repo, ".git", "hooks", hookTypeCommitMsg))
}

func TestExecuteInstallGlobalHook_QuietSuppressesSuccessOutput(t *testing.T) {
	isolateGlobalGitConfig(t)

	var stdout bytes.Buffer

	root := hookCommandTestRoot("", &stdout)

	err := root.Run(t.Context(), []string{"gommitlint", "--quiet", commandNameHook, commandNameHookInstall, "--global"})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.FileExists(t, apphooks.GlobalHookPluginScriptPath(GlobalHooksPath(), hookTypeCommitMsg))
}

func TestExecuteInstallHook_ValidatesPresentationBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		arg  string
	}{
		{name: "invalid theme", arg: "--theme=invalid"},
		{name: "invalid color", arg: "--color=invalid"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repo := t.TempDir()
			gitIn(t, repo, "init", "-q")
			isolateGlobalGitConfig(t)

			root := hookCommandTestRoot(repo, io.Discard)

			err := root.Run(t.Context(), []string{"gommitlint", testCase.arg, commandNameHook, commandNameHookInstall})

			check.Error(t, err)

			_, statErr := os.Stat(filepath.Join(repo, ".git", "hooks", hookTypeCommitMsg))
			check.True(t, errors.Is(statErr, os.ErrNotExist), "invalid presentation option must not install hooks")
		})
	}
}

func TestExecuteInstallHook_PreflightsAllHooksBeforeMutation(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	isolateGlobalGitConfig(t)

	prePushPath := filepath.Join(repo, ".git", "hooks", hookTypePrePush)
	check.NoError(t, os.WriteFile(prePushPath, []byte("#!/bin/sh\necho custom"), 0o755))

	root := hookCommandTestRoot(repo, io.Discard)

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, commandNameHookInstall})

	check.ErrorIs(t, err, ErrCustomHookExists)

	_, statErr := os.Stat(filepath.Join(repo, ".git", "hooks", hookTypeCommitMsg))
	check.True(t, errors.Is(statErr, os.ErrNotExist), "failed batch preflight must not install an earlier hook")
}

func TestExecuteInstallHook_LoadsPolicyBeforeMutation(t *testing.T) {
	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	isolateGlobalGitConfig(t)
	configPath := filepath.Join(t.TempDir(), "gommitlint.yaml")
	check.NoError(t, os.WriteFile(configPath, []byte("rules: ["), 0o600))

	root := hookCommandTestRoot(repo, io.Discard)

	err := root.Run(t.Context(), []string{"gommitlint", "--config=" + configPath, commandNameHook, commandNameHookInstall})

	check.Error(t, err)

	_, statErr := os.Stat(filepath.Join(repo, ".git", "hooks", hookTypeCommitMsg))
	check.True(t, errors.Is(statErr, os.ErrNotExist), "invalid policy config must not install hooks")
}

func TestFormatManagedHookInstallResults_SanitizesPaths(t *testing.T) {
	t.Parallel()

	results := map[string]HookInstallResult{}
	for _, hookType := range apphooks.DefaultHookTypes() {
		results[hookType] = HookInstallResult{
			RepoPath: "/repo\nforged\x1b]2;title\a",
			HookPath: "/repo/hooks/" + hookType + "\nforged\x1b]52;c;x\a",
			HookType: hookType,
		}
	}

	formatted := formatManagedHookInstallResults(results, false, output.DefaultTheme, nil, hookInstallPolicy{})

	check.NotContains(t, formatted, "\x1b")
	check.NotContains(t, formatted, "\a")
	check.NotContains(t, formatted, "\nforged")
}

func TestFormatGlobalHookInstallResult_SanitizesPaths(t *testing.T) {
	t.Parallel()

	hostile := "/hooks\nforged\x1b]52;c;x\a"
	formatted := formatGlobalHookInstallResult(GlobalInstallResult{
		HooksPath:         hostile,
		HookTypes:         apphooks.DefaultHookTypes(),
		HooksPathAction:   apphooks.HooksPathReplaced,
		PreviousHooksPath: hostile,
	}, false, output.DefaultTheme, nil)

	check.NotContains(t, formatted, "\x1b")
	check.NotContains(t, formatted, "\a")
	check.NotContains(t, formatted, "\nforged")
}

func TestHookScript_DefaultHasCorrectStructure(t *testing.T) {
	t.Parallel()

	script := createCommitMsgHookScript()

	// Verify script structure
	check.Contains(t, script, "#!/bin/sh", "script should have shebang")
	check.Contains(t, script, "gommitlint commit-msg hook", "script should have description")
	check.Contains(t, script, "git commit --no-verify", "script should mention bypass option")

	// Verify environment variables are documented
	check.Contains(t, script, "GOMMITLINT_DEBUG=1", "script should document debug env var")
	check.Contains(t, script, "NO_COLOR=1", "script should document no-color env var")

	// Verify error handling
	check.Contains(t, script, "set -e", "script should exit on error")
	check.Contains(t, script, "command -v gommitlint", "script should check for gommitlint")
	check.Contains(t, script, "gommitlint not found in PATH", "script should have helpful error message")

	// Verify command building
	check.Contains(t, script, `FLAGS=""`, "script should initialize flags")
	check.Contains(t, script, "--color=never", "script should handle no-color")
	check.Contains(t, script, "--log-level=debug", "script should handle debug mode")

	// Verify validation command
	check.Contains(t, script, `"$GOMMITLINT" validate --message-file=`, "script should run validation through the resolved binary")
	check.Contains(t, script, "GOMMITLINT_HOOK_MISSING=skip", "script should document the missing-binary opt-out")
	check.NotContains(t, script, "@GOMMITLINT_INSTALLED_BIN@", "installed binary placeholder must be substituted")
	check.Contains(t, script, "$FLAGS", "script should use built flags")

	// Verify error messages
	check.Contains(t, script, "commit-msg failed", "script should have rejection message")
	check.Contains(t, script, "gommitlint config show", "script should suggest showing config")
	check.Contains(t, script, "[gommitlint]", "script should identify gommitlint as the source")
}

func TestInstallSingleHook_ValidationLogic(t *testing.T) {
	tests := []struct {
		name         string
		force        bool
		existingHook string // "" = no hook, "gommitlint" = gommitlint hook, "custom" = custom hook
		wantErr      bool
		errContains  string
	}{
		{
			name:         "no existing hook",
			force:        false,
			existingHook: "",
			wantErr:      false,
		},
		{
			name:         "existing gommitlint hook without force",
			force:        false,
			existingHook: gommitlintKey,
			wantErr:      true,
			errContains:  "hook already exists",
		},
		{
			name:         "existing gommitlint hook with force",
			force:        true,
			existingHook: gommitlintKey,
			wantErr:      false,
		},
		{
			name:         "existing custom hook without force",
			force:        false,
			existingHook: testValueCustom,
			wantErr:      true,
			errContains:  "custom hook exists",
		},
		{
			name:         "existing custom hook with force",
			force:        true,
			existingHook: testValueCustom,
			wantErr:      true,
			errContains:  "custom hook exists",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Create a test environment with real filesystem (needed for git operations)
			env := testhelpers.NewRealFSTestEnv(t)
			tmpDir := env.RepoPath
			gitDir := env.GitDir()
			hooksDir := env.HooksDir()

			// Create a minimal git config file
			configFile := filepath.Join(gitDir, "config")
			configContent := gitConfigContent
			env.FS.WriteFile(configFile, []byte(configContent))

			hookPath := filepath.Join(hooksDir, hookTypeCommitMsg)

			// Setup existing hook if needed
			var existing []byte

			switch testCase.existingHook {
			case "gommitlint":
				existing = []byte("#!/bin/sh\n# Generated by gommitlint\ngommitlint validate")
			case "custom":
				existing = []byte("#!/bin/sh\n# Custom hook\necho 'custom'")
			}

			if existing != nil {
				env.FS.WriteFile(hookPath, existing)
			}

			// Test via installSingleHookWithFS which contains the validation logic
			_, err := installSingleHookWithFS(t.Context(), filesystem.NewOSFileSystem(), testCase.force, tmpDir, hookTypeCommitMsg, createCommitMsgHookScript(), nil)
			if testCase.wantErr {
				check.Error(t, err)

				errorMsg := err.Error()
				check.Contains(t, errorMsg, testCase.errContains)

				// A refusal must leave the existing hook byte for byte as it was.
				content, readErr := os.ReadFile(hookPath)
				check.NoError(t, readErr)
				check.Equal(t, string(existing), string(content))
			} else {
				check.NoError(t, err)
			}
		})
	}
}

func TestFormatManagedHookInstallResults_ReportsInstalledHooks(t *testing.T) {
	t.Parallel()

	results := map[string]HookInstallResult{
		hookTypeCommitMsg: {
			RepoPath: testHookRepoPath,
			HookPath: testHookCommitMsgPath,
			HookType: hookTypeCommitMsg,
		},
		hookTypePrePush: {
			RepoPath:       testHookRepoPath,
			HookPath:       testHookRepoPath + "/.git/hooks/pre-push",
			HookType:       hookTypePrePush,
			WasOverwritten: true,
		},
	}

	formatted := formatManagedHookInstallResults(results, false, output.DefaultTheme, nil, hookInstallPolicy{SignoffRequired: true})

	for _, expected := range []string{
		testOutputRepoPath,
		"Installing git hooks",
		"commit-msg hook:",
		"Installed: /test/repo/.git/hooks/commit-msg",
		"pre-push hook:",
		"Installed: /test/repo/.git/hooks/pre-push",
		"Overwritten: previous hook replaced",
		"Git hooks installed successfully",
		testOutputNextSteps,
		"Test: git commit --signoff -m \"feat: your message\"",
		"Status: gommitlint hook status",
	} {
		check.Contains(t, formatted, expected)
	}
}

func TestHookInstallPolicy_TestCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy hookInstallPolicy
		want   string
	}{
		{
			name:   "neither_signoff_nor_signature_required",
			policy: hookInstallPolicy{},
			want:   "git commit -m \"feat: your message\"",
		},
		{
			name:   "signoff_required",
			policy: hookInstallPolicy{SignoffRequired: true},
			want:   "git commit --signoff -m \"feat: your message\"",
		},
		{
			name:   "signature_required",
			policy: hookInstallPolicy{SignatureRequired: true},
			want:   "git commit -S -m \"feat: your message\"",
		},
		{
			name:   "both_required",
			policy: hookInstallPolicy{SignoffRequired: true, SignatureRequired: true},
			want:   "git commit --signoff -S -m \"feat: your message\"",
		},
		{
			name:   "multiple_signoffs_required",
			policy: hookInstallPolicy{SignoffRequired: true, MinSignoffCount: 2},
			want:   "git commit --signoff -m \"feat: your message\" -m 'Signed-off-by: Signer 2 <signer2@example.com>'",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, testCase.policy.TestCommand())
		})
	}
}

func TestInstallHook_CreatesCommitMsgHook(t *testing.T) {
	t.Parallel()

	// Test the components that ExecuteInstallHook uses for better coverage
	// without complex CLI mocking

	t.Run("installSingleHook error handling", func(t *testing.T) {
		t.Parallel()

		_, err := installSingleHook(t.Context(), false, "/nonexistent/repo/path", hookTypeCommitMsg, createCommitMsgHookScript(), nil)

		// Should return an error for invalid repo path
		check.Error(t, err, "should error for nonexistent repo path")
		check.ErrorIs(t, err, fileops.ErrRepositoryPathNotExist)
	})
}
