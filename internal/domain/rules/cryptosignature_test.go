// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"errors"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Test constants.
const (
	testAllowedSignerOther = "other@example.com"
	testAllowedSignerUser  = "John Doe <john@example.com>"
	testEmail              = "test@example.com"
	testCommitSubject      = "test commit"
	testGPGSignature       = "-----BEGIN PGP SIGNATURE-----\ntest\n-----END PGP SIGNATURE-----"
	testSSHRSASignature    = "ssh-rsa:LS0tLS1CRUdJTiBTU0ggU0lHTkFUVVJFLS0tLS0K"
	testSSHSignature       = "ssh-ed25519:AAAAC3NzaC1lZDI1NTE5AAAAI"

	testActiveCheckFormat    = "format"
	testActiveCheckPresence  = "presence"
	testActiveCheckType      = "type"
	testActiveCheckKeyPolicy = "key policy"
)

// Static errors for testing.
var (
	errKeyLoadFailed    = errors.New("key load failed")
	errInvalidDirectory = errors.New("invalid directory")
)

// mockKeyProvider provides all key-provider capabilities for testing.
type mockKeyProvider struct {
	gpgKeys        []CryptoKeyInfo
	sshKeys        []CryptoKeyInfo
	shouldFail     bool
	failError      error
	directoryValid bool
}

func (m *mockKeyProvider) ValidateDirectory() error {
	if !m.directoryValid {
		return errInvalidDirectory
	}

	return nil
}

func (m *mockKeyProvider) LoadGPGKeys() ([]CryptoKeyInfo, error) {
	if m.shouldFail {
		return nil, m.failError
	}

	return m.gpgKeys, nil
}

func (m *mockKeyProvider) LoadSSHKeys() ([]CryptoKeyInfo, error) {
	if m.shouldFail {
		return nil, m.failError
	}

	return m.sshKeys, nil
}

func cryptoKeyProviders(provider *mockKeyProvider) CryptoKeyProviders {
	return CryptoKeyProviders{
		Directory: provider,
		GPG:       provider,
		SSH:       provider,
	}
}

type gpgOnlyKeyProvider struct {
	directoryValid bool
	gpgKeys        []CryptoKeyInfo
}

func (p gpgOnlyKeyProvider) ValidateDirectory() error {
	if !p.directoryValid {
		return errInvalidDirectory
	}

	return nil
}

func (p gpgOnlyKeyProvider) LoadGPGKeys() ([]CryptoKeyInfo, error) {
	return p.gpgKeys, nil
}

type sshOnlyKeyProvider struct {
	directoryValid bool
	sshKeys        []CryptoKeyInfo
}

func (p sshOnlyKeyProvider) ValidateDirectory() error {
	if !p.directoryValid {
		return errInvalidDirectory
	}

	return nil
}

func (p sshOnlyKeyProvider) LoadSSHKeys() ([]CryptoKeyInfo, error) {
	return p.sshKeys, nil
}

// validGPGKeyInfo returns a CryptoKeyInfo representing a valid GPG key.
func validGPGKeyInfo() CryptoKeyInfo {
	return CryptoKeyInfo{
		Algorithm: KeyAlgorithmEd25519,
		BitLength: 256,
		Revoked:   false,
		Expired:   false,
		Identity: KeyIdentity{
			Fingerprint: "ABCD1234",
			UserIDs:     []string{"Test User <test@>"},
			Emails:      []string{"test@"},
		},
	}
}

// validSSHKeyInfo returns a CryptoKeyInfo representing a valid SSH key.
func validSSHKeyInfo(comment string) CryptoKeyInfo {
	info := CryptoKeyInfo{
		Algorithm: KeyAlgorithmEd25519,
		BitLength: 256,
		Revoked:   false,
		Expired:   false,
		Identity: KeyIdentity{
			Fingerprint: "SHA256:mock-fingerprint",
		},
	}

	if comment == "" {
		return info
	}

	info.Identity.UserIDs = append(info.Identity.UserIDs, comment)

	if strings.Contains(comment, "@") && !strings.Contains(comment, " ") {
		info.Identity.Emails = append(info.Identity.Emails, comment)
	}

	return info
}

