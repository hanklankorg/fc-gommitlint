// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package validation

import (
	"context"
	"errors"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const (
	testBranchDevelop = "develop"
	testBranchMain    = "main"
	testBranchMaster  = "master"
	testHashABC       = "abc123"
	testHashDEF       = "def456"
	testHeadRef       = "HEAD"
	testMessage       = "feat: add message"
)

var errTestRepo = errors.New("test repository error")

func TestService_Validate_Targets(t *testing.T) {
	t.Parallel()

	// wantValidated is the point of this table, not a decoration. Every rule
	// the fixture installs passes, so Summary.AllPassed is true whether the
	// target resolved to its commits or to none at all — a routing bug that
	// sent a target to the wrong repository method, or to no method, would
	// return an empty report that reads as a clean run. The count is what
	// distinguishes "the target reached its commits" from "nothing happened
	// and nothing complained".
	tests := []struct {
		name          string
		target        Target
		configure     func(*testRepository)
		readMessage   MessageReader
		wantValidated int
		wantErr       error
	}{
		{
			name:   "commit_target",
			target: CommitTarget(testHeadRef),
			configure: func(repo *testRepository) {
				repo.commits[testHeadRef] = domain.Commit{Hash: testHashABC, Subject: "feat: add target"}
			},
			wantValidated: 1,
		},
		{
			name:   "commits_target",
			target: CommitsTarget([]string{testHashABC, testHashDEF}),
			configure: func(repo *testRepository) {
				repo.commits[testHashABC] = domain.Commit{Hash: testHashABC, Subject: "feat: first"}
				repo.commits[testHashDEF] = domain.Commit{Hash: testHashDEF, Subject: "fix: second"}
			},
			wantValidated: 2,
		},
		{
			name:   "range_target",
			target: RangeTarget(testBranchMain, testHeadRef),
			configure: func(repo *testRepository) {
				repo.ranges[testBranchMain+".."+testHeadRef] = []domain.Commit{
					{Hash: testHashABC, Subject: "feat: first"},
					{Hash: testHashDEF, Subject: "fix: second"},
				}
			},
			wantValidated: 2,
		},
		{
			name:   "count_target",
			target: CountTarget(2),
			configure: func(repo *testRepository) {
				repo.headCommits = []domain.Commit{
					{Hash: testHashABC, Subject: "feat: first"},
					{Hash: testHashDEF, Subject: "fix: second"},
				}
			},
			wantValidated: 2,
		},
		{
			name:          "message_target",
			target:        MessageTarget("COMMIT_EDITMSG"),
			readMessage:   func(string) (string, error) { return testMessage, nil },
			wantValidated: 1,
		},
		{
			name:        "empty_message_target",
			target:      MessageTarget("COMMIT_EDITMSG"),
			readMessage: func(string) (string, error) { return " \n\t", nil },
			wantErr:     ErrEmptyCommitMessage,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := newTestRepository()
			if testCase.configure != nil {
				testCase.configure(repo)
			}

			service := testValidationService(repo)
			if testCase.readMessage != nil {
				service.ReadMessage = testCase.readMessage
			}

			report, err := service.Validate(t.Context(), Request{Target: testCase.target, Config: settings.Config{}})
			if testCase.wantErr != nil {
				check.ErrorIs(t, err, testCase.wantErr)

				return
			}

			check.NoError(t, err)
			check.True(t, report.Summary.AllPassed)
			check.Equal(t, testCase.wantValidated, report.Summary.ValidatedCommits,
				"the target must reach exactly its own commits")
		})
	}
}

func TestService_Validate_RequiresRepositoryForCommitTargets(t *testing.T) {
	t.Parallel()

	service := testValidationService(nil)
	service.OpenRepository = nil

	_, err := service.Validate(t.Context(), Request{Target: CommitTarget(testHeadRef)})

	check.ErrorIs(t, err, ErrRepositoryRequired)
}

// A message-file target validates text that lives in no repository, so a
// missing repository is skipped there; every other target reads its commits
// from the repository and must surface the same error instead.
func TestService_Validate_MissingRepositoryIsSkippedOnlyForMessageTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		target  Target
		wantErr error
	}{
		{name: "message_target_validates_without_a_repository", target: MessageTarget("COMMIT_EDITMSG")},
		{name: "commit_target_surfaces_the_missing_repository", target: CommitTarget(testHeadRef), wantErr: domain.ErrNotGitRepository},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service := testValidationService(nil)
			service.OpenRepository = func(context.Context) (domain.Repository, error) {
				return nil, domain.ErrNotGitRepository
			}
			service.ReadMessage = func(string) (string, error) { return testMessage, nil }

			report, err := service.Validate(t.Context(), Request{
				BaseBranchCandidates: []string{testBranchMain},
				BaseBranchRequired:   true,
				Target:               testCase.target,
			})
			if testCase.wantErr != nil {
				check.ErrorIs(t, err, testCase.wantErr)

				return
			}

			check.NoError(t, err)
			check.True(t, report.Summary.AllPassed)
			check.Equal(t, 1, report.Summary.ValidatedCommits,
				"skipping the repository must skip the repository, not the validation")
		})
	}
}

