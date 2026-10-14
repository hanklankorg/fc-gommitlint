// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const (
	subjectAddFeature             = "add feature"
	subjectAddFeatureUppercase    = "Add feature"
	subjectAddNewFeature          = "add new feature"
	subjectAddNewFeatureUppercase = "Add new feature"
	subjectFeatAddNewFeature      = "feat: add new feature"
	subjectFeatAddNewFeatureUpper = "feat: Add new feature"
	subjectInvalidSuffixesASCII   = ".!?"
	subjectPastTenseMessage       = "is past tense"
)

func TestSubjectRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		subject        string
		config         settings.Config
		wantErrCount   int
		wantErrCodes   []expectedReason
		wantErrMessage string
	}{
		{
			name:    "valid subject - all rules pass",
			subject: subjectAddNewFeature,
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:     new(50),
						Case:          new(CaseLower),
						ForbidEndings: []string{".", ","},
					},
				},
			},
			wantErrCount: 0,
		},
		{
			name:    "too long subject",
			subject: "this is a very long subject line that exceeds the maximum allowed length",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
			},
			wantErrCount:   1,
			wantErrCodes:   []expectedReason{diagnostics.ReasonSubjectTooLong},
			wantErrMessage: "Subject too long",
		},
		{
			name:    "wrong case",
			subject: subjectAddNewFeatureUppercase,
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
			},
			wantErrCount:   1,
			wantErrCodes:   []expectedReason{diagnostics.ReasonWrongCaseLower},
			wantErrMessage: "First letter 'A' should be 'a'",
		},
		{
			name:    "invalid suffix",
			subject: subjectAddNewFeature + ".",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:     new(50),
						Case:          new(CaseLower),
						ForbidEndings: []string{".", ","},
					},
				},
			},
			wantErrCount:   1,
			wantErrCodes:   []expectedReason{diagnostics.ReasonSubjectSuffix},
			wantErrMessage: "invalid suffix",
		},
		{
			name:    "multiple errors - length and case",
			subject: "This is a very long subject line that exceeds the maximum allowed length and has wrong case",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
			},
			wantErrCount: 2,
			wantErrCodes: []expectedReason{diagnostics.ReasonSubjectTooLong, diagnostics.ReasonWrongCaseLower},
		},
		{
			name:    "conventional commit valid",
			subject: subjectFeatAddNewFeature,
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
				Rules: settings.RulesConfig{
					Enabled: []string{rulecatalog.Conventional},
				},
			},
			wantErrCount: 0,
		},
		{
			name:    "conventional commit wrong case in description",
			subject: subjectFeatAddNewFeatureUpper,
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
				Rules: settings.RulesConfig{
					Enabled: []string{rulecatalog.Conventional},
				},
			},
			wantErrCount:   1,
			wantErrCodes:   []expectedReason{diagnostics.ReasonWrongCaseLower},
			wantErrMessage: "First letter 'A' should be 'a'",
		},
		{
			name:    "empty subject",
			subject: "",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseLower),
					},
				},
			},
			wantErrCount: 1, // Empty subject error
			wantErrCodes: []expectedReason{diagnostics.ReasonEmptySubject},
		},
		{
			name:         "invalid utf8 subject",
			subject:      string([]byte{'f', 'e', 'a', 't', ':', ' ', 0xff}),
			wantErrCount: 1,
			wantErrCodes: []expectedReason{diagnostics.ReasonInvalidUTF8},
		},
		{
			name:    "case any - always valid",
			subject: "whatever CaSe ThIs Is",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
						Case:      new(CaseAny),
					},
				},
			},
			wantErrCount: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// The rows enable conventional explicitly or not at all; the
			// factory resolves the real dependency in rulefactory.
			isConventionalEnabled := slices.Contains(testCase.config.Rules.Enabled, rulecatalog.Conventional)
			rule := NewSubjectRule(testCase.config, isConventionalEnabled)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				foundCodes := make(map[expectedReason]bool)
				for _, validationErr := range errors {
					foundCodes[reasonOf(validationErr)] = true

					if testCase.wantErrMessage != "" {
						check.Contains(t, validationErr.Message, testCase.wantErrMessage)
					}
				}

				for _, expectedCode := range testCase.wantErrCodes {
					check.True(t, foundCodes[expectedCode], "expected error code %s not found", expectedCode)
				}
			}
		})
	}
}

func TestSubjectRule_DefaultConfig_ValidatesCorrectly(t *testing.T) {
	t.Parallel()

	t.Run("conventional_commits_default_to_lowercase", func(t *testing.T) {
		t.Parallel()
		// Default case for conventional commits is "lower"
		rule := NewSubjectRule(settings.Config{}, true)

		// Should allow lowercase subjects
		validCommit := domain.Commit{Subject: subjectAddNewFeature}
		errors := rule.Validate(validCommit)
		check.Empty(t, errors, "lowercase subject should be valid with conventional default")

		// Should reject uppercase first letter
		uppercaseCommit := domain.Commit{Subject: subjectAddNewFeatureUppercase}
		errors = rule.Validate(uppercaseCommit)
		check.NotEmpty(t, errors, "uppercase first letter should be invalid with conventional default")
		check.Equal(t, diagnostics.ReasonWrongCaseLower, reasonOf(errors[0]))
	})

	t.Run("non_conventional_commits_default_to_sentence", func(t *testing.T) {
		t.Parallel()
		// Default case for non-conventional commits is "sentence"
		rule := NewSubjectRule(settings.Config{}, false)

		// Should allow uppercase first letter
		validCommit := domain.Commit{Subject: subjectAddNewFeatureUppercase}
		errors := rule.Validate(validCommit)
		check.Empty(t, errors, "sentence case subject should be valid with non-conventional default")

		// Should reject lowercase first letter
		lowercaseCommit := domain.Commit{Subject: subjectAddNewFeature}
		errors = rule.Validate(lowercaseCommit)
		check.NotEmpty(t, errors, "lowercase first letter should be invalid with non-conventional default")
		check.Equal(t, diagnostics.ReasonWrongCaseSentence, reasonOf(errors[0]))
	})

	t.Run("length_validation", func(t *testing.T) {
		t.Parallel()

		rule := NewSubjectRule(settings.Config{}, true)

		// Should reject subjects over 100 chars
		tooLongCommit := domain.Commit{Subject: strings.Repeat("a", 101)}
		errors := rule.Validate(tooLongCommit)
		check.NotEmpty(t, errors, "101-char subject should be invalid")
		check.Equal(t, diagnostics.ReasonSubjectTooLong, reasonOf(errors[0]))
	})

	t.Run("suffix_validation", func(t *testing.T) {
		t.Parallel()

		rule := NewSubjectRule(settings.Config{}, true)

		// Should reject subjects with forbidden endings
		invalidEndingCommit := domain.Commit{Subject: subjectAddNewFeature + "."}
		errors := rule.Validate(invalidEndingCommit)
		check.NotEmpty(t, errors, "Subject ending with period should be invalid")
		check.Equal(t, diagnostics.ReasonSubjectSuffix, reasonOf(errors[0]))
	})
}

