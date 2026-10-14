// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import "slices"

import "strings"

// KeyIdentity holds all matchable identity forms for a cryptographic key.
// This allows flexible matching against fingerprints, emails, or full identity strings.
type KeyIdentity struct {
	Fingerprint string   // SHA256 for SSH, hex for GPG
	Emails      []string // Extracted from User IDs or comments
	UserIDs     []string // Full User ID strings (GPG) or comment (SSH)
}

// KeyStrengthPolicy defines minimum accepted key sizes by algorithm family.
type KeyStrengthPolicy struct {
	MinECBits  int
	MinRSABits int
}

// Matches checks if any allowed signer matches this key identity.
// Empty allowedSigners permits all keys.
func (k KeyIdentity) Matches(allowedSigners []string) bool {
	if len(allowedSigners) == 0 {
		return true
	}

	return slices.ContainsFunc(allowedSigners, k.matchesSigner)
}

// PrimaryIdentity returns the best identifier for display purposes.
func (k KeyIdentity) PrimaryIdentity() string {
	if len(k.UserIDs) > 0 {
		return k.UserIDs[0]
	}

	if len(k.Emails) > 0 {
		return k.Emails[0]
	}

	return k.Fingerprint
}

// matchesSigner checks if a single allowed signer matches this identity.
func (k KeyIdentity) matchesSigner(allowed string) bool {
	// Match fingerprint (case-insensitive for hex)
	if strings.EqualFold(k.Fingerprint, allowed) {
		return true
	}

	// Match any email (case-insensitive)
	for _, email := range k.Emails {
		if strings.EqualFold(email, allowed) {
			return true
		}
	}

	// Match any User ID exactly
	return slices.Contains(k.UserIDs, allowed)
}

// meetsMinimumKeyStrength checks if a key meets minimum strength requirements.
func meetsMinimumKeyStrength(key CryptoKeyInfo, minRSABits, minECBits int) bool {
	return KeyStrengthPolicy{MinRSABits: minRSABits, MinECBits: minECBits}.Allows(key)
}

// Allows reports whether a key satisfies this strength policy.
func (p KeyStrengthPolicy) Allows(key CryptoKeyInfo) bool {
	switch key.Algorithm {
	case KeyAlgorithmRSA:
		return key.BitLength >= p.MinRSABits
	case KeyAlgorithmECDSA, KeyAlgorithmEd25519:
		return key.BitLength >= p.MinECBits
	default:
		return false
	}
}
