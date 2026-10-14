// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"crypto/rsa"
	"fmt"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"golang.org/x/crypto/ssh"

	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

// loadKeysFromProvider lists matching files in the provider and parses each via parse.
// Files that fail to read or parse are silently skipped — caller validates the aggregate.
func loadKeysFromProvider[T any](
	provider *FileSystemKeyProvider,
	extensions []string,
	parse func(data []byte, keyFile string) ([]T, error),
) ([]T, error) {
	keyFiles, err := provider.ListKeyFiles(extensions)
	if err != nil {
		return nil, err
	}

	items, _ := loadParsedKeyFiles(keyFiles, func(keyFile string) ([]T, error) {
		data, readErr := provider.ReadKeyFile(keyFile)
		if readErr != nil {
			return nil, readErr
		}

		return parse(data, keyFile)
	}, func(keyFile string, err error) {
		if provider.logger != nil {
			provider.logger.Warn("Skipping key file that could not be parsed", "file", keyFile, "error", err.Error())
		}
	})

	return items, nil
}

// LoadGPGKeys loads and analyzes all GPG keys from the key directory.
// Keys that fail to parse are skipped with a warning.
func (p *FileSystemKeyProvider) LoadGPGKeys() ([]rules.CryptoKeyInfo, error) {
	now := time.Now()

	return loadKeysFromProvider(p, gpgKeyExtensions(), func(data []byte, _ string) ([]rules.CryptoKeyInfo, error) {
		entities, parseErr := parseGPGKeyFile(data)
		if parseErr != nil {
			return nil, parseErr
		}

		keys := make([]rules.CryptoKeyInfo, 0, len(entities))

		for _, entity := range entities {
			if entity == nil {
				continue
			}

			keys = append(keys, gpgEntityToCryptoKeyInfo(entity, now))
		}

		return keys, nil
	})
}

// LoadSSHKeys loads and analyzes all SSH keys from the key directory.
// Keys that fail to parse are skipped with a warning.
func (p *FileSystemKeyProvider) LoadSSHKeys() ([]rules.CryptoKeyInfo, error) {
	return loadKeysFromProvider(p, sshKeyExtensions(), func(data []byte, keyFile string) ([]rules.CryptoKeyInfo, error) {
		parsed, parseErr := parseSSHPublicKeyFile(keyFile, data)
		if parseErr != nil {
			return nil, parseErr
		}

		return []rules.CryptoKeyInfo{sshKeyToCryptoKeyInfo(parsed.key, parsed.comment)}, nil
	})
}

// parseGPGKeyFile attempts to parse GPG key data in armored or binary format.
func parseGPGKeyFile(data []byte) (openpgp.EntityList, error) {
	// Try armored format first
	entities, err := openpgp.ReadArmoredKeyRing(strings.NewReader(string(data)))
	if err == nil {
		return entities, nil
	}

	// Fall back to binary format
	entities, err = openpgp.ReadKeyRing(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("parse GPG key: %w", err)
	}

	return entities, nil
}

// gpgEntityToCryptoKeyInfo converts an OpenPGP entity to domain CryptoKeyInfo.
func gpgEntityToCryptoKeyInfo(entity *openpgp.Entity, now time.Time) rules.CryptoKeyInfo {
	info := rules.CryptoKeyInfo{
		Revoked: IsGPGKeyRevoked(entity, now),
		Expired: IsGPGKeyExpired(entity, now),
	}

	if entity.PrimaryKey != nil {
		info.Identity.Fingerprint = fmt.Sprintf("%X", entity.PrimaryKey.Fingerprint)
		info.Algorithm, info.BitLength = extractGPGAlgorithmInfo(entity)
	}

	for name := range entity.Identities {
		info.Identity.UserIDs = append(info.Identity.UserIDs, name)

		if start := strings.LastIndex(name, "<"); start != -1 {
			if end := strings.LastIndex(name, ">"); end > start+1 {
				info.Identity.Emails = append(info.Identity.Emails, name[start+1:end])
			}
		}
	}

	return info
}

