// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestValidateCommit_WithVariousRules_ReturnsExpectedErrors verifies that
// validation correctly applies commit rules, aggregates errors from multiple
// failing rules, and preserves error context for debugging.
func TestValidateCommit_WithVariousRules_ReturnsExpectedErrors(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name        string
		commit      Commit
		commitRules []CommitRule
		repoRules   []RepositoryRule
		wantErrs    int
	}{
		{
			name: "Valid commit with no rules",
			commit: Commit{
				Hash:    "abc123",
				Subject: "feat: add feature",
				Message: "feat: add feature\n\nDetailed description",
			},
			commitRules: []CommitRule{},
			repoRules:   []RepositoryRule{},
			wantErrs:    0,
		},
		{
			name: "Commit with failing commit rule",
			commit: Commit{
				Hash:    "abc123",
				Subject: "feat: add feature",
				Message: "feat: add feature",
			},
			commitRules: []CommitRule{
				createFailingRule("subject",
					New("subject", diagnostics.ReasonSubjectTooLong, "Subject too long"),
				),
			},
			repoRules: []RepositoryRule{},
			wantErrs:  1,
		},
		{
			name: "Commit with repository rule (not executed)",
			commit: Commit{
				Hash:    "abc123",
				Subject: "feat: add feature",
				Message: "feat: add feature",
			},
			commitRules: []CommitRule{},
			repoRules: []RepositoryRule{
				createFailingRepoRule("branchahead",
					New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits ahead"),
				),
			},
			wantErrs: 0, // Repository rules no longer run in ValidateCommit
		},
		{
			name: "Commit with multiple failing commit rules",
			commit: Commit{
				Hash:    "abc123",
				Subject: "invalid subject",
				Message: "invalid subject",
			},
			commitRules: []CommitRule{
				createFailingRule("subject",
					New("subject", diagnostics.ReasonSubjectTooLong, "Subject too long"),
				),
				createFailingRule("conventional",
					New("conventional", diagnostics.ReasonInvalidFormat, "Not conventional format"),
				),
			},
			repoRules: []RepositoryRule{
				createFailingRepoRule("branchahead",
					New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits ahead"),
				),
			},
			wantErrs: 2, // Only commit rules run, not repository rules
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{}
			logger := noopLogger()

			result := ValidateCommit(testCase.commit, testCase.commitRules, cfg, logger)

			check.Equal(t, testCase.commit, result.Commit)
			check.Len(t, result.Errors, testCase.wantErrs)
		})
	}
}

// TestValidateCommits_MultipleCommits_AppliesRulesToEach verifies that
// when validating multiple commits, each commit is independently validated
// with the same set of rules and errors are collected per commit.
func TestValidateCommits_MultipleCommits_AppliesRulesToEach(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Hash: "abc123", Subject: "feat: add feature", Message: "feat: add feature"},
		{Hash: "def456", Subject: "fix: bug fix", Message: "fix: bug fix"},
	}

	commitRules := []CommitRule{
		createFailingRule("subject",
			New("subject", diagnostics.ReasonSubjectTooLong, "Subject too long"),
		),
	}

	repoRules := []RepositoryRule{}
	cfg := settings.Config{}
	repo := testRepository{}
	logger := noopLogger()

	vc := &ValidationContext{
		BranchAnalyzer: repo,
		CommitRules:    commitRules,
		RepoRules:      repoRules,
		Config:         cfg,
		Logger:         logger,
	}
	results, _, _ := ValidateCommits(t.Context(), commits, vc)

	check.Len(t, results, 2)

	// Each commit should have the same error from the failing rule
	for i, result := range results {
		check.Equal(t, commits[i], result.Commit)
		check.Len(t, result.Errors, 1)
		check.Equal(t, "subject", result.Errors[0].Rule)
	}
}

