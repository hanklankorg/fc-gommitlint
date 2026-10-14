// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"github.com/urfave/cli/v3"
)

const flagNameDryRun = "dry-run"

// Hook removal errors.
var (
	ErrNotGommitlintHook = errors.New("hook was not installed by gommitlint")
)

// NewHookRemoveCommand creates the hook remove subcommand.
func NewHookRemoveCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameHookRemove,
		Usage: localizer.Get("remove Gommitlint Git hooks"),
		Description: localizer.Get(`Removes Gommitlint-managed Git hooks while preserving custom hooks.

Examples:
  # Remove hooks from the current repository
  gommitlint hook remove

  # Remove global hooks
  gommitlint hook remove --global`),

		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  flagNameGlobal,
				Usage: localizer.Get("remove global hooks from $XDG_CONFIG_HOME/git/git-hooks/ (default: ~/.config/git/git-hooks/) and restore, unset, or preserve git config core.hooksPath as appropriate"),
			},
			&cli.BoolFlag{
				Name:  flagNameDryRun,
				Usage: localizer.Get("preview repository-local hook removal without changing files"),
			},
		},

		Action: ExecuteRemoveHook,
	}
}

// ExecuteRemoveHook removes git hooks either locally or globally based on command flags.
// Only removes gommitlint-generated hooks, preserving custom or third-party hooks.
// Supports dry-run mode to preview changes without modifying files.
func ExecuteRemoveHook(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint hook remove", localizer.Get("The 'hook remove' command doesn't accept arguments")); err != nil {
		return err
	}

	global := cmd.Bool(flagNameGlobal)
	dryRun := cmd.Bool(flagNameDryRun)
	repoPath := GetRepoPath(cmd)

	if global && dryRun {
		return newRecoveryErrorWithCause(
			ErrUnexpectedArgument,
			output.Msg("--global and --dry-run cannot be used together"),
			localizer.Get("Use --dry-run for repository-local hooks only"),
		)
	}

	theme, err := resolveThemeValue(cmd.Root().String(flags.Theme), localizer)
	if err != nil {
		return err
	}

	if _, err := resolveColor(cmd, localizer); err != nil {
		return err
	}

	if global {
		return executeGlobalHookRemoval(ctx, cmd, theme, localizer)
	}

	return executeLocalHookRemoval(ctx, cmd, repoPath, theme, localizer)
}

func executeGlobalHookRemoval(ctx context.Context, cmd *cli.Command, theme string, localizer output.Localizer) error {
	logger := logadapter.GetDomainLogger(ctx)
	logger.Trace("Removing global git hooks", "dispatcher", true)

	result, err := RemoveGlobalHooksWithStatusFS(ctx, filesystem.NewOSFileSystem())

	var operationErr error
	if err != nil {
		operationErr = hookRemovalError(localizer, output.Msg("global hook removal failed"), err)
		if result.HooksPath == "" {
			return operationErr
		}
	}

	if cmd.Root().Bool(flags.Quiet) {
		return operationErr
	}

	useColor := shouldUseColorForCommand(cmd)
	output := formatGlobalHookRemovalResultWithStatus(result, useColor, theme, localizer)

	writeErr := writeCommandOutput(cmd.Writer, output, localizer)

	return errors.Join(operationErr, writeErr)
}

func executeLocalHookRemoval(ctx context.Context, cmd *cli.Command, repoPath string, theme string, localizer output.Localizer) error {
	logger := logadapter.GetDomainLogger(ctx)

	dryRun := cmd.Bool(flagNameDryRun)
	if dryRun {
		return executeRemoveHookDryRun(ctx, cmd, repoPath, theme, localizer)
	}

	logger.Trace("Removing git hooks", "types", strings.Join(apphooks.DefaultHookTypes(), ","), "repo_path", repoPath)

	results := removeManagedLocalHooks(ctx, repoPath, localizer)
	operationErr := hookRemovalErrors(results, localizer)

	logger.Trace("Git hooks removal completed")

	if cmd.Root().Bool(flags.Quiet) {
		return operationErr
	}

	useColor := shouldUseColorForCommand(cmd)
	removalOutput := formatManagedHookRemovalResult(results, useColor, theme, localizer)
	writeErr := writeCommandOutput(cmd.Writer, removalOutput, localizer)

	return errors.Join(operationErr, writeErr)
}

