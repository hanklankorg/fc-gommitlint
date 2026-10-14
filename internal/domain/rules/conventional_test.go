// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestConventionalCommitRule_BasicValidation(t *testing.T) {
	t.Parallel()

	// Default config - breaking changes allowed (DisallowBreaking: false is default)
	cfg := settings.Config{}

	tests := []struct {
		name       string
		commit     domain.Commit
		wantErrors bool
	}{
		// Valid conventional commits using fixtures
		{name: "standard feat", commit: rules.TestCommits.ValidConventional, wantErrors: false},
		{name: "with scope", commit: rules.TestCommits.ValidWithScope, wantErrors: false},
		{name: "breaking change", commit: rules.TestCommits.BreakingChange, wantErrors: false},
		{name: "scope and breaking", commit: rules.CreateTestCommit("123", "feat(api)!: breaking change"), wantErrors: false},
		{name: "all default types", commit: rules.CreateTestCommit("456", "fix: bug fix"), wantErrors: false},

		// Invalid formats using fixtures
		{name: "not conventional", commit: rules.TestCommits.InvalidConventional, wantErrors: true},
		{name: "missing type", commit: rules.CreateTestCommit("789", ": just description"), wantErrors: true},
		{name: "uppercase type", commit: rules.CreateTestCommit("abc", "FEAT: not allowed"), wantErrors: true},
		{name: "numeric type", commit: rules.CreateTestCommit("def", "v1: version tag"), wantErrors: true},
		{name: "no description", commit: rules.TestCommits.EmptyDescription, wantErrors: true},
		{name: "whitespace only desc", commit: rules.CreateTestCommit("ghi", "feat:   "), wantErrors: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewConventionalCommitRule(cfg)
			failures := rule.Validate(testCase.commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestConventionalCommitRule_CustomTypesAndScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subject    string
		types      []string
		scopes     []string
		wantErrors bool
	}{
		// Custom types
		{
			name:       "allowed custom type",
			subject:    "custom: custom type commit",
			types:      []string{"custom", "feat"},
			wantErrors: false,
		},
		{
			name:       "disallowed type",
			subject:    "invalid: not in allowed list",
			types:      []string{"feat", testConventionalFixType},
			wantErrors: true,
		},

		// Custom scopes
		{
			name:       "allowed custom scope",
			subject:    "feat(custom): with custom scope",
			scopes:     []string{"custom", testConventionalAPIScope},
			wantErrors: false,
		},
		{
			name:       "disallowed scope",
			subject:    "feat(invalid): not in allowed list",
			scopes:     []string{testConventionalAPIScope, "auth"},
			wantErrors: true,
		},

		// Empty configs mean use defaults
		{
			name:       "empty types uses defaults",
			subject:    "feat: standard type works",
			types:      []string{},
			wantErrors: false,
		},
		{
			name:       "no scope restriction",
			subject:    "feat(anything): any scope with empty config",
			scopes:     []string{},
			wantErrors: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			cfg := settings.Config{
				Conventional: settings.ConventionalConfig{
					Types:  testCase.types,
					Scopes: testCase.scopes,
				},
			}

			rule := rules.NewConventionalCommitRule(cfg)
			failures := rule.Validate(commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestConventionalCommit_ScopeSpecialCharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subject    string
		wantErrors bool
	}{
		{name: "hyphenated scope", subject: "feat(api-v2): with hyphens", wantErrors: false},
		{name: "nested scope", subject: "feat(api/v2): with slash", wantErrors: false},
		{name: "underscore scope", subject: "feat(api_v2): with underscore", wantErrors: true}, // underscores not allowed
		{name: "numeric in scope", subject: "feat(v2): numeric allowed in scope", wantErrors: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(settings.Config{})
			failures := rule.Validate(commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestConventionalCommit_MinimalFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subject    string
		wantErrors bool
	}{
		{name: "minimal valid", subject: "fix: x", wantErrors: false},
		{name: "too short type", subject: "f: x", wantErrors: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(settings.Config{})
			failures := rule.Validate(commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestConventionalCommit_SpacingValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subject    string
		wantErrors bool
	}{
		{name: "no space after colon", subject: "feat:no space", wantErrors: true},
		{name: "multiple spaces", subject: "feat:  multiple spaces", wantErrors: true},
		{name: "tab after colon", subject: "feat:\ttab character", wantErrors: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(settings.Config{})
			failures := rule.Validate(commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestConventionalCommitRule_ErrorCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		expectedCode expectedReason
	}{
		{
			name:         "invalid format",
			subject:      "not conventional at all",
			expectedCode: diagnostics.ReasonInvalidConventionalFormat,
		},
		{
			name:         "empty description",
			subject:      testConventionalFeatColon,
			expectedCode: diagnostics.ReasonEmptyConventionalDesc,
		},
		{
			name:         "invalid spacing",
			subject:      "feat:  double space",
			expectedCode: diagnostics.ReasonInvalidSpacing,
		},
		{
			name:         "invalid type",
			subject:      "invalid: not in default list",
			expectedCode: diagnostics.ReasonInvalidConventionalType,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(settings.Config{})
			failures := rule.Validate(commit)

			check.NotEmpty(t, failures)
			check.Equal(t, testCase.expectedCode, reasonOf(failures[0]))
		})
	}
}

// A tab after the required single space is extra whitespace, like a second
// space; the "tab after colon" row above only reaches the missing-space check.
func TestConventionalCommitRule_TabAfterTheSpaceIsInvalidSpacing(t *testing.T) {
	t.Parallel()

	failures := rules.NewConventionalCommitRule(settings.Config{}).Validate(minimalCommit("feat: \tadd x"))

	check.Len(t, failures, 1)
	check.Equal(t, diagnostics.ReasonInvalidSpacing, reasonOf(failures[0]))
}

func TestConventionalCommitRule_RequireScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		requireScope bool
		wantReasons  []expectedReason
	}{
		{name: "required_and_missing", subject: "feat: add x", requireScope: true, wantReasons: []expectedReason{diagnostics.ReasonMissingConventionalScope}},
		{name: "required_and_present", subject: "feat(api): add x", requireScope: true},
		{name: "not_required_and_missing", subject: "feat: add x", requireScope: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewConventionalCommitRule(settings.Config{
				Conventional: settings.ConventionalConfig{RequireScope: new(testCase.requireScope)},
			})

			var reasons []expectedReason
			for _, failure := range rule.Validate(minimalCommit(testCase.subject)) {
				reasons = append(reasons, reasonOf(failure))
			}

			check.Equal(t, testCase.wantReasons, reasons)
		})
	}
}

// The description limit is a character count, as the subject and body limits
// are: one multibyte description must not measure differently per rule.
func TestConventionalCommitRule_DescriptionLengthCountsCharacters(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{}
	cfg.Conventional.MaxDescriptionLength = new(72)
	rule := rules.NewConventionalCommitRule(cfg)

	// 72 characters, 143 bytes: counting bytes would reject this description.
	atLimit := strings.Repeat("ä", 36) + " " + strings.Repeat("ö", 35)
	check.Empty(t, rule.Validate(minimalCommit("feat: "+atLimit)))

	failures := rule.Validate(minimalCommit("feat: " + atLimit + "ö"))
	check.NotEmpty(t, failures)
	check.Equal(t, diagnostics.ReasonConventionalDescTooLong, reasonOf(failures[0]))
	check.Equal(t, "73", failures[0].Context[diagnostics.ContextActual])
}

func TestConventionalCommitRule_MultilineCommits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		subject    string
		body       string
		wantErrors bool
	}{
		{
			name:       "valid with body",
			subject:    testConventionalFeatAdd,
			body:       "This is a longer description\n\nWith multiple paragraphs",
			wantErrors: false,
		},
		{
			name:       "invalid subject with valid body",
			subject:    "not conventional",
			body:       "Even with a good body",
			wantErrors: true,
		},
		{
			name:       "breaking change in body",
			subject:    testConventionalFeatAdd,
			body:       "BREAKING CHANGE: This breaks the API",
			wantErrors: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := domain.Commit{
				Hash:    "abc123",
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			rule := rules.NewConventionalCommitRule(settings.Config{})
			failures := rule.Validate(commit)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func minimalCommit(subject string) domain.Commit {
	return domain.Commit{
		Hash:    "abc123",
		Subject: subject,
	}
}

func TestConventionalCommitRule_MultiScope(t *testing.T) {
	t.Parallel()

	// Tests for multi-scope validation when AllowMultiScope is enabled
	tests := []struct {
		name          string
		subject       string
		allowedScopes []string
		wantError     bool
		expectedCode  expectedReason
	}{
		// Valid multi-scope formats (when AllowMultiScope is true)
		{
			name:          "valid single scope",
			subject:       testSubjectFeatAPIScope,
			allowedScopes: []string{testConventionalAPIScope, "ui"},
			wantError:     false,
		},
		{
			name:          "valid multi scope",
			subject:       testSubjectFeatUIAPI,
			allowedScopes: []string{"ui", testConventionalAPIScope, "db"},
			wantError:     false,
		},
		{
			name:          "valid three scopes",
			subject:       "fix(auth,user,admin): security fix",
			allowedScopes: []string{"auth", "user", "admin"},
			wantError:     false,
		},

		// Invalid multi-scope formats (parser fails on these)
		{
			name:          "space after comma",
			subject:       "feat(ui, api): bad format",
			allowedScopes: []string{"ui", testConventionalAPIScope},
			wantError:     true,
			expectedCode:  diagnostics.ReasonInvalidConventionalFormat, // Parser can't parse spaces in scope
		},
		{
			name:          "space before comma",
			subject:       "feat(ui ,api): bad format",
			allowedScopes: []string{"ui", testConventionalAPIScope},
			wantError:     true,
			expectedCode:  diagnostics.ReasonInvalidConventionalFormat, // Parser can't parse spaces in scope
		},
		{
			name:         "starts with comma",
			subject:      "feat(,api): bad format",
			wantError:    true,
			expectedCode: diagnostics.ReasonInvalidMultiScope, // Multi-scope validation catches this
		},
		{
			name:         "ends with comma",
			subject:      "feat(api,): bad format",
			wantError:    true,
			expectedCode: diagnostics.ReasonInvalidMultiScope, // Multi-scope validation catches this
		},
		{
			name:         "consecutive commas",
			subject:      "feat(ui,,api): bad format",
			wantError:    true,
			expectedCode: diagnostics.ReasonInvalidMultiScope, // Multi-scope validation catches this
		},

		// Scope validation with multi-scope
		{
			name:          "invalid scope in multi",
			subject:       "feat(ui,invalid): add feature",
			allowedScopes: []string{"ui", testConventionalAPIScope},
			wantError:     true,
			expectedCode:  diagnostics.ReasonInvalidConventionalScope,
		},
		{
			name:          "all scopes must be valid",
			subject:       "feat(ui,api,invalid): add feature",
			allowedScopes: []string{"ui", testConventionalAPIScope},
			wantError:     true,
			expectedCode:  diagnostics.ReasonInvalidConventionalScope,
		},
		{
			name:          "no scope restriction",
			subject:       "feat(anything,goes): when no scopes defined",
			allowedScopes: []string{}, // Empty allows any scope
			wantError:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := minimalCommit(testCase.subject)
			cfg := settings.Config{
				Conventional: settings.ConventionalConfig{
					AllowMultiScope: new(true), // Enable multi-scope for these tests
					Scopes:          testCase.allowedScopes,
				},
			}

			rule := rules.NewConventionalCommitRule(cfg)
			failures := rule.Validate(commit)

			if testCase.wantError {
				check.NotEmpty(t, failures, "expected validation error")

				if testCase.expectedCode != "" {
					check.Equal(t, testCase.expectedCode, reasonOf(failures[0]),
						"expected error code %s, got %s", testCase.expectedCode, reasonOf(failures[0]))
				}
			} else {
				check.Empty(t, failures, "unexpected validation error: %v", failures)
			}
		})
	}
}

func TestConventionalCommitRule_BreakingChangeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		subject          string
		disallowBreaking bool
		expectError      bool
		expectedErrCode  expectedReason
	}{
		{
			name:             "breaking_allowed_with_indicator",
			subject:          "feat!: breaking change",
			disallowBreaking: false,
			expectError:      false,
		},
		{
			name:             "breaking_allowed_with_scope",
			subject:          "feat(api)!: breaking change",
			disallowBreaking: false,
			expectError:      false,
		},
		{
			name:             "breaking_disallowed_rejects_indicator",
			subject:          "feat!: breaking change",
			disallowBreaking: true,
			expectError:      true,
			expectedErrCode:  diagnostics.ReasonBreakingChangeNotAllowed,
		},
		{
			name:             "breaking_disallowed_rejects_scoped_indicator",
			subject:          "feat(api)!: breaking change",
			disallowBreaking: true,
			expectError:      true,
			expectedErrCode:  diagnostics.ReasonBreakingChangeNotAllowed,
		},
		{
			name:             "breaking_disallowed_accepts_normal_commit",
			subject:          "feat: normal change",
			disallowBreaking: true,
			expectError:      false,
		},
		{
			name:             "breaking_disallowed_accepts_scoped_commit",
			subject:          "feat(api): normal change",
			disallowBreaking: true,
			expectError:      false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Conventional: settings.ConventionalConfig{
					DisallowBreaking: new(testCase.disallowBreaking),
				},
			}

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(cfg)
			errors := rule.Validate(commit)

			if testCase.expectError {
				check.NotEmpty(t, errors, "expected validation error")
				check.Equal(t, testCase.expectedErrCode, reasonOf(errors[0]),
					"expected error code %s, got %s", testCase.expectedErrCode, reasonOf(errors[0]))
			} else {
				check.Empty(t, errors, "expected no validation errors")
			}
		})
	}
}

func TestConventionalCommitRule_AllowMultiScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		subject         string
		allowMultiScope bool
		expectError     bool
		expectedErrCode expectedReason
	}{
		{
			name:            "default_single_scope_allowed",
			subject:         testSubjectFeatAPIScope,
			allowMultiScope: false,
			expectError:     false,
		},
		{
			name:            "default_rejects_multiple_scopes",
			subject:         testSubjectFeatUIAPI,
			allowMultiScope: false,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMultiScopeNotAllowed,
		},
		{
			name:            "default_rejects_three_scopes",
			subject:         "fix(auth,user,admin): security fix",
			allowMultiScope: false,
			expectError:     true,
			expectedErrCode: diagnostics.ReasonMultiScopeNotAllowed,
		},
		{
			name:            "default_accepts_no_scope",
			subject:         testConventionalFeatAdd,
			allowMultiScope: false,
			expectError:     false,
		},
		{
			name:            "allowed_single_scope",
			subject:         testSubjectFeatAPIScope,
			allowMultiScope: true,
			expectError:     false,
		},
		{
			name:            "allowed_multiple_scopes",
			subject:         testSubjectFeatUIAPI,
			allowMultiScope: true,
			expectError:     false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Conventional: settings.ConventionalConfig{
					AllowMultiScope: new(testCase.allowMultiScope),
				},
			}

			commit := minimalCommit(testCase.subject)
			rule := rules.NewConventionalCommitRule(cfg)
			errors := rule.Validate(commit)

			if testCase.expectError {
				check.NotEmpty(t, errors, "expected validation error")
				check.Equal(t, testCase.expectedErrCode, reasonOf(errors[0]),
					"expected error code %s, got %s", testCase.expectedErrCode, reasonOf(errors[0]))
			} else {
				check.Empty(t, errors, "expected no validation errors")
			}
		})
	}
}

