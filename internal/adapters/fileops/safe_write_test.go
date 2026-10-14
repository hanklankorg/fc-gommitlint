// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
)

var (
	errTestCheckRejected = errors.New("custom hook")
	errTestLock          = errors.New("test lock failed")
)

func TestFileOperations_SafeWriteFileWithCheck_UsesInjectedLocker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "commit-msg")
	locker := &recordingLocker{}
	fs := filesystem.NewOSFileSystem()
	ops := newFileOperationsWithLocker(fs, locker)

	err := ops.SafeWriteFileWithCheck(target, []byte("new script"), 0o600, nil)

	check.NoError(t, err)
	check.True(t, locker.called)
	check.Equal(t, filepath.Join(dir, ".gommitlint.lock"), locker.lockPath)

	content, err := fs.ReadFile(target)
	check.NoError(t, err)
	check.Equal(t, "new script", string(content))
}

func TestFileOperations_SafeWriteFileWithCheck_ReturnsLockerError(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "commit-msg")
	locker := &recordingLocker{err: errTestLock}
	ops := newFileOperationsWithLocker(filesystem.NewOSFileSystem(), locker)

	err := ops.SafeWriteFileWithCheck(target, []byte("new script"), 0o600, nil)

	check.ErrorIs(t, err, errTestLock)
	check.True(t, locker.called)
}

func TestFileOperations_SafeWriteFileWithCheck_RefusesUnreadableExistingFile(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "commit-msg")
	check.NoError(t, os.WriteFile(target, []byte("custom hook"), 0o600))

	fs := &readFailingFileSystem{OSFileSystem: filesystem.NewOSFileSystem(), readErr: os.ErrPermission}
	ops := newFileOperationsWithLocker(fs, &recordingLocker{})

	err := ops.SafeWriteFileWithCheck(target, []byte("gommitlint hook"), 0o755, func([]byte) error { return nil })

	check.ErrorIs(t, err, os.ErrPermission)
	check.False(t, fs.wrote)
}

func TestFileOperations_SafeWriteFileWithCheck_RefusesOversizedExistingFile(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "commit-msg")
	check.NoError(t, os.WriteFile(target, make([]byte, MaxCheckedFileSize+1), 0o600))

	fs := filesystem.NewOSFileSystem()
	ops := newFileOperationsWithLocker(fs, &recordingLocker{})

	err := ops.SafeWriteFileWithCheck(target, []byte("gommitlint hook"), 0o755, func([]byte) error { return nil })

	check.ErrorIs(t, err, filesystem.ErrFileTooLarge)
}

func TestFileOperations_SafeRemoveFileWithCheck_LeavesRejectedFile(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "commit-msg")
	check.NoError(t, os.WriteFile(target, []byte("custom hook"), 0o600))

	locker := &recordingLocker{}
	ops := newFileOperationsWithLocker(filesystem.NewOSFileSystem(), locker)

	err := ops.SafeRemoveFileWithCheck(target, func([]byte) error { return errTestCheckRejected })

	check.ErrorIs(t, err, ErrCheckFailed)
	check.ErrorIs(t, err, errTestCheckRejected)
	check.True(t, locker.called)

	_, statErr := os.Stat(target)
	check.NoError(t, statErr)
}

