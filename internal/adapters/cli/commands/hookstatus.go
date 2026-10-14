// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

// NewHookStatusCommand creates the hook status subcommand.
func NewHookStatusCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameHookStatus,
		Usage: localizer.Get("show current Gommitlint Git hook status"),
		Description: localizer.Get(`Shows the current state of local and global Gommitlint Git hooks for a repository.

Examples:
  # Show hook status for current repository
  gommitlint hook status

  # Show hook status with details (timestamps and file size)
  gommitlint hook status -v

  # Show hook status for specific repository
  gommitlint hook status --repo-path /path/to/repo`),

		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    flagNameVerbose,
				Aliases: []string{"v"},
				Usage:   localizer.Get("show detailed information (timestamps and file size)"),
			},
		},

		Action: ExecuteHookStatus,
	}
}

// ExecuteHookStatus displays hook status.
func ExecuteHookStatus(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint hook status", localizer.Get("The 'hook status' command doesn't accept arguments")); err != nil {
		return err
	}

	repoPath := GetRepoPath(cmd)
	verbose := cmd.Bool("verbose")

	status, err := defaultLocalHookService(ctx).Status(ctx, repoPath)
	if err != nil {
		err = appHookStatusError(err, localizer)

		if _, ok := errors.AsType[RecoveryError](err); ok {
			return err
		}

		return newRecoveryErrorWithCause(
			err,
			output.Msg("failed to get hook status"),
			helpSuggestion(localizer, "gommitlint hook status"),
			localizer.Get("Check repository path and hook directory"),
		)
	}

	if cmd.Root().Bool(flags.Quiet) {
		return nil
	}

	return writeCommandOutput(cmd.Writer, formatHookStatus(status, verbose, localizer), localizer)
}

// formatHookStatus formats hook status for display.
func formatHookStatus(status RepositoryHookStatus, verbose bool, localizer output.Localizer) string {
	var builder strings.Builder

	localizer = output.EnsureLocalizer(localizer)

	formatRepositoryHeader(&builder, status, localizer)
	formatHookStatusSection(&builder, status.Hooks, verbose, localizer)

	return builder.String()
}

// formatRepositoryHeader formats the repository header section.
func formatRepositoryHeader(builder *strings.Builder, status RepositoryHookStatus, localizer output.Localizer) {
	localizer = output.EnsureLocalizer(localizer)
	fmt.Fprint(builder, localizer.Get("Gommitlint Git Hooks:\n"))

	if status.HooksPathRedirected {
		fmt.Fprint(builder, localizer.Get(
			"  ! Git runs hooks from %s; local entries under %s are inactive\n",
			output.CleanForTerminal(status.EffectiveHooksPath),
			output.CleanForTerminal(status.HooksDir),
		))
	}
}

// formatHookStatusSection formats the hook status section.
func formatHookStatusSection(builder *strings.Builder, hooks []HookStatus, verbose bool, localizer output.Localizer) {
	if len(hooks) == 0 {
		fmt.Fprint(builder, localizer.Get("  No hooks directory found\n"))

		return
	}

	for _, hook := range hooks {
		if hook.Local.Exists || hook.Global.Exists {
			formatExistingHook(builder, hook, verbose, localizer)
		} else {
			fmt.Fprint(builder, localizer.Get("  - %-12s not installed\n", hook.Name))
		}
	}
}

// formatExistingHook formats an existing hook entry.
func formatExistingHook(builder *strings.Builder, hook HookStatus, verbose bool, localizer output.Localizer) {
	localizer = output.EnsureLocalizer(localizer)
	symbol, sourceInfo := hookDisplayState(hook, localizer)

	fmt.Fprint(builder, localizer.Get("  %s %-12s %s\n", symbol, hook.Name, sourceInfo))

	if verbose {
		formatVerboseHookDetails(builder, hook, localizer)
	}
}

type hookFileState uint8

const (
	hookFileInstalled hookFileState = iota
	hookFileUnknown
	hookFileNotExecutable
)

