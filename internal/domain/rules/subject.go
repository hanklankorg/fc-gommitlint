// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Subject case constants, aliased from the settings package so the config
// enum and the rule that consumes it cannot drift apart.
const (
	CaseLower    = settings.CaseLower
	CaseUpper    = settings.CaseUpper
	CaseSentence = settings.CaseSentence
	CaseAny      = settings.CaseAny
	CaseAuto     = settings.CaseAuto
)

const (
	activeCheckLength     = "length"
	activeCheckCase       = "case"
	activeCheckSuffix     = "suffix"
	activeCheckImperative = "imperative"
)

// contextValueTrue is the canonical truthy marker for boolean flags stored
// in validation-error context maps. Context values are strings (so the
// shape can be serialised and localised); "true" is the conventional
// truthy value mirrored by readers as `== contextValueTrue`.
const contextValueTrue = "true"

// Subject validation errors.
var (
	ErrSubjectEmpty                 = errors.New("subject is empty")
	ErrMissingSubjectAfterType      = errors.New("missing subject after type")
	ErrInvalidConventionalCommitFmt = errors.New("invalid conventional commit format")
	ErrInvalidFormat                = errors.New("invalid format")
	ErrNoLetterFound                = errors.New("no letter found")
)

// SubjectRule validates commit subject length, case, suffix, and imperative mood.
type SubjectRule struct {
	maxLength            int
	caseChoice           string
	invalidSuffixes      string
	isConventionalFormat bool
	allowedTypes         []string
	allowNonAlpha        bool
	requireImperative    bool
	imperativeValidator  *ImperativeValidator
}

// NewSubjectRule creates a new SubjectRule from config with explicit conventional rule dependency.
func NewSubjectRule(cfg settings.Config, isConventionalEnabled bool) SubjectRule {
	return NewSubjectRuleWithStemmer(cfg, isConventionalEnabled, defaultImperativeStemmer{})
}

// NewSubjectRuleWithStemmer creates a new SubjectRule with an explicit imperative stemmer dependency.
func NewSubjectRuleWithStemmer(cfg settings.Config, isConventionalEnabled bool, stemmer Stemmer) SubjectRule {
	maxLength := settings.DerefPositiveInt(cfg.Message.Subject.MaxLength, settings.DefaultSubjectMaxLength)

	caseChoice := resolveCaseChoice(settings.DerefString(cfg.Message.Subject.Case, settings.DefaultSubjectCase), isConventionalEnabled)
	invalidSuffixes := resolveInvalidSuffixes(cfg.Message.Subject.ForbidEndings)

	requireImperative := settings.DerefBool(cfg.Message.Subject.RequireImperative, false)

	var imperativeValidator *ImperativeValidator
	if requireImperative {
		imperativeValidator = NewImperativeValidator(stemmer)
	}

	return SubjectRule{
		maxLength:            maxLength,
		caseChoice:           caseChoice,
		invalidSuffixes:      invalidSuffixes,
		isConventionalFormat: isConventionalEnabled,
		allowedTypes:         cfg.Conventional.Types,
		allowNonAlpha:        false,
		requireImperative:    requireImperative,
		imperativeValidator:  imperativeValidator,
	}
}

// resolveCaseChoice determines the case validation setting with context-aware defaults.
// For conventional commits, lowercase is standard (e.g., "feat: add feature").
// For non-conventional commits, sentence case is standard (e.g., "Add new feature").
//
// Matching is exact: settings.NormalizeMessageConfig canonicalises the value at
// config-merge time and ValidateSubjectConfig rejects unknown ones, so the
// default branch means "auto or unset" rather than doubling as a typo sink.
func resolveCaseChoice(configCase string, isConventionalEnabled bool) string {
	switch configCase {
	case CaseLower, CaseUpper, CaseSentence, CaseAny:
		return configCase
	default:
		return contextAwareCaseDefault(isConventionalEnabled)
	}
}

