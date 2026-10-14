// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// invalid_config is shared by several rules; the help must follow the context
// the rule attached, not default to branchahead guidance.
func TestLookupHelpMessage_InvalidConfigFollowsContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ctx      map[string]string
		expected string
	}{
		{
			name:     "identity entry with formats",
			ctx:      map[string]string{diagnostics.ContextInvalidEntry: "Name <a@b.c>", contextValidFormats: "email@domain.com or *@domain.com"},
			expected: "Fix the allowed_authors entry 'Name <a@b.c>' — use email@domain.com or *@domain.com",
		},
		{
			name:     "identity entry without formats",
			ctx:      map[string]string{diagnostics.ContextInvalidEntry: "nobody"},
			expected: "Fix the allowed_authors entry 'nobody'",
		},
		{
			name:     "jira pattern",
			ctx:      map[string]string{diagnostics.ContextPattern: "["},
			expected: "Ensure all ignore_ticket_patterns are valid regular expressions",
		},
		{
			name:     "branchahead fallback",
			ctx:      map[string]string{},
			expected: "Set repo.max_commits_ahead to a positive number, or disable the rule by adding 'branchahead' to rules.disabled",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			msg, ok := LookupHelpMessage(diagnostics.ReasonInvalidConfig, testCase.ctx)
			check.True(t, ok)
			check.Equal(t, testCase.expected, msg.String())
		})
	}
}
