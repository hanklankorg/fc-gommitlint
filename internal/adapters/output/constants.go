// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

// Table formatting constants.
const (
	contextActualValue     = "actual_value"
	contextCharPosition    = "char_position"
	contextColumnNumber    = "column_number"
	contextExpectedPattern = "expected_pattern"
	contextExpectedWord    = "expected_word"
	contextFirstWord       = "first_word"
	contextLineNumber      = "line_number"
	// contextValidFormats is set by the identity rule next to invalid_entry.
	contextValidFormats = "valid_formats"

	messageExpectedLabel             = "Expected"
	messageMissingSignoff            = "Missing sign-off"
	messageRemoveTrailingPunctuation = "Remove the period or exclamation mark at the end"
	messageShortenDescription        = "Shorten the description in your commit message"
	messageStartLowercase            = "Start commit message with lowercase letter"
	messageStartWithActionVerb       = "Start with an action verb"
	messageStartWithWord             = "Start with a word (not a symbol or number)"
	messageSubjectIs                 = "Subject is"
	messageUnknownError              = "Unknown error"
	messageUseAllowedSignatureType   = "Use an allowed signature type"
	messageUseBaseForm               = "Use base form of verb (like 'add' not 'adding')"
	messageUseCorrectSignatureFormat = "Use the correct signature format"
	messageUsePresentTense           = "Use present tense (like 'add' not 'added')"
	messageUseTrustedSigningKey      = "Use a trusted signing key"
	messageUseValidScope             = "Use a valid scope or remove the scope"
	messageWriteCommitMessage        = "Write a commit message"
	messageWrongCase                 = "Wrong case"
	outputHelpHeader                 = "Help:"
	textPlainStatusFail              = "FAIL"
	textPlainStatusPass              = "PASS"

	// TableTotalWidth is the total width of the validation table.
	TableTotalWidth = 100
	// TableInternalWidth is the internal width (total minus borders).
	TableInternalWidth = 98
	// TableBorderChar is the character used for table borders.
	TableBorderChar = "─"

	// ColTypeWidth is the width of the type column.
	ColTypeWidth = 8
	// ColTargetWidth is the width of the target column.
	ColTargetWidth = 14
	// ColRuleWidth is the width of the rule column.
	ColRuleWidth = 18
	// ColStatusWidth is the width of the status column.
	ColStatusWidth = 6
	// ColDetailsWidth is the width of the details column.
	ColDetailsWidth = 38

	// TablePadding is the total padding in the table.
	TablePadding = 10
	// TableSeparators is the total width of column separators.
	TableSeparators = 4

	// CommitBodyMaxWidth is the maximum width for commit body lines (Git convention).
	CommitBodyMaxWidth = 72
)
