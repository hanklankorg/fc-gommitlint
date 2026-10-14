// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/urfave/cli/v3"
)

type doctorReport struct {
	ConfigSource string
	Hooks        RepositoryHookStatus
	Policy       hookInstallPolicy
	Warnings     []domain.Warning
}

// NewDoctorCommand creates the doctor command.
func NewDoctorCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  commandNameDoctor,
		Usage: localizer.Get("inspect configuration, hooks, and effective policy"),
		Description: localizer.Get(`Inspects the current repository without modifying files.

Examples:
  gommitlint doctor
  gommitlint --repo-path /path/to/repo doctor`),
		Action: ExecuteDoctor,
	}
}

// ExecuteDoctor inspects repository configuration and hook status without modifying files.
func ExecuteDoctor(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()
	if err := rejectUnexpectedArgs(cmd, localizer, "gommitlint doctor", localizer.Get("The 'doctor' command doesn't accept arguments")); err != nil {
		return err
	}

	cfgResult, err := LoadConfigFromCommand(cmd.Root())
	if err != nil {
		return configLoadError(err, localizer)
	}

	status, err := defaultLocalHookService(ctx).Status(ctx, GetRepoPath(cmd))
	if err != nil {
		return appHookStatusError(err, localizer)
	}

	report := doctorReport{
		ConfigSource: cfgResult.DisplaySource(),
		Hooks:        status,
		Policy:       hookInstallPolicyFromConfig(cfgResult.Config),
		Warnings:     cfgResult.Warnings,
	}

	return writeCommandOutput(cmd.Writer, formatDoctorReport(report, localizer), localizer)
}

func formatDoctorReport(report doctorReport, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	var builder strings.Builder

	fmt.Fprint(&builder, localizer.Get("Gommitlint Doctor\n"))
	fmt.Fprint(&builder, "================\n")
	fmt.Fprint(&builder, localizer.Get("Repository: %s\n", output.CleanForTerminal(report.Hooks.RepoPath)))
	fmt.Fprint(&builder, localizer.Get("Configuration: %s\n", output.CleanForTerminal(report.ConfigSource)))
	fmt.Fprint(&builder, localizer.Get("Warnings: %d\n\n", len(report.Warnings)))

	formatDoctorHooks(&builder, report.Hooks, localizer)
	fmt.Fprint(&builder, "\n")
	formatDoctorPolicy(&builder, report.Policy, localizer)
	fmt.Fprint(&builder, "\n")
	formatDoctorNextSteps(&builder, report, localizer)

	return builder.String()
}

func formatDoctorHooks(builder *strings.Builder, status RepositoryHookStatus, localizer output.Localizer) {
	fmt.Fprint(builder, localizer.Get("Hooks:\n"))

	if doctorHooksPathRedirectedAwayFromGlobal(status) {
		fmt.Fprintf(builder, "  ! %s\n", localizer.Get(
			"this repository's hooks run from %s (core.hooksPath), so a hook in .git/hooks would never run",
			output.CleanForTerminal(status.EffectiveHooksPath),
		))
	}

	if len(status.Hooks) == 0 {
		fmt.Fprint(builder, localizer.Get("  - no hook directory found\n"))

		return
	}

	for _, hook := range status.Hooks {
		activeHook := doctorActiveHook(status, hook)
		fmt.Fprint(builder, localizer.Get("  - %-12s %s\n", output.CleanForTerminal(hook.Name), doctorHookState(activeHook, localizer)))
	}
}

func doctorHookState(hook HookStatus, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)
	if !hook.Local.Exists && !hook.Global.Exists {
		return localizer.Get("not installed")
	}

	_, state := hookDisplayState(hook, localizer)

	return state
}

func formatDoctorPolicy(builder *strings.Builder, policy hookInstallPolicy, localizer output.Localizer) {
	fmt.Fprint(builder, localizer.Get("Policy:\n"))
	fmt.Fprint(builder, localizer.Get("  - sign-off required: %s\n", doctorBool(policy.SignoffRequired, localizer)))
	fmt.Fprint(builder, localizer.Get("  - signature required: %s\n", doctorBool(policy.SignatureRequired, localizer)))
	fmt.Fprint(builder, localizer.Get("  - test command: %s\n", policy.TestCommand()))
}

func doctorBool(value bool, localizer output.Localizer) string {
	if value {
		return localizer.Get("yes")
	}

	return localizer.Get("no")
}

func formatDoctorNextSteps(builder *strings.Builder, report doctorReport, localizer output.Localizer) {
	fmt.Fprint(builder, localizer.Get("Next steps:\n"))

	if len(report.Warnings) > 0 {
		fmt.Fprint(builder, localizer.Get("  - review config warnings: gommitlint config check\n"))
	}

	if doctorHooksNeedInstall(report.Hooks) {
		switch {
		case doctorHooksPathRedirectedAwayFromGlobal(report.Hooks):
			fmt.Fprintf(builder, "  - %s\n", localizer.Get("Or run per-repository hooks here: git config core.hooksPath .git/hooks"))
			fmt.Fprint(builder, localizer.Get("  - install hooks: gommitlint hook install\n"))
		case doctorUsesGlobalHooks(report.Hooks):
			fmt.Fprintf(builder, "  - %s\n", localizer.Get("Install globally instead: gommitlint hook install --global"))
		default:
			fmt.Fprint(builder, localizer.Get("  - install hooks: gommitlint hook install\n"))
		}
	}

	fmt.Fprint(builder, localizer.Get("  - validate commits: gommitlint validate\n"))
}

func doctorHooksNeedInstall(status RepositoryHookStatus) bool {
	if len(status.Hooks) == 0 {
		return true
	}

	for _, hook := range status.Hooks {
		activeHook := doctorActiveHook(status, hook)
		if !healthyHookFile(activeHook.Local) && !healthyHookFile(activeHook.Global) {
			return true
		}
	}

	return false
}

func healthyHookFile(status HookFileStatus) bool {
	return status.Exists && status.IsExecutable && status.IsGommitlint
}

func doctorActiveHook(status RepositoryHookStatus, hook HookStatus) HookStatus {
	active := HookStatus{Name: hook.Name}
	if !status.HooksPathRedirected {
		active.Local = hook.Local

		return active
	}

	if !doctorHooksPathRedirectedAwayFromGlobal(status) {
		active.Global = hook.Global
	}

	return active
}

func doctorHooksPathRedirectedAwayFromGlobal(status RepositoryHookStatus) bool {
	return status.HooksPathRedirected && !doctorUsesGlobalHooks(status)
}

func doctorUsesGlobalHooks(status RepositoryHookStatus) bool {
	return status.HooksPathRedirected && status.GlobalHooksPath != "" &&
		filepath.Clean(status.EffectiveHooksPath) == filepath.Clean(status.GlobalHooksPath)
}
