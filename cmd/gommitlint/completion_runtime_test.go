//
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
)

const (
	testCompletionPrefixDra    = "dra"
	testCompletionSubject      = rulecatalog.Subject
	testCompletionSubjectComma = rulecatalog.Subject + ","
	testCompletionConventional = rulecatalog.Conventional
	testCompletionSignoff      = rulecatalog.SignOff
)

func TestShellCompleteWithValues_CompletesShellAfterCompletionFlag(t *testing.T) {
	originalArgs := os.Args

	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{appName, completionCommandName, flagStdoutLong, "--generate-shell-completion"}

	var stdout bytes.Buffer

	root := &cli.Command{
		Name:                  appName,
		EnableShellCompletion: true,
		ShellComplete:         shellCompleteWithValues,
		ConfigureShellCompletionCommand: func(completion *cli.Command) {
			configureCompletionCommand(completion, nil)
		},
		Writer: &stdout,
	}

	err := root.Run(t.Context(), os.Args)

	check.NoError(t, err)
	check.Equal(t, "bash\nzsh\nfish\n", stdout.String())
}

func TestPrintCommandCompletions_UsesExplicitShellProtocol(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv(completionShellEnv, shellBash)

	var outputBuffer bytes.Buffer

	commands := []*cli.Command{{Name: commandValidate, Usage: "validate commits"}}
	printCommandCompletions(commands, &outputBuffer)
	check.Equal(t, commandValidate+"\n", outputBuffer.String())

	t.Setenv(completionShellEnv, shellZsh)
	outputBuffer.Reset()
	printCommandCompletions(commands, &outputBuffer)
	check.Equal(t, commandValidate+":validate commits\n", outputBuffer.String())
}

func TestFlagValuePrefix_ParsesFlagPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		flagName    string
		previousArg string
		lastArg     string
		wantPrefix  string
		wantSameArg bool
		wantMatches bool
	}{
		{
			name:        "equals sign syntax with value",
			flagName:    flags.Theme,
			previousArg: commandValidate,
			lastArg:     "--theme=dra",
			wantPrefix:  testCompletionPrefixDra,
			wantSameArg: true,
			wantMatches: true,
		},
		{
			name:        "equals sign syntax empty value",
			flagName:    flags.Theme,
			previousArg: commandValidate,
			lastArg:     "--theme=",
			wantPrefix:  "",
			wantSameArg: true,
			wantMatches: true,
		},
		{
			name:        "space separated with value",
			flagName:    flags.Theme,
			previousArg: flagThemeLong,
			lastArg:     testCompletionPrefixDra,
			wantPrefix:  testCompletionPrefixDra,
			wantSameArg: false,
			wantMatches: true,
		},
		{
			name:        "flag only no value yet",
			flagName:    flags.Theme,
			previousArg: commandValidate,
			lastArg:     flagThemeLong,
			wantPrefix:  "",
			wantSameArg: false,
			wantMatches: true,
		},
		{
			name:        "different flag no match",
			flagName:    flags.Theme,
			previousArg: flagFormatLong,
			lastArg:     output.FormatJSON,
			wantPrefix:  "",
			wantSameArg: false,
			wantMatches: false,
		},
		{
			name:        "no flags",
			flagName:    flags.Theme,
			previousArg: appName,
			lastArg:     commandValidate,
			wantPrefix:  "",
			wantSameArg: false,
			wantMatches: false,
		},
		{
			name:        "previous is flag and last is same flag",
			flagName:    flags.Color,
			previousArg: flagColorLong,
			lastArg:     flagColorLong,
			wantPrefix:  "",
			wantSameArg: false,
			wantMatches: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			prefix, sameArg, matches := flagValuePrefix(tt.flagName, tt.previousArg, tt.lastArg)

			check.Equal(t, tt.wantPrefix, prefix, "prefix mismatch")
			check.Equal(t, tt.wantSameArg, sameArg, "sameArg mismatch")
			check.Equal(t, tt.wantMatches, matches, "matches mismatch")
		})
	}
}

