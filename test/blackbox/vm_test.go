//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func requireNativeVM(t *testing.T) {
	t.Helper()
	const path = "/var/run/itiquette-vm/offline"
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		t.Fatal("native black-box tests require the Just VM launcher and its root-owned offline record")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		t.Fatal("VM offline record is not root-owned")
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "arch="+runtime.GOARCH+"\n") ||
		!strings.Contains(strings.ToLower(string(data)), "kernel="+runtime.GOOS+"\n") ||
		!strings.Contains(string(data), "network=none\n") {
		t.Fatal("VM record does not match this execution")
	}
}
