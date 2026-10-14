// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/scripts/hooks"
	"github.com/urfave/cli/v3"
)

// NewHookInstallCommand creates the hook install subcommand.
func NewHookInstallCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameHookInstall,
		Usage: localizer.Get("install Gommitlint Git hooks"),
		Description: localizer.Get(`Installs commit-msg and pre-push Git hooks for automatic validation.

Examples:
  # Install hooks in the current repository
  gommitlint hook install

  # Replace existing Gommitlint hooks
  gommitlint hook install --force

  # Install global hooks
  gommitlint hook install --global`),

		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    flagNameForce,
				Aliases: []string{"f"},
				Usage:   localizer.Get("replace existing Gommitlint hooks or another global core.hooksPath"),
			},
			&cli.BoolFlag{
				Name:  flagNameGlobal,
				Usage: localizer.Get("install hooks globally to $XDG_CONFIG_HOME/git/git-hooks/ (default: ~/.config/git/git-hooks/) and configure git config core.hooksPath"),
			},
		},

		Action: ExecuteInstallHook,
	}
}

// ExecuteInstallHook installs git hooks either locally or globally based on command flags.
// Local installation creates commit-msg and pre-push hooks in .git/hooks/.
// Global installation uses the dispatcher pattern under the XDG config directory.
func ExecuteInstallHook(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint hook install", localizer.Get("This command does not accept positional arguments")); err != nil {
		return err
	}

	logger := logadapter.GetDomainLogger(ctx)
	force := cmd.Bool(flagNameForce)
	global := cmd.Bool(flagNameGlobal)
	repoPath := GetRepoPath(cmd)
	quiet := cmd.Root().Bool(flags.Quiet)

	theme, err := resolveThemeValue(cmd.Root().String(flags.Theme), localizer)
	if err != nil {
		return err
	}

	if _, err := resolveColor(cmd, localizer); err != nil {
		return err
	}

	// Handle global installation
	if global {
		logger.Trace("Installing global git hooks", "dispatcher", true)

		result, err := InstallGlobalHooks(ctx, force, localizer)
		if err != nil {
			return hookInstallError(flagNameGlobal, err, localizer.Get("Retry with: gommitlint hook install --global --force"), localizer)
		}

		if quiet {
			return nil
		}

		useColor := shouldUseColorForCommand(cmd)
		output := formatGlobalHookInstallResult(result, useColor, theme, localizer)

		if err := writeCommandOutput(cmd.Writer, output, localizer); err != nil {
			return err
		}

		return nil
	}

	// Install both commit-msg and pre-push hooks (local mode)
	policy, err := hookInstallPolicyFromCommand(cmd, localizer)
	if err != nil {
		return err
	}

	logger.Trace("Installing git hooks", "types", strings.Join(apphooks.DefaultHookTypes(), ","), "repo_path", repoPath, "force", force)

	results, err := installManagedLocalHooks(ctx, force, repoPath, localizer)
	if err != nil {
		return err
	}

	logger.Trace("Git hooks installation completed", "success", true)

	if quiet {
		return nil
	}

	useColor := shouldUseColorForCommand(cmd)
	output := formatManagedHookInstallResults(results, useColor, theme, localizer, policy)

	return writeCommandOutput(cmd.Writer, output, localizer)
}

func installManagedLocalHooks(ctx context.Context, force bool, repoPath string, localizer output.Localizer) (map[string]HookInstallResult, error) {
	results := make(map[string]HookInstallResult, len(apphooks.DefaultHookTypes()))
	service := newLocalHookService(ctx, filesystem.NewOSFileSystem())
	scripts := make(map[string]string, len(apphooks.DefaultHookTypes()))

	for _, hookType := range apphooks.DefaultHookTypes() {
		script, err := createHookScript(hookType)
		if err != nil {
			return nil, err
		}

		scripts[hookType] = script

		if err := service.PreflightInstall(apphooks.InstallRequest{Force: force, HookType: hookType, RepoPath: repoPath}); err != nil {
			return nil, hookInstallError(hookType, appHookInstallError(err, hookType, localizer), localizer.Get("Retry with: gommitlint hook install --force"), localizer)
		}
	}

	for _, hookType := range apphooks.DefaultHookTypes() {
		result, err := installSingleHookWithService(service, force, repoPath, hookType, scripts[hookType], localizer)
		if err != nil {
			return nil, hookInstallError(hookType, err, localizer.Get("Retry with: gommitlint hook install --force"), localizer)
		}

		results[hookType] = result
	}

	return results, nil
}

