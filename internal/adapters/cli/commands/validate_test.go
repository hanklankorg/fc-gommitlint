// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/ci"
	cliAdapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	appValidation "codefloe.com/itiquette/gommitlint/internal/app/validation"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

func TestValidateCommand_HasCorrectStructure(t *testing.T) {
	t.Parallel()

	cmd := NewValidateCommand(nil)

	check.Equal(t, commandNameValidate, cmd.Name)
	check.Equal(t, "validate commit messages against configured rules", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.NotNil(t, cmd.Action)

	flagNames := make(map[string]bool)
	for _, flag := range cmd.Flags {
		flagNames[flag.Names()[0]] = true
	}

	expectedFlags := []string{
		flagNameMessageFile, flagNameRef, flagNameRange, flagNameBaseBranch, flagNameCount,
		flagNameVerbose, flagNameRuleHelp, flagNameReportFile, flagNameRule, flagNameExcludeRule, flagNameIgnoreRepoRules,
		flagNameFormat, // Command-specific format flag (text, plain, json, sarif, github, gitlab, forgejo, table)
	}

	for _, expectedFlag := range expectedFlags {
		check.True(t, flagNames[expectedFlag], "missing flag: %s", expectedFlag)
	}

	// Verify format flag has correct default value
	for _, flag := range cmd.Flags {
		if flag.Names()[0] == flagNameFormat {
			stringFlag, ok := flag.(*cli.StringFlag)
			check.True(t, ok, "format flag should be StringFlag")
			check.Equal(t, output.FormatText, stringFlag.Value, "format flag should default to 'text'")
		}
	}
}

func TestCreateOutputOptions_StdoutUsesCommandWriter(t *testing.T) {
	t.Parallel()

	var outputBuffer bytes.Buffer

	cmd := createTestValidateCommand("", "", "", "", 1, "-", "")
	cmd.Writer = &outputBuffer
	cmd.Flags = append(cmd.Flags, &cli.StringFlag{Name: flagNameFormat, Value: output.FormatText})

	options, err := createOutputOptions(cmd, configTypes.Config{}, ci.Environment{}, nil)
	check.NoError(t, err)
	check.Same(t, &outputBuffer, options.Writer)
}

func TestExecuteValidation_InvalidOptionsPreserveReportFile(t *testing.T) {
	tests := []struct {
		name string
		args []string
		err  error
	}{
		{
			name: "invalid format",
			args: []string{"--format", "invalid"},
			err:  ErrUnsupportedFormat,
		},
		{
			name: "unknown rule",
			args: []string{"--rule", "not-a-rule"},
			err:  ErrUnknownRule,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			reportPath := filepath.Join(t.TempDir(), "report.txt")

			const existingReport = "existing report\n"
			check.NoError(t, os.WriteFile(reportPath, []byte(existingReport), 0o600))

			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)

			root := &cli.Command{
				Name:      gommitlintKey,
				Writer:    &stdout,
				ErrWriter: &stderr,
				Flags:     cliAdapter.NewGlobalFlags(nil, ""),
				Commands:  []*cli.Command{NewValidateCommand(nil)},
			}
			args := []string{
				gommitlintKey,
				"--ignore-config",
				commandNameValidate,
				"--message-file", "-",
				"--report-file", reportPath,
			}
			args = append(args, testCase.args...)

			err := root.Run(t.Context(), args)
			check.ErrorIs(t, err, testCase.err)

			contents, readErr := os.ReadFile(reportPath)
			check.NoError(t, readErr)
			check.Equal(t, existingReport, string(contents))
		})
	}
}