func TestSubjectRule_ConfigOverrides(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{
				MaxLength:         new(100),
				Case:              new(CaseLower),
				ForbidEndings:     []string{"!", "?"},
				RequireImperative: new(true),
			},
		},
		Rules: settings.RulesConfig{
			Enabled: []string{rulecatalog.Conventional},
		},
	}

	rule := NewSubjectRule(cfg, true) // conventional is enabled in config

	check.Equal(t, 100, rule.maxLength)
	check.Equal(t, CaseLower, rule.caseChoice)
	check.Equal(t, "!?", rule.invalidSuffixes)
	check.True(t, rule.isConventionalFormat)
	check.False(t, rule.allowNonAlpha) // Fixed: this should not be tied to imperative setting
	check.True(t, rule.requireImperative)
}

// TestNewSubjectRule_ResolvesCaseChoiceFromConfigAndConventionalMode is the
// comprehensive test of resolveCaseChoice, driven through the constructor.
//
// The interesting part is the context-aware default: an empty or unrecognised
// case setting resolves to lower under conventional commits and to sentence
// without them. Nowhere else asserts rule.caseChoice across the full set of
// settings, which is what makes this test worth keeping distinct from the
// several that check how a wrongly-cased subject is reported.
//
// It deliberately asserts an error count rather than a diagnostic reason. The
// reasons are held in three other places -- TestSubjectRule_CaseWithUTF8,
// TestSubjectRule_DefaultConfig_ValidatesCorrectly, and the rule factory's
// tests -- and the normalised case asserted just above is the thing that
// decides which reason follows, so pinning it here would restate a conclusion
// this test already establishes at its source.
func TestNewSubjectRule_ResolvesCaseChoiceFromConfigAndConventionalMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		configCase            string
		subject               string
		isConventionalEnabled bool
		wantErrCount          int
		wantNormalizedCase    string
	}{
		// Context-aware defaults
		{
			name:                  "empty_config_conventional_defaults_to_lower",
			configCase:            "",
			subject:               subjectAddFeature,
			isConventionalEnabled: true,
			wantErrCount:          0,
			wantNormalizedCase:    CaseLower,
		},
		{
			name:                  "empty_config_conventional_rejects_uppercase",
			configCase:            "",
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: true,
			wantErrCount:          1,
			wantNormalizedCase:    CaseLower,
		},
		{
			name:                  "empty_config_non_conventional_defaults_to_sentence",
			configCase:            "",
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseSentence,
		},
		{
			name:                  "empty_config_non_conventional_rejects_lowercase",
			configCase:            "",
			subject:               subjectAddFeature,
			isConventionalEnabled: false,
			wantErrCount:          1,
			wantNormalizedCase:    CaseSentence,
		},
		// Explicit case settings
		{
			name:                  "sentence_case_valid_uppercase",
			configCase:            CaseSentence,
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseSentence,
		},
		{
			name:                  "sentence_case_invalid_lowercase",
			configCase:            CaseSentence,
			subject:               subjectAddFeature,
			isConventionalEnabled: false,
			wantErrCount:          1,
			wantNormalizedCase:    CaseSentence,
		},
		{
			name:                  "any_case_allows_lowercase",
			configCase:            CaseAny,
			subject:               subjectAddFeature,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseAny,
		},
		{
			name:                  "any_case_allows_uppercase",
			configCase:            CaseAny,
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseAny,
		},
		{
			name:                  "any_case_mixed_valid",
			configCase:            CaseAny,
			subject:               "MiXeD CaSe feature",
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseAny,
		},
		{
			name:                  "unknown_value_conventional_defaults_to_lower",
			configCase:            "unknown_invalid_value",
			subject:               subjectAddFeature,
			isConventionalEnabled: true,
			wantErrCount:          0,
			wantNormalizedCase:    CaseLower,
		},
		{
			name:                  "unknown_value_non_conventional_defaults_to_sentence",
			configCase:            "unknown_invalid_value",
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseSentence,
		},
		{
			name:                  "lower_case_valid",
			configCase:            CaseLower,
			subject:               subjectAddFeature,
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseLower,
		},
		{
			name:                  "lower_case_invalid_uppercase",
			configCase:            CaseLower,
			subject:               subjectAddFeatureUppercase,
			isConventionalEnabled: false,
			wantErrCount:          1,
			wantNormalizedCase:    CaseLower,
		},
		{
			name:                  "upper_case_valid",
			configCase:            CaseUpper,
			subject:               "ADD FEATURE",
			isConventionalEnabled: false,
			wantErrCount:          0,
			wantNormalizedCase:    CaseUpper,
		},
		{
			name:                  "upper_case_invalid_lowercase",
			configCase:            CaseUpper,
			subject:               subjectAddFeature,
			isConventionalEnabled: false,
			wantErrCount:          1,
			wantNormalizedCase:    CaseUpper,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(100),
						Case:      new(testCase.configCase),
					},
				},
			}

			rule := NewSubjectRule(cfg, testCase.isConventionalEnabled)

			// Verify case was normalized correctly
			check.Equal(t, testCase.wantNormalizedCase, rule.caseChoice,
				"case should be normalized to %s", testCase.wantNormalizedCase)

			// Validate the subject
			commit := domain.Commit{Subject: testCase.subject}
			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount,
				"expected %d errors for subject %q with case %q",
				testCase.wantErrCount, testCase.subject, testCase.configCase)
		})
	}
}