//nolint:unparam // This test seam mirrors force-capable installation even though current direct callers do not force.
func installSingleHook(ctx context.Context, force bool, repoPath string, hookType string, hookScript string, localizer output.Localizer) (HookInstallResult, error) {
	return installSingleHookWithService(newLocalHookService(ctx, filesystem.NewOSFileSystem()), force, repoPath, hookType, hookScript, localizer)
}

//nolint:unparam // The filesystem-injection seam supports every hook type even though current tests use commit-msg.
func installSingleHookWithFS(ctx context.Context, fs filesystem.FileSystem, force bool, repoPath string, hookType string, hookScript string, localizer output.Localizer) (HookInstallResult, error) {
	service := newLocalHookServiceWithRedirectCheck(ctx, fs, nil)

	return installSingleHookWithService(service, force, repoPath, hookType, hookScript, localizer)
}

func installSingleHookWithService(service apphooks.Service, force bool, repoPath string, hookType string, hookScript string, localizer output.Localizer) (HookInstallResult, error) {
	result, err := service.Install(apphooks.InstallRequest{
		Force:    force,
		HookType: hookType,
		RepoPath: repoPath,
		Script:   hookScript,
	})
	if err != nil {
		return HookInstallResult{}, appHookInstallError(err, hookType, localizer)
	}

	return result, nil
}