// contextAwareCaseDefault returns the default case based on commit style.
func contextAwareCaseDefault(isConventionalEnabled bool) string {
	if isConventionalEnabled {
		return CaseLower
	}

	return CaseSentence
}

// resolveInvalidSuffixes builds the invalid suffix string from config or returns default.
// Uses settings.GetDefaultForbiddenEndings() as the single source of truth for defaults.
func resolveInvalidSuffixes(forbidEndings []string) string {
	if len(forbidEndings) == 0 {
		forbidEndings = settings.GetDefaultForbiddenEndings()
	}

	var builder strings.Builder
	for _, suffix := range forbidEndings {
		builder.WriteString(suffix)
	}

	if builder.Len() == 0 {
		return strings.Join(settings.GetDefaultForbiddenEndings(), "")
	}

	return builder.String()
}

// Name implements Rule interface.
func (r SubjectRule) Name() string {
	return rulecatalog.Subject
}

// ActiveChecks implements CheckDescriber, returning labels for each active sub-check.
func (r SubjectRule) ActiveChecks() []string {
	checks := []string{activeCheckLength}

	if r.caseChoice != CaseAny {
		checks = append(checks, activeCheckCase)
	}

	if r.invalidSuffixes != "" {
		checks = append(checks, activeCheckSuffix)
	}

	if r.requireImperative {
		checks = append(checks, activeCheckImperative)
	}

	return checks
}

// Validate validates commit subject requirements.
func (r SubjectRule) Validate(commit domain.Commit) []domain.ValidationError {
	var errors []domain.ValidationError

	if commit.Subject == "" {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonEmptySubject, "Subject is empty").
				WithContextMap(diagnostics.Context{
					diagnostics.ContextSubject:  commit.Subject,
					diagnostics.ContextExpected: r.caseChoice,
				}),
		}
	}

	if !utf8.ValidString(commit.Subject) {
		return []domain.ValidationError{r.invalidUTF8Error(commit.Subject)}
	}

	// Length validation with UTF-8 character counting
	subjectLength := utf8.RuneCountInString(commit.Subject)
	if subjectLength > r.maxLength {
		// Calculate how much over the limit
		excess := subjectLength - r.maxLength

		errors = append(errors,
			domain.New(r.Name(), diagnostics.ReasonSubjectTooLong,
				fmt.Sprintf("Subject too long: %d characters (maximum allowed: %d)", subjectLength, r.maxLength)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextActual:      strconv.Itoa(subjectLength),
					diagnostics.ContextExpectedMax: strconv.Itoa(r.maxLength),
					diagnostics.ContextSubject:     commit.Subject,
					diagnostics.ContextOver:        strconv.Itoa(excess),
				}))
	}

	// Case validation. If the case check failed because the subject was
	// unparseable (no extractable first letter — e.g. emoji-only "🎉🎉🎉"),
	// skip the imperative check too: it would emit a second error pointing
	// at the same root cause ("no first word") and the user gets two
	// messages for one problem. The suffix check runs unconditionally
	// because it doesn't depend on extracting words.
	caseErrors := r.validateCase(commit.Subject)
	errors = append(errors, caseErrors...)

	// Suffix validation
	if suffixErrors := r.validateSuffix(commit.Subject); len(suffixErrors) > 0 {
		errors = append(errors, suffixErrors...)
	}

	// Imperative validation — only when the foundational extract-first-word
	// step succeeded. Otherwise we'd emit a duplicate root-cause error.
	if !caseErrorsIndicateExtractionFailure(caseErrors) {
		if imperativeErrors := r.validateImperative(commit.Subject); len(imperativeErrors) > 0 {
			errors = append(errors, imperativeErrors...)
		}
	}

	return errors
}

// caseErrorsIndicateExtractionFailure reports whether validateCase failed
// because the subject contained no extractable text (emoji-only, all
// punctuation, etc.). When true, the imperative check has nothing to work
// with either and would emit a duplicate "no first word" error.
func caseErrorsIndicateExtractionFailure(errs []domain.ValidationError) bool {
	for _, validationErr := range errs {
		switch validationErr.Diagnostic.Reason { //nolint:exhaustive // only the three extraction-failure reasons signal early-return; others intentionally fall through.
		case diagnostics.ReasonInvalidFormat,
			diagnostics.ReasonInvalidConventionalFormat,
			diagnostics.ReasonMissingConventionalSubject:
			return true
		}
	}

	return false
}

