//
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestCompletionCommand_InstallShell(t *testing.T) {
	root, stdout, stderr := newCompletionTestApp(t)
	ctx := t.Context()

	err := root.Run(ctx, []string{appName, completionCommandName, shellFish})

	check.NoError(t, err)
	check.NotEmpty(t, stderr.String())
	check.Contains(t, stderr.String(), "Installed fish completion")
	check.Empty(t, stdout.String())

	installPath, err := completionInstallPath(shellFish, root.Name)
	check.NoError(t, err)

	check.FileExists(t, installPath)
	content := readFile(t, installPath)
	check.NotEmpty(t, content)
	check.Contains(t, string(content), "-l theme -r -a 'catppuccin-latte")
}

func TestCompletionCommand_InstallAll(t *testing.T) {
	root, stdout, stderr := newCompletionTestApp(t)
	ctx := t.Context()

	err := root.Run(ctx, []string{appName, completionCommandName, flagAllLong})

	check.NoError(t, err)
	check.NotEmpty(t, stderr.String())
	check.Empty(t, stdout.String())

	for _, shell := range completionShells() {
		installPath, err := completionInstallPath(shell, root.Name)
		check.NoError(t, err)
		check.FileExists(t, installPath)
		content := readFile(t, installPath)
		check.NotEmpty(t, content)
		check.Contains(t, string(content), completionContentMarker(shell, root.Name))
		check.Contains(t, stderr.String(), "completion at "+installPath)
	}

	fishPath, err := completionInstallPath(shellFish, root.Name)
	check.NoError(t, err)
	fishContent := readFile(t, fishPath)
	check.Contains(t, string(fishContent), "__fish_seen_subcommand_from completion")
	check.Contains(t, string(fishContent), "-a 'bash zsh fish'")
	check.Contains(t, string(fishContent), " -F -l repo-path -r")
	check.Contains(t, string(fishContent), " -F -l message-file -s f -r")
	check.Contains(t, string(fishContent), " -F -l report-file -r")
	check.Contains(t, string(fishContent), "__fish_seen_subcommand_from completion validate")
	check.Contains(t, string(fishContent), " -l color -r -a 'auto always never'")
	check.Contains(t, string(fishContent), "-a '(__fish_gommitlint_dynamic_completion ref)'")
}

func TestCompletionCommand_StdoutMatchesRenderer(t *testing.T) {
	for _, shell := range completionShells() {
		t.Run(shell, func(t *testing.T) {
			root, stdout, stderr := newCompletionTestApp(t)

			err := root.Run(t.Context(), []string{appName, completionCommandName, flagStdoutLong, shell})

			check.NoError(t, err)
			check.Empty(t, stderr.String())

			if shell != shellFish {
				check.Contains(t, stdout.String(), completionShellEnv+"="+shell)
			}

			if shell == shellBash {
				check.NotContains(t, stdout.String(), "eval ")
				check.Contains(t, stdout.String(), `"${words[@]}"`)
			}

			completion := root.Command(completionCommandName)
			check.NotNil(t, completion)
			expected, err := renderCompletionScript(completion, shell, appName)
			check.NoError(t, err)
			check.Equal(t, expected, stdout.String())
		})
	}
}

func TestCompletionCommand_FullAppInstallModeKeepsStdoutEmpty(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "config"))

	originalArgs := os.Args

	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{appName}

	var stdout, stderr bytes.Buffer

	app := createCLIApp(nil)
	app.Writer = &stdout
	app.ErrWriter = &stderr

	err := app.Run(t.Context(), []string{appName, completionCommandName, shellFish})

	check.NoError(t, err)
	check.Empty(t, stdout.String())
	check.Contains(t, stderr.String(), "Installed fish completion")

	installPath, err := completionInstallPath(shellFish, appName)
	check.NoError(t, err)
	check.FileExists(t, installPath)
	content := readFile(t, installPath)
	check.Contains(t, string(content), "__fish_seen_subcommand_from completion config doctor hook man validate")
}

func TestCompletionCommand_MissingShell(t *testing.T) {
	root, _, _ := newCompletionTestApp(t)
	ctx := t.Context()

	err := root.Run(ctx, []string{appName, completionCommandName})

	var recovery commands.RecoveryError

	check.Error(t, err)
	check.ErrorAs(t, err, &recovery)
	check.ErrorIs(t, err, errCompletionMissingShell)
}