func TestSubjectRule_ImperativeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		subject             string
		requireImperative   bool
		conventionalEnabled bool
		wantErrCount        int
		wantErrCode         expectedReason
		wantErrMessage      string
	}{
		{
			name:              "imperative disabled - no validation",
			subject:           "Added new feature",
			requireImperative: false,
			wantErrCount:      0,
		},
		{
			name:              "valid imperative",
			subject:           subjectAddNewFeatureUppercase,
			requireImperative: true,
			wantErrCount:      0,
		},
		{
			name:              "past tense violation",
			subject:           "Added new feature",
			requireImperative: true,
			wantErrCount:      1,
			wantErrCode:       diagnostics.ReasonPastTense,
			wantErrMessage:    subjectPastTenseMessage,
		},
		{
			name:              "gerund violation",
			subject:           "Adding new feature",
			requireImperative: true,
			wantErrCount:      1,
			wantErrCode:       diagnostics.ReasonGerund,
			wantErrMessage:    "is gerund (present participle)",
		},
		{
			name:              "third person violation",
			subject:           "Adds new feature",
			requireImperative: true,
			wantErrCount:      1,
			wantErrCode:       diagnostics.ReasonThirdPerson,
			wantErrMessage:    "is third person singular",
		},
		{
			name:                "conventional commit - valid imperative",
			subject:             subjectFeatAddNewFeatureUpper,
			requireImperative:   true,
			conventionalEnabled: true,
			wantErrCount:        0,
		},
		{
			name:                "conventional commit - past tense violation",
			subject:             "feat: Added new feature",
			requireImperative:   true,
			conventionalEnabled: true,
			wantErrCount:        1,
			wantErrCode:         diagnostics.ReasonPastTense,
			wantErrMessage:      subjectPastTenseMessage,
		},
		{
			name:              "empty subject - no imperative error",
			subject:           "",
			requireImperative: true,
			wantErrCount:      0, // Empty subject handled by other validation
		},
		{
			name:              "single word imperative",
			subject:           "Fix",
			requireImperative: true,
			wantErrCount:      0,
		},
		{
			name:              "single word past tense",
			subject:           "Fixed",
			requireImperative: true,
			wantErrCount:      1,
			wantErrCode:       diagnostics.ReasonPastTense,
			wantErrMessage:    subjectPastTenseMessage,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(72),
						Case:              new(CaseLower), // Focus on imperative testing
						RequireImperative: new(testCase.requireImperative),
					},
				},
			}

			if testCase.conventionalEnabled {
				cfg.Rules.Enabled = []string{rulecatalog.Conventional}
			}

			rule := NewSubjectRule(cfg, testCase.conventionalEnabled)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			if testCase.wantErrCount == 0 {
				validateNoImperativeErrors(t, errors)
			} else {
				validateExpectedImperativeError(t, errors, testCase.wantErrCode, testCase.wantErrMessage)
			}
		})
	}
}

// A subject with no extractable first word fails the case check; the imperative
// check must not add a second error for the same root cause.
func TestSubjectRule_UnparseableSubjectIsReportedOnce(t *testing.T) {
	t.Parallel()

	rule := NewSubjectRule(settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{RequireImperative: new(true)},
		},
	}, false)

	tests := []struct {
		name        string
		subject     string
		wantReasons []expectedReason
	}{
		{name: "emoji_only_subject", subject: "🎉🎉🎉", wantReasons: []expectedReason{diagnostics.ReasonInvalidFormat}},
		{name: "parseable_subject_still_gets_the_imperative_verdict", subject: "Added feature", wantReasons: []expectedReason{diagnostics.ReasonPastTense}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var reasons []expectedReason
			for _, validationErr := range rule.Validate(domain.Commit{Subject: testCase.subject}) {
				reasons = append(reasons, reasonOf(validationErr))
			}

			check.Equal(t, testCase.wantReasons, reasons)
		})
	}
}

// The renderer's "subjects start with a verb" reminder only makes sense for a
// lowercase policy that also requires the imperative mood.
func TestSubjectRule_ImperativeHintOnlyForLowerCaseWithImperative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		caseChoice        string
		requireImperative bool
		subject           string
		wantReason        expectedReason
		wantHint          bool
	}{
		{name: "lower_with_imperative_carries_the_hint", caseChoice: CaseLower, requireImperative: true, subject: subjectAddFeatureUppercase, wantReason: diagnostics.ReasonWrongCaseLower, wantHint: true},
		{name: "sentence_with_imperative_has_no_hint", caseChoice: CaseSentence, requireImperative: true, subject: subjectAddFeature, wantReason: diagnostics.ReasonWrongCaseSentence},
		{name: "lower_without_imperative_has_no_hint", caseChoice: CaseLower, requireImperative: false, subject: subjectAddFeatureUppercase, wantReason: diagnostics.ReasonWrongCaseLower},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := NewSubjectRule(settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						Case:              new(testCase.caseChoice),
						RequireImperative: new(testCase.requireImperative),
					},
				},
			}, false)

			errors := rule.Validate(domain.Commit{Subject: testCase.subject})
			check.Len(t, errors, 1)
			check.Equal(t, testCase.wantReason, reasonOf(errors[0]))

			hint, hasHint := errors[0].Context[diagnostics.ContextRequiresImperativeHint]
			check.Equal(t, testCase.wantHint, hasHint)

			if testCase.wantHint {
				check.Equal(t, "true", hint)
			}
		})
	}
}