// TestValidateRepository_WithFailingRules_CollectsAllErrors verifies that
// repository-level validation rules are executed independently and all
// errors are collected and returned together.
func TestValidateRepository_WithFailingRules_CollectsAllErrors(t *testing.T) {
	t.Parallel()

	repoRules := []RepositoryRule{
		createFailingRepoRule("branchahead",
			New("branchahead", diagnostics.ReasonTooManyCommits, "Too many commits ahead"),
		),
		createPassingRepoRule("passing_rule"),
	}

	repo := testRepository{}
	logger := noopLogger()

	errors, _ := ValidateRepositoryWithInfo(t.Context(), repoRules, repo, logger)

	check.Len(t, errors, 1)
	check.Equal(t, "branchahead", errors[0].Rule)
	check.Equal(t, "Too many commits ahead", errors[0].Message)
}

func TestCommitRulesValidation_FiltersAndAppliesCorrectly(t *testing.T) {
	t.Parallel()

	commit := Commit{
		Hash:    "abc123",
		Subject: "feat: add feature",
		Message: "feat: add feature",
	}

	rules := []CommitRule{
		createPassingRule("passing_rule"),
		createFailingRule("failing_rule",
			New("failing_rule", diagnostics.ReasonSubjectTooLong, "Error 1"),
			New("failing_rule", diagnostics.ReasonInvalidFormat, "Error 2"),
		),
	}

	logger := noopLogger()

	errors := ValidateCommitRules(commit, rules, logger)

	check.Len(t, errors, 2)
	check.Equal(t, "failing_rule", errors[0].Rule)
	check.Equal(t, "failing_rule", errors[1].Rule)
}

// TestDisabledRules_ContainsExpectedDefaults pins the default-disabled list
// AGENTS.md documents, independently of the catalog's own tests. Rule
// precedence itself is resolved by settings.ResolveEnabledRules and guarded in
// rulefactory and configuration.
func TestDisabledRules_ContainsExpectedDefaults(t *testing.T) {
	t.Parallel()

	want := []string{"jirareference", "identity", "linearhistory"}
	check.ElementsMatch(t, want, rulecatalog.DefaultDisabled())
}

func TestValidateCommitRules_RepeatedCalls(t *testing.T) {
	t.Parallel()

	// Test that ValidateCommitRules works correctly
	commit := Commit{
		Hash:    "abc123",
		Subject: "feat: add feature",
		Message: "feat: add feature",
	}

	rule := createFailingRule("test_rule",
		New("test_rule", diagnostics.ReasonSubjectTooLong, "Test error"),
	)

	rules := []CommitRule{rule}
	logger := noopLogger()

	// Call function multiple times with same inputs
	result1 := ValidateCommitRules(commit, rules, logger)
	result2 := ValidateCommitRules(commit, rules, logger)
	result3 := ValidateCommitRules(commit, rules, logger)

	// Results should be identical (deterministic)
	check.Equal(t, result1, result2)
	check.Equal(t, result1, result3)

	// Original inputs should be unchanged
	check.Equal(t, "abc123", commit.Hash)
	check.Len(t, rules, 1)
}

