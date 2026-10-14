// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

// Commit message constraints.
const (
	// ConventionalCommitFormatTemplate is the expected conventional commit format string.
	ConventionalCommitFormatTemplate = "type(scope): description"

	// ShortHashLength is the abbreviated git SHA length used in human-facing
	// output. Matches git's default --short=7 behaviour.
	ShortHashLength = 7
)

// File permissions.
const (
	// FilePermReadWrite is the permission for files that should be read-write by owner only.
	FilePermReadWrite = 0600

	// FilePermReadWriteExec is the permission for executable files.
	FilePermReadWriteExec = 0700

	// FilePermPublicRead is the permission for files that can be read by anyone.
	FilePermPublicRead = 0644

	// FilePermPublicExec is the permission for executable files that can be run by anyone.
	FilePermPublicExec = 0755
)

// Size limits.
const (
	// MaxCommitMessageSize is the maximum reasonable size for a commit message (10MB).
	MaxCommitMessageSize = 10 * 1024 * 1024

	// MaxPathLength is a reasonable maximum path length (1024 characters).
	// While Linux supports 4096, Windows traditionally limits to 260.
	// 1024 is a reasonable middle ground for cross-platform compatibility.
	MaxPathLength = 1024

	// MaxRefLength is a conservative maximum length for git references.
	// Uses the same threshold as path length for consistent limits.
	MaxRefLength = 1024
)
