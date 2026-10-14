// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import "codefloe.com/itiquette/gommitlint/internal/domain"

// WarningMessages returns the message strings for warnings.
func WarningMessages(warnings []domain.Warning) []string {
	messages := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		messages = append(messages, warning.Message)
	}

	return messages
}