func TestCombineValidationErrors(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name         string
		errorSlices  [][]ValidationError
		wantLen      int
		validateFunc func(t *testing.T, result []ValidationError)
	}{
		{
			name:        "empty slices",
			errorSlices: [][]ValidationError{},
			wantLen:     0,
		},
		{
			name: "single slice with errors",
			errorSlices: [][]ValidationError{
				{
					New("rule1", "code1", "message1"),
					New("rule2", "code2", "message2"),
				},
			},
			wantLen: 2,
			validateFunc: func(t *testing.T, result []ValidationError) {
				t.Helper()
				check.Equal(t, "rule1", result[0].Rule)
				check.Equal(t, "rule2", result[1].Rule)
			},
		},
		{
			name: "multiple slices",
			errorSlices: [][]ValidationError{
				{New("rule1", "code1", "message1")},
				{New("rule2", "code2", "message2")},
				{New("rule3", "code3", "message3")},
			},
			wantLen: 3,
			validateFunc: func(t *testing.T, result []ValidationError) {
				t.Helper()
				check.Equal(t, "rule1", result[0].Rule)
				check.Equal(t, "rule2", result[1].Rule)
				check.Equal(t, "rule3", result[2].Rule)
			},
		},
		{
			name: "with nil slices",
			errorSlices: [][]ValidationError{
				{New("rule1", "code1", "message1")},
				nil,
				{New("rule2", "code2", "message2")},
			},
			wantLen: 2,
			validateFunc: func(t *testing.T, result []ValidationError) {
				t.Helper()
				check.Equal(t, "rule1", result[0].Rule)
				check.Equal(t, "rule2", result[1].Rule)
			},
		},
		{
			name: "with empty slices mixed in",
			errorSlices: [][]ValidationError{
				{New("rule1", "code1", "message1")},
				{},
				{New("rule2", "code2", "message2")},
				{},
			},
			wantLen: 2,
		},
		{
			name: "preserves order",
			errorSlices: [][]ValidationError{
				{
					New("rule1", "code1", "message1"),
					New("rule2", "code2", "message2"),
				},
				{
					New("rule3", "code3", "message3"),
					New("rule4", "code4", "message4"),
				},
			},
			wantLen: 4,
			validateFunc: func(t *testing.T, result []ValidationError) {
				t.Helper()

				for i := range 4 {
					check.Equal(t, fmt.Sprintf("rule%d", i+1), result[i].Rule)
				}
			},
		},
		{
			name: "large number of slices",
			errorSlices: func() [][]ValidationError {
				slices := make([][]ValidationError, 100)
				for i := range 100 {
					slices[i] = []ValidationError{
						New(fmt.Sprintf("rule%d", i), diagnostics.Reason(fmt.Sprintf("code%d", i)), fmt.Sprintf("message%d", i)),
					}
				}

				return slices
			}(),
			wantLen: 100,
			validateFunc: func(t *testing.T, result []ValidationError) {
				t.Helper()
				check.Len(t, result, 100)
			},
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := CombineValidationErrors(testCase.errorSlices...)
			check.Len(t, result, testCase.wantLen)

			if testCase.validateFunc != nil {
				testCase.validateFunc(t, result)
			}
		})
	}
}

// Tests consolidated from validation_rules_test.go.
func TestValidation_RuleOrdering(t *testing.T) {
	t.Parallel()

	// Test that rules are applied in order and all errors are collected
	commit := Commit{Hash: "abc123", Subject: "test", Message: "test"}

	rules := []CommitRule{
		createFailingRule("rule1", New("rule1", diagnostics.ReasonSubjectTooLong, "first error")),
		createFailingRule("rule2", New("rule2", diagnostics.ReasonInvalidFormat, "second error")),
		createFailingRule("rule3", New("rule3", diagnostics.ReasonInvalidFormat, "third error")),
	}

	errors := ValidateCommitRules(commit, rules, noopLogger())

	// All rules should be evaluated
	check.Len(t, errors, 3)
	check.Equal(t, "rule1", errors[0].Rule)
	check.Equal(t, "rule2", errors[1].Rule)
	check.Equal(t, "rule3", errors[2].Rule)
}

func TestValidation_EmptyMessageHandling(t *testing.T) {
	t.Parallel()

	// Test behavior with empty message
	commit := Commit{Hash: "abc123", Message: ""}

	ruleInvoked := false
	rules := []CommitRule{
		testFuncRule{
			name: "test_rule",
			fn: func(candidate Commit) []ValidationError {
				ruleInvoked = true

				if candidate.Message == "" {
					return []ValidationError{New("test_rule", diagnostics.ReasonInvalidFormat, "empty message")}
				}

				return nil
			},
		},
	}

	result := ValidateCommit(commit, rules, settings.Config{}, noopLogger())

	// Rules are still invoked even for empty messages
	check.True(t, ruleInvoked, "rule should be invoked")
	check.True(t, result.HasFailures())
}

func TestValidateCommitRules_ReevaluatesRule(t *testing.T) {
	t.Parallel()

	commit := Commit{Hash: "abc123", Subject: "test", Message: "test"}
	activeMaxLength := 3

	rules := []CommitRule{
		testFuncRule{
			name: "length_check",
			fn: func(candidate Commit) []ValidationError {
				if len(candidate.Subject) > activeMaxLength {
					return []ValidationError{New("length_check", diagnostics.ReasonSubjectTooLong, "subject exceeds limit")}
				}

				return nil
			},
		},
	}

	errors := ValidateCommitRules(commit, rules, noopLogger())
	check.Len(t, errors, 1)
	check.Contains(t, errors[0].Message, "subject exceeds limit")

	activeMaxLength = 10
	errors = ValidateCommitRules(commit, rules, noopLogger())
	check.Empty(t, errors)
}