func TestConventionalCommitRule_MultipleScopeErrors(t *testing.T) {
	t.Parallel()

	t.Run("reports_multiple_invalid_scopes_at_once", func(t *testing.T) {
		t.Parallel()

		cfg := settings.Config{
			Conventional: settings.ConventionalConfig{
				Scopes:          []string{testConventionalAPIScope, "cli"},
				AllowMultiScope: new(true),
			},
		}

		// Two invalid scopes should produce two errors
		commit := minimalCommit("feat(invalid1,invalid2): add feature")
		rule := rules.NewConventionalCommitRule(cfg)
		errors := rule.Validate(commit)

		check.Len(t, errors, 2, "should report both invalid scopes")
		check.Equal(t, diagnostics.ReasonInvalidConventionalScope, reasonOf(errors[0]))
		check.Equal(t, diagnostics.ReasonInvalidConventionalScope, reasonOf(errors[1]))
		check.Contains(t, errors[0].Message, "invalid1")
		check.Contains(t, errors[1].Message, "invalid2")
	})

	t.Run("three_invalid_scopes_reported_together", func(t *testing.T) {
		t.Parallel()

		cfg := settings.Config{
			Conventional: settings.ConventionalConfig{
				Scopes:          []string{testConventionalAPIScope, "cli", "web"},
				AllowMultiScope: new(true),
			},
		}

		// Three invalid scopes should produce three errors
		commit := minimalCommit("feat(bad1,bad2,bad3): add feature")
		rule := rules.NewConventionalCommitRule(cfg)
		errors := rule.Validate(commit)

		check.Len(t, errors, 3, "should report all three invalid scopes")

		for _, err := range errors {
			check.Equal(t, diagnostics.ReasonInvalidConventionalScope, reasonOf(err))
		}
	})
}

