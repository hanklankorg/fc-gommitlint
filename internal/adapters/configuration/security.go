// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"errors"
	"os"
	"path/filepath"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// Config path security errors.
var (
	ErrConfigPathSymlink       = errors.New("symlink not allowed")
	ErrConfigPathExecutable    = errors.New("config file should not be executable")
	ErrConfigPathWorldWritable = errors.New("insecure permissions (world-writable)")
	ErrConfigPathGroupWritable = errors.New("insecure permissions (group-writable)")
)

// ValidateConfigPathSecurity validates config path security constraints.
func ValidateConfigPathSecurity(path string, fs FileSystem) error {
	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	if err := domain.DefaultSecurityPolicy().ValidatePath(path); err != nil {
		return err
	}

	if err := validateConfigPathSymlink(path, fs); err != nil {
		return err
	}

	return validateConfigPathPermissions(path, fs)
}

// ResolveConfigPathWithWorkingDir resolves a config path and rejects symlinks when the file exists.
func ResolveConfigPathWithWorkingDir(configPath, workingDir string, fs FileSystem) (string, error) {
	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	if err := domain.DefaultSecurityPolicy().ValidatePath(configPath); err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(configPath)
	if cleanPath == "" {
		return "", nil
	}

	resolvedPath := cleanPath
	if !filepath.IsAbs(cleanPath) {
		resolvedPath = filepath.Join(workingDir, cleanPath)
	}

	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", err
	}

	if err := validateConfigPathSymlinkIfExists(absPath, fs); err != nil {
		return "", err
	}

	return absPath, nil
}

// ResolveSecureConfigPathWithWorkingDir resolves an existing config path and validates file security.
func ResolveSecureConfigPathWithWorkingDir(configPath, workingDir string, fs FileSystem) (string, error) {
	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	resolvedPath, err := ResolveConfigPathWithWorkingDir(configPath, workingDir, fs)
	if err != nil {
		return "", err
	}

	if _, err := fs.Stat(resolvedPath); err != nil {
		return "", err
	}

	if err := validateConfigPathPermissions(resolvedPath, fs); err != nil {
		// Group-writable is advisory: matches normal repo umasks (0002 -> 0664). The loader emits
		// a WarnConfigInsecurePerms warning and proceeds; only true tampering vectors
		// (world-writable, executable, symlink) abort the load.
		if errors.Is(err, ErrConfigPathGroupWritable) {
			return resolvedPath, nil
		}

		return "", err
	}

	return resolvedPath, nil
}

func validateConfigPathSymlinkIfExists(path string, fs FileSystem) error {
	if err := validateConfigPathSymlink(path, fs); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return err
	}

	return nil
}

func validateConfigPathSymlink(path string, fs FileSystem) error {
	info, err := fs.Lstat(path)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return ErrConfigPathSymlink
	}

	return nil
}

func validateConfigPathPermissions(path string, fs FileSystem) error {
	info, err := fs.Stat(path)
	if err != nil {
		return err
	}

	perm := info.Mode().Perm()
	if perm&0o111 != 0 {
		return ErrConfigPathExecutable
	}

	if perm&0o002 != 0 {
		return ErrConfigPathWorldWritable
	}

	if perm&0o020 != 0 {
		return ErrConfigPathGroupWritable
	}

	return nil
}
