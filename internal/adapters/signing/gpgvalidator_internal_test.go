// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

func TestHasMinimumGPGKeyStrength_EnforcesRSAMinimumBits(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		keyBitSize  int
		minRSABits  uint16
		expected    bool
		description string
	}{
		{
			name:        "4096-bit key meets 2048-bit requirement",
			keyBitSize:  4096,
			minRSABits:  2048,
			expected:    true,
			description: "4096-bit RSA key should pass 2048-bit minimum",
		},
		{
			name:        "2048-bit key meets 2048-bit requirement",
			keyBitSize:  2048,
			minRSABits:  2048,
			expected:    true,
			description: "2048-bit RSA key should pass 2048-bit minimum",
		},
		{
			name:        "1024-bit key fails 2048-bit requirement",
			keyBitSize:  1024,
			minRSABits:  2048,
			expected:    false,
			description: "1024-bit RSA key should fail 2048-bit minimum",
		},
		{
			name:        "2048-bit key fails 4096-bit requirement",
			keyBitSize:  2048,
			minRSABits:  4096,
			expected:    false,
			description: "2048-bit RSA key should fail 4096-bit minimum",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create RSA entity with specific bit size
			entity := createTestRSAEntityInternal(t, testCase.keyBitSize)
			settings := GPGSecuritySettings{
				MinimumRSABits: testCase.minRSABits,
				MinimumECBits:  256,
			}

			result := HasMinimumGPGKeyStrength(entity, settings)
			check.Equal(t, testCase.expected, result, testCase.description)
		})
	}
}

