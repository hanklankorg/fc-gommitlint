// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"

	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

// Configuration loading errors.
var (
	ErrConflictingFlags = errors.New("cannot specify both --config and --ignore-config flags")
)

const configSourceDefaults = string(configuration.SourceTypeDefaults)

type configLoadMode string

const (
	configLoadModeDefault      configLoadMode = ""
	configLoadModeIgnoreConfig configLoadMode = "ignore-config"
	configLoadModeExplicitPath configLoadMode = "config"
	configLoadModeRepoPath     configLoadMode = "repo-path"
)

// ConfigResult holds both the config and information about where it came from.
type ConfigResult struct {
	configuration.LoadResult

	loadMode configLoadMode
}

// DisplaySource returns the user-facing primary configuration source.
func (r ConfigResult) DisplaySource() string {
	source := primaryConfigSourcePath(r.Sources)

	switch r.loadMode {
	case configLoadModeIgnoreConfig:
		return source + " (--ignore-config)"
	case configLoadModeExplicitPath:
		return source + " (--config)"
	case configLoadModeRepoPath:
		if source != configSourceDefaults {
			return source + " (--repo-path)"
		}
	case configLoadModeDefault:
	}

	return source
}

func primaryConfigSourcePath(sources []configuration.ConfigSource) string {
	if len(sources) == 0 {
		return configSourceDefaults
	}

	return sources[len(sources)-1].Path
}

// LoadConfigFromCommand loads configuration based on command flags.
func LoadConfigFromCommand(cmd *cli.Command) (ConfigResult, error) {
	return LoadConfigFromCommandWithWorkingDir(cmd, "")
}

// LoadConfigFromCommandWithWorkingDir loads configuration based on command flags from a specific working directory.
func LoadConfigFromCommandWithWorkingDir(cmd *cli.Command, workingDir string) (ConfigResult, error) {
	configPath := cmd.String(flagNameConfig)
	ignoreConfig := cmd.Bool(flagNameIgnoreConfig)
	repoPath := cmd.Root().String(flagNameRepoPath)

	if configPath != "" && ignoreConfig {
		return ConfigResult{}, ErrConflictingFlags
	}

	if ignoreConfig {
		return loadDefaultConfig()
	}

	if configPath != "" {
		return loadConfigFromPathWithWorkingDir(configPath, workingDir)
	}

	return loadConfigWithRepoPathFromWorkingDir(repoPath, workingDir)
}

// loadDefaultConfig loads only default configuration.
func loadDefaultConfig() (ConfigResult, error) {
	cfg := configuration.NewConfigWithDefaults()

	return ConfigResult{
		Config:      cfg,
		SourceGraph: []configuration.ConfigSourceNode{{Path: configSourceDefaults, Type: configuration.SourceTypeDefaults}},
		Sources:     []configuration.ConfigSource{{Path: configSourceDefaults, Type: configuration.SourceTypeDefaults}},
		loadMode:    configLoadModeIgnoreConfig,
	}, nil
}

// loadConfigFromPathWithWorkingDir loads configuration from a specific path with a working directory context.
func loadConfigFromPathWithWorkingDir(configPath, workingDir string) (ConfigResult, error) {
	validatedPath, err := secureConfigPathValidationWithWorkingDir(configPath, workingDir)
	if err != nil {
		return ConfigResult{}, err
	}

	loadResult, err := configuration.LoadConfigFromPathDetailed(validatedPath)
	if err != nil {
		return ConfigResult{}, err
	}

	return ConfigResult{
		LoadResult: loadResult,
		loadMode:   configLoadModeExplicitPath,
	}, nil
}

// loadConfigWithRepoPathFromWorkingDir loads configuration with repository path validation from a specific working directory.
// Uses hierarchical config loading: defaults → global → project.
func loadConfigWithRepoPathFromWorkingDir(repoPath, workingDir string) (ConfigResult, error) {
	validatedRepoPath, err := cliAdapter.ValidateRepoPath(repoPath)
	if err != nil {
		return ConfigResult{}, err
	}

	loadResult, err := configuration.LoadConfigHierarchy(validatedRepoPath, workingDir)
	if err != nil {
		return ConfigResult{}, err
	}

	loadMode := configLoadModeDefault
	if repoPath != "" {
		loadMode = configLoadModeRepoPath
	}

	return ConfigResult{
		LoadResult: loadResult,
		loadMode:   loadMode,
	}, nil
}

// resolveConfigPresentation reads the --format/--view flags, fills in
// defaults, validates them, and loads the effective configuration. Callers
// that share the "pick a format/view, then load config" preamble (config
// show, config check) use this to collapse the boilerplate; argument-shape
// errors fail fast, before any disk I/O.
func resolveConfigPresentation(cmd *cli.Command, localizer output.Localizer) (ConfigResult, string, configTextView, error) {
	format := cmd.String("format")
	if format == "" {
		format = output.FormatText
	}

	view := configTextView(cmd.String("view"))
	if view == "" {
		view = configViewVerbose
	}

	if !isConfigFormatValid(format) {
		return ConfigResult{}, "", "", unsupportedConfigFormatError(format, localizer)
	}

	if err := validateConfigView(format, view, localizer); err != nil {
		return ConfigResult{}, "", "", err
	}

	cfgResult, err := LoadConfigFromCommand(cmd.Root())
	if err != nil {
		return ConfigResult{}, "", "", configLoadError(err, localizer)
	}

	return cfgResult, format, view, nil
}