// TestConventionalCommitRule_EmptyScopeSegmentIsRefusedInBothModes pins that
// a comma with nothing on one side is malformed multi-scope syntax whether or
// not multiple scopes are allowed; the parser drops the empty segment, so
// without this check the default mode saw a single valid scope.
func TestConventionalCommitRule_EmptyScopeSegmentIsRefusedInBothModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		subject         string
		allowMultiScope bool
		wantReasons     []expectedReason
	}{
		{name: "trailing_comma_multi_scope_off", subject: "feat(api,): add x", wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "leading_comma_multi_scope_off", subject: "feat(,api): add x", wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "lone_comma_multi_scope_off", subject: "feat(,): add x", wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "trailing_comma_multi_scope_on", subject: "feat(api,): add x", allowMultiScope: true, wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "leading_comma_multi_scope_on", subject: "feat(,api): add x", allowMultiScope: true, wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "lone_comma_multi_scope_on", subject: "feat(,): add x", allowMultiScope: true, wantReasons: []expectedReason{diagnostics.ReasonInvalidMultiScope}},
		{name: "two_scopes_multi_scope_off_is_the_existing_refusal", subject: testSubjectFeatUIAPI, wantReasons: []expectedReason{diagnostics.ReasonMultiScopeNotAllowed}},
		{name: "two_scopes_multi_scope_on_pass", subject: testSubjectFeatUIAPI, allowMultiScope: true},
		{name: "single_scope_multi_scope_off_pass", subject: testSubjectFeatAPIScope},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Conventional: settings.ConventionalConfig{
					AllowMultiScope: new(testCase.allowMultiScope),
				},
			}

			errors := rules.NewConventionalCommitRule(cfg).Validate(minimalCommit(testCase.subject))

			var reasons []expectedReason
			for _, err := range errors {
				reasons = append(reasons, reasonOf(err))
			}

			check.Equal(t, testCase.wantReasons, reasons)
		})
	}
}