// TestIsGPGKeyExpired_RequiresSigningCapability covers how signing capability on
// the primary key or a subkey determines whether the key counts as expired.
func TestIsGPGKeyExpired_RequiresSigningCapability(t *testing.T) {
	t.Parallel()

	now := time.Now()

	testCases := []struct {
		name     string
		entity   *openpgp.Entity
		expected bool
	}{
		{
			name:     "entity with no signing capability considered expired",
			entity:   createTestRSAEntityInternal(t, 2048),
			expected: true,
		},
		{
			name:     "primary signing key without subkeys is not expired",
			entity:   withPrimarySigningIdentity(createTestRSAEntityInternal(t, 2048), true),
			expected: false,
		},
		{
			name:     "primary key without signing flag is expired without signing subkey",
			entity:   withPrimarySigningIdentity(createTestRSAEntityInternal(t, 2048), false),
			expected: true,
		},
		{
			name:     "eddsa primary with a signing self-signature is not expired",
			entity:   primaryOnlyEntity(newEd25519Entity(t), now, 0),
			expected: false,
		},
		{
			name:     "ecdsa primary with a signing self-signature is not expired",
			entity:   primaryOnlyEntity(newECDSAEntity(t, packet.CurveNistP256), now, 0),
			expected: false,
		},
		{
			name:     "primary whose algorithm cannot sign is expired",
			entity:   primaryOnlyEntity(newEd25519Entity(t), now, packet.PubKeyAlgoECDH),
			expected: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := IsGPGKeyExpired(testCase.entity, now)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// primaryOnlyEntity strips the generated subkeys and gives the identity
// self-signature a lifetime still running at now, so whether the key can sign
// depends only on the primary key's algorithm. A non-zero algo relabels the
// primary key, which is how an algorithm without signing is expressed.
func primaryOnlyEntity(entity *openpgp.Entity, now time.Time, algo packet.PublicKeyAlgorithm) *openpgp.Entity {
	expireIdentities(entity, now.Add(-time.Hour), 24*3600)
	entity.Subkeys = nil

	if algo != 0 {
		entity.PrimaryKey.PubKeyAlgo = algo
	}

	return entity
}

func withPrimarySigningIdentity(entity *openpgp.Entity, canSign bool) *openpgp.Entity {
	uid := packet.NewUserId("Test User", "test@example.com", "")
	entity.Identities[uid.Id] = &openpgp.Identity{
		Name:   uid.Id,
		UserId: uid,
		SelfSignature: &packet.Signature{
			CreationTime: time.Now(),
			FlagsValid:   true,
			FlagSign:     canSign,
		},
	}

	return entity
}

func TestIsGPGKeyRevoked_FalseWithoutRevocations(t *testing.T) {
	t.Parallel()

	entity := createTestRSAEntityInternal(t, 2048)
	check.False(t, IsGPGKeyRevoked(entity, time.Now()))
}

// TestExtractGPGIdentity_FallsBackForMissingKeyOrIdentities covers the empty-identity
// and key-ID fallbacks when key data or identities are absent.
func TestExtractGPGIdentity_FallsBackForMissingKeyOrIdentities(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		entity        *openpgp.Entity
		expectedName  string
		expectedEmail string
		description   string
	}{
		{
			name:         "entity with nil primary key",
			entity:       &openpgp.Entity{PrimaryKey: nil},
			expectedName: "",
			description:  "Should return empty identity when key data is missing",
		},
		{
			name:         "entity with no identities uses key ID",
			entity:       createTestRSAEntityInternal(t, 2048),
			expectedName: "GPG Key ",
			description:  "Should use key ID when no identities present",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			identity := ExtractGPGIdentity(testCase.entity)

			// For key ID fallback, we just check that name starts with "GPG Key "
			if testCase.expectedName == "GPG Key " {
				check.Greater(t, len(identity.Name), len("GPG Key "), "Name should include key ID")
				check.True(t, strings.HasPrefix(identity.Name, "GPG Key "), "Name should start with 'GPG Key '")
			} else {
				check.Equal(t, testCase.expectedName, identity.Name)
			}

			check.Equal(t, testCase.expectedEmail, identity.Email)
		})
	}
}

func createTestRSAEntityInternal(t *testing.T, bitSize int) *openpgp.Entity {
	t.Helper()
	// Use shared cached key instead of generating new ones - much faster
	privateKey := getSharedRSAKeyInternal(t, bitSize)

	// Create entity with RSA public key
	pubKey := packet.NewRSAPublicKey(time.Now(), &privateKey.PublicKey)

	entity := &openpgp.Entity{
		PrimaryKey: pubKey,
		Identities: make(map[string]*openpgp.Identity),
		Subkeys:    []openpgp.Subkey{},
	}

	return entity
}

// getSharedRSAKeyInternal returns a cached RSA private key following functional principles.
// This avoids expensive key generation in tests by using shared immutable test resources.
func getSharedRSAKeyInternal(t *testing.T, bitSize int) *rsa.PrivateKey {
	t.Helper()

	// Use cached keys based on bit size to avoid regeneration
	switch bitSize {
	case 4096:
		return getSharedRSA4096KeyInternal(t)
	case 2048:
		return getSharedRSA2048KeyInternal(t)
	case 1024:
		return getSharedRSA1024KeyInternal(t)
	default:
		t.Fatalf("Unsupported key size for shared internal test key: %d", bitSize)

		return nil
	}
}

// Static errors for consistent error handling.
var (
	errDecodePEM = errors.New("failed to decode RSA private key PEM")
)

// loadRSAKeyFromTestdata loads an RSA private key from testdata directory.
func loadRSAKeyFromTestdata(filename string) (*rsa.PrivateKey, error) {
	// Find project root by walking up from current directory
	currentDir, _ := os.Getwd()
	for currentDir != "/" {
		if _, err := os.Stat(filepath.Join(currentDir, "go.mod")); err == nil {
			break
		}

		currentDir = filepath.Dir(currentDir)
	}

	testKeyPath := filepath.Join(currentDir, "test", "integration", "testdata", "keys", filename)

	keyData, err := os.ReadFile(testKeyPath) // #nosec G304 - Test file path is controlled
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", filename, err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("failed to decode %s: %w", filename, errDecodePEM)
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	return key, nil
}

// Shared internal RSA key caches using testdata for true immutable resources.
var getSharedRSA4096KeyInternal = func() func(*testing.T) *rsa.PrivateKey {
	var (
		key    *rsa.PrivateKey
		keyErr error
	)

	once := &sync.Once{}

	return func(t *testing.T) *rsa.PrivateKey {
		t.Helper()
		once.Do(func() {
			key, keyErr = loadRSAKeyFromTestdata("rsa_4096_private.pem")
		})
		check.NoError(t, keyErr, "Failed to load shared RSA 4096-bit test key from testdata")

		return key
	}
}()

var getSharedRSA2048KeyInternal = func() func(*testing.T) *rsa.PrivateKey {
	var (
		key    *rsa.PrivateKey
		keyErr error
	)

	once := &sync.Once{}

	return func(t *testing.T) *rsa.PrivateKey {
		t.Helper()
		once.Do(func() {
			key, keyErr = loadRSAKeyFromTestdata("rsa_2048_private.pem")
		})
		check.NoError(t, keyErr, "Failed to load shared RSA 2048-bit test key from testdata")

		return key
	}
}()

var getSharedRSA1024KeyInternal = func() func(*testing.T) *rsa.PrivateKey {
	var (
		key    *rsa.PrivateKey
		keyErr error
	)

	once := &sync.Once{}

	return func(t *testing.T) *rsa.PrivateKey {
		t.Helper()
		once.Do(func() {
			key, keyErr = loadRSAKeyFromTestdata("rsa_1024_private.pem")
		})
		check.NoError(t, keyErr, "Failed to load shared RSA 1024-bit weak test key from testdata")

		return key
	}
}()

// A revocation signature is valid without the optional "reason for revocation"
// subpacket; such a key must still count as revoked, matching openpgp's Revoked.
func TestIsGPGKeyRevoked_RevocationWithoutReasonSubpacket(t *testing.T) {
	t.Parallel()

	entity, err := openpgp.NewEntity("Revoked", "", "revoked@example.com", nil)
	check.NoError(t, err)

	revocation := &packet.Signature{
		Version:           entity.PrimaryKey.Version,
		SigType:           packet.SigTypeKeyRevocation,
		PubKeyAlgo:        entity.PrimaryKey.PubKeyAlgo,
		Hash:              crypto.SHA256,
		CreationTime:      time.Now(),
		IssuerKeyId:       &entity.PrimaryKey.KeyId,
		IssuerFingerprint: entity.PrimaryKey.Fingerprint,
	}
	check.NoError(t, revocation.RevokeKey(entity.PrimaryKey, entity.PrivateKey, nil))
	entity.Revocations = append(entity.Revocations, revocation)

	var serialized bytes.Buffer

	check.NoError(t, entity.Serialize(&serialized))

	parsed, err := openpgp.ReadKeyRing(&serialized)
	check.NoError(t, err)
	check.Len(t, parsed, 1)
	check.Nil(t, parsed[0].Revocations[0].RevocationReason, "the fixture must have no reason subpacket")

	now := time.Now()
	check.True(t, IsGPGKeyRevoked(parsed[0], now))
	check.False(t, ValidateGPGKey(parsed[0], DefaultGPGSecuritySettings(), now))
	check.True(t, gpgEntityToCryptoKeyInfo(parsed[0], now).Revoked)
}
