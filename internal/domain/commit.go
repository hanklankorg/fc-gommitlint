// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"context"
	"strings"
)

// Commit for validation.
type Commit struct {
	// Hash is the Git commit SHA.
	Hash string

	// Subject is the first line of the commit message.
	Subject string

	// Body is the rest of the commit message after the subject.
	Body string

	// Message is the complete commit message including subject and body.
	Message string

	// Author is the name of the commit author.
	Author string

	// AuthorEmail is the email address of the commit author.
	AuthorEmail string

	// CommitDate is the date of the commit in ISO format.
	CommitDate string

	// Signature is the signature attached to the commit, if any.
	Signature string

	// SignaturePayload is the raw commit payload used for signature verification.
	// This is the commit content without the signature header.
	SignaturePayload []byte

	// IsMergeCommit indicates whether this is a merge commit.
	IsMergeCommit bool
}

// NewCommit creates a Commit from its components.
func NewCommit(hash, message, author, authorEmail, commitDate, signature string, isMerge bool) Commit {
	subject, body := SplitCommitMessage(message)

	return Commit{
		Hash:             hash,
		Message:          message,
		Subject:          subject,
		Body:             body,
		Author:           author,
		AuthorEmail:      authorEmail,
		CommitDate:       commitDate,
		Signature:        signature,
		SignaturePayload: nil,
		IsMergeCommit:    isMerge,
	}
}

// NewCommitWithSignaturePayload creates a Commit with an explicit signature payload.
func NewCommitWithSignaturePayload(hash, message, author, authorEmail, commitDate, signature string, isMerge bool, payload []byte) Commit {
	commit := NewCommit(hash, message, author, authorEmail, commitDate, signature, isMerge)
	commit.SignaturePayload = payload

	return commit
}

// ParseCommitMessage creates a Commit from a message string.
func ParseCommitMessage(message string) Commit {
	return NewCommit("", message, "", "", "", "", false)
}

// HasBody reports whether the commit has a body.
func (c Commit) HasBody() bool {
	return strings.TrimSpace(c.Body) != ""
}

// IsValid reports whether the commit has basic required fields.
func (c Commit) IsValid() bool {
	return c.Hash != "" && strings.TrimSpace(c.Subject) != ""
}

// IsSigned reports whether the commit has a signature.
func (c Commit) IsSigned() bool {
	return c.Signature != ""
}

// getFixupPrefixes returns the prefixes that indicate temporary commits to be squashed.
// Includes "amend! " from `git commit --fixup=amend:<sha>` (since Git 2.32).
func getFixupPrefixes() []string {
	return []string{"fixup! ", "squash! ", "amend! "}
}

// IsFixupCommit reports whether the commit is a fixup, squash, or amend commit.
// These are temporary commits created by `git commit --fixup`, `--squash`, or
// `--fixup=amend:<sha>` that are meant to be resolved during interactive rebase.
func (c Commit) IsFixupCommit() bool {
	for _, prefix := range getFixupPrefixes() {
		if strings.HasPrefix(c.Subject, prefix) {
			return true
		}
	}

	return false
}

// FixupCommitType returns the type of fixup commit ("fixup", "squash", or "amend") or empty string.
func (c Commit) FixupCommitType() string {
	for _, prefix := range getFixupPrefixes() {
		if strings.HasPrefix(c.Subject, prefix) {
			// Return type without "! " suffix
			return strings.TrimSuffix(prefix, "! ")
		}
	}

	return ""
}

// SplitCommitMessage splits a commit message into subject and body.
func SplitCommitMessage(message string) (string, string) {
	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return "", ""
	}

	subject := strings.TrimSpace(lines[0])

	if len(lines) >= 3 && strings.TrimSpace(lines[1]) == "" {
		bodyLines := lines[2:]
		body := strings.TrimSpace(strings.Join(bodyLines, "\n"))

		return subject, body
	}

	return subject, ""
}

// CommitReader defines the contract for reading commits from a repository.
// It retrieves commits with consistent ordering semantics:
// - GetHeadCommits returns newest-first, up to count.
// - GetCommitRange returns oldest-first for two-dot ranges (from..to).
// - Empty ranges return an empty slice without error.
type CommitReader interface {
	// GetCommit retrieves a single commit by reference.
	GetCommit(ctx context.Context, ref string) (Commit, error)

	// GetCommitRange retrieves commits in a range.
	GetCommitRange(ctx context.Context, from, to string) ([]Commit, error)

	// GetHeadCommits retrieves N commits from HEAD.
	GetHeadCommits(ctx context.Context, count int) ([]Commit, error)
}

// BranchAnalyzer analyzes branch relationships.
type BranchAnalyzer interface {
	// GetCommitsAheadCount returns how many commits the current branch is ahead of the reference.
	GetCommitsAheadCount(ctx context.Context, referenceBranch string) (int, error)
	// GetCurrentBranch returns the name of the current branch.
	GetCurrentBranch(ctx context.Context) (string, error)
	// BranchExists checks if a branch reference exists (local or remote).
	BranchExists(ctx context.Context, branchName string) (bool, error)
}

// Repository combines all repository capabilities.
type Repository interface {
	CommitReader
	BranchAnalyzer
}

// ValidationResult for a single commit.
type ValidationResult struct {
	Commit Commit
	Errors []ValidationError
}

// HasFailures returns true if there are any validation failures.
func (v ValidationResult) HasFailures() bool {
	return len(v.Errors) > 0
}
