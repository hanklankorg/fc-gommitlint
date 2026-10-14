// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var errCommitNotFound = errors.New("commit not found")

const (
	logLevelTrace = "trace"
	logLevelDebug = "debug"
	logLevelInfo  = "info"
	logLevelWarn  = "warn"
	logLevelError = "error"
)

// createFailingRule creates a test rule that always fails with given errors.
func createFailingRule(name string, errors ...ValidationError) CommitRule {
	return testRule{name: name, errors: errors}
}

// createPassingRule creates a test rule that always passes.
func createPassingRule(name string) CommitRule {
	return testRule{name: name, errors: nil}
}

// createPassingRepoRule creates a repository rule that always passes.
func createPassingRepoRule(name string) RepositoryRule {
	return testRepoRule{name: name}
}

func createFailingRepoRule(name string, errors ...ValidationError) RepositoryRule {
	return testRepoRule{name: name, errors: errors}
}

// testRule implements CommitRule for testing.
type testRule struct {
	name   string
	errors []ValidationError
}

func (r testRule) Name() string { return r.name }
func (r testRule) Validate(commit Commit) []ValidationError {
	return r.errors
}

// testRuleWithChecks implements both CommitRule and CheckDescriber for testing.
type testRuleWithChecks struct {
	testRule

	checks []string
}

func (r testRuleWithChecks) ActiveChecks() []string { return r.checks }

// createPassingRuleWithChecks creates a test rule that passes and reports active checks.
func createPassingRuleWithChecks(name string, checks []string) CommitRule {
	return testRuleWithChecks{
		name: name, errors: nil,
		checks: checks,
	}
}

// testRepoRule implements RepositoryRule for testing.
type testRepoRule struct {
	name   string
	errors []ValidationError
}

func (r testRepoRule) Name() string { return r.name }
func (r testRepoRule) Validate(_ context.Context, _ BranchAnalyzer) (string, []ValidationError) {
	return "", r.errors
}

// testRepository is a test implementation of Repository.
type testRepository struct {
	commits []Commit
}

func (r testRepository) GetCommit(ctx context.Context, ref string) (Commit, error) {
	for _, c := range r.commits {
		if c.Hash == ref {
			return c, nil
		}
	}

	if len(r.commits) > 0 {
		return r.commits[0], nil
	}

	return Commit{}, fmt.Errorf("%w: %s", errCommitNotFound, ref)
}

func (r testRepository) GetCommitRange(ctx context.Context, from, to string) ([]Commit, error) {
	return r.commits, nil
}

func (r testRepository) GetHeadCommits(ctx context.Context, count int) ([]Commit, error) {
	if count > len(r.commits) {
		return r.commits, nil
	}

	return r.commits[:count], nil
}

func (r testRepository) GetCommitsAheadCount(ctx context.Context, referenceBranch string) (int, error) {
	return len(r.commits), nil
}

func (r testRepository) GetCurrentBranch(ctx context.Context) (string, error) {
	return "test-branch", nil
}

func (r testRepository) BranchExists(ctx context.Context, branchName string) (bool, error) {
	return true, nil
}

// testLogger implements Logger interface for testing.
type testLogger struct {
	mu   sync.Mutex
	logs []struct {
		level string
		msg   string
		args  []any
	}
}

func (t *testLogger) Log(level string, msg string, args ...any) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.logs = append(t.logs, struct {
		level string
		msg   string
		args  []any
	}{level: level, msg: msg, args: args})
}

func (t *testLogger) Trace(msg string, args ...any) { t.Log(logLevelTrace, msg, args...) }
func (t *testLogger) TraceFunc(fn func() (string, []any)) {
	msg, args := fn()
	t.Log(logLevelTrace, msg, args...)
}
func (t *testLogger) Debug(msg string, args ...any) { t.Log(logLevelDebug, msg, args...) }
func (t *testLogger) Info(msg string, args ...any)  { t.Log(logLevelInfo, msg, args...) }
func (t *testLogger) Warn(msg string, args ...any)  { t.Log(logLevelWarn, msg, args...) }
func (t *testLogger) Error(msg string, args ...any) { t.Log(logLevelError, msg, args...) }

// noopLogger returns a test logger that does nothing.
func noopLogger() Logger {
	return &testLogger{}
}
