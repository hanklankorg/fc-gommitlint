// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"errors"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"golang.org/x/term"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Output option errors.
var (
	ErrUnknownRule = errors.New("unknown rule")
)

const (
	colorAuto   = "auto"
	colorAlways = "always"
	colorNever  = "never"
	termDumb    = "dumb"
)

// ColorAuto is the canonical default for the --color flag.
const ColorAuto = colorAuto

// ColorAlways forces colored output regardless of terminal capability.
const ColorAlways = colorAlways

// ColorNever disables colored output.
const ColorNever = colorNever

// SupportedColorSettings is the public allow-list for --color.
// Kept in display order so the CLI's "Available --color values: …" hint reads
// naturally instead of being alphabetised.
func SupportedColorSettings() []string {
	return []string{colorAuto, colorAlways, colorNever}
}

// IsValidColorSetting reports whether the value matches a supported --color mode.
func IsValidColorSetting(setting string) bool {
	switch setting {
	case colorAuto, colorAlways, colorNever:
		return true
	}

	return false
}

// UnknownRuleError is returned when an unrecognised rule name is encountered.
type UnknownRuleError struct {
	Rule       string
	ValidRules []string
	localizer  output.Localizer
}

func (e UnknownRuleError) Error() string {
	localizer := output.EnsureLocalizer(e.localizer)

	return localizer.Get(
		"unknown rule %q. Valid rules: %s",
		e.Rule,
		strings.Join(e.ValidRules, ", "),
	)
}

func (e UnknownRuleError) Unwrap() error {
	return ErrUnknownRule
}

// OutputOptions how validation results should be formatted and displayed.
type OutputOptions struct {
	Format          string    // "text", "plain", "json", "sarif", "github", "gitlab", "forgejo", "table"
	VerboseLevel    int       // Verbose level (0=quiet, 1=verbose, 2=extra verbose)
	ShowHelp        bool      // Show help text and error codes
	RuleHelp        string    // Show detailed help for a specific rule
	Color           string    // When to colorize: "auto", "always", "never"
	Theme           string    // Output color theme
	Quiet           bool      // Suppress all output except errors
	Writer          io.Writer // Where to write output
	Localizer       output.Localizer
	AnnotationTitle string
	ToolVersion     string // gommitlint version, reported by metadata-carrying formats (SARIF)
	// ConfigSources are display-ready configuration sources, highest precedence
	// first. Rendered at the CLI boundary; the raw paths travel in the report
	// metadata for machine-readable formats.
	ConfigSources []string
	// ConfigSourcesUnexpected marks configuration that came from outside the
	// repository, so human formats surface it even on a clean run.
	ConfigSourcesUnexpected bool
	closer                  io.Closer // Optional closer for the writer
	useColor                bool
	useColorSet             bool
}

// NewOutputOptions creates OutputOptions with sensible defaults.
func NewOutputOptions(writer io.Writer) OutputOptions {
	return OutputOptions{
		Format:       output.FormatText,
		VerboseLevel: 0,
		ShowHelp:     false,
		RuleHelp:     "",
		Color:        colorAuto,
		Theme:        output.DefaultTheme,
		Quiet:        false,
		Writer:       writer,
	}
}

// WithFormat returns a new OutputOptions with the specified format.
func (o OutputOptions) WithFormat(format string) OutputOptions {
	o.Format = format

	return o
}

// WithToolVersion returns a new OutputOptions carrying the gommitlint version.
func (o OutputOptions) WithToolVersion(version string) OutputOptions {
	o.ToolVersion = version

	return o
}

// WithVerboseLevel returns a new OutputOptions with the specified verbose level.
func (o OutputOptions) WithVerboseLevel(level int) OutputOptions {
	o.VerboseLevel = level

	return o
}

// WithRuleHelp returns a new OutputOptions with rule help.
func (o OutputOptions) WithRuleHelp(ruleHelp string) OutputOptions {
	o.RuleHelp = ruleHelp

	return o
}

// WithQuiet returns a new OutputOptions with quiet mode enabled/disabled.
func (o OutputOptions) WithQuiet(quiet bool) OutputOptions {
	o.Quiet = quiet

	return o
}

// WithLocalizer returns a new OutputOptions with a localizer.
func (o OutputOptions) WithLocalizer(localizer output.Localizer) OutputOptions {
	o.Localizer = localizer

	return o
}

