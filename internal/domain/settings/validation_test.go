// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const invalidValue = "invalid"

func requireConfigValidation(t *testing.T, errors []string, wantError bool, contains string) {
	t.Helper()

	if !wantError {
		check.Empty(t, errors)

		return
	}

	check.NotEmpty(t, errors)

	if contains != "" {
		check.Contains(t, strings.Join(errors, " "), contains)
	}
}

func TestConfig_Validate_ConsistentOutput(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()
	cfg.Message.Subject.MaxLength = new(-1) // Make it invalid

	originalMaxLength := cfg.Message.Subject.MaxLength

	// Call validate multiple times
	errors1 := settings.ValidateConfig(cfg)
	errors2 := settings.ValidateConfig(cfg)

	// Results should be identical (deterministic)
	check.Equal(t, errors1, errors2)

	// Config should be unchanged
	check.Equal(t, originalMaxLength, cfg.Message.Subject.MaxLength)
}

func TestConfig_Validate_SubjectMaxLength(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		maxLength int
		wantError bool
	}{
		{"positive length", 72, false},
		{"small positive", 1, false},
		{"large length", 200, false},
		{"zero length", 0, true},
		{"negative length", -1, true},
		{"very negative", -999, true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Message.Subject.MaxLength = new(testCase.maxLength)

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, "subject max_length must be positive")
		})
	}
}

func TestConfig_Validate_ConventionalTypes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		types     []string
		wantError bool
	}{
		{"default types", settings.GetDefaultConventionalTypes(), false},
		{"single type", []string{settingsTestTypeFeat}, false},
		{"custom types", []string{"feature", "bugfix"}, false},
		{"uppercase type", []string{"Feat"}, true},
		{"numeric type", []string{"fix2"}, true},
		{"punctuated type", []string{"breaking-change"}, true},
		{"empty type", []string{""}, true},
		{"empty slice uses defaults", []string{}, false},
		{"nil slice uses defaults", nil, false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Conventional.Types = testCase.types

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, "conventional types contains invalid type")
		})
	}
}