func TestParseListPrefix_HandlesCommaSeparatedValues(t *testing.T) {
	t.Parallel()

	normalize := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

	tests := []struct {
		name              string
		prefix            string
		wantListPrefix    string
		wantSegmentPrefix string
		wantUsedCount     int
	}{
		{
			name:              "no comma single value",
			prefix:            testCompletionSubject,
			wantListPrefix:    "",
			wantSegmentPrefix: testCompletionSubject,
			wantUsedCount:     0,
		},
		{
			name:              "one used value typing second",
			prefix:            testCompletionSubjectComma + "conv",
			wantListPrefix:    testCompletionSubjectComma,
			wantSegmentPrefix: "conv",
			wantUsedCount:     1,
		},
		{
			name:              "two used values typing third",
			prefix:            testCompletionSubjectComma + testCompletionConventional + ",sig",
			wantListPrefix:    testCompletionSubjectComma + testCompletionConventional + ",",
			wantSegmentPrefix: "sig",
			wantUsedCount:     2,
		},
		{
			name:              "trailing comma no segment",
			prefix:            testCompletionSubjectComma,
			wantListPrefix:    testCompletionSubjectComma,
			wantSegmentPrefix: "",
			wantUsedCount:     1,
		},
		{
			name:              "empty string",
			prefix:            "",
			wantListPrefix:    "",
			wantSegmentPrefix: "",
			wantUsedCount:     0,
		},
		{
			name:              "spaces around values",
			prefix:            testCompletionSubject + " , " + testCompletionConventional,
			wantListPrefix:    "subject , ",
			wantSegmentPrefix: testCompletionConventional,
			wantUsedCount:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			listPrefix, segmentPrefix, usedValues := parseListPrefix(tt.prefix, normalize)

			check.Equal(t, tt.wantListPrefix, listPrefix, "listPrefix mismatch")
			check.Equal(t, tt.wantSegmentPrefix, segmentPrefix, "segmentPrefix mismatch")
			check.Len(t, usedValues, tt.wantUsedCount, "usedValues count mismatch")
		})
	}
}

func TestWriteSingleValueCompletions_OutputsFilteredValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		flagName   string
		values     []string
		prefix     string
		sameArg    bool
		wantOutput []string
	}{
		{
			name:       "no prefix shows all",
			flagName:   flags.Color,
			values:     []string{cliadapter.ColorAuto, cliadapter.ColorAlways, cliadapter.ColorNever},
			prefix:     "",
			sameArg:    false,
			wantOutput: []string{cliadapter.ColorAuto, cliadapter.ColorAlways, cliadapter.ColorNever},
		},
		{
			name:       "prefix filters values",
			flagName:   flags.Color,
			values:     []string{cliadapter.ColorAuto, cliadapter.ColorAlways, cliadapter.ColorNever},
			prefix:     "a",
			sameArg:    false,
			wantOutput: []string{cliadapter.ColorAuto, cliadapter.ColorAlways},
		},
		{
			name:       "sameArg prepends flag",
			flagName:   flags.Color,
			values:     []string{cliadapter.ColorAuto, cliadapter.ColorAlways, cliadapter.ColorNever},
			prefix:     "n",
			sameArg:    true,
			wantOutput: []string{flagColorLong + "=" + cliadapter.ColorNever},
		},
		{
			name:       "no matches",
			flagName:   flags.Color,
			values:     []string{cliadapter.ColorAuto, cliadapter.ColorAlways, cliadapter.ColorNever},
			prefix:     "xyz",
			sameArg:    false,
			wantOutput: []string{},
		},
		{
			name:       "case insensitive matching",
			flagName:   flags.Theme,
			values:     []string{"Dracula", "Nord"},
			prefix:     testCompletionPrefixDra,
			sameArg:    false,
			wantOutput: []string{"Dracula"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			cmd := &cli.Command{
				Name:   commandTest,
				Writer: buf,
			}

			result := writeSingleValueCompletions(cmd, tt.flagName, tt.values, tt.prefix, tt.sameArg, normalizeLower)

			check.True(t, result, "should return true")

			output := strings.TrimSpace(buf.String())
			if len(tt.wantOutput) == 0 {
				check.Empty(t, output)
			} else {
				lines := strings.Split(output, "\n")
				check.Equal(t, tt.wantOutput, lines)
			}
		})
	}
}

