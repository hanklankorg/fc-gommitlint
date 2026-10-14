// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const (
	branchAheadFeatureBranch = "feature/test"
	branchAheadMainBranch    = "main"
)

// Test error constants for err113 compliance.
var (
	ErrTestError = errors.New("test error")
	ErrGitError  = errors.New("git error")
)

// mockRepository is a test implementation of the Repository interface.
type mockRepository struct {
	commitsAhead     int
	err              error
	refBranchName    string
	currentBranch    string
	currentBranchErr error
	shouldError      bool
	errorMsg         string
}

func (m *mockRepository) GetCommitsAhead(_ context.Context, refBranch string) (int, error) {
	m.refBranchName = refBranch

	return m.commitsAhead, m.err
}

func (m *mockRepository) GetCommitsAheadCount(_ context.Context, refBranch string) (int, error) {
	m.refBranchName = refBranch

	if m.shouldError {
		return 0, fmt.Errorf("%s: %w", m.errorMsg, ErrTestError)
	}

	if m.err != nil {
		return 0, m.err
	}

	return m.commitsAhead, nil
}

// BranchExists implements Repository interface.
func (m *mockRepository) BranchExists(_ context.Context, branchName string) (bool, error) {
	return branchName == m.refBranchName, nil
}

// Stub implementations for Repository interface.
func (m *mockRepository) GetCommit(_ context.Context, _ string) (domain.Commit, error) {
	return domain.Commit{}, nil
}

func (m *mockRepository) GetCommits(_ context.Context, _ int) ([]domain.Commit, error) {
	return nil, nil
}

func (m *mockRepository) GetCommitRange(_ context.Context, _, _ string) ([]domain.Commit, error) {
	return nil, nil
}

func (m *mockRepository) GetHeadCommits(_ context.Context, _ int) ([]domain.Commit, error) {
	return nil, nil
}

func (m *mockRepository) GetCurrentBranch(_ context.Context) (string, error) {
	if m.currentBranchErr != nil {
		return "", m.currentBranchErr
	}

	if m.currentBranch != "" {
		return m.currentBranch, nil
	}

	return branchAheadFeatureBranch, nil
}

func (m *mockRepository) GetRepositoryName(_ context.Context) string { return "" }
func (m *mockRepository) IsValid(_ context.Context) (bool, error)    { return true, nil }

func TestBranchAheadRule_BasicValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		maxCommits    int
		commitsAhead  int
		repoErr       error
		wantErrors    bool
		wantErrorCode expectedReason
	}{
		{
			name:         "within limit",
			maxCommits:   10,
			commitsAhead: 5,
			wantErrors:   false,
		},
		{
			name:          "exceeds limit",
			maxCommits:    5,
			commitsAhead:  10,
			wantErrors:    true,
			wantErrorCode: diagnostics.ReasonTooManyCommits,
		},
		{
			name:          "repository error",
			maxCommits:    10,
			repoErr:       ErrTestError,
			wantErrors:    true,
			wantErrorCode: diagnostics.ReasonGitOperationFailed,
		},
		{
			name:         "exactly at limit",
			maxCommits:   10,
			commitsAhead: 10,
			wantErrors:   false,
		},
		{
			name:         "zero commits ahead",
			maxCommits:   10,
			commitsAhead: 0,
			wantErrors:   false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Repo: settings.RepoConfig{
					MaxCommitsAhead: new(testCase.maxCommits),
				},
			}

			repo := &mockRepository{
				commitsAhead: testCase.commitsAhead,
				err:          testCase.repoErr,
			}

			rule := NewBranchAheadRule(cfg, branchAheadMainBranch)
			_, failures := rule.Validate(t.Context(), repo)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)

				if testCase.wantErrorCode != "" {
					check.Equal(t, testCase.wantErrorCode, reasonOf(failures[0]))
				}
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestBranchAheadRule_MissingReference(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Repo: settings.RepoConfig{
			MaxCommitsAhead: new(settings.DefaultMaxCommitsAhead),
		},
	}

	repo := &mockRepository{commitsAhead: 1}
	rule := NewBranchAheadRule(cfg, "")
	_, failures := rule.Validate(t.Context(), repo)

	check.Len(t, failures, 1)
	check.Equal(t, diagnostics.ReasonMissingReference, reasonOf(failures[0]))
}

