// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Validation benchmarks against fixed-size synthetic commit fixtures.
//
// Purpose: tracking regressions in ValidateCommits and surfacing hot allocations.
// Run with -benchmem to see allocs/op; -cpuprofile to feed pprof. Examples:
//
//	go test -bench=BenchmarkValidateCommits -benchmem ./internal/domain/...
//	go test -bench=BenchmarkValidateCommits_5000 -cpuprofile=cpu.out ./internal/domain/...
//	go tool pprof -web cpu.out
//
// The fixtures are deterministic so numbers are comparable across runs and
// across machines (modulo wall-time differences). Each commit's subject and
// hash are computed from its index — no randomness, no I/O.
//
// Rule choice: the benchmarks use a mix of passing + failing synthetic rules
// from helpers_internal_test.go so they exercise both happy and failure code
// paths inside ValidateCommits without depending on the production rule
// implementations (which would couple this micro-benchmark to rule behaviour).

func benchmarkCommits(n int) []Commit {
	commits := make([]Commit, n)
	for i := range commits {
		commits[i] = Commit{
			Hash:    fmt.Sprintf("%040x", i+1),
			Subject: fmt.Sprintf("feat: bench commit %d", i+1),
			Message: fmt.Sprintf("feat: bench commit %d", i+1),
		}
	}

	return commits
}

func benchmarkRules() []CommitRule {
	return []CommitRule{
		createPassingRule(rulecatalog.Subject),
		createPassingRule(rulecatalog.Conventional),
		createFailingRule(rulecatalog.SignOff, New(rulecatalog.SignOff, diagnostics.ReasonInsufficientSignoffs, "Add a sign-off")),
		createPassingRule(rulecatalog.Spell),
		createPassingRule(rulecatalog.CryptoSignature),
	}
}

func runValidateBench(b *testing.B, commits []Commit) {
	b.Helper()

	rules := benchmarkRules()
	cfg := settings.Config{}
	vc := &ValidationContext{
		BranchAnalyzer: testRepository{},
		CommitRules:    rules,
		Config:         cfg,
		Logger:         noopLogger(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _, _ = ValidateCommits(b.Context(), commits, vc)
	}
}

// BenchmarkValidateCommits_10 sits below the parallelisation threshold
// (parallelValidationThreshold = 50). Measures the sequential fast path used
// by commit-msg hooks and small PR validations.
func BenchmarkValidateCommits_10(b *testing.B) {
	runValidateBench(b, benchmarkCommits(10))
}

// BenchmarkValidateCommits_100 sits just above the threshold; first
// data point for the parallel path. Useful for tracking the overhead
// of goroutine setup at the small-batch end.
func BenchmarkValidateCommits_100(b *testing.B) {
	runValidateBench(b, benchmarkCommits(100))
}

// BenchmarkValidateCommits_1000 is the realistic upper bound for typical
// branch validation. Should show the parallel path's steady-state speedup.
func BenchmarkValidateCommits_1000(b *testing.B) {
	runValidateBench(b, benchmarkCommits(1000))
}

// BenchmarkValidateCommits_5000 exercises the high end. Useful for catching
// regressions in allocation cost or GC pressure that only surface at scale.
func BenchmarkValidateCommits_5000(b *testing.B) {
	runValidateBench(b, benchmarkCommits(5000))
}