func TestValidation_ConcurrentValidation(t *testing.T) {
	t.Parallel()

	// Test thread safety of validation
	commits := []Commit{
		{Hash: "abc123", Subject: "feat: one", Message: "feat: one"},
		{Hash: "def456", Subject: "fix: two", Message: "fix: two"},
		{Hash: "ghi789", Subject: "docs: three", Message: "docs: three"},
	}

	rules := []CommitRule{
		createFailingRule("always_fail", New("always_fail", diagnostics.ReasonSubjectTooLong, "error")),
	}

	var wg sync.WaitGroup

	results := make([]ValidationResult, len(commits))

	for i, commit := range commits { //nolint:varnamelen // loop index
		wg.Add(1)

		go func(idx int, c Commit) {
			defer wg.Done()

			r := ValidateCommit(c, rules, settings.Config{}, noopLogger())
			results[idx] = r
		}(i, commit)
	}

	wg.Wait()

	// Each commit should have been validated independently
	for i, result := range results {
		check.Equal(t, commits[i], result.Commit)
		check.Len(t, result.Errors, 1)
		check.Equal(t, "always_fail", result.Errors[0].Rule)
	}
}

func TestValidation_RepositoryRulesExecution(t *testing.T) {
	t.Parallel()

	// Test repository rule execution
	ctx := t.Context()

	rulesCalled := 0
	rules := []RepositoryRule{
		testFuncRepoRule{
			name: "rule1",
			fn: func(ctx context.Context, repo BranchAnalyzer) []ValidationError {
				rulesCalled++

				return []ValidationError{New("rule1", diagnostics.ReasonValidationFailed, "error from rule1")}
			},
		},
		testFuncRepoRule{
			name: "rule2",
			fn: func(ctx context.Context, repo BranchAnalyzer) []ValidationError {
				rulesCalled++

				return nil
			},
		},
	}

	repo := testRepository{}

	errors, _ := ValidateRepositoryWithInfo(ctx, rules, repo, noopLogger())

	check.Equal(t, 2, rulesCalled, "both rules should be called")
	check.Len(t, errors, 1)
	check.Equal(t, "rule1", errors[0].Rule)
}

func TestValidation_ErrorAggregation(t *testing.T) {
	t.Parallel()

	// Test that multiple errors from single rule are all captured
	commit := Commit{Hash: "abc123", Subject: "test", Message: "test"}

	rules := []CommitRule{
		testFuncRule{
			name: "multi_error",
			fn: func(c Commit) []ValidationError {
				return []ValidationError{
					New("multi_error", diagnostics.ReasonSubjectTooLong, "error 1"),
					New("multi_error", diagnostics.ReasonInvalidFormat, "error 2"),
					New("multi_error", diagnostics.ReasonInvalidFormat, "error 3"),
				}
			},
		},
	}

	errors := ValidateCommitRules(commit, rules, noopLogger())

	check.Len(t, errors, 3)

	for i, err := range errors {
		check.Equal(t, "multi_error", err.Rule)
		check.Contains(t, err.Message, string(rune('1'+i)))
	}
}

func TestValidation_LoggerUsage(t *testing.T) {
	t.Parallel()

	// Test that logger is called appropriately during validation
	commit := Commit{Hash: "abc123", Subject: "test", Message: "test"}

	logger := &testLogger{}
	rules := []CommitRule{
		createFailingRule("failing", New("failing", diagnostics.ReasonSubjectTooLong, "error")),
		createPassingRule("passing"),
	}

	_ = ValidateCommitRules(commit, rules, logger)

	// Logger should have recorded debug messages
	hasValidationLog := false

	for _, log := range logger.logs {
		if log.level == "debug" && log.msg != "" {
			hasValidationLog = true

			break
		}
	}

	check.True(t, hasValidationLog, "logger should record validation activity")
}

