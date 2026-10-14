// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// configInitDefaultPath is the canonical config filename that `config init`
// creates when neither --output nor --stdout is given. Other extensions
// (.yml, .toml) are still loaded by auto-discovery, but the init command
// commits to the single canonical name so users have one obvious answer.
const configInitDefaultPath = ".gommitlint.yaml"

// Error sentinels for config init failure modes.
var (
	errConfigInitConflictingFlags = errors.New("--stdout and --output are mutually exclusive")
	errConfigInitInsideGitDir     = errors.New("refusing to write config file inside .git/")
	errConfigInitUnknownPreset    = errors.New("unknown config preset")
)

// ExecuteConfigInit generates a configuration template.
//
// Default behaviour writes `.gommitlint.yaml` in the current directory. The
// previous "print to stdout, pipe yourself" flow is preserved behind --stdout
// (or --output -). This matches `git init`, `cargo init`, `npm init` and every
// other `<tool> init` users encounter — `init` means "create the thing".
func ExecuteConfigInit(_ context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()

	if err := validateConfigSubcommandArgs(cmd, localizer, commandNameConfigInit); err != nil {
		return err
	}

	dest, err := resolveConfigInitDestination(cmd, localizer)
	if err != nil {
		return err
	}

	renderer, err := configInitRenderer(cmd, localizer)
	if err != nil {
		return err
	}

	if dest.toStdout {
		return renderer(cmd.Writer)
	}

	return writeConfigInitFile(dest, localizer, renderer)
}

type configInitRenderFunc func(io.Writer) error

func configInitRenderer(cmd *cli.Command, localizer output.Localizer) (configInitRenderFunc, error) {
	preset := strings.TrimSpace(cmd.String(flagNamePreset))
	if preset == "" {
		defaultCfg := configuration.NewConfigWithDefaults()

		return func(writer io.Writer) error {
			return renderConfigInitYAML(defaultCfg, writer, localizer)
		}, nil
	}

	if !configuration.IsKnownPreset(preset) {
		return nil, unknownConfigInitPresetError(preset, localizer)
	}

	return func(writer io.Writer) error {
		return renderConfigInitPresetYAML(preset, writer, localizer)
	}, nil
}

func unknownConfigInitPresetError(preset string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)
	presets := configuration.PresetNames()

	return newRecoveryErrorWithCause(
		errConfigInitUnknownPreset,
		output.Msg("unknown preset: %s", preset),
		loc.Get("Use one of: %s", strings.Join(presets, ", ")),
		loc.Get("Use full template instead: gommitlint config init"),
	)
}

type configInitDestination struct {
	path     string
	toStdout bool
	force    bool
}

func resolveConfigInitDestination(cmd *cli.Command, localizer output.Localizer) (configInitDestination, error) {
	outputPath := cmd.String(flagNameOutput)
	stdout := cmd.Bool(flagNameStdout)
	force := cmd.Bool(flagNameForce)

	// `--output -` is the POSIX convention for stdout; treat it as an alias
	// for --stdout so users can pick whichever form they already know.
	if outputPath == "-" {
		outputPath = ""
		stdout = true
	}

	if stdout && outputPath != "" {
		return configInitDestination{}, newRecoveryErrorWithCause(
			errConfigInitConflictingFlags,
			output.Msg("--stdout and --output are mutually exclusive"),
			localizer.Get("Use either --stdout to print, or --output PATH to write a file"),
		)
	}

	if stdout {
		return configInitDestination{toStdout: true, force: force}, nil
	}

	if outputPath == "" {
		outputPath = configInitDefaultPath
	}

	return configInitDestination{path: outputPath, force: force}, nil
}

func writeConfigInitFile(dest configInitDestination, localizer output.Localizer, renderer configInitRenderFunc) (returnErr error) {
	// Route the user-supplied path through the existing security validator —
	// rejects traversal, embedded NULs, dangerous patterns. The validator
	// returns an absolute path for the actual write.
	validated, err := cliAdapter.ValidateOutputFilePath(dest.path)
	if err != nil {
		return newRecoveryErrorWithCause(
			err,
			output.Msg("invalid --output path: %s", err),
			localizer.Get("Use a relative path inside the repo, e.g. .gommitlint.yaml"),
			localizer.Get("Or print to stdout: gommitlint config init --stdout"),
		)
	}

	// Refuse to write inside .git/ — config files there are never auto-loaded
	// and the user almost certainly typo'd. Better to surface than silently
	// produce a no-op file.
	if isInsideGitDir(validated) {
		return newRecoveryErrorWithCause(
			errConfigInitInsideGitDir,
			output.Msg("refusing to write config file inside .git/: %s", validated),
			localizer.Get("Place the config at the repo root: .gommitlint.yaml"),
		)
	}

	var rendered []byte

	if dest.force {
		var buf bytes.Buffer
		if err := renderer(&buf); err != nil {
			return err
		}

		rendered = buf.Bytes()
	}

	if dest.force {
		if _, err := os.Stat(filepath.Dir(validated)); err != nil {
			return newRecoveryErrorWithCause(
				err,
				output.Msg("failed to create config file: %s", err),
				localizer.Get("Check directory permissions"),
				localizer.Get("Print to stdout instead: gommitlint config init --stdout"),
			)
		}

		if err := filesystem.NewOSFileSystem().WriteFileAtomic(validated, rendered, 0o644); err != nil {
			return configOutputError(err, output.Msg("failed to write output"))
		}

		return nil
	}

	// 0644: world-readable, owner-only writable. Group-writable would trip the
	// loader's own insecure-perm warning on the file we just wrote — gosec
	// G302 prefers 0600, but config files are intentionally readable so the
	// repo's CI / collaborators can use them without group setup.
	file, err := os.OpenFile(validated, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o644) //nolint:gosec // see above; readability is the documented behaviour.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return newRecoveryErrorWithCause(
				err,
				output.Msg("config file already exists: %s", validated),
				localizer.Get("Overwrite: gommitlint config init --force"),
				localizer.Get("Edit existing config in your editor"),
				localizer.Get("Print instead of write: gommitlint config init --stdout"),
			)
		}

		return newRecoveryErrorWithCause(
			err,
			output.Msg("failed to create config file: %s", err),
			localizer.Get("Check directory permissions"),
			localizer.Get("Print to stdout instead: gommitlint config init --stdout"),
		)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && returnErr == nil {
			returnErr = configOutputError(
				closeErr,
				output.Msg("failed to finalize config file: %s", closeErr),
			)
		}
	}()

	if err := renderer(file); err != nil {
		return err
	}

	return nil
}