// BaseBranchRequired is the explicit --base-branch contract, pinned by
// TestService_Validate_ExplicitMissingBaseBranchFails. Any other target only
// carries the candidates for branch-ahead analysis, so an absent candidate is
// a warning there, not a failed run.
func TestService_Validate_BaseBranchRequiredAppliesOnlyToBaseBranchTargets(t *testing.T) {
	t.Parallel()

	repo := newTestRepository()
	repo.commits[testHeadRef] = domain.Commit{Hash: testHashABC, Subject: "feat: add target"}
	service := testValidationService(repo)

	report, err := service.Validate(t.Context(), Request{
		BaseBranchCandidates: []string{testBranchMain},
		BaseBranchRequired:   true,
		Target:               CommitTarget(testHeadRef),
	})

	check.NoError(t, err)
	check.Equal(t, 1, report.Summary.ValidatedCommits)
	check.Len(t, report.Metadata.WarningDetails, 1)
	check.Equal(t, domain.WarnBaseBranchNotFound, report.Metadata.WarningDetails[0].Code)
}

// A lookup that fails is not a branch that is absent: treating it as absent
// would silently drop the branch-ahead check and warn about a branch that may
// well exist.
func TestService_Validate_SurfacesBranchLookupErrors(t *testing.T) {
	t.Parallel()

	repo := newTestRepository()
	repo.commits[testHeadRef] = domain.Commit{Hash: testHashABC, Subject: "feat: never validated"}
	repo.branchErr = errTestRepo
	service := testValidationService(repo)

	_, err := service.Validate(t.Context(), Request{
		BaseBranchCandidates: []string{testBranchMain},
		Target:               CommitTarget(testHeadRef),
	})

	check.ErrorIs(t, err, errTestRepo)
	check.Equal(t, 0, repo.getCommitCalls, "a failed lookup must stop the run before any commit is read")
}

// branchahead counts commits against the base branch, so without one the
// service excludes the rule itself instead of failing every commit for a
// branch that is not there.
func TestService_Validate_ExcludesBranchAheadOnlyWhenBaseBranchUnresolved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		existing     map[string]bool
		excludeRules []string
		wantExcluded []string
	}{
		{
			name:         "resolved_base_branch_keeps_the_rule",
			existing:     map[string]bool{testBranchMain: true},
			excludeRules: []string{rulecatalog.Spell},
			wantExcluded: []string{rulecatalog.Spell},
		},
		{
			name:         "unresolved_base_branch_excludes_it",
			existing:     map[string]bool{},
			excludeRules: []string{rulecatalog.Spell},
			wantExcluded: []string{rulecatalog.Spell, rulecatalog.BranchAhead},
		},
		{
			name:         "an_explicit_exclusion_is_not_duplicated",
			existing:     map[string]bool{},
			excludeRules: []string{"BranchAhead"},
			wantExcluded: []string{"BranchAhead"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo := newTestRepository()
			repo.branches = testCase.existing
			repo.commits[testHeadRef] = domain.Commit{Hash: testHashABC, Subject: "feat: add target"}

			var gotExcluded []string

			service := testValidationService(repo)
			service.BuildRules = func(_ settings.Config, _ []string, excludeRules []string, _ string) (Rules, error) {
				gotExcluded = excludeRules

				return Rules{Commit: []domain.CommitRule{testCommitRule{}}}, nil
			}

			_, err := service.Validate(t.Context(), Request{
				BaseBranchCandidates: []string{testBranchMain},
				ExcludeRules:         testCase.excludeRules,
				Target:               CommitTarget(testHeadRef),
			})

			check.NoError(t, err)
			check.Equal(t, testCase.wantExcluded, gotExcluded)
		})
	}
}

func TestService_Validate_ExplicitMissingBaseBranchFails(t *testing.T) {
	t.Parallel()

	repo := newTestRepository()
	repo.commits[testHeadRef] = domain.Commit{Hash: testHashABC, Subject: "feat: fallback should not run"}
	service := testValidationService(repo)

	_, err := service.Validate(t.Context(), Request{
		BaseBranchCandidates: []string{testBranchMain, testBranchMaster},
		BaseBranchRequired:   true,
		Target:               BaseBranchTarget(testBranchMain),
	})

	var validationErr domain.ValidationError
	check.ErrorAs(t, err, &validationErr)
	check.Equal(t, diagnostics.ReasonRangeNotFound, validationErr.Diagnostic.Reason)
}

