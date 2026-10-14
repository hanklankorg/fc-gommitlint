// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// ruleHelp is the single source of help metadata for one rule: the statically
// extractable i18n key, fallback English template, format args, and config keys.
type ruleHelp struct {
	message    output.LocaleMsg
	template   string
	args       []any
	configKeys []string
}

const (
	ruleConventional    = rulecatalog.Conventional
	ruleSubject         = rulecatalog.Subject
	ruleJiraReference   = rulecatalog.JiraReference
	ruleSignoff         = rulecatalog.SignOff
	ruleSpell           = rulecatalog.Spell
	ruleCryptoSignature = rulecatalog.CryptoSignature
	ruleIdentity        = rulecatalog.Identity
	ruleCommitBody      = rulecatalog.CommitBody
	ruleBranchAhead     = rulecatalog.BranchAhead
	ruleLinearHistory   = rulecatalog.LinearHistory
)

func ruleHelpRegistry() map[string]ruleHelp {
	return map[string]ruleHelp{
		rulecatalog.Conventional: {
			message: output.Msg("rulehelp.conventional"),
			template: `CONVENTIONAL COMMIT RULE

Validates commit messages follow conventional commit format: %s

FORMAT:
  %s
  
VALID TYPES:
  %s

EXAMPLES:
  ✓ feat: add user authentication
  ✓ fix(api): resolve login timeout issue  
  ✓ docs: update README installation steps
  ✗ Add user auth (missing type prefix)
  ✗ feat add auth (missing colon)
`,
			args: []any{domain.ConventionalCommitFormatTemplate, domain.ConventionalCommitFormatTemplate, strings.Join(settings.GetDefaultConventionalTypes(), ", ")},
			configKeys: []string{
				"conventional.types",
				"conventional.scopes",
				"conventional.require_scope",
				"conventional.allow_multi_scope",
				"conventional.max_description_length",
				"conventional.disallow_breaking",
			},
		},
		rulecatalog.Subject: {
			message: output.Msg("rulehelp.subject"),
			template: `SUBJECT RULE

Validates commit subject line format, length, and content requirements.

REQUIREMENTS:
  • Length within configured maximum
  • No trailing punctuation
  • Imperative mood when enabled
  • Case rules when configured

EXAMPLES:
  ✓ Add user authentication system
  ✓ Fix memory leak in parser
  ✗ Added user authentication (past tense)
  ✗ Fix memory leak. (trailing punctuation)
  ✗ fix memory leak (not capitalized when sentence case is configured)
`,
			configKeys: []string{
				"message.subject.max_length",
				"message.subject.case",
				"message.subject.require_imperative",
				"message.subject.forbid_endings",
			},
		},
		rulecatalog.JiraReference: {
			message: output.Msg("rulehelp.jirareference"),
			template: `JIRA REFERENCE RULE

Validates that commits include valid JIRA issue references in subject or body.

VALID POSITIONS:
  • Subject: feat(PROJ-123): description
  • Subject end: feat: description PROJ-123  
  • Body: "Refs: PROJ-123"

EXAMPLES:
  ✓ feat(TEAM-456): add login feature
  ✓ fix: resolve timeout issue PROJ-123
  ✓ Body with "Refs: PROJ-123, TEAM-456"
  ✗ feat: add login (no JIRA reference)
  ✗ feat: add TEAM-abc (invalid format)
`,
			configKeys: []string{
				"jira.project_prefixes",
				"jira.require_in_subject",
				"jira.require_in_body",
				"jira.ignore_ticket_patterns",
			},
		},
		rulecatalog.SignOff: {
			message: output.Msg("rulehelp.signoff"),
			template: `SIGN-OFF RULE

Validates Developer Certificate of Origin (DCO) compliance with proper sign-off format.

REQUIRED FORMAT:
  Signed-off-by: Your Real Name <your.email@domain.com>

PLACEMENT:
  • Must be at end of commit body
  • Before any automated signatures
  • Multiple sign-offs allowed for co-authoring

EXAMPLES:
  ✓ Signed-off-by: John Doe <john@example.com>
  ✗ Signed-off-by: <john@example.com> (missing name)
  ✗ Signed-off-by:John Doe<john@example.com> (missing spaces)
`,
			configKeys: []string{"message.body.min_signoff_count"},
		},
		rulecatalog.Spell: {
			message: output.Msg("rulehelp.spell"),
			//nolint:misspell // The examples intentionally show common misspellings.
			template: `SPELL CHECKING RULE

Validates commit message text for spelling errors using misspell library.

SCOPE:
  • Subject and body text
  • Uses locale-specific misspelling replacements

EXAMPLES:
  ✓ Fix authentication timeout issue
  ✗ Subject containing typos like "teh" or "hte"
  ✗ Body with spelling mistakes triggers warnings
`,
			configKeys: []string{"spell.locale", "spell.ignore_words"},
		},
		rulecatalog.CryptoSignature: {
			message: output.Msg("rulehelp.cryptosignature"),
			template: `CRYPTO SIGNATURE RULE

Checks that a commit carries a signature (GPG/SSH) and that its block is
well-formed and of an accepted kind, plus optional key-inventory policy.

SCOPE (IMPORTANT):
  This rule does NOT cryptographically verify the signature against a key.
  It confirms a signature is present, correctly armored, of an accepted kind,
  and — when key_directory is set — that an eligible public key exists. It does
  not assert the commit was actually signed BY that key. Use your platform's
  signature verification (e.g. git verify-commit, forge branch protection) for
  real cryptographic trust.

VALIDATION:
  • Signature presence (when required: true)
  • Signature block is well-formed (when verify_format: true)
  • Signature kind enforcement (require_gpg/require_ssh)
  • An eligible public key exists in key_directory
  • Key strength requirements (min_rsa_bits/min_ec_bits)
  • Key expiration and revocation (GPG only)

EXAMPLES:
  ✓ Commit with a present, well-formed GPG signature block
  ✓ Commit with a present, well-formed SSH signature block
  ✗ Unsigned commit (when required: true)
  ✗ Incomplete/malformed signature block (when verify_format: true)
  ✗ SSH signature when require_gpg: true
`,
			configKeys: []string{
				"crypto_signature.required",
				"crypto_signature.verify_format",
				"crypto_signature.key_directory",
				"crypto_signature.require_gpg",
				"crypto_signature.require_ssh",
				"crypto_signature.min_rsa_bits",
				"crypto_signature.min_ec_bits",
				"crypto_signature.allowed_signers",
			},
		},
		rulecatalog.Identity: {
			message: output.Msg("rulehelp.identity"),
			template: `IDENTITY RULE

Validates commit author identity against configured allowed authors.

VALIDATION:
  • Matches allowed author patterns
  • Supports domain wildcards (e.g., "*@company.com")

EXAMPLES:
  ✓ John Doe <john.doe@company.com>
  ✓ Jane Doe <jane@company.com>
  ✗ John Doe <john@personal.com>
`,
			configKeys: []string{"identity.allowed_authors"},
		},
		rulecatalog.CommitBody: {
			message: output.Msg("rulehelp.commitbody"),
			template: `COMMIT BODY RULE

Validates commit message body format, length, and content requirements.

REQUIREMENTS:
  • Body required when enabled
  • Minimum body length
  • Line length limits  
  • Proper formatting

EXAMPLES:
  ✓ Properly formatted body with details
  ✗ Body line exceeding character limit
  ✗ Missing required body when configured
`,
			configKeys: []string{
				"message.body.required",
				"message.body.min_length",
				"message.body.max_line_length",
				"message.body.allow_signoff_only",
				"message.body.min_signoff_count",
			},
		},
		rulecatalog.BranchAhead: {
			message: output.Msg("rulehelp.branchahead"),
			template: `BRANCH AHEAD RULE

Validates that current branch is not too far ahead of the base branch.

PURPOSE:
  • Helps keep feature branches small
  • Prevents outdated merge requests
  • Maintains clean git history

NOTES:
  • Runs only when --base-branch is provided

EXAMPLES:
  ✓ Feature branch within the configured maximum
  ✓ Branch with no commits ahead of main
  ✗ Feature branch exceeds the configured maximum
`,
			configKeys: []string{"repo.max_commits_ahead"},
		},
		rulecatalog.LinearHistory: {
			message: output.Msg("rulehelp.linearhistory"),
			template: `LINEAR HISTORY RULE

Enforces a rebase-only workflow by rejecting merge commits in the commit range.

PURPOSE:
  • Maintains clean, linear git history
  • Prevents merge commits in feature branches
  • Encourages rebase workflow over merge workflow

EXAMPLES:
  ✓ Regular commits (non-merge)
  ✓ Commits after rebase onto main
  ✗ "Merge branch 'main' into feature"
  ✗ "Merge pull request #123 from user/branch"

RESOLUTION:
  Use 'git rebase <target-branch>' instead of 'git merge <target-branch>'
  to update your feature branch with upstream changes.
  Alternatively, use 'git merge --ff-only <target-branch>' for fast-forward merges.

  Basic rebase:
    git fetch origin
    git rebase origin/main

  IMPORTANT: Rebase recreates commits, which removes GPG signatures and
  Signed-off-by trailers. If you use signed commits or signoffs:

    git rebase origin/main --gpg-sign          # Re-sign with GPG
    git rebase origin/main --signoff           # Re-add Signed-off-by
    git rebase origin/main --gpg-sign --signoff  # Both

  TIP: Configure git to always sign on rebase:
    git config commit.gpgsign true

`,
		},
	}
}