// Test basic configuration and rule creation.
func TestCryptoSignatureRule_BasicConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     settings.Config
		wantRSA int
		wantEC  int
		wantGPG bool
		wantSSH bool
	}{
		{
			name:    "default_values",
			cfg:     settings.Config{},
			wantRSA: settings.DefaultMinRSABits,
			wantEC:  settings.DefaultMinECBits,
		},
		{
			name: "custom_values",
			cfg: settings.Config{
				CryptoSignature: settings.CryptoSignatureConfig{
					MinRSABits: new(4096),
					MinECBits:  new(384),
					RequireGPG: new(true),
				},
			},
			wantRSA: 4096,
			wantEC:  384,
			wantGPG: true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rule := NewCryptoSignatureRule(testCase.cfg, CryptoKeyProviders{})

			check.Equal(t, rulecatalog.CryptoSignature, rule.Name())
			check.Equal(t, testCase.wantRSA, rule.minRSABits)
			check.Equal(t, testCase.wantEC, rule.minECBits)
			check.Equal(t, testCase.wantGPG, rule.requireGPG)
			check.Equal(t, testCase.wantSSH, rule.requireSSH)
		})
	}
}

// Test that validation is skipped when no key provider is configured.
func TestCryptoSignatureRule_NoKeyProvider(t *testing.T) {
	t.Parallel()

	rule := NewCryptoSignatureRule(settings.Config{}, CryptoKeyProviders{})
	commit := domain.Commit{Signature: testGPGSignature}

	errors := rule.Validate(commit)
	check.Empty(t, errors, "Should skip validation when no key provider")
}

func TestCryptoSignatureRule_WithKeyProviders_GPGOnlyProvider(t *testing.T) {
	t.Parallel()

	provider := gpgOnlyKeyProvider{directoryValid: true, gpgKeys: []CryptoKeyInfo{validGPGKeyInfo()}}
	rule := NewCryptoSignatureRule(settings.Config{}, CryptoKeyProviders{
		Directory: provider,
		GPG:       provider,
	})
	commit := domain.Commit{Signature: testGPGSignature}

	errors := rule.Validate(commit)
	check.Empty(t, errors)
}

func TestCryptoSignatureRule_WithKeyProviders_SSHOnlyProvider(t *testing.T) {
	t.Parallel()

	provider := sshOnlyKeyProvider{directoryValid: true, sshKeys: []CryptoKeyInfo{validSSHKeyInfo("test@example.com")}}
	rule := NewCryptoSignatureRule(settings.Config{}, CryptoKeyProviders{
		Directory: provider,
		SSH:       provider,
	})
	commit := domain.Commit{Signature: testSSHSignature}

	errors := rule.Validate(commit)
	check.Empty(t, errors)
}

// Test that validation is skipped when no signature is present.
func TestCryptoSignatureRule_NoSignature(t *testing.T) {
	t.Parallel()

	provider := &mockKeyProvider{directoryValid: true}
	rule := NewCryptoSignatureRule(settings.Config{}, cryptoKeyProviders(provider))
	commit := domain.Commit{Signature: ""}

	errors := rule.Validate(commit)
	check.Empty(t, errors, "Should skip validation when no signature")
}

func TestCryptoSignatureRule_Required_RejectsAbsentSignature(t *testing.T) {
	t.Parallel()

	rule := NewCryptoSignatureRule(settings.Config{
		CryptoSignature: settings.CryptoSignatureConfig{
			Required: new(true),
		},
	}, CryptoKeyProviders{})

	for _, testCase := range []struct {
		name      string
		signature string
	}{
		{name: "empty_signature", signature: ""},
		{name: "whitespace_only_signature", signature: " \n\t"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := rule.Validate(domain.Commit{Signature: testCase.signature})
			check.Len(t, errors, 1)
			check.Equal(t, diagnostics.ReasonMissingSignature, reasonOf(errors[0]))
		})
	}

	t.Run("present_signature_satisfies_required", func(t *testing.T) {
		t.Parallel()

		check.Empty(t, rule.Validate(domain.Commit{Signature: testGPGSignature}))
	})
}

