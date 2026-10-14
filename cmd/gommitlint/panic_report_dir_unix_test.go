// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestPrivatePanicDirectory_DescriptorResistsPathReplacement(t *testing.T) {
	root := panicTestDirectory(t)
	reportDir := filepath.Join(root, "state", appName, "crash")
	directory, err := openPrivatePanicDirectory(reportDir)
	check.NoError(t, err)

	defer func() { _ = directory.Close() }()

	movedDir := reportDir + "-moved"
	check.NoError(t, os.Rename(reportDir, movedDir))

	attackerDir := filepath.Join(root, "attacker")
	check.NoError(t, os.Mkdir(attackerDir, 0o700))
	check.NoError(t, os.Symlink(attackerDir, reportDir))

	const reportName = "panic-race.log"

	// Pass the path as it now resolves, through the attacker's symlink: only
	// creation relative to the held directory descriptor lands the report in
	// the moved directory.
	report, openErr := openPanicReportAt(directory, filepath.Join(reportDir, reportName), reportName)
	check.NoError(t, openErr)
	check.NoError(t, writeAndClosePanicReport(report, []byte("private")))

	content, readErr := os.ReadFile(filepath.Join(movedDir, reportName))
	check.NoError(t, readErr)
	check.Equal(t, "private", string(content))

	_, attackerErr := os.Stat(filepath.Join(attackerDir, reportName))
	check.Error(t, attackerErr)
}