type hookRemovalAttempt struct {
	result HookRemovalResult
	err    error
}

func removeManagedLocalHooks(ctx context.Context, repoPath string, localizer output.Localizer) map[string]hookRemovalAttempt {
	results := make(map[string]hookRemovalAttempt, len(apphooks.DefaultHookTypes()))

	for _, hookType := range apphooks.DefaultHookTypes() {
		result, err := removeSingleHook(ctx, repoPath, hookType, localizer)
		results[hookType] = hookRemovalAttempt{result: result, err: err}
	}

	return results
}

func hookRemovalErrors(results map[string]hookRemovalAttempt, localizer output.Localizer) error {
	var removalErrors []error

	for _, hookType := range apphooks.DefaultHookTypes() {
		if err := results[hookType].err; err != nil {
			removalErrors = append(removalErrors, hookRemovalError(localizer, output.Msg("%s hook removal failed", hookType), err))
		}
	}

	return errors.Join(removalErrors...)
}

// executeRemoveHookDryRun shows what would be removed without making changes.
func executeRemoveHookDryRun(ctx context.Context, cmd *cli.Command, repoPath string, theme string, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	inspections, err := inspectManagedLocalHookRemovals(ctx, repoPath, localizer)
	if err != nil {
		return err
	}

	useColor := shouldUseColorForCommand(cmd)
	output := formatManagedRemoveDryRun(inspections, useColor, theme, localizer)

	if cmd.Root().Bool(flags.Quiet) {
		return nil
	}

	return writeCommandOutput(cmd.Writer, output, localizer)
}

func inspectManagedLocalHookRemovals(ctx context.Context, repoPath string, localizer output.Localizer) (map[string]apphooks.RemovalInspection, error) {
	service := defaultLocalHookService(ctx)
	inspections := make(map[string]apphooks.RemovalInspection, len(apphooks.DefaultHookTypes()))

	for _, hookType := range apphooks.DefaultHookTypes() {
		inspection, err := service.InspectRemoval(apphooks.RemovalRequest{
			HookType: hookType,
			RepoPath: repoPath,
		})
		if err != nil {
			return nil, appHookRemovalError(err, hookType, localizer)
		}

		inspections[hookType] = inspection
	}

	return inspections, nil
}

// removeDryRunInfo contains information for formatting remove dry-run output.
type removeDryRunInfo struct {
	repoPath     string
	hookPath     string
	hookType     string
	size         int64
	content      string
	isGommitlint bool
}

func formatManagedRemoveDryRun(inspections map[string]apphooks.RemovalInspection, useColor bool, theme string, localizer output.Localizer) string {
	colors := output.GetColorScheme(useColor, theme)
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	anyExists := false

	fmt.Fprintf(&builder, "%s\n\n", colors.Header(localizer.Get("* Dry-run mode: showing what would be removed")))
	fmt.Fprint(&builder, localizer.Get("Repository: %s\n", output.CleanForTerminal(removalInspectionRepoPath(inspections))))

	for index, hookType := range apphooks.DefaultHookTypes() {
		inspection := inspections[hookType]

		if index > 0 {
			builder.WriteString("\n")
		}

		if !inspection.Exists {
			fmt.Fprint(&builder, localizer.Get("Hook type: %s\n\n", inspection.HookType))
			fmt.Fprintf(&builder, "%s\n", colors.Error(localizer.Get("✗ No %s hook found at %s", inspection.HookType, output.CleanForTerminal(inspection.HookPath))))

			continue
		}

		anyExists = true

		writeRemoveDryRunDetails(&builder, removeDryRunInfo{
			repoPath:     inspection.RepoPath,
			hookPath:     inspection.HookPath,
			hookType:     inspection.HookType,
			size:         inspection.Size,
			content:      inspection.Content,
			isGommitlint: inspection.IsGommitlint,
		}, localizer)
	}

	if !anyExists {
		fmt.Fprint(&builder, localizer.Get("\nNothing to remove\n"))
	}

	return builder.String()
}

func removalInspectionRepoPath(inspections map[string]apphooks.RemovalInspection) string {
	for _, hookType := range apphooks.DefaultHookTypes() {
		if repoPath := inspections[hookType].RepoPath; repoPath != "" {
			return repoPath
		}
	}

	return ""
}