func TestCryptoSignatureRule_VerifyFormat_SSHArmor(t *testing.T) {
	t.Parallel()

	rule := NewCryptoSignatureRule(settings.Config{
		CryptoSignature: settings.CryptoSignatureConfig{
			VerifyFormat: new(true),
		},
	}, CryptoKeyProviders{})

	tests := []struct {
		name      string
		signature string
		wantCode  expectedReason
	}{
		{
			name: "complete_armor_passes_format_check",
			signature: `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAADMAAAALc3NoLWVkMjU1MTk=
-----END SSH SIGNATURE-----`,
		},
		{
			name: "missing_end_marker_fails_format_check",
			signature: `-----BEGIN SSH SIGNATURE-----
U1NIU0lHAAAAAQAAADMAAAALc3NoLWVkMjU1MTk=`,
			wantCode: diagnostics.ReasonIncompleteSSHSig,
		},
		{
			name: "markers_without_body_fail_format_check",
			signature: `-----BEGIN SSH SIGNATURE-----
-----END SSH SIGNATURE-----`,
			wantCode: diagnostics.ReasonIncompleteSSHSig,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := rule.Validate(domain.Commit{Signature: testCase.signature})
			if testCase.wantCode == "" {
				check.Empty(t, errors)

				return
			}

			check.Len(t, errors, 1)
			check.Equal(t, testCase.wantCode, reasonOf(errors[0]))
		})
	}
}

func TestCryptoSignatureRule_VerifyFormat_GPGArmor(t *testing.T) {
	t.Parallel()

	rule := NewCryptoSignatureRule(settings.Config{
		CryptoSignature: settings.CryptoSignatureConfig{
			VerifyFormat: new(true),
		},
	}, CryptoKeyProviders{})

	tests := []struct {
		name      string
		signature string
		wantCode  expectedReason
	}{
		{
			name: "complete_armor_passes_format_check",
			signature: `-----BEGIN PGP SIGNATURE-----

abc123
-----END PGP SIGNATURE-----`,
		},
		{
			name: "missing_end_marker_fails_format_check",
			signature: `-----BEGIN PGP SIGNATURE-----

abc123`,
			wantCode: diagnostics.ReasonIncompleteGPGSig,
		},
		{
			name:      "bare_packet_prefix_is_unknown",
			signature: "iQEcBAABCAAGBQJtest=",
			wantCode:  diagnostics.ReasonUnknownSigFormat,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			errors := rule.Validate(domain.Commit{Signature: testCase.signature})
			if testCase.wantCode == "" {
				check.Empty(t, errors)

				return
			}

			check.NotEmpty(t, errors)
			check.Equal(t, testCase.wantCode, reasonOf(errors[0]))
		})
	}
}

// Test key directory validation.
func TestCryptoSignatureRule_InvalidKeyDirectory(t *testing.T) {
	t.Parallel()

	provider := &mockKeyProvider{directoryValid: false}
	rule := NewCryptoSignatureRule(settings.Config{}, cryptoKeyProviders(provider))
	commit := domain.Commit{Signature: testGPGSignature}

	errors := rule.Validate(commit)
	check.Len(t, errors, 1)
	check.Equal(t, diagnostics.ReasonInvalidKeyDirectory, reasonOf(errors[0]))
}

