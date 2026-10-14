// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"fmt"
	"strings"
)

// SignatureType the type of cryptographic signature.
type SignatureType string

// Known signature types.
const (
	SignatureTypeGPG     SignatureType = "gpg"
	SignatureTypeSSH     SignatureType = "ssh"
	SignatureTypeUnknown SignatureType = "unknown"
	truncateEllipsis                   = "..."

	// GPGSignatureBegin is the documented ASCII armor begin marker for OpenPGP signatures.
	GPGSignatureBegin = "-----BEGIN PGP SIGNATURE-----"
	// GPGSignatureEnd is the documented ASCII armor end marker for OpenPGP signatures.
	GPGSignatureEnd = "-----END PGP SIGNATURE-----"
	// SSHSignatureBegin is the documented ASCII armor begin marker for SSH signatures.
	SSHSignatureBegin = "-----BEGIN SSH SIGNATURE-----"
	// SSHSignatureEnd is the documented ASCII armor end marker for SSH signatures.
	SSHSignatureEnd = "-----END SSH SIGNATURE-----"
)

// Signature a cryptographic signature with its format and data.
type Signature struct {
	sigType SignatureType
	data    string
}

// NewSignature creates a new signature from the given data.
// It automatically detects the signature type.
func NewSignature(data string) Signature {
	return Signature{
		sigType: detectSignatureType(data),
		data:    data,
	}
}

// NewSignatureWithType creates a new signature with the specified type.
func NewSignatureWithType(data string, sigType SignatureType) Signature {
	return Signature{
		sigType: sigType,
		data:    data,
	}
}

// Type returns the type of the signature.
func (s Signature) Type() SignatureType {
	return s.sigType
}

// Data returns the raw signature data.
func (s Signature) Data() string {
	return s.data
}

// IsEmpty reports whether the signature data is empty.
func (s Signature) IsEmpty() bool {
	return s.data == ""
}

// IsValid reports whether the signature has a known type and non-empty data.
func (s Signature) IsValid() bool {
	return s.sigType != SignatureTypeUnknown && !s.IsEmpty()
}

// String returns a string representation of the signature.
func (s Signature) String() string {
	return fmt.Sprintf("%s signature: %s", s.sigType, truncate(s.data, 20))
}

// detectSignatureType determines the type of signature from its content.
func detectSignatureType(signature string) SignatureType {
	signature = strings.TrimSpace(signature)

	if signature == "" {
		return SignatureTypeUnknown
	}

	if isGPGSignature(signature) {
		return SignatureTypeGPG
	}

	if isSSHSignature(signature) {
		return SignatureTypeSSH
	}

	return SignatureTypeUnknown
}

// isGPGSignature checks if the signature is a GPG signature.
func isGPGSignature(signature string) bool {
	return HasSignatureArmorHeader(signature, SignatureTypeGPG)
}

// isSSHSignature checks if the signature is an SSH signature.
func isSSHSignature(signature string) bool {
	if hasSSHSignatureHeaders(signature) {
		return true
	}

	return hasSSHSignatureFormat(signature)
}

// hasSSHSignatureHeaders checks for SSH signature header patterns.
func hasSSHSignatureHeaders(signature string) bool {
	sshPatterns := []string{
		SSHSignatureBegin,
		"-----BEGIN SSH SIGN",
		"SSH SIGNATURE",
		"ssh-rsa",
		"ssh-ed25519",
	}

	for _, pattern := range sshPatterns {
		if strings.Contains(signature, pattern) {
			return true
		}
	}

	return false
}

// SignatureArmorMarkers returns the documented armor markers for a signature type.
func SignatureArmorMarkers(sigType SignatureType) (string, string, bool) {
	switch sigType {
	case SignatureTypeGPG:
		return GPGSignatureBegin, GPGSignatureEnd, true
	case SignatureTypeSSH:
		return SSHSignatureBegin, SSHSignatureEnd, true
	case SignatureTypeUnknown:
		return "", "", false
	}

	return "", "", false
}

// HasSignatureArmorHeader reports whether data contains an exact armor header line.
func HasSignatureArmorHeader(data string, sigType SignatureType) bool {
	beginMarker, _, ok := SignatureArmorMarkers(sigType)
	if !ok {
		return false
	}

	for line := range strings.SplitSeq(data, "\n") {
		if strings.TrimSpace(line) == beginMarker {
			return true
		}
	}

	return false
}

// HasCompleteSignatureArmor reports whether data has begin and end marker lines with body content.
func HasCompleteSignatureArmor(data string, sigType SignatureType) bool {
	beginMarker, endMarker, ok := SignatureArmorMarkers(sigType)
	if !ok {
		return false
	}

	seenBegin := false
	hasBody := false

	for line := range strings.SplitSeq(data, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == beginMarker:
			seenBegin = true
		case trimmed == endMarker:
			return seenBegin && hasBody
		case seenBegin && trimmed != "" && !strings.Contains(trimmed, ":"):
			hasBody = true
		}
	}

	return false
}

// hasSSHSignatureFormat checks for SSH signature format (format:blob).
func hasSSHSignatureFormat(signature string) bool {
	if !strings.Contains(signature, ":") {
		return false
	}

	prefix, _, found := strings.Cut(signature, ":")
	if !found {
		return false
	}

	prefix = strings.ToLower(prefix)
	sshPrefixes := []string{"ssh-", "ecdsa-", "sk-"}

	for _, sshPrefix := range sshPrefixes {
		if strings.HasPrefix(prefix, sshPrefix) {
			return true
		}
	}

	return false
}

// truncate shortens a string to the specified length in runes, adding "..." if truncated.
func truncate(str string, maxLen int) string {
	if maxLen <= 0 {
		return truncateEllipsis
	}

	runes := []rune(str)
	if len(runes) <= maxLen {
		return str
	}

	return string(runes[:maxLen]) + truncateEllipsis
}