// The lock is claimed by creating the lock file exclusively, so a lock left by
// another writer refuses the write: the check never runs, the target keeps its
// content and the foreign lock is not removed. Once that lock is gone the same
// write goes through.
func TestFileOperations_SafeWriteFileWithCheck_RefusesWhileAnotherLockIsHeld(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "commit-msg")
	lockPath := filepath.Join(dir, ".gommitlint.lock")

	const (
		existing    = "custom hook"
		foreignLock = "held by another gommitlint\n"
	)

	check.NoError(t, os.WriteFile(target, []byte(existing), 0o600))
	check.NoError(t, os.WriteFile(lockPath, []byte(foreignLock), 0o600))

	ops := NewFileOperations(filesystem.NewOSFileSystem())
	checked := false

	err := ops.SafeWriteFileWithCheck(target, []byte("gommitlint hook"), 0o755, func([]byte) error {
		checked = true

		return nil
	})

	check.ErrorIs(t, err, ErrLockAcquisitionFailed)
	check.False(t, checked, "the check must not run without the lock")

	content, readErr := os.ReadFile(target)
	check.NoError(t, readErr)
	check.Equal(t, existing, string(content))

	lock, readErr := os.ReadFile(lockPath)
	check.NoError(t, readErr)
	check.Equal(t, foreignLock, string(lock), "a lock held by another writer must be left in place")

	check.NoError(t, os.Remove(lockPath))
	check.NoError(t, ops.SafeWriteFileWithCheck(target, []byte("gommitlint hook"), 0o755, nil))

	content, readErr = os.ReadFile(target)
	check.NoError(t, readErr)
	check.Equal(t, "gommitlint hook", string(content))
	check.NoFileExists(t, lockPath)
}

func TestFileOperations_SafeWriteFileWithCheck_RejectsNonRegularTargetBeforeRead(t *testing.T) {
	t.Parallel()

	fs := &specialFileSystem{OSFileSystem: filesystem.NewOSFileSystem()}
	ops := newFileOperationsWithLocker(fs, &recordingLocker{})

	err := ops.SafeWriteFileWithCheck("/hooks/commit-msg", []byte("hook"), 0o755, nil)

	check.ErrorIs(t, err, ErrNotRegularFile)
	check.False(t, fs.read)
}

// A symlink is refused as a symlink, not as a generic non-regular file, before
// the check runs, and both the link and the file it points at are left as they
// were.
func TestFileOperations_SafeWriteFileWithCheck_RefusesSymlinkAndLeavesTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "commit-msg")

	check.NoError(t, os.WriteFile(target, []byte("custom hook"), 0o600))
	check.NoError(t, os.Symlink(target, link))

	ops := NewFileOperations(filesystem.NewOSFileSystem())
	checked := false

	err := ops.SafeWriteFileWithCheck(link, []byte("gommitlint hook"), 0o755, func([]byte) error {
		checked = true

		return nil
	})

	check.ErrorIs(t, err, ErrSymlinkNotAllowed)
	check.False(t, checked, "the check must not run for a refused target")

	content, readErr := os.ReadFile(target)
	check.NoError(t, readErr)
	check.Equal(t, "custom hook", string(content))

	info, lstatErr := os.Lstat(link)
	check.NoError(t, lstatErr)
	check.NotEqual(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "the link must still be a link")
}

type readFailingFileSystem struct {
	filesystem.OSFileSystem

	readErr error
	wrote   bool
}

type specialFileSystem struct {
	filesystem.OSFileSystem

	read bool
}

func (fs *specialFileSystem) Lstat(string) (os.FileInfo, error) {
	return specialFileInfo{}, nil
}

func (fs *specialFileSystem) ReadFileWithLimit(string, int64) ([]byte, error) {
	fs.read = true

	return nil, nil
}

type specialFileInfo struct{}

func (specialFileInfo) Name() string       { return "commit-msg" }
func (specialFileInfo) Size() int64        { return 0 }
func (specialFileInfo) Mode() os.FileMode  { return os.ModeNamedPipe }
func (specialFileInfo) ModTime() time.Time { return time.Time{} }
func (specialFileInfo) IsDir() bool        { return false }
func (specialFileInfo) Sys() any           { return nil }

func (fs *readFailingFileSystem) ReadFileWithLimit(string, int64) ([]byte, error) {
	return nil, fs.readErr
}

func (fs *readFailingFileSystem) WriteFileAtomic(string, []byte, os.FileMode) error {
	fs.wrote = true

	return nil
}

type recordingLocker struct {
	err      error
	lockPath string
	called   bool
}

func (l *recordingLocker) WithLock(lockPath string, operation func() error) error {
	l.called = true
	l.lockPath = lockPath

	if l.err != nil {
		return l.err
	}

	return operation()
}
