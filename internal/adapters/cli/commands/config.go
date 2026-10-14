// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// Command errors.
var (
	ErrConfigCheckFailed       = errors.New("configuration check failed")
	ErrUnsupportedConfigFormat = errors.New("unsupported config format")
	ErrUnsupportedConfigView   = errors.New("unsupported config view")
	ErrConfigDependencyFailed  = errors.New("configuration dependency failed")
)

const (
	colorModeAlways          = "always"
	colorModeAuto            = "auto"
	colorModeNever           = "never"
	commandNameCheck         = "check"
	commandNameDoctor        = "doctor"
	commandNameConfigInit    = "init"
	commandNameConfigSchema  = "schema"
	commandNameConfigShow    = "show"
	commandNameConfigSync    = "sync"
	commandNameHookInstall   = "install"
	commandNameHookRemove    = "remove"
	commandNameHookStatus    = "status"
	commandNameValidate      = "validate"
	configCheckCodeWarning   = "config_warning"
	configCheckSourceWarning = "warning"
	gommitlintKey            = "gommitlint"

	configViewCompact = "compact"
	configViewVerbose = "verbose"
)

// Flag-name aliases onto the canonical owner in the flags package. Kept as
// package-local names so the many existing references read naturally; the
// string literals themselves live only in flags, so cmd and commands cannot
// drift apart.
const (
	flagNameBaseBranch      = flags.BaseBranch
	flagNameColor           = flags.Color
	flagNameCompact         = flags.Compact
	flagNameCount           = flags.Count
	flagNameExcludeRule     = flags.ExcludeRule
	flagNameFormat          = flags.Format
	flagNameForce           = flags.Force
	flagNameGlobal          = flags.Global
	flagNameConfig          = flags.Config
	flagNameIgnoreConfig    = flags.IgnoreConfig
	flagNameIgnoreRepoRules = flags.IgnoreRepoRules
	flagNameMessageFile     = flags.MessageFile
	flagNameOutput          = flags.Output
	flagNamePreset          = flags.Preset
	flagNameRange           = flags.Range
	flagNameRef             = flags.Ref
	flagNameReportFile      = flags.ReportFile
	flagNameRepoPath        = flags.RepoPath
	flagNameRule            = flags.Rule
	flagNameRuleHelp        = flags.RuleHelp
	flagNameStdout          = flags.Stdout
	flagNameVerbose         = flags.Verbose
)

