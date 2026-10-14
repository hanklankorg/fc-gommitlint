// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// testErrorCode is a sentinel used for test-specific formatting paths.
const testErrorCode diagnostics.Reason = "test_error"

// unknownCodeConstant is used as a fallback label when no code is available.
const unknownCodeConstant = "unknown_code"

// FormatValidationError formats a validation error for text output.
func FormatValidationError(err domain.ValidationError, options TextOptions) string {
	var builder strings.Builder

	if options.Colors.Error == nil {
		options.Colors = GetColorScheme(options.UseColor, options.Theme)
	}

	options.Localizer = ensureLocalizer(options.Localizer)

	// Auto-set options based on verbosity level if not explicitly set
	if options.VerboseLevel >= 1 {
		options.ShowRuleName = true
		options.ShowContext = true // Level 1+ shows technical details including context
	}

	if options.VerboseLevel >= 2 {
		options.ShowHelp = true // Level 2+ adds help text
	}

	// Format error based on verbosity level
	writeValidationError(&builder, err, options, options.ShowHelp, options.Colors, options.Localizer)

	return builder.String()
}

// cleanForTerminal removes control characters and escape sequences.
func cleanForTerminal(text string) string {
	var builder strings.Builder

	for _, currentRune := range text {
		writeTerminalSafeRune(&builder, currentRune)
	}

	return strings.TrimSpace(builder.String())
}

// CleanForTerminal returns text that cannot emit terminal control sequences.
func CleanForTerminal(text string) string {
	return cleanForTerminal(text)
}

func writeTerminalSafeRune(builder *strings.Builder, currentRune rune) {
	if currentRune == 0 {
		return
	}

	if isTerminalSpacingRune(currentRune) {
		builder.WriteRune(' ')

		return
	}

	if currentRune == '\x1b' {
		builder.WriteString("\\x1b")

		return
	}

	if isTerminalControlRune(currentRune) {
		writeEscapedControlRune(builder, currentRune)

		return
	}

	builder.WriteRune(currentRune)
}

func isTerminalSpacingRune(currentRune rune) bool {
	return currentRune == '\n' || currentRune == '\r' || currentRune == '\t'
}

func isTerminalControlRune(currentRune rune) bool {
	return currentRune < 0x20 || currentRune == 0x7f || (currentRune >= 0x80 && currentRune <= 0x9f)
}

func writeEscapedControlRune(builder *strings.Builder, currentRune rune) {
	builder.WriteString("\\x")

	if currentRune < 0x10 {
		builder.WriteByte('0')
	}

	builder.WriteString(strconv.FormatInt(int64(currentRune), 16))
}

// writeValidationError formats validation error based on verbosity level.
func writeValidationError(builder *strings.Builder, err domain.ValidationError, options TextOptions, showHelpText bool, colors ColorScheme, localizer Localizer) {
	if options.VerboseLevel == 0 {
		writeUserFriendlyError(builder, err, options, showHelpText, colors, localizer)
	} else {
		writeComprehensiveError(builder, err, options, showHelpText, colors, localizer)
	}
}

// writeLevel0UserMessage writes user message for verbosity level 0.
func writeLevel0UserMessage(builder *strings.Builder, userMessage string, err domain.ValidationError, colors ColorScheme, localizer Localizer) {
	// For level 0, show the original message with indentation if no user-friendly message found
	if userMessage == err.Message {
		builder.WriteString(colors.Error(DefaultIndent + err.Message))
	} else {
		builder.WriteString(colors.Error(userMessage))
	}

	// For level 0, also show suggestion if available
	suggestion := generateUserSuggestion(err, localizer)
	if suggestion != "" {
		builder.WriteString("\n")
		// Add arrow for level 0 suggestions
		if !strings.HasPrefix(suggestion, "→") {
			suggestion = "→ " + suggestion
		}

		builder.WriteString(colors.Warning(suggestion))
	}
}

