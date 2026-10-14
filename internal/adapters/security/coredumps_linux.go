// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build linux

package security

import "golang.org/x/sys/unix"

// disableProcDumps sets PR_SET_DUMPABLE=0 on Linux. Beyond preventing
// coredumps (already covered by RLIMIT_CORE=0), this also restricts the
// /proc/$PID/{mem,environ,maps} interfaces to root only — closing the
// same-UID memory and env-block peek path.
func disableProcDumps() error {
	return unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0)
}
