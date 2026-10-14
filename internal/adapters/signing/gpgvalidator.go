// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	domainrules "codefloe.com/itiquette/gommitlint/internal/domain/rules"
	configsettings "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// GPGSecuritySettings defines security requirements for GPG keys.
type GPGSecuritySettings struct {
	MinimumRSABits uint16
	MinimumECBits  uint16
}

// DefaultGPGSecuritySettings provides reasonable default security settings.
func DefaultGPGSecuritySettings() GPGSecuritySettings {
	return GPGSecuritySettings{
		MinimumRSABits: uint16(configsettings.DefaultMinRSABits),
		MinimumECBits:  uint16(configsettings.DefaultMinECBits),
	}
}

// ValidateGPGKey checks if a GPG key is valid for verification.
func ValidateGPGKey(entity *openpgp.Entity, settings GPGSecuritySettings, now time.Time) bool {
	if entity == nil {
		return false
	}

	if IsGPGKeyRevoked(entity, now) {
		return false
	}

	if IsGPGKeyExpired(entity, now) {
		return false
	}

	if !HasMinimumGPGKeyStrength(entity, settings) {
		return false
	}

	return true
}

// IsGPGKeyRevoked checks if a GPG key has been revoked at time now.
//
// A key is revoked when it carries a valid revocation signature; the "reason
// for revocation" subpacket is optional (RFC 4880 §5.2.3.23), so its absence
// must not turn a revoked key back into an eligible one. The library's own
// Revoked answers that for the primary key; identity revocations are checked
// against the field that holds them.
func IsGPGKeyRevoked(entity *openpgp.Entity, now time.Time) bool {
	if entity == nil {
		return true
	}

	if entity.Revoked(now) {
		return true
	}

	for _, id := range entity.Identities {
		for _, sig := range id.Revocations {
			if !sig.SigExpired(now) {
				return true
			}
		}
	}

	return false
}

// IsGPGKeyExpired checks if a GPG key has expired.
func IsGPGKeyExpired(entity *openpgp.Entity, now time.Time) bool {
	if entity == nil {
		return true
	}

	if isPrimaryKeyExpired(entity, now) {
		return true
	}

	return !hasValidPrimarySigningKey(entity) && !hasValidSigningSubkey(entity, now)
}

// isPrimaryKeyExpired checks if the primary key is expired.
func isPrimaryKeyExpired(entity *openpgp.Entity, now time.Time) bool {
	for _, ident := range entity.Identities {
		if ident.SelfSignature != nil && signatureKeyExpired(ident.SelfSignature, now) {
			return true
		}
	}

	return false
}

// hasValidSigningSubkey checks if there are any valid signing subkeys.
func hasValidSigningSubkey(entity *openpgp.Entity, now time.Time) bool {
	for _, subkey := range entity.Subkeys {
		// Only check subkeys that can sign
		if subkey.Sig != nil && subkey.Sig.FlagsValid && subkey.Sig.FlagSign && !signatureKeyExpired(subkey.Sig, now) {
			return true
		}
	}

	return false
}

// signatureKeyExpired reports whether the key lifetime a self-signature
// carries has run out at now. A key expiration subpacket that is absent or
// zero means the key never expires (RFC 4880 §5.2.3.6); go-crypto parses an
// explicit zero into a non-nil KeyLifetimeSecs, so nil and zero are one case.
func signatureKeyExpired(sig *packet.Signature, now time.Time) bool {
	if sig.KeyLifetimeSecs == nil || *sig.KeyLifetimeSecs == 0 {
		return false
	}

	expiry := sig.CreationTime.Add(time.Duration(*sig.KeyLifetimeSecs) * time.Second)

	return now.After(expiry)
}

func hasValidPrimarySigningKey(entity *openpgp.Entity) bool {
	if entity.PrimaryKey == nil || !primaryGPGKeyCanSign(entity.PrimaryKey.PubKeyAlgo) {
		return false
	}

	for _, ident := range entity.Identities {
		if ident.SelfSignature == nil {
			continue
		}

		if ident.SelfSignature.FlagsValid && ident.SelfSignature.FlagSign {
			return true
		}
	}

	return false
}

func primaryGPGKeyCanSign(algo packet.PublicKeyAlgorithm) bool {
	return algo == packet.PubKeyAlgoRSA ||
		algo == packet.PubKeyAlgoRSASignOnly ||
		algo == packet.PubKeyAlgoECDSA ||
		algo == packet.PubKeyAlgoEdDSA
}

// HasMinimumGPGKeyStrength checks if a GPG key meets minimum strength requirements.
func HasMinimumGPGKeyStrength(entity *openpgp.Entity, settings GPGSecuritySettings) bool {
	algorithm, bitLength := extractGPGAlgorithmInfo(entity)

	return domainrules.KeyStrengthPolicy{
		MinECBits:  int(settings.MinimumECBits),
		MinRSABits: int(settings.MinimumRSABits),
	}.Allows(domainrules.CryptoKeyInfo{Algorithm: algorithm, BitLength: bitLength})
}

// isRSAKey checks if the algorithm is an RSA variant.
func isRSAKey(algo packet.PublicKeyAlgorithm) bool {
	return algo == packet.PubKeyAlgoRSA ||
		algo == packet.PubKeyAlgoRSAEncryptOnly ||
		algo == packet.PubKeyAlgoRSASignOnly
}

// isECKey checks if the algorithm is an EC variant.
func isECKey(algo packet.PublicKeyAlgorithm) bool {
	return algo == packet.PubKeyAlgoECDSA ||
		algo == packet.PubKeyAlgoEdDSA ||
		algo == packet.PubKeyAlgoECDH
}

// ExtractGPGIdentity extracts a domain.Identity from an OpenPGP entity.
func ExtractGPGIdentity(entity *openpgp.Entity) domain.Identity {
	if entity == nil {
		return domain.NewIdentity("", "")
	}

	for name := range entity.Identities {
		// Usually in format "Name <email@example.com>"
		return domain.NewIdentityFromString(name)
	}

	// Fallback: Use key ID if no identities
	if entity.PrimaryKey == nil {
		return domain.NewIdentity("", "")
	}

	keyID := entity.PrimaryKey.KeyIdString()

	return domain.NewIdentity("GPG Key "+keyID, "")
}
