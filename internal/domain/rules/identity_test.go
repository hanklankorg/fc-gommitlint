// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// createTestCommit creates a commit with the given author details.
func createTestCommit(authorName, authorEmail string) domain.Commit {
	return domain.Commit{
		Hash:        "abc123def456789012345678901234567890abcd",
		Subject:     "feat: add new feature",
		Message:     "feat: add new feature\n\nThis commit adds a new feature.",
		Body:        "This commit adds a new feature.",
		Author:      authorName,
		AuthorEmail: authorEmail,
	}
}

func TestIdentityRule_Name(t *testing.T) {
	t.Parallel()

	rule := rules.NewIdentityRule(settings.Config{})
	check.Equal(t, rulecatalog.Identity, rule.Name())
}

func TestIdentityRule_EmptyAllowedAuthors(t *testing.T) {
	t.Parallel()

	rule := rules.NewIdentityRule(settings.Config{})
	commit := createTestCommit("Anyone", "anyone@anywhere.com")

	errors := rule.Validate(commit)
	check.Empty(t, errors, "Empty allowed_authors should allow all authors")
}

func TestIdentityRule_EmailMatching(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedAuthors []string
		authorEmail    string
		wantErr        bool
	}{
		{
			name:           "exact_email_match",
			allowedAuthors: []string{testEmailJohn},
			authorEmail:    testEmailJohn,
			wantErr:        false,
		},
		{
			name:           "case_insensitive_email_match",
			allowedAuthors: []string{testEmailJohn},
			authorEmail:    "JOHN@EXAMPLE.COM",
			wantErr:        false,
		},
		{
			name:           "case_insensitive_config",
			allowedAuthors: []string{"JOHN@EXAMPLE.COM"},
			authorEmail:    testEmailJohn,
			wantErr:        false,
		},
		{
			name:           "email_not_in_list",
			allowedAuthors: []string{testEmailJohn},
			authorEmail:    testEmailJane,
			wantErr:        true,
		},
		{
			name:           "multiple_allowed_first_matches",
			allowedAuthors: []string{testEmailJohn, testEmailJane},
			authorEmail:    testEmailJohn,
			wantErr:        false,
		},
		{
			name:           "multiple_allowed_second_matches",
			allowedAuthors: []string{testEmailJohn, testEmailJane},
			authorEmail:    testEmailJane,
			wantErr:        false,
		},
		{
			name:           "plus_addressing",
			allowedAuthors: []string{"john+test@example.com"},
			authorEmail:    "john+test@example.com",
			wantErr:        false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Identity: settings.IdentityConfig{
					AllowedAuthors: testCase.allowedAuthors,
				},
			}
			rule := rules.NewIdentityRule(cfg)
			commit := createTestCommit("Any Name", testCase.authorEmail)

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
				check.Equal(t, diagnostics.ReasonIdentityNotAllowed, reasonOf(errors[0]))
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestIdentityRule_DomainWildcard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedAuthors []string
		authorEmail    string
		wantErr        bool
	}{
		{
			name:           "wildcard_matches_domain",
			allowedAuthors: []string{testCompanyWildcard},
			authorEmail:    "john@company.com",
			wantErr:        false,
		},
		{
			name:           "wildcard_matches_any_user",
			allowedAuthors: []string{testCompanyWildcard},
			authorEmail:    "anyone@company.com",
			wantErr:        false,
		},
		{
			name:           "wildcard_case_insensitive",
			allowedAuthors: []string{testCompanyWildcard},
			authorEmail:    "JOHN@COMPANY.COM",
			wantErr:        false,
		},
		{
			name:           "wildcard_no_match_different_domain",
			allowedAuthors: []string{testCompanyWildcard},
			authorEmail:    "john@other.com",
			wantErr:        true,
		},
		{
			name:           "wildcard_no_match_subdomain",
			allowedAuthors: []string{testCompanyWildcard},
			authorEmail:    "john@sub.company.com",
			wantErr:        true,
		},
		{
			name:           "wildcard_with_exact_email",
			allowedAuthors: []string{"*@company.com", "external@partner.com"},
			authorEmail:    "external@partner.com",
			wantErr:        false,
		},
		{
			name:           "multiple_wildcards",
			allowedAuthors: []string{"*@company.com", "*@subsidiary.com"},
			authorEmail:    "john@subsidiary.com",
			wantErr:        false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Identity: settings.IdentityConfig{
					AllowedAuthors: testCase.allowedAuthors,
				},
			}
			rule := rules.NewIdentityRule(cfg)
			commit := createTestCommit("Any Name", testCase.authorEmail)

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
				check.Equal(t, diagnostics.ReasonIdentityNotAllowed, reasonOf(errors[0]))
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestIdentityRule_InvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedAuthors []string
		wantErrCode    expectedReason
		wantErrMsg     string
	}{
		{
			name:           "name_email_format_rejected",
			allowedAuthors: []string{"John Doe <" + testEmailJohn + ">"},
			wantErrCode:    diagnostics.ReasonInvalidConfig,
			wantErrMsg:     testUseEmailOnlyMessage,
		},
		{
			name:           "angle_brackets_rejected",
			allowedAuthors: []string{"<" + testEmailJohn + ">"},
			wantErrCode:    diagnostics.ReasonInvalidConfig,
			wantErrMsg:     testUseEmailOnlyMessage,
		},
		{
			name:           "no_at_sign_rejected",
			allowedAuthors: []string{"notanemail"},
			wantErrCode:    diagnostics.ReasonInvalidConfig,
			wantErrMsg:     "must be an email",
		},
		{
			name:           "mixed_valid_invalid",
			allowedAuthors: []string{"valid@example.com", "John <invalid@example.com>"},
			wantErrCode:    diagnostics.ReasonInvalidConfig,
			wantErrMsg:     testUseEmailOnlyMessage,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Identity: settings.IdentityConfig{
					AllowedAuthors: testCase.allowedAuthors,
				},
			}
			rule := rules.NewIdentityRule(cfg)
			commit := createTestCommit("Any Name", "any@example.com")

			errors := rule.Validate(commit)

			check.NotEmpty(t, errors)
			check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			check.Contains(t, errors[0].Message, testCase.wantErrMsg)
		})
	}
}

