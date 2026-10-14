// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// parallelValidationThreshold controls when ValidateCommits switches from
// sequential to a bounded worker pool. Below this, goroutine setup overhead
// outweighs the gain — typical commit-msg hook (1 commit) and PR-review
// runs (<30 commits) stay on the sequential path.
const parallelValidationThreshold = 50

// maxParallelValidationWorkers caps concurrency regardless of CPU count.
// Past this, workers contend on shared resources (misspell dictionary
// pages, logger writer) for diminishing returns. Determined empirically;
// revisit if benchmarks show otherwise.
const maxParallelValidationWorkers = 8

// Validation errors.
var (
	ErrEmptyCommitMessage = errors.New("empty commit message")
)

// ValidationStatus of a validation rule.
type ValidationStatus string

// Validation status constants.
const (
	StatusPassed  ValidationStatus = "passed"
	StatusFailed  ValidationStatus = "failed"
	StatusSkipped ValidationStatus = "skipped"
)

// ValidationContext holds all dependencies for validation operations.
type ValidationContext struct {
	CommitRules     []CommitRule
	RepoRules       []RepositoryRule
	BranchAnalyzer  BranchAnalyzer
	CommitReader    CommitReader
	Config          settings.Config
	Logger          Logger
	IgnoreRepoRules bool // Flag to skip repository rules even when available
}

// ValidationContextOptions groups the optional fields for NewValidationContext,
// keeping the constructor to a manageable signature.
type ValidationContextOptions struct {
	// CommitReader overrides the repository as the commit reader.
	// When nil, the branch analyzer is used if it also implements CommitReader.
	CommitReader    CommitReader
	IgnoreRepoRules bool
}

// NewValidationContext creates a new validation context.
// All fields are set at construction; no post-construction mutation is required.
func NewValidationContext(commitRules []CommitRule, repoRules []RepositoryRule,
	branchAnalyzer BranchAnalyzer, cfg settings.Config, logger Logger, opts ValidationContextOptions) *ValidationContext {
	var commitReader CommitReader
	if opts.CommitReader != nil {
		commitReader = opts.CommitReader
	} else if reader, ok := branchAnalyzer.(CommitReader); ok {
		commitReader = reader
	}

	return &ValidationContext{
		CommitRules:     commitRules,
		RepoRules:       repoRules,
		BranchAnalyzer:  branchAnalyzer,
		CommitReader:    commitReader,
		Config:          cfg,
		Logger:          logger,
		IgnoreRepoRules: opts.IgnoreRepoRules,
	}
}

// ReportBuildContext returns a ReportBuildContext derived from the current ValidationContext.
func (vc *ValidationContext) ReportBuildContext() ReportBuildContext {
	return ReportBuildContext{
		CommitRules:   vc.CommitRules,
		RepoRules:     vc.RepoRules,
		HasRepository: vc.BranchAnalyzer != nil,
		SkipRepoRules: vc.IgnoreRepoRules,
	}
}

// ValidateCommit validates a single commit against commit rules only.
// Repository rules should be run separately using ValidateRepository.
//
// Two early-return policies apply before rules run. They look similar (both
// return an empty-error result) but come from different sources of truth:
//
//   - Fixup policy (AllowFixupCommits config): if the user opted in, fixup
//     commits pass silently; otherwise they produce a FixupCommit error.
//   - Merge policy (presence of LinearHistory in the rule set): if the user
//     did NOT enable LinearHistory, merge commits are skipped entirely — their
//     auto-generated subjects shouldn't be linted by content rules. If
//     LinearHistory IS enabled it runs as part of commitRules and produces the
//     "merge commit found" error itself.
//
// For a loop over commits, prefer the batched path (ValidateCommits), which
// caches the LinearHistory lookup across iterations.
func ValidateCommit(commit Commit, commitRules []CommitRule, cfg settings.Config, logger Logger) ValidationResult {
	return validateCommitWithSkipGate(commit, commitRules, cfg, logger, hasLinearHistoryRule(commitRules))
}

// skipReason identifies why a commit was short-circuited before rules ran.
// Empty string means "not skipped" (either ran rules or produced a rule error).
type skipReason string