func TestSubjectRule_UTF8Length(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		maxLength    int
		wantErrCount int
		wantErrCode  expectedReason
	}{
		{
			name:         "ASCII subject within limit",
			subject:      subjectAddFeatureUppercase,
			maxLength:    50,
			wantErrCount: 0,
		},
		{
			name:         "UTF-8 subject within limit",
			subject:      "Add 🚀 feature",
			maxLength:    50,
			wantErrCount: 0,
		},
		{
			name:         "UTF-8 subject exceeding limit",
			subject:      "This is a very long subject line with émojis 🚀🎉 that exceeds the maximum allowed length",
			maxLength:    50,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectTooLong,
		},
		{
			name:         "Unicode characters counted correctly",
			subject:      "测试中文字符长度验证功能",
			maxLength:    10,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectTooLong,
		},
		{
			name:         "Mixed ASCII and Unicode",
			subject:      "Fix bug in 用户认证 system",
			maxLength:    15,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectTooLong,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(testCase.maxLength),
						Case:      new(CaseAny), // Focus on length testing
					},
				},
			}

			rule := NewSubjectRule(cfg, false)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
				check.NotEmpty(t, errors[0].Context[diagnostics.ContextActual], "should have actual length")
				check.NotEmpty(t, errors[0].Context[diagnostics.ContextExpectedMax], "should have max length")
				check.NotEmpty(t, errors[0].Context["over"], "should have excess count")
			}
		})
	}
}

// TestSubjectRule_LengthLimitCountsRunesAtBothEdges pins the subject length
// limit from both sides. Every existing row is well past the limit, so a rule
// that refused a subject exactly at the limit, or that measured UTF-8 bytes
// instead of runes, would still pass them. The reported numbers are asserted
// too: an over-limit error carries the rune count, the limit and the excess.
func TestSubjectRule_LengthLimitCountsRunesAtBothEdges(t *testing.T) {
	t.Parallel()

	const maxLength = 10

	// 10 runes but 19 bytes: at the limit for a rune count, far over it for a
	// byte count.
	atLimit := "A" + strings.Repeat("ä", maxLength-1)
	overLimit := atLimit + "ä"

	cfg := settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{
				MaxLength: new(maxLength),
				Case:      new(CaseAny),
			},
		},
	}
	rule := NewSubjectRule(cfg, false)

	check.Empty(t, rule.Validate(domain.Commit{Subject: atLimit}), "a subject of exactly max_length characters is allowed")

	errors := rule.Validate(domain.Commit{Subject: overLimit})
	check.Len(t, errors, 1)
	check.Equal(t, diagnostics.ReasonSubjectTooLong, reasonOf(errors[0]))
	check.Equal(t, "11", errors[0].Context[diagnostics.ContextActual])
	check.Equal(t, "10", errors[0].Context[diagnostics.ContextExpectedMax])
	check.Equal(t, "1", errors[0].Context[diagnostics.ContextOver])
}

func TestSubjectRule_ComprehensiveSuffixValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		wantErrCount int
		wantErrCode  expectedReason
		invalidChar  string
	}{
		{
			name:         "Valid subject without forbidden suffixes",
			subject:      subjectAddNewFeatureUppercase,
			wantErrCount: 0,
		},
		{
			name:         "Period suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + ".",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  ".",
		},
		{
			name:         "Comma suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + ",",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  ",",
		},
		{
			name:         "Semicolon suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + ";",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  ";",
		},
		{
			name:         "Colon suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + ":",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  ":",
		},
		{
			name:         "Exclamation mark suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + "!",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  "!",
		},
		{
			name:         "Question mark suffix forbidden",
			subject:      subjectAddNewFeatureUppercase + "?",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonSubjectSuffix,
			invalidChar:  "?",
		},
		{
			name:         "UTF-8 suffix handling",
			subject:      subjectAddNewFeatureUppercase + "™",
			wantErrCount: 0, // Should not be in default forbidden list
		},
		{
			name:         "Empty subject handled",
			subject:      "",
			wantErrCount: 1, // Empty subject error
			wantErrCode:  diagnostics.ReasonEmptySubject,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(100),
						Case:      new(CaseAny), // Focus on suffix testing
					},
				},
			}

			rule := NewSubjectRule(cfg, false)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 && testCase.wantErrCode != "" {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))

				if testCase.invalidChar != "" {
					check.Contains(t, errors[0].Context[diagnostics.ContextActual], testCase.invalidChar)
				}
			}
		})
	}
}

