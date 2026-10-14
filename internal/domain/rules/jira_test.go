// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// createJiraTestCommit creates a test commit with default values.
func createJiraTestCommit() domain.Commit {
	return domain.Commit{
		Hash:          "abc123def456",
		Subject:       "feat: add new feature",
		Message:       "feat: add new feature\n\nThis commit adds a new feature that enhances the user experience.",
		Body:          "This commit adds a new feature that enhances the user experience.",
		Author:        "Test User",
		AuthorEmail:   "test@example.com",
		CommitDate:    time.Now().Format(time.RFC3339),
		IsMergeCommit: false,
	}
}

func TestJiraReferenceRule_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commit      domain.Commit
		configSetup func() settings.Config
		wantErrors  bool
		description string
	}{
		{
			name: "jira required in subject",
			configSetup: func() settings.Config {
				return settings.Config{}
			},
			commit: func() domain.Commit {
				commit := createJiraTestCommit()
				commit.Subject = "Add new feature PROJ-123"
				commit.Body = testJiraBodyDescription

				return commit
			}(),
			wantErrors:  false,
			description: "Should pass with valid Jira reference in subject",
		},
		{
			name: "jira required in body with body ref enabled",
			configSetup: func() settings.Config {
				return settings.Config{
					Jira: settings.JiraConfig{
						RequireInBody: new(true),
					},
				}
			},
			commit: func() domain.Commit {
				commit := createJiraTestCommit()
				commit.Subject = testSubjectAddNewFeature
				commit.Body = testJiraBodyDescription + "\n\nRefs: PROJ-123"

				return commit
			}(),
			wantErrors:  false,
			description: "Should pass with valid Jira reference in body",
		},
		{
			name: "jira required but missing",
			configSetup: func() settings.Config {
				return settings.Config{}
			},
			commit: func() domain.Commit {
				commit := createJiraTestCommit()
				commit.Subject = testSubjectAddNewFeature
				commit.Body = testJiraBodyDescription

				return commit
			}(),
			wantErrors:  true,
			description: "Should fail when Jira is required but missing",
		},
		{
			name: "jira with conventional and valid projects",
			configSetup: func() settings.Config {
				return settings.Config{
					Jira: settings.JiraConfig{
						ProjectPrefixes: []string{testJiraProjectPROJ, "TEST"},
					},
				}
			},
			commit: func() domain.Commit {
				commit := createJiraTestCommit()
				commit.Subject = "feat: add login feature PROJ-123"
				commit.Body = testJiraBodyDescription

				return commit
			}(),
			wantErrors:  false,
			description: "Should pass with conventional format and valid project",
		},
		{
			name: "jira with conventional and invalid project",
			configSetup: func() settings.Config {
				return settings.Config{
					Jira: settings.JiraConfig{
						ProjectPrefixes: []string{testJiraProjectPROJ, "TEST"},
					},
				}
			},
			commit: func() domain.Commit {
				commit := createJiraTestCommit()
				commit.Subject = "feat: add login feature OTHER-123"
				commit.Body = testJiraBodyDescription

				return commit
			}(),
			wantErrors:  true,
			description: "Should fail with conventional format and invalid project",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Setup config
			cfg := testCase.configSetup()

			// Create rule with configuration
			// For this test, assume conventional commit is enabled
			cfg.Rules.Enabled = []string{rulecatalog.Conventional}

			rule := rules.NewJiraReferenceRule(cfg, false)

			// Execute
			failures := rule.Validate(testCase.commit)

			// Assert
			if testCase.wantErrors {
				check.NotEmpty(t, failures, "Expected validation errors but got none")
			} else {
				check.Empty(t, failures, "Expected no validation errors but got: %v", failures)
			}
		})
	}
}

// TestJiraReferenceRule_ConfigurationValidation tests the configuration validation functionality.

// TestJiraReferenceRule_ErrorHandling tests error handling with specific error codes.

