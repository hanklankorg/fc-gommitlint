// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"github.com/urfave/cli/v3"
)

func TestHookStatus_ReportsCurrentState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		hookName       string
		setupHook      bool
		hookContent    string
		executable     bool
		wantExists     bool
		wantExecutable bool
		wantGommitlint bool
	}{
		{
			name:           "non-existent hook",
			hookName:       hookTypeCommitMsg,
			setupHook:      false,
			wantExists:     false,
			wantExecutable: false,
			wantGommitlint: false,
		},
		{
			name:           "gommitlint hook executable",
			hookName:       hookTypeCommitMsg,
			setupHook:      true,
			hookContent:    createCommitMsgHookScript(),
			executable:     true,
			wantExists:     true,
			wantExecutable: true,
			wantGommitlint: true,
		},
		{
			name:           "gommitlint hook not executable",
			hookName:       hookTypeCommitMsg,
			setupHook:      true,
			hookContent:    createCommitMsgHookScript(),
			executable:     false,
			wantExists:     true,
			wantExecutable: false,
			wantGommitlint: true,
		},
		{
			name:           "custom hook",
			hookName:       "pre-commit",
			setupHook:      true,
			hookContent:    "#!/bin/sh\necho 'custom validation'",
			executable:     true,
			wantExists:     true,
			wantExecutable: true,
			wantGommitlint: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			hookPath := filepath.Join(tmpDir, testCase.hookName)

			if testCase.setupHook {
				perm := os.FileMode(0o600)
				if testCase.executable {
					perm = 0o700
				}

				check.NoError(t, os.WriteFile(hookPath, []byte(testCase.hookContent), perm))
			}

			fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
			service := defaultLocalHookService(t.Context())
			service.Store = hookStore{fs: filesystem.NewOSFileSystem(), fileOps: fileOps}
			status, err := service.HookStatus(testCase.hookName, hookPath, "")
			check.NoError(t, err)

			check.Equal(t, testCase.hookName, status.Name)
			check.Equal(t, hookPath, status.Local.Path)
			check.Equal(t, testCase.wantExists, status.Local.Exists)
			check.Equal(t, testCase.wantExecutable, status.Local.IsExecutable)
			check.Equal(t, testCase.wantGommitlint, status.Local.IsGommitlint)
			check.Empty(t, status.Global.Path)
			check.False(t, status.Global.Exists)

			if testCase.setupHook {
				check.Equal(t, testCase.hookContent, status.Local.Content)
				check.NotZero(t, status.Local.Size)
				check.NotZero(t, status.Local.ModifiedTime)
			}
		})
	}
}

