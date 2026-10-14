// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"errors"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// CryptoSignatureRule validates signature presence, format, type, and key
// inventory. It answers "is this commit signed, and is the signature
// well-formed and of an accepted kind?" — NOT "was this commit signed by a key
// we trust?".
//
// This rule validates:
//   - Signature presence (when required: true)
//   - Signature format structure (when verify_format: true): correct armor for
//     the detected kind (GPG/SSH)
//   - Signature type matches requirements (require_gpg/require_ssh)
//   - An eligible public key EXISTS in key_directory (when key_directory is set):
//     of an accepted kind, meeting minimum strength (min_rsa_bits/min_ec_bits),
//     not revoked or expired (GPG only), and matching allowed_signers
//
// Cryptographic verification is intentionally OUT OF SCOPE for this rule: it
// never binds the commit's signature to any eligible key. A well-formed
// signature block of the right kind, plus a usable key in the directory, is
// enough to pass — the two are checked independently. No package in the
// product performs signature-to-key verification; use git verify-commit or
// the forge for that. User-facing messages are worded to reflect this
// narrower scope; do not reword them to imply trust is enforced.
type CryptoSignatureRule struct {
	gpgKeyLoader   GPGKeyLoader
	keyDirectory   KeyDirectoryValidator
	sshKeyLoader   SSHKeyLoader
	minRSABits     int
	minECBits      int
	allowedSigners []string
	required       bool
	requireGPG     bool
	requireSSH     bool
	verifyFormat   bool
}

// NewCryptoSignatureRule creates a signature rule with explicit key-provider capabilities.
func NewCryptoSignatureRule(cfg settings.Config, keyProviders CryptoKeyProviders) CryptoSignatureRule {
	minRSABits := settings.DerefPositiveInt(cfg.CryptoSignature.MinRSABits, settings.DefaultMinRSABits)
	minECBits := settings.DerefPositiveInt(cfg.CryptoSignature.MinECBits, settings.DefaultMinECBits)

	return CryptoSignatureRule{
		gpgKeyLoader:   keyProviders.GPG,
		keyDirectory:   keyProviders.Directory,
		sshKeyLoader:   keyProviders.SSH,
		minRSABits:     minRSABits,
		minECBits:      minECBits,
		allowedSigners: cfg.CryptoSignature.AllowedSigners,
		required:       settings.DerefBool(cfg.CryptoSignature.Required, false),
		requireGPG:     settings.DerefBool(cfg.CryptoSignature.RequireGPG, false),
		requireSSH:     settings.DerefBool(cfg.CryptoSignature.RequireSSH, false),
		verifyFormat:   settings.DerefBool(cfg.CryptoSignature.VerifyFormat, false),
	}
}

// Name implements Rule interface.
func (r CryptoSignatureRule) Name() string {
	return rulecatalog.CryptoSignature
}

// ActiveChecks implements CheckDescriber, returning labels for each active sub-check.
func (r CryptoSignatureRule) ActiveChecks() []string {
	var checks []string

	if r.required {
		checks = append(checks, "presence")
	}

	if r.verifyFormat {
		checks = append(checks, "format")
	}

	if r.requireGPG || r.requireSSH {
		checks = append(checks, "type")
	}

	if r.gpgKeyLoader != nil || r.sshKeyLoader != nil {
		checks = append(checks, "key policy")
	}

	return checks
}

// Validate implements CommitRule interface.
func (r CryptoSignatureRule) Validate(commit domain.Commit) []domain.ValidationError {
	// Check if signature is required but missing
	if errs := r.validatePresence(commit); len(errs) > 0 {
		return errs
	}

	// Skip remaining validation if no signature present
	if commit.Signature == "" {
		return nil
	}

	// Parse and validate the signature
	return r.validateSignature(commit.Signature)
}

// validatePresence checks if signature is present when required.
func (r CryptoSignatureRule) validatePresence(commit domain.Commit) []domain.ValidationError {
	if r.required && strings.TrimSpace(commit.Signature) == "" {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonMissingSignature, "Missing cryptographic signature").
				WithActualExpected("no signature", "signed commit"),
		}
	}

	return nil
}

// validateSignature validates the signature format, type, and key inventory.
func (r CryptoSignatureRule) validateSignature(rawSignature string) []domain.ValidationError {
	signature := domain.NewSignature(rawSignature)
	if signature.IsEmpty() {
		return nil
	}

	sigType := signature.Type()

	// Validate signature format if enabled
	if r.verifyFormat {
		if formatErrors := r.validateFormat(signature, sigType); len(formatErrors) > 0 {
			return formatErrors
		}
	}

	// Validate signature type is known
	if sigType == domain.SignatureTypeUnknown {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonUnknownSigFormat, "Unknown signature format").
				WithActualExpected("unknown format", "GPG or SSH signature"),
		}
	}

	// Validate signature type requirements (require_gpg/require_ssh)
	if errs := r.validateSignatureRequirements(sigType); len(errs) > 0 {
		return errs
	}

	// Validate key inventory if a key provider is configured.
	return r.validateKeyDirectory(sigType)
}