func TestIdentityRule_EmptyAndWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		allowedAuthors []string
		authorEmail    string
		wantErr        bool
	}{
		{
			name:           "empty_entries_ignored",
			allowedAuthors: []string{"", testEmailJohn, "  "},
			authorEmail:    testEmailJohn,
			wantErr:        false,
		},
		{
			name:           "whitespace_trimmed_in_config",
			allowedAuthors: []string{"  " + testEmailJohn + "  "},
			authorEmail:    testEmailJohn,
			wantErr:        false,
		},
		{
			name:           "whitespace_trimmed_in_commit",
			allowedAuthors: []string{testEmailJohn},
			authorEmail:    "  " + testEmailJohn + "  ",
			wantErr:        false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Identity: settings.IdentityConfig{
					AllowedAuthors: testCase.allowedAuthors,
				},
			}
			rule := rules.NewIdentityRule(cfg)
			commit := createTestCommit("Any Name", testCase.authorEmail)

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestIdentityRule_ErrorContext(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Identity: settings.IdentityConfig{
			AllowedAuthors: []string{"allowed@example.com"},
		},
	}
	rule := rules.NewIdentityRule(cfg)
	commit := createTestCommit("Unauthorized User", "unauthorized@example.com")

	errors := rule.Validate(commit)

	check.Len(t, errors, 1)
	check.Equal(t, diagnostics.ReasonIdentityNotAllowed, reasonOf(errors[0]))
	check.Equal(t, "unauthorized@example.com", errors[0].Context["author_email"])
}