func TestRepositoryHookStatus_ReportsCurrentState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		setupGitRepo    bool
		setupConfig     bool
		setupHooks      map[string]string // hookName -> content
		wantErr         bool
		expectedErrCode diagnostics.Reason
	}{
		{
			name:            "non-git repository",
			setupGitRepo:    false,
			wantErr:         true,
			expectedErrCode: diagnostics.ReasonInvalidRepo,
		},
		{
			name:         "empty git repository",
			setupGitRepo: true,
			setupConfig:  false,
			setupHooks:   map[string]string{},
			wantErr:      false,
		},
		{
			name:         "repository with config",
			setupGitRepo: true,
			setupConfig:  true,
			setupHooks: map[string]string{
				hookTypeCommitMsg: createCommitMsgHookScript(),
			},
			wantErr: false,
		},
		{
			name:         "repository with multiple hooks",
			setupGitRepo: true,
			setupConfig:  false,
			setupHooks: map[string]string{
				hookTypeCommitMsg: createCommitMsgHookScript(),
				"pre-commit":      "#!/bin/sh\necho 'pre-commit'",
				hookTypePrePush:   "#!/bin/sh\necho 'pre-push'",
			},
			wantErr: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()

			if testCase.setupGitRepo {
				gitDir := filepath.Join(tmpDir, ".git")
				hooksDir := filepath.Join(gitDir, "hooks")
				check.NoError(t, os.MkdirAll(hooksDir, 0o750))

				// Create minimal git config
				configFile := filepath.Join(gitDir, "config")
				configContent := gitConfigContent
				check.NoError(t, os.WriteFile(configFile, []byte(configContent), 0o600))

				// Setup hooks
				for hookName, content := range testCase.setupHooks {
					hookPath := filepath.Join(hooksDir, hookName)
					check.NoError(t, os.WriteFile(hookPath, []byte(content), 0o600))
				}

				// Setup config if requested
				if testCase.setupConfig {
					configPath := filepath.Join(tmpDir, testGommitlintConfigFile)
					configContent := "rules:\n  conventional: enabled\n"
					check.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))
				}
			}

			service := newLocalHookServiceWithRedirectCheck(t.Context(), filesystem.NewOSFileSystem(), nil)
			status, err := service.Status(t.Context(), tmpDir)

			if testCase.wantErr {
				err = appHookStatusError(err, nil)
				check.Error(t, err)

				var validationErr domain.ValidationError
				check.ErrorAs(t, err, &validationErr)
				check.Equal(t, testCase.expectedErrCode, validationErr.Diagnostic.Reason)

				return
			}

			check.NoError(t, err)
			check.Equal(t, tmpDir, status.RepoPath)

			if testCase.setupGitRepo {
				check.Equal(t, filepath.Join(tmpDir, ".git"), status.GitDir)
				check.Equal(t, filepath.Join(tmpDir, ".git", "hooks"), status.HooksDir)

				// Verify all expected hooks are checked
				expectedHooks := []string{hookTypeCommitMsg, hookTypePrePush}
				check.Len(t, status.Hooks, len(expectedHooks))

				for hookIndex, hookName := range expectedHooks {
					check.Equal(t, hookName, status.Hooks[hookIndex].Name)

					if content, exists := testCase.setupHooks[hookName]; exists {
						check.True(t, status.Hooks[hookIndex].Local.Exists)
						check.Equal(t, content, status.Hooks[hookIndex].Local.Content)
						check.Equal(t, strings.Contains(content, "gommitlint"), status.Hooks[hookIndex].Local.IsGommitlint)
					} else {
						check.False(t, status.Hooks[hookIndex].Local.Exists)
					}
				}
			}
		})
	}
}

func TestHookStatus_ReportsGlobalInstallWithoutLocalHooks(t *testing.T) {
	// Avoid t.Parallel() due to global git config env usage
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")
	gitDir := filepath.Join(repoDir, ".git")
	globalHooksDir := filepath.Join(tempDir, "global-hooks")

	check.NoError(t, os.MkdirAll(gitDir, 0o750))
	configFile := filepath.Join(gitDir, "config")
	check.NoError(t, os.WriteFile(configFile, []byte(gitConfigContent), 0o600))

	commitMsgDir := filepath.Join(globalHooksDir, "commit-msg.d")
	check.NoError(t, os.MkdirAll(commitMsgDir, 0o750))
	commitMsgPath := filepath.Join(commitMsgDir, "50-gommitlint.sh")
	check.NoError(t, os.WriteFile(commitMsgPath, []byte(createCommitMsgPluginScript()), 0o755))

	prePushDir := filepath.Join(globalHooksDir, "pre-push.d")
	check.NoError(t, os.MkdirAll(prePushDir, 0o750))
	prePushPath := filepath.Join(prePushDir, "50-gommitlint.sh")
	check.NoError(t, os.WriteFile(prePushPath, []byte(createPrePushPluginScript()), 0o755))

	gitConfigPath := filepath.Join(tempDir, "gitconfig")
	check.NoError(t, os.WriteFile(gitConfigPath, []byte("[core]\n\thooksPath = "+globalHooksDir+"\n"), 0o600))
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfigPath)

	service := newLocalHookServiceWithRedirectCheck(t.Context(), filesystem.NewOSFileSystem(), nil)
	status, err := service.Status(t.Context(), repoDir)
	check.NoError(t, err)
	check.Len(t, status.Hooks, 2)
	check.True(t, status.Hooks[0].Global.Exists)
	check.Contains(t, formatHookStatus(status, false, nil), "installed (global)")
}