// validateKeyDirectory validates the key directory and eligible keys.
func (r CryptoSignatureRule) validateKeyDirectory(sigType domain.SignatureType) []domain.ValidationError {
	if !r.hasKeyLoader(sigType) {
		return nil
	}

	if r.keyDirectory != nil {
		if err := r.keyDirectory.ValidateDirectory(); err != nil {
			return []domain.ValidationError{
				domain.New(r.Name(), diagnostics.ReasonInvalidKeyDirectory, "Key directory validation failed").
					WithContextMap(keyProviderErrorContext(err)),
			}
		}
	}

	return r.validateTrustedKeys(sigType)
}

func (r CryptoSignatureRule) hasKeyLoader(sigType domain.SignatureType) bool {
	switch sigType {
	case domain.SignatureTypeGPG:
		return r.gpgKeyLoader != nil
	case domain.SignatureTypeSSH:
		return r.sshKeyLoader != nil
	case domain.SignatureTypeUnknown:
		return false
	}

	return false
}

func keyProviderErrorContext(err error) map[string]string {
	switch {
	case errors.Is(err, domain.ErrKeyDirectoryNotSpecified):
		return map[string]string{diagnostics.ContextReason: "directory_not_specified"}
	case errors.Is(err, domain.ErrKeyDirectoryNotAccessible):
		return map[string]string{diagnostics.ContextReason: "directory_inaccessible"}
	case errors.Is(err, domain.ErrKeyDirectoryNotDirectory):
		return map[string]string{diagnostics.ContextReason: "not_a_directory"}
	case errors.Is(err, domain.ErrKeyFileOutsideDirectory):
		return map[string]string{diagnostics.ContextReason: "file_outside_directory"}
	default:
		return map[string]string{diagnostics.ContextReason: "unknown"}
	}
}

// validateFormat validates signature format structure.
func (r CryptoSignatureRule) validateFormat(signature domain.Signature, sigType domain.SignatureType) []domain.ValidationError {
	switch sigType {
	case domain.SignatureTypeGPG:
		if !isCompleteGPGSignature(signature.Data()) {
			return []domain.ValidationError{
				domain.New(r.Name(), diagnostics.ReasonIncompleteGPGSig, "Incomplete GPG signature").
					WithActualExpected("incomplete GPG signature", "complete GPG signature"),
			}
		}
	case domain.SignatureTypeSSH:
		if !isCompleteSSHSignature(signature.Data()) {
			return []domain.ValidationError{
				domain.New(r.Name(), diagnostics.ReasonIncompleteSSHSig, "Incomplete SSH signature").
					WithActualExpected("incomplete SSH signature", "complete SSH signature"),
			}
		}
	case domain.SignatureTypeUnknown:
		// Will be caught by unknown type check in main Validate
	}

	return nil
}

// isCompleteGPGSignature checks if a GPG signature has the required components.
func isCompleteGPGSignature(signature string) bool {
	return domain.HasCompleteSignatureArmor(signature, domain.SignatureTypeGPG)
}

// isCompleteSSHSignature checks if an SSH signature has the required components.
func isCompleteSSHSignature(signature string) bool {
	return domain.HasCompleteSignatureArmor(signature, domain.SignatureTypeSSH)
}

// validateSignatureRequirements checks require_gpg/require_ssh constraints.
func (r CryptoSignatureRule) validateSignatureRequirements(sigType domain.SignatureType) []domain.ValidationError {
	if settings.HasConflictingSignatureRequirements(r.requireGPG, r.requireSSH) {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonInvalidConfig,
				"Conflicting signature requirements: both require_gpg and require_ssh are enabled").
				WithContextMap(diagnostics.Context{
					diagnostics.ContextRequireGPG: contextValueTrue,
					"require_ssh":                 contextValueTrue,
				}),
		}
	}

	if r.requireGPG && sigType != domain.SignatureTypeGPG {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonSignatureTypeNotAllowed, "GPG signature required").
				WithActualExpected(string(sigType), "GPG"),
		}
	}

	if r.requireSSH && sigType != domain.SignatureTypeSSH {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonSignatureTypeNotAllowed, "SSH signature required").
				WithActualExpected(string(sigType), "SSH"),
		}
	}

	return nil
}