func TestSubjectRule_CustomForbidEndings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		subject       string
		forbidEndings []string
		wantErrCount  int
		wantErrCode   expectedReason
		invalidChar   string
	}{
		{
			name:          "custom_forbid_comma",
			subject:       subjectAddNewFeatureUppercase + ",",
			forbidEndings: []string{",", ";", ":"},
			wantErrCount:  1,
			wantErrCode:   diagnostics.ReasonSubjectSuffix,
			invalidChar:   ",",
		},
		{
			name:          "custom_forbid_semicolon",
			subject:       subjectAddNewFeatureUppercase + ";",
			forbidEndings: []string{",", ";", ":"},
			wantErrCount:  1,
			wantErrCode:   diagnostics.ReasonSubjectSuffix,
			invalidChar:   ";",
		},
		{
			name:          "custom_forbid_colon",
			subject:       subjectAddNewFeatureUppercase + ":",
			forbidEndings: []string{",", ";", ":"},
			wantErrCount:  1,
			wantErrCode:   diagnostics.ReasonSubjectSuffix,
			invalidChar:   ":",
		},
		{
			name:          "custom_forbid_allows_period_when_excluded",
			subject:       subjectAddNewFeatureUppercase + ".",
			forbidEndings: []string{",", ";", ":"}, // Period explicitly excluded from custom list
			wantErrCount:  0,
		},
		{
			name:          "extended_forbid_list",
			subject:       subjectAddNewFeatureUppercase + "~",
			forbidEndings: []string{".", "!", "?", ",", ";", ":", "~"},
			wantErrCount:  1,
			wantErrCode:   diagnostics.ReasonSubjectSuffix,
			invalidChar:   "~",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:     new(100),
						Case:          new(CaseAny),
						ForbidEndings: testCase.forbidEndings,
					},
				},
			}

			rule := NewSubjectRule(cfg, false)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 && testCase.wantErrCode != "" {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))

				if testCase.invalidChar != "" {
					check.Contains(t, errors[0].Context[diagnostics.ContextActual], testCase.invalidChar)
				}
			}
		})
	}
}

func TestSubjectRule_CaseWithUTF8(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		caseChoice   string
		wantErrCount int
		wantErrCode  expectedReason
	}{
		{
			name:         "Valid upper case ASCII",
			subject:      "ADD NEW FEATURE",
			caseChoice:   CaseUpper,
			wantErrCount: 0,
		},
		{
			name:         "Valid upper case UTF-8",
			subject:      "AÑADIR NUEVA FUNCIÓN",
			caseChoice:   CaseUpper,
			wantErrCount: 0,
		},
		{
			name:         "Invalid upper case UTF-8",
			subject:      "añadir nueva función",
			caseChoice:   CaseUpper,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonWrongCaseUpper,
		},
		{
			name:         "Valid upper case",
			subject:      "ADD NEW FEATURE",
			caseChoice:   CaseUpper,
			wantErrCount: 0,
		},
		{
			name:         "Invalid upper case - should use specific error code",
			subject:      subjectAddNewFeature,
			caseChoice:   CaseUpper,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonWrongCaseUpper,
		},
		{
			name:         "Valid lower case",
			subject:      subjectAddNewFeature,
			caseChoice:   CaseLower,
			wantErrCount: 0,
		},
		{
			name:         "Invalid lower case - should use specific error code",
			subject:      subjectAddNewFeatureUppercase,
			caseChoice:   CaseLower,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonWrongCaseLower,
		},
		{
			name:         "Any case always valid",
			subject:      "whatever CaSe ThIs Is",
			caseChoice:   CaseAny,
			wantErrCount: 0,
		},
		{
			name:         "Empty subject specific error",
			subject:      "",
			caseChoice:   CaseLower,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonEmptySubject,
		},
		{
			name:         "UTF-8 character as first letter",
			subject:      "øverføre data",
			caseChoice:   CaseLower,
			wantErrCount: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(100),
						Case:      new(testCase.caseChoice),
					},
				},
			}

			rule := NewSubjectRule(cfg, false)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
				check.Contains(t, errors[0].Context[diagnostics.ContextExpected], testCase.caseChoice)
			}
		})
	}
}

func TestSubjectRule_ConventionalCommitParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		subject             string
		conventionalEnabled bool
		wantErrCount        int
		wantErrCode         expectedReason
	}{
		{
			name:                "Valid conventional commit",
			subject:             subjectFeatAddNewFeatureUpper,
			conventionalEnabled: true,
			wantErrCount:        0,
		},
		{
			name:                "Valid conventional with scope",
			subject:             "feat(auth): Add login functionality",
			conventionalEnabled: true,
			wantErrCount:        0,
		},
		{
			name:                "Invalid conventional - missing description",
			subject:             "feat: ",
			conventionalEnabled: true,
			wantErrCount:        1,
			wantErrCode:         diagnostics.ReasonMissingConventionalSubject,
		},
		{
			name:                "Invalid conventional format",
			subject:             "feat add feature",
			conventionalEnabled: true,
			wantErrCount:        1,
			wantErrCode:         diagnostics.ReasonInvalidConventionalFormat,
		},
		{
			name:                "Non-conventional when not enabled",
			subject:             subjectAddNewFeatureUppercase,
			conventionalEnabled: false,
			wantErrCount:        0,
		},
		{
			name:                "Conventional-like but invalid",
			subject:             "feat(scope: missing closing paren",
			conventionalEnabled: true,
			wantErrCount:        1,
			wantErrCode:         diagnostics.ReasonInvalidConventionalFormat,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(100),
						Case:      new(CaseAny), // Focus on conventional commit parsing
					},
				},
			}

			if testCase.conventionalEnabled {
				cfg.Rules.Enabled = []string{rulecatalog.Conventional}
			}

			rule := NewSubjectRule(cfg, testCase.conventionalEnabled)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 && testCase.wantErrCode != "" {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
				check.Equal(t, testCase.subject, errors[0].Context[diagnostics.ContextSubject])
			}
		})
	}
}