func localizedRuleHelpTemplate(localizer output.Localizer, rule string) (string, string, bool) {
	entry, ok := ruleHelpRegistry()[rule]
	if !ok {
		return "", "", false
	}

	return entry.message.Localize(localizer), entry.message.ID, true
}

func formatRuleHelpContent(ruleName string, content string, localizer output.Localizer) string {
	configKeys := appendConfigKeys(
		getRuleConfigKeys(ruleName),
		"rules.enabled",
		"rules.disabled",
		"checks.enabled",
		"checks.disabled",
	)
	if len(configKeys) == 0 {
		return content
	}

	var builder strings.Builder
	builder.WriteString(content)

	if !strings.HasSuffix(content, "\n") {
		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	builder.WriteString(output.EnsureLocalizer(localizer).Get("CONFIGURATION:"))
	builder.WriteString("\n")

	for _, key := range configKeys {
		builder.WriteString("  ")
		builder.WriteString(key)
		builder.WriteString("\n")
	}

	return builder.String()
}

func getRuleConfigKeys(ruleName string) []string {
	entry, exists := ruleHelpRegistry()[ruleName]
	if !exists {
		return nil
	}

	result := make([]string, len(entry.configKeys))
	copy(result, entry.configKeys)

	return result
}

func appendConfigKeys(keys []string, extra ...string) []string {
	if len(keys) == 0 && len(extra) == 0 {
		return nil
	}

	result := make([]string, 0, len(keys)+len(extra))
	seen := make(map[string]bool, len(keys)+len(extra))

	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}

		seen[key] = true
		result = append(result, key)
	}

	for _, key := range extra {
		if key == "" || seen[key] {
			continue
		}

		seen[key] = true
		result = append(result, key)
	}

	return result
}