// writeUserFriendlyError formats user-friendly error message.
func writeUserFriendlyError(builder *strings.Builder, err domain.ValidationError, options TextOptions, showHelpText bool, colors ColorScheme, localizer Localizer) {
	userMessage := convertToUserFriendlyMessage(err, localizer)
	if options.VerboseLevel == 0 {
		writeLevel0UserMessage(builder, userMessage, err, colors, localizer)
	} else {
		builder.WriteString(colors.Error(userMessage))
	}

	if showHelpText {
		if helpMsg, ok := LookupHelpMessage(err.Diagnostic.Reason, err.Context); ok {
			builder.WriteString("\n")
			builder.WriteString(DefaultIndent + ensureLocalizer(localizer).Get(outputHelpHeader) + " " + helpMsg.Localize(localizer))
		}
	}

	builder.WriteString("\n")
}

// writeComprehensiveError formats error with technical details.
func writeComprehensiveError(builder *strings.Builder, err domain.ValidationError, options TextOptions, showHelpText bool, colors ColorScheme, localizer Localizer) {
	writeRulePrefix(builder, err, options, colors)
	writeUserMessage(builder, err, colors, localizer)
	writeSuggestion(builder, err, colors, localizer)
	writeTechnicalDetails(builder, err, options, colors, localizer)
	writeContextIfNeeded(builder, err, options, colors, localizer)
	writeHelpIfNeeded(builder, err, showHelpText, localizer)

	builder.WriteString("\n")
}

// writeRulePrefix writes the rule name prefix if requested.
func writeRulePrefix(builder *strings.Builder, err domain.ValidationError, options TextOptions, colors ColorScheme) {
	if options.ShowRuleName {
		displayName := DisplayName(err.Rule, options.Localizer)
		builder.WriteString(colors.Info(fmt.Sprintf("[%s] ", displayName)))
	}
}

// writeUserMessage writes the user-friendly error message.
func writeUserMessage(builder *strings.Builder, err domain.ValidationError, colors ColorScheme, localizer Localizer) {
	userMessage := convertToUserFriendlyMessage(err, localizer)
	builder.WriteString(colors.Error(userMessage))
}

// writeSuggestion writes error-specific suggestions.
func writeSuggestion(builder *strings.Builder, err domain.ValidationError, colors ColorScheme, localizer Localizer) {
	if err.Diagnostic.Reason == testErrorCode {
		builder.WriteString("\n")
		builder.WriteString(colors.Warning("→ " + localizer.Get("Fix the test error")))

		return
	}

	suggestion := generateUserSuggestion(err, localizer)
	if suggestion != "" {
		builder.WriteString("\n")

		if !strings.HasPrefix(suggestion, "→") {
			suggestion = "→ " + suggestion
		}

		builder.WriteString(colors.Warning(suggestion))
	}
}

// writeTechnicalDetails writes technical error details based on verbosity.
func writeTechnicalDetails(builder *strings.Builder, err domain.ValidationError, options TextOptions, _ ColorScheme, localizer Localizer) {
	shouldShow := (options.VerboseLevel >= 2 && err.Diagnostic.Reason == testErrorCode) || (options.VerboseLevel >= 1 && err.Diagnostic.Reason != testErrorCode)
	if !shouldShow {
		return
	}

	localizer = ensureLocalizer(localizer)

	if err.Diagnostic.ID != "" {
		builder.WriteString("\n")
		builder.WriteString(DefaultIndent + localizer.Get("Check ID: %s", err.Diagnostic.ID))
	}

	if err.Message != "" {
		builder.WriteString("\n")
		builder.WriteString(DefaultIndent + localizer.Get("Error Message: %s", err.Message))
	}
}

// writeContextIfNeeded writes context information if appropriate.
func writeContextIfNeeded(builder *strings.Builder, err domain.ValidationError, options TextOptions, colors ColorScheme, localizer Localizer) {
	if options.ShowContext || options.VerboseLevel >= 2 {
		writeFriendlyContext(builder, err, colors, localizer)
	}
}