// TestJiraReferenceRule_ProjectPrefixValidation tests project prefix validation functionality.
func TestJiraReferenceRule_ProjectPrefixValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		subject         string
		body            string
		projectPrefixes []string
		wantErrCount    int
		wantErrCode     expectedReason
	}{
		{
			name:            "Valid single project",
			subject:         testJiraSubjectFeatPROJ,
			body:            "",
			projectPrefixes: []string{testJiraProjectPROJ},
			wantErrCount:    0,
		},
		{
			name:            "Valid multiple projects",
			subject:         "feat: add feature PROJ-123 TEAM-456",
			body:            "",
			projectPrefixes: []string{testJiraProjectPROJ, testJiraProjectTEAM},
			wantErrCount:    0,
		},
		{
			// An unlisted project on its own is not a reference at all, so the
			// presence check fails before the prefix check can name it.
			name:            "Unlisted project alone is no reference",
			subject:         "feat: add feature OTHER-123",
			body:            "",
			projectPrefixes: []string{testJiraProjectPROJ, testJiraProjectTEAM},
			wantErrCount:    1,
			wantErrCode:     diagnostics.ReasonMissingJira,
		},
		{
			name:            "Mixed valid and invalid projects",
			subject:         "feat: add feature PROJ-123 OTHER-456",
			body:            "",
			projectPrefixes: []string{testJiraProjectPROJ, testJiraProjectTEAM},
			wantErrCount:    1,
			wantErrCode:     diagnostics.ReasonInvalidProject,
		},
		{
			name:            "No project prefixes - accept all",
			subject:         "feat: add feature ANYTHING-123",
			body:            "",
			projectPrefixes: []string{},
			wantErrCount:    0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: testCase.projectPrefixes,
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			}
		})
	}
}

// TestJiraReferenceRule_ConventionalCommitFormat tests conventional commit format.
func TestJiraReferenceRule_ConventionalCommitFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		body         string
		wantErrCount int
		wantErrCode  expectedReason
	}{
		{
			name:         "Valid conventional with JIRA at end",
			subject:      "feat: add new feature PROJ-123",
			body:         testJiraDescription,
			wantErrCount: 0,
		},
		{
			name:         "Valid conventional with JIRA in scope",
			subject:      "feat(PROJ-123): add new feature",
			body:         testJiraDescription,
			wantErrCount: 0,
		},
		{
			name:         "Invalid - JIRA in middle of description",
			subject:      "feat: add PROJ-123 new feature",
			body:         testJiraDescription,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonJiraKeyNotAtEnd,
		},
		{
			// The rule is built with isConventionalEnabled=false here, so the
			// excluded-type path is not what makes this row pass: the key is at
			// the end. The exclusion itself is covered by
			// TestJiraReferenceRule_ExcludedTypesSkipTheRuleOnlyInConventionalMode.
			name:         "Valid - docs type with JIRA at end",
			subject:      "docs: update README PROJ-123",
			body:         testJiraDescription,
			wantErrCount: 0,
		},
		{
			name:         "Valid - merge type with JIRA at end",
			subject:      "merge: merge feature branch PROJ-123",
			body:         testJiraDescription,
			wantErrCount: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes:  []string{testJiraProjectPROJ},
					RequireInSubject: new(true),
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, "unexpected number of errors")

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			}
		})
	}
}

// TestJiraReferenceRule_ExcludedTypesSkipTheRuleOnlyInConventionalMode covers
// the docs/chore/style/refactor/test exclusion, which every other Jira test
// leaves unreached by building the rule with isConventionalEnabled=false.
func TestJiraReferenceRule_ExcludedTypesSkipTheRuleOnlyInConventionalMode(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Jira: settings.JiraConfig{
			ProjectPrefixes:  []string{testJiraProjectPROJ},
			RequireInSubject: new(true),
		},
		Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
	}

	for _, testCase := range []struct {
		name                string
		subject             string
		conventionalEnabled bool
		wantReasons         []expectedReason
	}{
		{
			name:                "an excluded type needs no reference",
			subject:             "docs: update readme",
			conventionalEnabled: true,
			wantReasons:         []expectedReason{},
		},
		{
			name:                "a type outside the exclusion still needs one",
			subject:             "feat: add feature",
			conventionalEnabled: true,
			wantReasons:         []expectedReason{diagnostics.ReasonMissingJiraKeySubject},
		},
		{
			name:        "the control: the same excluded type outside conventional mode",
			subject:     "docs: update readme",
			wantReasons: []expectedReason{diagnostics.ReasonMissingJiraKeySubject},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := rules.NewJiraReferenceRule(cfg, testCase.conventionalEnabled).
				Validate(domain.Commit{Subject: testCase.subject, Body: testJiraDescription})

			reasons := make([]expectedReason, 0, len(errors))
			for _, err := range errors {
				reasons = append(reasons, reasonOf(err))
			}

			check.Equal(t, testCase.wantReasons, reasons)
		})
	}
}

