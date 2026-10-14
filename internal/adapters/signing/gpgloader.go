// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"errors"
	"fmt"

	"github.com/ProtonMail/go-crypto/openpgp"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// GPG key loading errors.
var (
	ErrNoGPGKeysFound     = errors.New("no GPG key files found")
	ErrTooManyGPGKeyFiles = errors.New("too many GPG key files")
	ErrAllGPGKeysFailed   = errors.New("all GPG key files failed to load")
)

// GPGKeyLoader defines functions for loading GPG keys.
type GPGKeyLoader func(path string) ([]*openpgp.Entity, error)

// LoadGPGKey loads GPG keys from a file using the injected filesystem.
func LoadGPGKey(fileOps fileops.FileOperations) GPGKeyLoader {
	return func(path string) ([]*openpgp.Entity, error) {
		data, err := fileOps.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read GPG key file: %w", err)
		}

		return parseGPGKeyFile(data)
	}
}

// LoadGPGKeysFromDirectory finds and loads all GPG keys from a directory.
func LoadGPGKeysFromDirectory(fileOps fileops.FileOperations, keyDir string) ([]*openpgp.Entity, error) {
	return LoadGPGKeysFromDirectoryWithLogger(fileOps, keyDir, nil)
}

// LoadGPGKeysFromDirectoryWithLogger finds and loads all GPG keys from a directory with optional debug logging.
func LoadGPGKeysFromDirectoryWithLogger(fileOps fileops.FileOperations, keyDir string, logger domain.Logger) ([]*openpgp.Entity, error) {
	provider := newFileSystemKeyProvider(fileOps, keyDir).WithLogger(logger)

	keyFiles, err := requireKeyFiles(provider, gpgKeyExtensions(), ErrNoGPGKeysFound, ErrTooManyGPGKeyFiles)
	if err != nil {
		return nil, err
	}

	loader := func(path string) ([]*openpgp.Entity, error) {
		data, readErr := provider.ReadKeyFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("failed to read GPG key file: %w", readErr)
		}

		return parseGPGKeyFile(data)
	}

	allEntities, failedCount := loadGPGKeyFiles(loader, keyFiles, logger)

	if err := validateGPGLoadResult(allEntities, failedCount); err != nil {
		return nil, err
	}

	logGPGLoadSummary(logger, len(allEntities), failedCount, len(keyFiles))

	return allEntities, nil
}

// loadGPGKeyFiles loads keys from each file, logging failures at Warning level.
func loadGPGKeyFiles(loader GPGKeyLoader, keyFiles []string, logger domain.Logger) ([]*openpgp.Entity, int) {
	return loadParsedKeyFiles(keyFiles, func(keyFile string) ([]*openpgp.Entity, error) {
		return loader(keyFile)
	}, func(keyFile string, err error) {
		if logger != nil {
			logger.Warn("Failed to load GPG key file (expected armored or binary OpenPGP format)",
				"file", keyFile, "error", err.Error())
		}
	})
}

// validateGPGLoadResult returns an error if all keys failed to load.
func validateGPGLoadResult(entities []*openpgp.Entity, failedCount int) error {
	if len(entities) == 0 && failedCount > 0 {
		return fmt.Errorf("%w: %d files found but none could be parsed (expected armored or binary OpenPGP format)",
			ErrAllGPGKeysFailed, failedCount)
	}

	return nil
}

// logGPGLoadSummary logs a warning summary if some keys failed to load.
func logGPGLoadSummary(logger domain.Logger, loaded, failed, total int) {
	if failed > 0 && logger != nil {
		logger.Warn("Some GPG keys failed to load",
			"loaded", loaded, "failed", failed, "total", total)
	}
}