// writeHelpIfNeeded writes help text if available and requested.
func writeHelpIfNeeded(builder *strings.Builder, err domain.ValidationError, showHelpText bool, localizer Localizer) {
	if !showHelpText {
		return
	}

	helpMsg, ok := LookupHelpMessage(err.Diagnostic.Reason, err.Context)
	if !ok {
		return
	}

	localizer = ensureLocalizer(localizer)

	builder.WriteString("\n")
	builder.WriteString(DefaultIndent + localizer.Get(outputHelpHeader) + " " + helpMsg.Localize(localizer))
}

// writeFriendlyContext writes context in human-readable terms.
func writeFriendlyContext(builder *strings.Builder, err domain.ValidationError, colors ColorScheme, localizer Localizer) {
	switch err.Diagnostic.Reason {
	case diagnostics.ReasonInvalidConventionalFormat:
		writeActualExpectedContext(builder, err.Context, diagnostics.ContextActual, diagnostics.ContextExpected, localizer)
	case diagnostics.ReasonSubjectTooLong:
		writeLengthContext(builder, err.Context, localizer)
	case diagnostics.ReasonSubjectCase, diagnostics.ReasonWrongCaseUpper, diagnostics.ReasonWrongCaseLower, diagnostics.ReasonWrongCaseSentence:
		writeWordContext(builder, err.Context, localizer)
	case diagnostics.ReasonMisspelledWord, diagnostics.ReasonSpelling:
		writeSpellContext(builder, err.Context, localizer)
	default:
		// Generic context handling for all other error codes
		if len(err.Context) > 0 {
			writeGenericContext(builder, err.Context, colors, localizer)
		}
	}
}

// writeActualExpectedContext writes actual/expected context pairs.
func writeActualExpectedContext(builder *strings.Builder, context map[string]string, actualKey, expectedKey string, localizer Localizer) {
	if actual, actualExists := context[actualKey]; actualExists {
		if expected, expectedExists := context[expectedKey]; expectedExists {
			localizer = ensureLocalizer(localizer)
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Actual: %s", actual))
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Expected: %s", expected))
		}
	}
}

// writeLengthContext writes length-related context.
func writeLengthContext(builder *strings.Builder, context map[string]string, localizer Localizer) {
	if actualLen, actualExists := context[diagnostics.ContextActual]; actualExists {
		if maxLen, maxExists := context[diagnostics.ContextExpectedMax]; maxExists {
			localizer = ensureLocalizer(localizer)
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Current length: %s characters", actualLen))
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Maximum allowed: %s characters", maxLen))
		}
	}
}

// writeSpellContext writes context for spelling errors.
func writeSpellContext(builder *strings.Builder, context map[string]string, localizer Localizer) {
	localizer = ensureLocalizer(localizer)

	if word := context[diagnostics.ContextWord]; word != "" {
		fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Misspelled: '%s'", word))
	}

	if suggestion := context[diagnostics.ContextSuggestion]; suggestion != "" {
		fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Suggestion: '%s'", suggestion))
	}

	if locale := context[diagnostics.ContextLocale]; locale != "" {
		fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Locale: %s", locale))
	}
}

// writeWordContext writes word-related context for case errors.
func writeWordContext(builder *strings.Builder, context map[string]string, localizer Localizer) {
	if actual, actualExists := context[contextFirstWord]; actualExists {
		if expected, expectedExists := context[contextExpectedWord]; expectedExists {
			localizer = ensureLocalizer(localizer)
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Current: '%s'", actual))
			fmt.Fprintf(builder, "%s%s\n", DefaultIndent, localizer.Get("Expected: '%s'", expected))
		}
	}
}

