// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	urfavecli "github.com/urfave/cli/v3"
)

func TestNewGlobalFlags_Contract(t *testing.T) {
	t.Parallel()

	const repoPath = "/test/repository"

	expected := map[string]any{
		flags.Config:         "",
		flags.IgnoreConfig:   false,
		flags.RepoPath:       repoPath,
		flags.Color:          ColorAuto,
		flags.Theme:          output.DefaultTheme,
		flags.Quiet:          false,
		flags.LogLevel:       logadapter.LogLevelInfoName,
		flags.AllowCoredumps: false,
	}

	globalFlags := NewGlobalFlags(nil, repoPath)
	check.Len(t, globalFlags, len(expected))

	for _, globalFlag := range globalFlags {
		name := globalFlag.Names()[0]
		want, ok := expected[name]
		check.True(t, ok, "unexpected global flag %q", name)
		delete(expected, name)

		switch typedFlag := globalFlag.(type) {
		case *urfavecli.StringFlag:
			check.Equal(t, want, typedFlag.Value, "default for --%s", name)
		case *urfavecli.BoolFlag:
			check.Equal(t, want, typedFlag.Value, "default for --%s", name)
		default:
			t.Fatalf("global flag %q has unexpected type %T", name, globalFlag)
		}
	}

	check.Empty(t, expected, "all expected global flags must be present")

	quietFlag := findGlobalBoolFlag(t, globalFlags, flags.Quiet)
	check.Equal(t, []string{flags.Quiet, "q"}, quietFlag.Names())

	coredumpFlag := findGlobalBoolFlag(t, globalFlags, flags.AllowCoredumps)
	check.Equal(t, []string{"GOMMITLINT_ALLOW_COREDUMPS"}, coredumpFlag.GetEnvVars())

	for _, name := range []string{flags.Config, flags.IgnoreConfig, flags.RepoPath} {
		check.Contains(t, globalFlagUsage(t, globalFlags, name), "does not affect global hook operations")
	}
}

func globalFlagUsage(t *testing.T, globalFlags []urfavecli.Flag, name string) string {
	t.Helper()

	for _, globalFlag := range globalFlags {
		if globalFlag.Names()[0] != name {
			continue
		}

		switch typedFlag := globalFlag.(type) {
		case *urfavecli.StringFlag:
			return typedFlag.Usage
		case *urfavecli.BoolFlag:
			return typedFlag.Usage
		default:
			t.Fatalf("global flag %q has unexpected type %T", name, globalFlag)
		}
	}

	t.Fatalf("global flag %q not found", name)

	return ""
}

func findGlobalBoolFlag(t *testing.T, globalFlags []urfavecli.Flag, name string) *urfavecli.BoolFlag {
	t.Helper()

	for _, globalFlag := range globalFlags {
		if globalFlag.Names()[0] != name {
			continue
		}

		typedFlag, ok := globalFlag.(*urfavecli.BoolFlag)
		check.True(t, ok, "global flag %q must be boolean", name)

		return typedFlag
	}

	t.Fatalf("global flag %q not found", name)

	return nil
}