// validateTrustedKeys checks that eligible keys exist for the signature type.
func (r CryptoSignatureRule) validateTrustedKeys(sigType domain.SignatureType) []domain.ValidationError {
	switch sigType {
	case domain.SignatureTypeGPG:
		return r.validateGPGKeys()
	case domain.SignatureTypeSSH:
		return r.validateSSHKeys()
	case domain.SignatureTypeUnknown:
		// Unknown type already handled in Validate before this is called
		return nil
	}

	return nil
}

// validateGPGKeys validates GPG keys in the key directory.
func (r CryptoSignatureRule) validateGPGKeys() []domain.ValidationError {
	return r.validateTrustedKeySet(trustedKeyValidationPolicy{
		keyType:         "GPG",
		loadError:       "Failed to load GPG keys",
		emptyError:      "No GPG keys found",
		expectedFiles:   "GPG public key files (.gpg, .asc)",
		loadKeys:        r.gpgKeyLoader.LoadGPGKeys,
		rejectKeyBefore: rejectUnavailableGPGKey,
	})
}

// validateSSHKeys validates SSH keys in the key directory.
func (r CryptoSignatureRule) validateSSHKeys() []domain.ValidationError {
	return r.validateTrustedKeySet(trustedKeyValidationPolicy{
		keyType:       "SSH",
		loadError:     "Failed to load SSH keys",
		emptyError:    "No SSH keys found",
		expectedFiles: "SSH public key files (.pub)",
		loadKeys:      r.sshKeyLoader.LoadSSHKeys,
	})
}

type trustedKeyValidationPolicy struct {
	keyType         string
	loadError       string
	emptyError      string
	expectedFiles   string
	loadKeys        func() ([]CryptoKeyInfo, error)
	rejectKeyBefore func(CryptoKeyInfo, *keyRejectionStats) bool
}

func (r CryptoSignatureRule) validateTrustedKeySet(policy trustedKeyValidationPolicy) []domain.ValidationError {
	keys, err := policy.loadKeys()
	if err != nil {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonKeyLoadFailed, policy.loadError).
				WithContextMap(keyProviderErrorContext(err)),
		}
	}

	if len(keys) == 0 {
		return []domain.ValidationError{
			domain.New(r.Name(), diagnostics.ReasonNoTrustedKeys, policy.emptyError).
				WithContextMap(diagnostics.Context{
					diagnostics.ContextExpected: policy.expectedFiles,
				}),
		}
	}

	var stats keyRejectionStats

	for _, key := range keys {
		stats.total++

		if policy.rejectKeyBefore != nil && policy.rejectKeyBefore(key, &stats) {
			continue
		}

		if !meetsMinimumKeyStrength(key, r.minRSABits, r.minECBits) {
			stats.weak++

			continue
		}

		if !key.Identity.Matches(r.allowedSigners) {
			stats.notAllowed++

			continue
		}

		return nil
	}

	return []domain.ValidationError{
		r.buildNoValidKeysError(policy.keyType, stats),
	}
}

func rejectUnavailableGPGKey(key CryptoKeyInfo, stats *keyRejectionStats) bool {
	if key.Revoked {
		stats.revoked++

		return true
	}

	if key.Expired {
		stats.expired++

		return true
	}

	return false
}

// keyRejectionStats tracks why keys were rejected.
type keyRejectionStats struct {
	total      int
	revoked    int
	expired    int
	weak       int
	notAllowed int
}

// buildNoValidKeysError creates an error when no valid keys are found.
func (r CryptoSignatureRule) buildNoValidKeysError(keyType string, stats keyRejectionStats) domain.ValidationError {
	message := "No valid " + keyType + " keys found"

	context := map[string]string{
		diagnostics.ContextType:      keyType,
		diagnostics.ContextKeysFound: strconv.Itoa(stats.total),
	}

	var reasons []string
	if stats.revoked > 0 {
		reasons = append(reasons, strconv.Itoa(stats.revoked)+" revoked")
	}

	if stats.expired > 0 {
		reasons = append(reasons, strconv.Itoa(stats.expired)+" expired")
	}

	if stats.weak > 0 {
		reasons = append(reasons, strconv.Itoa(stats.weak)+" below minimum strength")
	}

	if stats.notAllowed > 0 {
		reasons = append(reasons, strconv.Itoa(stats.notAllowed)+" not in allowed_signers")
	}

	if len(reasons) > 0 {
		context[diagnostics.ContextRejected] = strings.Join(reasons, ", ")
	}

	return domain.New(r.Name(), diagnostics.ReasonNoTrustedKeys, message).
		WithContextMap(context)
}