const (
	skipReasonNone        skipReason = ""
	skipReasonFixupAllow  skipReason = "fixup_allowed"
	skipReasonMergeNoRule skipReason = "merge_linearhistory_disabled"
)

// validateCommitWithSkipGate is the internal dispatcher. linearHistoryActive
// is passed in so callers iterating over many commits can compute it once.
// The returned skipReason is for statistics only; it does not affect the
// ValidationResult shape.
func validateCommitWithSkipGate(commit Commit, commitRules []CommitRule, cfg settings.Config, logger Logger, linearHistoryActive bool) ValidationResult {
	result, _ := validateCommitTraced(commit, commitRules, cfg, logger, linearHistoryActive)

	return result
}

func validateCommitTraced(commit Commit, commitRules []CommitRule, cfg settings.Config, logger Logger, linearHistoryActive bool) (ValidationResult, skipReason) {
	logger.Trace("Entering ValidateCommit", "commit_hash", commit.Hash, "commit_rules", len(commitRules))

	defer func() {
		logger.Trace("Exiting ValidateCommit", "commit_hash", commit.Hash)
	}()

	// Fixup policy: user opts in via AllowFixupCommits.
	if commit.IsFixupCommit() {
		commitType := commit.FixupCommitType()
		logger.Debug("Fixup commit detected", "commit_hash", commit.Hash, "type", commitType)

		if settings.DerefBool(cfg.AllowFixupCommits, false) {
			logger.Debug("Fixup commit allowed, skipping validation", "commit_hash", commit.Hash)

			return ValidationResult{Commit: commit, Errors: nil}, skipReasonFixupAllow
		}

		if settings.IsCheckDisabled(cfg.Checks, string(diagnostics.CheckFixupCommit)) {
			logger.Debug("Fixup commit check disabled, skipping validation", "commit_hash", commit.Hash)

			return ValidationResult{Commit: commit, Errors: nil}, skipReasonFixupAllow
		}

		return ValidationResult{
			Commit: commit,
			Errors: []ValidationError{
				New(FixupCommitRuleName, diagnostics.ReasonFixupCommitFound,
					"Fixup commit found - squash before merge").
					WithContextMap(diagnostics.Context{
						diagnostics.ContextType:    commitType,
						diagnostics.ContextSubject: commit.Subject,
					}),
			},
		}, skipReasonNone
	}

	// Merge policy: user opts in to linting merges by enabling LinearHistory.
	// We branch only on the trusted commit.IsMergeCommit flag (set by the git
	// adapter from parent-count), not on parsed subject content.
	if commit.IsMergeCommit && !linearHistoryActive {
		logger.Trace("Skipping commit (merge commit, LinearHistory rule disabled)",
			"commit_hash", commit.Hash, "subject", commit.Subject)

		return ValidationResult{Commit: commit, Errors: nil}, skipReasonMergeNoRule
	}

	errors := ValidateCommitRules(commit, commitRules, logger)

	result := ValidationResult{Commit: commit, Errors: errors}
	logger.Debug("Commit validation completed", "commit_hash", commit.Hash, "passed", !result.HasFailures(), "error_count", len(errors))

	return result, skipReasonNone
}