func TestValidation_NilHandling(t *testing.T) {
	t.Parallel()

	// Test graceful handling of nil/empty inputs
	commit := Commit{Hash: "abc123", Subject: "test", Message: "test"}

	// Nil rules slice
	errors := ValidateCommitRules(commit, nil, noopLogger())
	check.Empty(t, errors)

	// Empty rules slice
	errors = ValidateCommitRules(commit, []CommitRule{}, noopLogger())
	check.Empty(t, errors)

	// Rule returning nil
	rules := []CommitRule{
		testFuncRule{
			name: "nil_return",
			fn: func(c Commit) []ValidationError {
				return nil
			},
		},
	}
	errors = ValidateCommitRules(commit, rules, noopLogger())
	check.Empty(t, errors)
}

func TestValidation_RealWorldScenario(t *testing.T) {
	t.Parallel()

	// Simulate a real validation scenario with multiple commits and rules
	commits := []Commit{
		{Hash: "abc123", Subject: "feat: add new feature", Message: "feat: add new feature\n\nThis adds the new feature X"},
		{Hash: "def456", Subject: "fix", Message: "fix"}, // Invalid conventional format
		{Hash: "ghi789", Subject: "", Message: ""},       // Empty
		{Hash: "jkl012", Subject: "docs: update README", Message: "docs: update README"},
	}

	// Simulate conventional commit validation
	rules := []CommitRule{
		testFuncRule{
			name: "conventional",
			fn: func(c Commit) []ValidationError {
				if c.Subject == "" {
					return []ValidationError{New("conventional", diagnostics.ReasonInvalidFormat, "empty subject")}
				}

				if c.Subject == "fix" {
					return []ValidationError{New("conventional", diagnostics.ReasonInvalidFormat, "missing scope/description")}
				}

				return nil
			},
		},
		testFuncRule{
			name: "message_length",
			fn: func(c Commit) []ValidationError {
				if len(c.Message) < 10 && c.Message != "" {
					return []ValidationError{New("message_length", diagnostics.ReasonBodyTooShort, "message too short")}
				}

				return nil
			},
		},
	}

	vc := &ValidationContext{
		BranchAnalyzer: testRepository{commits: commits},
		CommitRules:    rules,
		RepoRules:      nil,
		Config:         settings.Config{},
		Logger:         noopLogger(),
	}
	results, _, _ := ValidateCommits(t.Context(), commits, vc)

	check.Len(t, results, 4)

	// First commit should pass
	check.False(t, results[0].HasFailures())

	// Second commit should fail conventional and length
	check.True(t, results[1].HasFailures())
	check.Len(t, results[1].Errors, 2)

	// Third commit validation should error (empty message)
	check.True(t, results[2].HasFailures())

	// Fourth commit should pass
	check.False(t, results[3].HasFailures())
}

// TestValidateCommits_PanickingRuleReachesTheCallerFromBothPaths pins the
// contract validateCommitsParallel promises: "Output to the user is identical
// to the sequential path". A rule that panics is a bug in the rule, not a
// validation failure, so the panic must reach the caller — and through it the
// process-level panic boundary — whether the batch is below or above
// parallelValidationThreshold.
func TestValidateCommits_PanickingRuleReachesTheCallerFromBothPaths(t *testing.T) {
	t.Parallel()

	const rulePanicValue = "rule panic: malformed commit"

	for _, testCase := range []struct {
		name        string
		commitCount int
	}{
		{name: "sequential path", commitCount: 1},
		{name: "parallel path", commitCount: parallelValidationThreshold},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commits := make([]Commit, testCase.commitCount)
			for idx := range commits {
				commits[idx] = Commit{Hash: fmt.Sprintf("hash%03d", idx), Subject: "feat: x", Message: "feat: x"}
			}

			vc := &ValidationContext{
				CommitRules: []CommitRule{testFuncRule{
					name: "Panicking",
					fn: func(Commit) []ValidationError {
						panic(rulePanicValue)
					},
				}},
				Config: settings.NewDefault(),
				Logger: noopLogger(),
			}

			var recovered any

			func() {
				defer func() { recovered = recover() }()

				_, _, _ = ValidateCommits(t.Context(), commits, vc)
			}()

			check.Equal(t, rulePanicValue, recovered, "the rule's panic value must reach the caller unchanged")
		})
	}
}

