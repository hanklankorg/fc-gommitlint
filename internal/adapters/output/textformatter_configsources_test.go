// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// Config provenance is earned, not automatic: it answers a question the reader
// only asks when a rule fired unexpectedly, when they asked for detail, or when
// the config came from somewhere they could not have guessed. A passing
// commit-msg hook should not print a path on every commit.
func TestTextFormatter_ConfigSourcesVisibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		passing    bool
		verbose    int
		quiet      bool
		unexpected bool
		wantShown  bool
	}{
		{
			name:      "hidden on a clean run",
			passing:   true,
			wantShown: false,
		},
		{
			name:      "shown when something failed",
			passing:   false,
			wantShown: true,
		},
		{
			name:      "shown on a clean run when verbose",
			passing:   true,
			verbose:   1,
			wantShown: true,
		},
		{
			name:       "shown on a clean run when config came from outside the repo",
			passing:    true,
			unexpected: true,
			wantShown:  true,
		},
		{
			name:      "quiet wins over failure",
			passing:   false,
			quiet:     true,
			wantShown: false,
		},
		{
			name:       "quiet wins over unexpected config",
			passing:    true,
			quiet:      true,
			unexpected: true,
			wantShown:  false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			builder := newReportBuilder()
			if testCase.passing {
				builder = builder.passingCommit(testCommitHashABC1234, "feat: add widget")
			} else {
				builder = builder.failingCommit(testCommitHashABC1234, "bad subject", "subject",
					domain.New("subject", diagnostics.ReasonSubjectTooLong, "subject is too long"))
			}

			result := Text(builder.build(), TextOptions{
				VerboseLevel:            testCase.verbose,
				Quiet:                   testCase.quiet,
				ConfigSources:           []string{".gommitlint.yaml"},
				ConfigSourcesUnexpected: testCase.unexpected,
			})

			check.Equal(t, testCase.wantShown, strings.Contains(result, "Config: .gommitlint.yaml"))
		})
	}
}

// Without prepared sources, the formatter omits config metadata.
func TestTextFormatter_ConfigSourcesOmittedWhenUnset(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		configSources("defaults", "/home/dev/project/.gommitlint.yaml").
		failingCommit(testCommitHashABC1234, "bad subject", "subject",
			domain.New("subject", diagnostics.ReasonSubjectTooLong, "subject is too long")).
		build()

	result := Text(report, TextOptions{})

	check.NotContains(t, result, "Config:")
	check.NotContains(t, result, "/home/dev/project/.gommitlint.yaml")
}

// Sources arrive highest-precedence first and are joined, not re-sorted.
func TestTextFormatter_ConfigSourcesJoinInOrder(t *testing.T) {
	t.Parallel()

	report := newReportBuilder().
		failingCommit(testCommitHashABC1234, "bad subject", "subject",
			domain.New("subject", diagnostics.ReasonSubjectTooLong, "subject is too long")).
		build()

	result := Text(report, TextOptions{
		ConfigSources: []string{".gommitlint.yaml", "~/.config/gommitlint/settings.yaml (global)"},
	})

	check.Contains(t, result, "Config: .gommitlint.yaml + ~/.config/gommitlint/settings.yaml (global)")
}
