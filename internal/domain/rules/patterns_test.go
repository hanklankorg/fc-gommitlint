// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSignOffPattern_MatchesOnlyStrictDCOFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Valid DCO sign-off formats
		{
			name:  "valid_standard_format",
			input: "Signed-off-by: John Doe <john@example.com>",
			want:  true,
		},
		{
			name:  "valid_with_middle_name",
			input: "Signed-off-by: John Michael Doe <john.doe@example.com>",
			want:  true,
		},
		{
			name:  "valid_hyphenated_name",
			input: "Signed-off-by: Mary-Jane Smith <mary-jane@example.com>",
			want:  true,
		},
		{
			name:  "valid_unicode_name",
			input: "Signed-off-by: Tanaka Hiroshi <tanaka@example.jp>",
			want:  true,
		},
		{
			name:  "valid_complex_email",
			input: "Signed-off-by: Dev User <dev.user+tag@sub.domain.co.uk>",
			want:  true,
		},
		{
			name:  "valid_single_name",
			input: "Signed-off-by: Mononym <mono@example.com>",
			want:  true,
		},
		// Invalid formats
		{
			name:  "invalid_missing_email",
			input: "Signed-off-by: John Doe",
			want:  false,
		},
		{
			name:  "invalid_missing_name",
			input: "Signed-off-by: <john@example.com>",
			want:  false,
		},
		{
			name:  "invalid_no_space_before_email",
			input: "Signed-off-by: JohnDoe<john@example.com>",
			want:  false,
		},
		{
			name:  "invalid_no_angle_brackets",
			input: "Signed-off-by: John Doe john@example.com",
			want:  false,
		},
		{
			name:  "invalid_missing_at_symbol",
			input: "Signed-off-by: John Doe <johnexample.com>",
			want:  false,
		},
		{
			name:  "invalid_empty_angle_brackets",
			input: "Signed-off-by: John Doe <>",
			want:  false,
		},
		{
			name:  "invalid_lowercase",
			input: "signed-off-by: John Doe <john@example.com>",
			want:  false,
		},
		{
			name:  "invalid_mixed_case",
			input: "Signed-Off-By: John Doe <john@example.com>",
			want:  false,
		},
		{
			name:  "invalid_no_space_after_colon",
			input: "Signed-off-by:John Doe <john@example.com>",
			want:  false,
		},
		{
			name:  "invalid_partial_match_embedded",
			input: "Some text Signed-off-by: John Doe <john@example.com>",
			want:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := rules.SignOffPattern.MatchString(testCase.input)
			check.Equal(t, testCase.want, got)
		})
	}
}

func TestEmailPattern_CapturesBracketedAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantEmail string
	}{
		{
			name:      "simple_email",
			input:     "<john@example.com>",
			wantMatch: true,
			wantEmail: testEmailJohn,
		},
		{
			name:      "complex_email",
			input:     "<dev.user+tag@sub.domain.co.uk>",
			wantMatch: true,
			wantEmail: "dev.user+tag@sub.domain.co.uk",
		},
		{
			name:      "email_in_signoff",
			input:     "Signed-off-by: John <" + testEmailJohn + ">",
			wantMatch: true,
			wantEmail: testEmailJohn,
		},
		{
			name:      "no_brackets",
			input:     testEmailJohn,
			wantMatch: false,
			wantEmail: "",
		},
		{
			name:      "empty_brackets",
			input:     "<>",
			wantMatch: false,
			wantEmail: "",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			matches := rules.EmailPattern.FindStringSubmatch(testCase.input)

			if !testCase.wantMatch {
				check.Nil(t, matches)

				return
			}

			check.NotNil(t, matches)
			check.Len(t, matches, 2)
			check.Equal(t, testCase.wantEmail, matches[1])
		})
	}
}

func TestTrailerPattern_AcceptsKnownTrailersOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// Sign-off style trailers
		{
			name:  "valid_signoff",
			input: "Signed-off-by: John Doe <john@example.com>",
			want:  true,
		},
		{
			name:  "valid_coauthored",
			input: "Co-authored-by: Jane Smith <jane@example.com>",
			want:  true,
		},
		{
			name:  "valid_reviewed",
			input: "Reviewed-by: Reviewer Name <reviewer@example.com>",
			want:  true,
		},
		{
			name:  "valid_acked",
			input: "Acked-by: Maintainer <maintainer@example.com>",
			want:  true,
		},
		{
			name:  "valid_tested",
			input: "Tested-by: QA Engineer <qa@example.com>",
			want:  true,
		},
		{
			name:  "valid_reported",
			input: "Reported-by: Bug Reporter <reporter@example.com>",
			want:  true,
		},
		// Reference style trailers
		{
			name:  "valid_fixes_issue_number",
			input: "Fixes: #123",
			want:  true,
		},
		{
			name:  "valid_closes_issue_number",
			input: "Closes: #456",
			want:  true,
		},
		{
			name:  "valid_resolves_issue_number",
			input: "Resolves: #789",
			want:  true,
		},
		{
			name:  "valid_fixes_url",
			input: "Fixes: https://github.com/owner/repo/issues/123",
			want:  true,
		},
		{
			name:  "valid_closes_url_with_path",
			input: "Closes: https://jira.example.com/browse/PROJ-123",
			want:  true,
		},
		// Invalid trailers
		{
			name:  "invalid_unknown_trailer",
			input: "Custom-by: Someone <someone@example.com>",
			want:  false,
		},
		{
			name:  "invalid_signoff_no_email",
			input: "Signed-off-by: John Doe",
			want:  false,
		},
		{
			name:  "invalid_fixes_no_number",
			input: "Fixes: issue",
			want:  false,
		},
		{
			name:  "invalid_lowercase_trailer",
			input: "signed-off-by: John Doe <john@example.com>",
			want:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := rules.TrailerPattern.MatchString(testCase.input)
			check.Equal(t, testCase.want, got)
		})
	}
}