// writeGenericContext writes context with proper ordering and formatting.
func writeGenericContext(builder *strings.Builder, context map[string]string, colors ColorScheme, localizer Localizer) {
	// Define key ordering - ordered keys appear first
	orderedKeys := []string{
		contextFirstWord, contextExpectedWord, contextCharPosition, diagnostics.ContextSuggestion,
	}

	orderedSet := make(map[string]struct{}, len(orderedKeys))

	// Write ordered keys first
	for _, key := range orderedKeys {
		if value, exists := context[key]; exists {
			writeContextPair(builder, key, value, colors, localizer)
			builder.WriteString("\n")

			orderedSet[key] = struct{}{}
		}
	}

	// Write remaining unordered keys
	remainingKeys := make([]string, 0, len(context))
	for key := range context {
		if _, isOrdered := orderedSet[key]; isOrdered {
			continue
		}

		remainingKeys = append(remainingKeys, key)
	}

	if len(remainingKeys) == 0 {
		return
	}

	slices.Sort(remainingKeys)

	for _, key := range remainingKeys {
		writeContextPair(builder, key, context[key], colors, localizer)
		builder.WriteString("\n")
	}
}

// createErrorSummary creates a summary of validation errors.
func createErrorSummary(errors []domain.ValidationError) string {
	if len(errors) == 0 {
		return messageUnknownError
	}

	// Create comma-separated summary for tests
	messages := make([]string, 0, len(errors))
	for _, validationErr := range errors {
		// Clean message for terminal safety
		cleanMessage := cleanForTerminal(validationErr.Message)
		messages = append(messages, strings.TrimSpace(cleanMessage))
	}

	return strings.Join(messages, ", ")
}

// extractErrorType extracts the error type from a validation error.
func extractErrorType(err domain.ValidationError) string {
	// For known reasons, return predefined error types
	if errorType := getKnownErrorType(err.Diagnostic.Reason); errorType != "" {
		return errorType
	}

	// For unknown reasons, extract from message
	return extractWordsFromMessage(err.Message, string(err.Diagnostic.Reason))
}

// getKnownErrorType returns predefined error types for known reasons.
func getKnownErrorType(reason diagnostics.Reason) string {
	switch reason {
	case diagnostics.ReasonSubjectTooLong:
		return messageSubjectIs
	case diagnostics.ReasonSubjectCase:
		return messageWrongCase
	case diagnostics.ReasonMisspelledWord:
		return "Misspelled"
	case diagnostics.ReasonMissingSignoff:
		return messageMissingSignoff
	case diagnostics.ReasonInvalidConventionalFormat:
		return "Format issue"
	default:
		return ""
	}
}

// extractWordsFromMessage extracts first two words from a message for error type.
func extractWordsFromMessage(message, fallbackCode string) string {
	words := strings.Fields(message)

	if len(words) == 0 {
		if fallbackCode == unknownCodeConstant {
			return ""
		}

		return fallbackCode
	}

	if len(words) == 1 {
		return words[0]
	}

	return words[0] + " " + words[1]
}

// writeFormattedError writes a formatted error to a string builder.
func writeFormattedError(builder *strings.Builder, err domain.ValidationError, options TextOptions, showHelp bool, colors ColorScheme) {
	options.ShowHelp = showHelp
	options.Colors = colors
	builder.WriteString(FormatValidationError(err, options))
}

// writeContextPair writes a context key-value pair.
func writeContextPair(builder *strings.Builder, key, value string, colors ColorScheme, localizer Localizer) {
	localizer = ensureLocalizer(localizer)
	formattedKey := formatTechnicalContextKey(key, localizer)
	builder.WriteString(colors.Muted(fmt.Sprintf("    %s: %s", formattedKey, value)))
}

// writeHelpSection writes a help section.
func writeHelpSection(builder *strings.Builder, help string, _ ColorScheme) {
	if help != "" {
		builder.WriteString("\n")

		lines := strings.Split(help, "\n")
		for i, line := range lines {
			builder.WriteString(DefaultIndent + line)

			if i < len(lines)-1 {
				builder.WriteString("\n")
			}
		}
	}
}