// TestJiraReferenceRule_CoreBehavior tests core behavior.
func TestJiraReferenceRule_CoreBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		body         string
		config       settings.Config
		wantErrCount int
		description  string
	}{
		{
			name:    "Original behavior - JIRA in subject",
			subject: "Add new feature PROJ-123",
			body:    testJiraDescription,
			config: settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: []string{testJiraProjectPROJ},
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			},
			wantErrCount: 0,
			description:  "Basic JIRA validation should work as before",
		},
		{
			name:    "Original behavior - missing JIRA",
			subject: testSubjectAddNewFeature,
			body:    testJiraDescription,
			config: settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: []string{testJiraProjectPROJ},
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			},
			wantErrCount: 1,
			description:  "Missing JIRA should still fail as before",
		},
		{
			name:    "Original behavior - invalid project",
			subject: "Add new feature OTHER-123",
			body:    testJiraDescription,
			config: settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: []string{testJiraProjectPROJ},
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			},
			wantErrCount: 1,
			description:  "Invalid project should still fail as before",
		},
		{
			name:    "Original behavior - body search enabled",
			subject: testSubjectAddNewFeature,
			body:    "Description\n\nRefs: PROJ-123",
			config: settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: []string{testJiraProjectPROJ},
					RequireInBody:   new(true),
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			},
			wantErrCount: 0,
			description:  "Body search should work as before",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewJiraReferenceRule(testCase.config, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, testCase.description)
		})
	}
}

// TestJiraReferenceRule_SubjectPlacement tests subject placement validation.
func TestJiraReferenceRule_SubjectPlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		wantErrCount int
		wantErrCode  expectedReason
		description  string
	}{
		{
			name:         "Conventional commit - JIRA at end (valid)",
			subject:      "feat: Add authentication system PROJ-123",
			wantErrCount: 0,
			description:  "JIRA key at end of conventional commit should be valid",
		},
		{
			name:         "Conventional commit - JIRA not at end (invalid)",
			subject:      "feat: PROJ-123 add authentication system",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonJiraKeyNotAtEnd,
			description:  "JIRA key not at end should trigger specific error",
		},
		{
			name:         "Conventional commit - multiple JIRA at end (valid)",
			subject:      "feat: Add feature PROJ-123 TEAM-456",
			wantErrCount: 0,
			description:  "Multiple JIRA keys at end should be valid",
		},
		{
			name:         "Non-conventional commit - JIRA anywhere (valid)",
			subject:      "PROJ-123 add authentication system",
			wantErrCount: 0,
			description:  "Non-conventional commits can have JIRA anywhere",
		},
		{
			name:         "Conventional commit with scope - JIRA at end (valid)",
			subject:      "feat(auth): Add login functionality PROJ-123",
			wantErrCount: 0,
			description:  "JIRA at end of scoped conventional commit should be valid",
		},
		{
			name:         "Breaking change with JIRA at end (valid)",
			subject:      "feat!: Add breaking API change PROJ-123",
			wantErrCount: 0,
			description:  "Breaking change commits with JIRA at end should be valid",
		},
		{
			name:         "Conventional commit - bracketed JIRA at end (valid)",
			subject:      "feat: add login [PROJ-123]",
			wantErrCount: 0,
			description:  "A closing bracket after the key still counts as the end",
		},
		{
			name:         "Conventional commit - parenthesised JIRA at end (valid)",
			subject:      "feat: add login (PROJ-123)",
			wantErrCount: 0,
			description:  "A closing parenthesis after the key still counts as the end",
		},
		{
			name:         "Conventional commit - bracketed JIRA before the end (invalid)",
			subject:      "feat: add [PROJ-123] login",
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonJiraKeyNotAtEnd,
			description:  "Brackets do not excuse a key that is followed by more text",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes:  []string{testJiraProjectPROJ, testJiraProjectTEAM},
					RequireInSubject: new(true),
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{Subject: testCase.subject}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, testCase.description)

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			}
		})
	}
}

