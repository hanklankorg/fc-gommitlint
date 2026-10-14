// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build linux || darwin || freebsd

package security

import "golang.org/x/sys/unix"

// DisableCoredumps shrinks RLIMIT_CORE to zero so the kernel will not write
// a core file on crash, and on Linux additionally marks the process
// non-dumpable (PR_SET_DUMPABLE=0) which further restricts /proc/$PID/{mem,
// environ} to root only.
//
// Why: a crash with sensitive material in memory writes the entire process
// image to disk. The file lands somewhere readable (often /var/lib/systemd/
// coredump/ or the CWD) and persists. mlock does not defend against
// coredumps — the kernel includes locked pages in the dump. Suppressing
// the dump at the rlimit level is the only general defense.
//
// Policy lives in the caller: the CLI layer decides whether to invoke this
// (gated on --allow-coredumps for developers diagnosing gommitlint itself).
// This helper is unconditional — it does what its name says.
//
// Returns a non-nil error only when the syscall itself failed (rare —
// shrinking your own soft limit is generally permitted even for
// unprivileged processes). Callers should debug-log and continue.
func DisableCoredumps() error {
	if err := unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{Cur: 0, Max: 0}); err != nil {
		return err
	}

	return disableProcDumps()
}