func TestConfig_Validate_RulesConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		enabled       []string
		disabled      []string
		wantError     bool
		errorContains string
	}{
		{
			name:      "valid_rules",
			enabled:   []string{rulecatalog.Subject, rulecatalog.Conventional},
			disabled:  []string{rulecatalog.CommitBody},
			wantError: false,
		},
		{
			name:          "unknown_enabled_rule",
			enabled:       []string{"subjet"},
			wantError:     true,
			errorContains: "rules enabled contains unknown rule",
		},
		{
			name:          "unknown_disabled_rule",
			disabled:      []string{"nope"},
			wantError:     true,
			errorContains: "rules disabled contains unknown rule",
		},
		{
			name:          "empty_rule_name",
			enabled:       []string{" "},
			wantError:     true,
			errorContains: "rules enabled contains empty rule name",
		},
		{
			// Rule names fold case, like the whitespace and quotes around
			// them: rulecatalog.Normalize lower-cases, so this resolves to the
			// canonical ID instead of being reported as an unknown rule.
			name:      "uppercase_rule_is_accepted",
			enabled:   []string{strings.ToUpper(rulecatalog.Subject)},
			wantError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Rules.Enabled = testCase.enabled
			cfg.Rules.Disabled = testCase.disabled

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestConfig_Validate_ChecksConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		enabled       []string
		disabled      []string
		wantError     bool
		errorContains string
	}{
		{
			name:      "valid_checks",
			enabled:   []string{testCheckIDGL102, "gl301"},
			disabled:  []string{testCheckIDGL901},
			wantError: false,
		},
		{
			name:          "dispatcher_check_cannot_be_enabled",
			enabled:       []string{testCheckIDGL901},
			wantError:     true,
			errorContains: "checks enabled contains dispatcher check ID \"GL901\"",
		},
		{
			name:          "dispatcher_runtime_check_cannot_be_disabled",
			disabled:      []string{testCheckIDGL902},
			wantError:     true,
			errorContains: "checks disabled contains dispatcher check ID \"GL902\"",
		},
		{
			name:          "unknown_enabled_check",
			enabled:       []string{"GL000"},
			wantError:     true,
			errorContains: "checks enabled contains unknown check ID",
		},
		{
			name:          "unknown_disabled_check",
			disabled:      []string{"GML102"},
			wantError:     true,
			errorContains: "checks disabled contains unknown check ID",
		},
		{
			name:          "empty_check_id",
			enabled:       []string{" "},
			wantError:     true,
			errorContains: "checks enabled contains empty check ID",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Checks.Enabled = testCase.enabled
			cfg.Checks.Disabled = testCase.disabled

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestValidateManagedSources(t *testing.T) {
	t.Parallel()

	t.Run("accepts direct https url", func(t *testing.T) {
		t.Parallel()

		errors := settings.ValidateManagedSources([]settings.ManagedSource{{
			ID:          settingsTestManagedSourceID,
			Source:      "https://example.test/policy.yaml",
			Destination: settingsTestManagedSourceDestination,
			Integrity:   settings.ManagedSourceIntegrity{SHA256: strings.Repeat("a", 64)},
		}})

		check.Empty(t, errors)
	})

	t.Run("accepts environment source reference", func(t *testing.T) {
		t.Parallel()

		errors := settings.ValidateManagedSources([]settings.ManagedSource{{
			ID:          settingsTestManagedSourceID,
			Source:      "${GOMMITLINT_POLICY_URL}",
			Destination: settingsTestManagedSourceDestination,
			Integrity:   settings.ManagedSourceIntegrity{SHA256: strings.Repeat("a", 64)},
		}})

		check.Empty(t, errors)
	})

	t.Run("rejects invalid source value", func(t *testing.T) {
		t.Parallel()

		errors := settings.ValidateManagedSources([]settings.ManagedSource{{
			ID:          settingsTestManagedSourceID,
			Source:      "not-a-url",
			Destination: settingsTestManagedSourceDestination,
			Integrity:   settings.ManagedSourceIntegrity{SHA256: strings.Repeat("a", 64)},
		}})

		check.Len(t, errors, 1)
		check.Contains(t, errors[0], "source must be a valid https URL")
	})
}

func TestIsValidManagedSourceURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{name: "https_url", rawURL: "https://example.test/policy.yaml", want: true},
		{name: "https_url_with_spaces", rawURL: "  https://example.test/policy.yaml  ", want: true},
		{name: "http_url", rawURL: "http://example.test/policy.yaml", want: false},
		{name: "missing_host", rawURL: "https:///policy.yaml", want: false},
		{name: "not_url", rawURL: "not-a-url", want: false},
		{name: "empty", rawURL: "", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, settings.IsValidManagedSourceURL(testCase.rawURL))
		})
	}
}

func TestIsLocalManagedSourceDestination(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		destination string
		want        bool
	}{
		{name: "relative_file", destination: ".gommitlint.team.yaml", want: true},
		{name: "relative_nested_file", destination: "config/team.yaml", want: true},
		{name: "empty", destination: "", want: false},
		{name: "absolute_path", destination: "/tmp/team.yaml", want: false},
		{name: "parent_path", destination: "../team.yaml", want: false},
		{name: "nested_parent_escape", destination: "config/../../team.yaml", want: false},
		{name: "dotdot_prefixed_file", destination: "..team.yaml", want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, settings.IsLocalManagedSourceDestination(testCase.destination))
		})
	}
}

func TestParseManagedSourceEnvReference(t *testing.T) {
	t.Parallel()

	variable, ok := settings.ParseManagedSourceEnvReference("${GOMMITLINT_POLICY_URL}")
	check.True(t, ok)
	check.Equal(t, "GOMMITLINT_POLICY_URL", variable)

	_, ok = settings.ParseManagedSourceEnvReference("https://example.test/policy.yaml")
	check.False(t, ok)
}

