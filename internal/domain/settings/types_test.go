// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestConfig_JSONMarshaling(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config settings.Config
	}{
		{
			name:   "empty config",
			config: settings.Config{},
		},
		{
			name: "full config",
			config: settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength:         new(72),
						Case:              new("lower"),
						RequireImperative: new(true),
						ForbidEndings:     []string{".", "!"},
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
				CryptoSignature: settings.CryptoSignatureConfig{
					Required:       new(true),
					VerifyFormat:   new(true),
					KeyDirectory:   "/test/fake/keys",
					AllowedSigners: []string{"user@example.com"},
				},
				Identity: settings.IdentityConfig{
					AllowedAuthors: []string{"john@example.com", "*@company.com"},
				},
				Repo: settings.RepoConfig{
					MaxCommitsAhead: new(settings.DefaultMaxCommitsAhead),
				},
				Jira: settings.JiraConfig{
					ProjectPrefixes:      []string{settingsTestProjectPROJ, "TEAM"},
					RequireInBody:        new(true),
					RequireInSubject:     new(false),
					IgnoreTicketPatterns: []string{"IGNORE-*"},
				},
				Spell: settings.SpellConfig{
					IgnoreWords: []string{"gommitlint", "config"},
					Locale:      new("en_US"),
				},
				Rules: settings.RulesConfig{
					Enabled:  []string{rulecatalog.Subject, rulecatalog.Conventional},
					Disabled: []string{rulecatalog.Spell},
				},
				Output: settingsTestOutputJSON,
				Theme:  settingsTestThemeDracula,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Test marshaling
			data, err := json.Marshal(testCase.config)
			check.NoError(t, err)
			check.NotEmpty(t, data)

			// Test unmarshaling
			var unmarshaled settings.Config

			err = json.Unmarshal(data, &unmarshaled)
			check.NoError(t, err)

			// Verify round-trip preservation
			check.Equal(t, testCase.config, unmarshaled)
		})
	}
}

func TestConfig_StructTags(t *testing.T) {
	t.Parallel()

	// Test that struct tags are properly defined for different formats
	cfg := settings.Config{
		Output: "json",
		Theme:  "dracula",
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{
				MaxLength: new(72),
				Case:      new("lower"),
			},
		},
		Rules: settings.RulesConfig{
			Enabled:  []string{"subject"},
			Disabled: []string{"spell"},
		},
		Checks: settings.ChecksConfig{
			Enabled:  []string{testCheckIDGL102},
			Disabled: []string{testCheckIDGL901},
		},
	}

	// Test JSON marshaling uses correct field names
	data, err := json.Marshal(cfg)
	check.NoError(t, err)

	jsonStr := string(data)
	check.Contains(t, jsonStr, fmt.Sprintf(`"output":"%s"`, "json"))
	check.Contains(t, jsonStr, `"theme":"dracula"`)
	check.Contains(t, jsonStr, `"message":{`)
	check.Contains(t, jsonStr, `"subject":{`)
	check.Contains(t, jsonStr, `"max_length":72`)
	check.Contains(t, jsonStr, `"case":"lower"`)
	check.Contains(t, jsonStr, `"rules":{`)
	check.Contains(t, jsonStr, `"enabled":["subject"]`)
	check.Contains(t, jsonStr, `"disabled":["spell"]`)
	check.Contains(t, jsonStr, `"checks":{`)
	check.Contains(t, jsonStr, `"enabled":["`+testCheckIDGL102+`"]`)
	check.Contains(t, jsonStr, `"disabled":["`+testCheckIDGL901+`"]`)
}

func TestConfig_Sanitized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   settings.Config
		expected settings.Config
	}{
		{
			name: "config with few ignore commits unchanged",
			config: settings.Config{
				IgnoreCommits: []string{"abc123", "def456", "ghi789"},
			},
			expected: settings.Config{
				IgnoreCommits: []string{"abc123", "def456", "ghi789"},
			},
		},
		{
			name: "config with many ignore commits gets masked",
			config: settings.Config{
				IgnoreCommits: []string{
					"commit1", "commit2", "commit3", "commit4", "commit5",
					"commit6", "commit7", "commit8", "commit9", "commit10",
					"commit11", "commit12", "commit13", "commit14", "commit15",
				},
			},
			expected: settings.Config{
				IgnoreCommits: []string{
					"commit1", "commit2", "commit3", "commit4", "commit5",
					"... 5 more ...",
					"commit11", "commit12", "commit13", "commit14", "commit15",
				},
			},
		},
		{
			name:     "empty config unchanged",
			config:   settings.Config{},
			expected: settings.Config{},
		},
		{
			name: "config with exactly 10 commits unchanged",
			config: settings.Config{
				IgnoreCommits: []string{
					"c1", "c2", "c3", "c4", "c5",
					"c6", "c7", "c8", "c9", "c10",
				},
			},
			expected: settings.Config{
				IgnoreCommits: []string{
					"c1", "c2", "c3", "c4", "c5",
					"c6", "c7", "c8", "c9", "c10",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sanitized := tt.config.Sanitized()
			check.Equal(t, tt.expected.IgnoreCommits, sanitized.IgnoreCommits)

			// Verify other fields are unchanged
			check.Equal(t, tt.config.Message, sanitized.Message)
			check.Equal(t, tt.config.Conventional, sanitized.Conventional)
			check.Equal(t, tt.config.Rules, sanitized.Rules)
		})
	}
}

