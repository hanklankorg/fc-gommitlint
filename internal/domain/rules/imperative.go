// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// Pre-compiled regex patterns for performance (avoids recompilation in loops).
var (
	// firstWordRegex extracts the first word from commit subject for imperative checking
	// Captures: "Add" from "Add feature", "Fix" from "  Fix bug" (ignores leading whitespace).
	firstWordRegex = regexp.MustCompile(`^\s*([a-zA-Z0-9]+)`)
)

const (
	imperativeErrorTypePastTense   = "past_tense"
	imperativeErrorTypeGerund      = "gerund"
	imperativeErrorTypeThirdPerson = "third_person"
	nonImperativeStarterAny        = "any"
)

// Stemmer stems words for imperative mood validation.
type Stemmer interface {
	Stem(word string) (string, error)
}

// ImperativeValidator validates imperative mood using an injected stemmer.
type ImperativeValidator struct {
	stemmer Stemmer
}

type defaultImperativeStemmer struct{}

func (defaultImperativeStemmer) Stem(word string) (string, error) {
	switch {
	case strings.HasSuffix(word, "ed"):
		return strings.TrimSuffix(word, "ed"), nil
	case strings.HasSuffix(word, "ing"):
		return strings.TrimSuffix(word, "ing"), nil
	case strings.HasSuffix(word, "es"):
		return strings.TrimSuffix(word, "es"), nil
	case strings.HasSuffix(word, "s"):
		return strings.TrimSuffix(word, "s"), nil
	default:
		return word, nil
	}
}

// NewImperativeValidator creates a new imperative validator. A nil stemmer
// means the built-in suffix stemmer, the same one NewSubjectRule uses.
func NewImperativeValidator(stemmer Stemmer) *ImperativeValidator {
	if stemmer == nil {
		stemmer = defaultImperativeStemmer{}
	}

	return &ImperativeValidator{stemmer: stemmer}
}

// ValidateImperative checks if text is in imperative mood. When isConventional is
// true, allowedTypes controls which type prefixes are recognized; empty means the
// standard conventional types.
func (v ImperativeValidator) ValidateImperative(text, originalSubject string, isConventional bool, allowedTypes []string, ruleName string) []domain.ValidationError {
	// Extract the text to validate based on commit type
	textToCheck, errs := extractValidationText(text, isConventional, allowedTypes, ruleName)
	if len(errs) > 0 {
		return errs
	}

	// Extract first word for validation
	firstWord, errs := extractFirstWord(textToCheck, ruleName)
	if len(errs) > 0 {
		return errs
	}

	// Validate the first word using linguistic analysis
	return v.analyzeWord(firstWord, originalSubject, ruleName)
}

func extractValidationText(subject string, isConventional bool, allowedTypes []string, ruleName string) (string, []domain.ValidationError) {
	text, parsed, err := conventionalValidationText(subject, isConventional, allowedTypes)
	if err == nil {
		return text, nil
	}

	if errors.Is(err, ErrMissingSubjectAfterType) {
		return "", []domain.ValidationError{
			domain.New(ruleName, diagnostics.ReasonMissingConventionalSubject,
				"Missing subject after conventional commit type").
				WithContextMap(diagnostics.Context{
					diagnostics.ContextSubject: subject,
					diagnostics.ContextType:    parsed.Type,
				}),
		}
	}

	return "", []domain.ValidationError{
		domain.New(ruleName, diagnostics.ReasonInvalidConventionalFormat,
			"Invalid conventional commit format").
			WithContextMap(diagnostics.Context{
				diagnostics.ContextSubject:  subject,
				diagnostics.ContextExpected: domain.ConventionalCommitFormatTemplate,
			}),
	}
}

// extractFirstWord extracts the first word from the text to validate.
func extractFirstWord(text, ruleName string) (string, []domain.ValidationError) {
	// Use pre-compiled regex to extract first word (alphanumeric characters)
	matches := firstWordRegex.FindStringSubmatch(text)

	if len(matches) < 2 {
		return "", []domain.ValidationError{
			domain.New(ruleName, diagnostics.ReasonNoFirstWord,
				"Cannot extract first word from commit message").
				WithContextMap(diagnostics.Context{
					"text": text,
				}),
		}
	}

	return matches[1], nil
}