func TestWriteMultiValueCompletions_ExcludesUsedValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		flagName   string
		values     []string
		prefix     string
		sameArg    bool
		wantOutput []string
	}{
		{
			name:       "excludes already used values",
			flagName:   flags.Rule,
			values:     []string{testCompletionSubject, testCompletionConventional, testCompletionSignoff},
			prefix:     testCompletionSubjectComma,
			sameArg:    false,
			wantOutput: []string{testCompletionSubjectComma + testCompletionConventional, testCompletionSubjectComma + testCompletionSignoff},
		},
		{
			name:       "filters by segment prefix",
			flagName:   flags.Rule,
			values:     []string{testCompletionSubject, testCompletionConventional, testCompletionSignoff},
			prefix:     testCompletionSubjectComma + "sig",
			sameArg:    false,
			wantOutput: []string{testCompletionSubjectComma + testCompletionSignoff},
		},
		{
			name:       "multiple used values",
			flagName:   flags.Rule,
			values:     []string{testCompletionSubject, testCompletionConventional, testCompletionSignoff, rulecatalog.Spell},
			prefix:     testCompletionSubjectComma + testCompletionSignoff + ",",
			sameArg:    false,
			wantOutput: []string{testCompletionSubjectComma + testCompletionSignoff + "," + testCompletionConventional, testCompletionSubjectComma + testCompletionSignoff + "," + rulecatalog.Spell},
		},
		{
			name:       "sameArg includes flag prefix",
			flagName:   flags.Rule,
			values:     []string{testCompletionSubject, testCompletionConventional},
			prefix:     testCompletionSubjectComma,
			sameArg:    true,
			wantOutput: []string{flagRuleLong + "=" + testCompletionSubjectComma + testCompletionConventional},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			buf := &bytes.Buffer{}
			cmd := &cli.Command{
				Name:   commandTest,
				Writer: buf,
			}

			result := writeMultiValueCompletions(cmd, tt.flagName, tt.values, tt.prefix, tt.sameArg, normalizeLower)

			check.True(t, result, "should return true")

			output := strings.TrimSpace(buf.String())
			if len(tt.wantOutput) == 0 {
				check.Empty(t, output)
			} else {
				lines := strings.Split(output, "\n")
				check.Equal(t, tt.wantOutput, lines)
			}
		})
	}
}