func TestCompletionCommand_RejectsExtraShellArguments(t *testing.T) {
	testCases := []struct {
		name     string
		args     []string
		expected error
	}{
		{name: "multiple_shells", args: []string{shellBash, shellZsh}, expected: errCompletionExtraArgs},
		{name: "all_alias_with_extra_argument", args: []string{completionAll, shellBash}, expected: errCompletionAllExtraArgs},
		{name: "all_flag_with_shell", args: []string{flagAllLong, shellBash}, expected: errCompletionAllExtraArgs},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			root, _, _ := newCompletionTestApp(t)

			args := append([]string{appName, completionCommandName, flagStdoutLong}, testCase.args...)

			err := root.Run(t.Context(), args)

			check.ErrorIs(t, err, testCase.expected)
		})
	}
}

func TestCompletionCommand_StdoutReportsWriteFailure(t *testing.T) {
	root, _, _ := newCompletionTestApp(t)
	writer := failingWriter{err: io.ErrClosedPipe}
	root.Writer = writer
	root.Command(completionCommandName).Writer = writer

	err := root.Run(t.Context(), []string{appName, completionCommandName, flagStdoutLong, shellBash})

	check.ErrorIs(t, err, errCompletionOutput)
	check.ErrorIs(t, err, commands.ErrOutputWrite)
	check.ErrorIs(t, err, io.ErrClosedPipe)
}

func TestCompletionCommand_FishUsesDynamicRepositoryValues(t *testing.T) {
	root, _, _ := newCompletionTestApp(t)
	ctx := t.Context()

	err := root.Run(ctx, []string{appName, completionCommandName, shellFish})

	check.NoError(t, err)

	fishPath, err := completionInstallPath(shellFish, root.Name)
	check.NoError(t, err)
	fishContent := readFile(t, fishPath)
	check.Contains(t, string(fishContent), " -l ref -s r -r -d")
	check.Contains(t, string(fishContent), "-a '(__fish_gommitlint_dynamic_completion ref)'")
	check.Contains(t, string(fishContent), " -l base-branch -s b -r -d")
	check.Contains(t, string(fishContent), "-a '(__fish_gommitlint_dynamic_completion base-branch)'")
	check.Contains(t, string(fishContent), " -l range -r -d")
	check.Contains(t, string(fishContent), "-a '(__fish_gommitlint_dynamic_completion range)'")
	check.NotContains(t, string(fishContent), " -a 'HEAD")
	check.Contains(t, string(fishContent), " -F -l repo-path -r")
	check.Contains(t, string(fishContent), "__fish_seen_subcommand_from completion validate")
}

func TestCompletionCommand_StdoutFishUsesDynamicRepositoryValuesAndExcludesHiddenCommands(t *testing.T) {
	root, stdout, stderr := newCompletionTestApp(t)
	root.Commands = append(root.Commands, &cli.Command{
		Name: "hook",
		Commands: []*cli.Command{{
			Name:   "run",
			Hidden: true,
			Commands: []*cli.Command{{
				Name:   "pre-push",
				Hidden: true,
			}},
		}},
	})

	err := root.Run(t.Context(), []string{appName, completionCommandName, flagStdoutLong, shellFish})

	check.NoError(t, err)
	check.Empty(t, stderr.String())
	check.Contains(t, stdout.String(), "-a '(__fish_gommitlint_dynamic_completion ref)'")
	check.Contains(t, stdout.String(), "-a '(__fish_gommitlint_dynamic_completion base-branch)'")
	check.Contains(t, stdout.String(), "-a '(__fish_gommitlint_dynamic_completion range)'")
	check.NotContains(t, stdout.String(), " -a 'HEAD")
	check.NotContains(t, stdout.String(), "hook run")
	check.NotContains(t, stdout.String(), "pre-push")
}

func TestCompletionAppName_RejectsShellSyntax(t *testing.T) {
	originalArgs := os.Args

	t.Cleanup(func() { os.Args = originalArgs })

	root := &cli.Command{Name: appName}
	testCases := []struct {
		binaryName string
		expected   string
	}{
		{binaryName: "/tmp/custom-name.exe", expected: "custom-name.exe"},
		{binaryName: "/tmp/custom;name", expected: appName},
		{binaryName: "/tmp/custom'name", expected: appName},
		{binaryName: "/tmp/custom\nname", expected: appName},
		{binaryName: "-", expected: appName},
	}

	for _, testCase := range testCases {
		os.Args = []string{testCase.binaryName}

		check.Equal(t, testCase.expected, completionAppName(root))
	}
}

func TestCompletionCommand_FishRewritesDynamicCommandName(t *testing.T) {
	root, stdout, _ := newCompletionTestApp(t)
	os.Args = []string{"/tmp/custom-name"}

	err := root.Run(t.Context(), []string{"custom-name", completionCommandName, flagStdoutLong, shellFish})

	check.NoError(t, err)
	check.Contains(t, stdout.String(), "function __fish_custom_name_dynamic_completion")
	check.Contains(t, stdout.String(), "env GOMMITLINT_COMPLETION_SHELL=fish custom-name $repository_args validate --$argv[1] --generate-shell-completion")
	check.Contains(t, stdout.String(), "-a '(__fish_custom_name_dynamic_completion ref)'")
	check.Contains(t, stdout.String(), "'(custom-name completion --generate-shell-completion")
	check.NotContains(t, stdout.String(), "env GOMMITLINT_COMPLETION_SHELL=fish gommitlint $repository_args")
	check.NotContains(t, stdout.String(), "'(gommitlint completion --generate-shell-completion")
}

