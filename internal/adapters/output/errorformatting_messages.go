// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Message conversion functions that convert errors to user-friendly messages

// convertToUserFriendlyMessage converts technical error messages to user-friendly language using declarative mapping.
func convertToUserFriendlyMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if formatter, exists := errorFormatterRegistry()[err.Diagnostic.Reason]; exists {
		return formatter(err, localizer)
	}

	if err.Message != "" {
		return err.Message
	}

	return localizer.Get("Validation failed: %s", err.Diagnostic.Reason)
}

type errorFormatter func(domain.ValidationError, Localizer) string

func errorFormatterRegistry() map[diagnostics.Reason]errorFormatter {
	return map[diagnostics.Reason]errorFormatter{
		// Subject line errors
		diagnostics.ReasonEmptySubject: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Subject is empty")
		},
		diagnostics.ReasonMissingSubject: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Commit message has no subject")
		},
		diagnostics.ReasonWrongCaseUpper:    formatWrongCaseUpperMessage,
		diagnostics.ReasonWrongCaseLower:    formatWrongCaseLowerMessage,
		diagnostics.ReasonWrongCaseSentence: formatWrongCaseUpperMessage, // sentence = capital first letter, same wording
		diagnostics.ReasonSubjectSuffix:     formatSubjectSuffixMessage,
		diagnostics.ReasonSubjectTooLong:    formatSubjectTooLongMessage,
		diagnostics.ReasonSubjectCase:       formatInvalidCaseMessage,

		// Imperative mood errors
		diagnostics.ReasonNoFirstWord: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Subject does not start with a word")
		},
		diagnostics.ReasonNonVerb:     formatNonVerbMessage,
		diagnostics.ReasonPastTense:   formatPastTenseMessage,
		diagnostics.ReasonGerund:      formatGerundMessage,
		diagnostics.ReasonThirdPerson: formatThirdPersonMessage,

		// Conventional commit errors
		diagnostics.ReasonInvalidConventionalType: formatInvalidConventionalTypeMessage,
		diagnostics.ReasonEmptyConventionalDesc: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Conventional commit description is empty")
		},
		diagnostics.ReasonConventionalDescTooLong: formatDescriptionTooLongMessage,
		diagnostics.ReasonDescriptionTooLong:      formatDescriptionTooLongMessage,
		diagnostics.ReasonMissingConventionalSubject: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Subject does not follow the conventional commit format")
		},
		diagnostics.ReasonInvalidFormat:             formatInvalidConventionalFormatMessage,
		diagnostics.ReasonInvalidConventionalFormat: formatInvalidConventionalFormatMessage,
		diagnostics.ReasonMissingConventionalScope: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Type is missing a required scope")
		},
		diagnostics.ReasonInvalidConventionalScope: formatInvalidScopeMessage,
		diagnostics.ReasonInvalidSpacing: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Missing single space after the type colon")
		},
		diagnostics.ReasonBreakingChangeNotAllowed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Breaking change marker (!) is not allowed")
		},
		diagnostics.ReasonInvalidMultiScope: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Scope list is malformed (expected comma-separated without spaces)")
		},
		diagnostics.ReasonMultiScopeNotAllowed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Multiple scopes are not allowed")
		},

		// Metadata and sign-off errors
		diagnostics.ReasonMissingSignoff: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("No Signed-off-by trailer found")
		},
		diagnostics.ReasonInvalidSignoffFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Sign-off is not in 'Signed-off-by: Name <email>' format")
		},
		diagnostics.ReasonMisplacedSignoff: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Content appears after sign-off lines")
		},
		diagnostics.ReasonInsufficientSignoffs: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Too few distinct sign-offs")
		},

		// Commit body errors
		diagnostics.ReasonBodyLineTooLong: formatBodyLineTooLongMessage,
		diagnostics.ReasonMissingBody: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Body is required but missing")
		},
		diagnostics.ReasonMissingBlankLine: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("No blank line between subject and body")
		},
		diagnostics.ReasonBodyTooShort: formatBodyTooShortMessage,
		diagnostics.ReasonInvalidBody:  formatInvalidBodyMessage,
		diagnostics.ReasonInvalidStructure: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Message is not structured as subject, blank line, body")
		},

		// JIRA/Issue reference errors
		diagnostics.ReasonMissingJira: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("No JIRA reference found")
		},
		diagnostics.ReasonMissingJiraKeySubject: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Subject has no JIRA reference")
		},
		diagnostics.ReasonMissingJiraKeyBody: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Body has no JIRA reference")
		},
		diagnostics.ReasonInvalidJiraFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("JIRA reference is not in the PROJ-123 format")
		},
		diagnostics.ReasonJiraKeyNotAtEnd: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("JIRA key is not at an allowed position")
		},
		diagnostics.ReasonInvalidProject: formatInvalidProjectMessage,
		diagnostics.ReasonRefsAfterSignoff: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Refs: lines appear after sign-offs")
		},
		diagnostics.ReasonInvalidRefsFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Refs: trailer is not in 'Refs: PROJ-123' format")
		},
		diagnostics.ReasonInvalidKeyFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("JIRA key in Refs: line is not uppercase")
		},

		// GPG/SSH signature errors
		diagnostics.ReasonMissingSignature: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Commit is not cryptographically signed")
		},
		diagnostics.ReasonInvalidSignature: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Commit signature is invalid")
		},
		diagnostics.ReasonInvalidSignatureFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Commit signature is malformed")
		},
		diagnostics.ReasonUnknownSigFormat: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Signature format is not recognized")
		},
		diagnostics.ReasonSignatureTypeNotAllowed: formatSignatureTypeNotAllowedMessage,
		diagnostics.ReasonNoTrustedKeys: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("No eligible public key found in key directory")
		},
		diagnostics.ReasonKeyLoadFailed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Signing keys could not be loaded")
		},
		diagnostics.ReasonVerificationFailed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Signature verification failed")
		},
		diagnostics.ReasonIncompleteGPGSig: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("GPG signature is incomplete")
		},
		diagnostics.ReasonIncompleteSSHSig: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("SSH signature is incomplete")
		},
		diagnostics.ReasonInvalidKeyDirectory: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Signing key directory is not usable")
		},
		diagnostics.ReasonInvalidConfig: formatInvalidConfigMessage,

		// Spelling errors
		diagnostics.ReasonMisspelledWord: formatMisspelledWordMessage,
		diagnostics.ReasonSpelling:       formatMisspelledWordMessage,

		// Author/identity errors
		diagnostics.ReasonIdentityNotAllowed: formatIdentityNotAllowedMessage,

		// Branch and merge errors
		diagnostics.ReasonTooManyCommits: formatTooManyCommitsMessage,
		diagnostics.ReasonMergeCommitFound: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("History contains a merge commit")
		},
		diagnostics.ReasonFixupCommitFound: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("History contains a fixup commit")
		},

		// Encoding and format errors
		diagnostics.ReasonInvalidUTF8: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Message contains invalid UTF-8")
		},

		// Security errors
		diagnostics.ReasonWeakKey: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Signing key is weaker than the required minimum")
		},

		// System/operational errors
		diagnostics.ReasonInvalidRepo: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Check that you are in a valid Git repository")
		},
		diagnostics.ReasonCancelled: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Operation cancelled")
		},
		diagnostics.ReasonContextCancelled: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Operation cancelled by user or timeout")
		},
		diagnostics.ReasonInvalidReference: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Use a valid commit hash, branch name, or tag")
		},
		diagnostics.ReasonInvalidCommitRange: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Use format: <from>..<to> or <from>...<to>")
		},
		diagnostics.ReasonMissingReference: formatMissingReferenceMessage,
		diagnostics.ReasonGitOperationFailed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Git operation failed — check repository state")
		},
		diagnostics.ReasonConfigNotFound: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Check your configuration file path")
		},
		diagnostics.ReasonMessageFileMissing: func(err domain.ValidationError, localizer Localizer) string {
			// err.Message already names the path; the registry default would
			// drop it, and "which file?" is the whole question here.
			if err.Message != "" {
				return err.Message
			}

			return localizer.Get("Message file does not exist")
		},
		diagnostics.ReasonRepoRuleNoRepo: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Run this command inside a Git repository or use --ignore-repo-rules")
		},
		diagnostics.ReasonValidationFailed: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Validation failed")
		},

		// Other errors
		diagnostics.ReasonNonImperative: func(_ domain.ValidationError, localizer Localizer) string {
			return localizer.Get("Subject is not in imperative mood")
		},
	}
}

