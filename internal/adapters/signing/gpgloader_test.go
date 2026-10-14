// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestLoadGPGKey_NonexistentFileFails(t *testing.T) {
	t.Parallel()

	t.Run("handles nonexistent file", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		loader := LoadGPGKey(fileOps)

		entities, err := loader("/nonexistent/key.gpg")
		check.Error(t, err)
		check.Nil(t, entities)
	})
}

func TestLoadGPGKeysFromDirectory_FailsClosedOnInvalidInputs(t *testing.T) {
	t.Parallel()

	t.Run("handles missing directory", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		_, err := LoadGPGKeysFromDirectory(fileOps, "/nonexistent")
		check.Error(t, err)
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		_, err := LoadGPGKeysFromDirectory(fileOps, "../invalid")
		check.Error(t, err)
	})

	t.Run("rejects too many key files", func(t *testing.T) {
		t.Parallel()

		keyDir := t.TempDir()
		fs := filesystem.NewOSFileSystem()
		fileOps := fileops.NewFileOperations(fs)

		// Create more than MaxKeyFiles
		for i := 0; i <= settings.DefaultMaxKeyFiles; i++ {
			keyFile := fmt.Sprintf("%s/key%d.gpg", keyDir, i)
			err := fs.WriteFile(keyFile, []byte("dummy"), 0644)
			check.NoError(t, err)
		}

		_, err := LoadGPGKeysFromDirectory(fileOps, keyDir)
		check.Error(t, err)
		check.ErrorIs(t, err, ErrTooManyGPGKeyFiles)
	})

	t.Run("returns error when all keys fail to parse", func(t *testing.T) {
		t.Parallel()

		keyDir := t.TempDir()
		fs := filesystem.NewOSFileSystem()
		fileOps := fileops.NewFileOperations(fs)

		// Create invalid key files
		for i := range 3 {
			keyFile := fmt.Sprintf("%s/invalid%d.gpg", keyDir, i)
			err := fs.WriteFile(keyFile, []byte("not a valid gpg key"), 0644)
			check.NoError(t, err)
		}

		logger := &mockGPGLogger{}
		_, err := LoadGPGKeysFromDirectoryWithLogger(fileOps, keyDir, logger)

		check.Error(t, err)
		check.ErrorIs(t, err, ErrAllGPGKeysFailed)

		// Verify warnings were logged for each failed key
		check.Equal(t, 3, logger.warnCount)
	})
}

// mockGPGLogger captures log calls for testing.
type mockGPGLogger struct {
	warnCount int
}

func (m *mockGPGLogger) Trace(_ string, _ ...any)           {}
func (m *mockGPGLogger) TraceFunc(_ func() (string, []any)) {}
func (m *mockGPGLogger) Debug(_ string, _ ...any)           {}
func (m *mockGPGLogger) Info(_ string, _ ...any)            {}
func (m *mockGPGLogger) Warn(_ string, _ ...any)            { m.warnCount++ }
func (m *mockGPGLogger) Error(_ string, _ ...any)           {}
func (m *mockGPGLogger) Log(_ string, _ string, _ ...any)   {}