func TestService_Validate_CommitTargetHonorsIgnoreCommits(t *testing.T) {
	t.Parallel()

	ignoredHash := "abc1234"
	repo := newTestRepository()
	repo.commits[testHeadRef] = domain.Commit{Hash: ignoredHash, Subject: "bad subject"}
	service := testValidationService(repo)

	report, err := service.Validate(t.Context(), Request{
		Target: CommitTarget(testHeadRef),
		Config: settings.Config{IgnoreCommits: []string{ignoredHash}},
	})

	check.NoError(t, err)
	check.True(t, report.Summary.AllPassed)
	check.Equal(t, 1, report.Summary.IgnoredCommits)
	check.Zero(t, report.Summary.ValidatedCommits)
}

// TestService_Validate_CancelledContextDoesNotPass is the regression guard for
// the false-pass bug: a cancelled run leaves un-validated commits as zero-value
// ValidationResult{} (no failures), which would render as "passed" if the
// report were built from partial results. The message-target path reaches
// buildMultipleCommitReport without an earlier cancellation check, so it
// exercises the guard that propagates ValidateCommits' error instead.
func TestService_Validate_CancelledContextDoesNotPass(t *testing.T) {
	t.Parallel()

	service := testValidationService(nil)
	service.OpenRepository = nil
	service.ReadMessage = func(string) (string, error) { return testMessage, nil }

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	report, err := service.Validate(ctx, Request{
		Target: MessageTarget("COMMIT_EDITMSG"),
		Config: settings.Config{},
	})

	check.ErrorIs(t, err, context.Canceled)
	check.False(t, report.Summary.AllPassed)
}

func TestService_Validate_CommitsTargetStopsReadingAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	repo := newTestRepository()
	repo.commits[testHashABC] = domain.Commit{Hash: testHashABC, Subject: "feat: first"}
	repo.commits[testHashDEF] = domain.Commit{Hash: testHashDEF, Subject: "fix: second"}
	repo.onGetCommit = cancel

	service := testValidationService(repo)
	_, err := service.Validate(ctx, Request{
		Target: CommitsTarget([]string{testHashABC, testHashDEF}),
		Config: settings.Config{},
	})

	check.ErrorIs(t, err, context.Canceled)
	check.Equal(t, 1, repo.getCommitCalls)
}

func TestService_ResolveBaseBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		candidates      []string
		existing        map[string]bool
		wantBranch      string
		wantWarnings    int
		wantWarningCode domain.WarningCode
	}{
		{name: "single_branch_exists", candidates: []string{testBranchMain}, existing: map[string]bool{testBranchMain: true}, wantBranch: testBranchMain},
		{name: "single_branch_missing", candidates: []string{testBranchMain}, existing: map[string]bool{}, wantWarnings: 1, wantWarningCode: domain.WarnBaseBranchNotFound},
		{name: "first_missing_second_exists", candidates: []string{testBranchMain, testBranchMaster, testBranchDevelop}, existing: map[string]bool{testBranchMaster: true}, wantBranch: testBranchMaster, wantWarnings: 1, wantWarningCode: domain.WarnBaseBranchFallback},
		{name: "none_exist", candidates: []string{testBranchMain, testBranchMaster, testBranchDevelop}, existing: map[string]bool{}, wantWarnings: 1, wantWarningCode: domain.WarnBaseBranchNotFound},
		{name: "empty_candidates", candidates: []string{}, existing: map[string]bool{}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			service := testValidationService(nil)
			repo := newTestRepository()
			repo.branches = testCase.existing

			branch, warnings, err := service.resolveBaseBranch(t.Context(), testCase.candidates, repo, false)

			check.NoError(t, err)
			check.Equal(t, testCase.wantBranch, branch)
			check.Len(t, warnings, testCase.wantWarnings)

			if testCase.wantWarningCode != "" {
				check.Equal(t, testCase.wantWarningCode, warnings[0].Code)
				check.Empty(t, warnings[0].Message)
				check.NotEmpty(t, warnings[0].Context)
			}
		})
	}
}

func TestRebuildTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     Target
		candidates []string
		resolved   string
		want       Target
	}{
		{name: "no_base_branch", target: DefaultTarget(), want: DefaultTarget()},
		{name: "resolved_matches_first", target: BaseBranchTarget(testBranchMain), candidates: []string{testBranchMain, testBranchMaster}, resolved: testBranchMain, want: BaseBranchTarget(testBranchMain)},
		{name: "resolved_to_fallback", target: BaseBranchTarget(testBranchMain), candidates: []string{testBranchMain, testBranchMaster}, resolved: testBranchMaster, want: BaseBranchTarget(testBranchMaster)},
		{name: "unresolved_uses_default", target: BaseBranchTarget(testBranchMain), candidates: []string{testBranchMain, testBranchMaster}, want: DefaultTarget()},
		{name: "message_target_preserved", target: MessageTarget("COMMIT_EDITMSG"), candidates: []string{testBranchMain}, want: MessageTarget("COMMIT_EDITMSG")},
		{name: "explicit_range_preserved", target: RangeTarget(testBranchDevelop, testHeadRef), candidates: []string{testBranchMain}, want: RangeTarget(testBranchDevelop, testHeadRef)},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := rebuildTarget(testCase.target, testCase.candidates, testCase.resolved)

			check.Equal(t, testCase.want, got)
		})
	}
}

// TestValidateCommitCount_RejectsZeroAndNegative pins the one rule this
// function has. The sentinel is asserted rather than "an error", because the
// caller distinguishes a bad --count from every other way target resolution
// can fail, and a bare check would pass on any of them.
func TestValidateCommitCount_RejectsZeroAndNegative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		count   int
		wantErr error
	}{
		{name: "positive count is accepted", count: 5},
		{name: "zero is rejected", count: 0, wantErr: ErrCommitCountNotPositive},
		{name: "negative is rejected", count: -1, wantErr: ErrCommitCountNotPositive},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateCommitCount(testCase.count)
			if testCase.wantErr != nil {
				check.ErrorIs(t, err, testCase.wantErr)

				return
			}

			check.NoError(t, err)
		})
	}
}

type testCommitRule struct{}

func (testCommitRule) Name() string { return "test" }

func (testCommitRule) Validate(domain.Commit) []domain.ValidationError {
	return nil
}

type testRepository struct {
	branches       map[string]bool
	branchErr      error
	commits        map[string]domain.Commit
	ranges         map[string][]domain.Commit
	headCommits    []domain.Commit
	onGetCommit    func()
	getCommitCalls int
}

func newTestRepository() *testRepository {
	return &testRepository{
		branches: map[string]bool{},
		commits:  map[string]domain.Commit{},
		ranges:   map[string][]domain.Commit{},
	}
}

func (r *testRepository) GetCommit(_ context.Context, ref string) (domain.Commit, error) {
	r.getCommitCalls++
	if r.onGetCommit != nil {
		r.onGetCommit()
		r.onGetCommit = nil
	}

	commit, ok := r.commits[ref]
	if !ok {
		return domain.Commit{}, errTestRepo
	}

	return commit, nil
}

func (r *testRepository) GetCommitRange(_ context.Context, from string, to string) ([]domain.Commit, error) {
	commits, ok := r.ranges[from+".."+to]
	if !ok {
		return nil, errTestRepo
	}

	return commits, nil
}

func (r *testRepository) GetHeadCommits(_ context.Context, count int) ([]domain.Commit, error) {
	if count > len(r.headCommits) {
		return r.headCommits, nil
	}

	return r.headCommits[:count], nil
}

func (r *testRepository) GetCommitsAheadCount(context.Context, string) (int, error) {
	return 0, nil
}

func (r *testRepository) GetCurrentBranch(context.Context) (string, error) {
	return testBranchMain, nil
}

func (r *testRepository) BranchExists(_ context.Context, branchName string) (bool, error) {
	if r.branchErr != nil {
		return false, r.branchErr
	}

	return r.branches[branchName], nil
}

type testLogger struct{}

func (testLogger) Log(string, string, ...any)       {}
func (testLogger) Trace(string, ...any)             {}
func (testLogger) TraceFunc(func() (string, []any)) {}
func (testLogger) Debug(string, ...any)             {}
func (testLogger) Info(string, ...any)              {}
func (testLogger) Warn(string, ...any)              {}
func (testLogger) Error(string, ...any)             {}

func testValidationService(repo domain.Repository) Service {
	return Service{
		BuildRules: func(settings.Config, []string, []string, string) (Rules, error) {
			return Rules{Commit: []domain.CommitRule{testCommitRule{}}}, nil
		},
		Logger: testLogger{},
		OpenRepository: func(context.Context) (domain.Repository, error) {
			return repo, nil
		},
		ReadMessage: func(string) (string, error) {
			return testMessage, nil
		},
	}
}
