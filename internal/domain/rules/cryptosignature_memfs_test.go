// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestCryptoSignatureRule_SignatureTypeValidation_MemFS tests signature type validation.
func TestCryptoSignatureRule_SignatureTypeValidation_MemFS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		signature   string
		requireGPG  bool
		requireSSH  bool
		expectError bool
		errorCode   expectedReason
	}{
		{
			name:        "gpg_signature_with_gpg_required",
			signature:   testGPGSignature,
			requireGPG:  true,
			requireSSH:  false,
			expectError: false,
		},
		{
			name:        "ssh_signature_with_ssh_required",
			signature:   testSSHRSASignature,
			requireGPG:  false,
			requireSSH:  true,
			expectError: false,
		},
		{
			name:        "gpg_signature_with_ssh_required",
			signature:   testGPGSignature,
			requireGPG:  false,
			requireSSH:  true,
			expectError: true,
			errorCode:   diagnostics.ReasonSignatureTypeNotAllowed,
		},
		{
			name:        "ssh_signature_with_gpg_required",
			signature:   testSSHRSASignature,
			requireGPG:  true,
			requireSSH:  false,
			expectError: true,
			errorCode:   diagnostics.ReasonSignatureTypeNotAllowed,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
				gpgKeys:        []CryptoKeyInfo{validGPGKeyInfo()},
				sshKeys:        []CryptoKeyInfo{validSSHKeyInfo(testEmail)},
			}

			rule := NewCryptoSignatureRule(settings.Config{
				CryptoSignature: settings.CryptoSignatureConfig{
					RequireGPG: new(testCase.requireGPG),
					RequireSSH: new(testCase.requireSSH),
				},
			}, cryptoKeyProviders(provider))

			commit := domain.Commit{
				Subject:   testCommitSubject,
				Signature: testCase.signature,
			}

			errors := rule.Validate(commit)

			if testCase.expectError {
				check.NotEmpty(t, errors)
				check.Equal(t, testCase.errorCode, reasonOf(errors[0]))
			} else {
				// May have other errors, but never the type-requirement error.
				for _, err := range errors {
					check.NotEqual(t, diagnostics.ReasonSignatureTypeNotAllowed, reasonOf(err))
				}
			}
		})
	}
}

// TestCryptoSignatureRule_ConfigurationScenarios_MemFS tests configuration scenarios.
func TestCryptoSignatureRule_ConfigurationScenarios_MemFS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    settings.CryptoSignatureConfig
		commit    domain.Commit
		wantError bool
		errorCode expectedReason
	}{
		{
			name: "both_gpg_and_ssh_required_is_invalid",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(true),
				RequireSSH: new(true),
			},
			commit: domain.Commit{
				Subject:   testCommitSubject,
				Signature: "-----BEGIN PGP SIGNATURE-----\ntest\n-----END PGP SIGNATURE-----",
			},
			wantError: true,
			errorCode: diagnostics.ReasonInvalidConfig,
		},
		{
			name: "no_requirements_allows_gpg",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(false),
				RequireSSH: new(false),
			},
			commit: domain.Commit{
				Subject:   testCommitSubject,
				Signature: "-----BEGIN PGP SIGNATURE-----\ntest\n-----END PGP SIGNATURE-----",
			},
			wantError: false,
		},
		{
			name: "no_requirements_allows_ssh",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(false),
				RequireSSH: new(false),
			},
			commit: domain.Commit{
				Subject:   testCommitSubject,
				Signature: "ssh-rsa:LS0tLS1CRUdJTiBTU0ggU0lHTkFUVVJFLS0tLS0K",
			},
			wantError: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
				gpgKeys:        []CryptoKeyInfo{validGPGKeyInfo()},
				sshKeys:        []CryptoKeyInfo{validSSHKeyInfo("test@example.com")},
			}

			rule := NewCryptoSignatureRule(settings.Config{
				CryptoSignature: testCase.config,
			}, cryptoKeyProviders(provider))

			errors := rule.Validate(testCase.commit)

			if testCase.wantError {
				check.NotEmpty(t, errors)
				check.Equal(t, testCase.errorCode, reasonOf(errors[0]))
			} else {
				// May have other errors, but never the specific expected one.
				for _, err := range errors {
					check.NotEqual(t, testCase.errorCode, reasonOf(err))
				}
			}
		})
	}
}
