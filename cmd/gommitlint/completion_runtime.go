// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

const (
	completionCommandName = "completion"
	completionShellEnv    = "GOMMITLINT_COMPLETION_SHELL"
	completionRequestFlag = "--generate-shell-completion"
)

func shellCompleteWithValues(ctx context.Context, cmd *cli.Command) {
	if printValueCompletions(ctx, cmd) {
		return
	}

	printCompletionCommandArgs(cmd)
	completeWithGlobalFlags(ctx, cmd)
}

type completionSpec struct {
	flag      string
	values    func(context.Context, *cli.Command) []string
	multi     bool
	normalize func(string) string
}

func printValueCompletions(ctx context.Context, cmd *cli.Command) bool {
	lastArg, previousArg, ok := completionArguments()
	if !ok {
		return false
	}

	if completionWantsFileValues(previousArg, lastArg) {
		return true
	}

	for _, spec := range completionSpecs(cmd) {
		prefix, sameArg, matches := flagValuePrefix(spec.flag, previousArg, lastArg)
		if !matches {
			continue
		}

		if writeCompletionValues(ctx, cmd, spec, prefix, sameArg) {
			return true
		}
	}

	return false
}

func completionWantsFileValues(previousArg string, lastArg string) bool {
	for _, flagName := range completionFileFlags() {
		flagToken := "--" + flagName
		if previousArg == flagToken {
			return true
		}

		if strings.HasPrefix(lastArg, flagToken+"=") {
			return true
		}
	}

	return false
}

func completionFileFlags() []string {
	return []string{flags.Config, flags.RepoPath, flags.MessageFile, flags.ReportFile}
}

func completeWithGlobalFlags(ctx context.Context, cmd *cli.Command) {
	args := completionArgs()
	if len(args) == 0 {
		return
	}

	lastArg := completionLastArg(args)
	if lastArg == "--" {
		return
	}

	if strings.HasPrefix(lastArg, "-") {
		flags := completionFlagsForCommand(cmd)
		printFlagCompletions(lastArg, flags, cmd.Root().Writer)

		return
	}

	if cmd != nil {
		printCommandCompletions(cmd.Commands, cmd.Root().Writer)
	}

	_ = ctx
}

func completionLastArg(args []string) string {
	argsLen := len(args)
	if argsLen == 0 {
		return ""
	}

	return args[argsLen-1]
}

func completionFlagsForCommand(cmd *cli.Command) []cli.Flag {
	if cmd == nil {
		return nil
	}

	if len(cmd.Lineage()) == 1 {
		return cmd.Flags
	}

	flags := append([]cli.Flag{}, cmd.Flags...)
	flags = append(flags, cmd.Root().Flags...)

	return flags
}

func printCommandCompletions(commands []*cli.Command, writer io.Writer) {
	for _, command := range commands {
		if command.Hidden {
			continue
		}

		if isZshCompletion() && len(command.Usage) > 0 {
			_, _ = fmt.Fprintf(writer, "%s:%s\n", command.Name, command.Usage)

			continue
		}

		_, _ = fmt.Fprintf(writer, "%s\n", command.Name)
	}
}

func printFlagCompletions(lastArg string, flags []cli.Flag, writer io.Writer) {
	cur := strings.TrimLeft(lastArg, "-")
	for _, flag := range flags {
		completion, ok := flagCompletionCandidate(lastArg, cur, flag)
		if !ok {
			continue
		}

		_, _ = fmt.Fprintln(writer, completion)
	}
}

func flagCompletionCandidate(lastArg string, cur string, flag cli.Flag) (string, bool) {
	if boolFlag, ok := flag.(*cli.BoolFlag); ok && boolFlag.Hidden {
		return "", false
	}

	name := strings.TrimSpace(flag.Names()[0])
	if !strings.HasPrefix(name, cur) || cur == name {
		return "", false
	}

	dashCount := min(utf8.RuneCountInString(name), 2)

	if strings.HasPrefix(lastArg, "--") && dashCount == 1 {
		return "", false
	}

	completion := fmt.Sprintf("%s%s", strings.Repeat("-", dashCount), name)

	usage := flagUsage(flag)
	if usage != "" && isZshCompletion() {
		completion = fmt.Sprintf("%s:%s", completion, usage)
	}

	return completion, true
}

func flagUsage(flag cli.Flag) string {
	if docFlag, ok := flag.(cli.DocGenerationFlag); ok {
		return docFlag.GetUsage()
	}

	return ""
}

func printCompletionCommandArgs(cmd *cli.Command) {
	if cmd == nil || cmd.Name != completionCommandName || cmd.Bool(completionAll) || cmd.Args().Present() {
		return
	}

	for _, shell := range []string{shellBash, shellZsh, shellFish} {
		_, _ = fmt.Fprintln(cmd.Root().Writer, shell)
	}
}

func completionArguments() (string, string, bool) {
	args := completionArgs()
	if len(args) == 0 {
		return "", "", false
	}

	lastArg := args[len(args)-1]

	previousArg := ""
	if len(args) > 1 {
		previousArg = args[len(args)-2]
	}

	return lastArg, previousArg, true
}

func writeCompletionValues(ctx context.Context, cmd *cli.Command, spec completionSpec, prefix string, sameArg bool) bool {
	values, normalize := completionValues(ctx, cmd, spec)
	if spec.multi {
		return writeMultiValueCompletions(cmd, spec.flag, values, prefix, sameArg, normalize)
	}

	return writeSingleValueCompletions(cmd, spec.flag, values, prefix, sameArg, normalize)
}

func completionValues(ctx context.Context, cmd *cli.Command, spec completionSpec) ([]string, func(string) string) {
	values := spec.values(ctx, cmd)

	normalize := spec.normalize
	if normalize == nil {
		normalize = normalizeLower
	}

	return values, normalize
}