func writeRemoveDryRunDetails(builder *strings.Builder, info removeDryRunInfo, localizer output.Localizer) {
	safeHookPath := output.CleanForTerminal(info.hookPath)
	fmt.Fprint(builder, localizer.Get("Hook type: %s\n", info.hookType))
	fmt.Fprint(builder, localizer.Get("Target path: %s\n\n", safeHookPath))

	// Current state
	fmt.Fprint(builder, localizer.Get("Current state:\n"))

	bytePluralCount := int(info.size)
	fmt.Fprint(builder, localizer.GetN("  • Hook exists: %s (%d byte)\n", "  • Hook exists: %s (%d bytes)\n", bytePluralCount, safeHookPath, info.size))

	if info.isGommitlint {
		fmt.Fprint(builder, localizer.Get("  • Type: gommitlint hook\n"))
		fmt.Fprint(builder, localizer.Get("  • Action: Would remove\n"))
	} else {
		fmt.Fprint(builder, localizer.Get("  • Type: custom hook (not installed by gommitlint)\n"))
		fmt.Fprint(builder, localizer.Get("  • Action: Would refuse removal\n"))
	}

	// Show content preview for custom hooks
	if !info.isGommitlint {
		lines := strings.Split(info.content, "\n")
		previewLines := min(5, len(lines))

		fmt.Fprint(builder, localizer.GetN("\nCustom hook preview (first %d line):\n", "\nCustom hook preview (first %d lines):\n", previewLines, previewLines))
		fmt.Fprint(builder, "```\n")

		for i := range previewLines {
			fmt.Fprintf(builder, "%s\n", boundedHookPreviewLine(lines[i]))
		}

		if len(lines) > previewLines {
			remainingLines := len(lines) - previewLines
			fmt.Fprint(builder, localizer.GetN("... (%d more line)\n", "... (%d more lines)\n", remainingLines, remainingLines))
		}

		fmt.Fprint(builder, "```\n")
	}

	if !info.isGommitlint {
		fmt.Fprint(builder, localizer.Get("\nNext steps:\n"))
		fmt.Fprint(builder, localizer.Get("  • Keep the custom hook, or remove it manually after review\n"))
		fmt.Fprint(builder, localizer.Get("  • Remove manually if needed: rm -- %s\n", shellQuote(safeHookPath)))
		fmt.Fprint(builder, localizer.Get("  • View current hook: cat -- %s\n", shellQuote(safeHookPath)))
		fmt.Fprint(builder, localizer.Get("  • Check hook status: gommitlint hook status\n"))

		return
	}

	// What would happen
	fmt.Fprint(builder, localizer.Get("\nWould remove:\n"))
	fmt.Fprint(builder, localizer.Get("  • File: %s\n", safeHookPath))

	if info.hookType == hookTypePrePush {
		fmt.Fprint(builder, localizer.Get("  • Impact: Configured signature validation will no longer run automatically before pushes\n"))
	} else {
		fmt.Fprint(builder, localizer.Get("  • Impact: Commit messages will no longer be automatically validated\n"))
	}

	fmt.Fprint(builder, localizer.Get("  • Recovery: Can reinstall with 'gommitlint hook install'\n"))

	// Next steps
	fmt.Fprint(builder, localizer.Get("\nNext steps:\n"))

	fmt.Fprint(builder, localizer.Get("  • Proceed with removal: gommitlint hook remove\n"))
	fmt.Fprint(builder, localizer.Get("  • View current hook: cat -- %s\n", shellQuote(safeHookPath)))
	fmt.Fprint(builder, localizer.Get("  • Check hook status: gommitlint hook status\n"))
}

func boundedHookPreviewLine(line string) string {
	const maxPreviewRunes = 120

	runes := []rune(output.CleanForTerminal(line))
	if len(runes) <= maxPreviewRunes {
		return string(runes)
	}

	return string(runes[:maxPreviewRunes-3]) + "..."
}

func removeSingleHook(ctx context.Context, repoPath string, hookType string, localizer output.Localizer) (HookRemovalResult, error) {
	result, err := defaultLocalHookService(ctx).Remove(apphooks.RemovalRequest{
		HookType: hookType,
		RepoPath: repoPath,
	})
	if err != nil {
		return HookRemovalResult{}, appHookRemovalError(err, hookType, localizer)
	}

	return result, nil
}