// Test signature type requirements.
func TestCryptoSignatureRule_SignatureTypeRequirements(t *testing.T) {
	t.Parallel()

	sshSig := testSSHSignature

	tests := []struct {
		name        string
		requireGPG  bool
		requireSSH  bool
		signature   string
		wantErr     bool
		wantErrCode expectedReason
	}{
		{
			name:       "gpg_required_and_provided",
			requireGPG: true,
			signature:  testGPGSignature,
			wantErr:    false,
		},
		{
			name:        "gpg_required_but_ssh_provided",
			requireGPG:  true,
			signature:   sshSig,
			wantErr:     true,
			wantErrCode: diagnostics.ReasonSignatureTypeNotAllowed,
		},
		{
			name:       "ssh_required_and_provided",
			requireSSH: true,
			signature:  sshSig,
			wantErr:    false,
		},
		{
			name:        "ssh_required_but_gpg_provided",
			requireSSH:  true,
			signature:   testGPGSignature,
			wantErr:     true,
			wantErrCode: diagnostics.ReasonSignatureTypeNotAllowed,
		},
		{
			name:        "both_required_is_invalid_config",
			requireGPG:  true,
			requireSSH:  true,
			signature:   testGPGSignature,
			wantErr:     true,
			wantErrCode: diagnostics.ReasonInvalidConfig,
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

			cfg := settings.Config{
				CryptoSignature: settings.CryptoSignatureConfig{
					RequireGPG: new(testCase.requireGPG),
					RequireSSH: new(testCase.requireSSH),
				},
			}
			rule := NewCryptoSignatureRule(cfg, cryptoKeyProviders(provider))
			commit := domain.Commit{Signature: testCase.signature}

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			} else {
				// May have other errors (e.g., no valid keys) but not the type requirement error
				for _, err := range errors {
					check.NotEqual(t, diagnostics.ReasonSignatureTypeNotAllowed, reasonOf(err))
					check.NotEqual(t, diagnostics.ReasonInvalidConfig, reasonOf(err))
				}
			}
		})
	}
}

// Test validation when no keys are found.
func TestCryptoSignatureRule_NoKeysFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		signature string
		wantCode  expectedReason
	}{
		{
			name:      "gpg_signature_no_gpg_keys",
			signature: testGPGSignature,
			wantCode:  diagnostics.ReasonNoTrustedKeys,
		},
		{
			name:      "ssh_signature_no_ssh_keys",
			signature: "ssh-ed25519:AAAAC3NzaC1lZDI1NTE5AAAAI",
			wantCode:  diagnostics.ReasonNoTrustedKeys,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
			}
			rule := NewCryptoSignatureRule(settings.Config{}, cryptoKeyProviders(provider))
			commit := domain.Commit{Signature: testCase.signature}

			errors := rule.Validate(commit)
			check.NotEmpty(t, errors)
			check.Equal(t, testCase.wantCode, reasonOf(errors[0]))
		})
	}
}

// Test KeyIdentity matching.
func TestKeyIdentity_Matches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		identity       KeyIdentity
		allowedSigners []string
		want           bool
	}{
		{
			name: "empty_allowed_permits_all",
			identity: KeyIdentity{
				Fingerprint: "ABC123",
			},
			allowedSigners: []string{},
			want:           true,
		},
		{
			name: "fingerprint_match_case_insensitive",
			identity: KeyIdentity{
				Fingerprint: "ABC123DEF456",
			},
			allowedSigners: []string{"abc123def456"},
			want:           true,
		},
		{
			name: "email_match_case_insensitive",
			identity: KeyIdentity{
				Emails: []string{"User@Example.COM"},
			},
			allowedSigners: []string{"user@example.com"},
			want:           true,
		},
		{
			name: "user_id_match",
			identity: KeyIdentity{
				UserIDs: []string{testAllowedSignerUser},
			},
			allowedSigners: []string{testAllowedSignerUser},
			want:           true,
		},
		{
			name: "user_id_no_match_different_case",
			identity: KeyIdentity{
				UserIDs: []string{testAllowedSignerUser},
			},
			allowedSigners: []string{"john doe <john@example.com>"},
			want:           false,
		},
		{
			name: "no_match",
			identity: KeyIdentity{
				Fingerprint: "ABC123",
				Emails:      []string{"user@example.com"},
			},
			allowedSigners: []string{testAllowedSignerOther},
			want:           false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := testCase.identity.Matches(testCase.allowedSigners)
			check.Equal(t, testCase.want, got)
		})
	}
}

