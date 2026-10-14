// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import "strings"

// CommitRange describes a supported two-dot Git commit range.
type CommitRange struct {
	From string
	To   string
}

// ParseCommitRange parses the supported two-dot Git range syntax.
func ParseCommitRange(raw string) (CommitRange, error) {
	commitRange := strings.TrimSpace(raw)
	if commitRange == "" {
		return CommitRange{}, ErrCommitRangeEmpty
	}

	if strings.Contains(commitRange, "...") {
		return CommitRange{}, ErrCommitRangeFormat
	}

	parts := strings.Split(commitRange, "..")
	switch len(parts) {
	case 1:
		return CommitRange{}, ErrCommitRangeFormat
	case 2:
		return CommitRange{From: strings.TrimSpace(parts[0]), To: strings.TrimSpace(parts[1])}, nil
	default:
		return CommitRange{}, ErrCommitRangeParts
	}
}
