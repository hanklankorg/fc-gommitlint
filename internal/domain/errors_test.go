// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"fmt"
	"strconv"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestValidationError_WithContextMap(t *testing.T) {
	t.Parallel()

	original := ValidationError{
		Rule:       "TestRule",
		Diagnostic: diagnostics.Diagnostic{Reason: "test_reason"},
		Message:    "test message",
		Context:    map[string]string{testContextExisting: testContextValue},
	}

	updated := original.WithContextMap(diagnostics.Context{
		"new_key": "new_value",
		"key2":    "value2",
	})

	// Original should be unchanged (value semantics)
	check.Equal(t, map[string]string{testContextExisting: testContextValue}, original.Context)

	// Updated should have all context
	want := map[string]string{
		testContextExisting: testContextValue,
		"new_key":           "new_value",
		"key2":              "value2",
	}
	check.Equal(t, want, updated.Context)
}

func TestValidationError_WithUserMessage(t *testing.T) {
	t.Parallel()

	original := ValidationError{
		Rule:       "TestRule",
		Diagnostic: diagnostics.Diagnostic{Reason: "test_reason"},
		Message:    "original message",
	}

	updated := original.WithUserMessage("formatted %s %d", "message", 42)

	// Original should be unchanged (value semantics)
	check.Equal(t, "original message", original.Message)

	// Updated should have formatted message
	check.Equal(t, "formatted message 42", updated.Message)
}

func TestValidationError_ChainedMethods(t *testing.T) {
	t.Parallel()

	maxLength := settings.DefaultSubjectMaxLength
	maxLengthStr := strconv.Itoa(maxLength)
	actualLength := maxLength + 25
	actualLengthStr := strconv.Itoa(actualLength)

	err := New("TestRule", diagnostics.ReasonSubjectTooLong, "Subject is too long").
		WithContextMap(diagnostics.Context{
			testContextActualLength: actualLengthStr,
			testContextMaxLength:    maxLengthStr,
		}).
		WithUserMessage("Subject '%s' is too long (%s/%s chars)", "feat: add feature", actualLengthStr, maxLengthStr)

	check.Equal(t, "TestRule", err.Rule)
	check.Equal(t, diagnostics.ReasonSubjectTooLong, err.Diagnostic.Reason)
	check.Equal(t, fmt.Sprintf("Subject 'feat: add feature' is too long (%s/%s chars)", actualLengthStr, maxLengthStr), err.Message)
	check.Equal(t, map[string]string{
		testContextActualLength: actualLengthStr,
		testContextMaxLength:    maxLengthStr,
	}, err.Context)
}

func TestValidationError_NewAssignsCheckMetadata(t *testing.T) {
	t.Parallel()

	err := New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "Subject is too long")

	check.Equal(t, diagnostics.ID("GL102"), err.Diagnostic.ID)
	check.Equal(t, diagnostics.Category("message.subject"), err.Diagnostic.Category)
	check.Equal(t, diagnostics.ReasonSubjectTooLong, err.Diagnostic.Reason)
}
