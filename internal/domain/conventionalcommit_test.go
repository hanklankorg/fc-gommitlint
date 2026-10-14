// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestParseConventionalCommit_ClassifiesEachSubjectShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		subject string
		want    ConventionalCommitFormat
	}{
		{
			name:    "simple feat commit",
			subject: testSubjectFeatAddNewFeature,
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      nil,
				Breaking:    false,
				Description: testConventionalDescriptionAddNewFeature,
				RawScope:    "",
				IsValid:     true,
			},
		},
		{
			name:    "feat with scope",
			subject: "feat(auth): add login functionality",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      []string{"auth"},
				Breaking:    false,
				Description: "add login functionality",
				RawScope:    "auth",
				IsValid:     true,
			},
		},
		{
			name:    "breaking change",
			subject: "feat(api)!: change auth endpoint structure",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      []string{testConventionalScopeAPI},
				Breaking:    true,
				Description: "change auth endpoint structure",
				RawScope:    testConventionalScopeAPI,
				IsValid:     true,
			},
		},
		{
			name:    "multiple scopes",
			subject: "feat(ui,api): add login functionality",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      []string{"ui", testConventionalScopeAPI},
				Breaking:    false,
				Description: "add login functionality",
				RawScope:    "ui,api",
				IsValid:     true,
			},
		},
		{
			name:    "not conventional - missing colon",
			subject: "feat add new feature",
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "not conventional - no type",
			subject: testSubjectAddNewFeature,
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "not conventional - random text",
			subject: "not conventional commit",
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "malformed conventional - missing closing paren",
			subject: "feat(scope: missing closing paren",
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "invalid uppercase type",
			subject: "Feat: add feature",
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "invalid numeric type",
			subject: "fix2: repair parser",
			want: ConventionalCommitFormat{
				IsValid: false,
			},
		},
		{
			name:    "valid structure - no space after colon",
			subject: "feat:",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      nil,
				Breaking:    false,
				Description: "",
				RawScope:    "",
				IsValid:     true,
			},
		},
		{
			name:    "valid structure - empty description",
			subject: "feat: ",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      nil,
				Breaking:    false,
				Description: "",
				RawScope:    "",
				IsValid:     true,
			},
		},
		{
			// The description feeds max_description_length, so the whitespace
			// around it must not count.
			name:    "description is trimmed of surrounding whitespace",
			subject: "feat:  add x ",
			want: ConventionalCommitFormat{
				Type:        testConventionalTypeFeat,
				Scopes:      nil,
				Breaking:    false,
				Description: "add x",
				RawScope:    "",
				IsValid:     true,
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ParseConventionalCommit(testCase.subject)
			check.Equal(t, testCase.want, result)
		})
	}
}

func TestIsConventionalCommitLike_DetectsAttempts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		subject      string
		allowedTypes []string
		want         bool
	}{
		{
			name:    "valid conventional commit",
			subject: testSubjectFeatAddNewFeature,
			want:    true,
		},
		{
			name:    "valid with scope",
			subject: "feat(auth): add login",
			want:    true,
		},
		{
			name:    "malformed - missing closing paren",
			subject: "feat(scope: missing closing paren",
			want:    true,
		},
		{
			name:    "malformed - missing colon",
			subject: "feat add new feature",
			want:    true,
		},
		{
			name:    "not conventional at all",
			subject: "random commit message",
			want:    false,
		},
		{
			name:    "not conventional - starts with add",
			subject: testSubjectAddNewFeature,
			want:    false,
		},
		{
			name:         "custom type prefix with custom allowed types",
			subject:      "wip something",
			allowedTypes: []string{"wip"},
			want:         true,
		},
		{
			name:    "custom type prefix with the default types",
			subject: "wip something",
			want:    false,
		},
		{
			name:         "default type prefix is not in the custom allowed types",
			subject:      "feat add new feature",
			allowedTypes: []string{"wip"},
			want:         false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := IsConventionalCommitLike(testCase.subject, testCase.allowedTypes)
			check.Equal(t, testCase.want, result)
		})
	}
}