func formatSubjectSuffixMessage(err domain.ValidationError, localizer Localizer) string {
	if actual := err.Context[diagnostics.ContextActual]; actual != "" {
		return localizer.Get("Subject ends with '%s'", actual)
	}

	return localizer.Get("Subject ends with forbidden punctuation")
}

func formatPastTenseMessage(err domain.ValidationError, localizer Localizer) string {
	suggestion := err.Context[diagnostics.ContextSuggestion]
	actual := err.Context[diagnostics.ContextActual]

	if suggestion != "" && actual != "" {
		return localizer.Get("'%s' is past tense (use '%s')", actual, suggestion)
	}

	return localizer.Get("First word is past tense")
}

func formatGerundMessage(err domain.ValidationError, localizer Localizer) string {
	suggestion := err.Context[diagnostics.ContextSuggestion]
	actual := err.Context[diagnostics.ContextActual]

	if suggestion != "" && actual != "" {
		return localizer.Get("'%s' is a gerund (use '%s')", actual, suggestion)
	}

	return localizer.Get("First word is a gerund (-ing form)")
}

func formatThirdPersonMessage(err domain.ValidationError, localizer Localizer) string {
	suggestion := err.Context[diagnostics.ContextSuggestion]
	actual := err.Context[diagnostics.ContextActual]

	if suggestion != "" && actual != "" {
		return localizer.Get("'%s' is third person (use '%s')", actual, suggestion)
	}

	return localizer.Get("First word is third person")
}