func TestCompletionCommand_GeneratedFishDynamicValuesUseActiveRepoPath(t *testing.T) {
	currentRepo := createTestRepository(t)
	selectedRepoSource := createTestRepository(t)
	selectedRepo := filepath.Join(t.TempDir(), "selected repo '$not_shell_syntax'")
	check.NoError(t, os.Rename(selectedRepoSource, selectedRepo))
	addTestBranch(t, currentRepo, "current-only")
	addTestBranch(t, selectedRepo, "selected-only")
	t.Chdir(currentRepo)
	t.Setenv(completionShellEnv, shellFish)

	root, stdout, _ := newCompletionTestApp(t)
	os.Args = []string{"/tmp/custom-name"}

	err := root.Run(t.Context(), []string{"custom-name", completionCommandName, flagStdoutLong, shellFish})

	check.NoError(t, err)
	check.Contains(t, stdout.String(), "case '--repo-path'")
	check.Contains(t, stdout.String(), "case '--repo-path=*'")
	check.Contains(t, stdout.String(), "set repository_args --repo-path $tokens[(math $index + 1)]")
	check.Contains(t, stdout.String(), "string replace -- '--repo-path=' '' $tokens[$index]")
	check.Contains(t, stdout.String(), "custom-name $repository_args validate --$argv[1]")
	check.NotContains(t, stdout.String(), "eval ")
	check.NotContains(t, stdout.String(), selectedRepo)

	testCases := []struct {
		name      string
		args      []string
		expected  string
		forbidden string
	}{
		{
			name:      "ref_with_space_separated_global_flag",
			args:      []string{"custom-name", "--repo-path", selectedRepo, "validate", "--ref", "--generate-shell-completion"},
			expected:  "selected-only\n",
			forbidden: "current-only\n",
		},
		{
			name:      "base_with_equals_global_flag",
			args:      []string{"custom-name", "--repo-path=" + selectedRepo, "validate", "--base-branch", "--generate-shell-completion"},
			expected:  "selected-only\n",
			forbidden: "current-only\n",
		},
		{
			name:      "range_with_inherited_global_flag",
			args:      []string{"custom-name", "validate", "--repo-path", selectedRepo, "--range", "--generate-shell-completion"},
			expected:  "selected-only..\n",
			forbidden: "current-only..\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			os.Args = testCase.args

			var completionOutput, completionError bytes.Buffer

			exitCode := runWithIO(testCase.args, strings.NewReader(""), &completionOutput, &completionError)

			check.Equal(t, 0, exitCode)
			check.Empty(t, completionError.String())
			check.Contains(t, completionOutput.String(), testCase.expected)
			check.NotContains(t, completionOutput.String(), testCase.forbidden)
		})
	}
}

func TestFishCompletionArguments_EscapesStaticValues(t *testing.T) {
	t.Parallel()

	arguments := fishCompletionArguments(appName, fishValueCompletionUpdate{values: []string{`a\b`, `x'y`}})

	check.Equal(t, `a\\b x\'y`, arguments)
}

func newCompletionTestApp(t *testing.T) (*cli.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tempDir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempDir, "config"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	originalArgs := os.Args

	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{appName}

	completion := &cli.Command{Name: completionCommandName, Writer: stdout, ErrWriter: stderr}
	localizer := output.EnsureLocalizer(nil)
	configureCompletionCommand(completion, localizer)

	validate := commands.NewValidateCommand(localizer)

	root := &cli.Command{
		Name: appName,
		Flags: []cli.Flag{
			&cli.StringFlag{Name: flags.RepoPath},
			&cli.StringFlag{Name: flags.Color, Value: cliadapter.ColorAuto},
			&cli.StringFlag{Name: flags.Theme, Value: output.DefaultTheme},
			&cli.StringFlag{Name: flags.LogLevel, Value: logadapter.LogLevelInfoName},
		},
		Commands:  []*cli.Command{completion, validate},
		Writer:    stdout,
		ErrWriter: stderr,
	}

	return root, stdout, stderr
}

func completionContentMarker(shell string, appName string) string {
	switch shell {
	case shellBash:
		return "__" + appName + "_bash_autocomplete"
	case shellZsh:
		return "compdef _" + appName + " " + appName
	case shellFish:
		return "complete -c " + appName
	default:
		return ""
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	check.NoError(t, err)

	return content
}
