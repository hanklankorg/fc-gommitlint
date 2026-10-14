// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"errors"
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func newFileSystemKeyProvider(fileOps fileops.FileOperations, keyDir string) *FileSystemKeyProvider {
	return NewFileSystemKeyProviderWithFSAndLimit(keyDir, fileOps, settings.DefaultMaxKeyFiles)
}

func gpgKeyExtensions() []string {
	return []string{keyExtGPG, keyExtASC, keyExtPub}
}

func sshKeyExtensions() []string {
	return []string{keyExtPub, keyExtSSH}
}

func requireKeyFiles(provider *FileSystemKeyProvider, extensions []string, noKeysErr error, tooManyErr error) ([]string, error) {
	keyFiles, err := provider.ListKeyFiles(extensions)
	if err != nil {
		if errors.Is(err, errTooManyKeyFiles) {
			return nil, fmt.Errorf("%w: %w", tooManyErr, err)
		}

		return nil, err
	}

	if len(keyFiles) == 0 {
		return nil, fmt.Errorf("%w in %s", noKeysErr, provider.directory)
	}

	return keyFiles, nil
}

func loadParsedKeyFiles[T any](keyFiles []string, load func(string) ([]T, error), onError func(string, error)) ([]T, int) {
	items := make([]T, 0, len(keyFiles))
	failedCount := 0

	for _, keyFile := range keyFiles {
		loaded, err := load(keyFile)
		if err != nil {
			failedCount++

			if onError != nil {
				onError(keyFile, err)
			}

			continue
		}

		items = append(items, loaded...)
	}

	return items, failedCount
}
