// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/ci"
	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/flags"
	"codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// ErrVerboseFlagCount signals excessive verbosity flags.
var ErrVerboseFlagCount = errors.New("verbose flag may be specified at most twice")

func outputFormatUsage(localizer output.Localizer) string {
	formats := output.SupportedFormats()
	localizer = output.EnsureLocalizer(localizer)

	// 'table' is suited for small commit ranges (≤30); on very large runs it
	// produces thousands of lines that are unreadable in a terminal. Mention
	// it in the flag help so users discover the constraint before they hit
	// it. JSON / plain are the right choices for large-batch machine output.
	return localizer.Get("output `FORMAT`: %s (table best for ≤30 commits)", strings.Join(formats, ", "))
}

// createOutputWriter creates the appropriate output writer based on CLI flags.
func createOutputWriter(cmd *cli.Command, localizer output.Localizer) (io.Writer, io.Closer, error) {
	reportFile := cmd.String(flagNameReportFile)

	// Handle stdout case (empty or "-")
	if reportFile == "" || reportFile == "-" {
		if cmd.Writer != nil {
			return cmd.Writer, nil, nil
		}

		return os.Stdout, nil, nil
	}

	// Validate the report file path for security
	validatedPath, err := cliAdapter.ValidateOutputFilePath(reportFile)
	if err != nil {
		return nil, nil, err
	}

	dir := filepath.Dir(validatedPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, nil, newRecoveryErrorWithCause(
			err,
			output.Msg("cannot create output directory"),
			localizer.Get("Check directory permissions"),
			localizer.Get("Use --report-file to choose a different path"),
		)
	}

	file, err := os.Create(validatedPath) // #nosec G304 - validatedPath has been validated for security
	if err != nil {
		return nil, nil, newRecoveryErrorWithCause(
			err,
			output.Msg("failed to create report file"),
			localizer.Get("Check report file path or permissions"),
			localizer.Get("Use --report-file - to write to stdout"),
		)
	}

	return file, file, nil
}

func resolveThemeValue(theme string, localizer output.Localizer) (string, error) {
	if theme == "" {
		theme = output.DefaultTheme
	}

	if !output.IsValidTheme(theme) {
		return "", invalidThemeError(theme, localizer)
	}

	return theme, nil
}

func invalidThemeError(theme string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	validThemes := output.SupportedThemes()

	return newRecoveryErrorWithCause(
		ErrUnsupportedTheme,
		output.Msg("%s '%s'", ErrUnsupportedTheme, theme),
		loc.Get("Available themes: %s", strings.Join(validThemes, ", ")),
		loc.Get("Run 'gommitlint --help' to see global options"),
	)
}

func verboseFlagCountError(localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrVerboseFlagCount,
		output.Msg("verbose flag may be specified at most twice"),
		loc.Get("Use -v or --verbose for verbose output"),
		loc.Get("Use -vv for extra verbose output"),
	)
}

func resolveTheme(cmd *cli.Command, cfg configTypes.Config, localizer output.Localizer) (string, error) {
	theme := cmd.Root().String(flags.Theme)
	if !cmd.Root().IsSet(flags.Theme) {
		theme = cfg.Theme
	}

	return resolveThemeValue(theme, localizer)
}

// resolveColorValue validates --color against the canonical set. Unknown
// values fail loudly instead of silently degrading to 'auto', matching the
// actionable feedback of --theme and --format.
func resolveColorValue(color string, localizer output.Localizer) (string, error) {
	if color == "" {
		return cliAdapter.ColorAuto, nil
	}

	if !cliAdapter.IsValidColorSetting(color) {
		return "", invalidColorError(color, localizer)
	}

	return color, nil
}

func invalidColorError(color string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	valid := cliAdapter.SupportedColorSettings()

	return newRecoveryErrorWithCause(
		ErrUnsupportedColor,
		output.Msg("%s '%s'", ErrUnsupportedColor, color),
		loc.Get("Available --color values: %s", strings.Join(valid, ", ")),
		loc.Get("Honor terminal capability: --color=auto"),
		loc.Get("Run 'gommitlint --help' to see global options"),
	)
}