// Test meetsMinimumKeyStrength.
func TestMeetsMinimumKeyStrength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        CryptoKeyInfo
		minRSABits int
		minECBits  int
		want       bool
	}{
		{
			name:       "rsa_meets_minimum",
			key:        CryptoKeyInfo{Algorithm: KeyAlgorithmRSA, BitLength: 2048},
			minRSABits: 2048,
			want:       true,
		},
		{
			name:       "rsa_below_minimum",
			key:        CryptoKeyInfo{Algorithm: KeyAlgorithmRSA, BitLength: 1024},
			minRSABits: 2048,
			want:       false,
		},
		{
			name:      "ed25519_meets_default",
			key:       CryptoKeyInfo{Algorithm: KeyAlgorithmEd25519, BitLength: 256},
			minECBits: 256,
			want:      true,
		},
		{
			name:      "ed25519_below_384",
			key:       CryptoKeyInfo{Algorithm: KeyAlgorithmEd25519, BitLength: 256},
			minECBits: 384,
			want:      false,
		},
		{
			name:      "ecdsa_meets_minimum",
			key:       CryptoKeyInfo{Algorithm: KeyAlgorithmECDSA, BitLength: 384},
			minECBits: 256,
			want:      true,
		},
		{
			name: "unknown_algorithm_rejected",
			key:  CryptoKeyInfo{Algorithm: "Unknown", BitLength: 4096},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := meetsMinimumKeyStrength(testCase.key, testCase.minRSABits, testCase.minECBits)
			check.Equal(t, testCase.want, got)
		})
	}
}

// Test allowed_signers filtering in GPG key validation.
func TestCryptoSignatureRule_AllowedSignersFiltering_GPG(t *testing.T) {
	t.Parallel()

	gpgSig := testGPGSignature

	tests := []struct {
		name           string
		allowedSigners []string
		wantErr        bool
		wantErrCode    expectedReason
	}{
		{
			name:           "empty_allowed_signers_permits_all",
			allowedSigners: []string{},
			wantErr:        false,
		},
		{
			name:           "matching_email_allowed",
			allowedSigners: []string{"test@"},
			wantErr:        false,
		},
		{
			name:           "non_matching_signer_rejected",
			allowedSigners: []string{"other@example.com"},
			wantErr:        true,
			wantErrCode:    diagnostics.ReasonNoTrustedKeys,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
				gpgKeys:        []CryptoKeyInfo{validGPGKeyInfo()},
			}

			cfg := settings.Config{
				CryptoSignature: settings.CryptoSignatureConfig{
					AllowedSigners: testCase.allowedSigners,
				},
			}
			rule := NewCryptoSignatureRule(cfg, cryptoKeyProviders(provider))
			commit := domain.Commit{Signature: gpgSig}

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			} else {
				check.Empty(t, errors, "Should pass when allowed_signers matches or is empty")
			}
		})
	}
}