// testFuncRule allows injecting custom validation logic for testing.
type testFuncRule struct {
	name string
	fn   func(Commit) []ValidationError
}

func (r testFuncRule) Name() string { return r.name }
func (r testFuncRule) Validate(commit Commit) []ValidationError {
	return r.fn(commit)
}

// testFuncRepoRule allows injecting custom repository validation logic.
type testFuncRepoRule struct {
	name string
	fn   func(context.Context, BranchAnalyzer) []ValidationError
}

func (r testFuncRepoRule) Name() string { return r.name }
func (r testFuncRepoRule) Validate(ctx context.Context, repo BranchAnalyzer) (string, []ValidationError) {
	return "", r.fn(ctx, repo)
}

func TestValidation_RepositoryWithErrors(t *testing.T) {
	t.Parallel()

	// Test repository validation with actual errors
	ctx := t.Context()

	var testErr = errors.New("repository access failed")

	errorRepo := &errorRepository{
		err: testErr,
	}

	rules := []RepositoryRule{
		testFuncRepoRule{
			name: "branch_check",
			fn: func(ctx context.Context, repo BranchAnalyzer) []ValidationError {
				_, err := repo.GetCommitsAheadCount(ctx, "main")
				if err != nil {
					return []ValidationError{
						New("branch_check", diagnostics.ReasonValidationFailed, "repo error: "+err.Error()),
					}
				}

				return nil
			},
		},
	}

	errors, _ := ValidateRepositoryWithInfo(ctx, rules, errorRepo, noopLogger())

	check.Len(t, errors, 1)
	check.Contains(t, errors[0].Message, "repository access failed")
}

// errorRepository is a test repository that returns errors.
type errorRepository struct {
	err error
}

func (r *errorRepository) GetCommit(ctx context.Context, ref string) (Commit, error) {
	return Commit{}, r.err
}

func (r *errorRepository) GetCommitRange(ctx context.Context, from, to string) ([]Commit, error) {
	return nil, r.err
}

func (r *errorRepository) GetHeadCommits(ctx context.Context, count int) ([]Commit, error) {
	return nil, r.err
}

func (r *errorRepository) GetCommitsAheadCount(ctx context.Context, referenceBranch string) (int, error) {
	return 0, r.err
}

func (r *errorRepository) GetCurrentBranch(ctx context.Context) (string, error) {
	return "", r.err
}

func (r *errorRepository) BranchExists(ctx context.Context, branchName string) (bool, error) {
	return false, r.err
}

// TestValidateCommits_EdgeCases tests edge cases in batch validation.
func TestValidateCommits_EdgeCases(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name             string
		commits          []Commit
		rules            []CommitRule
		config           settings.Config
		expectResults    int
		expectRepoErrors int
	}{
		{
			name:          "empty commits list",
			commits:       []Commit{},
			rules:         []CommitRule{createPassingRule("test")},
			expectResults: 0,
		},
		{
			name: "single commit validation",
			commits: []Commit{
				{Hash: "abc123", Subject: "test", Message: "test"},
			},
			rules:         []CommitRule{createPassingRule("test")},
			expectResults: 1,
		},
		{
			name: "large batch of commits",
			commits: func() []Commit {
				commits := make([]Commit, 100)
				for i := range commits {
					commits[i] = Commit{
						Hash:    fmt.Sprintf("hash%d", i),
						Subject: fmt.Sprintf("commit %d", i),
						Message: fmt.Sprintf("commit %d", i),
					}
				}

				return commits
			}(),
			rules:         []CommitRule{createPassingRule("test")},
			expectResults: 100,
		},
		{
			name: "commits with ignored SHAs",
			commits: []Commit{
				{Hash: "abc123def456", Subject: "ignored", Message: "ignored"},
				{Hash: "def456ghi789", Subject: "validated", Message: "validated"},
			},
			rules: []CommitRule{
				createFailingRule("test", New("test", diagnostics.ReasonSubjectTooLong, "error")),
			},
			config: settings.Config{
				IgnoreCommits: []string{"abc123d"}, // Matches first commit
			},
			expectResults: 1, // Only second commit is validated (first is filtered out)
		},
	}

	for _, testCase := range tcs {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := testRepository{commits: testCase.commits}
			logger := noopLogger()
			vc := NewValidationContext(testCase.rules, nil, repo, testCase.config, logger, ValidationContextOptions{})

			results, _, _ := ValidateCommits(
				t.Context(),
				testCase.commits,
				vc,
			)

			check.Len(t, results, testCase.expectResults)
		})
	}
}