// generateUserSuggestion generates a user suggestion based on the error.
func generateUserSuggestion(err domain.ValidationError, localizer Localizer) string {
	if suggestion := generateContextSuggestion(err, localizer); suggestion != "" {
		return suggestion
	}

	return generateCodeSpecificSuggestion(err, localizer)
}

// generateContextSuggestion generates suggestions from error context.
func generateContextSuggestion(err domain.ValidationError, localizer Localizer) string {
	if suggestion, ok := err.Context[diagnostics.ContextSuggestion]; ok {
		localizer = ensureLocalizer(localizer)

		switch err.Diagnostic.Reason {
		case diagnostics.ReasonMisspelledWord:
			return localizer.Get("Try: \"%s\"", suggestion)
		default:
			return localizer.Get("Try: %s", suggestion)
		}
	}

	return ""
}

// generateCodeSpecificSuggestion generates suggestions based on diagnostic reasons.
func generateCodeSpecificSuggestion(err domain.ValidationError, localizer Localizer) string {
	switch err.Diagnostic.Reason {
	case diagnostics.ReasonInvalidConventionalFormat:
		return generateConventionalFormatSuggestion(err, localizer)
	case diagnostics.ReasonSubjectTooLong:
		return generateSubjectTooLongSuggestion(err, localizer)
	case diagnostics.ReasonSubjectCase, diagnostics.ReasonWrongCaseUpper, diagnostics.ReasonWrongCaseLower, diagnostics.ReasonWrongCaseSentence:
		return generateInvalidCaseSuggestion(err, localizer)
	case diagnostics.ReasonMissingJira:
		return generateMissingJiraSuggestion(err, localizer)
	case diagnostics.ReasonMissingSignoff:
		return ensureLocalizer(localizer).Get("Add: git commit --signoff")
	default:
		return ""
	}
}

// generateConventionalFormatSuggestion generates suggestions for conventional format errors.
func generateConventionalFormatSuggestion(_ domain.ValidationError, localizer Localizer) string {
	return ensureLocalizer(localizer).Get("Try: fix: your message")
}

// generateSubjectTooLongSuggestion generates suggestions for subject too long errors.
func generateSubjectTooLongSuggestion(err domain.ValidationError, localizer Localizer) string {
	if subject, ok := err.Context[diagnostics.ContextSubject]; ok {
		subjectLength := utf8.RuneCountInString(subject)
		if subjectLength > 50 {
			truncated := truncateStringToRunes(subject, 45) + "..."

			return ensureLocalizer(localizer).Get("Try: \"%s\"", truncated)
		}
	}

	return ""
}

// truncateStringToRunes safely truncates a string to a maximum number of runes,
// ensuring UTF-8 characters are not broken.
func truncateStringToRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}

	return string(runes[:maxRunes])
}

// generateInvalidCaseSuggestion generates suggestions for invalid case errors.
func generateInvalidCaseSuggestion(err domain.ValidationError, localizer Localizer) string {
	subject, hasSubject := err.Context[diagnostics.ContextSubject]
	firstWord, hasFirstWord := err.Context[contextFirstWord]
	expectedWord, hasExpectedWord := err.Context[contextExpectedWord]

	if hasSubject && hasFirstWord && hasExpectedWord {
		newSubject := strings.Replace(subject, firstWord, expectedWord, 1)

		return ensureLocalizer(localizer).Get("Try: \"%s\"", newSubject)
	}

	return ""
}

// generateMissingJiraSuggestion generates suggestions for missing JIRA errors.
func generateMissingJiraSuggestion(err domain.ValidationError, localizer Localizer) string {
	if subject, ok := err.Context[diagnostics.ContextSubject]; ok {
		return ensureLocalizer(localizer).Get("Try: PROJ-123: %s", subject)
	}

	return ""
}