func TestConfig_Validate_JiraConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		config        settings.JiraConfig
		wantError     bool
		errorContains string
	}{
		{
			name: "valid_jira_config",
			config: settings.JiraConfig{
				IgnoreTicketPatterns: []string{"^TEMP-.*"},
				ProjectPrefixes:      []string{settingsTestProjectPROJ},
			},
			wantError: false,
		},
		{
			name: "invalid_regex",
			config: settings.JiraConfig{
				IgnoreTicketPatterns: []string{"["},
			},
			wantError:     true,
			errorContains: "jira ignore_ticket_patterns contains invalid regex",
		},
		{
			name: "empty_prefix",
			config: settings.JiraConfig{
				ProjectPrefixes: []string{" "},
			},
			wantError:     true,
			errorContains: "jira project_prefixes contains empty prefix",
		},
		{
			name: "invalid_prefix",
			config: settings.JiraConfig{
				ProjectPrefixes: []string{"proj"},
			},
			wantError:     true,
			errorContains: "jira project_prefixes contains invalid prefix",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Jira = testCase.config

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestConfig_Validate_IdentityConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		allowed       []string
		wantError     bool
		errorContains string
	}{
		{
			name:      "valid_emails",
			allowed:   []string{"user@example.com", "*@example.com"},
			wantError: false,
		},
		{
			name:          "invalid_brackets",
			allowed:       []string{"User <user@example.com>"},
			wantError:     true,
			errorContains: "identity allowed_authors contains invalid entry",
		},
		{
			name:          "missing_at_sign",
			allowed:       []string{"notanemail"},
			wantError:     true,
			errorContains: "identity allowed_authors contains invalid entry",
		},
		{
			name:      "empty_entries_ignored",
			allowed:   []string{"", "  "},
			wantError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Identity.AllowedAuthors = testCase.allowed

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestConfig_Validate_RepoConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		repo          settings.RepoConfig
		wantError     bool
		errorContains string
	}{
		{
			name: "valid_repo_config",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(5),
			},
			wantError: false,
		},
		{
			name: "negative_max_commits_ahead",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(-1),
			},
			wantError:     true,
			errorContains: "repo max_commits_ahead must be positive",
		},
		{
			name: "zero_max_commits_ahead",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(0),
			},
			wantError:     true,
			errorContains: "repo max_commits_ahead must be positive",
		},
		{
			name: "smallest_positive_max_commits_ahead",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(1),
			},
			wantError: false,
		},
		{
			name: "max_commits_ahead_at_1000",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(1000),
			},
			wantError: false,
		},
		{
			name: "max_commits_ahead_too_high",
			repo: settings.RepoConfig{
				MaxCommitsAhead: new(1001),
			},
			wantError:     true,
			errorContains: "repo max_commits_ahead cannot exceed 1000",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Repo = testCase.repo

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestConfig_Validate_LimitsConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		limits        settings.LimitsConfig
		wantError     bool
		errorContains string
	}{
		{
			name: "default limits",
			limits: settings.LimitsConfig{
				MaxCommitCount: nil,
			},
			wantError: false,
		},
		{
			name: "valid limit",
			limits: settings.LimitsConfig{
				MaxCommitCount: new(3000),
			},
			wantError: false,
		},
		{
			name: "zero max commit count",
			limits: settings.LimitsConfig{
				MaxCommitCount: new(0),
			},
			wantError:     true,
			errorContains: "limits max_commit_count must be positive",
		},
		{
			name: "negative max commit count",
			limits: settings.LimitsConfig{
				MaxCommitCount: new(-5),
			},
			wantError:     true,
			errorContains: "limits max_commit_count must be positive",
		},
		{
			name: "zero max key files",
			limits: settings.LimitsConfig{
				MaxKeyFiles: new(0),
			},
			wantError:     true,
			errorContains: "limits max_key_files must be positive",
		},
		{
			name: "negative max key files",
			limits: settings.LimitsConfig{
				MaxKeyFiles: new(-2),
			},
			wantError:     true,
			errorContains: "limits max_key_files must be positive",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := newBaseConfig()
			cfg.Limits = testCase.limits

			errors := settings.ValidateConfig(cfg)

			requireConfigValidation(t, errors, testCase.wantError, testCase.errorContains)
		})
	}
}