// extractGPGAlgorithmInfo determines the algorithm category and bit length of a GPG key.
//
// BitLength is the size of the MPI the key is encoded as, which is the key size
// only for RSA. For an elliptic curve it measures the encoded public point --
// 515 bits for P-256 and 771 for P-384 -- so comparing it against a minimum
// expressed in curve bits compares two different quantities, and compares them
// in the permissive direction: a P-256 key reads as 515 and satisfies a policy
// asking for 384. Curve size comes from the curve itself.
func extractGPGAlgorithmInfo(entity *openpgp.Entity) (string, int) {
	if entity == nil || entity.PrimaryKey == nil {
		return "", 0
	}

	algo := entity.PrimaryKey.PubKeyAlgo
	bitLength, _ := entity.PrimaryKey.BitLength()

	switch {
	case isRSAKey(algo):
		return rules.KeyAlgorithmRSA, int(bitLength)
	case algo == packet.PubKeyAlgoEdDSA:
		return rules.KeyAlgorithmEd25519, 256
	case isECKey(algo):
		return rules.KeyAlgorithmECDSA, ecCurveBits(entity.PrimaryKey, int(bitLength))
	default:
		return "", int(bitLength)
	}
}

// ecCurveBits reports the curve size of an elliptic-curve public key.
//
// fallback is returned for a key whose curve cannot be named -- an unknown OID,
// or a key the OpenPGP parser did not produce. Keeping the encoded length there
// preserves the previous reading for a shape no real signature can arrive in,
// rather than reporting 0 and failing a key whose strength was never
// established.
func ecCurveBits(key *packet.PublicKey, fallback int) int {
	curve, err := key.Curve()
	if err != nil {
		return fallback
	}

	// Named rather than measured: the curve is an identifier here, not a
	// parameter set, and the format defines exactly these.
	switch curve {
	case packet.Curve25519, packet.CurveNistP256, packet.CurveSecP256k1, packet.CurveBrainpoolP256:
		return 256
	case packet.CurveNistP384, packet.CurveBrainpoolP384:
		return 384
	case packet.Curve448:
		return 448
	case packet.CurveBrainpoolP512:
		return 512
	case packet.CurveNistP521:
		return 521
	default:
		return fallback
	}
}

// sshKeyToCryptoKeyInfo converts an SSH public key to domain CryptoKeyInfo.
func sshKeyToCryptoKeyInfo(pubKey ssh.PublicKey, comment string) rules.CryptoKeyInfo {
	algorithm, bitLength := extractSSHAlgorithmInfo(pubKey)

	info := rules.CryptoKeyInfo{
		Algorithm: algorithm,
		BitLength: bitLength,
		Revoked:   false, // SSH keys don't have built-in revocation
		Expired:   false, // SSH keys don't expire
	}

	info.Identity.Fingerprint = ssh.FingerprintSHA256(pubKey)

	if comment != "" {
		info.Identity.UserIDs = append(info.Identity.UserIDs, comment)

		if strings.Contains(comment, "@") && !strings.Contains(comment, " ") {
			info.Identity.Emails = append(info.Identity.Emails, comment)
		}
	}

	return info
}

// extractSSHAlgorithmInfo determines the algorithm category and bit length of an SSH key.
func extractSSHAlgorithmInfo(pubKey ssh.PublicKey) (string, int) {
	switch pubKey.Type() {
	case sshKeyTypeRSA:
		cryptoKey, ok := pubKey.(ssh.CryptoPublicKey)
		if !ok {
			return rules.KeyAlgorithmRSA, 0
		}

		rsaKey, ok := cryptoKey.CryptoPublicKey().(*rsa.PublicKey)
		if !ok {
			return rules.KeyAlgorithmRSA, 0
		}

		return rules.KeyAlgorithmRSA, rsaKey.N.BitLen()
	case sshKeyTypeEd25519:
		return rules.KeyAlgorithmEd25519, 256
	case sshKeyTypeECDSAP256:
		return rules.KeyAlgorithmECDSA, 256
	case sshKeyTypeECDSAP384:
		return rules.KeyAlgorithmECDSA, 384
	case sshKeyTypeECDSAP521:
		return rules.KeyAlgorithmECDSA, 521
	default:
		return "", 0
	}
}