// formatGlobalHookRemovalResultWithStatus formats global removal with per-hook status.
func formatGlobalHookRemovalResultWithStatus(result GlobalRemovalResult, useColor bool, theme string, localizer output.Localizer) string {
	colors := output.GetColorScheme(useColor, theme)
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	hooksPath := result.HooksPath
	if hooksPath == "" {
		hooksPath = GlobalHooksPath()
	}

	safeHooksPath := output.CleanForTerminal(hooksPath)

	fmt.Fprint(&builder, localizer.Get("Global hooks directory: %s\n\n", safeHooksPath))
	fmt.Fprint(&builder, localizer.Get("Removing global git hooks...\n\n"))

	anyFound := formatGlobalHookRemovalStatuses(&builder, result.Hooks, colors, localizer)
	anyRemoved := result.AnyRemoved()

	if len(result.ForeignHooks) > 0 {
		fmt.Fprint(&builder, localizer.Get("\nOther files kept in the global hooks directory:\n"))

		for _, path := range result.ForeignHooks {
			fmt.Fprint(&builder, localizer.Get("  • %s\n", output.CleanForTerminal(path)))
		}
	}

	formatGlobalHooksPathAction(&builder, result, localizer)

	if result.Incomplete {
		fmt.Fprintf(&builder, "\n%s\n", colors.Error(localizer.Get("✗ Global hook removal stopped before all operations completed")))
		fmt.Fprint(&builder, localizer.Get("Review the statuses above before retrying.\n"))

		return builder.String()
	}

	if !anyFound {
		fmt.Fprint(&builder, localizer.Get("\nNo global hooks found to remove\n\n"))
		fmt.Fprint(&builder, localizer.Get("Next steps:\n"))
		fmt.Fprint(&builder, localizer.Get("  • Install globally: gommitlint hook install --global\n"))
		fmt.Fprint(&builder, localizer.Get("  • Install locally: gommitlint hook install\n"))

		return builder.String()
	}

	if anyRemoved {
		fmt.Fprintf(&builder, "\n%s\n\n", colors.Success(localizer.Get("✓ Global git hooks removed successfully!")))
	} else {
		fmt.Fprint(&builder, localizer.Get("\nNo gommitlint hooks found to remove\n\n"))
	}

	fmt.Fprint(&builder, localizer.Get("\nNext steps:\n"))
	fmt.Fprint(&builder, localizer.Get("  • Reinstall globally: gommitlint hook install --global\n"))
	fmt.Fprint(&builder, localizer.Get("  • Install locally: gommitlint hook install\n"))

	return builder.String()
}

func formatGlobalHookRemovalStatuses(builder *strings.Builder, statuses []apphooks.GlobalHookRemovalStatus, colors output.ColorScheme, localizer output.Localizer) bool {
	anyFound := false

	for _, status := range statuses {
		fmt.Fprintf(builder, "%s\n", localizer.Get("%s hook:", status.HookType))

		switch {
		case status.Removed:
			fmt.Fprintf(builder, "  %s\n", colors.Success(localizer.Get("✓ Removed")))
		case status.Found:
			fmt.Fprintf(builder, "  %s\n", colors.Info(localizer.Get("ℹ Not a gommitlint hook")))
		default:
			fmt.Fprintf(builder, "  %s\n", colors.Warning(localizer.Get("⚠ Hook not found")))
		}

		if len(status.ForeignPlugins) > 0 {
			fmt.Fprintf(builder, "  %s\n", colors.Info(localizer.Get("ℹ Dispatcher kept: other tools still plug into it")))

			for _, plugin := range status.ForeignPlugins {
				fmt.Fprint(builder, localizer.Get("    • %s\n", output.CleanForTerminal(plugin)))
			}
		}

		if status.ForeignDispatcher {
			fmt.Fprintf(builder, "  %s\n", colors.Info(localizer.Get("ℹ Custom dispatcher kept")))
		}

		anyFound = anyFound || status.Found
	}

	return anyFound
}

