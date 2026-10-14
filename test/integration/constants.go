// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build integration

package integration

// File permissions for tests.
const (
	// FilePermReadWrite is the permission for files that should be read-write by owner only.
	FilePermReadWrite = 0600

	// FilePermPublicRead is the permission for files that can be read by anyone.
	FilePermPublicRead = 0644

	// FilePermPublicExec is the permission for executable files that can be run by anyone.
	FilePermPublicExec = 0755
)