// TestJiraReferenceRule_BodyValidation tests body validation with Refs format.
func TestJiraReferenceRule_BodyValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		body         string
		wantErrCount int
		wantErrCode  expectedReason
		description  string
	}{
		{
			name:    "Valid Refs format - single key",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs: PROJ-123

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 0,
			description:  "Valid Refs format should pass validation",
		},
		{
			name:    "Valid Refs format - multiple keys",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs: PROJ-123, TEAM-456

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 0,
			description:  "Multiple JIRA keys in Refs should be valid",
		},
		{
			name:    "Invalid Refs format - missing colon",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs PROJ-123

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonInvalidRefsFormat,
			description:  "Missing colon should trigger format error",
		},
		{
			name:    "Invalid Refs format - lowercase key",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs: proj-123

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonInvalidKeyFormat,
			description:  "Lowercase JIRA key should trigger format error",
		},
		{
			name:    "Refs after Signed-off-by - invalid ordering",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Signed-off-by: Developer <dev@example.com>
Refs: PROJ-123`,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonRefsAfterSignoff,
			description:  "Refs after Signed-off-by should trigger ordering error",
		},
		{
			name:    "Missing Refs line",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonMissingJiraKeyBody,
			description:  "Missing Refs line should trigger missing key error",
		},
		{
			name:    "Invalid project in Refs",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs: INVALID-123

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 1,
			wantErrCode:  diagnostics.ReasonInvalidProject,
			description:  "Invalid project in Refs should trigger project error",
		},
		{
			name:    "Multiple Refs lines with valid ordering",
			subject: testSubjectFeatUpper,
			body: `This is the commit body.

Refs: PROJ-123
Refs: TEAM-456

Signed-off-by: Developer <dev@example.com>`,
			wantErrCount: 0,
			description:  "Multiple Refs lines should be valid if ordered correctly",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes: []string{testJiraProjectPROJ, testJiraProjectTEAM},
					RequireInBody:   new(true),
				},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, testCase.description)

			if testCase.wantErrCount > 0 {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			}
		})
	}
}

// TestJiraReferenceRule_RichErrorTaxonomy tests that all 8 original error codes are restored.
func TestJiraReferenceRule_RichErrorTaxonomy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		subject     string
		body        string
		config      settings.Config
		wantErrCode expectedReason
		description string
	}{
		{
			name:    "diagnostics.ReasonEmptySubject",
			subject: "",
			body:    "",
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInSubject: new(true)},
			},
			wantErrCode: diagnostics.ReasonEmptySubject,
			description: "Empty subject should trigger specific error",
		},
		{
			name:    "diagnostics.ReasonMissingJiraKeySubject",
			subject: testSubjectFeatUpper,
			body:    "",
			config: settings.Config{
				Jira: settings.JiraConfig{
					RequireInSubject: new(true),
					ProjectPrefixes:  []string{testJiraProjectPROJ},
				},
			},
			wantErrCode: diagnostics.ReasonMissingJiraKeySubject,
			description: "Missing JIRA in subject should trigger specific error",
		},
		{
			name:    "diagnostics.ReasonMissingJiraKeyBody",
			subject: testSubjectFeatUpper,
			body:    "No Refs line here",
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInBody: new(true)},
			},
			wantErrCode: diagnostics.ReasonMissingJiraKeyBody,
			description: "Missing JIRA in body should trigger specific error",
		},
		{
			name:    "diagnostics.ReasonJiraKeyNotAtEnd",
			subject: "feat: PROJ-123 add new feature",
			body:    "",
			config: settings.Config{
				Jira:  settings.JiraConfig{RequireInSubject: new(true)},
				Rules: settings.RulesConfig{Enabled: []string{rulecatalog.Conventional}},
			},
			wantErrCode: diagnostics.ReasonJiraKeyNotAtEnd,
			description: "JIRA not at end in conventional commit should trigger specific error",
		},
		{
			name:    "diagnostics.ReasonInvalidProject",
			subject: "feat: Add new feature INVALID-123",
			body:    "",
			config: settings.Config{
				Jira: settings.JiraConfig{
					RequireInSubject: new(true),
					ProjectPrefixes:  []string{testJiraProjectPROJ},
				},
			},
			wantErrCode: diagnostics.ReasonInvalidProject,
			description: "Invalid project prefix should trigger specific error",
		},
		{
			name:    "diagnostics.ReasonMissingJiraKeyBody for Refs text without a key",
			subject: testSubjectFeatUpper,
			body:    "Refs invalid format without colon",
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInBody: new(true)},
			},
			wantErrCode: diagnostics.ReasonMissingJiraKeyBody,
			description: "Text that looks like Refs but invalid format should trigger missing key error",
		},
		{
			name:    "diagnostics.ReasonInvalidRefsFormat",
			subject: testSubjectFeatUpper,
			body:    "Refs: #123",
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInBody: new(true)},
			},
			wantErrCode: diagnostics.ReasonInvalidRefsFormat,
			description: "Refs line without a JIRA key should trigger refs format error",
		},
		{
			name:    "diagnostics.ReasonInvalidKeyFormat",
			subject: testSubjectFeatUpper,
			body:    "Refs: proj-123",
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInBody: new(true)},
			},
			wantErrCode: diagnostics.ReasonInvalidKeyFormat,
			description: "Refs with invalid key format should trigger key format error",
		},
		{
			name:    "diagnostics.ReasonRefsAfterSignoff",
			subject: testSubjectFeatUpper,
			body: `Signed-off-by: Dev <dev@example.com>
Refs: PROJ-123`,
			config: settings.Config{
				Jira: settings.JiraConfig{RequireInBody: new(true)},
			},
			wantErrCode: diagnostics.ReasonRefsAfterSignoff,
			description: "Refs after Signed-off-by should trigger specific error",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := rules.NewJiraReferenceRule(testCase.config, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, 1, testCase.description)
			check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]), testCase.description)
		})
	}
}

// TestJiraReferenceRule_IgnoreTicketPatterns tests the ignore_ticket_patterns filtering.
func TestJiraReferenceRule_IgnoreTicketPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		subject              string
		body                 string
		ignoreTicketPatterns []string
		projectPrefixes      []string
		wantErrCount         int
		wantErrCode          expectedReason
		description          string
	}{
		{
			name:                 "no_patterns_all_tickets_count",
			subject:              testJiraSubjectFeatPROJ,
			body:                 "",
			ignoreTicketPatterns: []string{},
			projectPrefixes:      []string{testJiraProjectPROJ},
			wantErrCount:         0,
			description:          "With no ignore patterns, all tickets should count",
		},
		{
			name:                 "pattern_filters_matching_ticket",
			subject:              "feat: add feature TEMP-123",
			body:                 "",
			ignoreTicketPatterns: []string{testTempTicketPattern},
			projectPrefixes:      []string{},
			wantErrCount:         1,
			wantErrCode:          diagnostics.ReasonMissingJira,
			description:          "TEMP-123 should be filtered out, leaving no valid tickets",
		},
		{
			name:                 "pattern_keeps_non_matching_ticket",
			subject:              testJiraSubjectFeatPROJ,
			body:                 "",
			ignoreTicketPatterns: []string{testTempTicketPattern},
			projectPrefixes:      []string{testJiraProjectPROJ},
			wantErrCount:         0,
			description:          "PROJ-123 should not be filtered out",
		},
		{
			name:                 "multiple_patterns_filter_multiple_tickets",
			subject:              "feat: add feature TEMP-123 TEST-456",
			body:                 "",
			ignoreTicketPatterns: []string{testTempTicketPattern, "^TEST-.*"},
			projectPrefixes:      []string{},
			wantErrCount:         1,
			wantErrCode:          diagnostics.ReasonMissingJira,
			description:          "Both tickets filtered, leaving none",
		},
		{
			name:                 "mixed_filtered_and_valid",
			subject:              "feat: add feature TEMP-123 PROJ-456",
			body:                 "",
			ignoreTicketPatterns: []string{testTempTicketPattern},
			projectPrefixes:      []string{testJiraProjectPROJ},
			wantErrCount:         0,
			description:          "TEMP-123 filtered, PROJ-456 remains valid",
		},
		{
			name:                 "pattern_in_body",
			subject:              testConventionalFeatAdd,
			body:                 "Description\n\nRefs: TEMP-123",
			ignoreTicketPatterns: []string{testTempTicketPattern},
			projectPrefixes:      []string{},
			wantErrCount:         1,
			wantErrCode:          diagnostics.ReasonMissingJira,
			description:          "Body ticket also filtered by pattern",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					IgnoreTicketPatterns: testCase.ignoreTicketPatterns,
					ProjectPrefixes:      testCase.projectPrefixes,
				},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{
				Subject: testCase.subject,
				Body:    testCase.body,
			}

			errors := rule.Validate(commit)

			check.Len(t, errors, testCase.wantErrCount, testCase.description)

			if testCase.wantErrCount > 0 && testCase.wantErrCode != "" {
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			}
		})
	}
}

// TestJiraReferenceRule_InvalidIgnorePattern tests that invalid regex patterns are rejected.
func TestJiraReferenceRule_InvalidIgnorePattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		ignoreTicketPatterns []string
		wantErrCount         int
		wantErrCode          expectedReason
	}{
		{
			name:                 "valid_pattern",
			ignoreTicketPatterns: []string{testTempTicketPattern},
			wantErrCount:         0,
		},
		{
			name:                 "invalid_pattern_unclosed_bracket",
			ignoreTicketPatterns: []string{"^TEMP-["},
			wantErrCount:         1,
			wantErrCode:          diagnostics.ReasonInvalidConfig,
		},
		{
			name:                 "invalid_pattern_bad_quantifier",
			ignoreTicketPatterns: []string{"^TEMP-*"},
			wantErrCount:         0, // This is actually valid regex (zero or more '-')
		},
		{
			name:                 "mixed_valid_invalid",
			ignoreTicketPatterns: []string{"^VALID-.*", "^INVALID-["},
			wantErrCount:         1,
			wantErrCode:          diagnostics.ReasonInvalidConfig,
		},
		{
			name:                 "multiple_invalid",
			ignoreTicketPatterns: []string{"^BAD-[", "^ALSO-["},
			wantErrCount:         2,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					IgnoreTicketPatterns: testCase.ignoreTicketPatterns,
				},
			}

			rule := rules.NewJiraReferenceRule(cfg, false)
			commit := domain.Commit{
				Subject: testJiraSubjectFeatPROJ,
				Body:    "",
			}

			errors := rule.Validate(commit)

			if testCase.wantErrCount == 0 {
				// Should not have config errors (may have other errors)
				for _, err := range errors {
					check.NotEqual(t, diagnostics.ReasonInvalidConfig, reasonOf(err),
						"Should not have config error")
				}
			} else {
				check.GreaterOrEqual(t, len(errors), testCase.wantErrCount)

				if testCase.wantErrCode != "" {
					check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
					check.Contains(t, errors[0].Context["pattern"], "[",
						"Error should contain the invalid pattern")
				}
			}
		})
	}
}

// TestJiraReferenceRule_InvalidProjectReportedOnce pins that each disallowed,
// non-ignored project key yields exactly one invalid_project diagnostic,
// whether or not the subject requirement also inspects the subject's keys.
func TestJiraReferenceRule_InvalidProjectReportedOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		subject        string
		conventional   bool
		ignorePatterns []string
		wantActuals    []string
	}{
		{
			name:        "non_conventional_disallowed_project",
			subject:     "Add feature OTHER-123",
			wantActuals: []string{"OTHER"},
		},
		{
			name:        "non_conventional_allowed_project",
			subject:     "Add feature PROJ-123",
			wantActuals: []string{},
		},
		{
			name:           "non_conventional_ignored_disallowed_project",
			subject:        "Add feature TEMP-9 PROJ-123",
			ignorePatterns: []string{testTempTicketPattern},
			wantActuals:    []string{},
		},
		{
			name:         "conventional_disallowed_project",
			subject:      "feat: add feature OTHER-123",
			conventional: true,
			wantActuals:  []string{"OTHER"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					RequireInSubject:     new(true),
					ProjectPrefixes:      []string{testJiraProjectPROJ},
					IgnoreTicketPatterns: testCase.ignorePatterns,
				},
			}

			rule := rules.NewJiraReferenceRule(cfg, testCase.conventional)
			errors := rule.Validate(domain.Commit{Subject: testCase.subject})

			actuals := []string{}

			for _, validationErr := range errors {
				if reasonOf(validationErr) == diagnostics.ReasonInvalidProject {
					actuals = append(actuals, validationErr.Context[diagnostics.ContextActual])
				}
			}

			check.Equal(t, testCase.wantActuals, actuals)
			check.Len(t, errors, len(testCase.wantActuals))
		})
	}
}

// TestJiraReferenceRule_KeysMustStandAlone pins that a key is a whole token
// (brackets and parentheses around it are fine, letters glued to it are not)
// and that, once project_prefixes is set, only a listed prefix counts as a
// key for the presence check, so a hash algorithm such as SHA-256 is not a
// reference.
func TestJiraReferenceRule_KeysMustStandAlone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		subject         string
		projectPrefixes []string
		ignorePatterns  []string
		wantReasons     []expectedReason
	}{
		{
			name:            "a hash algorithm is not a key when prefixes are configured",
			subject:         "Support SHA-256 hashing",
			projectPrefixes: []string{testJiraProjectPROJ},
			wantReasons:     []expectedReason{diagnostics.ReasonMissingJira},
		},
		{
			name:        "a key inside a longer token does not count",
			subject:     "feat: add xPROJ-123 support",
			wantReasons: []expectedReason{diagnostics.ReasonMissingJira},
		},
		{
			name:        "a key followed by letters does not count",
			subject:     "feat: add PROJ-123abc",
			wantReasons: []expectedReason{diagnostics.ReasonMissingJira},
		},
		{
			name:            "an unlisted prefix alone is no reference",
			subject:         "feat: add feature OTHER-123",
			projectPrefixes: []string{testJiraProjectPROJ},
			wantReasons:     []expectedReason{diagnostics.ReasonMissingJira},
		},
		{
			name:    "a bare key counts",
			subject: testJiraSubjectFeatPROJ,
		},
		{
			name:            "a bracketed key counts",
			subject:         "feat: add login [PROJ-123]",
			projectPrefixes: []string{testJiraProjectPROJ},
		},
		{
			name:            "a parenthesised key counts",
			subject:         "feat: add login (PROJ-123)",
			projectPrefixes: []string{testJiraProjectPROJ},
		},
		{
			name:            "a key in the scope counts",
			subject:         "feat(PROJ-123): add login",
			projectPrefixes: []string{testJiraProjectPROJ},
		},
		{
			name:            "an ignored key does not count even with a listed prefix",
			subject:         "feat: add TEMP-1",
			projectPrefixes: []string{testJiraProjectPROJ, "TEMP"},
			ignorePatterns:  []string{testTempTicketPattern},
			wantReasons:     []expectedReason{diagnostics.ReasonMissingJira},
		},
		{
			name:            "a listed key next to an ignored one counts",
			subject:         "feat: add TEMP-1 PROJ-2",
			projectPrefixes: []string{testJiraProjectPROJ},
			ignorePatterns:  []string{testTempTicketPattern},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Jira: settings.JiraConfig{
					ProjectPrefixes:      testCase.projectPrefixes,
					IgnoreTicketPatterns: testCase.ignorePatterns,
				},
			}

			errors := rules.NewJiraReferenceRule(cfg, false).Validate(domain.Commit{Subject: testCase.subject})

			var reasons []expectedReason
			for _, err := range errors {
				reasons = append(reasons, reasonOf(err))
			}

			check.Equal(t, testCase.wantReasons, reasons)
		})
	}
}
