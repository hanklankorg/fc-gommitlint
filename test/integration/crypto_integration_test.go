//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestCryptoSignature_IntegrationPipeline tests cryptographic signature validation in the integration pipeline.
// Most crypto validation business logic is tested in internal/domain/rules/crypto_validation_unit_test.go
func TestCryptoSignature_IntegrationPipeline(t *testing.T) {
	t.Parallel()

	t.Run("crypto validation integration pipeline", func(t *testing.T) {
		t.Parallel()

		// Test basic crypto validation integration
		config := DefaultConfig()
		config.Rules.Enabled = []string{rulecatalog.CryptoSignature}
		config.Rules.Disabled = []string{rulecatalog.Subject, rulecatalog.Conventional}
		config.CryptoSignature.Required = new(true)

		// Verify integration pipeline works
		result := ValidateMessageHelper(t, "feat: add feature", config)
		check.False(t, result.Valid, "Integration pipeline should handle signature validation")

		// Verify signature rule is integrated correctly
		found := false

		for _, validationErr := range result.Errors {
			if validationErr.Rule == rulecatalog.CryptoSignature {
				found = true

				break
			}
		}

		check.True(t, found, "Expected Signature rule integration to work")
	})
}

// TestSignedIdentity_RequiresMatchingSignature tests signed identity validation in the integration pipeline.
// Most identity validation business logic is tested in internal/domain/rules/crypto_validation_unit_test.go
func TestSignedIdentity_RequiresMatchingSignature(t *testing.T) {
	t.Parallel()

	t.Run("identity validation integration pipeline", func(t *testing.T) {
		t.Parallel()

		// Test identity validation integration
		config := DefaultConfig()
		config.Rules.Enabled = []string{rulecatalog.Identity}
		config.Rules.Disabled = []string{rulecatalog.Subject, rulecatalog.Conventional, rulecatalog.CommitBody, rulecatalog.CryptoSignature, rulecatalog.SignOff, rulecatalog.JiraReference, rulecatalog.Spell, rulecatalog.BranchAhead}
		config.Identity.AllowedAuthors = []string{"allowed@example.com"}

		// Verify integration pipeline works
		result := ValidateMessageHelper(t, "feat: add feature", config)
		check.False(t, result.Valid, "Integration pipeline should handle identity validation")

		foundIdentity := false

		for _, validationErr := range result.Errors {
			if validationErr.Rule == rulecatalog.Identity {
				foundIdentity = true

				break
			}
		}

		check.True(t, foundIdentity, "the failure must come from the Identity rule, got: %v", result.Errors)
	})
}

// TestSignOff_RequiresSignedOffByLine tests sign-off validation in the integration pipeline.
// Most sign-off validation business logic is tested in internal/domain/rules/crypto_validation_unit_test.go
func TestSignOff_RequiresSignedOffByLine(t *testing.T) {
	t.Parallel()

	t.Run("sign-off validation integration pipeline", func(t *testing.T) {
		t.Parallel()

		// Test sign-off validation integration
		config := DefaultConfig()
		config.Rules.Enabled = []string{rulecatalog.SignOff}
		config.Rules.Disabled = []string{rulecatalog.Subject, rulecatalog.Conventional}
		config.Message.Body.MinSignoffCount = new(1)

		// Verify integration pipeline works
		result := ValidateMessageHelper(t, "feat: add feature", config)
		check.False(t, result.Valid, "Integration pipeline should handle sign-off validation")

		foundSignOff := false

		for _, validationErr := range result.Errors {
			if validationErr.Rule == rulecatalog.SignOff {
				foundSignOff = true

				break
			}
		}

		check.True(t, foundSignOff, "the failure must come from the SignOff rule, got: %v", result.Errors)
	})
}