// ValidateCommits validates multiple commits against both rule types.
// Commits in ignore_commits configuration are skipped with info logging.
//
// Concurrency: above parallelValidationThreshold commits the implementation
// uses a worker pool; below it, a sequential loop. Both paths preserve order
// — results[i] corresponds to the (post-filter) commits[i] — and both let a
// panic raised by a rule propagate to the caller.
//
// Cancellation contract: the returned error is ctx.Err() (nil on success). On
// cancellation both paths exit early, leaving un-validated slots as zero-value
// ValidationResult{}; the non-nil error is what tells callers those partial
// results are not a clean pass. Callers MUST propagate the error rather than
// build a report from the returned slice — a zero-value result has no failures
// and would otherwise render an un-validated commit as "passed".
func ValidateCommits(ctx context.Context, commits []Commit, vc *ValidationContext) ([]ValidationResult, CommitIgnoreInfo, error) {
	vc.Logger.Trace("Entering ValidateCommits", "commit_count", len(commits), "commit_rules", len(vc.CommitRules), "repo_rules", len(vc.RepoRules))

	defer func() {
		vc.Logger.Trace("Exiting ValidateCommits", "commit_count", len(commits))
	}()

	// Filter out ignored commits
	validatedCommits, ignoreInfo := FilterIgnoredCommits(commits, vc.Config)

	// Log ignore information
	if ignoreInfo.IgnoredCount > 0 {
		vc.Logger.Info("Commits ignored based on ignore_commits configuration",
			"ignored_count", ignoreInfo.IgnoredCount,
			"validated_count", ignoreInfo.ValidatedCount,
			"total_count", ignoreInfo.TotalCommits)
		vc.Logger.Debug("Ignored commit SHAs", "ignored_shas", ignoreInfo.IgnoredSHAs)
	}

	// Cache the LinearHistory lookup once for the whole batch — the rule set
	// doesn't change across commits in a single validation run.
	linearHistoryActive := hasLinearHistoryRule(vc.CommitRules)

	results := make([]ValidationResult, len(validatedCommits))

	var mergeSkipped int

	if len(validatedCommits) >= parallelValidationThreshold {
		mergeSkipped = validateCommitsParallel(ctx, validatedCommits, vc, linearHistoryActive, results)
	} else {
		mergeSkipped = validateCommitsSequential(ctx, validatedCommits, vc, linearHistoryActive, results)
	}

	logValidationStatistics(commits, validatedCommits, results, ignoreInfo, mergeSkipped, vc.Logger)

	// ctx.Err() is non-nil iff a path bailed early on cancellation, leaving
	// zero-value slots. Surfacing it here makes the partial-results hazard a
	// type-level contract instead of a doc-comment caveat callers can miss.
	return results, ignoreInfo, ctx.Err()
}

// validateCommitsSequential is the original loop, retained for the small-batch
// path where parallel-dispatch overhead exceeds the gain.
func validateCommitsSequential(ctx context.Context, commits []Commit, vc *ValidationContext, linearHistoryActive bool, results []ValidationResult) int {
	mergeSkipped := 0

	for idx, commit := range commits {
		select {
		case <-ctx.Done():
			vc.Logger.Warn("Validation cancelled, returning partial results",
				"completed", idx, "total", len(commits))

			return mergeSkipped
		default:
		}

		result, reason := validateCommitTraced(commit, vc.CommitRules, vc.Config, vc.Logger, linearHistoryActive)
		if reason == skipReasonMergeNoRule {
			mergeSkipped++
		}

		results[idx] = result
	}

	return mergeSkipped
}

// validateCommitsParallel applies rules across commits using a fixed worker
// pool. Order preservation is structural — workers write to results[idx] —
// so no reorder buffer is needed. Output to the user is identical to the
// sequential path.
//
// Why a fixed worker pool (vs goroutine-per-commit + semaphore): for very
// large batches (20k+ commits), spawning 20k goroutines incurs measurable
// scheduler + stack-allocation overhead that exceeds the gain. A pool of N
// long-lived workers pulling from a channel keeps fixed-cost low; the cost
// of one channel receive (~50 ns) is far below one goroutine spawn (~3 μs).
//
// Safety: validateCommitTraced takes (commit, rules, config, logger,
// linearHistoryActive). Commit is per-call; rules are read-only after rule
// construction; config is read-only; the zerolog-backed logger is documented
// thread-safe; linearHistoryActive is a bool. No shared mutable state in
// the rule layer.
//
// Cancellation: workers check ctx between jobs; the dispatcher closes the
// jobs channel on cancel so workers drain cleanly. Partial results are
// returned (whatever was already written to results[idx]).
//
// Panics: a rule that panics is a bug in the rule, not a validation failure.
// The first worker panic stops dispatch and is re-panicked here, in the
// caller's goroutine, once every worker has returned. The process-level panic
// boundary therefore reports it exactly as it reports a panic raised by the
// sequential path.
func validateCommitsParallel(ctx context.Context, commits []Commit, vc *ValidationContext, linearHistoryActive bool, results []ValidationResult) int {
	workers := max(min(runtime.GOMAXPROCS(0), maxParallelValidationWorkers), 1)

	vc.Logger.Debug("Validating commits in parallel",
		"commit_count", len(commits),
		"workers", workers)

	var (
		mergeSkipped atomic.Int64
		wg           sync.WaitGroup
	)

	firstPanic := newWorkerPanic()
	jobs := make(chan int, workers*2)

	for range workers {
		wg.Go(func() {
			validateCommitWorker(ctx, commits, vc, linearHistoryActive, results, jobs, &mergeSkipped, firstPanic)
		})
	}

	// Dispatcher: feed indices into the jobs channel. Stops on context
	// cancellation or on the first worker panic, closes the channel so workers
	// exit cleanly.
DISPATCH:
	for idx := range commits {
		select {
		case <-ctx.Done():
			vc.Logger.Warn("Validation cancelled, draining in-flight workers",
				"dispatched", idx, "total", len(commits))

			break DISPATCH
		case <-firstPanic.raised:
			break DISPATCH
		case jobs <- idx:
		}
	}

	close(jobs)
	wg.Wait()

	// Every worker has returned, so reading the recorded value is race-free.
	if value, panicked := firstPanic.value(); panicked {
		panic(value)
	}

	return int(mergeSkipped.Load())
}

