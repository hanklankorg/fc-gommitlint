// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package rules implements commit validation rules for gommitlint.
//
// # Rule Interfaces
//
// Rules implement one of two interfaces defined in the domain package:
//
//   - domain.CommitRule: For rules that only need commit data (subject, body, author, etc.)
//   - domain.RepositoryRule: For rules that need repository access (branch comparison, signatures)
//
// # Implementation Pattern
//
// All rules follow a consistent pattern:
//
//  1. Use value receivers (not pointer receivers) for immutability
//  2. Accept configuration via constructor when needed (e.g., NewSubjectRule(cfg settings.Config))
//  3. Return []domain.ValidationError from Validate method
//  4. Use fluent error building with domain.New().WithContextMap()
//  5. Rule enablement is handled by the factory, not internally by the rule
//
// Example implementation:
//
//	type MyRule struct {
//	    maxLength int
//	}
//
//	func NewMyRule(cfg settings.Config) MyRule {
//	    return MyRule{maxLength: settings.DerefPositiveInt(
//	        cfg.Message.Subject.MaxLength, settings.DefaultSubjectMaxLength,
//	    )}
//	}
//
//	func (r MyRule) Name() string { return "myrule" }
//
//	func (r MyRule) Validate(commit domain.Commit) []domain.ValidationError {
//	    if len(commit.Subject) > r.maxLength {
//	        return []domain.ValidationError{
//	            domain.New(r.Name(), diagnostics.ReasonSubjectTooLong, "Subject too long").
//	                WithContextMap(diagnostics.Context{
//	                    diagnostics.ContextActual:      strconv.Itoa(len(commit.Subject)),
//	                    diagnostics.ContextExpectedMax: strconv.Itoa(r.maxLength),
//	                }),
//	        }
//	    }
//	    return nil
//	}
//
// # Shared Patterns
//
// The patterns.go file contains compiled regex patterns shared across rules:
//
//   - SignOffPattern: Validates DCO sign-off format ("Signed-off-by: Name <email>")
//   - EmailPattern: Extracts email addresses from angle brackets
//   - TrailerPattern: Matches common Git trailer formats (Signed-off-by, Co-authored-by, Fixes, etc.)
//
// These patterns are compiled once at package initialization for performance.
//
// # Rule Categories
//
// CommitRules (validate single commits):
//   - subject: Subject line length validation
//   - conventional: configured Conventional Commit subject policy
//   - imperative: Imperative mood verb checking
//   - signoff: DCO sign-off validation
//   - commitbody: Commit body structure validation
//   - spell: Spelling validation
//   - jira: JIRA reference validation
//   - identity: Author identity validation
//   - linearhistory: Linear history checking (no merge commits)
//   - cryptosignature: GPG/SSH signature policy validation (presence, format, type, key inventory)
//
// RepositoryRules (require repository access):
//   - branchahead: Branch comparison with upstream
package rules
