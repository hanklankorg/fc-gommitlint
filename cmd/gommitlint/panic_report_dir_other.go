// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd

package main

import "errors"

func writePrivatePanicReport(_, _ string, _ []byte) (string, error) {
	return "", errors.New("secure panic reports are unsupported on this platform")
}
