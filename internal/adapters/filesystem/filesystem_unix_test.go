//go:build unix

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package filesystem

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"golang.org/x/sys/unix"
)

func TestOSFileSystem_ReadFileWithLimit_RejectsGrowingStream(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "growing-stream")
	check.NoError(t, unix.Mkfifo(path, 0o600))

	writeResult := make(chan error, 1)

	go func() {
		writer, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			writeResult <- err

			return
		}

		_, writeErr := writer.WriteString("abcde")
		writeResult <- errors.Join(writeErr, writer.Close())
	}()

	content, err := NewOSFileSystem().ReadFileWithLimit(path, 4)
	check.ErrorIs(t, err, ErrFileTooLarge)
	check.Empty(t, content)
	check.NoError(t, <-writeResult)
}

// TestOSFileSystem_WriteFileAtomic_SetsModeAndLeavesNoTempFile pins the mode a
// written file gets (git ignores a hook that is not executable) and that a
// failed rename removes the temporary file.
func TestOSFileSystem_WriteFileAtomic_SetsModeAndLeavesNoTempFile(t *testing.T) {
	t.Parallel()

	fsys := NewOSFileSystem()

	for _, perm := range []os.FileMode{0o755, 0o600} {
		t.Run(perm.String(), func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "hook")

			check.NoError(t, fsys.WriteFileAtomic(path, []byte("#!/bin/sh\n"), perm))

			info, err := os.Stat(path)
			check.NoError(t, err)
			check.Equal(t, perm, info.Mode().Perm())

			content, err := os.ReadFile(path)
			check.NoError(t, err)
			check.Equal(t, "#!/bin/sh\n", string(content))

			entries, err := os.ReadDir(dir)
			check.NoError(t, err)
			check.Len(t, entries, 1)
		})
	}

	t.Run("rename failure removes the temp file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		// A non-empty directory cannot be replaced by a file, so the rename fails.
		target := filepath.Join(dir, "occupied")
		check.NoError(t, os.Mkdir(target, 0o700))
		check.NoError(t, os.WriteFile(filepath.Join(target, "keep"), []byte("x"), 0o600))

		err := fsys.WriteFileAtomic(target, []byte("data"), 0o600)
		check.ErrorContains(t, err, "failed to rename file")

		entries, err := os.ReadDir(dir)
		check.NoError(t, err)
		check.Len(t, entries, 1)
		check.Equal(t, "occupied", entries[0].Name())
	})
}
