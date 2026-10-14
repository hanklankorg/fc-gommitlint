// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// diagnosticMessage renders a diagnostic as one terminal-safe display line for
// the human-facing formatters; messages can quote untrusted commit text.
func diagnosticMessage(err domain.ValidationError, localizer Localizer) string {
	return cleanForTerminal(plainDiagnosticMessage(err, localizer))
}

// plainDiagnosticMessage is diagnosticMessage without terminal escaping, for
// the plain CSV format, which applies its own field encoding.
func plainDiagnosticMessage(err domain.ValidationError, localizer Localizer) string {
	message := convertToUserFriendlyMessage(err, localizer)
	if err.Diagnostic.ID == "" {
		return message
	}

	return fmt.Sprintf("[%s] %s", err.Diagnostic.ID, message)
}

// PublicValidationMessage returns the terminal-safe public text for a
// ValidationError. It is shared by report formatters and the pre-report process
// boundary so the latter never has to expose an arbitrary wrapped cause.
func PublicValidationMessage(err domain.ValidationError, localizer Localizer) string {
	return cleanForTerminal(convertToUserFriendlyMessage(err, localizer))
}
