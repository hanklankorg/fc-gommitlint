// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

func TestGPGSecuritySettings_UseSecureDefaults(t *testing.T) {
	t.Parallel()

	settings := DefaultGPGSecuritySettings()

	check.Equal(t, uint16(2048), settings.MinimumRSABits)
	check.Equal(t, uint16(256), settings.MinimumECBits)
}

func TestValidateGPGKey_RejectsNilEntity(t *testing.T) {
	t.Parallel()

	settings := DefaultGPGSecuritySettings()
	now := time.Now()

	t.Run("rejects nil entity", func(t *testing.T) {
		t.Parallel()

		result := ValidateGPGKey(nil, settings, now)
		check.False(t, result)
	})
}

func TestIsGPGKeyRevoked_TreatsNilAsRevoked(t *testing.T) {
	t.Parallel()

	t.Run("returns true for nil entity", func(t *testing.T) {
		t.Parallel()

		result := IsGPGKeyRevoked(nil, time.Now())
		check.True(t, result)
	})
}

func TestIsGPGKeyExpired_TreatsNilAsExpired(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("returns true for nil entity", func(t *testing.T) {
		t.Parallel()

		result := IsGPGKeyExpired(nil, now)
		check.True(t, result)
	})
}

func TestHasMinimumGPGKeyStrength_FailsClosedOnNil(t *testing.T) {
	t.Parallel()

	settings := DefaultGPGSecuritySettings()

	t.Run("returns false for nil entity", func(t *testing.T) {
		t.Parallel()

		result := HasMinimumGPGKeyStrength(nil, settings)
		check.False(t, result)
	})

	t.Run("returns false for nil primary key", func(t *testing.T) {
		t.Parallel()

		entity := &openpgp.Entity{PrimaryKey: nil}
		result := HasMinimumGPGKeyStrength(entity, settings)
		check.False(t, result)
	})
}

func TestExtractGPGIdentity_EmptyForNil(t *testing.T) {
	t.Parallel()

	t.Run("handles nil entity", func(t *testing.T) {
		t.Parallel()

		identity := ExtractGPGIdentity(nil)
		check.Empty(t, identity.Name)
		check.Empty(t, identity.Email)
	})
}

// TestValidateGPGKey_RefusesExpiredAndRevokedKeys covers the three ways a
// real key stops being usable. Every other validator test passes nil, so the
// expiry and revocation walks ran against no key material at all.
//
// The entities are generated in-process (Ed25519, no ambient GNUPGHOME or
// agent); only the self-signature fields that carry lifetimes and revocations
// are adjusted, which is what a key server hands the loader.
func TestValidateGPGKey_RefusesExpiredAndRevokedKeys(t *testing.T) {
	t.Parallel()

	settings := GPGSecuritySettings{MinimumECBits: 256, MinimumRSABits: 2048}
	now := time.Now()

	t.Run("an expired primary key is refused", func(t *testing.T) {
		t.Parallel()

		entity := newEd25519Entity(t)
		expireIdentities(entity, now.Add(-2*time.Hour), 3600)

		check.True(t, IsGPGKeyExpired(entity, now))
		check.False(t, ValidateGPGKey(entity, settings, now))

		// The control: the same key with a lifetime that has not run out.
		unexpired := newEd25519Entity(t)
		expireIdentities(unexpired, now.Add(-time.Hour), 24*3600)

		check.False(t, IsGPGKeyExpired(unexpired, now))
		check.True(t, ValidateGPGKey(unexpired, settings, now))
	})

	t.Run("an expired signing subkey leaves no way to sign", func(t *testing.T) {
		t.Parallel()

		entity := newSigningSubkeyEntity(t, now.Add(-2*time.Hour), 3600)

		check.True(t, IsGPGKeyExpired(entity, now))
		check.False(t, ValidateGPGKey(entity, settings, now))

		// The control: the same shape with a subkey that is still valid.
		valid := newSigningSubkeyEntity(t, now.Add(-time.Hour), 24*3600)

		check.False(t, IsGPGKeyExpired(valid, now))
		check.True(t, ValidateGPGKey(valid, settings, now))
	})

	t.Run("a revoked identity refuses the key", func(t *testing.T) {
		t.Parallel()

		entity := newEd25519Entity(t)
		expireIdentities(entity, now.Add(-time.Hour), 24*3600)

		// The control first: the same key answers "not revoked" before the
		// revocation is attached, so the assertion below is the revocation's
		// doing and not a constant.
		check.False(t, IsGPGKeyRevoked(entity, now))
		check.True(t, ValidateGPGKey(entity, settings, now))

		for _, identity := range entity.Identities {
			identity.Revocations = append(identity.Revocations, &packet.Signature{
				SigType:      packet.SigTypeCertificationRevocation,
				CreationTime: now.Add(-time.Minute),
			})
		}

		check.True(t, IsGPGKeyRevoked(entity, now))
		check.False(t, ValidateGPGKey(entity, settings, now))
	})
}

