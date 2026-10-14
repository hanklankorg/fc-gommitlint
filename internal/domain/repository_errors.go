// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import "errors"

// ErrReferenceNotFound indicates a git reference could not be resolved.
var ErrReferenceNotFound = errors.New("reference not found")

// ErrRepositoryAccess indicates repository data could not be accessed.
var ErrRepositoryAccess = errors.New("repository access error")

// ErrNotGitRepository indicates a path is not a git repository.
var ErrNotGitRepository = errors.New("not a git repository")