func TestConfig_Sanitized_ManagedSources(t *testing.T) {
	t.Parallel()

	original := settings.Config{ManagedSources: []settings.ManagedSource{{
		ID:     "team-policy",
		Source: "https://source-user:source-pass@example.test:8443/policy.yaml?source-token=secret#source-fragment",
		Headers: map[string]string{
			"Authorization": "Bearer secret",
			"X-Token":       "${POLICY_TOKEN}",
		},
		Resolved: settings.ManagedSourceResolved{
			FinalURL: "https://final-user:final-pass@example.test/final.yaml?final-token=secret#final-fragment",
			Revision: "release-1",
		},
	}}}

	sanitized := original.Sanitized()
	check.Equal(t, "https://example.test:8443/policy.yaml", sanitized.ManagedSources[0].Source)
	check.Equal(t, "https://example.test/final.yaml", sanitized.ManagedSources[0].Resolved.FinalURL)
	check.Equal(t, "[REDACTED]", sanitized.ManagedSources[0].Headers["Authorization"])
	check.Equal(t, "${POLICY_TOKEN}", sanitized.ManagedSources[0].Headers["X-Token"])
	check.Equal(t, "team-policy", sanitized.ManagedSources[0].ID)
	check.Equal(t, "release-1", sanitized.ManagedSources[0].Resolved.Revision)

	sanitized.ManagedSources[0].Headers["Authorization"] = "changed"
	sanitized.ManagedSources[0].ID = "changed"
	check.Equal(t, "Bearer secret", original.ManagedSources[0].Headers["Authorization"])
	check.Equal(t, "team-policy", original.ManagedSources[0].ID)
}

func TestSanitizeManagedSourceURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		rawURL string
		want   string
	}{
		{name: "userinfo query and fragment", rawURL: "https://user:token@example.test:8443/policy.yaml?token=secret#private", want: "https://example.test:8443/policy.yaml"},
		{name: "URL without metadata", rawURL: "https://example.test/policy.yaml", want: "https://example.test/policy.yaml"},
		{name: "environment reference", rawURL: "${POLICY_URL}", want: "[REDACTED]"},
		{name: "empty metadata", rawURL: "", want: "[REDACTED]"},
		{name: "bare token", rawURL: "private-token", want: "[REDACTED]"},
		{name: "relative path", rawURL: "../policy.yaml", want: "[REDACTED]"},
		{name: "missing host", rawURL: "https:///policy.yaml", want: "[REDACTED]"},
		{name: "unsupported scheme", rawURL: "http://example.test/policy.yaml", want: "[REDACTED]"},
		{name: "malformed URL", rawURL: "https://user:secret@example.test/%zz", want: "[REDACTED]"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			check.Equal(t, testCase.want, settings.SanitizeManagedSourceURL(testCase.rawURL))
		})
	}
}

func TestConfig_Sanitized_PreservesOnlyConfiguredSourcePlaceholder(t *testing.T) {
	t.Parallel()

	sanitized := (settings.Config{ManagedSources: []settings.ManagedSource{{
		Source: "${POLICY_URL}",
		Resolved: settings.ManagedSourceResolved{
			FinalURL: "${RESOLVED_POLICY_URL}",
		},
	}}}).Sanitized()

	check.Equal(t, "${POLICY_URL}", sanitized.ManagedSources[0].Source)
	check.Equal(t, "[REDACTED]", sanitized.ManagedSources[0].Resolved.FinalURL)
}

