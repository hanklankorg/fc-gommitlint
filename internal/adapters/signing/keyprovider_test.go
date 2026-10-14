// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"golang.org/x/crypto/ssh"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
)

func TestFileSystemKeyProvider_ReadKeyFile_PathValidation(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	provider := NewFileSystemKeyProviderWithFS(baseDir, filesystem.NewOSFileSystem())

	validPath := filepath.Join(baseDir, "signing.gpg")
	err := os.WriteFile(validPath, []byte("key-data"), 0o600)
	check.NoError(t, err)

	prefixCollisionDir := baseDir + "-old"
	err = os.MkdirAll(prefixCollisionDir, 0o700)
	check.NoError(t, err)

	prefixCollisionPath := filepath.Join(prefixCollisionDir, "escape.gpg")
	err = os.WriteFile(prefixCollisionPath, []byte("other"), 0o600)
	check.NoError(t, err)

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.gpg")
	err = os.WriteFile(outsidePath, []byte("outside"), 0o600)
	check.NoError(t, err)

	testCases := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "path inside directory",
			path:        validPath,
			expectError: false,
		},
		{
			name:        "prefix collision",
			path:        prefixCollisionPath,
			expectError: true,
		},
		{
			name:        "outside directory",
			path:        outsidePath,
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := provider.ReadKeyFile(testCase.path)
			if testCase.expectError {
				check.ErrorIs(t, err, domain.ErrKeyFileOutsideDirectory)
			} else {
				check.NoError(t, err)
			}
		})
	}
}

func TestFileSystemKeyProvider_ListKeyFiles_RespectsLimit(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	provider := NewFileSystemKeyProviderWithFSAndLimit(baseDir, filesystem.NewOSFileSystem(), 2)

	writeKeyFile := func(name string) {
		t.Helper()

		path := filepath.Join(baseDir, name)
		err := os.WriteFile(path, []byte("key"), 0o600)
		check.NoError(t, err)
	}

	writeKeyFile("a.gpg")
	writeKeyFile("b.gpg")

	files, err := provider.ListKeyFiles([]string{keyExtGPG})
	check.NoError(t, err)
	check.Len(t, files, 2)

	writeKeyFile("c.gpg")

	_, err = provider.ListKeyFiles([]string{keyExtGPG})
	check.ErrorIs(t, err, errTooManyKeyFiles)
}

type keyProviderLogger struct {
	warnings int
}

func (l *keyProviderLogger) Log(_ string, _ string, _ ...any) {}
func (l *keyProviderLogger) Trace(_ string, _ ...any)         {}
func (l *keyProviderLogger) TraceFunc(_ func() (string, []any)) {
}
func (l *keyProviderLogger) Debug(_ string, _ ...any) {}
func (l *keyProviderLogger) Info(_ string, _ ...any)  {}
func (l *keyProviderLogger) Warn(_ string, _ ...any)  { l.warnings++ }
func (l *keyProviderLogger) Error(_ string, _ ...any) {}

var _ domain.Logger = (*keyProviderLogger)(nil)

func TestFileSystemKeyProvider_ListKeyFiles_IgnoresPrivateKeys(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	logger := &keyProviderLogger{}
	provider := NewFileSystemKeyProviderWithFSAndLimit(baseDir, filesystem.NewOSFileSystem(), 10).
		WithLogger(logger)

	privatePath := filepath.Join(baseDir, "private.pem")
	err := os.WriteFile(privatePath, []byte("key"), 0o600)
	check.NoError(t, err)

	publicPath := filepath.Join(baseDir, "key.pub")
	err = os.WriteFile(publicPath, []byte("key"), 0o600)
	check.NoError(t, err)

	files, err := provider.ListKeyFiles([]string{".pem", ".pub"})
	check.NoError(t, err)
	check.Equal(t, []string{publicPath}, files)
	check.Equal(t, 1, logger.warnings)
}