// TestIsGPGKeyExpired_ZeroLifetimeNeverExpires pins RFC 4880 §5.2.3.6: a key
// expiration subpacket that is absent or carries zero means the key never
// expires. go-crypto parses an explicit zero subpacket into a non-nil
// KeyLifetimeSecs of 0 (packet/signature.go, keyExpirationSubpacket), and its
// own PublicKey.KeyExpired treats that as unlimited; the validator has to
// agree, or such a key is refused as expired at its creation time.
func TestIsGPGKeyExpired_ZeroLifetimeNeverExpires(t *testing.T) {
	t.Parallel()

	settings := GPGSecuritySettings{MinimumECBits: 256, MinimumRSABits: 2048}
	now := time.Now()

	t.Run("a primary key with a zero lifetime", func(t *testing.T) {
		t.Parallel()

		entity := newEd25519Entity(t)
		expireIdentities(entity, now.Add(-time.Hour), 0)

		check.False(t, IsGPGKeyExpired(entity, now))
		check.True(t, ValidateGPGKey(entity, settings, now))

		// The control: a lifetime of one second that has run out still expires.
		expired := newEd25519Entity(t)
		expireIdentities(expired, now.Add(-time.Hour), 1)

		check.True(t, IsGPGKeyExpired(expired, now))
	})

	t.Run("a signing subkey with a zero lifetime", func(t *testing.T) {
		t.Parallel()

		entity := newSigningSubkeyEntity(t, now.Add(-time.Hour), 0)

		check.False(t, IsGPGKeyExpired(entity, now))
		check.True(t, ValidateGPGKey(entity, settings, now))

		expired := newSigningSubkeyEntity(t, now.Add(-time.Hour), 1)

		check.True(t, IsGPGKeyExpired(expired, now))
	})
}

// expireIdentities gives every identity self-signature a creation time and a
// lifetime, which is how a primary key's expiry is expressed. A generated
// entity carries a lifetime of zero seconds, which RFC 4880 reads as "never
// expires"; every entity used here states its own so the expiry is explicit.
func expireIdentities(entity *openpgp.Entity, createdAt time.Time, lifetimeSecs uint32) {
	for _, identity := range entity.Identities {
		identity.SelfSignature.CreationTime = createdAt
		identity.SelfSignature.KeyLifetimeSecs = &lifetimeSecs
	}
}

// newSigningSubkeyEntity returns an entity whose primary key cannot sign and
// whose own lifetime has not run out, so eligibility depends entirely on the
// signing subkey's lifetime.
func newSigningSubkeyEntity(t *testing.T, createdAt time.Time, lifetimeSecs uint32) *openpgp.Entity {
	t.Helper()

	entity := newEd25519Entity(t)
	expireIdentities(entity, createdAt, 30*24*3600)

	for _, identity := range entity.Identities {
		identity.SelfSignature.FlagSign = false
	}

	check.NotEmpty(t, entity.Subkeys, "the generated entity must carry a subkey to sign with")

	for index := range entity.Subkeys {
		entity.Subkeys[index].Sig.FlagsValid = true
		entity.Subkeys[index].Sig.FlagSign = true
		entity.Subkeys[index].Sig.CreationTime = createdAt
		entity.Subkeys[index].Sig.KeyLifetimeSecs = &lifetimeSecs
	}

	return entity
}

// keyAlgoTestCase a test case for key algorithm functions.
type keyAlgoTestCase struct {
	name     string
	algo     packet.PublicKeyAlgorithm
	expected bool
}

func TestIsRSAKey_ClassifiesEveryAlgorithm(t *testing.T) {
	t.Parallel()

	testCases := []keyAlgoTestCase{
		{
			name:     "RSA algorithm",
			algo:     packet.PubKeyAlgoRSA,
			expected: true,
		},
		{
			name:     "RSA encrypt-only algorithm",
			algo:     packet.PubKeyAlgoRSAEncryptOnly,
			expected: true,
		},
		{
			name:     "RSA sign-only algorithm",
			algo:     packet.PubKeyAlgoRSASignOnly,
			expected: true,
		},
		{
			name:     "ECDSA algorithm",
			algo:     packet.PubKeyAlgoECDSA,
			expected: false,
		},
		{
			name:     "EdDSA algorithm",
			algo:     packet.PubKeyAlgoEdDSA,
			expected: false,
		},
		{
			name:     "ECDH algorithm",
			algo:     packet.PubKeyAlgoECDH,
			expected: false,
		},
		{
			name:     "unknown algorithm",
			algo:     packet.PublicKeyAlgorithm(99),
			expected: false,
		},
	}

	testKeyAlgorithmFunction(t, testCases, isRSAKey)
}

func TestIsECKey_ClassifiesEveryAlgorithm(t *testing.T) {
	t.Parallel()

	testCases := []keyAlgoTestCase{
		{
			name:     "ECDSA algorithm",
			algo:     packet.PubKeyAlgoECDSA,
			expected: true,
		},
		{
			name:     "EdDSA algorithm",
			algo:     packet.PubKeyAlgoEdDSA,
			expected: true,
		},
		{
			name:     "ECDH algorithm",
			algo:     packet.PubKeyAlgoECDH,
			expected: true,
		},
		{
			name:     "RSA algorithm",
			algo:     packet.PubKeyAlgoRSA,
			expected: false,
		},
		{
			name:     "RSA encrypt-only algorithm",
			algo:     packet.PubKeyAlgoRSAEncryptOnly,
			expected: false,
		},
		{
			name:     "RSA sign-only algorithm",
			algo:     packet.PubKeyAlgoRSASignOnly,
			expected: false,
		},
		{
			name:     "unknown algorithm",
			algo:     packet.PublicKeyAlgorithm(99),
			expected: false,
		},
	}

	testKeyAlgorithmFunction(t, testCases, isECKey)
}

// testKeyAlgorithmFunction is a helper to test key algorithm functions.
func testKeyAlgorithmFunction(t *testing.T, testCases []keyAlgoTestCase, testFunc func(packet.PublicKeyAlgorithm) bool) {
	t.Helper()

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := testFunc(testCase.algo)
			check.Equal(t, testCase.expected, result)
		})
	}
}