// formatGlobalHookInstallResult formats global installation result for display.
func formatGlobalHookInstallResult(result GlobalInstallResult, useColor bool, theme string, localizer output.Localizer) string {
	colors := output.GetColorScheme(useColor, theme)
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	hooksPath := result.HooksPath
	if hooksPath == "" {
		hooksPath = GlobalHooksPath()
	}

	safeHooksPath := output.CleanForTerminal(hooksPath)
	fmt.Fprint(&builder, localizer.Get("Global hooks directory: %s\n\n", safeHooksPath))
	fmt.Fprint(&builder, localizer.Get("Installing global git hooks using dispatcher pattern...\n\n"))

	fmt.Fprint(&builder, localizer.Get("Dispatchers installed:\n"))

	hookTypes := result.HookTypes
	if len(hookTypes) == 0 {
		hookTypes = apphooks.DefaultHookTypes()
	}

	for _, hookType := range hookTypes {
		path := output.CleanForTerminal(apphooks.GlobalHookDispatcherPath(hooksPath, hookType))
		fmt.Fprint(&builder, localizer.Get("  • %s: %s\n", hookType, path))
	}

	fmt.Fprint(&builder, localizer.Get("\nPlugins installed:\n"))

	for _, hookType := range hookTypes {
		path := output.CleanForTerminal(apphooks.GlobalHookPluginScriptPath(hooksPath, hookType))
		fmt.Fprint(&builder, localizer.Get("  • %s: %s\n", hookType, path))
	}

	fmt.Fprintf(&builder, "\n%s\n\n", colors.Success(localizer.Get("✓ Global git hooks installed successfully!")))
	fmt.Fprint(&builder, localizer.Get("Configuration:\n"))

	switch result.HooksPathAction {
	case apphooks.HooksPathKept:
		fmt.Fprint(&builder, localizer.Get("  • Git config 'core.hooksPath' already pointed here: %s\n", safeHooksPath))
	case apphooks.HooksPathReplaced:
		fmt.Fprint(&builder, localizer.Get("  • Git config 'core.hooksPath' set to: %s\n", safeHooksPath))
		fmt.Fprintf(&builder, "  %s\n", colors.Warning(localizer.Get("⚠ Replaced previous value %s (restored by 'hook remove --global')", output.CleanForTerminal(result.PreviousHooksPath))))
	case apphooks.HooksPathSet, apphooks.HooksPathUnset, apphooks.HooksPathRestored, apphooks.HooksPathUntouched:
		fmt.Fprint(&builder, localizer.Get("  • Git config 'core.hooksPath' set to: %s\n", safeHooksPath))
	}

	fmt.Fprint(&builder, localizer.Get("\nHow it works:\n"))
	fmt.Fprint(&builder, localizer.Get("  • All repositories now use these global hooks\n"))
	fmt.Fprint(&builder, localizer.Get("  • A global core.hooksPath disables per-repository .git/hooks (husky, lefthook, ...)\n"))
	fmt.Fprint(&builder, localizer.Get("  • Keep per-repository hooks where needed: git config core.hooksPath .git/hooks\n"))
	fmt.Fprint(&builder, localizer.Get("  • Dispatcher pattern allows multiple tools to coexist\n"))
	fmt.Fprint(&builder, localizer.Get("  • Add more tools by placing scripts in *.d/ directories\n"))
	fmt.Fprint(&builder, localizer.Get("  • Remove with: gommitlint hook remove --global\n"))
	fmt.Fprint(&builder, localizer.Get("\nNext steps:\n"))
	fmt.Fprint(&builder, localizer.Get("  • Inspect a repository: gommitlint doctor\n"))
	fmt.Fprint(&builder, localizer.Get("  • Test after reviewing that repository's active config\n"))
	fmt.Fprint(&builder, localizer.Get("  • View dispatchers: ls -la -- %s\n", shellQuote(safeHooksPath)))

	pluginPath := output.CleanForTerminal(apphooks.GlobalHookPluginPath(hooksPath, firstManagedHookType()))
	fmt.Fprint(&builder, localizer.Get("  • View plugins: ls -la -- %s\n", shellQuote(pluginPath)))

	return builder.String()
}

// formatManagedHookInstallResults formats local installation results for display.
func formatManagedHookInstallResults(results map[string]HookInstallResult, useColor bool, theme string, localizer output.Localizer, policy hookInstallPolicy) string {
	colors := output.GetColorScheme(useColor, theme)
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	repoPath := hookInstallRepoPath(results)

	fmt.Fprint(&builder, localizer.Get("Repository: %s\n", output.CleanForTerminal(repoPath)))
	fmt.Fprint(&builder, localizer.Get("Installing git hooks...\n\n"))

	for _, hookType := range apphooks.DefaultHookTypes() {
		result := results[hookType]
		fmt.Fprint(&builder, localizer.Get("%s hook:\n", hookType))
		fmt.Fprint(&builder, localizer.Get("  • Installed: %s\n", output.CleanForTerminal(result.HookPath)))

		if result.WasOverwritten {
			fmt.Fprintf(&builder, "  • %s\n", colors.Warning(localizer.Get("Overwritten: previous hook replaced")))
		}
	}

	fmt.Fprintf(&builder, "\n%s\n\n", colors.Success(localizer.Get("✓ Git hooks installed successfully!")))
	fmt.Fprint(&builder, localizer.Get("Active policy:\n"))
	fmt.Fprint(&builder, localizer.Get("  • commit-msg: message rules from active config\n"))
	fmt.Fprint(&builder, localizer.Get("  • pre-push: signature checks only when configured\n"))
	fmt.Fprint(&builder, "\n")
	fmt.Fprint(&builder, localizer.Get("Next steps:\n"))
	fmt.Fprint(&builder, localizer.Get("  • Test: %s\n", policy.TestCommand()))
	fmt.Fprint(&builder, localizer.Get("  • Status: gommitlint hook status\n"))
	fmt.Fprint(&builder, localizer.Get("  • Inspect setup: gommitlint doctor\n"))

	return builder.String()
}