func renderConfigInitPresetYAML(preset string, writer io.Writer, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	lines := []string{
		"# " + localizer.Get("Gommitlint Configuration"),
		"# " + localizer.Get("Generated by: gommitlint config init --preset %s", preset),
		"#",
		"# " + localizer.Get("Inspect active rules: gommitlint config show --view=compact"),
		"gommitlint:",
		"  extends:",
		"    - preset:" + preset,
		"",
	}

	for _, line := range lines {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return configOutputError(err, output.Msg("failed to write preset config template"))
		}
	}

	return nil
}

func isInsideGitDir(absPath string) bool {
	cleaned := filepath.Clean(absPath)
	for part := range strings.SplitSeq(cleaned, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}

	return false
}

func renderConfigInitYAML(cfg configTypes.Config, writer io.Writer, localizer output.Localizer) error {
	if err := printConfigInitYAML(cfg, writer, localizer); err != nil {
		if _, ok := errors.AsType[RecoveryError](err); ok {
			return err
		}

		return newRecoveryErrorWithCause(
			err,
			output.Msg("failed to generate config template"),
			helpSuggestion(localizer, "gommitlint config init"),
			localizer.Get("Check output destination"),
		)
	}

	return nil
}

// printConfigInitYAML generates a complete configuration template with defaults commented out.
// Default values are commented so users only need to uncomment what they want to change.
func printConfigInitYAML(cfg configTypes.Config, writer io.Writer, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)

	header := []string{
		"# " + localizer.Get("Gommitlint Configuration"),
		"# " + localizer.Get("Generated by: gommitlint config init"),
		"#",
		"# " + localizer.Get("Default values are commented out. Uncomment and modify settings you want to change."),
		"# " + localizer.Get("For more information, see: https://codefloe.com/itiquette/gommitlint"),
		"",
	}
	for _, line := range header {
		if _, err := fmt.Fprintln(writer, line); err != nil {
			return configOutputError(err, output.Msg("failed to write output"))
		}
	}

	// Sort all list fields for consistent output
	sortedCfg := cfg.Sorted()

	// Wrap the config in the gommitlint root structure
	configWithRoot := map[string]any{
		gommitlintKey: sortedCfg,
	}

	// Generate YAML to a buffer first
	var buf strings.Builder

	encoder := newIndentedYAMLEncoder(&buf)

	if err := encoder.Encode(configWithRoot); err != nil {
		return configOutputError(err, output.Msg("failed to encode config template as YAML"))
	}

	if err := encoder.Close(); err != nil {
		return configOutputError(err, output.Msg("failed to close YAML encoder"))
	}

	// Process the YAML to comment out default values
	commented := commentOutDefaultValues(buf.String())
	if _, err := fmt.Fprint(writer, commented); err != nil {
		return configOutputError(err, output.Msg("failed to write output"))
	}

	return nil
}

// commentOutDefaultValues processes YAML and comments out lines with default values.
// It preserves the structure keys (like "gommitlint:", "conventional:") but comments
// out the actual value settings.
func commentOutDefaultValues(yamlContent string) string {
	lines := strings.Split(yamlContent, "\n")

	var result strings.Builder

	for index, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Keep empty lines as-is
		if trimmed == "" {
			result.WriteString(line)
			result.WriteString("\n")

			continue
		}

		if isYAMLSectionLine(lines, index) {
			// Keep section keys uncommented
			result.WriteString(line)
			result.WriteString("\n")
		} else {
			// Comment out value lines by adding "# " after the leading whitespace
			leadingSpaces := leadingSpaceCount(line)
			if leadingSpaces > 0 {
				result.WriteString(line[:leadingSpaces])
				result.WriteString("# ")
				result.WriteString(line[leadingSpaces:])
			} else {
				result.WriteString("# ")
				result.WriteString(line)
			}

			result.WriteString("\n")
		}
	}

	return result.String()
}

func isYAMLSectionLine(lines []string, index int) bool {
	line := lines[index]

	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, ":") {
		return false
	}

	indent := leadingSpaceCount(line)

	for _, next := range lines[index+1:] {
		if strings.TrimSpace(next) == "" {
			continue
		}

		return leadingSpaceCount(next) > indent
	}

	return false
}

func leadingSpaceCount(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