// Test allowed_signers filtering in SSH key validation.
func TestCryptoSignatureRule_AllowedSignersFiltering_SSH(t *testing.T) {
	t.Parallel()

	sshSig := "ssh-ed25519:AAAAC3NzaC1lZDI1NTE5AAAAI"

	tests := []struct {
		name           string
		allowedSigners []string
		keyComment     string
		wantErr        bool
		wantErrCode    expectedReason
	}{
		{
			name:           "empty_allowed_signers_permits_all",
			allowedSigners: []string{},
			keyComment:     testEmail,
			wantErr:        false,
		},
		{
			name:           "matching_email_allowed",
			allowedSigners: []string{testEmail},
			keyComment:     testEmail,
			wantErr:        false,
		},
		{
			name:           "non_matching_signer_rejected",
			allowedSigners: []string{testAllowedSignerOther},
			keyComment:     testEmail,
			wantErr:        true,
			wantErrCode:    diagnostics.ReasonNoTrustedKeys,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
				sshKeys:        []CryptoKeyInfo{validSSHKeyInfo(testCase.keyComment)},
			}

			cfg := settings.Config{
				CryptoSignature: settings.CryptoSignatureConfig{
					AllowedSigners: testCase.allowedSigners,
				},
			}
			rule := NewCryptoSignatureRule(cfg, cryptoKeyProviders(provider))
			commit := domain.Commit{Signature: sshSig}

			errors := rule.Validate(commit)

			if testCase.wantErr {
				check.NotEmpty(t, errors)
				check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
				check.Contains(t, errors[0].Context["rejected"], "not in allowed_signers")
			} else {
				check.Empty(t, errors, "Should pass when allowed_signers matches or is empty")
			}
		})
	}
}

// Test that keyRejectionStats correctly tracks not_allowed rejections.
func TestCryptoSignatureRule_KeyRejectionStats_NotAllowed(t *testing.T) {
	t.Parallel()

	provider := &mockKeyProvider{
		directoryValid: true,
		sshKeys:        []CryptoKeyInfo{validSSHKeyInfo("user@example.com")},
	}

	cfg := settings.Config{
		CryptoSignature: settings.CryptoSignatureConfig{
			AllowedSigners: []string{"different@example.com"},
		},
	}
	rule := NewCryptoSignatureRule(cfg, cryptoKeyProviders(provider))
	commit := domain.Commit{Signature: testSSHSignature}

	errors := rule.Validate(commit)

	check.NotEmpty(t, errors)
	check.Equal(t, diagnostics.ReasonNoTrustedKeys, reasonOf(errors[0]))
	check.Equal(t, "1", errors[0].Context["keys_found"])
	check.Contains(t, errors[0].Context["rejected"], "1 not in allowed_signers")
}

// Test GPG key rejection reasons.
func TestCryptoSignatureRule_GPGKeyRejections(t *testing.T) {
	t.Parallel()

	gpgSig := testGPGSignature

	tests := []struct {
		name          string
		key           CryptoKeyInfo
		wantRejection string
		wantKeysFound string
		wantErrCode   expectedReason
	}{
		{
			name: "revoked_key",
			key: CryptoKeyInfo{
				Algorithm: KeyAlgorithmEd25519,
				BitLength: 256,
				Revoked:   true,
				Identity:  KeyIdentity{Emails: []string{testEmail}},
			},
			wantRejection: "1 revoked",
			wantKeysFound: "1",
			wantErrCode:   diagnostics.ReasonNoTrustedKeys,
		},
		{
			name: "expired_key",
			key: CryptoKeyInfo{
				Algorithm: KeyAlgorithmEd25519,
				BitLength: 256,
				Expired:   true,
				Identity:  KeyIdentity{Emails: []string{testEmail}},
			},
			wantRejection: "1 expired",
			wantKeysFound: "1",
			wantErrCode:   diagnostics.ReasonNoTrustedKeys,
		},
		{
			name: "weak_key",
			key: CryptoKeyInfo{
				Algorithm: KeyAlgorithmRSA,
				BitLength: 1024,
				Identity:  KeyIdentity{Emails: []string{testEmail}},
			},
			wantRejection: "1 below minimum strength",
			wantKeysFound: "1",
			wantErrCode:   diagnostics.ReasonNoTrustedKeys,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := &mockKeyProvider{
				directoryValid: true,
				gpgKeys:        []CryptoKeyInfo{testCase.key},
			}
			rule := NewCryptoSignatureRule(settings.Config{}, cryptoKeyProviders(provider))
			commit := domain.Commit{Signature: gpgSig}

			errors := rule.Validate(commit)

			check.NotEmpty(t, errors)
			check.Equal(t, testCase.wantErrCode, reasonOf(errors[0]))
			check.Equal(t, testCase.wantKeysFound, errors[0].Context["keys_found"])
			check.Contains(t, errors[0].Context["rejected"], testCase.wantRejection)
		})
	}
}