// A refused --repo-path keeps the reason it was refused for: the path
// validator's sentinel and its cause for a bad path, and the repository
// sentinel for a directory that holds no repository.
func TestValidatedRepository_HandlesErrorPaths(t *testing.T) {
	t.Parallel()

	notARepository := t.TempDir()

	testCases := []struct {
		name     string
		repoPath string
		wantErrs []error
	}{
		{
			name:     "invalid repository path",
			repoPath: "/nonexistent/path/that/does/not/exist",
			wantErrs: []error{cliAdapter.ErrInvalidRepoPath, os.ErrNotExist},
		},
		{
			name:     testNamePathTraversalAttempt,
			repoPath: "../../etc/passwd",
			wantErrs: []error{cliAdapter.ErrInvalidRepoPath, domain.ErrPathTraversal},
		},
		{
			name:     "non-repository path",
			repoPath: notARepository,
			wantErrs: []error{domain.ErrNotGitRepository},
		},
	}

	logger := &testLogger{}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cmd := &cli.Command{Name: commandNameValidate}
			cmd.Flags = []cli.Flag{
				&cli.StringFlag{Name: flagNameRepoPath, Value: testCase.repoPath},
			}

			_, err := createRepositoryAdapter(t.Context(), cmd, logger, nil)

			for _, wantErr := range testCase.wantErrs {
				check.ErrorIs(t, err, wantErr)
			}
		})
	}
}

type testLogger struct{}

func (t *testLogger) Log(_ string, _ string, _ ...any)   {}
func (t *testLogger) Trace(_ string, _ ...any)           {}
func (t *testLogger) TraceFunc(_ func() (string, []any)) {}
func (t *testLogger) Debug(_ string, _ ...any)           {}
func (t *testLogger) Info(_ string, _ ...any)            {}
func (t *testLogger) Warn(_ string, _ ...any)            {}
func (t *testLogger) Error(_ string, _ ...any)           {}

// createTestValidateCommand creates a test command with specified flag values to avoid duplication.
func createTestValidateCommand(messageFile, ref, rng, baseBranch string, count int, reportFile, repoPath string) *cli.Command {
	var output bytes.Buffer

	cmd := &cli.Command{
		Name:   commandNameValidate,
		Writer: &output,
	}

	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: flagNameMessageFile, Value: messageFile},
		&cli.StringFlag{Name: flagNameRef, Value: ref},
		&cli.StringFlag{Name: flagNameRange, Value: rng},
		&cli.StringFlag{Name: flagNameBaseBranch, Value: baseBranch},
		&cli.IntFlag{Name: flagNameCount, Value: count},
		&cli.StringFlag{Name: flagNameRuleHelp},
		&cli.StringFlag{Name: flagNameReportFile, Value: reportFile},
		&cli.StringFlag{Name: flagNameRepoPath, Value: repoPath},
	}

	return cmd
}

// A repository target that cannot be opened surfaces the reason: a bad
// --repo-path keeps the validator's sentinel and its cause, and a directory
// without a repository becomes the invalid_repo diagnostic that selects the
// EX_UNAVAILABLE exit code.
func TestValidationReport_HandlesInvalidRepository(t *testing.T) {
	t.Parallel()

	notARepository := t.TempDir()

	testCases := []struct {
		name       string
		repoPath   string
		target     appValidation.Target
		wantErrs   []error
		wantReason diagnostics.Reason
	}{
		{
			name:     "nonexistent repository path",
			repoPath: "/nonexistent/path",
			target:   appValidation.DefaultTarget(),
			wantErrs: []error{cliAdapter.ErrInvalidRepoPath, os.ErrNotExist},
		},
		{
			name:       "ref target in a directory that is not a repository",
			repoPath:   notARepository,
			target:     appValidation.CommitTarget(testHeadRef),
			wantReason: diagnostics.ReasonInvalidRepo,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			cmd := &cli.Command{Name: commandNameValidate, Writer: &buf}
			cmd.Flags = []cli.Flag{
				&cli.StringFlag{Name: flagNameRepoPath, Value: testCase.repoPath},
			}

			outputOptions := cliAdapter.NewOutputOptions(&buf).
				WithFormat(output.FormatText).
				WithColor(colorModeNever)
			setup := ValidationSetup{Target: testCase.target, OutputOptions: outputOptions}

			err := executeValidationAndReport(t.Context(), cmd, setup, outputOptions, nil, nil)

			for _, wantErr := range testCase.wantErrs {
				check.ErrorIs(t, err, wantErr)
			}

			if testCase.wantReason != "" {
				var validationErr domain.ValidationError
				check.ErrorAs(t, err, &validationErr)
				check.Equal(t, testCase.wantReason, validationErr.Diagnostic.Reason)
			}
		})
	}
}

func TestParseRuleFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single rule",
			input:    ruleConventional,
			expected: []string{ruleConventional},
		},
		{
			name:     "multiple rules",
			input:    "conventional,signoff,spell",
			expected: []string{ruleConventional, rulecatalog.SignOff, rulecatalog.Spell},
		},
		{
			name:     "rules with spaces",
			input:    " conventional , signoff , spell ",
			expected: []string{ruleConventional, rulecatalog.SignOff, rulecatalog.Spell},
		},
		{
			name:     "mixed case converted to lowercase",
			input:    "Conventional,SignOff,SPELL",
			expected: []string{ruleConventional, rulecatalog.SignOff, rulecatalog.Spell},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := parseRuleFilter(tt.input)
			check.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateRuleNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		flag    string
		names   []string
		wantErr bool
	}{
		{
			name:    "nil names - no error",
			flag:    flagNameRule,
			names:   nil,
			wantErr: false,
		},
		{
			name:    "empty names - no error",
			flag:    flagNameRule,
			names:   []string{},
			wantErr: false,
		},
		{
			name:    "all valid rule names",
			flag:    flagNameRule,
			names:   []string{rulecatalog.Subject, ruleConventional, rulecatalog.SignOff},
			wantErr: false,
		},
		{
			name:    "single unknown rule",
			flag:    flagNameRule,
			names:   []string{"typo_rule"},
			wantErr: true,
		},
		{
			name:    "mixed valid and unknown",
			flag:    flagNameRule,
			names:   []string{rulecatalog.Subject, "typo_rule"},
			wantErr: true,
		},
		{
			name:    "unknown in exclude-rule flag",
			flag:    flagNameExcludeRule,
			names:   []string{"notarule"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateRuleNames(tt.flag, tt.names, output.EnsureLocalizer(nil))

			if tt.wantErr {
				check.Error(t, err)
				check.ErrorIs(t, err, ErrUnknownRule)
				check.Contains(t, err.Error(), "--"+tt.flag)
			} else {
				check.NoError(t, err)
			}
		})
	}
}

func TestExcludeRulesByName_RemovesOnlyNamedRules(t *testing.T) {
	t.Parallel()

	commitRules := []domain.CommitRule{
		mockCommitRule{name: rulecatalog.Subject},
		mockCommitRule{name: rulecatalog.Conventional},
	}
	remaining := excludeRulesByName(commitRules, []string{rulecatalog.Conventional})
	check.Len(t, remaining, 1)
	check.Equal(t, rulecatalog.Subject, remaining[0].Name())

	repoRules := []domain.RepositoryRule{mockRepoRule{name: ruleBranchAhead}}
	check.Empty(t, excludeRulesByName(repoRules, []string{ruleBranchAhead}))
	check.Len(t, excludeRulesByName(repoRules, []string{"unrelated"}), 1)
}

func TestEnsureRuleExcluded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []string
		ruleName string
		expected []string
	}{
		{
			name:     "empty_rule_name_no_change",
			input:    []string{rulecatalog.Subject},
			ruleName: "",
			expected: []string{rulecatalog.Subject},
		},
		{
			name:     "rule_added_when_missing",
			input:    []string{rulecatalog.Subject},
			ruleName: ruleBranchAhead,
			expected: []string{rulecatalog.Subject, ruleBranchAhead},
		},
		{
			name:     "rule_added_with_non_default_name",
			input:    []string{rulecatalog.Subject},
			ruleName: "customrule",
			expected: []string{rulecatalog.Subject, "customrule"},
		},
		{
			name:     "rule_not_duplicated_case_insensitive",
			input:    []string{rulecatalog.BranchAhead},
			ruleName: ruleBranchAhead,
			expected: []string{rulecatalog.BranchAhead},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := ensureRuleExcluded(testCase.input, testCase.ruleName)
			check.Equal(t, testCase.expected, result)
		})
	}
}