func TestCompletionSpecs_ReturnsContextualSpecs(t *testing.T) {
	t.Parallel()

	t.Run("root command has global specs only", func(t *testing.T) {
		t.Parallel()

		cmd := &cli.Command{Name: appName}
		specs := completionSpecs(cmd)

		flagNames := make(map[string]bool)
		for _, spec := range specs {
			flagNames[spec.flag] = true
		}

		check.True(t, flagNames[flags.Color], "should have color")
		check.True(t, flagNames[flags.Theme], "should have theme")
		check.True(t, flagNames[flags.LogLevel], "should have log-level")
		check.False(t, flagNames[flags.Rule], "should not have rule")
		check.False(t, flagNames[flags.Ref], "should not have ref")
	})

	t.Run("validate command has validate specs", func(t *testing.T) {
		t.Parallel()

		cmd := &cli.Command{Name: commandValidate}
		specs := completionSpecs(cmd)

		flagNames := make(map[string]bool)
		for _, spec := range specs {
			flagNames[spec.flag] = true
		}

		check.True(t, flagNames[flags.Color], "should have color")
		check.True(t, flagNames[flags.Theme], "should have theme")
		check.True(t, flagNames[flags.Format], "should have format")
		check.True(t, flagNames[flags.Rule], "should have rule")
		check.True(t, flagNames[flags.ExcludeRule], "should have exclude-rule")
		check.True(t, flagNames[flags.Ref], "should have ref")
		check.True(t, flagNames[flags.BaseBranch], "should have base-branch")
	})

	t.Run("validate specs include multi-value rules", func(t *testing.T) {
		t.Parallel()

		cmd := &cli.Command{Name: commandValidate}
		specs := completionSpecs(cmd)

		var ruleSpec, excludeRuleSpec *completionSpec

		for idx := range specs {
			if specs[idx].flag == flags.Rule {
				ruleSpec = &specs[idx]
			}

			if specs[idx].flag == flags.ExcludeRule {
				excludeRuleSpec = &specs[idx]
			}
		}

		check.NotNil(t, ruleSpec, "should have rule spec")
		check.True(t, ruleSpec.multi, "rule should be multi-value")

		check.NotNil(t, excludeRuleSpec, "should have exclude-rule spec")
		check.True(t, excludeRuleSpec.multi, "exclude-rule should be multi-value")
	})
}

func TestNormalizeLower_HandlesEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"UPPERCASE", "uppercase"},
		{"MixedCase", "mixedcase"},
		{"  spaces  ", "spaces"},
		{"", ""},
		{"already-lower", "already-lower"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := normalizeLower(tt.input)
			check.Equal(t, tt.want, got)
		})
	}
}

func TestThemeCompletionValues_ReturnsValidThemes(t *testing.T) {
	t.Parallel()

	values := themeCompletionValues(t.Context(), nil)

	check.NotEmpty(t, values)
	check.Contains(t, values, output.DefaultTheme)
	check.Contains(t, values, "dracula")
}

func TestRuleCompletionValues_ReturnsSortedRules(t *testing.T) {
	t.Parallel()

	values := ruleCompletionValues(t.Context(), nil)

	check.NotEmpty(t, values)
	check.Contains(t, values, rulecatalog.Subject)
	check.Contains(t, values, rulecatalog.Conventional)

	// Verify sorted
	for i := 1; i < len(values); i++ {
		check.LessOrEqual(t, values[i-1], values[i], "values should be sorted")
	}
}

func TestOutputFormatCompletions_ReturnsValidFormats(t *testing.T) {
	t.Parallel()

	values := outputFormatCompletions(t.Context(), nil)

	check.NotEmpty(t, values)
	check.Contains(t, values, output.FormatText)
	check.Contains(t, values, output.FormatJSON)
	check.Contains(t, values, output.FormatGitHub)
}

func TestCompletionFileFlags_ReturnsExpectedFlags(t *testing.T) {
	t.Parallel()

	fileFlags := completionFileFlags()

	check.Contains(t, fileFlags, flags.Config)
	check.Contains(t, fileFlags, flags.RepoPath)
	check.Contains(t, fileFlags, flags.MessageFile)
	check.Contains(t, fileFlags, flags.ReportFile)
}

func TestCompletionWantsFileValues_DetectsFileFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		previousArg string
		lastArg     string
		want        bool
	}{
		{
			name:        "config flag previous",
			previousArg: "--config",
			lastArg:     "partial",
			want:        true,
		},
		{
			name:        "repo-path equals syntax",
			previousArg: commandValidate,
			lastArg:     "--repo-path=/some/path",
			want:        true,
		},
		{
			name:        "message-file flag",
			previousArg: "--message-file",
			lastArg:     "",
			want:        true,
		},
		{
			name:        "non-file flag",
			previousArg: flagThemeLong,
			lastArg:     "dracula",
			want:        false,
		},
		{
			name:        "no flags",
			previousArg: appName,
			lastArg:     commandValidate,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := completionWantsFileValues(tt.previousArg, tt.lastArg)
			check.Equal(t, tt.want, got)
		})
	}
}