// analyzeWord determines if a word is in imperative mood.
func (v ImperativeValidator) analyzeWord(word, subject, ruleName string) []domain.ValidationError {
	wordLower := strings.ToLower(word)

	if isNonImperativeStarter(wordLower) {
		return []domain.ValidationError{
			domain.New(ruleName, diagnostics.ReasonNonVerb,
				fmt.Sprintf("'%s' is not a verb", word)).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextActual:  word,
					diagnostics.ContextType:    "non_verb",
					diagnostics.ContextSubject: subject,
				}),
		}
	}

	stem, err := v.stemmer.Stem(wordLower)
	if err != nil {
		// A word the stemmer cannot handle is left unjudged rather than
		// guessed at. Snowball only fails for an unknown language, so this
		// does not fire in production.
		return nil
	}

	// Analyze using stemming results
	return validateWithStemming(wordLower, word, stem, subject, ruleName)
}

// validateWithStemming uses stemming to detect non-imperative forms.
// Enforces Git's imperative mood convention for commit subjects.
func validateWithStemming(wordLower, originalWord, stem, subject, ruleName string) []domain.ValidationError {
	// Define validation rules for different suffixes
	type suffixRule struct {
		suffix      string
		checkFunc   func(string) bool
		reason      diagnostics.Reason
		errorType   string
		description string
	}

	rules := []suffixRule{
		{
			suffix:      "ed",
			checkFunc:   func(w string) bool { return !isBaseFormEndingWithED(w) },
			reason:      diagnostics.ReasonPastTense,
			errorType:   imperativeErrorTypePastTense,
			description: "past tense",
		},
		{
			suffix:      "ing",
			checkFunc:   func(w string) bool { return true }, // Always check -ing words
			reason:      diagnostics.ReasonGerund,
			errorType:   imperativeErrorTypeGerund,
			description: "gerund (present participle)",
		},
		{
			suffix:      "s",
			checkFunc:   func(w string) bool { return !isBaseFormEndingWithS(w) },
			reason:      diagnostics.ReasonThirdPerson,
			errorType:   imperativeErrorTypeThirdPerson,
			description: "third person singular",
		},
	}

	for _, rule := range rules {
		if strings.HasSuffix(wordLower, rule.suffix) && stem != wordLower {
			if rule.checkFunc(wordLower) {
				suggestion := createImperativeSuggestion(stem, originalWord)

				return []domain.ValidationError{
					domain.New(ruleName, rule.reason,
						fmt.Sprintf("'%s' is %s", originalWord, rule.description)).
						WithContextMap(diagnostics.Context{
							diagnostics.ContextActual:     originalWord,
							diagnostics.ContextSubject:    subject,
							diagnostics.ContextSuggestion: suggestion,
							diagnostics.ContextType:       rule.errorType,
						}),
				}
			}
		}
	}

	// Word appears to be in imperative form
	return nil
}

// isNonImperativeStarter checks if a word is a non-imperative starter.
func isNonImperativeStarter(word string) bool {
	nonImperativeStarters := map[string]bool{
		// Pronouns
		"i": true, "we": true, "they": true, "he": true, "she": true, "it": true,
		// Articles
		"the": true, "a": true, "an": true,
		// Demonstratives
		"this": true, "that": true, "these": true, "those": true,
		// Possessives
		"my": true, "your": true, "our": true, "his": true, "her": true, "its": true,
		// Other non-verb starters
		"all": true, "some": true, "every": true, "no": true, nonImperativeStarterAny: true,
	}

	return nonImperativeStarters[word]
}

// isBaseFormEndingWithED checks if a word ending with 'ed' is actually a base form.
func isBaseFormEndingWithED(word string) bool {
	baseFormsEndingWithED := map[string]bool{
		// Words that naturally end with 'ed' in their base form
		"shed": true, "embed": true, "speed": true, "proceed": true,
		"exceed": true, "succeed": true, "feed": true, "need": true,
		"breed": true, "seed": true, "bleed": true, "freed": true,
		"greed": true, "creed": true, "deed": true, "weed": true,
	}

	return baseFormsEndingWithED[word]
}