func TestSubjectRule_ImperativeMood(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		wantErrCount int
		wantErrCode  expectedReason
		category     string
	}{
		{
			name:         "Valid imperative",
			subject:      subjectAddNewFeature,
			wantErrCount: 0,
		},
		{
			name:         "Past tense violation - specific error code",
			subject:      "added new feature",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonPastTense,
			category:     "past_tense",
		},
		{
			name:         "Gerund violation - specific error code",
			subject:      "adding new feature",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonGerund,
			category:     "gerund",
		},
		{
			name:         "Third person violation - specific error code",
			subject:      "adds new feature",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonThirdPerson,
			category:     "third_person",
		},
		{
			name:         "Conventional commit imperative valid",
			subject:      subjectFeatAddNewFeature,
			wantErrCount: 0,
		},
		{
			name:         "Conventional commit past tense violation",
			subject:      "feat: added new feature",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonPastTense,
		},
		{
			name:         "Base form ending with 'ed' should be valid",
			subject:      "need to update documentation",
			wantErrCount: 0,
		},
		{
			name:         "Enhanced suggestion generation",
			subject:      "fixed bug in authentication",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonPastTense,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(100),
						Case:              new(CaseLower), // Focus on imperative testing
						RequireImperative: new(true),
					},
				},
				Rules: settings.RulesConfig{
					Enabled: []string{rulecatalog.Conventional},
				},
			}

			rule := NewSubjectRule(cfg, true) // conventional is enabled in config
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))

				if testCase.category != "" {
					check.Equal(t, testCase.category, errors[0].Context["type"])
				}

				check.Contains(t, errors[0].Context, diagnostics.ContextSuggestion)
			}
		})
	}
}

func TestSubjectRule_BackwardCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		subject               string
		config                settings.Config
		isConventionalEnabled bool
		wantErr               bool
	}{
		{
			name:                  "Conventional default lowercase accepts lowercase",
			subject:               subjectAddNewFeature,
			config:                settings.Config{},
			isConventionalEnabled: true,
			wantErr:               false,
		},
		{
			name:                  "Conventional default lowercase rejects uppercase",
			subject:               subjectAddNewFeatureUppercase,
			config:                settings.Config{},
			isConventionalEnabled: true,
			wantErr:               true,
		},
		{
			name:                  "Non-conventional default sentence accepts uppercase",
			subject:               subjectAddNewFeatureUppercase,
			config:                settings.Config{},
			isConventionalEnabled: false,
			wantErr:               false,
		},
		{
			name:                  "Non-conventional default sentence rejects lowercase",
			subject:               subjectAddNewFeature,
			config:                settings.Config{},
			isConventionalEnabled: false,
			wantErr:               true,
		},
		{
			name:    "Original max length still works",
			subject: "this is a very long subject line that exceeds the maximum allowed length for testing purposes",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(50),
					},
				},
			},
			isConventionalEnabled: true,
			wantErr:               true,
		},
		{
			name:    "Explicit case override works",
			subject: subjectAddNewFeature,
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						Case: new(CaseUpper),
					},
				},
			},
			isConventionalEnabled: true,
			wantErr:               true,
		},
		{
			name:    "Suffix validation still works",
			subject: subjectAddNewFeature + ".",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						ForbidEndings: []string{"."},
					},
				},
			},
			isConventionalEnabled: true,
			wantErr:               true,
		},
		{
			name:                  "Default forbidden suffixes work",
			subject:               subjectAddNewFeature + "!",
			config:                settings.Config{},
			isConventionalEnabled: true,
			wantErr:               true, // ! is in default forbidden suffixes
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := NewSubjectRule(testCase.config, testCase.isConventionalEnabled)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors, "Expected validation error")
			} else {
				check.Empty(t, errors, "Expected no validation errors")
			}
		})
	}
}

// validateNoImperativeErrors validates that no imperative validation errors are present.
func validateNoImperativeErrors(t *testing.T, errors []domain.ValidationError) {
	t.Helper()

	// Keep only the reasons imperative validation emits; other subject checks
	// (case, length) are not under test here.
	imperativeErrors := make([]domain.ValidationError, 0, len(errors))

	imperativeReasons := []expectedReason{
		diagnostics.ReasonNonImperative, diagnostics.ReasonNoFirstWord, diagnostics.ReasonNonVerb,
		diagnostics.ReasonPastTense, diagnostics.ReasonGerund, diagnostics.ReasonThirdPerson,
	}

	for _, validationErr := range errors {
		if slices.Contains(imperativeReasons, reasonOf(validationErr)) {
			imperativeErrors = append(imperativeErrors, validationErr)
		}
	}

	check.Empty(t, imperativeErrors, "unexpected imperative validation errors")
}

// validateExpectedImperativeError validates that the expected imperative error is present.
func validateExpectedImperativeError(t *testing.T, errors []domain.ValidationError, wantErrCode expectedReason, wantErrMessage string) {
	t.Helper()

	found := false

	for _, validationErr := range errors {
		if reasonOf(validationErr) == wantErrCode {
			found = true

			if wantErrMessage != "" {
				check.Contains(t, validationErr.Message, wantErrMessage)
			}

			break
		}
	}

	check.True(t, found, "expected error code %s not found", wantErrCode)
}

