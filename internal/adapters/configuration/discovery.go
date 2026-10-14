// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// EnvLookup reads environment variables. Named for the capability rather than
// for a thing: its whole surface is one lookup, and adapters/ci.Environment --
// a struct of detected CI facts -- is what "an environment" means elsewhere in
// this codebase.
type EnvLookup interface {
	GetEnv(key string) string
}

// EnvUnsetter is an optional capability on EnvLookup for callers that want
// to scrub secret env vars from the process environment block after first
// use. Used by managed-source sync: every variable referenced by a sync
// (source URL or header value) is treated as a secret and unset once the
// sync completes, narrowing the /proc/$PID/environ exposure window.
//
// Optional rather than required so test Environments don't accidentally
// mutate the test process's real env.
type EnvUnsetter interface {
	UnsetEnv(key string)
}

// OSEnvironment implements EnvLookup using os.Getenv.
type OSEnvironment struct{}

// GetEnv returns the value of the environment variable named by the key.
func (OSEnvironment) GetEnv(key string) string {
	return os.Getenv(key)
}

// UnsetEnv removes the named variable from the process environment block.
// Best-effort: the only documented failure is an invalid name containing
// '=', which IsValidManagedSourceEnvName already rules out at config-parse
// time. Callers treat this as residue minimization, not a security boundary.
func (OSEnvironment) UnsetEnv(key string) {
	_ = os.Unsetenv(key)
}

// FileChecker is a function type for checking if a file exists.
type FileChecker func(path string) bool

// FileSystem is an interface for file system operations.
type FileSystem = filesystem.FileSystem

// GetConfigPaths returns configuration file search paths.
// Pass empty strings to use current directory. EnvLookup parameter allows dependency injection.
func GetConfigPaths(repoPath, workingDir string, env EnvLookup) []string {
	// Determine base directory based on provided paths
	var baseDir string
	if workingDir != "" {
		baseDir = determineBaseDirectoryFromWorkingDir(repoPath, workingDir)
	} else {
		baseDir = determineBaseDirectory(repoPath)
	}

	localPaths := getLocalConfigPaths(baseDir)
	xdgPaths := getXDGConfigPathsWithEnv(env)

	return append(localPaths, xdgPaths...)
}

// determineBaseDirectory safely determines the base directory for config search.
func determineBaseDirectory(repoPath string) string {
	if repoPath == "" {
		return "."
	}
	// Basic security validation without external dependencies
	cleanPath := filepath.Clean(repoPath)
	if domain.HasPathTraversal(cleanPath) {
		return "." // Fall back to current directory for suspicious paths
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "." // Fall back on error
	}

	return absPath
}

// determineBaseDirectoryFromWorkingDir safely determines the base directory for config search from a specific working directory.
func determineBaseDirectoryFromWorkingDir(repoPath, workingDir string) string {
	if repoPath == "" {
		if workingDir == "" {
			return "."
		}

		return workingDir
	}
	// Basic security validation without external dependencies
	cleanPath := filepath.Clean(repoPath)
	if domain.HasPathTraversal(cleanPath) {
		if workingDir == "" {
			return "." // Fall back to current directory for suspicious paths
		}

		return workingDir
	}
	// If repoPath is relative, resolve it from workingDir
	if !filepath.IsAbs(cleanPath) && workingDir != "" {
		cleanPath = filepath.Join(workingDir, cleanPath)
	}

	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		if workingDir == "" {
			return "." // Fall back on error
		}

		return workingDir
	}

	return absPath
}

// getLocalConfigPaths returns local configuration file paths for a base directory.
func getLocalConfigPaths(baseDir string) []string {
	return []string{
		filepath.Join(baseDir, ".gommitlint.yaml"),
		filepath.Join(baseDir, ".gommitlint.yml"),
		filepath.Join(baseDir, ".gommitlint.toml"),
	}
}

// getXDGConfigPathsWithEnv returns XDG configuration directory paths using a custom environment.
func getXDGConfigPathsWithEnv(env EnvLookup) []string {
	return getXDGConfigPathsWithEnvAndFS(env, filesystem.NewOSFileSystem())
}

// getXDGConfigPathsWithEnvAndFS returns XDG configuration directory paths using custom environment and filesystem.
func getXDGConfigPathsWithEnvAndFS(env EnvLookup, fs FileSystem) []string {
	xdgConfigHome := env.GetEnv("XDG_CONFIG_HOME")
	// Fall back to HOME/.config if XDG_CONFIG_HOME is not set
	if xdgConfigHome == "" {
		home := env.GetEnv("HOME")
		if home != "" {
			xdgConfigHome = filepath.Join(home, ".config")
		} else {
			return []string{}
		}
	}
	// Validate XDG path for security - check for null bytes
	if strings.ContainsRune(xdgConfigHome, 0) {
		return []string{} // Reject paths containing null bytes
	}
	// Validate XDG path for security
	cleanXDG := filepath.Clean(xdgConfigHome)
	if !filepath.IsAbs(cleanXDG) || domain.HasPathTraversal(cleanXDG) {
		return []string{}
	}

	gommitlintDir := filepath.Join(cleanXDG, "gommitlint")
	if _, err := fs.Stat(gommitlintDir); err != nil {
		return []string{}
	}

	return []string{
		filepath.Join(gommitlintDir, "settings.yaml"),
		filepath.Join(gommitlintDir, "settings.yml"),
		filepath.Join(gommitlintDir, "settings.toml"),
	}
}