func formatInvalidBodyMessage(err domain.ValidationError, localizer Localizer) string {
	switch err.Context[diagnostics.ContextActual] {
	case "starts with sign-off":
		return localizer.Get("Body starts with a sign-off line")
	case "only sign-off lines":
		return localizer.Get("Body contains only sign-off lines")
	default:
		return localizer.Get("Body has no descriptive content")
	}
}

func formatSignatureTypeNotAllowedMessage(err domain.ValidationError, localizer Localizer) string {
	if expected := err.Context[diagnostics.ContextExpected]; expected != "" {
		return localizer.Get("Commit is not signed with %s", expected)
	}

	return localizer.Get("Signature type is not allowed")
}

func formatIdentityNotAllowedMessage(err domain.ValidationError, localizer Localizer) string {
	// The identity rule reports the offending address as author_email.
	author := err.Context[diagnostics.ContextAuthorEmail]
	if author == "" {
		author = err.Context[diagnostics.ContextActual]
	}

	if author != "" {
		return localizer.Get("Author '%s' is not in the allowed list", author)
	}

	return localizer.Get("Author is not in the allowed list")
}

func formatDescriptionTooLongMessage(err domain.ValidationError, localizer Localizer) string {
	if maxLength, ok := maxFromContext(err); ok {
		return localizer.Get("Description is longer than %d characters", maxLength)
	}

	return localizer.Get("Description is too long")
}

// formatBodyLineTooLongMessage states which body line is too wide. The line
// number is what turns N identical-looking violations into N distinct,
// actionable findings.
func formatBodyLineTooLongMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	line := err.Context[diagnostics.ContextLine]
	actual := err.Context[diagnostics.ContextActual]
	maxLen := err.Context[diagnostics.ContextMax]

	if line != "" && actual != "" && maxLen != "" {
		return localizer.Get("Line %s is %s characters (max %s)", line, actual, maxLen)
	}

	return localizer.Get("A body line is longer than the configured width")
}

func formatBodyTooShortMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	actual := err.Context[diagnostics.ContextActual]
	minLen := err.Context[diagnostics.ContextMin]

	if actual != "" && minLen != "" {
		return localizer.Get("Body is %s characters (min %s)", actual, minLen)
	}

	return localizer.Get("Body is shorter than the configured minimum")
}