// validateCase checks commit subject case style.
func (r SubjectRule) validateCase(subject string) []domain.ValidationError {
	firstLetter, textChecked, err := r.extractFirstLetterWithConventionalHandling(subject)
	if err != nil {
		return r.handleExtractionError(subject, err)
	}

	// Happy path: valid case
	if r.isCaseValid(firstLetter) {
		return nil
	}

	return r.generateCaseError(subject, firstLetter, textChecked)
}

// handleExtractionError handles errors from first letter extraction.
func (r SubjectRule) handleExtractionError(subject string, err error) []domain.ValidationError {
	errorCode := diagnostics.ReasonInvalidFormat

	if r.isConventionalFormat {
		switch {
		case errors.Is(err, ErrMissingSubjectAfterType):
			errorCode = diagnostics.ReasonMissingConventionalSubject
		case errors.Is(err, ErrInvalidConventionalCommitFmt):
			errorCode = diagnostics.ReasonInvalidConventionalFormat
		}
	}

	return []domain.ValidationError{
		domain.New(r.Name(), errorCode, err.Error()).
			WithContextMap(diagnostics.Context{
				diagnostics.ContextSubject: subject,
			}),
	}
}

// isCaseValid checks if the first letter meets case requirements.
func (r SubjectRule) isCaseValid(firstLetter rune) bool {
	switch r.caseChoice {
	case CaseUpper, CaseSentence:
		// Both upper and sentence case require uppercase first letter
		return unicode.IsUpper(firstLetter)
	case CaseLower:
		return unicode.IsLower(firstLetter)
	case CaseAny:
		return true
	default:
		return false
	}
}

// generateCaseError creates a validation error for case violations.
//
// When the configured case is `lower` AND require_imperative is on, we tag
// the error with requires_imperative_hint so the renderer can append a
// short reminder that subjects start with a verb. Gating on `lower` keeps
// the hint consistent with the user's case choice — projects on `upper`
// or `sentence` don't get a "lowercase verb" nudge that contradicts their
// config.
func (r SubjectRule) generateCaseError(subject string, firstLetter rune, textChecked string) []domain.ValidationError {
	expectedLetter := r.getExpectedLetter(firstLetter)
	firstWord, expectedWord := r.extractWordVariations(firstLetter, textChecked)
	errorCode := r.getCaseErrorCode()
	basicMessage := fmt.Sprintf("First letter '%c' should be '%c'", firstLetter, expectedLetter)

	ctx := map[string]string{
		"first_word":                firstWord,
		"expected_word":             expectedWord,
		diagnostics.ContextSubject:  subject,
		diagnostics.ContextExpected: r.caseChoice,
	}
	if r.requireImperative && r.caseChoice == CaseLower {
		ctx[diagnostics.ContextRequiresImperativeHint] = contextValueTrue
	}

	return []domain.ValidationError{
		domain.New(r.Name(), errorCode, basicMessage).
			WithContextMap(ctx),
	}
}

// getExpectedLetter returns the expected case for the first letter.
func (r SubjectRule) getExpectedLetter(firstLetter rune) rune {
	switch r.caseChoice {
	case CaseUpper, CaseSentence:
		return unicode.ToUpper(firstLetter)
	case CaseLower:
		return unicode.ToLower(firstLetter)
	default:
		return firstLetter
	}
}

