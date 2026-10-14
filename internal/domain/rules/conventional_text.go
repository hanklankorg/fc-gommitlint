// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func conventionalValidationText(subject string, isConventional bool, allowedTypes []string) (string, domain.ConventionalCommitFormat, error) {
	if !isConventional {
		return subject, domain.ConventionalCommitFormat{}, nil
	}

	parsed := domain.ParseConventionalCommit(subject)
	if parsed.IsValid {
		description := strings.TrimSpace(parsed.Description)
		if description == "" {
			return "", parsed, ErrMissingSubjectAfterType
		}

		return description, parsed, nil
	}

	if domain.IsConventionalCommitLike(subject, allowedTypes) {
		return "", parsed, ErrInvalidConventionalCommitFmt
	}

	return subject, parsed, nil
}
