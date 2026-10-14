// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build linux

package security

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestDisableCoredumps_SuccessIsObservable(t *testing.T) {
	// Not parallel: this lowers process-wide limits irreversibly (the hard
	// limit cannot be raised again), so it must not interleave with anything
	// reading them.
	if err := DisableCoredumps(); err != nil {
		t.Fatalf("DisableCoredumps: the coredump suppression this package exists to provide did not apply: %v", err)
	}

	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &limit); err != nil {
		t.Fatalf("Getrlimit(RLIMIT_CORE): %v", err)
	}
	// Both halves: a soft limit of zero alone can be raised again by the
	// process, so the hard limit is what makes the suppression stick.
	if limit.Cur != 0 || limit.Max != 0 {
		t.Fatalf("DisableCoredumps reported success but RLIMIT_CORE = {%d, %d}", limit.Cur, limit.Max)
	}

	// The Linux-only half of the contract. PR_SET_DUMPABLE=0 is what closes
	// /proc/$PID/{mem,environ,maps} to same-UID readers; RLIMIT_CORE alone does
	// not, so the two are separate claims and deserve separate checks.
	dumpable, err := unix.PrctlRetInt(unix.PR_GET_DUMPABLE, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("PR_GET_DUMPABLE: %v", err)
	}

	if dumpable != 0 {
		t.Fatalf("DisableCoredumps reported success but the process is still dumpable (%d)", dumpable)
	}
}