// workerPanic carries the first panic raised by a validation worker back to the
// goroutine that started the pool.
type workerPanic struct {
	once     sync.Once
	recorded any
	raised   chan struct{}
}

func newWorkerPanic() *workerPanic {
	return &workerPanic{raised: make(chan struct{})}
}

// record keeps the first panic value and releases the dispatcher.
func (p *workerPanic) record(value any) {
	p.once.Do(func() {
		p.recorded = value
		close(p.raised)
	})
}

// value reports the recorded panic. It must only be called once every worker
// has returned.
func (p *workerPanic) value() (any, bool) {
	select {
	case <-p.raised:
		return p.recorded, true
	default:
		return nil, false
	}
}

// validateCommitWorker consumes indices from jobs and validates the
// corresponding commit, writing the result into results[idx].
//
// A panic inside validateCommitTraced is recorded and ends this worker: a
// goroutine panic would otherwise kill the process without reaching the panic
// boundary that writes the crash report. validateCommitsParallel re-panics the
// recorded value once every worker has returned, so the sequential and the
// parallel path produce the same output.
func validateCommitWorker(ctx context.Context, commits []Commit, vc *ValidationContext, linearHistoryActive bool, results []ValidationResult, jobs <-chan int, mergeSkipped *atomic.Int64, firstPanic *workerPanic) {
	for idx := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// A recovered panic leaves the result at its zero value, so completed
		// is true only when the commit was validated without panicking.
		completed := func() bool {
			defer func() {
				if r := recover(); r != nil {
					firstPanic.record(r)
				}
			}()

			result, reason := validateCommitTraced(commits[idx], vc.CommitRules, vc.Config, vc.Logger, linearHistoryActive)
			if reason == skipReasonMergeNoRule {
				mergeSkipped.Add(1)
			}

			results[idx] = result

			return true
		}()

		if !completed {
			return
		}
	}
}

// logValidationStatistics logs the final validation statistics.
func logValidationStatistics(allCommits, validatedCommits []Commit, results []ValidationResult, ignoreInfo CommitIgnoreInfo, mergeSkipped int, logger Logger) {
	failedCount := 0

	for _, result := range results {
		if result.HasFailures() {
			failedCount++
		}
	}

	logger.Debug("Multiple commits validation completed",
		"total", len(allCommits),
		"validated", len(validatedCommits),
		"ignored", ignoreInfo.IgnoredCount,
		"merge_skipped", mergeSkipped,
		"failed", failedCount,
		"passed", len(validatedCommits)-failedCount)
}

