// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import "errors"

// Key directory errors.
var (
	ErrKeyDirectoryNotSpecified  = errors.New("key directory not specified")
	ErrKeyDirectoryNotAccessible = errors.New("key directory inaccessible")
	ErrKeyDirectoryNotDirectory  = errors.New("key directory path is not a directory")
	ErrKeyFileOutsideDirectory   = errors.New("key file path outside configured directory")
)
