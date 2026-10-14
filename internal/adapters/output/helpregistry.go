// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"strconv"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

// LookupHelpMessage returns remediation guidance for the given diagnostic reason.
// Dynamic values are extracted from ctx (ValidationError.Context).
// Returns ok=false when no help text is registered for the reason.
func LookupHelpMessage(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	if msg, ok := lookupSubjectHelp(reason, ctx); ok {
		return msg, true
	}

	if msg, ok := lookupImperativeAndBodyHelp(reason, ctx); ok {
		return msg, true
	}

	if msg, ok := lookupSignoffAndSpellHelp(reason, ctx); ok {
		return msg, true
	}

	if msg, ok := lookupJiraHelp(reason); ok {
		return msg, true
	}

	if msg, ok := lookupIdentityAndCryptoHelp(reason, ctx); ok {
		return msg, true
	}

	return lookupBranchHelp(reason, ctx)
}

//nolint:cyclop // lookup table: each case is a distinct reason, complexity is structural not logical
func lookupSubjectHelp(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonEmptySubject:
		if _, ok := ctx[diagnostics.ContextSubject]; ok {
			return Msg("Provide a non-empty commit message subject"), true
		}

		return Msg("Add a non-empty subject line"), true

	case diagnostics.ReasonSubjectTooLong:
		over := ctx[diagnostics.ContextOver]
		maxLen := ctx[diagnostics.ContextExpectedMax]

		if over != "" && maxLen != "" {
			return Msg("Shorten your commit message subject line by %s characters — aim for under %s characters", over, maxLen), true
		}

		return Msg("Shorten your commit message subject line"), true

	case diagnostics.ReasonWrongCaseLower:
		return Msg("Use lowercase for the first character of your commit subject"), true

	case diagnostics.ReasonWrongCaseUpper:
		return Msg("Use uppercase for the first character of your commit subject"), true

	case diagnostics.ReasonWrongCaseSentence:
		return Msg("Use sentence case (uppercase first character) for your commit subject"), true

	case diagnostics.ReasonSubjectCase:
		return Msg("Check the case of the first character of your commit subject"), true

	case diagnostics.ReasonSubjectSuffix:
		if suffixes := ctx[diagnostics.ContextInvalidSuffixes]; suffixes != "" {
			return Msg("Remove the punctuation or special character from the end of your subject line — it should end with a letter or number, not: %s", suffixes), true
		}

		return Msg("Remove the punctuation or special character from the end of your subject line"), true

	case diagnostics.ReasonInvalidUTF8:
		return Msg("Ensure your commit message contains only valid UTF-8 characters"), true

	case diagnostics.ReasonInvalidFormat, diagnostics.ReasonMissingConventionalSubject:
		return Msg("Format your commit message according to the proper convention"), true

	case diagnostics.ReasonInvalidConventionalFormat:
		return Msg("Use format: %s (e.g., 'feat: add login')", domain.ConventionalCommitFormatTemplate), true

	case diagnostics.ReasonInvalidConventionalType:
		if allowed := ctx[diagnostics.ContextExpected]; allowed != "" {
			return Msg("Use one of: %s", allowed), true
		}

		return Msg("Use one of the allowed commit types"), true

	case diagnostics.ReasonConventionalDescTooLong:
		if maxLen := ctx[diagnostics.ContextExpectedMax]; maxLen != "" {
			return Msg("Keep description under %s characters", maxLen), true
		}

		return Msg("Keep the description under the maximum allowed length"), true

	case diagnostics.ReasonInvalidSpacing:
		return Msg("Use exactly one space after the colon followed by description text"), true

	case diagnostics.ReasonEmptyConventionalDesc:
		return Msg("Add a meaningful description explaining what the commit does"), true

	case diagnostics.ReasonBreakingChangeNotAllowed:
		return Msg("Remove the breaking change indicator (!) from the commit type"), true

	case diagnostics.ReasonMissingConventionalScope:
		return Msg("Use format: %s", domain.ConventionalCommitFormatTemplate), true

	case diagnostics.ReasonMultiScopeNotAllowed:
		return Msg("Use a single scope: %s", domain.ConventionalCommitFormatTemplate), true

	case diagnostics.ReasonInvalidConventionalScope:
		if allowed := ctx[diagnostics.ContextExpected]; allowed != "" {
			return Msg("Use one of: %s", allowed), true
		}

		return Msg("Use one of the allowed commit scopes"), true

	case diagnostics.ReasonInvalidMultiScope:
		return Msg("Use comma-separated scopes without spaces: (scope1,scope2)"), true

	default:
		return LocaleMsg{}, false
	}
}