func resolveColor(cmd *cli.Command, localizer output.Localizer) (string, error) {
	return resolveColorValue(cmd.Root().String(flagNameColor), localizer)
}

// ValidateLogLevel validates --log-level against the canonical set. The
// logging adapter's parseLogLevel silently defaults invalid values to "info"
// — fine as a library safety net, hostile as user-facing CLI behaviour.
// Surface unknown values loudly at the CLI boundary so users learn about
// their typo instead of getting unexpected log output.
//
// Exported because the validation needs to happen in the root command's
// Before hook (cmd/gommitlint/main.go), which is outside the commands package.
func ValidateLogLevel(level string, localizer output.Localizer) error {
	if level == "" {
		return nil
	}

	// The logging adapter owns the allowlist (canonical names plus the
	// undocumented "warning" / "information" aliases). Defer to it so the set
	// lives in exactly one place.
	if logging.IsValidLogLevel(level) {
		return nil
	}

	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrUnsupportedLogLevel,
		output.Msg("%s '%s'", ErrUnsupportedLogLevel, level),
		// User-facing list is the canonical set (aliases omitted to avoid noise).
		loc.Get("Available --log-level values: %s", strings.Join(logging.LogLevelNames(), ", ")),
		loc.Get("Run 'gommitlint --help' to see global options"),
	)
}

// resolveOutputFormat determines the output format from CLI flags or CI auto-detection.
func resolveOutputFormat(cmd *cli.Command, ciEnv ci.Environment, localizer output.Localizer) (string, error) {
	format := cmd.String("format")

	// Apply CI-detected format when not explicitly set
	if !cmd.IsSet("format") && ciEnv.Detected() {
		format = ciEnv.Platform
	}

	if !output.IsValidFormat(format) {
		loc := output.EnsureLocalizer(localizer)

		return "", newRecoveryErrorWithCause(
			ErrUnsupportedFormat,
			output.Msg("%s '%s'", ErrUnsupportedFormat, format),
			loc.Get("Supported formats: %s", strings.Join(output.SupportedFormats(), ", ")),
			helpSuggestion(loc, "gommitlint validate"),
		)
	}

	return format, nil
}

// applyVerbosity configures verbose level on output options unless quiet mode is active.
func applyVerbosity(options cliAdapter.OutputOptions, cmd *cli.Command, quiet bool, localizer output.Localizer) (cliAdapter.OutputOptions, error) {
	verboseLevel := cmd.Count(flagNameVerbose)
	if verboseLevel > 2 {
		return cliAdapter.OutputOptions{}, verboseFlagCountError(localizer)
	}

	if verboseLevel > 0 && !quiet {
		options = options.WithVerboseLevel(verboseLevel)
	}

	return options, nil
}

// createOutputOptions configures result formatting and destination with path validation.
func createOutputOptions(cmd *cli.Command, cfg configTypes.Config, ciEnv ci.Environment, localizer output.Localizer) (cliAdapter.OutputOptions, error) {
	format, err := resolveOutputFormat(cmd, ciEnv, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	theme, err := resolveTheme(cmd, cfg, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	color, err := resolveColor(cmd, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	quiet := cmd.Root().Bool(flags.Quiet)

	options := cliAdapter.NewOutputOptions(cmd.Writer).
		WithFormat(format).
		WithColor(color).
		WithTheme(theme).
		WithQuiet(quiet).
		WithLocalizer(localizer).
		WithToolVersion(toolVersionFrom(cmd))

	options, err = applyVerbosity(options, cmd, quiet, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	writer, closer, err := createOutputWriter(cmd, localizer)
	if err != nil {
		return cliAdapter.OutputOptions{}, err
	}

	options.Writer = writer

	options = options.WithResolvedEnvironment(os.Getenv)
	if closer != nil {
		options = options.WithCloser(closer)
	}

	return options, nil
}

// toolVersionFrom extracts the bare version token from the root command.
// The CLI version string reads "0.9.12 (Build date: ...)"; metadata-carrying
// formats like SARIF want only the version itself.
func toolVersionFrom(cmd *cli.Command) string {
	fields := strings.Fields(cmd.Root().Version)
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}