// ValidateRepositoryWithInfo validates repository rules and collects pass details.
func ValidateRepositoryWithInfo(ctx context.Context, rules []RepositoryRule, repo BranchAnalyzer, logger Logger) ([]ValidationError, map[string]string) {
	logger.Trace("Entering ValidateRepository", "rule_count", len(rules))

	defer func() {
		logger.Trace("Exiting ValidateRepository")
	}()

	errors := make([]ValidationError, 0, len(rules)*2)
	info := make(map[string]string)

	for _, rule := range rules {
		ruleInfo, ruleErrors := rule.Validate(ctx, repo)

		logger.Debug("Repository rule validated", "rule", rule.Name(), "errors", len(ruleErrors))
		errors = append(errors, ruleErrors...)

		if ruleInfo != "" && len(ruleErrors) == 0 {
			info[rule.Name()] = ruleInfo
		}
	}

	logger.Debug("Repository validation completed", "total_rules", len(rules), "total_errors", len(errors))

	return errors, info
}

// CommitRule defines interface for rules that only need commit data.
type CommitRule interface {
	// Name returns the rule's name.
	Name() string

	// Validate performs commit validation.
	Validate(commit Commit) []ValidationError
}

// CheckDescriber is an optional interface for rules that perform multiple
// sub-checks. Rules implementing this return labels for each active check
// based on configuration, allowing output formatters to display them
// (e.g. "subject" with checks "length, case, suffix").
type CheckDescriber interface {
	ActiveChecks() []string
}

// RepositoryRule defines interface for rules that need repository access.
// Validate returns an optional info string (non-empty on pass when the rule
// has useful context to report, e.g. "3/5 commits ahead") and any errors.
type RepositoryRule interface {
	// Name returns the rule's name.
	Name() string

	// Validate performs validation with repository access.
	// The returned string is a human-readable pass message; empty means no info.
	Validate(ctx context.Context, repo BranchAnalyzer) (string, []ValidationError)
}

// FixupCommitRuleName is the source ID emitted by the dispatcher when a fixup
// commit is rejected (no AllowFixupCommits opt-in). It isn't a CommitRule —
// the dispatcher produces the error directly — but it appears in
// ValidationError.Rule so it's defined here for a single source of truth.
const FixupCommitRuleName = diagnostics.SourceIDFixup

// hasLinearHistoryRule reports whether the LinearHistory rule is in the active rule set.
func hasLinearHistoryRule(rules []CommitRule) bool {
	for _, rule := range rules {
		if rule.Name() == rulecatalog.LinearHistory {
			return true
		}
	}

	return false
}

// ValidateCommitRules validates commit using CommitRule implementations.
func ValidateCommitRules(commit Commit, rules []CommitRule, logger Logger) []ValidationError {
	logger.Trace("Entering ValidateCommitRules", "commit_hash", commit.Hash, "rule_count", len(rules))

	defer func() {
		logger.Trace("Exiting ValidateCommitRules", "commit_hash", commit.Hash)
	}()

	errors := collectErrors(rules, func(rule CommitRule) []ValidationError {
		ruleErrors := rule.Validate(commit)
		logger.Debug("Commit rule validated", "rule", rule.Name(), "commit_hash", commit.Hash, "errors", len(ruleErrors))

		return ruleErrors
	})

	logger.Debug("Commit rules validation completed", "commit_hash", commit.Hash, "total_rules", len(rules), "total_errors", len(errors))

	return errors
}

// Validation helpers for functional composition.

// collectErrors applies a validation function to each item and collects all errors.
func collectErrors[T any](items []T, validate func(T) []ValidationError) []ValidationError {
	// Pre-allocate with estimate: average 2 errors per item
	errors := make([]ValidationError, 0, len(items)*2)
	for _, item := range items {
		errors = append(errors, validate(item)...)
	}

	return errors
}

// CombineValidationErrors concatenates multiple validation error slices into a single slice.
func CombineValidationErrors(errorSlices ...[]ValidationError) []ValidationError {
	// Calculate total capacity
	totalLen := 0
	for _, slice := range errorSlices {
		totalLen += len(slice)
	}

	result := make([]ValidationError, 0, totalLen)
	for _, slice := range errorSlices {
		result = append(result, slice...)
	}

	return result
}