// TestValidateCommits_ErrorAggregation tests error aggregation across multiple commits.
func TestValidateCommits_ErrorAggregation(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Hash: "abc123", Subject: "test1", Message: "test1"},
		{Hash: "def456", Subject: "test2", Message: "test2"},
		{Hash: "ghi789", Subject: "test3", Message: "test3"},
	}

	// Multiple rules that all fail
	rules := []CommitRule{
		createFailingRule("rule1", New("rule1", diagnostics.ReasonSubjectTooLong, "error1")),
		createFailingRule("rule2", New("rule2", diagnostics.ReasonInvalidFormat, "error2")),
	}

	repo := testRepository{commits: commits}
	logger := noopLogger()
	vc := NewValidationContext(rules, nil, repo, settings.Config{}, logger, ValidationContextOptions{})

	results, _, _ := ValidateCommits(
		t.Context(),
		commits,
		vc,
	)

	check.Len(t, results, 3)

	// Each commit should have errors from both rules
	for _, result := range results {
		check.Len(t, result.Errors, 2, "Each commit should have 2 errors")
		check.True(t, result.HasFailures())
	}
}

// TestValidateCommits_MixedResults tests validation with mixed pass/fail results.
func TestValidateCommits_MixedResults(t *testing.T) {
	t.Parallel()

	commits := []Commit{
		{Hash: "pass1", Subject: "valid", Message: "valid"},
		{Hash: "fail1", Subject: "invalid", Message: "invalid"},
		{Hash: "pass2", Subject: "valid", Message: "valid"},
	}

	// Rule that only fails for commits with "invalid" in subject
	rule := testFuncRule{
		name: "conditional_rule",
		fn: func(c Commit) []ValidationError {
			if c.Subject == "invalid" {
				return []ValidationError{
					New("conditional", diagnostics.ReasonInvalidFormat, "Invalid subject"),
				}
			}

			return nil
		},
	}

	repo := testRepository{commits: commits}
	logger := noopLogger()
	vc := NewValidationContext([]CommitRule{rule}, nil, repo, settings.Config{}, logger, ValidationContextOptions{})

	results, _, _ := ValidateCommits(
		t.Context(),
		commits,
		vc,
	)

	check.Len(t, results, 3)

	check.False(t, results[0].HasFailures(), "First commit should pass")
	check.True(t, results[1].HasFailures(), "Second commit should fail")
	check.False(t, results[2].HasFailures(), "Third commit should pass")
}