// formatSubjectTooLongMessage formats subject too long errors.
func formatSubjectTooLongMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	actualStr, exists := err.Context[diagnostics.ContextActual]
	if !exists {
		return localizer.Get("Your commit message is too long")
	}

	actual, parseErr := strconv.Atoi(actualStr)
	if parseErr != nil {
		return localizer.Get("Your commit message is too long")
	}

	maxLength, ok := maxFromContext(err)
	if !ok {
		return localizer.Get("Your commit message is too long")
	}

	over := actual - maxLength
	if over == 1 {
		return localizer.Get("Your commit message is 1 character too long")
	}

	return localizer.Get("Your commit message is %d characters too long", over)
}

// formatWrongCaseUpperMessage renders diagnostics.ReasonWrongCaseUpper / diagnostics.ReasonWrongCaseSentence
// (both want a capital first letter; the message wording is identical).
func formatWrongCaseUpperMessage(err domain.ValidationError, localizer Localizer) string {
	if expectedWord := err.Context[contextExpectedWord]; expectedWord != "" {
		return localizer.Get("First word should be '%s'", expectedWord)
	}

	return localizer.Get("First word is not capitalized")
}

// formatWrongCaseLowerMessage renders diagnostics.ReasonWrongCaseLower, appending an
// imperative-mood reminder when subject.go's generateCaseError set
// requires_imperative_hint (i.e. case=lower AND require_imperative is on).
func formatWrongCaseLowerMessage(err domain.ValidationError, localizer Localizer) string {
	expectedWord := err.Context[contextExpectedWord]
	hint := err.Context[diagnostics.ContextRequiresImperativeHint] == "true"

	switch {
	case expectedWord != "" && hint:
		return localizer.Get(
			"First word should be '%s' (lowercase, imperative verb)",
			expectedWord,
		)
	case expectedWord != "":
		return localizer.Get("First word should be '%s'", expectedWord)
	case hint:
		return localizer.Get("First word is not a lowercase imperative verb")
	default:
		return localizer.Get("First word is not lowercase")
	}
}

// formatInvalidCaseMessage formats the generic diagnostics.ReasonSubjectCase fallback.
//
// When subject.go's generateCaseError set requires_imperative_hint (case
// is `lower` AND require_imperative is on), state that the first word must
// also be imperative. Saves the user a second guess on what the fix is
// when the offending first word is also a noun, not just a capitalisation
// mistake.
func formatInvalidCaseMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	expectedWord := err.Context[contextExpectedWord]
	hint := err.Context[diagnostics.ContextRequiresImperativeHint] == "true"

	switch {
	case expectedWord != "" && hint:
		return localizer.Get(
			"First word should be '%s' (imperative verb)",
			expectedWord,
		)
	case expectedWord != "":
		return localizer.Get("First word should be '%s'", expectedWord)
	case hint:
		return localizer.Get("First word is not an imperative verb in the configured case")
	default:
		return localizer.Get("First word does not match the configured case")
	}
}

// truncateToRunes truncates a string to maxRunes runes.
// UTF-8 safe, never splits multi-byte characters.
func truncateToRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}

	return string(runes[:maxRunes])
}

func maxFromContext(err domain.ValidationError) (int, bool) {
	expected, exists := err.Context[diagnostics.ContextExpectedMax]
	if !exists {
		return 0, false
	}

	maxValue, parseErr := strconv.Atoi(strings.TrimSpace(expected))
	if parseErr != nil {
		return 0, false
	}

	return maxValue, true
}

// formatInvalidConventionalTypeMessage formats invalid conventional type errors.
//
// When the rule provides both the offending value (diagnostics.ContextActual) and the
// allowed-types list (diagnostics.ContextExpected), surface both. Saying "'X' is not a
// valid type" without listing alternatives forces users to grep docs; listing
// types without naming the offender hides what they actually wrote.
func formatInvalidConventionalTypeMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	actual, hasActual := err.Context[diagnostics.ContextActual]
	expected, hasExpected := err.Context[diagnostics.ContextExpected]

	if !hasExpected {
		expected = strings.Join(settings.GetDefaultConventionalTypes(), ", ")
	}

	if hasActual {
		return localizer.Get("'%s' is not a valid commit type. Use one of: %s", actual, expected)
	}

	return localizer.Get("Use a valid commit type like: %s", expected)
}

