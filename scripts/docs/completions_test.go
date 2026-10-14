// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package docs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// Each renderer stub trips exactly one of the script's two guards, so a guard
// that stops working is visible on its own; the staging directory must be gone
// after every refusal.
func TestCompletionsScript_FailedRendererLeavesOnlyTheOriginals(t *testing.T) {
	tests := []struct {
		name     string
		renderer string
	}{
		{
			name: "non_zero_exit_after_output",
			renderer: `#!/bin/sh
printf 'new %s\n' "$3"
if [ "$3" = "fish" ]; then
	exit 9
fi
`,
		},
		{
			name: "zero_exit_with_empty_output",
			renderer: `#!/bin/sh
if [ "$3" = "fish" ]; then
	exit 0
fi
printf 'new %s\n' "$3"
`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := t.TempDir()
			outputDir := filepath.Join(tempDir, "output")
			check.NoError(t, os.Mkdir(outputDir, 0o700))

			for _, shell := range []string{"bash", "zsh", "fish"} {
				check.NoError(t, os.WriteFile(filepath.Join(outputDir, "gommitlint."+shell), []byte("old "+shell+"\n"), 0o600))
			}

			binaryPath := writeCompletionRenderer(t, tempDir, testCase.renderer)
			cmd := completionScriptCommand(t, binaryPath, outputDir)

			check.Error(t, cmd.Run())

			for _, shell := range []string{"bash", "zsh", "fish"} {
				content, readErr := os.ReadFile(filepath.Join(outputDir, "gommitlint."+shell))
				check.NoError(t, readErr)
				check.Equal(t, "old "+shell+"\n", string(content))
			}

			check.Equal(t, []string{"gommitlint.bash", "gommitlint.fish", "gommitlint.zsh"}, directoryNames(t, outputDir))
		})
	}
}

func TestCompletionsScript_PublishesExactStdoutForEveryShell(t *testing.T) {
	tempDir := t.TempDir()
	outputDir := filepath.Join(tempDir, "output")
	binaryPath := writeCompletionRenderer(t, tempDir, `#!/bin/sh
if [ "$1 $2" != "completion --stdout" ]; then
	exit 8
fi
if [ "${LC_ALL:-} ${LANG:-} ${LANGUAGE:-}" != "C C C" ]; then
	exit 7
fi
printf 'rendered %s for gommitlint\n' "$3"
`)
	cmd := completionScriptCommand(t, binaryPath, outputDir)

	check.NoError(t, cmd.Run())

	for _, shell := range []string{"bash", "zsh", "fish"} {
		content, err := os.ReadFile(filepath.Join(outputDir, "gommitlint."+shell))
		check.NoError(t, err)
		check.Equal(t, "rendered "+shell+" for gommitlint\n", string(content))
	}

	check.Equal(t, []string{"gommitlint.bash", "gommitlint.fish", "gommitlint.zsh"}, directoryNames(t, outputDir))
}

func directoryNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	check.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func writeCompletionRenderer(t *testing.T, tempDir, content string) string {
	t.Helper()

	binaryPath := filepath.Join(tempDir, "gommitlint")
	check.NoError(t, os.WriteFile(binaryPath, []byte(content), 0o700))

	return binaryPath
}

func completionScriptCommand(t *testing.T, binaryPath, outputDir string) *exec.Cmd {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "/bin/bash", "./completions.sh")
	cmd.Env = []string{
		"BINARY_PATH=" + binaryPath,
		"OUTPUT_DIR=" + outputDir,
		"PATH=" + os.Getenv("PATH"),
	}

	return cmd
}