func writeMultiValueCompletions(cmd *cli.Command, flagName string, values []string, prefix string, sameArg bool, normalize func(string) string) bool {
	listPrefix, segmentPrefix, usedValues := parseListPrefix(prefix, normalize)

	normalizedSegment := normalize(segmentPrefix)
	for _, value := range values {
		normalizedValue := normalize(value)
		if _, exists := usedValues[normalizedValue]; exists {
			continue
		}

		if normalizedSegment != "" && !strings.HasPrefix(normalizedValue, normalizedSegment) {
			continue
		}

		candidate := listPrefix + value
		if sameArg {
			candidate = "--" + flagName + "=" + candidate
		}

		_, _ = fmt.Fprintln(cmd.Root().Writer, candidate)
	}

	return true
}

func writeSingleValueCompletions(cmd *cli.Command, flagName string, values []string, prefix string, sameArg bool, normalize func(string) string) bool {
	normalizedPrefix := normalize(prefix)
	for _, value := range values {
		normalizedValue := normalize(value)
		if normalizedPrefix != "" && !strings.HasPrefix(normalizedValue, normalizedPrefix) {
			continue
		}

		candidate := value
		if sameArg {
			candidate = "--" + flagName + "=" + candidate
		}

		_, _ = fmt.Fprintln(cmd.Root().Writer, candidate)
	}

	return true
}

func completionSpecs(cmd *cli.Command) []completionSpec {
	specs := []completionSpec{
		{
			flag:      flags.Color,
			values:    staticCompletionValues(cliadapter.SupportedColorSettings()),
			normalize: normalizeLower,
		},
		{
			flag:      flags.Theme,
			values:    themeCompletionValues,
			normalize: output.NormalizeThemeName,
		},
		{
			flag:      flags.LogLevel,
			values:    staticCompletionValues(logadapter.LogLevelNames()),
			normalize: normalizeLower,
		},
	}

	if strings.HasSuffix(cmd.FullName(), "config show") {
		return append(specs, completionSpec{
			flag:      flags.Format,
			values:    staticCompletionValues([]string{output.FormatText, output.FormatJSON, "yaml"}),
			normalize: normalizeLower,
		})
	}

	if cmd.Name == commandValidate {
		return append(specs,
			completionSpec{
				flag:      flags.Format,
				values:    outputFormatCompletions,
				normalize: normalizeLower,
			},
			completionSpec{
				flag:      flags.Ref,
				values:    completionGitReferences,
				normalize: normalizeLower,
			},
			completionSpec{
				flag:      flags.BaseBranch,
				values:    completionGitBranches,
				normalize: normalizeLower,
			},
			completionSpec{
				flag:      flags.Range,
				values:    completionGitRanges,
				normalize: normalizeLower,
			},
			completionSpec{
				flag:      flags.Rule,
				values:    ruleCompletionValues,
				multi:     true,
				normalize: configTypes.NormalizeRuleName,
			},
			completionSpec{
				flag:      flags.ExcludeRule,
				values:    ruleCompletionValues,
				multi:     true,
				normalize: configTypes.NormalizeRuleName,
			},
			completionSpec{
				flag:      flags.RuleHelp,
				values:    ruleCompletionValues,
				normalize: configTypes.NormalizeRuleName,
			},
		)
	}

	return specs
}

func completionArgs() []string {
	if len(os.Args) == 0 {
		return nil
	}

	args := append([]string(nil), os.Args...)
	if args[len(args)-1] == completionRequestFlag {
		return args[:len(args)-1]
	}

	return args
}

func flagValuePrefix(flagName string, previousArg string, lastArg string) (string, bool, bool) {
	flagToken := "--" + flagName
	switch {
	case strings.HasPrefix(lastArg, flagToken+"="):
		return strings.TrimPrefix(lastArg, flagToken+"="), true, true
	case previousArg == flagToken:
		if lastArg == flagToken {
			return "", false, true
		}

		return lastArg, false, true
	case lastArg == flagToken:
		return "", false, true
	default:
		return "", false, false
	}
}

func parseListPrefix(prefix string, normalize func(string) string) (string, string, map[string]struct{}) {
	lastComma := strings.LastIndex(prefix, ",")
	if lastComma == -1 {
		return "", strings.TrimSpace(prefix), map[string]struct{}{}
	}

	rawSegment := prefix[lastComma+1:]
	segmentPrefix := strings.TrimLeft(rawSegment, " ")
	listPrefix := prefix[:lastComma+1] + rawSegment[:len(rawSegment)-len(segmentPrefix)]
	usedValues := map[string]struct{}{}

	for segment := range strings.SplitSeq(prefix[:lastComma], ",") {
		normalized := normalize(strings.TrimSpace(segment))
		if normalized != "" {
			usedValues[normalized] = struct{}{}
		}
	}

	return listPrefix, segmentPrefix, usedValues
}

func normalizeLower(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isZshCompletion() bool {
	return os.Getenv(completionShellEnv) == shellZsh
}

func staticCompletionValues(values []string) func(context.Context, *cli.Command) []string {
	return func(context.Context, *cli.Command) []string {
		return values
	}
}

func themeCompletionValues(context.Context, *cli.Command) []string {
	return output.SupportedThemes()
}

func ruleCompletionValues(context.Context, *cli.Command) []string {
	values := configTypes.GetValidRuleNames()
	slices.Sort(values)

	return values
}

func outputFormatCompletions(context.Context, *cli.Command) []string {
	return output.SupportedFormats()
}

func applyShellCompletion(cmd *cli.Command) {
	cmd.ShellComplete = shellCompleteWithValues
	for _, subcommand := range cmd.Commands {
		applyShellCompletion(subcommand)
	}
}
