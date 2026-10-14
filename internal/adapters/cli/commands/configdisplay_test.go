// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const (
	configDisplayRepo = "/home/dev/project"
	configDisplayHome = "/home/dev"
)

func configDisplayTestEnv() configDisplayEnv {
	return configDisplayEnv{WorkingDir: configDisplayRepo, HomeDir: configDisplayHome}
}

func configDisplayResult(mode configLoadMode, sources ...configuration.ConfigSource) ConfigResult {
	return ConfigResult{
		Sources:  sources,
		loadMode: mode,
	}
}

func defaultsSource() configuration.ConfigSource {
	return configuration.ConfigSource{Path: configSourceDefaults, Type: configuration.SourceTypeDefaults}
}

func TestConfigDisplay_Rendering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		result     ConfigResult
		expected   []string
		unexpected bool
	}{
		{
			// The common case: one repo-local file. Length here is what makes
			// the line worth suppressing on a clean run, so it must be short.
			name: "project config renders relative to the repository",
			result: configDisplayResult(configLoadModeDefault,
				defaultsSource(),
				configuration.ConfigSource{Path: configDisplayRepo + "/.gommitlint.yaml", Type: configuration.SourceTypeProject},
			),
			expected:   []string{".gommitlint.yaml"},
			unexpected: false,
		},
		{
			name: "global config is tagged, abbreviated, and listed after the winner",
			result: configDisplayResult(configLoadModeDefault,
				defaultsSource(),
				configuration.ConfigSource{Path: configDisplayHome + "/.config/gommitlint/settings.yaml", Type: configuration.SourceTypeGlobal},
				configuration.ConfigSource{Path: configDisplayRepo + "/.gommitlint.yaml", Type: configuration.SourceTypeProject},
			),
			expected:   []string{".gommitlint.yaml", "~/.config/gommitlint/settings.yaml (global)"},
			unexpected: true,
		},
		{
			// An extended file committed alongside the project config is part of
			// what the reader already assumes is in play.
			name: "in-repo extends is tagged but not surprising",
			result: configDisplayResult(configLoadModeDefault,
				defaultsSource(),
				configuration.ConfigSource{Path: configDisplayRepo + "/build/base.yaml", Type: configuration.SourceTypeExtended},
				configuration.ConfigSource{Path: configDisplayRepo + "/.gommitlint.yaml", Type: configuration.SourceTypeProject},
			),
			expected:   []string{".gommitlint.yaml", "build/base.yaml (extended)"},
			unexpected: false,
		},
		{
			// The flag explains the whole line, so it replaces the type tag
			// rather than stacking into "ci/strict.yaml (cli) (--config)".
			name: "explicit --config replaces the type tag",
			result: configDisplayResult(configLoadModeExplicitPath,
				defaultsSource(),
				configuration.ConfigSource{Path: configDisplayRepo + "/ci/strict.yaml", Type: configuration.SourceTypeCLI},
			),
			expected:   []string{"ci/strict.yaml (--config)"},
			unexpected: true,
		},
		{
			name:       "no config file found",
			result:     configDisplayResult(configLoadModeDefault, defaultsSource()),
			expected:   []string{"defaults (no config file found)"},
			unexpected: false,
		},
		{
			name:       "--ignore-config names the flag that emptied the chain",
			result:     configDisplayResult(configLoadModeIgnoreConfig, defaultsSource()),
			expected:   []string{"defaults (--ignore-config)"},
			unexpected: true,
		},
		{
			// Neither in the repo nor under home: nothing to shorten against,
			// and the absolute path is the whole point of showing the line.
			name: "system-wide config keeps its absolute path and is surprising",
			result: configDisplayResult(configLoadModeDefault,
				defaultsSource(),
				configuration.ConfigSource{Path: "/etc/gommitlint/policy.yaml", Type: configuration.SourceTypeGlobal},
				configuration.ConfigSource{Path: configDisplayRepo + "/.gommitlint.yaml", Type: configuration.SourceTypeProject},
			),
			expected:   []string{".gommitlint.yaml", "/etc/gommitlint/policy.yaml (global)"},
			unexpected: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			display := testCase.result.Display(configDisplayTestEnv(), nil)
			check.Equal(t, testCase.expected, display.Sources)
			check.Equal(t, testCase.unexpected, display.Unexpected)
		})
	}
}