func TestFormatHookStatus(t *testing.T) {
	t.Parallel()

	// Create test status with only managed hooks
	now := time.Now()
	status := RepositoryHookStatus{
		RepoPath: testHookRepoPath,
		GitDir:   testHookGitDir,
		HooksDir: testHookHooksDir,
		Hooks: []HookStatus{
			{
				Name: hookTypeCommitMsg,
				Local: HookFileStatus{
					Path:         testHookCommitMsgPath,
					Exists:       true,
					IsExecutable: true,
					IsGommitlint: true,
					ModifiedTime: now,
					Size:         1024,
					Content:      "Generated by gommitlint hook install command",
				},
			},
			{
				Name: hookTypePrePush,
				Local: HookFileStatus{
					Path:         testHookPrePushPath,
					Exists:       true,
					IsExecutable: true,
					IsGommitlint: true,
					ModifiedTime: now,
					Size:         512,
					Content:      "Generated by gommitlint hook install command",
				},
			},
		},
	}

	output := formatHookStatus(status, false, nil)

	// Verify header
	check.Contains(t, output, "Gommitlint Git Hooks:")

	// Verify hook status
	check.Contains(t, output, "✓ commit-msg")
	check.Contains(t, output, "installed")
	check.Contains(t, output, "✓ pre-push")
	check.Contains(t, output, "installed")
}

func TestFormatHookStatus_NoGommitlintHook(t *testing.T) {
	t.Parallel()

	// Create test status without gommitlint hook
	status := RepositoryHookStatus{
		RepoPath: testHookRepoPath,
		GitDir:   testHookGitDir,
		HooksDir: testHookHooksDir,
		Hooks: []HookStatus{
			{
				Name:  hookTypeCommitMsg,
				Local: HookFileStatus{Path: testHookCommitMsgPath},
			},
		},
	}

	output := formatHookStatus(status, false, nil)

	// Verify hook status
	check.Contains(t, output, "Gommitlint Git Hooks:")
	check.Contains(t, output, "- commit-msg")
	check.Contains(t, output, "not installed")
}

func TestFormatHookStatus_NoHooksDirectory(t *testing.T) {
	t.Parallel()

	// Create test status with no hooks directory
	status := RepositoryHookStatus{
		RepoPath: testHookRepoPath,
		GitDir:   testHookGitDir,
		HooksDir: testHookHooksDir,
		Hooks:    []HookStatus{},
	}

	output := formatHookStatus(status, false, nil)

	// Verify no hooks message
	check.Contains(t, output, "No hooks directory found")
}

func TestFormatHookStatus_LocalAndGlobalHooks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		localInstalled  bool
		globalInstalled bool
		wantContains    string
	}{
		{
			name:            "local only",
			localInstalled:  true,
			globalInstalled: false,
			wantContains:    "installed (local)",
		},
		{
			name:            "global only",
			localInstalled:  false,
			globalInstalled: true,
			wantContains:    "installed (global)",
		},
		{
			name:            "both local and global",
			localInstalled:  true,
			globalInstalled: true,
			wantContains:    "installed (local + global)",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			status := RepositoryHookStatus{
				RepoPath: testHookRepoPath,
				GitDir:   testHookGitDir,
				HooksDir: testHookHooksDir,
				Hooks: []HookStatus{
					{
						Name: hookTypeCommitMsg,
						Local: HookFileStatus{
							Path:         testHookCommitMsgPath,
							Exists:       testCase.localInstalled,
							IsExecutable: testCase.localInstalled,
							IsGommitlint: testCase.localInstalled,
						},
						Global: HookFileStatus{
							Path:         "/global/commit-msg.d/50-gommitlint.sh",
							Exists:       testCase.globalInstalled,
							IsExecutable: testCase.globalInstalled,
							IsGommitlint: testCase.globalInstalled,
						},
					},
				},
			}

			output := formatHookStatus(status, false, nil)

			check.Contains(t, output, testCase.wantContains)
		})
	}
}

