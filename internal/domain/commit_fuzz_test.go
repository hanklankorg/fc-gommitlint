// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzSplitCommitMessage tests commit message splitting with random inputs.
// It verifies the function never panics, subject is the first line, body follows Git conventions,
// and valid UTF-8 input produces valid UTF-8 output.
func FuzzSplitCommitMessage(f *testing.F) {
	// Seed corpus with representative examples.
	addSplitCommitMessageSeeds(f)

	f.Fuzz(func(t *testing.T, message string) {
		// Should never panic.
		subject, body := SplitCommitMessage(message)

		// Verify results.
		verifySplitCommitUTF8(t, message, subject, body)
		verifySplitCommitTrimming(t, subject, body)
		verifySplitCommitSubject(t, message, subject)
	})
}

//nolint:thelper // Fuzz seed helpers don't need f.Helper().
func addSplitCommitMessageSeeds(f *testing.F) {
	seeds := []string{
		"Add new feature",
		"Fix bug in parser",
		"Add new feature\n\nThis is the body of the commit message.",
		"Fix bug\n\nDetailed explanation here.\nWith multiple lines.",
		"",
		" ",
		"\n",
		"\n\n",
		"Subject\n",
		"Subject\n\n",
		"Subject\nNo blank line body",
		"Subject\n\nBody",
		"  Subject with leading space  ",
		strings.Repeat("a", 1000),
		"Fix: handle émojis 🎉",
		"日本語のコミットメッセージ\n\n詳細な説明",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}
}

func verifySplitCommitUTF8(t *testing.T, message, subject, body string) {
	t.Helper()

	if !utf8.ValidString(message) {
		return
	}

	if !utf8.ValidString(subject) {
		t.Errorf("Subject contains invalid UTF-8: %q", subject)
	}

	if !utf8.ValidString(body) {
		t.Errorf("Body contains invalid UTF-8: %q", body)
	}
}

func verifySplitCommitTrimming(t *testing.T, subject, body string) {
	t.Helper()

	if subject != strings.TrimSpace(subject) {
		t.Errorf("Subject has untrimmed whitespace: %q", subject)
	}

	if body != strings.TrimSpace(body) {
		t.Errorf("Body has untrimmed whitespace: %q", body)
	}
}

func verifySplitCommitSubject(t *testing.T, message, subject string) {
	t.Helper()

	lines := strings.Split(message, "\n")
	if len(lines) == 0 {
		return
	}

	expectedSubject := strings.TrimSpace(lines[0])
	if subject != expectedSubject {
		t.Errorf("Subject mismatch: got %q, want %q", subject, expectedSubject)
	}
}

// FuzzParseCommitMessage tests full commit message parsing.
func FuzzParseCommitMessage(f *testing.F) {
	seeds := []string{
		testSubjectFeatAddFeature,
		"fix: resolve bug\n\nDetailed description.",
		"",
		"Subject only",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, message string) {
		// Should never panic.
		commit := ParseCommitMessage(message)

		// Subject and Body should match SplitCommitMessage.
		expectedSubject, expectedBody := SplitCommitMessage(message)
		if commit.Subject != expectedSubject {
			t.Errorf("Subject mismatch: got %q, want %q", commit.Subject, expectedSubject)
		}

		if commit.Body != expectedBody {
			t.Errorf("Body mismatch: got %q, want %q", commit.Body, expectedBody)
		}

		// Message should be preserved.
		if commit.Message != message {
			t.Errorf("Message not preserved: got %q, want %q", commit.Message, message)
		}

		// HasBody should be consistent.
		hasBody := strings.TrimSpace(commit.Body) != ""
		if commit.HasBody() != hasBody {
			t.Errorf("HasBody() inconsistent: got %v, body is %q", commit.HasBody(), commit.Body)
		}
	})
}

// FuzzNewCommit tests commit creation with various inputs.
func FuzzNewCommit(f *testing.F) {
	f.Add(testHashABC123, testSubjectFeatAddFeature+"\n\nBody", "John Doe", "john@example.com", "2024-01-01", "", false)
	f.Add("", "", "", "", "", "", true)
	f.Add("hash", "Subject only", "Author", "email", "date", "signature", false)

	f.Fuzz(func(t *testing.T, hash, message, author, authorEmail, commitDate, signature string, isMerge bool) {
		// Should never panic.
		commit := NewCommit(hash, message, author, authorEmail, commitDate, signature, isMerge)

		// Verify fields.
		verifyCommitFields(t, commit, hash, message, author, authorEmail, commitDate, signature, isMerge)
	})
}

func verifyCommitFields(t *testing.T, commit Commit, hash, message, author, authorEmail, commitDate, signature string, isMerge bool) {
	t.Helper()

	if commit.Hash != hash {
		t.Errorf("Hash mismatch: got %q, want %q", commit.Hash, hash)
	}

	if commit.Author != author {
		t.Errorf("Author mismatch: got %q, want %q", commit.Author, author)
	}

	if commit.AuthorEmail != authorEmail {
		t.Errorf("AuthorEmail mismatch: got %q, want %q", commit.AuthorEmail, authorEmail)
	}

	if commit.CommitDate != commitDate {
		t.Errorf("CommitDate mismatch: got %q, want %q", commit.CommitDate, commitDate)
	}

	if commit.Signature != signature {
		t.Errorf("Signature mismatch: got %q, want %q", commit.Signature, signature)
	}

	if commit.IsMergeCommit != isMerge {
		t.Errorf("IsMergeCommit mismatch: got %v, want %v", commit.IsMergeCommit, isMerge)
	}

	if commit.Message != message {
		t.Errorf("Message mismatch: got %q, want %q", commit.Message, message)
	}

	if commit.IsSigned() != (signature != "") {
		t.Errorf("IsSigned() inconsistent: got %v, signature is %q", commit.IsSigned(), signature)
	}
}
