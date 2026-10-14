// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	urfavecli "github.com/urfave/cli/v3"
)

// NewGlobalFlags returns the root flags shared by the production command and
// in-process integration harnesses. repoPath is non-empty only in tests that
// need a repository default without parsing another argument.
func NewGlobalFlags(localizer output.Localizer, repoPath string) []urfavecli.Flag {
	localizer = output.EnsureLocalizer(localizer)

	return []urfavecli.Flag{
		&urfavecli.StringFlag{
			Name:     flags.Config,
			Usage:    localizer.Get("load built-in defaults plus `FILE` and its imports; bypass discovered user and project configs; does not affect global hook operations"),
			Category: localizer.Get("Configuration Control"),
		},
		&urfavecli.BoolFlag{
			Name:     flags.IgnoreConfig,
			Usage:    localizer.Get("use built-in defaults and ignore all configuration files; does not affect global hook operations"),
			Category: localizer.Get("Configuration Control"),
		},
		&urfavecli.StringFlag{
			Name:     flags.RepoPath,
			Value:    repoPath,
			Usage:    localizer.Get("operate on git repository at `PATH` (default: current directory); does not affect global hook operations"),
			Category: localizer.Get("Input Control"),
		},
		&urfavecli.StringFlag{
			Name:     flags.Color,
			Value:    ColorAuto,
			Usage:    localizer.Get("control colored output: %s", strings.Join(SupportedColorSettings(), ", ")),
			Category: localizer.Get("Output Control"),
		},
		&urfavecli.StringFlag{
			Name:     flags.Theme,
			Value:    output.DefaultTheme,
			Usage:    localizer.Get("set output theme: %s", strings.Join(output.SupportedThemes(), ", ")),
			Category: localizer.Get("Output Control"),
		},
		&urfavecli.BoolFlag{
			Name:     flags.Quiet,
			Aliases:  []string{"q"},
			Usage:    localizer.Get("suppress all output except errors"),
			Category: localizer.Get("Output Control"),
		},
		&urfavecli.StringFlag{
			Name:     flags.LogLevel,
			Value:    logadapter.LogLevelInfoName,
			Usage:    localizer.Get("set logging `LEVEL`: %s", strings.Join(logadapter.LogLevelNames(), ", ")),
			Category: localizer.Get("Debug and Logging"),
		},
		&urfavecli.BoolFlag{
			Name:     flags.AllowCoredumps,
			Usage:    localizer.Get("permit kernel coredumps on crash (default: suppressed for residue minimization)"),
			Category: localizer.Get("Debug and Logging"),
			Sources:  urfavecli.EnvVars("GOMMITLINT_ALLOW_COREDUMPS"),
		},
	}
}
