// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build !linux && !darwin && !freebsd

package security

// Fallback implementation for platforms without OS-specific hardening
// (e.g. netbsd, openbsd). The helper is best-effort by contract, so the
// defense is unavailable rather than an error. This keeps the package and
// release binaries buildable for every GOOS in the release matrix.

// DisableCoredumps is a no-op on platforms without a supported implementation.
func DisableCoredumps() error { return nil }
