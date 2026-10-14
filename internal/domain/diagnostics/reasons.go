// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package diagnostics

// Reasons for validation failures. Every reason belongs to a public diagnostic
// in Definitions, except ReasonMessageFileMissing (see AllValidationReasons);
// TestAllValidationReasons_MatchEveryDeclaredReason keeps this file and the
// catalog in step.
const (
	ReasonInvalidFormat Reason = "invalid_format"

	ReasonValidationFailed Reason = "validation_failed"

	ReasonSubjectTooLong Reason = "subject_too_long"

	ReasonSubjectCase                Reason = "invalid_case"
	ReasonSubjectSuffix              Reason = "invalid_suffix"
	ReasonMissingSubject             Reason = "missing_subject"
	ReasonNoFirstWord                Reason = "no_first_word"
	ReasonEmptyMessage               Reason = "empty_message"
	ReasonEmptySubject               Reason = "empty_subject"
	ReasonInvalidUTF8                Reason = "invalid_utf8"
	ReasonMissingConventionalSubject Reason = "missing_conventional_subject"
	ReasonWrongCaseUpper             Reason = "wrong_case_upper"
	ReasonWrongCaseLower             Reason = "wrong_case_lower"
	ReasonWrongCaseSentence          Reason = "wrong_case_sentence"

	ReasonInvalidBody      Reason = "invalid_body"
	ReasonMissingBody      Reason = "missing_body"
	ReasonBodyTooShort     Reason = "body_too_short"
	ReasonBodyLineTooLong  Reason = "body_line_too_long"
	ReasonMissingBlankLine Reason = "missing_blank_line"
	ReasonInvalidStructure Reason = "invalid_structure"

	ReasonDescriptionTooLong        Reason = "description_too_long"
	ReasonInvalidConventionalFormat Reason = "invalid_conventional_format"
	ReasonInvalidConventionalType   Reason = "invalid_conventional_type"
	ReasonMissingConventionalScope  Reason = "missing_conventional_scope"
	ReasonInvalidConventionalScope  Reason = "invalid_conventional_scope"
	ReasonConventionalDescTooLong   Reason = "conventional_desc_too_long"
	ReasonBreakingChangeNotAllowed  Reason = "breaking_change_not_allowed"
	ReasonInvalidMultiScope         Reason = "invalid_multi_scope"
	ReasonMultiScopeNotAllowed      Reason = "multi_scope_not_allowed"
	ReasonInvalidSpacing            Reason = "invalid_spacing"
	ReasonEmptyConventionalDesc     Reason = "empty_conventional_desc"

	ReasonMissingJira Reason = "missing_jira"

	ReasonInvalidProject Reason = "invalid_project"

	ReasonInvalidJiraFormat Reason = "invalid_jira_format"

	ReasonMissingJiraKeySubject Reason = "missing_jira_key_subject"
	ReasonMissingJiraKeyBody    Reason = "missing_jira_key_body"
	ReasonJiraKeyNotAtEnd       Reason = "jira_key_not_at_end"
	ReasonInvalidRefsFormat     Reason = "invalid_refs_format"
	ReasonInvalidKeyFormat      Reason = "invalid_key_format"
	ReasonRefsAfterSignoff      Reason = "refs_after_signoff"

	ReasonNonImperative Reason = "non_imperative"
	ReasonNonVerb       Reason = "non_verb"
	ReasonPastTense     Reason = "past_tense"
	ReasonGerund        Reason = "gerund"
	ReasonThirdPerson   Reason = "third_person"

	ReasonNoKeyDir               Reason = "no_key_dir"
	ReasonInvalidKeyDir          Reason = "invalid_key_dir"
	ReasonMissingSignature       Reason = "missing_signature"
	ReasonInvalidSignature       Reason = "invalid_signature"
	ReasonInvalidSignatureFormat Reason = "invalid_signature_format"
	ReasonUnknownSigFormat       Reason = "unknown_signature_format"
	ReasonKeyNotTrusted          Reason = "key_not_trusted"
	ReasonWeakKey                Reason = "weak_key"
	ReasonVerificationFailed     Reason = "verification_failed"
	ReasonDisallowedSigType      Reason = "disallowed_signature_type"
	ReasonIncompleteGPGSig       Reason = "incomplete_gpg_signature"
	ReasonIncompleteSSHSig       Reason = "incomplete_ssh_signature"

	ReasonInvalidCommit           Reason = "invalid_commit"
	ReasonMissingKeyDirectory     Reason = "missing_key_directory"
	ReasonInvalidKeyDirectory     Reason = "invalid_key_directory"
	ReasonNoTrustedKeys           Reason = "no_trusted_keys"
	ReasonKeyLoadFailed           Reason = "key_load_failed"
	ReasonSignatureTypeNotAllowed Reason = "signature_type_not_allowed"

	ReasonMissingSignoff       Reason = "missing_signoff"
	ReasonInvalidSignoffFormat Reason = "invalid_signoff_format"
	ReasonMisplacedSignoff     Reason = "misplaced_signoff"
	ReasonInsufficientSignoffs Reason = "insufficient_signoffs"

	ReasonSpelling         Reason = "spelling_error"
	ReasonMisspelledWord   Reason = "misspelled_word"
	ReasonSpellCheckFailed Reason = "spell_check_failed"

	ReasonTooManyCommits   Reason = "too_many_commits"
	ReasonMergeCommitFound Reason = "merge_commit_found"
	ReasonFixupCommitFound Reason = "fixup_commit_found"

	ReasonInvalidRepo        Reason = "invalid_repo"
	ReasonInvalidConfig      Reason = "invalid_config"
	ReasonConfigNotFound     Reason = "config_not_found"
	ReasonMessageFileMissing Reason = "message_file_missing"
	ReasonRepoRuleNoRepo     Reason = "repo_rule_no_repo"
	ReasonCancelled          Reason = "operation_cancelled"
	ReasonGitOperationFailed Reason = "git_operation_failed"
	ReasonContextCancelled   Reason = "context_cancelled"
	ReasonCommitNotFound     Reason = "commit_not_found"
	ReasonRangeNotFound      Reason = "range_not_found"

	ReasonInvalidReference   Reason = "invalid_reference"
	ReasonMissingReference   Reason = "missing_reference"
	ReasonIdentityNotAllowed Reason = "identity_not_allowed"
	ReasonInvalidCommitRange Reason = "invalid_commit_range"

	ReasonUnknown Reason = "unknown_error"
)

// AllValidationReasons returns every validation reason once, in catalog order:
// the reasons of Definitions followed by ReasonMessageFileMissing, which the
// CLI raises for a missing --message-file before any diagnostic ID applies.
func AllValidationReasons() []Reason {
	seen := make(map[Reason]struct{})

	var reasons []Reason

	for _, definition := range Definitions() {
		for _, reason := range definition.Reasons {
			if _, duplicate := seen[reason]; duplicate {
				continue
			}

			seen[reason] = struct{}{}
			reasons = append(reasons, reason)
		}
	}

	return append(reasons, ReasonMessageFileMissing)
}