func TestGetRuleHelpContent_UsesSchemaConfigKeys(t *testing.T) {
	tests := []struct {
		name        string
		ruleName    string
		mustContain []string
	}{
		{
			name:     "identity_help_uses_allowed_authors",
			ruleName: ruleIdentity,
			mustContain: []string{
				"identity.allowed_authors",
				"rules.enabled",
				"rules.disabled",
				"checks.enabled",
				"checks.disabled",
			},
		},
		{
			name:     "branch_ahead_help_uses_repo_config",
			ruleName: ruleBranchAhead,
			mustContain: []string{
				"repo.max_commits_ahead",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			help := getRuleHelpContent(testCase.ruleName, nil)

			for _, expected := range testCase.mustContain {
				check.Contains(t, help, expected)
			}
		})
	}
}

func TestGetRuleHelpContent_PreservesLiteralPercentAndLocalizesHeading(t *testing.T) {
	t.Parallel()

	help := getRuleHelpContent(ruleSubject, ruleHelpLiteralPercentLocalizer{})

	check.Contains(t, help, "100% klart")
	check.NotContains(t, help, "%!")
	check.Contains(t, help, "KONFIGURATION:") //nolint:misspell // Swedish fixture verifies the localized heading.
}

func TestGetRuleHelpContent_FallsBackFromMalformedTranslatedFormat(t *testing.T) {
	t.Parallel()

	help := getRuleHelpContent(ruleConventional, malformedRuleHelpLocalizer{})

	check.Contains(t, help, "CONVENTIONAL COMMIT RULE")
	check.NotContains(t, help, "%!")
}

func TestGetRuleHelpContent_ExamplesMatchImplementedRules(t *testing.T) {
	t.Parallel()

	subjectHelp := getRuleHelpContent(ruleSubject, nil)
	check.NotContains(t, subjectHelp, "too short")
	check.Contains(t, subjectHelp, "not capitalized when sentence case is configured")

	signoffHelp := getRuleHelpContent(ruleSignoff, nil)
	check.NotContains(t, signoffHelp, "John <john@example.com> (missing full name)")
	check.Contains(t, signoffHelp, "<john@example.com> (missing name)")

	spellHelp := getRuleHelpContent(ruleSpell, nil)
	check.NotContains(t, spellHelp, `typos like "the"`)
	check.NotContains(t, spellHelp, "Ignores URLs")
	check.Contains(t, spellHelp, `typos like "teh"`) //nolint:misspell // The help example intentionally contains a typo.

	branchHelp := getRuleHelpContent(ruleBranchAhead, nil)
	check.NotContains(t, branchHelp, "Feature branch behind main branch")
	check.Contains(t, branchHelp, "exceeds the configured maximum")
}

type ruleHelpLiteralPercentLocalizer struct{}

func (ruleHelpLiteralPercentLocalizer) Get(message string, args ...any) string {
	switch message {
	case "rulehelp.subject":
		return "100% klart"
	case "CONFIGURATION:":
		return "KONFIGURATION:" //nolint:misspell // Intentional Swedish translation fixture.
	default:
		return output.EnsureLocalizer(nil).Get(message, args...)
	}
}

func (ruleHelpLiteralPercentLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return output.EnsureLocalizer(nil).GetN(singular, plural, count, args...)
}

type malformedRuleHelpLocalizer struct{}

func (malformedRuleHelpLocalizer) Get(message string, args ...any) string {
	if message == "rulehelp.conventional" {
		return "%d"
	}

	return output.EnsureLocalizer(nil).Get(message, args...)
}

func (malformedRuleHelpLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return output.EnsureLocalizer(nil).GetN(singular, plural, count, args...)
}

func TestRuleHelpEntries_CoversAllRules(t *testing.T) {
	t.Parallel()

	registry := ruleHelpRegistry()

	// Forward: every catalog rule has a help entry.
	for _, ruleName := range rulecatalog.IDs() {
		check.Contains(t, registry, ruleName, "catalog rule %q must have a help entry", ruleName)
	}

	// Reverse: no orphan help entries for rules the catalog no longer knows.
	for ruleName := range registry {
		check.True(t, rulecatalog.IsValid(ruleName), "help entry %q has no catalog entry", ruleName)
	}
}