func TestConfig_Validate_MultipleErrors(t *testing.T) {
	t.Parallel()

	cfg := newBaseConfig()
	cfg.Message.Subject.MaxLength = new(0)          // Error 1
	cfg.Conventional.MaxDescriptionLength = new(-1) // Error 2

	errors := settings.ValidateConfig(cfg)

	check.Len(t, errors, 2)
	check.Contains(t, errors[0], "subject max_length must be positive")
	check.Contains(t, errors[1], "conventional max_description_length cannot be negative")
}

func TestConfig_Validate_ValidConfigurations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config settings.Config
	}{
		{
			name:   "default config",
			config: newBaseConfig(),
		},
		{
			name: "minimal valid config",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(1),
					},
				},
				Conventional: settings.ConventionalConfig{
					Types: []string{settingsTestTypeFeat},
				},
				Output: "text",
			},
		},
		{
			name: "fully configured",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(100),
						Case:              new("lower"),
						RequireImperative: new(true),

						ForbidEndings: []string{".", "!"},
					},
					Body: settings.BodyConfig{
						Required:         new(true),
						MinLength:        new(10),
						AllowSignoffOnly: new(false),
						MinSignoffCount:  new(1),
					},
				},
				Conventional: settings.ConventionalConfig{
					DisallowBreaking:     new(false),
					MaxDescriptionLength: new(50),
					RequireScope:         new(true),
					Scopes:               []string{settingsTestScopeAPI, "ui"},
					Types:                []string{settingsTestTypeFeat, settingsTestTypeFix, settingsTestTypeDocs},
				},
				Output: settingsTestOutputJSON,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateConfig(testCase.config)
			check.Empty(t, errors, "Configuration should be valid: %v", errors)
		})
	}
}

func TestConfig_Validate_ErrorMessages(t *testing.T) {
	t.Parallel()

	// Test that error messages are helpful and consistent
	cfg := newBaseConfig()
	cfg.Message.Subject.MaxLength = new(-5)

	errors := settings.ValidateConfig(cfg)
	check.NotEmpty(t, errors)

	errorMessage := errors[0]
	check.Contains(t, errorMessage, "subject")
	check.Contains(t, errorMessage, "max_length")
	check.Contains(t, errorMessage, "positive")
	check.NotContains(t, errorMessage, "nil")
	check.NotContains(t, errorMessage, "panic")
}

func TestConfig_Validate_InputNotModified(t *testing.T) {
	t.Parallel()

	// Test that validation doesn't modify the config
	cfg := settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{
				MaxLength: new(-1),
			},
		},
		Conventional: settings.ConventionalConfig{
			Types: []string{},
		},
		Output: invalidValue,
		Theme:  invalidValue,
	}

	originalMaxLength := cfg.Message.Subject.MaxLength
	originalTypes := cfg.Conventional.Types
	originalOutput := cfg.Output
	originalTheme := cfg.Theme

	errors := settings.ValidateConfig(cfg)
	check.NotEmpty(t, errors)

	// Config should be unchanged
	check.Equal(t, originalMaxLength, cfg.Message.Subject.MaxLength)
	check.Equal(t, originalTypes, cfg.Conventional.Types)
	check.Equal(t, originalOutput, cfg.Output)
	check.Equal(t, originalTheme, cfg.Theme)
}