//nolint:cyclop // lookup table: each case is a distinct reason, complexity is structural not logical
func lookupImperativeAndBodyHelp(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonNoFirstWord:
		return Msg("Start your commit message with a verb"), true

	case diagnostics.ReasonNonVerb:
		return Msg("Start with an imperative verb like: add, fix, update, remove, implement"), true

	case diagnostics.ReasonPastTense:
		if suggestion := ctx[diagnostics.ContextSuggestion]; suggestion != "" {
			return Msg("Use the base form '%s' instead of past tense", suggestion), true
		}

		return Msg("Use the base (imperative) form instead of past tense"), true

	case diagnostics.ReasonGerund:
		if suggestion := ctx[diagnostics.ContextSuggestion]; suggestion != "" {
			return Msg("Use the base form '%s' instead of the gerund (present participle)", suggestion), true
		}

		return Msg("Use the base (imperative) form instead of the gerund (present participle)"), true

	case diagnostics.ReasonThirdPerson:
		if suggestion := ctx[diagnostics.ContextSuggestion]; suggestion != "" {
			return Msg("Use the base form '%s' instead of third person singular", suggestion), true
		}

		return Msg("Use the base (imperative) form instead of third person singular"), true

	case diagnostics.ReasonNonImperative:
		return Msg("Use an imperative verb like: add, fix, update, remove, implement"), true

	case diagnostics.ReasonInvalidStructure:
		return Msg("Use format: subject line, blank line, then detailed body"), true

	case diagnostics.ReasonMissingBlankLine:
		return Msg("Git convention requires a blank line between subject and body"), true

	case diagnostics.ReasonMissingBody:
		return Msg("Add a blank line after the subject, followed by a detailed description"), true

	case diagnostics.ReasonBodyTooShort:
		if minLen := ctx[diagnostics.ContextMin]; minLen != "" {
			return Msg("Provide at least %s characters of detail explaining the change", minLen), true
		}

		return Msg("Add more detail to the commit body"), true

	case diagnostics.ReasonBodyLineTooLong:
		if maxLen := ctx[diagnostics.ContextMax]; maxLen != "" {
			return Msg("Wrap body lines at %s characters for better readability in git log", maxLen), true
		}

		return Msg("Wrap long body lines for better readability in git log"), true

	case diagnostics.ReasonInvalidBody:
		if ctx[diagnostics.ContextActual] == "starts with sign-off" {
			return Msg("Start with actual content explaining your changes, then add sign-off lines at the end"), true
		}

		return Msg("Add a detailed description before the sign-off line"), true

	default:
		return LocaleMsg{}, false
	}
}

func lookupSignoffAndSpellHelp(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonMissingSignoff:
		return Msg("Add DCO sign-off line: 'Signed-off-by: Your Real Name <your.email@domain.com>'"), true

	case diagnostics.ReasonInvalidSignoffFormat:
		return Msg("Use strict DCO format: 'Signed-off-by: Your Real Name <your.email@domain.com>'"), true

	case diagnostics.ReasonMisplacedSignoff:
		return Msg("Move all sign-off lines to the end"), true

	case diagnostics.ReasonInsufficientSignoffs:
		return Msg("Each sign-off must be from a different person"), true

	case diagnostics.ReasonMisspelledWord:
		word := ctx[diagnostics.ContextWord]
		suggestion := ctx[diagnostics.ContextSuggestion]

		if word != "" && suggestion != "" {
			return Msg("Replace %q with %q", word, suggestion), true
		}

		return Msg("Fix the spelling error"), true

	case diagnostics.ReasonSpellCheckFailed:
		return Msg("Ensure the spell checker is properly configured"), true

	default:
		return LocaleMsg{}, false
	}
}

func lookupJiraHelp(reason diagnostics.Reason) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonMissingJira:
		return Msg("Add a JIRA ticket reference like 'PROJ-123' to your commit message"), true

	case diagnostics.ReasonMissingJiraKeySubject:
		return Msg("Add JIRA ticket to subject (e.g., 'PROJ-123: description')"), true

	case diagnostics.ReasonMissingJiraKeyBody:
		return Msg("Add JIRA reference like 'Refs: PROJ-123' in the body"), true

	case diagnostics.ReasonInvalidJiraFormat:
		return Msg("Use valid JIRA format: PROJ-123 (uppercase letters, hyphen, numbers only)"), true

	case diagnostics.ReasonJiraKeyNotAtEnd:
		return Msg("Move JIRA to end: 'feat: description PROJ-123' or in scope: 'feat(PROJ-123): description'"), true

	case diagnostics.ReasonInvalidRefsFormat:
		return Msg("Use format: 'Refs: PROJ-123' (with colon)"), true

	case diagnostics.ReasonRefsAfterSignoff:
		return Msg("Move Refs lines before Signed-off-by lines"), true

	case diagnostics.ReasonInvalidKeyFormat:
		return Msg("Use uppercase for JIRA keys: 'Refs: PROJ-123'"), true

	case diagnostics.ReasonInvalidProject:
		return Msg("Use format: 'Refs: PROJ-123'"), true

	default:
		return LocaleMsg{}, false
	}
}

