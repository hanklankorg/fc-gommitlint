// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func managedSourceIntegrityMatches(expected string, content []byte) (string, bool) {
	actual := managedSourceIntegritySHA256(content)

	return actual, strings.EqualFold(actual, expected)
}

func managedSourceIntegritySHA256(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}