// extractWordVariations extracts the first word and its expected variation.
func (r SubjectRule) extractWordVariations(firstLetter rune, textChecked string) (string, string) {
	words := strings.Fields(textChecked)
	firstWord := string(firstLetter)

	if len(words) > 0 {
		firstWord = words[0]
	}

	expectedWord := firstWord

	runes := []rune(firstWord)
	if len(runes) > 0 {
		if r.caseChoice == CaseUpper || r.caseChoice == CaseSentence {
			runes[0] = unicode.ToUpper(runes[0])
		} else {
			runes[0] = unicode.ToLower(runes[0])
		}

		expectedWord = string(runes)
	}

	return firstWord, expectedWord
}

// getCaseErrorCode returns the appropriate error code for case violations.
func (r SubjectRule) getCaseErrorCode() diagnostics.Reason {
	switch r.caseChoice {
	case CaseUpper:
		return diagnostics.ReasonWrongCaseUpper
	case CaseSentence:
		return diagnostics.ReasonWrongCaseSentence
	case CaseLower:
		return diagnostics.ReasonWrongCaseLower
	default:
		return diagnostics.ReasonSubjectCase
	}
}

// validateSuffix checks commit subject doesn't end with invalid characters.
func (r SubjectRule) validateSuffix(subject string) []domain.ValidationError {
	if !utf8.ValidString(subject) {
		return []domain.ValidationError{r.invalidUTF8Error(subject)}
	}

	// Use UTF-8 aware last character extraction
	lastChar, size := utf8.DecodeLastRuneInString(subject)

	if lastChar == utf8.RuneError && size == 0 {
		return []domain.ValidationError{
			r.invalidUTF8Error(subject),
		}
	}

	if strings.ContainsRune(r.invalidSuffixes, lastChar) {
		// Generate clean subject suggestion
		cleanSubject := subject
		if size > 0 {
			cleanSubject = subject[:len(subject)-size]
		}

		// Create list of invalid suffixes for help message
		var suffixList []string
		for _, suffix := range r.invalidSuffixes {
			suffixList = append(suffixList, string(suffix))
		}

		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonSubjectSuffix,
				fmt.Sprintf("Subject has invalid suffix %q (invalid suffixes: %q)", string(lastChar), r.invalidSuffixes)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextSubject:         subject,
					diagnostics.ContextActual:          string(lastChar),
					diagnostics.ContextExpected:        cleanSubject,
					diagnostics.ContextInvalidSuffixes: strings.Join(suffixList, " "),
				}),
		}
	}

	return nil
}

func (r SubjectRule) invalidUTF8Error(subject string) domain.ValidationError {
	return domain.New(r.Name(), diagnostics.ReasonInvalidUTF8, "Subject contains invalid UTF-8 text").
		WithContextMap(diagnostics.Context{
			diagnostics.ContextSubject: subject,
		})
}

// extractFirstLetterWithConventionalHandling handles conventional commit prefixes when finding the subject's first letter.
func (r SubjectRule) extractFirstLetterWithConventionalHandling(subject string) (rune, string, error) {
	if subject == "" {
		return 0, "", ErrSubjectEmpty
	}

	textToCheck, err := r.extractTextToCheck(subject)
	if err != nil {
		return 0, "", err
	}

	return r.findFirstLetter(textToCheck)
}

func (r SubjectRule) extractTextToCheck(subject string) (string, error) {
	text, _, err := conventionalValidationText(subject, r.isConventionalFormat, r.allowedTypes)

	return text, err
}

// findFirstLetter finds the first letter character in the text.
func (r SubjectRule) findFirstLetter(textToCheck string) (rune, string, error) {
	textToCheck = strings.TrimSpace(textToCheck)
	if textToCheck == "" {
		return 0, "", ErrInvalidFormat
	}

	for _, char := range textToCheck {
		if unicode.IsLetter(char) {
			return char, textToCheck, nil
		}
	}

	return 0, "", ErrNoLetterFound
}

// validateImperative checks subject uses imperative mood.
func (r SubjectRule) validateImperative(subject string) []domain.ValidationError {
	if !r.requireImperative || r.imperativeValidator == nil {
		return nil
	}

	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil
	}

	return r.imperativeValidator.ValidateImperative(subject, subject, r.isConventionalFormat, r.allowedTypes, r.Name())
}
