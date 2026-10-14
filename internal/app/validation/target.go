// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package validation

import "strconv"

const (
	// TargetTypeCommit identifies single-commit validation.
	TargetTypeCommit = "commit"
	// TargetTypeCommits identifies validation of an explicit commit list.
	TargetTypeCommits = "commits"
	// TargetTypeCount identifies HEAD-count validation.
	TargetTypeCount = "count"
	// TargetTypeMessage identifies message-file validation.
	TargetTypeMessage = "message"
	// TargetTypeRange identifies commit-range validation.
	TargetTypeRange = "range"
	refHead         = "HEAD"
)

// Target describes the validation scope selected by the caller.
type Target struct {
	Type   string
	Source string
	Target string
	Count  int
	Refs   []string
}

// MessageTarget returns a target for a commit message file.
func MessageTarget(source string) Target {
	return Target{Type: TargetTypeMessage, Source: source}
}

// BaseBranchTarget returns a range target from baseBranch to HEAD.
func BaseBranchTarget(baseBranch string) Target {
	return Target{Type: TargetTypeRange, Source: baseBranch, Target: refHead}
}

// RangeTarget returns a target for an explicit commit range.
func RangeTarget(from, to string) Target {
	return Target{Type: TargetTypeRange, Source: from, Target: to}
}

// CommitTarget returns a target for one commit reference.
func CommitTarget(ref string) Target {
	return Target{Type: TargetTypeCommit, Source: ref}
}

// CommitsTarget returns a target for an explicit ordered list of commit references.
func CommitsTarget(refs []string) Target {
	return Target{Type: TargetTypeCommits, Refs: append([]string(nil), refs...)}
}

// CountTarget returns a target for the latest count commits.
func CountTarget(count int) Target {
	return Target{Type: TargetTypeCount, Source: strconv.Itoa(count), Count: count}
}

// NewCountTarget returns a validated target for the latest count commits.
func NewCountTarget(count int) (Target, error) {
	if err := ValidateCommitCount(count); err != nil {
		return Target{}, err
	}

	return CountTarget(count), nil
}

// ValidateCommitCount checks whether count can select commits from HEAD.
func ValidateCommitCount(count int) error {
	if count <= 0 {
		return ErrCommitCountNotPositive
	}

	return nil
}

// DefaultTarget returns the default HEAD target.
func DefaultTarget() Target {
	return CommitTarget(refHead)
}

// IsMessageFile reports whether the target reads a commit message file.
func (t Target) IsMessageFile() bool {
	return t.Type == TargetTypeMessage
}

// IsCommit reports whether the target validates one commit.
func (t Target) IsCommit() bool {
	return t.Type == TargetTypeCommit
}

// IsRange reports whether the target validates a commit range.
func (t Target) IsRange() bool {
	return t.Type == TargetTypeRange
}

// IsCount reports whether the target validates a commit count.
func (t Target) IsCount() bool {
	return t.Type == TargetTypeCount
}