// Test ActiveChecks returns correct sub-checks based on configuration.
func TestCryptoSignatureRule_ActiveChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		config         settings.CryptoSignatureConfig
		hasKeyProvider bool
		expectedChecks []string
	}{
		{
			name:           "no_checks_when_all_disabled_no_provider",
			config:         settings.CryptoSignatureConfig{},
			hasKeyProvider: false,
			expectedChecks: nil,
		},
		{
			name: "presence_only",
			config: settings.CryptoSignatureConfig{
				Required: new(true),
			},
			hasKeyProvider: false,
			expectedChecks: []string{testActiveCheckPresence},
		},
		{
			name: "format_only",
			config: settings.CryptoSignatureConfig{
				VerifyFormat: new(true),
			},
			hasKeyProvider: false,
			expectedChecks: []string{testActiveCheckFormat},
		},
		{
			name: "type_with_require_gpg",
			config: settings.CryptoSignatureConfig{
				RequireGPG: new(true),
			},
			hasKeyProvider: false,
			expectedChecks: []string{testActiveCheckType},
		},
		{
			name: "type_with_require_ssh",
			config: settings.CryptoSignatureConfig{
				RequireSSH: new(true),
			},
			hasKeyProvider: false,
			expectedChecks: []string{testActiveCheckType},
		},
		{
			name:           "key_policy_with_provider",
			config:         settings.CryptoSignatureConfig{},
			hasKeyProvider: true,
			expectedChecks: []string{testActiveCheckKeyPolicy},
		},
		{
			name: "all_checks_active",
			config: settings.CryptoSignatureConfig{
				Required:     new(true),
				VerifyFormat: new(true),
				RequireGPG:   new(true),
			},
			hasKeyProvider: true,
			expectedChecks: []string{testActiveCheckPresence, testActiveCheckFormat, testActiveCheckType, testActiveCheckKeyPolicy},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			provider := CryptoKeyProviders{}
			if testCase.hasKeyProvider {
				provider = cryptoKeyProviders(&mockKeyProvider{directoryValid: true})
			}

			rule := NewCryptoSignatureRule(settings.Config{
				CryptoSignature: testCase.config,
			}, provider)

			checks := rule.ActiveChecks()
			check.Equal(t, testCase.expectedChecks, checks)
		})
	}
}

func TestCryptoSignatureRule_ImplementsCheckDescriber(t *testing.T) {
	t.Parallel()

	rule := NewCryptoSignatureRule(settings.Config{
		CryptoSignature: settings.CryptoSignatureConfig{
			Required: new(true),
		},
	}, CryptoKeyProviders{})

	var commitRule domain.CommitRule = rule

	describer, ok := commitRule.(domain.CheckDescriber)
	check.True(t, ok, "CryptoSignatureRule should implement CheckDescriber")
	check.Equal(t, []string{"presence"}, describer.ActiveChecks())
}

// Test key load failure.
func TestCryptoSignatureRule_KeyLoadFailure(t *testing.T) {
	t.Parallel()

	provider := &mockKeyProvider{
		directoryValid: true,
		shouldFail:     true,
		failError:      errKeyLoadFailed,
	}
	rule := NewCryptoSignatureRule(settings.Config{}, cryptoKeyProviders(provider))
	commit := domain.Commit{Signature: testGPGSignature}

	errors := rule.Validate(commit)
	check.NotEmpty(t, errors)
	check.Equal(t, diagnostics.ReasonKeyLoadFailed, reasonOf(errors[0]))
}