// The last case above is the one worth stating twice: a project config outside
// the working directory is its own root, so it shortens against itself rather
// than reporting as surprising. Reaching in from elsewhere is what counts.
func TestConfigDisplay_RepoRootComesFromProjectConfig(t *testing.T) {
	t.Parallel()

	result := configDisplayResult(configLoadModeDefault,
		defaultsSource(),
		configuration.ConfigSource{Path: "/srv/repo/.gommitlint.yaml", Type: configuration.SourceTypeProject},
		configuration.ConfigSource{Path: "/srv/repo/shared/base.yaml", Type: configuration.SourceTypeExtended},
	)

	// Working directory is elsewhere entirely: paths still shorten against the
	// project config's directory, not the caller's cwd.
	display := result.Display(configDisplayEnv{WorkingDir: "/tmp", HomeDir: configDisplayHome}, nil)

	check.Equal(t, []string{"shared/base.yaml (extended)", ".gommitlint.yaml"}, display.Sources)
	check.False(t, display.Unexpected)
}

func TestConfigDisplay_SanitizesPathsForTerminal(t *testing.T) {
	t.Parallel()

	result := configDisplayResult(configLoadModeDefault,
		defaultsSource(),
		configuration.ConfigSource{
			Path: configDisplayRepo + "/policy\x1b[31m\nforged.yaml",
			Type: configuration.SourceTypeProject,
		},
	)

	display := result.Display(configDisplayTestEnv(), nil)

	check.Equal(t, []string{"policy\\x1b[31m forged.yaml"}, display.Sources)
}

func TestConfigDisplay_LocalizesSourceLabels(t *testing.T) {
	t.Parallel()

	localizer := configDisplayLabelLocalizer{}
	tests := []struct {
		sourceType configuration.ConfigSourceType
		expected   string
	}{
		{sourceType: configuration.SourceTypeDefaults, expected: "translated-defaults"},
		{sourceType: configuration.SourceTypeExtended, expected: "translated-extended"},
		{sourceType: configuration.SourceTypePreset, expected: "translated-preset"},
		{sourceType: configuration.SourceTypeGlobal, expected: "translated-global"},
		{sourceType: configuration.SourceTypeProject, expected: "translated-project"},
		{sourceType: configuration.SourceTypeCLI, expected: "translated-cli"},
	}

	for _, testCase := range tests {
		check.Equal(t, string(testCase.sourceType), configDisplaySourceLabel(testCase.sourceType, nil))
		check.Equal(t, testCase.expected, configDisplaySourceLabel(testCase.sourceType, localizer))
	}

	result := configDisplayResult(configLoadModeDefault,
		defaultsSource(),
		configuration.ConfigSource{Path: configDisplayHome + "/.config/gommitlint/base.yaml", Type: configuration.SourceTypeExtended},
		configuration.ConfigSource{Path: configDisplayRepo + "/.gommitlint.yaml", Type: configuration.SourceTypeProject},
	)
	display := result.Display(configDisplayTestEnv(), localizer)
	check.Equal(t, []string{".gommitlint.yaml", "~/.config/gommitlint/base.yaml (translated-extended)"}, display.Sources)
	check.Equal(t, []string{
		configSourceDefaults,
		configDisplayHome + "/.config/gommitlint/base.yaml",
		configDisplayRepo + "/.gommitlint.yaml",
	}, configSourcePaths(result.Sources))

	defaults := configDisplayResult(configLoadModeIgnoreConfig, defaultsSource()).Display(configDisplayTestEnv(), localizer)
	check.Equal(t, []string{"translated-defaults (--ignore-config)"}, defaults.Sources)
}

type configDisplayLabelLocalizer struct{}

func (configDisplayLabelLocalizer) Get(message string, args ...any) string {
	switch message {
	case "defaults", "extended", "preset", "global", "project", "cli":
		return "translated-" + message
	default:
		return fmt.Sprintf(message, args...)
	}
}

func (configDisplayLabelLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return output.EnsureLocalizer(nil).GetN(singular, plural, count, args...)
}

func TestShortenConfigPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "inside root", path: configDisplayRepo + "/.gommitlint.yaml", expected: ".gommitlint.yaml"},
		{name: "nested inside root", path: configDisplayRepo + "/ci/strict.yaml", expected: "ci/strict.yaml"},
		{name: "under home", path: configDisplayHome + "/.config/gommitlint/settings.yaml", expected: "~/.config/gommitlint/settings.yaml"},
		{name: "elsewhere stays absolute", path: "/etc/gommitlint/policy.yaml", expected: "/etc/gommitlint/policy.yaml"},
		{
			// A sibling directory shares a textual prefix with the root but is
			// not inside it. It must not shorten to "../project-notes/x.yaml";
			// falling through to the home form is the readable answer.
			name:     "sibling of root falls through to home-relative",
			path:     "/home/dev/project-notes/x.yaml",
			expected: "~/project-notes/x.yaml",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.expected, shortenConfigPath(testCase.path, configDisplayRepo, configDisplayHome))
		})
	}
}