// TestValidateCommit_FixupCommits tests the fixup commit handling.
func TestValidateCommit_FixupCommits(t *testing.T) {
	t.Parallel()

	logger := noopLogger()

	tests := []struct {
		name              string
		subject           string
		allowFixupCommits bool
		expectError       bool
		expectedErrCode   diagnostics.Reason
		wantType          string // the context the error reports back, with the subject
	}{
		{
			name:              "regular commit with allow_fixup_commits=false",
			subject:           "feat: add new feature",
			allowFixupCommits: false,
			expectError:       false,
		},
		{
			name:              "regular commit with allow_fixup_commits=true",
			subject:           "feat: add new feature",
			allowFixupCommits: true,
			expectError:       false,
		},
		{
			name:              "fixup commit with allow_fixup_commits=false",
			subject:           "fixup! feat: add new feature",
			allowFixupCommits: false,
			expectError:       true,
			expectedErrCode:   diagnostics.ReasonFixupCommitFound,
			wantType:          "fixup",
		},
		{
			name:              "fixup commit with allow_fixup_commits=true",
			subject:           "fixup! feat: add new feature",
			allowFixupCommits: true,
			expectError:       false,
		},
		{
			name:              "squash commit with allow_fixup_commits=false",
			subject:           "squash! feat: add new feature",
			allowFixupCommits: false,
			expectError:       true,
			expectedErrCode:   diagnostics.ReasonFixupCommitFound,
			wantType:          "squash",
		},
		{
			name:              "squash commit with allow_fixup_commits=true",
			subject:           "squash! feat: add new feature",
			allowFixupCommits: true,
			expectError:       false,
		},
		{
			name:              "amend commit with allow_fixup_commits=false",
			subject:           "amend! feat: add new feature",
			allowFixupCommits: false,
			expectError:       true,
			expectedErrCode:   diagnostics.ReasonFixupCommitFound,
			wantType:          "amend",
		},
		{
			name:              "amend commit with allow_fixup_commits=true",
			subject:           "amend! feat: add new feature",
			allowFixupCommits: true,
			expectError:       false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			commit := Commit{
				Hash:    "abc123",
				Subject: testCase.subject,
				Message: testCase.subject,
			}

			cfg := settings.NewDefault()
			cfg.AllowFixupCommits = new(testCase.allowFixupCommits)

			// Use empty rules - we're testing the early guard, not rules
			result := ValidateCommit(commit, nil, cfg, logger)

			if testCase.expectError {
				check.True(t, result.HasFailures(), "expected validation to fail")
				check.Len(t, result.Errors, 1, "expected exactly one error")
				check.Equal(t, testCase.expectedErrCode, result.Errors[0].Diagnostic.Reason)
				check.Equal(t, diagnostics.Context{
					diagnostics.ContextType:    testCase.wantType,
					diagnostics.ContextSubject: testCase.subject,
				}, result.Errors[0].Context)
			} else {
				check.False(t, result.HasFailures(), "expected validation to pass")
			}
		})
	}
}

// TestValidateCommit_FixupCommitSkipsRules verifies that allowed fixup commits skip rule validation.
func TestValidateCommit_FixupCommitSkipsRules(t *testing.T) {
	t.Parallel()

	logger := noopLogger()

	// Create a rule that always fails
	failingRule := testFuncRule{
		name: "AlwaysFail",
		fn: func(commit Commit) []ValidationError {
			return []ValidationError{
				New("AlwaysFail", diagnostics.ReasonValidationFailed, "This rule always fails"),
			}
		},
	}

	commit := Commit{
		Hash:    "abc123",
		Subject: "fixup! feat: add new feature",
		Message: "fixup! feat: add new feature",
	}

	cfg := settings.NewDefault()
	cfg.AllowFixupCommits = new(true)

	// With allow_fixup_commits=true, the failing rule should NOT run
	result := ValidateCommit(commit, []CommitRule{failingRule}, cfg, logger)

	check.False(t, result.HasFailures(), "fixup commit with allow_fixup_commits=true should skip all rules")
}

// TestValidateCommit_FixupCheckDisabledSkipsRules pins the checks.disabled
// bypass for GL901: a fixup commit then passes without running any rule, while
// disabling an unrelated check still reports it.
func TestValidateCommit_FixupCheckDisabledSkipsRules(t *testing.T) {
	t.Parallel()

	logger := noopLogger()

	for _, testCase := range []struct {
		name       string
		disabled   []string
		wantReason diagnostics.Reason
	}{
		{name: "GL901 disabled skips the commit", disabled: []string{"gl901"}},
		{name: "the control: GL102 disabled still reports the fixup", disabled: []string{"GL102"}, wantReason: diagnostics.ReasonFixupCommitFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			invoked := false
			recordingRule := testFuncRule{
				name: "Recording",
				fn: func(Commit) []ValidationError {
					invoked = true

					return nil
				},
			}

			commit := Commit{Hash: "abc123", Subject: "fixup! feat: x", Message: "fixup! feat: x"}
			cfg := settings.NewDefault()
			cfg.AllowFixupCommits = new(false)
			cfg.Checks.Disabled = testCase.disabled

			result := ValidateCommit(commit, []CommitRule{recordingRule}, cfg, logger)

			check.False(t, invoked, "a fixup commit must not reach the rules")

			if testCase.wantReason == "" {
				check.Empty(t, result.Errors)

				return
			}

			check.Len(t, result.Errors, 1)
			check.Equal(t, testCase.wantReason, result.Errors[0].Diagnostic.Reason)
		})
	}
}
