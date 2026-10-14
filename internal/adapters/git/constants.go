// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package git

// Git-related constants.
const (
	// HeadRefName is the branch name reported when the repository HEAD is detached.
	HeadRefName = "HEAD"

	// InitialCommitMapCapacity is the initial capacity for commit reachability maps.
	// This avoids frequent map resizing for typical repository sizes.
	InitialCommitMapCapacity = 200
)
