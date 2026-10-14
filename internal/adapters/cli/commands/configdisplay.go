// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
)

// ConfigDisplay is the user-facing description of where configuration came
// from, rendered once at the CLI boundary.
//
// Formatters receive finished strings rather than raw paths: shortening a path
// needs the working directory and $HOME, and the output layer has no business
// touching the filesystem. The raw absolute paths still travel separately in
// domain.ReportMetadata.ConfigSources, which is what the JSON and SARIF
// payloads publish.
type ConfigDisplay struct {
	// Sources are display-ready descriptions, highest precedence first — the
	// file that won is the one the reader is looking for.
	Sources []string
	// Unexpected reports that configuration came from somewhere other than
	// built-in defaults and the repository's own files. Formatters use it to
	// decide whether the provenance is worth showing on a clean run.
	Unexpected bool
}

// configDisplayEnv carries the paths used to shorten config paths. Injected
// rather than read from the process so the rendering stays testable.
type configDisplayEnv struct {
	WorkingDir string
	HomeDir    string
}

// currentConfigDisplayEnv reads the process environment. Lookup failures leave
// the field empty, which shortenConfigPath treats as "cannot shorten" and falls
// back to the absolute path — never an error path for a cosmetic concern.
func currentConfigDisplayEnv() configDisplayEnv {
	workingDir, err := os.Getwd()
	if err != nil {
		workingDir = ""
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	return configDisplayEnv{WorkingDir: workingDir, HomeDir: homeDir}
}

// Display renders the loaded configuration's provenance for humans.
func (r ConfigResult) Display(env configDisplayEnv, localizer output.Localizer) ConfigDisplay {
	localizer = output.EnsureLocalizer(localizer)
	flagTag := r.loadMode.displayTag()

	files := configFileSources(r.Sources)
	if len(files) == 0 {
		if flagTag != "" {
			return ConfigDisplay{Sources: []string{configDisplaySourceLabel(configuration.SourceTypeDefaults, localizer) + " " + flagTag}, Unexpected: true}
		}

		return ConfigDisplay{Sources: []string{localizer.Get("defaults (no config file found)")}}
	}

	root := configDisplayRoot(files, env.WorkingDir)
	rendered := make([]string, 0, len(files))
	unexpected := flagTag != ""

	for _, source := range slices.Backward(files) {
		text := output.CleanForTerminal(shortenConfigPath(source.Path, root, env.HomeDir))

		// The flag that produced this load explains the whole line, so it
		// replaces the type tag on the winning source rather than stacking
		// with it ("ci/strict.yaml (--config)", not "... (cli) (--config)").
		switch {
		case len(rendered) == 0 && flagTag != "":
			text += " " + flagTag
		case source.Type != configuration.SourceTypeProject:
			text += " (" + configDisplaySourceLabel(source.Type, localizer) + ")"
		}

		// Files inside the repository are part of what a reader already assumes
		// is in play, including any the project config extends. Only config
		// reaching in from outside the tree is news worth volunteering.
		if _, ok := relativeUnder(source.Path, root); !ok {
			unexpected = true
		}

		rendered = append(rendered, text)
	}

	return ConfigDisplay{Sources: rendered, Unexpected: unexpected}
}

func configDisplaySourceLabel(sourceType configuration.ConfigSourceType, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	switch sourceType {
	case configuration.SourceTypeDefaults:
		return localizer.Get("defaults")
	case configuration.SourceTypeExtended:
		return localizer.Get("extended")
	case configuration.SourceTypePreset:
		return localizer.Get("preset")
	case configuration.SourceTypeGlobal:
		return localizer.Get("global")
	case configuration.SourceTypeProject:
		return localizer.Get("project")
	case configuration.SourceTypeCLI:
		return localizer.Get("cli")
	}

	return output.CleanForTerminal(string(sourceType))
}

// configFileSources drops the built-in defaults pseudo-source. Defaults are
// present in every load by construction, so naming them adds length without
// adding information; the exception is a load with no files at all, which
// Display handles separately.
func configFileSources(sources []configuration.ConfigSource) []configuration.ConfigSource {
	files := make([]configuration.ConfigSource, 0, len(sources))

	for _, source := range sources {
		if source.Path == "" || source.Type == configuration.SourceTypeDefaults {
			continue
		}

		files = append(files, source)
	}

	return files
}

// configDisplayRoot picks the directory that paths are shown relative to: the
// project config's own directory, which is the repository root for a
// hierarchical load. Falls back to the working directory when the load had no
// project config (an explicit --config path, say).
func configDisplayRoot(files []configuration.ConfigSource, workingDir string) string {
	for _, source := range files {
		if source.Type == configuration.SourceTypeProject {
			return filepath.Dir(source.Path)
		}
	}

	return workingDir
}

// shortenConfigPath renders a config path the way a reader would say it aloud:
// relative to the repository, "~" for home, absolute only when it is genuinely
// somewhere else.
func shortenConfigPath(path, root, home string) string {
	if rel, ok := relativeUnder(path, root); ok {
		return rel
	}

	if rel, ok := relativeUnder(path, home); ok {
		return "~/" + rel
	}

	return path
}

// relativeUnder reports whether path sits inside base, and if so returns the
// relative form. Escaping the base (".." components) does not count as inside.
func relativeUnder(path, base string) (string, bool) {
	if path == "" || base == "" {
		return "", false
	}

	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(rel), true
}

// displayTag names the flag that steered config loading, so a line the user
// did not expect can explain why it looks the way it does.
func (m configLoadMode) displayTag() string {
	switch m {
	case configLoadModeExplicitPath:
		return "(--config)"
	case configLoadModeIgnoreConfig:
		return "(--ignore-config)"
	case configLoadModeRepoPath:
		return "(--repo-path)"
	case configLoadModeDefault:
		return ""
	}

	return ""
}