// isBaseFormEndingWithS checks if a word ending with 's' is actually a base form.
func isBaseFormEndingWithS(word string) bool {
	baseFormsEndingWithS := map[string]bool{
		// Words that naturally end with 's' in their base form
		"focus": true, "process": true, "pass": true, "address": true,
		"express": true, "dismiss": true, "access": true, "press": true,
		"cross": true, "miss": true, "toss": true, "guess": true,
		"dress": true, "bless": true, "stress": true, "class": true,
		"mass": true, "bass": true, "glass": true, "grass": true,
	}

	return baseFormsEndingWithS[word]
}

// createImperativeSuggestion returns a curated imperative suggestion for a
// known stem, with first-letter case matched to the original word so the
// suggestion does not fight a configured lowercase / uppercase subject case
// rule. Unknown stems return "" — the renderer then falls back to the safe
// generic message ("Use present tense", "Use base form") instead of leaking
// a fabricated Snowball fragment like "Manag" or "Warn".
//
// We tried an algorithmic fallback (return the stem capitalised when it
// looks "plausible"). It produces non-words for -e-stripped verbs
// (managed → manag, scoped → scop, staged → stag) and feels confident
// while being wrong. Curated map only.
func createImperativeSuggestion(stem, originalWord string) string {
	if stem == "" {
		return originalWord
	}

	irregular := getIrregularStemImperative(stem)
	if irregular == "" {
		return ""
	}

	return matchFirstLetterCase(irregular, originalWord)
}

// matchFirstLetterCase rewrites target's first rune to match source's first
// rune case (upper / lower). UTF-8 safe; degrades gracefully on bad input.
func matchFirstLetterCase(target, source string) string {
	if target == "" || source == "" {
		return target
	}

	srcRune, _ := utf8.DecodeRuneInString(source)
	tgtRune, tgtSize := utf8.DecodeRuneInString(target)

	if srcRune == utf8.RuneError || tgtRune == utf8.RuneError {
		return target
	}

	if unicode.IsUpper(srcRune) {
		return string(unicode.ToUpper(tgtRune)) + target[tgtSize:]
	}

	return string(unicode.ToLower(tgtRune)) + target[tgtSize:]
}

// getIrregularStemImperative returns the imperative form for common irregular stems.
//
// Snowball's English stemmer trims productive suffixes aggressively — including
// -e, -ize, -ation — so verbs like "create", "capitalize", "realize" stem to
// "creat", "capit", "realiz". Map them back to plausible imperatives. The list
// is intentionally curated rather than algorithmic: a verb dictionary would
// pull in megabytes of data for a marginal benefit on a fast-path check.
func getIrregularStemImperative(stem string) string {
	irregularStems := map[string]string{
		// Stems that ARE valid imperatives — listed explicitly so the
		// curated-only suggestion path emits something useful for the
		// simpler test stemmer (which doesn't strip the trailing 'd').
		"add":  "Add",
		"warn": "Warn",
		// Common imperatives stripped of trailing 'e' by Snowball.
		"ad":     "Add",
		"updat":  "Update",
		"creat":  "Create",
		"delet":  "Delete",
		"fix":    "Fix",
		"remov":  "Remove",
		"chang":  "Change",
		"modifi": "Modify",
		// -ize / -ization verbs (US) where Snowball over-strips.
		"capit":    "Capitalize",
		"realiz":   "Realize",
		"normal":   "Normalize",
		"gener":    "Generalize",
		"general":  "Generalize",
		"optim":    "Optimize",
		"refactor": "Refactor",
		"serializ": "Serialize",
		"organ":    "Organize",
		// Other common engineering verbs Snowball mangles.
		"implement": "Implement",
		"renam":     "Rename",
		"replac":    "Replace",
		"validat":   "Validate",
		"migrat":    "Migrate",
		"merg":      "Merge",
		"rebas":     "Rebase",
		"clos":      "Close",
		"reviv":     "Revive",
		"resolv":    "Resolve",
		"include":   "Include",
		"exclud":    "Exclude",
		"introduc":  "Introduce",
	}

	return irregularStems[stem]
}