// defaultFileExists is the default file existence checker.
// Uses os.Stat for checking file existence.
func defaultFileExists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// ConfigSourceType identifies the type of configuration source.
type ConfigSourceType string

const (
	// SourceTypeDefaults represents built-in default configuration.
	SourceTypeDefaults ConfigSourceType = "defaults"
	// SourceTypeExtended represents a config file loaded via extends.
	SourceTypeExtended ConfigSourceType = "extended"
	// SourceTypePreset represents a built-in preset loaded via extends.
	SourceTypePreset ConfigSourceType = "preset"
	// SourceTypeGlobal represents user-level global configuration (~/.config/gommitlint/).
	SourceTypeGlobal ConfigSourceType = "global"
	// SourceTypeProject represents project-level configuration (.gommitlint.yaml).
	SourceTypeProject ConfigSourceType = "project"
	// SourceTypeCLI represents configuration from CLI flags.
	SourceTypeCLI ConfigSourceType = "cli"
)

// ConfigSource describes where configuration was loaded from.
type ConfigSource struct {
	Path string           `json:"path" yaml:"path"` // file path, or "defaults" for built-in
	Type ConfigSourceType `json:"type" yaml:"type"` // type of source
}

// ConfigSourceNode describes one config file and its extended children.
type ConfigSourceNode struct {
	Children []ConfigSourceNode `json:"children,omitempty" yaml:"children,omitempty"`
	Path     string             `json:"path"               yaml:"path"`
	Type     ConfigSourceType   `json:"type"               yaml:"type"`
}

// FoundConfigs holds all discovered configuration files in merge order.
// Global configs come first (lower priority), project configs last (higher priority).
type FoundConfigs struct {
	Global  string // XDG global config path, empty if not found
	Project string // Project-local config path, empty if not found
}

// Sources returns ConfigSource entries for all found configs.
func (f FoundConfigs) Sources() []ConfigSource {
	var sources []ConfigSource
	if f.Global != "" {
		sources = append(sources, ConfigSource{Path: f.Global, Type: SourceTypeGlobal})
	}

	if f.Project != "" {
		sources = append(sources, ConfigSource{Path: f.Project, Type: SourceTypeProject})
	}

	return sources
}

// FindAllExistingConfigFilesWithEnv finds all existing config files using custom environment.
func FindAllExistingConfigFilesWithEnv(repoPath, workingDir string, env EnvLookup) FoundConfigs {
	return findAllConfigsWithChecker(repoPath, workingDir, env, defaultFileExists)
}

// FindAllExistingConfigFilesWithEnvAndFS finds all config files using custom environment and filesystem.
func FindAllExistingConfigFilesWithEnvAndFS(repoPath, workingDir string, env EnvLookup, fs FileSystem) FoundConfigs {
	checker := func(path string) bool {
		_, err := fs.Stat(path)

		return err == nil
	}

	return findAllConfigsWithCheckerAndFS(repoPath, workingDir, env, checker, fs)
}

// findAllConfigsWithChecker is the internal implementation for finding all configs.
func findAllConfigsWithChecker(repoPath, workingDir string, env EnvLookup, checker FileChecker) FoundConfigs {
	return findAllConfigsWithCheckerAndFS(repoPath, workingDir, env, checker, nil)
}

// findAllConfigsWithCheckerAndFS is the internal implementation for finding all configs with optional custom filesystem.
func findAllConfigsWithCheckerAndFS(repoPath, workingDir string, env EnvLookup, checker FileChecker, fs FileSystem) FoundConfigs {
	var found FoundConfigs

	// Determine base directory for local/project paths
	var baseDir string
	if workingDir != "" {
		baseDir = determineBaseDirectoryFromWorkingDir(repoPath, workingDir)
	} else {
		baseDir = determineBaseDirectory(repoPath)
	}

	// Find first existing XDG/global config
	var xdgPaths []string
	if fs != nil {
		xdgPaths = getXDGConfigPathsWithEnvAndFS(env, fs)
	} else {
		xdgPaths = getXDGConfigPathsWithEnv(env)
	}

	for _, p := range xdgPaths {
		if checker(p) {
			found.Global = p

			break // only use first matching global config
		}
	}

	// Find first existing local/project config
	localPaths := getLocalConfigPaths(baseDir)
	for _, p := range localPaths {
		if checker(p) {
			found.Project = p

			break // only use first matching project config
		}
	}

	return found
}