// formatInvalidProjectMessage formats invalid project errors.
func formatInvalidProjectMessage(err domain.ValidationError, localizer Localizer) string {
	actual := err.Context[diagnostics.ContextActual]
	expected := err.Context[diagnostics.ContextExpected]

	localizer = ensureLocalizer(localizer)

	if actual != "" && expected != "" {
		if strings.Contains(expected, ",") {
			return localizer.Get("'%s' is not valid — use one of: %s", actual, expected)
		}

		return localizer.Get("Use '%s' instead of '%s'", expected, actual)
	}

	return localizer.Get("Use a valid project key")
}

// formatInvalidConventionalFormatMessage formats invalid conventional format errors.
func formatInvalidConventionalFormatMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	// diagnostics.ContextActual contains the full subject when the conventional rule fires
	if actual, exists := err.Context[diagnostics.ContextActual]; exists {
		if utf8.RuneCountInString(actual) > 30 {
			actual = truncateToRunes(actual, 30) + "..."
		}

		return localizer.Get("'%s' does not follow conventional format", actual)
	}

	if subject, exists := err.Context[diagnostics.ContextSubject]; exists && utf8.RuneCountInString(subject) > 20 {
		preview := truncateToRunes(subject, 20) + "..."

		return localizer.Get("'%s' does not follow conventional format", preview)
	}

	return localizer.Get("Use format: %s", domain.ConventionalCommitFormatTemplate)
}

// formatInvalidConfigMessage formats invalid config errors from any rule.
func formatInvalidConfigMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	// Detect the source rule from context to provide relevant guidance
	if _, ok := err.Context[diagnostics.ContextRequireGPG]; ok {
		return localizer.Get("Check your signature policy configuration")
	}

	if _, ok := err.Context[diagnostics.ContextPattern]; ok {
		return localizer.Get("Fix the invalid regex in your configuration")
	}

	if _, ok := err.Context[diagnostics.ContextInvalidEntry]; ok {
		return localizer.Get("Fix the invalid entry in your configuration")
	}

	// Generic fallback
	return localizer.Get("Check your rule configuration")
}

// formatNonVerbMessage formats non-verb imperative mood errors.
func formatNonVerbMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if actual := err.Context[diagnostics.ContextActual]; actual != "" {
		return localizer.Get("'%s' is not a verb", actual)
	}

	return localizer.Get("Subject does not start with a verb")
}

// formatInvalidScopeMessage formats invalid conventional scope errors.
func formatInvalidScopeMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if actual := err.Context[diagnostics.ContextActual]; actual != "" {
		return localizer.Get("'%s' is not a valid scope", actual)
	}

	return localizer.Get("Use a valid scope")
}

// formatTooManyCommitsMessage formats too many commits errors.
func formatTooManyCommitsMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if over := err.Context[diagnostics.ContextOver]; over != "" {
		return localizer.Get("Branch has %s commits over the limit", over)
	}

	return localizer.Get("Reduce the number of commits on this branch")
}

// formatMissingReferenceMessage formats missing reference errors.
func formatMissingReferenceMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	if ref := err.Context[diagnostics.ContextRef]; ref != "" {
		return localizer.Get("Reference branch '%s' not found", ref)
	}

	return localizer.Get("Specify a base branch with --base-branch")
}

// formatMisspelledWordMessage formats misspelled word errors.
func formatMisspelledWordMessage(err domain.ValidationError, localizer Localizer) string {
	localizer = ensureLocalizer(localizer)

	word, exists := err.Context[diagnostics.ContextWord]
	if !exists {
		return localizer.Get("Fix spelling in the commit message")
	}

	return localizer.Get("'%s' appears to be misspelled", word)
}