type hookInstallPolicy struct {
	SignoffRequired   bool
	MinSignoffCount   int
	SignatureRequired bool
}

func hookInstallPolicyFromCommand(cmd *cli.Command, localizer output.Localizer) (hookInstallPolicy, error) {
	cfgResult, err := LoadConfigFromCommand(cmd.Root())
	if err != nil {
		return hookInstallPolicy{}, configLoadError(err, localizer)
	}

	return hookInstallPolicyFromConfig(cfgResult.Config), nil
}

func hookInstallPolicyFromConfig(cfg configTypes.Config) hookInstallPolicy {
	minSignoffCount := configTypes.DerefInt(cfg.Message.Body.MinSignoffCount, configTypes.DefaultMinSignoffCount)
	signoffRequired := rulefactory.IsCommitRuleEnabled(cfg, nil, rulecatalog.SignOff) && minSignoffCount > 0

	return hookInstallPolicy{
		SignoffRequired: signoffRequired,
		MinSignoffCount: minSignoffCount,
		SignatureRequired: rulefactory.IsCommitRuleEnabled(cfg, nil, rulecatalog.CryptoSignature) &&
			configTypes.DerefBool(cfg.CryptoSignature.Required, false),
	}
}

func (p hookInstallPolicy) TestCommand() string {
	args := []string{"git", "commit"}

	if p.SignoffRequired {
		args = append(args, "--signoff")
	}

	if p.SignatureRequired {
		args = append(args, "-S")
	}

	args = append(args, "-m", "\"feat: your message\"")

	if p.SignoffRequired && p.MinSignoffCount > 1 {
		additionalSignoffs := make([]string, 0, p.MinSignoffCount-1)
		for signer := 2; signer <= p.MinSignoffCount; signer++ {
			additionalSignoffs = append(additionalSignoffs, fmt.Sprintf("Signed-off-by: Signer %d <signer%d@example.com>", signer, signer))
		}

		args = append(args, "-m", shellQuote(strings.Join(additionalSignoffs, "\n")))
	}

	return strings.Join(args, " ")
}

func hookInstallRepoPath(results map[string]HookInstallResult) string {
	for _, hookType := range apphooks.DefaultHookTypes() {
		if repoPath := results[hookType].RepoPath; repoPath != "" {
			return repoPath
		}
	}

	return ""
}

func firstManagedHookType() string {
	hookTypes := apphooks.DefaultHookTypes()
	if len(hookTypes) == 0 {
		return hookTypeCommitMsg
	}

	return hookTypes[0]
}

// createCommitMsgHookScript returns the embedded commit-msg hook script.
func createCommitMsgHookScript() string {
	return hooks.WithInstalledBinary(hooks.CommitMsgHook, installedBinaryPath())
}

func createPrePushHookScript() string {
	return hooks.WithInstalledBinary(hooks.PrePushHook, installedBinaryPath())
}

// installedBinaryPath returns the absolute path of the running gommitlint
// binary, baked into generated hooks as a fallback when PATH lacks it. An
// unresolvable path yields "", and the hook then fails closed with guidance.
func installedBinaryPath() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}

	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}

	if !filepath.IsAbs(executable) {
		return ""
	}

	return executable
}

func createHookScript(hookType string) (string, error) {
	switch hookType {
	case hookTypeCommitMsg:
		return createCommitMsgHookScript(), nil
	case hookTypePrePush:
		return createPrePushHookScript(), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedHookType, hookType)
	}
}

// shouldUseColorForCommand determines if colors should be used based on CLI settings.
func shouldUseColorForCommand(cmd *cli.Command) bool {
	// Get color setting from root command
	colorSetting := cmd.Root().String(flagNameColor)

	// Create minimal output options to determine color usage
	outputOptions := cliAdapter.NewOutputOptions(cmd.Writer).WithColor(colorSetting)

	return outputOptions.ShouldUseColor()
}
