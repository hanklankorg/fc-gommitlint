// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"errors"
	"fmt"
	"os"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Config path errors.
var (
	ErrConfigFileNotFound = errors.New("config file not found")
)

// configPathValidationError represents a config path validation error.
type configPathValidationError struct {
	Path   string
	Reason string
	Cause  error
}

func (e configPathValidationError) Error() string {
	return fmt.Sprintf("invalid config path %s: %s", e.Path, e.Reason)
}

func (e configPathValidationError) Unwrap() error {
	return e.Cause
}

func newConfigPathValidationError(path string, err error) configPathValidationError {
	return configPathValidationError{
		Path:   path,
		Reason: configPathReason(err),
		Cause:  err,
	}
}

func configPathReason(err error) string {
	switch {
	case errors.Is(err, domain.ErrPathEmpty):
		return domain.ErrPathEmpty.Error()
	case errors.Is(err, domain.ErrPathTraversal):
		return domain.ErrPathTraversal.Error()
	case errors.Is(err, domain.ErrPathDangerous):
		return domain.ErrPathDangerous.Error()
	case errors.Is(err, domain.ErrParameterTooLong):
		return domain.ErrParameterTooLong.Error()
	case errors.Is(err, configuration.ErrConfigPathSymlink):
		return configuration.ErrConfigPathSymlink.Error()
	case errors.Is(err, configuration.ErrConfigPathExecutable):
		return configuration.ErrConfigPathExecutable.Error()
	case errors.Is(err, configuration.ErrConfigPathWorldWritable):
		return configuration.ErrConfigPathWorldWritable.Error()
	case errors.Is(err, configuration.ErrConfigPathGroupWritable):
		return configuration.ErrConfigPathGroupWritable.Error()
	case errors.Is(err, ErrConfigFileNotFound):
		return ErrConfigFileNotFound.Error()
	default:
		return err.Error()
	}
}

// secureConfigPathValidationWithWorkingDir validates config path security with an explicit working directory.
//
// Preset references ("preset:secure", "preset:conventional", etc.) skip
// filesystem validation: they're registry keys, not paths. The downstream
// loader at configuration.loadConfigTree recognises the same prefix via
// ParsePresetReference, so passing the value through unchanged lets the CLI
// flag accept everything that YAML `extends:` accepts; without it the flag
// errors with "config file not found" on a feature the help text advertises.
func secureConfigPathValidationWithWorkingDir(configPath, workingDir string) (string, error) {
	if _, ok := configuration.ParsePresetReference(configPath); ok {
		return configPath, nil
	}

	validatedPath, err := configuration.ResolveSecureConfigPathWithWorkingDir(configPath, workingDir, nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", newConfigPathValidationError(configPath, ErrConfigFileNotFound)
		}

		return "", newConfigPathValidationError(configPath, err)
	}

	return validatedPath, nil
}
