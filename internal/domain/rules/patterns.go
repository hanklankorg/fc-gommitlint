// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import "regexp"

// Shared regex patterns for commit message validation.
// These patterns are compiled once at package initialization for performance.

// SignOffPattern validates DCO (Developer Certificate of Origin) sign-off format.
// Enforces proper "Name <email@domain>" format as required by DCO specification.
//
// Pattern breakdown:
//
//	^Signed-off-by:  - Required trailer prefix
//	\s+              - Required whitespace after colon
//	.+               - Name (any characters)
//	\s+              - Required space before email
//	<.+@.+>$         - Email in angle brackets
//
// Valid examples:
//
//	✓ "Signed-off-by: John Doe <john@example.com>"
//	✓ "Signed-off-by: Jane Smith <j.smith@company.org>"
//	✗ "Signed-off-by:John<john@example.com>" (missing spaces)
//	✗ "Signed-off-by: John Doe john@example.com" (missing angle brackets)
var SignOffPattern = regexp.MustCompile(`^Signed-off-by:\s+.+\s+<.+@.+>$`)

// IdentityTrailerPattern matches a trailer that names one person and nothing
// else: a known key, a name, and one address in angle brackets ending the
// line. It is stricter than SignOffPattern on purpose: text after the address,
// or a second pair of angle brackets, makes the line ordinary body text.
//
//	✓ "Signed-off-by: Jane Smith <j.smith@company.org>"
//	✓ "Co-authored-by: John Doe <john@example.com>"
//	✗ "Reviewed-by: Bob <bob@example.com> and some prose"
//	✗ "Signed-off-by: Bob <bob@example.com> <bob@example.org>"
var IdentityTrailerPattern = regexp.MustCompile(
	`^(Signed-off-by|Co-authored-by|Reviewed-by|Acked-by|Tested-by|Reported-by):\s+[^<>]+\s<[^<>\s]+@[^<>\s]+>$`)

// EmailPattern extracts email addresses from angle brackets.
var EmailPattern = regexp.MustCompile(`<([^>]+)>`)

// TrailerPattern matches common Git trailer formats including:
// - Sign-off trailers: Signed-off-by, Co-authored-by, Reviewed-by, etc.
// - Reference trailers: Fixes, Closes, Resolves with issue numbers or URLs.
var TrailerPattern = regexp.MustCompile(`^(Signed-off-by|Co-authored-by|Reviewed-by|Acked-by|Tested-by|Reported-by):\s+.+\s+<.+@.+>$|^(Fixes|Closes|Resolves):\s*(#\d+|https?://[^\s]+).*$`)
