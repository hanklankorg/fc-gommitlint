// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	errPanicReportFileHandle = errors.New("create panic report file handle")
	errPanicReportDirNotAbs  = errors.New("panic report directory is not absolute")
)

func writePrivatePanicReport(reportDir, reportName string, content []byte) (string, error) {
	directory, err := openPrivatePanicDirectory(reportDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = directory.Close() }()

	reportPath := filepath.Join(reportDir, reportName)

	report, err := openPanicReportAt(directory, reportPath, reportName)
	if err != nil {
		return "", err
	}

	if err := writeAndClosePanicReport(report, content); err != nil {
		_ = unix.Unlinkat(int(directory.Fd()), reportName, 0)

		return "", err
	}

	return reportPath, nil
}

func openPanicReportAt(directory *os.File, reportPath, reportName string) (*os.File, error) {
	flags := unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC | unix.O_NOFOLLOW

	fd, err := unix.Openat(int(directory.Fd()), reportName, flags, uint32(panicReportMode))
	if err != nil {
		return nil, fmt.Errorf("create panic report: %w", err)
	}

	report := os.NewFile(uintptr(fd), reportPath)
	if report == nil {
		_ = unix.Close(fd)
		_ = unix.Unlinkat(int(directory.Fd()), reportName, 0)

		return nil, errPanicReportFileHandle
	}

	return report, nil
}

func openPrivatePanicDirectory(path string) (*os.File, error) {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return nil, errPanicReportDirNotAbs
	}

	currentFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open filesystem root: %w", err)
	}

	for component := range strings.SplitSeq(strings.TrimPrefix(cleanPath, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}

		nextFD, openErr := openOrCreatePanicDirectoryAt(currentFD, component)
		_ = unix.Close(currentFD)

		if openErr != nil {
			return nil, openErr
		}

		currentFD = nextFD
	}

	if err := unix.Fchmod(currentFD, uint32(panicReportDirMode)); err != nil {
		_ = unix.Close(currentFD)

		return nil, fmt.Errorf("secure panic report directory: %w", err)
	}

	return os.NewFile(uintptr(currentFD), cleanPath), nil
}

func openOrCreatePanicDirectoryAt(parentFD int, name string) (int, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW

	nextFD, err := unix.Openat(parentFD, name, flags, 0)
	if err == nil {
		return nextFD, nil
	}

	if !errors.Is(err, unix.ENOENT) {
		return -1, fmt.Errorf("open panic report directory component %q: %w", name, err)
	}

	if err := unix.Mkdirat(parentFD, name, uint32(panicReportDirMode)); err != nil && !errors.Is(err, unix.EEXIST) {
		return -1, fmt.Errorf("create panic report directory component %q: %w", name, err)
	}

	nextFD, err = unix.Openat(parentFD, name, flags, 0)
	if err != nil {
		return -1, fmt.Errorf("open created panic report directory component %q: %w", name, err)
	}

	return nextFD, nil
}

func writeAndClosePanicReport(report *os.File, content []byte) error {
	if err := report.Chmod(panicReportMode); err != nil {
		_ = report.Close()

		return fmt.Errorf("secure panic report: %w", err)
	}

	if _, err := report.Write(content); err != nil {
		_ = report.Close()

		return fmt.Errorf("write panic report: %w", err)
	}

	if err := report.Close(); err != nil {
		return fmt.Errorf("close panic report: %w", err)
	}

	return nil
}
