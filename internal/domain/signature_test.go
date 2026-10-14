// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestTruncate_ShortenLongStringsWithEllipsis tests the truncate function.
func TestTruncate_ShortenLongStringsWithEllipsis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "short string not truncated",
			input:  "short",
			maxLen: 10,
			want:   "short",
		},
		{
			name:   "exact length not truncated",
			input:  "exactlyten",
			maxLen: 10,
			want:   "exactlyten",
		},
		{
			name:   "long string truncated",
			input:  "this is a very long string that needs truncation",
			maxLen: 10,
			want:   "this is a ...",
		},
		{
			name:   testNameEmptyString,
			input:  "",
			maxLen: 10,
			want:   "",
		},
		{
			name:   "zero max length",
			input:  testString,
			maxLen: 0,
			want:   truncateEllipsis,
		},
		{
			name:   "negative max length",
			input:  testString,
			maxLen: -1,
			want:   truncateEllipsis,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := truncate(testCase.input, testCase.maxLen)

			check.Equal(t, testCase.want, result)
		})
	}
}

// TestHasSSHSignatureFormat tests SSH signature format detection.
func TestHasSSHSignatureFormat_RequiresFormatColonBlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		signature string
		want      bool
	}{
		{
			name:      "valid SSH public key format",
			signature: "ssh-rsa:AAAAB3NzaC1yc2EAAAADAQABAAABgQC7...",
			want:      true,
		},
		{
			name:      "valid SSH ed25519 key format",
			signature: "ssh-ed25519:AAAAC3NzaC1lZDI1NTE5AAAAI...",
			want:      true,
		},
		{
			name:      "invalid format without colon",
			signature: "-----BEGIN SSH SIGNATURE-----",
			want:      false,
		},
		{
			name:      testNameEmptyString,
			signature: "",
			want:      false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := hasSSHSignatureFormat(testCase.signature)

			check.Equal(t, testCase.want, result)
		})
	}
}

// TestSignature_AutoDetectsTypeFromData tests automatic type detection.
func TestSignature_AutoDetectsTypeFromData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     string
		wantType SignatureType
	}{
		{
			name:     "auto-detect GPG",
			data:     "-----BEGIN PGP SIGNATURE-----\nversion content\n-----END PGP SIGNATURE-----",
			wantType: SignatureTypeGPG,
		},
		{
			name:     "indented GPG armor header",
			data:     " -----BEGIN PGP SIGNATURE-----\n version content\n -----END PGP SIGNATURE-----",
			wantType: SignatureTypeGPG,
		},
		{
			name:     "bare OpenPGP packet prefix is unknown",
			data:     "iQAABCAAGBQJtest=",
			wantType: SignatureTypeUnknown,
		},
		{
			name:     "embedded GPG marker is unknown",
			data:     "not a signature -----BEGIN PGP SIGNATURE-----",
			wantType: SignatureTypeUnknown,
		},
		{
			name:     "whitespace only",
			data:     "   \n\t  ",
			wantType: SignatureTypeUnknown,
		},
		{
			name:     "auto-detect SSH",
			data:     "-----BEGIN SSH SIGNATURE-----\nsignature content\n-----END SSH SIGNATURE-----",
			wantType: SignatureTypeSSH,
		},
		{
			name:     "unknown format",
			data:     "some random data",
			wantType: SignatureTypeUnknown,
		},
		{
			name:     "empty data",
			data:     "",
			wantType: SignatureTypeUnknown,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sig := NewSignature(testCase.data)

			check.Equal(t, testCase.wantType, sig.Type())
		})
	}
}