func TestFormatHookStatus_DistinguishesMixedHooksAndShowsBothVerboseRecords(t *testing.T) {
	t.Parallel()

	localModified := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	globalModified := time.Date(2026, 5, 15, 11, 0, 0, 0, time.UTC)
	status := RepositoryHookStatus{Hooks: []HookStatus{{
		Name: hookTypeCommitMsg,
		Local: HookFileStatus{
			Path:         testHookCommitMsgPath,
			Exists:       true,
			IsExecutable: true,
			IsGommitlint: false,
			ModifiedTime: localModified,
			Size:         23,
			Content:      "#!/bin/sh\necho custom",
		},
		Global: HookFileStatus{
			Path:         "/global/commit-msg.d/50-gommitlint.sh",
			Exists:       true,
			IsExecutable: true,
			IsGommitlint: true,
			ModifiedTime: globalModified,
			Size:         49,
			Content:      "#!/bin/sh\n# Generated by: gommitlint hook install --global",
		},
	}}}

	formatted := formatHookStatus(status, true, nil)

	check.Contains(t, formatted, "? commit-msg   unknown source (local) + installed (global)")
	check.Equal(t, 1, strings.Count(formatted, "? commit-msg"))
	check.Contains(t, formatted, "      local:\n      Path: "+testHookCommitMsgPath)
	check.Contains(t, formatted, "      Modified: 2026-05-15 10:00:00")
	check.Contains(t, formatted, "      Size: 23 bytes")
	check.Contains(t, formatted, "      Preview: echo custom")
	check.Contains(t, formatted, "      global:\n      Path: /global/commit-msg.d/50-gommitlint.sh")
	check.Contains(t, formatted, "      Modified: 2026-05-15 11:00:00")
	check.Contains(t, formatted, "      Size: 49 bytes")
	check.Contains(t, formatted, "      Preview: Generated by: gommitlint hook install --global")
}

func TestFormatHookStatus_SanitizesVerboseRepositoryData(t *testing.T) {
	t.Parallel()

	status := RepositoryHookStatus{Hooks: []HookStatus{{
		Name: hookTypeCommitMsg,
		Local: HookFileStatus{
			Path:         "/repo\nforged\x1b]52;c;payload\a",
			Exists:       true,
			IsExecutable: true,
			IsGommitlint: true,
			Content:      "#!/bin/sh\n# status\x1b]2;forged\a",
		},
	}}}

	formatted := formatHookStatus(status, true, nil)

	check.NotContains(t, formatted, "\x1b")
	check.NotContains(t, formatted, "\a")
	check.NotContains(t, formatted, "\nforged")
}

func TestGetContentPreview_BoundsUnicodeComments(t *testing.T) {
	t.Parallel()

	preview := getContentPreview("# " + strings.Repeat("é", 100))

	check.True(t, utf8.ValidString(preview))
	check.LessOrEqual(t, len([]rune(preview)), 60)
}

func TestExecuteHookStatus_PropagatesWriterFailure(t *testing.T) {
	repo := hookStatusTestRepository(t)
	root := hookStatusTestCommand(repo, failingWriter{err: errMockWrite})

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, commandNameHookStatus})

	check.ErrorIs(t, err, ErrOutputWrite)
	check.ErrorIs(t, err, errMockWrite)
}

func TestExecuteHookStatus_QuietSuppressesSuccessOutput(t *testing.T) {
	repo := hookStatusTestRepository(t)

	var stdout bytes.Buffer

	root := hookStatusTestCommand(repo, &stdout)

	err := root.Run(t.Context(), []string{"gommitlint", "--quiet", commandNameHook, commandNameHookStatus})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
}

func TestExecuteHookStatus_PreservesInvalidRepositoryRecovery(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := hookStatusTestCommand(repo, io.Discard)

	err := root.Run(t.Context(), []string{"gommitlint", commandNameHook, commandNameHookStatus})

	check.ErrorIs(t, err, fileops.ErrNotGitRepository)

	var recovery RecoveryError
	check.ErrorAs(t, err, &recovery)
	check.Equal(t, "invalid repository path: %s", recovery.Msg.ID)
}

func hookStatusTestRepository(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	gitIn(t, repo, "init", "-q")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	return repo
}

func hookStatusTestCommand(repo string, writer io.Writer) *cli.Command {
	return &cli.Command{
		Name:   gommitlintKey,
		Writer: writer,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flagNameRepoPath, Value: repo},
			&cli.BoolFlag{Name: flags.Quiet},
		},
		Commands: []*cli.Command{{
			Name:     commandNameHook,
			Commands: []*cli.Command{NewHookStatusCommand(nil)},
		}},
	}
}