func formatGlobalHooksPathAction(builder *strings.Builder, result GlobalRemovalResult, localizer output.Localizer) {
	fmt.Fprint(builder, localizer.Get("\nConfiguration:\n"))

	switch result.HooksPathAction {
	case apphooks.HooksPathUnset:
		fmt.Fprint(builder, localizer.Get("  • Git config 'core.hooksPath' unset\n"))
		fmt.Fprint(builder, localizer.Get("  • Repositories now use local .git/hooks/ again\n"))
	case apphooks.HooksPathRestored:
		fmt.Fprint(builder, localizer.Get("  • Git config 'core.hooksPath' restored to: %s\n", output.CleanForTerminal(result.RestoredHooksPath)))
	case apphooks.HooksPathUntouched, apphooks.HooksPathSet, apphooks.HooksPathKept, apphooks.HooksPathReplaced:
		fmt.Fprint(builder, localizer.Get("  • Git config 'core.hooksPath' left unchanged\n"))
	}
}

func formatManagedHookRemovalResult(results map[string]hookRemovalAttempt, useColor bool, theme string, localizer output.Localizer) string {
	colors := output.GetColorScheme(useColor, theme)
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	repoPath := hookRemovalRepoPath(results)

	fmt.Fprint(&builder, localizer.Get("Repository: %s\n", output.CleanForTerminal(repoPath)))
	fmt.Fprint(&builder, localizer.Get("Removing git hooks...\n\n"))

	anyRemoved := false
	hasError := false

	for _, hookType := range apphooks.DefaultHookTypes() {
		attempt := results[hookType]
		fmt.Fprint(&builder, localizer.Get("%s hook:\n", hookType))
		formatSingleHookStatus(&builder, attempt.result.HookPath, attempt.err, colors, localizer)

		anyRemoved = anyRemoved || attempt.err == nil && attempt.result.HookPath != ""
		hasError = hasError || attempt.err != nil
	}

	formatRemovalSummary(&builder, hasError, anyRemoved, colors, localizer)

	return builder.String()
}

func hookRemovalRepoPath(results map[string]hookRemovalAttempt) string {
	for _, hookType := range apphooks.DefaultHookTypes() {
		if repoPath := results[hookType].result.RepoPath; repoPath != "" {
			return repoPath
		}
	}

	return ""
}

func formatSingleHookStatus(builder *strings.Builder, hookPath string, err error, colors output.ColorScheme, localizer output.Localizer) {
	localizer = output.EnsureLocalizer(localizer)

	switch {
	case err == nil && hookPath == "":
		fmt.Fprintf(builder, "  %s\n", colors.Warning(localizer.Get("⚠ Hook not found")))
	case err == nil:
		fmt.Fprintf(builder, "  %s\n", colors.Success(localizer.Get("✓ Removed: %s", output.CleanForTerminal(hookPath))))
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(builder, "  %s\n", colors.Warning(localizer.Get("⚠ Hook not found")))
	default:
		message := err.Error()

		if recovery, ok := errors.AsType[RecoveryError](err); ok {
			message = recovery.Msg.Localize(localizer)
		}

		fmt.Fprintf(builder, "  %s\n", colors.Error(localizer.Get("✗ Not removed: %s", output.CleanForTerminal(message))))
	}
}

func formatRemovalSummary(builder *strings.Builder, hasError bool, anyRemoved bool, colors output.ColorScheme, localizer output.Localizer) {
	localizer = output.EnsureLocalizer(localizer)

	if hasError {
		return // Errors already displayed
	}

	if anyRemoved {
		fmt.Fprintf(builder, "%s\n\n", colors.Success(localizer.Get("✓ Git hooks removed successfully!")))
		fmt.Fprint(builder, localizer.Get("Next steps:\n"))
		fmt.Fprint(builder, localizer.Get("  • Reinstall: gommitlint hook install\n"))
		fmt.Fprint(builder, localizer.Get("  • Check status: gommitlint hook status\n"))
	} else {
		fmt.Fprintf(builder, "%s\n\n", colors.Warning(localizer.Get("No local hooks found to remove")))
		fmt.Fprint(builder, localizer.Get("Next steps:\n"))
		fmt.Fprint(builder, localizer.Get("  • Install hooks: gommitlint hook install\n"))
		fmt.Fprint(builder, localizer.Get("  • Check status: gommitlint hook status\n"))
	}
}

func hookRemovalError(localizer output.Localizer, msg output.LocaleMsg, err error) error {
	if _, ok := errors.AsType[RecoveryError](err); ok {
		return err
	}

	localizer = output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		commandOperationFailure(err),
		msg,
		helpSuggestion(localizer, "gommitlint hook remove"),
		localizer.Get("Check repository path and hook permissions"),
	)
}