func TestValidateSubjectConfig_ConsistentOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		subject   settings.SubjectConfig
		wantError bool
	}{
		{
			name:      "valid positive length",
			subject:   settings.SubjectConfig{MaxLength: new(72)},
			wantError: false,
		},
		{
			name:      "invalid zero length",
			subject:   settings.SubjectConfig{MaxLength: new(0)},
			wantError: true,
		},
		{
			name:      "invalid negative length",
			subject:   settings.SubjectConfig{MaxLength: new(-1)},
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateSubjectConfig(testCase.subject)

			if testCase.wantError {
				check.NotEmpty(t, errors)
				check.Contains(t, errors[0], "subject max_length must be positive")
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

// TestValidateSubjectConfig_CaseIsAnEnum covers the subject-case enum check in
// config validation. Every other subject row varies max_length only, so an
// unrecognised case value — the reason the check exists — was never refused by
// a test.
func TestValidateSubjectConfig_CaseIsAnEnum(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		subject   settings.SubjectConfig
		wantError string
	}{
		{
			name:      "unknown case is refused",
			subject:   settings.SubjectConfig{Case: new("ROBOT_MODE")},
			wantError: "subject case must be one of",
		},
		{
			name:    "a recognised case is accepted, whatever its spelling",
			subject: settings.SubjectConfig{Case: new("Upper")},
		},
		{
			name:    "an unset case falls back to the default",
			subject: settings.SubjectConfig{},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateSubjectConfig(testCase.subject)

			if testCase.wantError == "" {
				check.Empty(t, errors)

				return
			}

			check.Len(t, errors, 1)
			check.Contains(t, errors[0], testCase.wantError)
		})
	}
}

func TestValidateConventionalConfig_ConsistentOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		conventional settings.ConventionalConfig
		wantError    bool
	}{
		{
			name:         "valid with types",
			conventional: settings.ConventionalConfig{Types: []string{settingsTestTypeFeat, settingsTestTypeFix}},
			wantError:    false,
		},
		{
			name:         "empty types uses defaults",
			conventional: settings.ConventionalConfig{Types: []string{}},
			wantError:    false,
		},
		{
			name:         "nil types uses defaults",
			conventional: settings.ConventionalConfig{Types: nil},
			wantError:    false,
		},
		{
			name:         "negative max description length",
			conventional: settings.ConventionalConfig{MaxDescriptionLength: new(-1)},
			wantError:    true,
		},
		{
			name:         "max description length at the limit",
			conventional: settings.ConventionalConfig{MaxDescriptionLength: new(200)},
			wantError:    false,
		},
		{
			name:         "max description length exceeds limit",
			conventional: settings.ConventionalConfig{MaxDescriptionLength: new(201)},
			wantError:    true,
		},
		{
			name:         "valid max description length",
			conventional: settings.ConventionalConfig{MaxDescriptionLength: new(100)},
			wantError:    false,
		},
		{
			name:         "zero max description length uses default",
			conventional: settings.ConventionalConfig{MaxDescriptionLength: new(0)},
			wantError:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateConventionalConfig(testCase.conventional)

			if testCase.wantError {
				check.NotEmpty(t, errors)
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestValidateCryptoSignatureConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      settings.CryptoSignatureConfig
		wantError   bool
		errContains string
	}{
		{
			name:      "valid_defaults",
			config:    settings.CryptoSignatureConfig{},
			wantError: false,
		},
		{
			name: "valid_custom_values",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(4096),
				MinECBits:  new(384),
			},
			wantError: false,
		},
		{
			name: "zero_values_use_defaults",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(0),
				MinECBits:  new(0),
			},
			wantError: false,
		},
		{
			name: "positive_values_below_defaults",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(1),
				MinECBits:  new(1),
			},
			wantError: false,
		},
		{
			name: "negative_min_rsa_bits",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(-1),
			},
			wantError:   true,
			errContains: "min_rsa_bits cannot be negative",
		},
		{
			// The documented maximum itself must be accepted: an off-by-one
			// here would refuse the largest RSA size the docs offer.
			name: "min_rsa_bits_at_max",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(8192),
			},
			wantError: false,
		},
		{
			name: "min_rsa_bits_exceeds_max",
			config: settings.CryptoSignatureConfig{
				MinRSABits: new(8193),
			},
			wantError:   true,
			errContains: "min_rsa_bits cannot exceed 8192",
		},
		{
			name: "negative_min_ec_bits",
			config: settings.CryptoSignatureConfig{
				MinECBits: new(-1),
			},
			wantError:   true,
			errContains: "min_ec_bits cannot be negative",
		},
		{
			// 521 is P-521's key size, not a round number: an off-by-one here
			// would reject the strongest supported curve.
			name: "min_ec_bits_at_max",
			config: settings.CryptoSignatureConfig{
				MinECBits: new(521),
			},
			wantError: false,
		},
		{
			name: "min_ec_bits_exceeds_max",
			config: settings.CryptoSignatureConfig{
				MinECBits: new(522),
			},
			wantError:   true,
			errContains: "min_ec_bits cannot exceed 521",
		},
		{
			name: "mutually_exclusive_require_gpg_ssh",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(true),
				RequireSSH: new(true),
			},
			wantError:   true,
			errContains: "mutually exclusive",
		},
		{
			name: "require_gpg_only",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(true),
			},
			wantError: false,
		},
		{
			name: "require_ssh_only",
			config: settings.CryptoSignatureConfig{
				RequireSSH: new(true),
			},
			wantError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateCryptoSignatureConfig(testCase.config)

			if testCase.wantError {
				check.NotEmpty(t, errors)
				check.Contains(t, errors[0], testCase.errContains)
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestValidateConfig_ComposesComponents(t *testing.T) {
	t.Parallel()

	// Test that ValidateConfig properly composes the individual validation functions
	cfg := settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{MaxLength: new(0)}, // Invalid
		},
		Conventional: settings.ConventionalConfig{MaxDescriptionLength: new(-1)}, // Invalid
	}

	errors := settings.ValidateConfig(cfg)
	check.Len(t, errors, 2)

	// Should include errors from all component validators
	check.Contains(t, errors[0], "subject max_length must be positive")
	check.Contains(t, errors[1], "conventional max_description_length cannot be negative")
}

func TestValidateSpellConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      settings.SpellConfig
		wantError   bool
		errContains string
	}{
		{
			name:      "empty_locale_is_valid",
			config:    settings.SpellConfig{Locale: new("")},
			wantError: false,
		},
		{
			name:      "UK_locale",
			config:    settings.SpellConfig{Locale: new("UK")},
			wantError: false,
		},
		{
			name:      "US_locale",
			config:    settings.SpellConfig{Locale: new("US")},
			wantError: false,
		},
		{
			name:      "GB_locale",
			config:    settings.SpellConfig{Locale: new("GB")},
			wantError: false,
		},
		{
			name:      "case_insensitive_uk",
			config:    settings.SpellConfig{Locale: new("uk")},
			wantError: false,
		},
		{
			name:      "case_insensitive_us",
			config:    settings.SpellConfig{Locale: new("us")},
			wantError: false,
		},
		{
			name:        "invalid_locale_fr_FR",
			config:      settings.SpellConfig{Locale: new("fr_FR")},
			wantError:   true,
			errContains: settingsTestSpellLocaleError,
		},
		{
			name:        "invalid_locale_en_GB_format",
			config:      settings.SpellConfig{Locale: new("en_GB")},
			wantError:   true,
			errContains: settingsTestSpellLocaleError,
		},
		{
			name:        "invalid_locale_british",
			config:      settings.SpellConfig{Locale: new("british")},
			wantError:   true,
			errContains: settingsTestSpellLocaleError,
		},
		{
			name:      "with_ignore_words",
			config:    settings.SpellConfig{Locale: new("UK"), IgnoreWords: []string{settingsTestScopeAPI, settingsTestOutputJSON}},
			wantError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateSpellConfig(testCase.config)

			if testCase.wantError {
				check.NotEmpty(t, errors)
				check.Contains(t, errors[0], testCase.errContains)
			} else {
				check.Empty(t, errors)
			}
		})
	}
}

