// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

func formatterFixtureTime() time.Time {
	return time.Date(2025, 6, 14, 10, 0, 0, 0, time.UTC)
}

// reportBuilder assembles a domain.Report for formatter tests without the
// deeply nested struct literals the formatter suites would otherwise repeat.
// Commits and repository rules are added fluently; the summary counts
// (totals, passed/failed, the failed-rule map, repository failures and
// AllPassed) are derived from what was added. Fields that don't follow from
// the added commits/rules — ignored or skipped counts — are set via summary().
type reportBuilder struct {
	commits   []domain.CommitReport
	repoRules []domain.RuleReport
	metadata  domain.ReportMetadata
	overrides []func(*domain.ReportSummary)
}

func newReportBuilder() *reportBuilder {
	return &reportBuilder{}
}

// at sets the report metadata timestamp.
func (b *reportBuilder) at(ts time.Time) *reportBuilder {
	b.metadata.Timestamp = ts

	return b
}

// warn appends configuration warnings to the report metadata.
func (b *reportBuilder) warn(warnings ...domain.Warning) *reportBuilder {
	b.metadata.WarningDetails = append(b.metadata.WarningDetails, warnings...)

	return b
}

// configSources sets the config source list in the report metadata.
func (b *reportBuilder) configSources(sources ...string) *reportBuilder {
	b.metadata.ConfigSources = sources

	return b
}

// passingCommit adds a commit with no failing rules.
func (b *reportBuilder) passingCommit(hash, subject string) *reportBuilder {
	return b.commit(domain.CommitReport{
		Commit: domain.Commit{Hash: hash, Subject: subject},
		Passed: true,
	})
}

// failingCommit adds a commit that fails a single rule with the given errors.
func (b *reportBuilder) failingCommit(hash, subject, rule string, errs ...domain.ValidationError) *reportBuilder {
	return b.commit(domain.CommitReport{
		Commit: domain.Commit{Hash: hash, Subject: subject},
		Passed: false,
		RuleResults: []domain.RuleReport{{
			Name:   rule,
			Status: domain.StatusFailed,
			Errors: errs,
		}},
	})
}

// commit adds a fully-formed commit report for cases needing exotic fields.
func (b *reportBuilder) commit(report domain.CommitReport) *reportBuilder {
	b.commits = append(b.commits, report)

	return b
}

// repoRule adds a repository-level rule report.
func (b *reportBuilder) repoRule(report domain.RuleReport) *reportBuilder {
	b.repoRules = append(b.repoRules, report)

	return b
}

// summary registers an override applied to the derived summary, for the fields
// that can't be inferred from the added commits and rules.
func (b *reportBuilder) summary(override func(*domain.ReportSummary)) *reportBuilder {
	b.overrides = append(b.overrides, override)

	return b
}

func (b *reportBuilder) build() domain.Report {
	summary := domain.ReportSummary{
		TotalCommits:     len(b.commits),
		ValidatedCommits: len(b.commits),
		FailedRules:      map[string]int{},
	}

	for _, commit := range b.commits {
		if commit.Passed {
			summary.PassedCommits++
		} else {
			summary.FailedCommits++
		}

		for _, ruleResult := range commit.RuleResults {
			if ruleResult.Status == domain.StatusFailed {
				summary.FailedRules[ruleResult.Name] += len(ruleResult.Errors)
			}
		}
	}

	failedRepoRules := make(map[string]struct{})

	for _, ruleResult := range b.repoRules {
		if ruleResult.Status == domain.StatusFailed {
			summary.FailedRules[ruleResult.Name] += len(ruleResult.Errors)
			failedRepoRules[ruleResult.Name] = struct{}{}
		}
	}

	summary.RepoFailedRules = len(failedRepoRules)
	summary.AllPassed = summary.FailedCommits == 0 && summary.RepoFailedRules == 0

	for _, override := range b.overrides {
		override(&summary)
	}

	report := domain.Report{
		Metadata: b.metadata,
		Summary:  summary,
		Commits:  b.commits,
	}
	report.Repository.RuleResults = b.repoRules

	return report
}
