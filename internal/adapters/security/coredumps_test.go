// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package security

import (
	"testing"
)

// TestDisableCoredumps_DoesNotPanic smoke-tests the public surface. We
// don't assert success because the syscall behaviour depends on the
// runner's RLIMIT_CORE policy and (on Linux) seccomp filters in CI. The
// contract being verified is "doesn't panic, doesn't deadlock, returns
// either nil or an error" — that the caller can wire it into the startup
// path without defensive guards.
//
// The policy gate (--allow-coredumps / GOMMITLINT_ALLOW_COREDUMPS) lives
// in the CLI layer and is exercised by the CLI tests, not here — this
// package is pure mechanism and unconditionally attempts the syscall.
func TestDisableCoredumps_DoesNotPanic(t *testing.T) {
	t.Parallel()

	// caller's signal to log, not to act on.
	_ = DisableCoredumps()
}
