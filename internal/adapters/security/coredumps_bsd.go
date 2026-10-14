// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build darwin || freebsd

package security

// disableProcDumps is a no-op on BSDs. There is no direct equivalent of
// Linux's PR_SET_DUMPABLE; RLIMIT_CORE=0 already suppresses the coredump
// (the primary defense), so this hook just lets the cross-platform
// DisableCoredumps signature stay uniform.
func disableProcDumps() error { return nil }