func TestConfig_Sanitized_DeepCopiesReferences(t *testing.T) {
	t.Parallel()

	var original settings.Config
	check.NoError(t, json.Unmarshal([]byte(`{
  "allow_fixup_commits": true,
  "checks": {"disabled":["GL001"],"enabled":["GL002"]},
  "conventional": {"allow_multi_scope":true,"disallow_breaking":true,"max_description_length":72,"require_scope":true,"scopes":["api"],"types":["feat"]},
  "crypto_signature": {"allowed_signers":["a@example.com"],"min_ec_bits":256,"min_rsa_bits":2048,"required":true,"require_gpg":true,"require_ssh":false,"verify_format":true},
  "extends": ["base.yaml"],
  "identity": {"allowed_authors":["a@example.com"]},
  "ignore_commits": ["abc123"],
  "jira": {"ignore_ticket_patterns":["^TMP"],"project_prefixes":["PROJ"],"require_in_body":true,"require_in_subject":true},
  "limits": {"max_commit_count":2000,"max_key_files":200},
  "managed_sources": [{"headers":{"Authorization":"secret"},"id":"policy","source":"https://example.test/policy.yaml"}],
  "message": {"body":{"allow_signoff_only":true,"max_line_length":72,"min_length":1,"min_signoff_count":1,"required":true},"subject":{"case":"lower","forbid_endings":["."],"max_length":72,"require_imperative":true}},
  "repo": {"max_commits_ahead":10},
  "rules": {"disabled":["spell"],"enabled":["subject"]},
  "spell": {"ignore_words":["gommitlint"],"locale":"both"}
}`), &original))

	before, err := json.Marshal(original)
	check.NoError(t, err)

	sanitized := original.Sanitized()

	*sanitized.AllowFixupCommits = false
	sanitized.Checks.Disabled[0] = "changed"
	sanitized.Checks.Enabled[0] = "changed"
	*sanitized.Conventional.AllowMultiScope = false
	*sanitized.Conventional.DisallowBreaking = false
	*sanitized.Conventional.MaxDescriptionLength = 1
	*sanitized.Conventional.RequireScope = false
	sanitized.Conventional.Scopes[0] = "changed"
	sanitized.Conventional.Types[0] = "changed"
	sanitized.CryptoSignature.AllowedSigners[0] = "changed"
	*sanitized.CryptoSignature.MinECBits = 1
	*sanitized.CryptoSignature.MinRSABits = 1
	*sanitized.CryptoSignature.Required = false
	*sanitized.CryptoSignature.RequireGPG = false
	*sanitized.CryptoSignature.RequireSSH = true
	*sanitized.CryptoSignature.VerifyFormat = false
	sanitized.Extends[0] = "changed"
	sanitized.Identity.AllowedAuthors[0] = "changed"
	sanitized.IgnoreCommits[0] = "changed"
	sanitized.Jira.IgnoreTicketPatterns[0] = "changed"
	sanitized.Jira.ProjectPrefixes[0] = "changed"
	*sanitized.Jira.RequireInBody = false
	*sanitized.Jira.RequireInSubject = false
	*sanitized.Limits.MaxCommitCount = 1
	*sanitized.Limits.MaxKeyFiles = 1
	sanitized.ManagedSources[0].Headers["Authorization"] = "changed"
	sanitized.ManagedSources[0].ID = "changed"
	*sanitized.Message.Body.AllowSignoffOnly = false
	*sanitized.Message.Body.MaxLineLength = 1
	*sanitized.Message.Body.MinLength = 2
	*sanitized.Message.Body.MinSignoffCount = 2
	*sanitized.Message.Body.Required = false
	*sanitized.Message.Subject.Case = "upper"
	sanitized.Message.Subject.ForbidEndings[0] = "changed"
	*sanitized.Message.Subject.MaxLength = 1
	*sanitized.Message.Subject.RequireImperative = false
	*sanitized.Repo.MaxCommitsAhead = 1
	sanitized.Rules.Disabled[0] = "changed"
	sanitized.Rules.Enabled[0] = "changed"
	sanitized.Spell.IgnoreWords[0] = "changed"
	*sanitized.Spell.Locale = "US"

	after, err := json.Marshal(original)
	check.NoError(t, err)
	check.Equal(t, before, after)
}

func TestConfig_Sanitized_MalformedURLsFailClosed(t *testing.T) {
	t.Parallel()

	const secret = "super-secret"

	original := settings.Config{ManagedSources: []settings.ManagedSource{{
		Source: "https://user:" + secret + "@example.test/%zz",
		Resolved: settings.ManagedSourceResolved{
			FinalURL: "https://redirect:" + secret + "@example.test/%zz",
		},
	}}}

	sanitized := original.Sanitized()
	check.Equal(t, "[REDACTED]", sanitized.ManagedSources[0].Source)
	check.Equal(t, "[REDACTED]", sanitized.ManagedSources[0].Resolved.FinalURL)
	check.NotContains(t, sanitized.ManagedSources[0].Source, secret)
	check.NotContains(t, sanitized.ManagedSources[0].Resolved.FinalURL, secret)
}
