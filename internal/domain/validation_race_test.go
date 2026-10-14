// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"context"
	"sync"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestValidateCommits_ParallelIsRaceFree exercises the worker-pool path
// (validateCommitsParallel, gated at parallelValidationThreshold = 50)
// under conditions the general suite under-stresses:
//
//   - batch size comfortably above the threshold so the parallel path is
//     selected
//   - multiple ValidateCommits calls overlapping in goroutines so the race
//     detector observes any shared writes the worker pool might introduce
//   - determinism assertion: results[i].Commit.Hash == commits[i].Hash for
//     every i, confirming order preservation under contention
//
// Run with `go test -race ./internal/domain/...` (the justfile's `just
// test` target already does this). The test is cheap (~ms) and catches
// regressions where someone accidentally introduces shared mutable state
// in a rule struct or in validateCommitWorker.
func TestValidateCommits_ParallelIsRaceFree(t *testing.T) {
	t.Parallel()

	const (
		commitCount     = 200 // > parallelValidationThreshold (50)
		concurrentCalls = 8
		repetitionsPer  = 4
	)

	commits := benchmarkCommits(commitCount)

	// Each call gets its own ValidationContext to keep the test honest about
	// the per-call invariants; the rule slice is shared (rules are expected
	// to be read-only after construction — that's the property being tested).
	rules := benchmarkRules()

	var wg sync.WaitGroup

	wg.Add(concurrentCalls)

	for range concurrentCalls {
		go func() {
			defer wg.Done()

			vc := &ValidationContext{
				BranchAnalyzer: testRepository{},
				CommitRules:    rules,
				Config:         settings.Config{},
				Logger:         noopLogger(),
			}

			for range repetitionsPer {
				results, _, _ := ValidateCommits(t.Context(), commits, vc)

				if len(results) != commitCount {
					t.Errorf("result count = %d, want %d", len(results), commitCount)

					return
				}

				// Order preservation is the structural property the worker pool
				// relies on: results[i] corresponds to commits[i] regardless of
				// which worker ran it.
				for i := range results {
					if commits[i].Hash != results[i].Commit.Hash {
						t.Errorf("results[%d] hash = %q, want %q", i, results[i].Commit.Hash, commits[i].Hash)

						return
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestValidateCommits_PartialResultsOnCancel locks in the cancellation
// contract: a cancelled run returns a non-nil error alongside the partial
// slice. The slice stays full-length (callers index by position) but its
// un-dispatched slots are zero-value ValidationResult{} — the error is what
// tells callers those are not "passed" commits.
//
// This is a regression guard: if someone ever drops the returned error, or
// "fixes" the partial-results behaviour by truncating the slice, this test
// catches it and forces an explicit decision.
func TestValidateCommits_PartialResultsOnCancel(t *testing.T) {
	t.Parallel()

	commits := benchmarkCommits(200) // above parallel threshold

	vc := &ValidationContext{
		BranchAnalyzer: testRepository{},
		CommitRules:    benchmarkRules(),
		Config:         settings.Config{},
		Logger:         noopLogger(),
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel so the dispatcher exits on its first send attempt

	results, _, err := ValidateCommits(ctx, commits, vc)

	check.Len(t, results, len(commits),
		"results slice must remain the same length as input on cancel — callers index by position")

	check.Error(t, err,
		"ValidateCommits must return a non-nil error on cancellation so callers never build a report from zero-value result slots")
}