//nolint:cyclop // lookup table: each case is a distinct reason, complexity is structural not logical
func lookupIdentityAndCryptoHelp(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonIdentityNotAllowed:
		return Msg("Use an authorized email address or add this email to allowed_authors"), true

	case diagnostics.ReasonInvalidConfig:
		// Only the identity rule attaches invalid_entry; other invalid-config
		// diagnostics fall through to lookupBranchHelp.
		if entry, ok := ctx[diagnostics.ContextInvalidEntry]; ok {
			if formats := ctx[contextValidFormats]; formats != "" {
				return Msg("Fix the allowed_authors entry '%s' — use %s", entry, formats), true
			}

			return Msg("Fix the allowed_authors entry '%s'", entry), true
		}

		return LocaleMsg{}, false

	case diagnostics.ReasonMissingSignature:
		return Msg("Sign your commits using 'git commit -S' for GPG or configure SSH signing"), true

	case diagnostics.ReasonUnknownSigFormat:
		return Msg("Ensure your signing key is properly configured"), true

	case diagnostics.ReasonInvalidKeyDirectory:
		return Msg("Ensure the key directory exists and is readable"), true

	case diagnostics.ReasonIncompleteGPGSig:
		return Msg("Ensure GPG signature includes complete BEGIN/END markers"), true

	case diagnostics.ReasonIncompleteSSHSig:
		return Msg("Ensure SSH signature includes complete BEGIN/END markers"), true

	case diagnostics.ReasonSignatureTypeNotAllowed:
		if ctx[diagnostics.ContextExpected] == "GPG" {
			return Msg("Configure GPG signing: git config --global commit.gpgsign true"), true
		}

		return Msg("Configure SSH signing: git config --global gpg.format ssh"), true

	case diagnostics.ReasonNoTrustedKeys:
		keyType := ctx[diagnostics.ContextType]
		rejected := ctx[diagnostics.ContextRejected]

		if keyType == "" {
			return Msg("Add eligible public keys to the key directory"), true
		}

		if rejected != "" {
			return Msg("Add valid %s public keys to the key directory — keys rejected: %s", keyType, rejected), true
		}

		return Msg("Add valid %s public keys to the key directory", keyType), true

	case diagnostics.ReasonKeyLoadFailed:
		keyType := ctx[diagnostics.ContextType]

		if keyType == "" {
			return Msg("Ensure public keys are present in the key directory"), true
		}

		return Msg("Ensure %s public keys are present in the key directory", keyType), true

	default:
		return LocaleMsg{}, false
	}
}

//nolint:cyclop // lookup table: each case is a distinct reason, complexity is structural not logical
func lookupBranchHelp(reason diagnostics.Reason, ctx map[string]string) (LocaleMsg, bool) {
	switch reason {
	case diagnostics.ReasonInvalidReference:
		return Msg("Use a valid commit hash, branch name, or tag"), true

	case diagnostics.ReasonInvalidCommitRange:
		return Msg("Use format: <from>..<to> or <from>...<to>"), true

	case diagnostics.ReasonInvalidConfig:
		if _, ok := ctx[diagnostics.ContextPattern]; ok {
			return Msg("Ensure all ignore_ticket_patterns are valid regular expressions"), true
		}

		if _, ok := ctx[diagnostics.ContextRequireGPG]; ok {
			return Msg("Set only one of require_gpg or require_ssh to true, or set both to false to allow either type"), true
		}

		return Msg("Set repo.max_commits_ahead to a positive number, or disable the rule by adding 'branchahead' to rules.disabled"), true

	case diagnostics.ReasonMissingReference:
		if ref := ctx[diagnostics.ContextRef]; ref != "" {
			return Msg("Ensure the branch '%s' exists, or use a full ref like 'origin/%s' if only the remote branch exists", ref, ref), true
		}

		return Msg("Provide a base branch with --base-branch (e.g., --base-branch main)"), true

	case diagnostics.ReasonTooManyCommits:
		ref := ctx[diagnostics.ContextRef]

		if ref == "" {
			return Msg("Consider squashing excess commits before merging"), true
		}

		disableNote := "\nDisable: rules.disabled: [\"branchahead\"]"
		over, _ := strconv.Atoi(ctx[diagnostics.ContextOver])

		switch {
		case over > 0 && over <= 3:
			return Msg("Consider squashing %d excess commits: 'git rebase -i %s'%s", over, ref, disableNote), true
		case over > 3 && over <= 10:
			return Msg("Squash related commits: 'git rebase -i %s'%s", ref, disableNote), true
		default:
			return Msg("Consider smaller PRs or rebase: 'git rebase -i %s'%s", ref, disableNote), true
		}

	case diagnostics.ReasonMergeCommitFound:
		return Msg("Rebase your branch instead of merging: 'git rebase <target-branch>', or use 'git merge --ff-only <target-branch>' for fast-forward only. Note: rebase recreates commits, removing signatures and signoffs. Add --gpg-sign and/or --signoff as needed. Disable rule:\n  Config File: rules.disabled: [\"linearhistory\"]\n  CLI Flag: --exclude-rule linearhistory"), true

	case diagnostics.ReasonFixupCommitFound:
		return Msg("Use 'git rebase -i <base>' and mark fixup commits as 'squash'. Note: rebase recreates commits, removing signatures and signoffs. Add --gpg-sign and/or --signoff as needed. Disable rule:\n  Config File: allow_fixup_commits: true"), true

	default:
		return LocaleMsg{}, false
	}
}