// GetAnnotationTitle returns the annotation title for CI formats.
func (o OutputOptions) GetAnnotationTitle() string {
	if strings.TrimSpace(o.AnnotationTitle) != "" {
		return o.AnnotationTitle
	}

	return annotationTitleFromEnv(getEnv)
}

// WithResolvedEnvironment returns a new OutputOptions with environment-dependent output settings resolved.
func (o OutputOptions) WithResolvedEnvironment(envFunc func(string) string) OutputOptions {
	if envFunc == nil {
		envFunc = getEnv
	}

	o.AnnotationTitle = annotationTitleFromEnv(envFunc)

	return o.WithUseColor(shouldUseColorWithEnv(o.Color, o.Writer, envFunc))
}

func annotationTitleFromEnv(envFunc func(string) string) string {
	if envFunc == nil {
		envFunc = getEnv
	}

	if jobName := envFunc("GITHUB_JOB"); jobName != "" {
		return jobName
	}

	return "Gommitlint"
}

// getValidRuleNames returns all valid rule names (sorted alphabetically).
func getValidRuleNames() []string {
	return settings.GetValidRuleNamesSorted()
}

// ValidateRuleHelp validates that the specified rule name exists.
func (o OutputOptions) ValidateRuleHelp() error {
	if o.RuleHelp == "" {
		return nil // Empty is valid
	}

	ruleName := settings.NormalizeRuleName(o.RuleHelp)
	if slices.Contains(getValidRuleNames(), ruleName) {
		return nil
	}

	return UnknownRuleError{Rule: o.RuleHelp, ValidRules: getValidRuleNames(), localizer: o.Localizer}
}

// GetNormalizedRuleHelp returns the rule name normalized for comparison with actual rule names.
func (o OutputOptions) GetNormalizedRuleHelp() string {
	if o.RuleHelp == "" {
		return ""
	}

	return settings.NormalizeRuleName(o.RuleHelp)
}

// WithColor returns a new OutputOptions with the specified color setting.
func (o OutputOptions) WithColor(color string) OutputOptions {
	o.Color = color
	o.useColorSet = false

	return o
}

// WithUseColor returns a new OutputOptions with color use resolved.
func (o OutputOptions) WithUseColor(useColor bool) OutputOptions {
	o.useColor = useColor
	o.useColorSet = true

	return o
}

// WithTheme returns a new OutputOptions with the specified theme.
func (o OutputOptions) WithTheme(theme string) OutputOptions {
	o.Theme = theme

	return o
}

// ShouldUseColor determines whether to use color based on the color setting and output type.
// Respects NO_COLOR environment variable as per https://no-color.org specification.
func (o OutputOptions) ShouldUseColor() bool {
	if o.useColorSet {
		return o.useColor
	}

	return shouldUseColorWithEnv(o.Color, o.Writer, getEnv)
}

// shouldUseColorWithEnv determines color usage with explicit environment function.
func shouldUseColorWithEnv(colorSetting string, writer io.Writer, envFunc func(string) string) bool {
	// NO_COLOR environment variable takes precedence over all flags
	// https://no-color.org - when set (any value), disable color output
	if envFunc("NO_COLOR") != "" {
		return false
	}

	// TERM=dumb indicates a non-interactive terminal that doesn't support colors
	if envFunc("TERM") == termDumb {
		return false
	}

	switch colorSetting {
	case colorAlways:
		return true
	case colorNever:
		return false
	case colorAuto:
		fallthrough
	default:
		return isTerminal(writer)
	}
}

// getEnv is a wrapper around os.Getenv for dependency injection in tests.
func getEnv(key string) string {
	return os.Getenv(key)
}

// isTerminal checks if the writer is a terminal using proper TTY detection.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		fd := f.Fd()
		if fd > math.MaxInt {
			return false
		}

		return term.IsTerminal(int(fd))
	}

	return false
}

// WriteReport formats and writes a report to o.Writer. In quiet mode it
// suppresses output for successful validations.
func (o OutputOptions) WriteReport(report domain.Report) error {
	return writeReport(o, report)
}

// WithCloser sets a closer for the writer if it implements io.Closer.
func (o OutputOptions) WithCloser(closer io.Closer) OutputOptions {
	o.closer = closer

	return o
}

// Close closes the underlying writer if it implements io.Closer.
func (o OutputOptions) Close() error {
	if o.closer != nil {
		return o.closer.Close()
	}

	return nil
}

// IsFile returns true if output is being written to a file (not stdout/stderr).
func (o OutputOptions) IsFile() bool {
	return o.closer != nil
}
