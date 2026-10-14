// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// Elliptic-curve strength checking for GPG keys.
//
// Generated entities contain the parsed key material that production receives.
// These tests remain untagged because key-strength validation must run in the
// ordinary test tier.

func newEd25519Entity(t *testing.T) *openpgp.Entity {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "", "test@example.com", &packet.Config{
		Algorithm: packet.PubKeyAlgoEdDSA,
		Curve:     packet.Curve25519,
	})
	check.NoError(t, err)

	return entity
}

func newECDSAEntity(t *testing.T, curve packet.Curve) *openpgp.Entity {
	t.Helper()

	entity, err := openpgp.NewEntity("Test User", "", "test@example.com", &packet.Config{
		Algorithm: packet.PubKeyAlgoECDSA,
		Curve:     curve,
	})
	check.NoError(t, err)

	return entity
}

func TestHasMinimumGPGKeyStrength_Ed25519(t *testing.T) {
	t.Parallel()

	entity := newEd25519Entity(t)

	t.Run("accepts a threshold at its strength", func(t *testing.T) {
		t.Parallel()

		result := HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{MinimumECBits: 256})
		check.True(t, result, "Ed25519 is 256 bits and should meet a 256-bit minimum")
	})

	t.Run("accepts a threshold below its strength", func(t *testing.T) {
		t.Parallel()

		result := HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{MinimumECBits: 128})
		check.True(t, result, "Ed25519 should meet a 128-bit minimum")
	})

	t.Run("rejects a threshold above its strength", func(t *testing.T) {
		t.Parallel()

		result := HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{MinimumECBits: 384})
		check.False(t, result, "Ed25519 is 256 bits and must not pass a 384-bit minimum")
	})
}

func TestHasMinimumGPGKeyStrength_ECDSA(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		curve    packet.Curve
		minimum  uint16
		expected bool
	}{
		{name: "P256 meets a 256-bit minimum", curve: packet.CurveNistP256, minimum: 256, expected: true},
		{name: "P256 fails a 384-bit minimum", curve: packet.CurveNistP256, minimum: 384, expected: false},
		{name: "P384 meets a 256-bit minimum", curve: packet.CurveNistP384, minimum: 256, expected: true},
		{name: "P384 meets a 384-bit minimum", curve: packet.CurveNistP384, minimum: 384, expected: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			entity := newECDSAEntity(t, testCase.curve)

			result := HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{MinimumECBits: testCase.minimum})
			check.Equal(t, testCase.expected, result)
		})
	}
}

// An EC key must not be waved through by an RSA threshold, or vice versa: the
// two minimums are separate settings because the bit counts are not comparable,
// and reading the wrong one would let a 256-bit curve satisfy a 4096-bit policy.
func TestHasMinimumGPGKeyStrength_ThresholdsDoNotCross(t *testing.T) {
	t.Parallel()

	entity := newEd25519Entity(t)

	result := HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{
		MinimumRSABits: 4096,
		MinimumECBits:  256,
	})
	check.True(t, result, "an EC key is judged by MinimumECBits, not MinimumRSABits")

	result = HasMinimumGPGKeyStrength(entity, GPGSecuritySettings{
		MinimumRSABits: 1024,
		MinimumECBits:  384,
	})
	check.False(t, result, "a lenient RSA minimum must not rescue an EC key below MinimumECBits")
}