// NewConfigCommand creates the config subcommand.
func NewConfigCommand(localizer output.Localizer) *cli.Command {
	localizer = output.EnsureLocalizer(localizer)

	return &cli.Command{
		Name:  flagNameConfig,
		Usage: localizer.Get("configuration operations"),
		Description: localizer.Get(`Operations for viewing and managing gommitlint configuration.

Configuration has three load modes:
  Discovery (default): built-in defaults, then the user XDG config, then the project config.
  Explicit file (--config PATH): built-in defaults plus PATH and its imports; discovered user and project configs are not loaded.
  Defaults only (--ignore-config): built-in defaults and no configuration files.

Examples:
  # Show current effective configuration
  gommitlint config show

  # Generate complete config template with all options
  gommitlint config init

  # Generate a small config using an existing preset
  gommitlint config init --preset minimal

  # Create user-specific configuration (XDG)
  mkdir -p ~/.config/gommitlint
  gommitlint config init --output ~/.config/gommitlint/settings.yaml

  # Show current config in YAML format
  gommitlint config show --format=yaml

  # Show configuration in JSON format
  gommitlint config show --format=json`),

		Commands: []*cli.Command{
			{
				Name:  commandNameConfigShow,
				Usage: localizer.Get("show effective configuration"),
				Description: localizer.Get(`Shows the effective configuration produced by the selected load mode.

Load modes:
  Discovery (default): built-in defaults, then the user XDG config, then the project config.
  Explicit file (--config PATH): built-in defaults plus PATH and its imports; discovered user and project configs are not loaded.
  Defaults only (--ignore-config): built-in defaults and no configuration files.

Output formats:
  text - Human-readable summary (default)
  json - Machine-readable JSON
  yaml - Config file format`),

				Flags:  configDisplayFlags(localizer),
				Action: ExecuteConfigShow,
			},
			{
				Name:  commandNameCheck,
				Usage: localizer.Get("check configuration health"),
				Description: localizer.Get(`Validates the active configuration, reports config warnings,
and checks vendored managed_sources files.

Examples:
  gommitlint config check`),
				Flags:  configDisplayFlags(localizer),
				Action: ExecuteConfigCheck,
			},
			{
				Name:  commandNameConfigInit,
				Usage: localizer.Get("create .gommitlint.yaml in the current directory"),
				Description: localizer.Get(`Writes a .gommitlint.yaml template in the current directory. By default it
contains every option with defaults commented out. With --preset, it writes a
small config that extends an existing built-in preset. Refuses to overwrite an
existing file unless --force is set.

Examples:
  gommitlint config init                          # write .gommitlint.yaml in CWD
  gommitlint config init --preset minimal         # write a small preset config
  gommitlint config init --force                  # overwrite existing
  gommitlint config init --output custom.yaml     # write to a specific path
  gommitlint config init --stdout                 # print to stdout, write nothing
  gommitlint config init -o -                     # POSIX shorthand for --stdout`),

				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    flagNameOutput,
						Aliases: []string{"o"},
						Usage:   localizer.Get("write template to `PATH` (use - for stdout)"),
					},
					&cli.StringFlag{
						Name:  flagNamePreset,
						Usage: localizer.Get("write a small config extending built-in `PRESET`"),
					},
					&cli.BoolFlag{
						Name:  flagNameStdout,
						Usage: localizer.Get("print template to stdout instead of writing a file"),
					},
					&cli.BoolFlag{
						Name:  flagNameForce,
						Usage: localizer.Get("overwrite the target file if it already exists"),
					},
				},
				Action: ExecuteConfigInit,
			},
			{
				Name:  commandNameConfigSchema,
				Usage: localizer.Get("print the JSON Schema for the config file"),
				Description: localizer.Get(`Prints the JSON Schema that describes the gommitlint configuration file.

The schema is generated from the Go config types and embedded in the binary,
so the output matches this exact gommitlint version. Pipe it to a file to
validate configs in editors (VS Code, JetBrains) without relying on the
network-fetched copy from SchemaStore.

Examples:
  gommitlint config schema > .gommitlint.schema.json
  gommitlint config schema --compact | jq .`),
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  flagNameCompact,
						Usage: localizer.Get("emit single-line JSON instead of indented"),
					},
				},
				Action: ExecuteConfigSchema,
			},
			{
				Name:  commandNameConfigSync,
				Usage: localizer.Get("sync managed configuration sources"),
				Description: localizer.Get(`Fetches remote managed_sources, verifies integrity, vendors them locally,
and updates the inline resolved metadata in the active config file.

Examples:
  gommitlint config sync
  gommitlint config sync --source team-policy
  gommitlint config sync --check`),
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "source",
						Usage: localizer.Get("sync only managed source with `ID`"),
					},
					&cli.BoolFlag{
						Name:  flagNameDryRun,
						Usage: localizer.Get("fetch and verify without writing files"),
					},
					&cli.BoolFlag{
						Name:  "check",
						Usage: localizer.Get("fail if vendored managed sources are out of date"),
					},
					&cli.DurationFlag{
						Name:  flagNameTimeout,
						Usage: localizer.Get("per-source download `TIMEOUT`"),
						Value: configuration.DefaultSyncTimeout,
						Validator: func(timeout time.Duration) error {
							if timeout <= 0 {
								return fmt.Errorf("%w: %s", ErrNonPositiveTimeout,
									localizer.Get("--timeout must be a positive duration"))
							}

							return nil
						},
					},
				},
				Action: ExecuteConfigSync,
			},
		},
	}
}

func configDisplayFlags(localizer output.Localizer) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  flagNameFormat,
			Usage: localizer.Get("output `FORMAT`: %s", strings.Join(configTypes.GetValidConfigShowFormats(), ", ")),
			Value: output.FormatText,
		},
		&cli.StringFlag{
			Name:  "view",
			Usage: localizer.Get("text output `VIEW`: compact, verbose"),
			Value: configViewVerbose,
		},
	}
}

type configTextView string

func validateConfigSubcommandArgs(cmd *cli.Command, localizer output.Localizer, subcommand string) error {
	message := configSubcommandArgumentMessage(localizer, subcommand)

	return rejectUnexpectedArgs(cmd, localizer, "gommitlint config "+subcommand, message)
}

func configSubcommandArgumentMessage(localizer output.Localizer, subcommand string) string {
	switch subcommand {
	case commandNameCheck:
		return localizer.Get("The 'config check' command doesn't accept arguments")
	case commandNameConfigInit:
		return localizer.Get("The 'config init' command doesn't accept arguments")
	case commandNameConfigSchema:
		return localizer.Get("The 'config schema' command doesn't accept arguments")
	case commandNameConfigShow:
		return localizer.Get("The 'config show' command doesn't accept arguments")
	default:
		return localizer.Get("The 'config sync' command doesn't accept arguments")
	}
}

func syncConfigPath(cfgResult ConfigResult) string {
	return cfgResult.ManagedSourcesPath
}

func isConfigFormatValid(format string) bool {
	return slices.Contains(configTypes.GetValidConfigShowFormats(), format)
}

func unsupportedConfigFormatError(format string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrUnsupportedConfigFormat,
		output.Msg("unsupported format '%s'", format),
		loc.Get("Valid formats: %s", strings.Join(configTypes.GetValidConfigShowFormats(), ", ")),
		loc.Get("Use --format to choose a supported format"),
	)
}

func configDependencyError(localizer output.Localizer, cause error) error {
	wrappedCause := fmt.Errorf("%w: %w", ErrConfigDependencyFailed, cause)

	return newRecoveryErrorWithCause(
		wrappedCause,
		output.Msg("failed to create spell checker"),
		localizer.Get("Check spell locale in configuration"),
		localizer.Get("Run 'gommitlint config show' to inspect settings"),
	)
}
