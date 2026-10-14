// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestDoctor_FormatReport_DefaultPolicy(t *testing.T) {
	t.Parallel()

	report := doctorReport{
		ConfigSource: "built-in defaults",
		Hooks: RepositoryHookStatus{
			RepoPath: "/repo",
			Hooks: []HookStatus{
				{
					Name: hookTypeCommitMsg,
					Local: HookFileStatus{
						Exists:       true,
						IsExecutable: true,
						IsGommitlint: true,
						ModifiedTime: time.Unix(1, 0),
					},
				},
				{Name: hookTypePrePush},
			},
		},
		Policy: hookInstallPolicy{SignoffRequired: true},
	}

	output := formatDoctorReport(report, nil)

	check.Contains(t, output, "Gommitlint Doctor")
	check.Contains(t, output, "Repository: /repo")
	check.Contains(t, output, "Configuration: built-in defaults")
	check.Contains(t, output, "Warnings: 0")
	check.Contains(t, output, "commit-msg   installed (local)")
	check.Contains(t, output, "pre-push     not installed")
	check.Contains(t, output, "sign-off required: yes")
	check.Contains(t, output, "signature required: no")
	check.Contains(t, output, "test command: git commit --signoff -m \"feat: your message\"")
	check.Contains(t, output, "install hooks: gommitlint hook install")
	check.Contains(t, output, "validate commits: gommitlint validate")
}

func TestDoctor_FormatReport_SignaturePolicy(t *testing.T) {
	t.Parallel()

	report := doctorReport{
		ConfigSource: "custom.yaml",
		Hooks: RepositoryHookStatus{
			RepoPath: "/repo",
			Hooks: []HookStatus{
				{
					Name:  hookTypeCommitMsg,
					Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true},
				},
				{
					Name:  hookTypePrePush,
					Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true},
				},
			},
		},
		Policy: hookInstallPolicy{SignoffRequired: true, SignatureRequired: true},
	}

	output := formatDoctorReport(report, nil)

	check.Contains(t, output, "signature required: yes")
	check.Contains(t, output, "test command: git commit --signoff -S -m \"feat: your message\"")
	check.NotContains(t, output, "install hooks: gommitlint hook install")
}

func TestDoctor_FormatReport_SanitizesTerminalPaths(t *testing.T) {
	t.Parallel()

	report := doctorReport{
		ConfigSource: "config\x1b[31m\nforged.yaml",
		Hooks: RepositoryHookStatus{
			RepoPath: "/repo\x1b[32m\nforged",
		},
	}

	formatted := formatDoctorReport(report, nil)

	check.Contains(t, formatted, "Repository: /repo\\x1b[32m forged")
	check.Contains(t, formatted, "Configuration: config\\x1b[31m forged.yaml")
	check.NotContains(t, formatted, "\x1b")
}

func TestDoctor_FormatReport_RedirectedHooksDoNotTreatGlobalFilesAsActive(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	effectiveHooksPath := filepath.Join(root, "repository-hooks")
	globalHooksPath := filepath.Join(root, "global-hooks")
	healthyGlobal := HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}
	report := doctorReport{
		Hooks: RepositoryHookStatus{
			RepoPath:            filepath.Join(root, "repo"),
			HooksDir:            filepath.Join(root, "repo", ".git", "hooks"),
			GlobalHooksPath:     globalHooksPath,
			EffectiveHooksPath:  effectiveHooksPath,
			HooksPathRedirected: true,
			Hooks: []HookStatus{
				{Name: hookTypeCommitMsg, Global: healthyGlobal},
				{Name: hookTypePrePush, Global: healthyGlobal},
			},
		},
	}

	formatted := formatDoctorReport(report, nil)

	check.Contains(t, formatted, effectiveHooksPath)
	check.NotContains(t, formatted, "installed (global)")
	check.Contains(t, formatted, "git config core.hooksPath .git/hooks")
	check.Contains(t, formatted, "install hooks: gommitlint hook install")
}

func TestDoctor_FormatReport_MissingEffectiveGlobalHooksSuggestsGlobalInstall(t *testing.T) {
	t.Parallel()

	globalHooksPath := filepath.Join(t.TempDir(), "global-hooks")
	report := doctorReport{Hooks: RepositoryHookStatus{
		GlobalHooksPath:     globalHooksPath,
		EffectiveHooksPath:  globalHooksPath,
		HooksPathRedirected: true,
		Hooks: []HookStatus{
			{Name: hookTypeCommitMsg},
			{Name: hookTypePrePush},
		},
	}}

	formatted := formatDoctorReport(report, nil)

	check.Contains(t, formatted, "Install globally instead: gommitlint hook install --global")
	check.NotContains(t, formatted, "  - install hooks: gommitlint hook install\n")
}

func TestDoctor_HooksNeedInstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status RepositoryHookStatus
		want   bool
	}{
		{
			name:   "no_hooks_directory",
			status: RepositoryHookStatus{},
			want:   true,
		},
		{
			name: "missing_hook",
			status: RepositoryHookStatus{Hooks: []HookStatus{
				{Name: hookTypeCommitMsg, Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
				{Name: hookTypePrePush},
			}},
			want: true,
		},
		{
			name: "ready",
			status: RepositoryHookStatus{Hooks: []HookStatus{
				{Name: hookTypeCommitMsg, Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
				{Name: hookTypePrePush, Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
			}},
			want: false,
		},
		{
			name: "redirected_global_does_not_cover_custom_local",
			status: RepositoryHookStatus{
				GlobalHooksPath:     "/global-hooks",
				EffectiveHooksPath:  "/custom-hooks",
				HooksPathRedirected: true,
				Hooks: []HookStatus{
					{
						Name:   hookTypeCommitMsg,
						Local:  HookFileStatus{Exists: true, IsExecutable: true},
						Global: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true},
					},
					{Name: hookTypePrePush, Local: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
				},
			},
			want: true,
		},
		{
			name: "effective_global_hooks_are_ready",
			status: RepositoryHookStatus{
				GlobalHooksPath:     "/global-hooks",
				EffectiveHooksPath:  "/global-hooks",
				HooksPathRedirected: true,
				Hooks: []HookStatus{
					{Name: hookTypeCommitMsg, Global: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
					{Name: hookTypePrePush, Global: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true}},
				},
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, doctorHooksNeedInstall(testCase.status))
		})
	}
}

func TestDoctor_HookState_DistinguishesMixedOwnership(t *testing.T) {
	t.Parallel()

	hook := HookStatus{
		Name:   hookTypeCommitMsg,
		Local:  HookFileStatus{Exists: true, IsExecutable: true},
		Global: HookFileStatus{Exists: true, IsExecutable: true, IsGommitlint: true},
	}

	check.Equal(t, "unknown source (local) + installed (global)", doctorHookState(hook, nil))
}
