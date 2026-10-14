// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package diagnostics_test

import (
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestDefinitions_UniqueIDsAndRuleReasons(t *testing.T) {
	t.Parallel()

	seenIDs := make(map[diagnostics.ID]struct{})
	seenRuleReasons := make(map[string]struct{})

	for _, definition := range diagnostics.Definitions() {
		check.NotEmpty(t, definition.ID)
		check.NotEmpty(t, definition.SourceKind)
		check.NotEmpty(t, definition.Category)
		check.NotEmpty(t, definition.Summary)
		check.NotEmpty(t, definition.Reasons)

		if definition.SourceKind == diagnostics.SourceRule {
			check.NotEmpty(t, definition.RuleID)
			check.True(t, rulecatalog.IsValid(definition.RuleID), "unknown rule ID %s", definition.RuleID)
		} else {
			check.NotEmpty(t, definition.SourceID)
		}

		_, duplicateID := seenIDs[definition.ID]
		check.False(t, duplicateID, "duplicate diagnostic ID %s", definition.ID)
		seenIDs[definition.ID] = struct{}{}

		_, inRange := diagnostics.RangeForID(string(definition.ID))
		check.True(t, inRange, "diagnostic ID %s is outside documented ranges", definition.ID)

		for _, reason := range definition.Reasons {
			key := fmt.Sprintf("%s/%s/%s", definition.SourceKind, definition.RuleID+definition.SourceID, reason)
			_, duplicatePair := seenRuleReasons[key]
			check.False(t, duplicatePair, "duplicate rule/reason mapping %s", key)
			seenRuleReasons[key] = struct{}{}
		}
	}
}

func TestByRuleAndReason_ResolvesCatalogEntry(t *testing.T) {
	t.Parallel()

	definition, ok := diagnostics.ByRuleAndReason(rulecatalog.Subject, "subject_too_long")

	check.True(t, ok)
	check.Equal(t, diagnostics.ID("GL102"), definition.ID)
	check.Equal(t, diagnostics.Category("message.subject"), definition.Category)
}

func TestBySourceAndReason_DispatcherInput(t *testing.T) {
	t.Parallel()

	definition, ok := diagnostics.BySourceAndReason(diagnostics.SourceIDValidation, diagnostics.ReasonInvalidReference)

	check.True(t, ok)
	check.Equal(t, diagnostics.ID("GL903"), definition.ID)
	check.Equal(t, diagnostics.Category("dispatcher.input"), definition.Category)
}

func TestDefinition_Diagnostic(t *testing.T) {
	t.Parallel()

	definition, ok := diagnostics.ByRuleAndReason(rulecatalog.CryptoSignature, "missing_signature")
	check.True(t, ok)

	diagnostic := definition.Diagnostic("missing_signature")

	check.Equal(t, diagnostics.ID("GL501"), diagnostic.ID)
	check.Equal(t, diagnostics.Reason("missing_signature"), diagnostic.Reason)
	check.Equal(t, diagnostics.Category("identity.signature"), diagnostic.Category)
	check.Contains(t, diagnostic.Summary, "signature")
}

func TestNormalizeID_StripsQuotesAndUppercases(t *testing.T) {
	t.Parallel()

	check.Equal(t, "GL102", diagnostics.NormalizeID(" 'gl102' "))
}
