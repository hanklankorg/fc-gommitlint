// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// TestErrorFormatterRegistry_CoversAllReasons verifies that every domain reason
// has either a formatter entry or an explicit exemption. This prevents new error codes
// from silently falling through to the raw err.Message fallback.
func TestErrorFormatterRegistry_CoversAllReasons(t *testing.T) {
	t.Parallel()

	registry := errorFormatterRegistry()

	// Codes intentionally not in the registry:
	// Sentinel codes that are only used in errors.Is() checks or internal matching,
	// never created via domain.New() and shown to users.
	exemptReasons := map[diagnostics.Reason]string{
		diagnostics.ReasonNoKeyDir:            testSentinelSwitchReason,
		diagnostics.ReasonInvalidKeyDir:       testSentinelSwitchReason,
		diagnostics.ReasonEmptyMessage:        testSentinelSwitchReason,
		diagnostics.ReasonDisallowedSigType:   testSentinelSwitchReason,
		diagnostics.ReasonKeyNotTrusted:       testSentinelSwitchReason,
		diagnostics.ReasonInvalidCommit:       testSentinelSwitchReason,
		diagnostics.ReasonMissingKeyDirectory: testSentinelSwitchReason,
		diagnostics.ReasonSpellCheckFailed:    "internal: spell checker configuration issue",
		diagnostics.ReasonCommitNotFound:      "system error: handled by CLI error handling, not formatter",
		diagnostics.ReasonRangeNotFound:       "system error: handled by CLI error handling, not formatter",
		diagnostics.ReasonUnknown:             "generic fallback code, not a specific user error",
	}

	for _, reason := range diagnostics.AllValidationReasons() {
		if note, exempt := exemptReasons[reason]; exempt {
			_ = note

			continue
		}

		_, exists := registry[reason]
		check.True(t, exists,
			"reason %q has no entry in errorFormatterRegistry(). "+
				"Add a formatter or add to exemptReasons with a note.", reason)
	}
}
