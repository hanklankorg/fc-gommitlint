// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import "codefloe.com/itiquette/gommitlint/internal/domain"

// Key algorithm categories for strength checking.
const (
	KeyAlgorithmRSA     = "RSA"
	KeyAlgorithmECDSA   = "ECDSA"
	KeyAlgorithmEd25519 = "Ed25519"
)

// CryptoKeyInfo holds pre-analyzed cryptographic key information.
// This allows the domain to validate key policies without depending
// on crypto-specific library types (openpgp, ssh).
type CryptoKeyInfo struct {
	Algorithm string
	BitLength int
	Revoked   bool
	Expired   bool
	Identity  KeyIdentity
}

// KeyDirectoryValidator checks whether configured key storage is usable.
type KeyDirectoryValidator interface {
	ValidateDirectory() error
}

// GPGKeyLoader loads and analyzes GPG keys.
type GPGKeyLoader interface {
	LoadGPGKeys() ([]CryptoKeyInfo, error)
}

// SSHKeyLoader loads and analyzes SSH keys.
type SSHKeyLoader interface {
	LoadSSHKeys() ([]CryptoKeyInfo, error)
}

// CryptoKeyProviders groups optional key-provider capabilities for signature policy validation.
type CryptoKeyProviders struct {
	Directory KeyDirectoryValidator
	GPG       GPGKeyLoader
	SSH       SSHKeyLoader
}

// SpellChecker defines the interface for spell checking operations.
// This interface is defined by the consumer (domain), following Go's principle
// of "accept interfaces, return concrete types".
type SpellChecker interface {
	CheckText(text string) []domain.Misspelling
}