func TestFileSystemKeyProvider_LoadKeys_WarnsOnUnparsableFiles(t *testing.T) {
	t.Parallel()

	const validSSHKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl valid@key\n"

	t.Run("SSH", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		logger := &keyProviderLogger{}
		provider := NewFileSystemKeyProviderWithFS(baseDir, filesystem.NewOSFileSystem()).WithLogger(logger)

		check.NoError(t, os.WriteFile(filepath.Join(baseDir, "broken.pub"), []byte("not an ssh key\n"), 0o600))
		check.NoError(t, os.WriteFile(filepath.Join(baseDir, "good.pub"), []byte(validSSHKey), 0o600))

		keys, err := provider.LoadSSHKeys()
		check.NoError(t, err)
		check.Len(t, keys, 1)
		check.Equal(t, 1, logger.warnings, "one warning per skipped key file")
	})

	t.Run("GPG", func(t *testing.T) {
		t.Parallel()

		baseDir := t.TempDir()
		logger := &keyProviderLogger{}
		provider := NewFileSystemKeyProviderWithFS(baseDir, filesystem.NewOSFileSystem()).WithLogger(logger)

		check.NoError(t, os.WriteFile(filepath.Join(baseDir, "broken.gpg"), []byte("not a gpg key\n"), 0o600))

		keys, err := provider.LoadGPGKeys()
		check.NoError(t, err)
		check.Empty(t, keys)
		check.Equal(t, 1, logger.warnings, "one warning per skipped key file")
	})
}

// TestCryptoKeyInfo_ReportsMeasuredStrengthAndIdentity covers the two
// conversions the key provider hands to the rules: the SSH bit length, which no
// test measured (a 1024-bit key would pass a 2048-bit policy if the length were
// assumed), and the user IDs, e-mail addresses and fingerprint extracted from a
// GPG and an SSH key.
//
// The keys are generated in-process; nothing reads an agent, a keyring or an
// ambient GNUPGHOME. The RSA sizes are deliberately small — the point is that
// the length is measured, not assumed, and generating larger keys only makes
// the test slower.
func TestCryptoKeyInfo_ReportsMeasuredStrengthAndIdentity(t *testing.T) {
	t.Parallel()

	t.Run("ssh-rsa bit length is measured, not assumed", func(t *testing.T) {
		t.Parallel()

		policy := rules.KeyStrengthPolicy{MinRSABits: 2048, MinECBits: 256}

		for _, bits := range []int{1024, 2048} {
			rsaKey, err := rsa.GenerateKey(rand.Reader, bits)
			check.NoError(t, err)

			pubKey, err := ssh.NewPublicKey(&rsaKey.PublicKey)
			check.NoError(t, err)

			info := sshKeyToCryptoKeyInfo(pubKey, "")

			check.Equal(t, rules.KeyAlgorithmRSA, info.Algorithm)
			check.Equal(t, bits, info.BitLength)
			check.Equal(t, bits >= 2048, policy.Allows(info), "a 2048-bit policy must refuse the weaker key and accept the stronger one")
		}
	})

	t.Run("gpg identity carries the user id, e-mail and fingerprint", func(t *testing.T) {
		t.Parallel()

		entity := newEd25519Entity(t)
		info := gpgEntityToCryptoKeyInfo(entity, time.Now())

		check.Equal(t, []string{"Test User <test@example.com>"}, info.Identity.UserIDs)
		check.Equal(t, []string{"test@example.com"}, info.Identity.Emails)
		check.Equal(t, fmt.Sprintf("%X", entity.PrimaryKey.Fingerprint), info.Identity.Fingerprint)
	})

	t.Run("ssh identity carries the comment and fingerprint", func(t *testing.T) {
		t.Parallel()

		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		check.NoError(t, err)

		pubKey, err := ssh.NewPublicKey(privateKey.Public())
		check.NoError(t, err)

		info := sshKeyToCryptoKeyInfo(pubKey, "user@example.com")

		check.Equal(t, []string{"user@example.com"}, info.Identity.UserIDs)
		check.Equal(t, []string{"user@example.com"}, info.Identity.Emails)
		check.Equal(t, ssh.FingerprintSHA256(pubKey), info.Identity.Fingerprint)

		// A comment that is not an address stays out of Emails.
		named := sshKeyToCryptoKeyInfo(pubKey, "work laptop")
		check.Equal(t, []string{"work laptop"}, named.Identity.UserIDs)
		check.Empty(t, named.Identity.Emails)
	})
}
