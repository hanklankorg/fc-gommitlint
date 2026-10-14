// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package diagnostics

// ContextKey identifies structured diagnostic context fields.
type ContextKey = string

// Context contains structured metadata for a validation diagnostic.
type Context = map[string]string

// Common diagnostic context keys.
const (
	ContextActual                 ContextKey = "actual"
	ContextExpected               ContextKey = "expected"
	ContextExpectedMax            ContextKey = "expected_max"
	ContextSubject                ContextKey = "subject"
	ContextScope                  ContextKey = "scope"
	ContextWord                   ContextKey = "word"
	ContextSuggestion             ContextKey = "suggestion"
	ContextLocale                 ContextKey = "locale"
	ContextLine                   ContextKey = "line"
	ContextReason                 ContextKey = "reason"
	ContextType                   ContextKey = "type"
	ContextOver                   ContextKey = "over"
	ContextRef                    ContextKey = "ref"
	ContextMin                    ContextKey = "min"
	ContextMax                    ContextKey = "max"
	ContextPattern                ContextKey = "pattern"
	ContextInvalidEntry           ContextKey = "invalid_entry"
	ContextRequireGPG             ContextKey = "require_gpg"
	ContextRequiresImperativeHint ContextKey = "requires_imperative_hint"
	ContextKeysFound              ContextKey = "keys_found"
	ContextRejected               ContextKey = "rejected"
	ContextAuthorEmail            ContextKey = "author_email"
	ContextCommitHash             ContextKey = "commit_hash"
	ContextError                  ContextKey = "error"
	ContextInvalidSuffixes        ContextKey = "invalid_suffixes"
)