func TestSubjectRule_FindFirstLetter(t *testing.T) {
	t.Parallel()

	rule := NewSubjectRule(settings.Config{}, false)

	tcs := []struct {
		name           string
		input          string
		expectedRune   rune
		expectedString string
		expectError    bool
		expectedErr    error
	}{
		{
			name:           "Normal text with first letter",
			input:          subjectAddNewFeature,
			expectedRune:   'a',
			expectedString: subjectAddNewFeature,
			expectError:    false,
		},
		{
			name:           "Text starting with number",
			input:          "123 add feature",
			expectedRune:   'a',
			expectedString: "123 add feature",
			expectError:    false,
		},
		{
			name:           "Text starting with symbol",
			input:          "!@# fix bug",
			expectedRune:   'f',
			expectedString: "!@# fix bug",
			expectError:    false,
		},
		{
			name:        "Empty string",
			input:       "",
			expectError: true,
			expectedErr: ErrInvalidFormat,
		},
		{
			name:        "Whitespace only",
			input:       "   \t  ",
			expectError: true,
			expectedErr: ErrInvalidFormat,
		},
		{
			name:        "No letters at all",
			input:       "123 !@# $%^",
			expectError: true,
			expectedErr: ErrNoLetterFound,
		},
		{
			name:           "Unicode characters",
			input:          "λ function implementation",
			expectedRune:   'λ',
			expectedString: "λ function implementation",
			expectError:    false,
		},
		{
			name:           "Text with leading whitespace",
			input:          "  \t  fix issue  ",
			expectedRune:   'f',
			expectedString: "fix issue",
			expectError:    false,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actualRune, actualString, err := rule.findFirstLetter(testCase.input)

			if testCase.expectError {
				check.Error(t, err, "should fail when no letters found in input")

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr)
				}
			} else {
				check.NoError(t, err)
				check.Equal(t, testCase.expectedRune, actualRune)
				check.Equal(t, testCase.expectedString, actualString)
			}
		})
	}
}

// TestSubjectRule_ValidateImperative tests the validateImperative method
// Note: Imperative validation is complex and tested separately in ImperativeValidator tests.
func TestSubjectRule_ValidateImperative(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name              string
		requireImperative bool
		subject           string
		expectedNoErrors  bool
	}{
		{
			name:              "Imperative not required",
			requireImperative: false,
			subject:           "added new feature",
			expectedNoErrors:  true,
		},
		{
			name:              "Empty subject",
			requireImperative: true,
			subject:           "",
			expectedNoErrors:  true,
		},
		{
			name:              "Whitespace only subject",
			requireImperative: true,
			subject:           "   \t  ",
			expectedNoErrors:  true,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := SubjectRule{
				requireImperative:   testCase.requireImperative,
				imperativeValidator: nil, // Tested in imperative_test.go
			}

			errors := rule.validateImperative(testCase.subject)

			if testCase.expectedNoErrors {
				check.Empty(t, errors)
			}
		})
	}
}

func TestSubjectRule_IsCaseValid(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name          string
		caseChoice    string
		firstLetter   rune
		expectedValid bool
	}{
		{
			name:          "Lower case required - valid",
			caseChoice:    CaseLower,
			firstLetter:   'f',
			expectedValid: true,
		},
		{
			name:          "Lower case required - invalid uppercase",
			caseChoice:    CaseLower,
			firstLetter:   'F',
			expectedValid: false,
		},
		{
			name:          "Upper case required - valid",
			caseChoice:    CaseUpper,
			firstLetter:   'F',
			expectedValid: true,
		},
		{
			name:          "Upper case required - invalid lowercase",
			caseChoice:    CaseUpper,
			firstLetter:   'f',
			expectedValid: false,
		},
		{
			name:          "Sentence case - uppercase valid",
			caseChoice:    CaseSentence,
			firstLetter:   'F',
			expectedValid: true,
		},
		{
			name:          "Sentence case - lowercase invalid",
			caseChoice:    CaseSentence,
			firstLetter:   'f',
			expectedValid: false,
		},
		{
			name:          "Any case - lowercase allowed",
			caseChoice:    CaseAny,
			firstLetter:   'f',
			expectedValid: true,
		},
		{
			name:          "Any case - uppercase allowed",
			caseChoice:    CaseAny,
			firstLetter:   'F',
			expectedValid: true,
		},

		{
			name:          "Non-letter character - not valid for lowercase check",
			caseChoice:    CaseLower,
			firstLetter:   '1',
			expectedValid: false,
		},
		{
			name:          "Unknown case config - defaults to false",
			caseChoice:    "unknown",
			firstLetter:   'f',
			expectedValid: false,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := SubjectRule{caseChoice: testCase.caseChoice}
			valid := rule.isCaseValid(testCase.firstLetter)

			check.Equal(t, testCase.expectedValid, valid)
		})
	}
}

func TestSubjectRule_ValidateSuffix(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name               string
		subject            string
		invalidSuffixes    string
		expectedErrorCount int
		expectedErrorCode  expectedReason
	}{
		{
			name:               "No forbidden endings",
			subject:            subjectAddNewFeature + ".",
			invalidSuffixes:    "",
			expectedErrorCount: 0,
		},
		{
			name:               "Subject without forbidden ending",
			subject:            subjectAddNewFeature,
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 0,
		},
		{
			name:               "Subject with forbidden period",
			subject:            subjectAddNewFeature + ".",
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 1,
			expectedErrorCode:  diagnostics.ReasonSubjectSuffix,
		},
		{
			name:               "Subject with forbidden exclamation",
			subject:            subjectAddNewFeature + "!",
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 1,
			expectedErrorCode:  diagnostics.ReasonSubjectSuffix,
		},
		{
			name:               "Subject with forbidden question mark",
			subject:            subjectAddNewFeature + "?",
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 1,
			expectedErrorCode:  diagnostics.ReasonSubjectSuffix,
		},
		{
			name:               "Empty subject - no validation needed",
			subject:            "",
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 1, // Empty string triggers UTF-8 error
			expectedErrorCode:  diagnostics.ReasonInvalidUTF8,
		},
		{
			name:               "Invalid UTF-8 subject",
			subject:            string([]byte{'f', 'e', 'a', 't', ':', ' ', 0xff}),
			invalidSuffixes:    subjectInvalidSuffixesASCII,
			expectedErrorCount: 1,
			expectedErrorCode:  diagnostics.ReasonInvalidUTF8,
		},
		{
			name:               "UTF-8 forbidden ending",
			subject:            subjectAddNewFeature + "…",
			invalidSuffixes:    "…。",
			expectedErrorCount: 1,
			expectedErrorCode:  diagnostics.ReasonSubjectSuffix,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := SubjectRule{invalidSuffixes: testCase.invalidSuffixes}
			errors := rule.validateSuffix(testCase.subject)

			check.Len(t, errors, testCase.expectedErrorCount)

			if testCase.expectedErrorCount > 0 && testCase.expectedErrorCode != "" {
				check.Equal(t, testCase.expectedErrorCode, reasonOf(errors[0]))
			}
		})
	}
}

