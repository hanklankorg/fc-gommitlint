// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import "codefloe.com/itiquette/gommitlint/internal/domain"

// TestCommits provides reusable test commit fixtures for tests.
var TestCommits = struct {
	ValidConventional   domain.Commit
	InvalidConventional domain.Commit
	ValidWithScope      domain.Commit
	BreakingChange      domain.Commit
	EmptyDescription    domain.Commit
}{
	ValidConventional:   CreateTestCommit("abc123", "feat: add new feature"),
	InvalidConventional: CreateTestCommit("def456", "not conventional at all"),
	ValidWithScope:      CreateTestCommit("ghi789", "feat(api): add endpoint"),
	BreakingChange:      CreateTestCommit("jkl012", "feat!: breaking API change"),
	EmptyDescription:    CreateTestCommit("yza567", "feat:"),
}

// CreateTestCommit is a helper to create custom test commits.
func CreateTestCommit(hash, message string) domain.Commit {
	return domain.NewCommit(
		hash,
		message,
		"Test Author",
		"test@example.com",
		"2024-01-01",
		"",
		false,
	)
}