func hookDisplayState(hook HookStatus, localizer output.Localizer) (string, string) {
	localExists := hook.Local.Exists
	globalExists := hook.Global.Exists
	localState := classifyHookFile(hook.Local)
	globalState := classifyHookFile(hook.Global)

	if localExists && globalExists && localState != globalState {
		return hookStateSymbol(localState, globalState), strings.Join([]string{
			hookFileStateText(localState, localizer.Get("local"), localizer),
			hookFileStateText(globalState, localizer.Get(flagNameGlobal), localizer),
		}, " + ")
	}

	locations := make([]string, 0, 2)
	state := localState

	if localExists {
		locations = append(locations, localizer.Get("local"))
	}

	if globalExists {
		locations = append(locations, localizer.Get(flagNameGlobal))
		state = globalState
	}

	return hookStateSymbol(state), hookFileStateText(state, strings.Join(locations, " + "), localizer)
}

func classifyHookFile(status HookFileStatus) hookFileState {
	if !status.IsExecutable {
		return hookFileNotExecutable
	}

	if !status.IsGommitlint {
		return hookFileUnknown
	}

	return hookFileInstalled
}

func hookStateSymbol(states ...hookFileState) string {
	symbol := "✓"

	for _, state := range states {
		if state == hookFileNotExecutable {
			return "!"
		}

		if state == hookFileUnknown {
			symbol = "?"
		}
	}

	return symbol
}

func hookFileStateText(state hookFileState, locations string, localizer output.Localizer) string {
	switch state {
	case hookFileNotExecutable:
		return localizer.Get("not executable (%s)", locations)
	case hookFileUnknown:
		return localizer.Get("unknown source (%s)", locations)
	case hookFileInstalled:
		return localizer.Get("installed (%s)", locations)
	default:
		return ""
	}
}

// formatVerboseHookDetails formats additional details for verbose output.
func formatVerboseHookDetails(builder *strings.Builder, hook HookStatus, localizer output.Localizer) {
	localizer = output.EnsureLocalizer(localizer)
	if hook.Local.Exists && hook.Global.Exists {
		fmt.Fprintf(builder, "      %s:\n", localizer.Get("local"))
		formatVerboseHookFileDetails(builder, hook.Local, localizer)
		fmt.Fprintf(builder, "      %s:\n", localizer.Get(flagNameGlobal))
		formatVerboseHookFileDetails(builder, hook.Global, localizer)

		return
	}

	if hook.Local.Exists {
		formatVerboseHookFileDetails(builder, hook.Local, localizer)

		return
	}

	formatVerboseHookFileDetails(builder, hook.Global, localizer)
}

func formatVerboseHookFileDetails(builder *strings.Builder, hook HookFileStatus, localizer output.Localizer) {
	// Show file path
	fmt.Fprint(builder, localizer.Get("      Path: %s\n", output.CleanForTerminal(hook.Path)))

	// Show modification time
	if !hook.ModifiedTime.IsZero() {
		fmt.Fprint(builder, localizer.Get("      Modified: %s\n", hook.ModifiedTime.Format("2006-01-02 15:04:05")))
	}

	// Show file size
	fmt.Fprint(builder, localizer.Get("      Size: %d bytes\n", hook.Size))

	// Show first non-empty line of content as preview
	if hook.Content != "" {
		preview := getContentPreview(hook.Content)
		if preview != "" {
			fmt.Fprint(builder, localizer.Get("      Preview: %s\n", preview))
		}
	}
}

// getContentPreview extracts first meaningful line from hook content.
func getContentPreview(content string) string {
	lines := strings.SplitSeq(content, "\n")

	for line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip shebang and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#!") {
			continue
		}

		preview := trimmed

		// Return first comment line as description
		if after, ok := strings.CutPrefix(trimmed, "#"); ok {
			// Remove leading # and whitespace
			preview = strings.TrimSpace(after)
			if preview == "" {
				continue
			}
		}

		previewRunes := []rune(output.CleanForTerminal(preview))
		if len(previewRunes) > 60 {
			return string(previewRunes[:57]) + "..."
		}

		return string(previewRunes)
	}

	return ""
}