func TestIsValidSpellLocale(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		locale string
		valid  bool
	}{
		{"UK", true},
		{"US", true},
		{"GB", true},
		{"uk", true},
		{"us", true},
		{"gb", true},
		{"Uk", true},
		{"en_GB", false},
		{"en_US", false},
		{"en-gb", false},
		{"british", false},
		{"american", false},
		{"fr_FR", false},
		{invalidValue, false},
		{"", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.locale, func(t *testing.T) {
			t.Parallel()

			result := settings.IsValidSpellLocale(testCase.locale)
			check.Equal(t, testCase.valid, result)
		})
	}
}

func TestGetValidSpellLocales(t *testing.T) {
	t.Parallel()

	locales := settings.GetValidSpellLocales()

	// "both" is the recommended dialect-neutral default; US/UK/GB select a single dialect.
	check.Contains(t, locales, "both")
	check.Contains(t, locales, "US")
	check.Contains(t, locales, "UK")
	check.Contains(t, locales, "GB")
	check.Len(t, locales, 4)
}

func TestValidateBodyConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		config      settings.BodyConfig
		wantError   bool
		errContains string
	}{
		{
			name:      "valid_defaults",
			config:    settings.BodyConfig{},
			wantError: false,
		},
		{
			name: "valid_max_line_length_72",
			config: settings.BodyConfig{
				MaxLineLength: new(72),
			},
			wantError: false,
		},
		{
			name: "valid_max_line_length_100",
			config: settings.BodyConfig{
				MaxLineLength: new(100),
			},
			wantError: false,
		},
		{
			name: "valid_max_line_length_200",
			config: settings.BodyConfig{
				MaxLineLength: new(200),
			},
			wantError: false,
		},
		{
			name: "zero_max_line_length_disables_check",
			config: settings.BodyConfig{
				MaxLineLength: new(0),
			},
			wantError: false,
		},
		{
			name: "negative_max_line_length",
			config: settings.BodyConfig{
				MaxLineLength: new(-1),
			},
			wantError:   true,
			errContains: "max_line_length cannot be negative",
		},
		{
			name: "max_line_length_exceeds_200",
			config: settings.BodyConfig{
				MaxLineLength: new(201),
			},
			wantError:   true,
			errContains: "max_line_length cannot exceed 200",
		},
		{
			name: "valid_min_length",
			config: settings.BodyConfig{
				MinLength: new(50),
			},
			wantError: false,
		},
		{
			name: "negative_min_length",
			config: settings.BodyConfig{
				MinLength: new(-1),
			},
			wantError:   true,
			errContains: "min_length cannot be negative",
		},
		{
			name: "min_length_at_1000",
			config: settings.BodyConfig{
				MinLength: new(1000),
			},
			wantError: false,
		},
		{
			name: "min_length_exceeds_1000",
			config: settings.BodyConfig{
				MinLength: new(1001),
			},
			wantError:   true,
			errContains: "min_length cannot exceed 1000",
		},
		{
			name: "min_signoff_count_at_0",
			config: settings.BodyConfig{
				MinSignoffCount: new(0),
			},
			wantError: false,
		},
		{
			name: "valid_min_signoff_count_boundaries",
			config: settings.BodyConfig{
				MinSignoffCount: new(10),
			},
			wantError: false,
		},
		{
			name: "negative_min_signoff_count",
			config: settings.BodyConfig{
				MinSignoffCount: new(-1),
			},
			wantError:   true,
			errContains: "min_signoff_count cannot be negative",
		},
		{
			name: "min_signoff_count_exceeds_10",
			config: settings.BodyConfig{
				MinSignoffCount: new(11),
			},
			wantError:   true,
			errContains: "min_signoff_count cannot exceed 10",
		},
		{
			name: "valid_combined_settings",
			config: settings.BodyConfig{
				Required:         new(true),
				MinLength:        new(10),
				MaxLineLength:    new(72),
				AllowSignoffOnly: new(false),
				MinSignoffCount:  new(1),
			},
			wantError: false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := settings.ValidateBodyConfig(testCase.config)

			if testCase.wantError {
				check.NotEmpty(t, errors)
				check.Contains(t, errors[0], testCase.errContains)
			} else {
				check.Empty(t, errors)
			}
		})
	}
}