func TestBranchAheadRule_ErrorHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		repoErr        error
		wantErrors     bool
		wantErrorCode  expectedReason
		wantInMessage  string
		wantRefContext string
	}{
		{
			name:           "reference not found - explicit config",
			repoErr:        domain.ErrReferenceNotFound,
			wantErrors:     true, // Explicitly configured missing branch is an error, not a silent skip
			wantErrorCode:  diagnostics.ReasonMissingReference,
			wantInMessage:  "Reference branch 'main' not found",
			wantRefContext: "main",
		},
		{
			name:          "not a git repository",
			repoErr:       domain.ErrRepositoryAccess,
			wantErrorCode: diagnostics.ReasonInvalidRepo,
			wantInMessage: "Cannot access repository",
			wantErrors:    true,
		},
		{
			name:          "generic git error",
			repoErr:       ErrGitError,
			wantErrorCode: diagnostics.ReasonGitOperationFailed,
			wantInMessage: "Failed to check commits ahead",
			wantErrors:    true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Repo: settings.RepoConfig{
					MaxCommitsAhead: new(10),
				},
			}

			repo := &mockRepository{err: testCase.repoErr}
			rule := NewBranchAheadRule(cfg, "main")
			_, failures := rule.Validate(t.Context(), repo)

			if testCase.wantErrors {
				check.NotEmpty(t, failures)
				check.Equal(t, testCase.wantErrorCode, reasonOf(failures[0]))
				check.Contains(t, failures[0].Message, testCase.wantInMessage)

				if testCase.wantRefContext != "" {
					check.Equal(t, testCase.wantRefContext, failures[0].Context["ref"])
				}
			} else {
				check.Empty(t, failures)
			}
		})
	}
}

func TestBranchAheadRule_SpecialCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		repo       domain.Repository
		wantErrors bool
		wantReason expectedReason
	}{
		{
			name:       "nil repository",
			repo:       nil,
			wantErrors: false, // Gracefully skips
		},
		{
			name: "negative commits ahead",
			repo: &mockRepository{
				commitsAhead: -1, // Should not happen but test edge case
			},
			wantErrors: false, // -1 > 10 is false: the rule only compares against the maximum
		},
		{
			name: "very large commits ahead",
			repo: &mockRepository{
				commitsAhead: 9999,
			},
			wantErrors: true,
			wantReason: diagnostics.ReasonTooManyCommits,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Repo: settings.RepoConfig{
					MaxCommitsAhead: new(10),
				},
			}

			rule := NewBranchAheadRule(cfg, "main")
			_, failures := rule.Validate(t.Context(), testCase.repo)

			if !testCase.wantErrors {
				check.Empty(t, failures)

				return
			}

			// Name the reason: "some failure" would also be satisfied by the
			// rule tripping over the repository rather than the commit count.
			check.NotEmpty(t, failures)
			check.Equal(t, testCase.wantReason, reasonOf(failures[0]))
		})
	}
}

func TestBranchAheadRule_ReferenceBranchHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		inputRef          string
		expectedStoredRef string // What's stored in the rule
	}{
		{
			name:              "short reference",
			inputRef:          "main",
			expectedStoredRef: "main", // Rule stores as-is
		},
		{
			name:              "already with remote",
			inputRef:          "origin/main",
			expectedStoredRef: "origin/main",
		},
		{
			name:              "custom remote",
			inputRef:          "upstream/develop",
			expectedStoredRef: "upstream/develop",
		},
		{
			name:              "feature branch",
			inputRef:          "feature/test",
			expectedStoredRef: "feature/test",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := &mockRepository{
				commitsAhead: 5,
			}

			cfg := settings.Config{
				Repo: settings.RepoConfig{
					MaxCommitsAhead: new(10),
				},
			}

			rule := NewBranchAheadRule(cfg, testCase.inputRef)
			_, _ = rule.Validate(t.Context(), repo)

			// The rule passes the reference as-is to the repository
			check.Equal(t, testCase.expectedStoredRef, repo.refBranchName)
		})
	}
}

func TestBranchAheadRule_DefaultValues(t *testing.T) {
	t.Parallel()

	// Test that the rule uses sensible defaults for unset values
	emptyConfig := settings.Config{
		Repo: settings.RepoConfig{
			MaxCommitsAhead: nil, // Unset value (nil pointer)
		},
	}

	rule := NewBranchAheadRule(emptyConfig, "")

	// Should use sensible defaults
	check.Equal(t, 10, rule.maxCommitsAhead, "Should default to 10 when unset")
	check.Empty(t, rule.reference, "Should be empty when not provided")
}

func TestBranchAheadRule_InvalidConfiguration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		maxCommits         int
		expectedMaxCommits int
	}{
		{
			name:               "negative max commits",
			maxCommits:         -5,
			expectedMaxCommits: settings.DefaultMaxCommitsAhead, // Should default to 10
		},
		{
			name:               "zero max commits",
			maxCommits:         0,
			expectedMaxCommits: settings.DefaultMaxCommitsAhead, // Should default to 10
		},
		{
			name:               "very large max commits",
			maxCommits:         999999,
			expectedMaxCommits: 999999, // Should accept large values
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Repo: settings.RepoConfig{
					// Explicit pointer even for zero: nil is TestBranchAheadRule_DefaultValues' case.
					MaxCommitsAhead: new(testCase.maxCommits),
				},
			}

			rule := NewBranchAheadRule(cfg, "main")
			check.Equal(t, testCase.expectedMaxCommits, rule.maxCommitsAhead)
		})
	}
}