func TestSubjectRule_ExtractFirstLetterWithConventionalHandling(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name           string
		subject        string
		checkCommit    bool
		expectedRune   rune
		expectedString string
		expectError    bool
		expectedErr    error
	}{
		{
			name:           "Simple subject without conventional processing",
			subject:        subjectAddNewFeature,
			checkCommit:    false,
			expectedRune:   'a',
			expectedString: subjectAddNewFeature,
			expectError:    false,
		},
		{
			name:           "Conventional commit with checkCommit enabled",
			subject:        subjectFeatAddNewFeature,
			checkCommit:    true,
			expectedRune:   'a',
			expectedString: subjectAddNewFeature,
			expectError:    false,
		},
		{
			name:           "Conventional commit with checkCommit disabled",
			subject:        subjectFeatAddNewFeature,
			checkCommit:    false,
			expectedRune:   'f',
			expectedString: subjectFeatAddNewFeature,
			expectError:    false,
		},
		{
			name:           "Conventional with scope",
			subject:        "feat(auth): add login functionality",
			checkCommit:    true,
			expectedRune:   'a',
			expectedString: "add login functionality",
			expectError:    false,
		},
		{
			name:           "Conventional with breaking change",
			subject:        "feat!: add breaking change",
			checkCommit:    true,
			expectedRune:   'a',
			expectedString: "add breaking change",
			expectError:    false,
		},
		{
			name:           "Conventional with scope and breaking change",
			subject:        "feat(api)!: add breaking API change",
			checkCommit:    true,
			expectedRune:   'a',
			expectedString: "add breaking API change",
			expectError:    false,
		},
		{
			name:        "Empty subject",
			subject:     "",
			checkCommit: false,
			expectError: true,
			expectedErr: ErrSubjectEmpty,
		},
		{
			name:        "Only conventional prefix, no description",
			subject:     "feat:",
			checkCommit: true,
			expectError: true,
			expectedErr: ErrMissingSubjectAfterType,
		},
		{
			name:        "Conventional prefix with only whitespace description",
			subject:     "feat:   \t  ",
			checkCommit: true,
			expectError: true,
			expectedErr: ErrMissingSubjectAfterType,
		},
		{
			name:           "Description starts with number",
			subject:        "fix: 123 error code handling",
			checkCommit:    true,
			expectedRune:   'e',
			expectedString: "123 error code handling",
			expectError:    false,
		},
		{
			name:        "Description with no letters",
			subject:     "fix: 123 !@# $%^",
			checkCommit: true,
			expectError: true,
			expectedErr: ErrNoLetterFound,
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := SubjectRule{isConventionalFormat: testCase.checkCommit}
			actualRune, actualString, err := rule.extractFirstLetterWithConventionalHandling(testCase.subject)

			if testCase.expectError {
				check.Error(t, err, "expected extraction to fail for: %s", testCase.subject)

				if testCase.expectedErr != nil {
					check.ErrorIs(t, err, testCase.expectedErr)
				}
			} else {
				check.NoError(t, err)
				check.Equal(t, testCase.expectedRune, actualRune)
				check.Equal(t, testCase.expectedString, actualString)
			}
		})
	}
}

func TestSubjectRule_ActiveChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         settings.Config
		conventional   bool
		expectedChecks []string
	}{
		{
			name: "all_checks_active",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(50),
						Case:              new(CaseLower),
						ForbidEndings:     []string{"."},
						RequireImperative: new(true),
					},
				},
			},
			conventional:   true,
			expectedChecks: []string{activeCheckLength, activeCheckCase, activeCheckSuffix, activeCheckImperative},
		},
		{
			name: "length_and_suffix_when_case_any_no_imperative",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(72),
						Case:              new(CaseAny),
						ForbidEndings:     []string{},
						RequireImperative: new(false),
					},
				},
			},
			// Empty ForbidEndings falls back to defaults, so suffix is always active
			expectedChecks: []string{activeCheckLength, activeCheckSuffix},
		},
		{
			name: "length_and_case_with_default_suffixes",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						Case: new(CaseSentence),
					},
				},
			},
			expectedChecks: []string{activeCheckLength, activeCheckCase, activeCheckSuffix},
		},
		{
			name: "length_case_suffix_imperative_with_empty_forbid",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						Case:              new(CaseLower),
						ForbidEndings:     []string{},
						RequireImperative: new(true),
					},
				},
			},
			conventional: true,
			// Empty ForbidEndings falls back to defaults, so suffix is always active
			expectedChecks: []string{activeCheckLength, activeCheckCase, activeCheckSuffix, activeCheckImperative},
		},
		{
			name:           "defaults_include_length_case_suffix",
			config:         settings.Config{},
			conventional:   true,
			expectedChecks: []string{activeCheckLength, activeCheckCase, activeCheckSuffix},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := NewSubjectRule(testCase.config, testCase.conventional)
			checks := rule.ActiveChecks()
			check.Equal(t, testCase.expectedChecks, checks)
		})
	}
}

func TestSubjectRule_ImplementsCheckDescriber(t *testing.T) {
	t.Parallel()

	rule := NewSubjectRule(settings.Config{}, false)

	// Verify interface compliance at the type level
	var commitRule domain.CommitRule = rule

	describer, ok := commitRule.(domain.CheckDescriber)
	check.True(t, ok, "SubjectRule should implement CheckDescriber")
	check.NotEmpty(t, describer.ActiveChecks())
}
