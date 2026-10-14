// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
)

const (
	signingTestCommitHash       = "abc123"
	signingTestCommitMessage    = "feat: add feature"
	signingTestEmptySignature   = "empty signature"
	signingTestGPGExtension     = ".gpg"
	signingTestGPGKeyFile       = "key1.gpg"
	signingTestKeyDir           = "/test/keys"
	signingTestKeyName          = "test-key"
	signingTestKeySource        = "test-source"
	signingTestMockUserPath     = "/test/mock/user"
	signingTestPath             = "/path/to/key"
	signingTestPGPSignature     = "-----BEGIN PGP SIGNATURE-----\ntest\n-----END PGP SIGNATURE-----"
	signingTestPubExtension     = ".pub"
	signingTestPubKeyFile       = "key2.pub"
	signingTestUser             = "Test User <test@example.com>"
	signingTestUserExampleEmail = "user@example.com"
	signingTestUserEmail        = "test@example.com"
	signingTestValue            = "test"
	testSSHKey                  = testSSHKeyWithoutComment + " " + signingTestUserEmail
	testSSHKeyWithoutComment    = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl"
	testSSHKeyWithoutCommentWS  = "  " + testSSHKeyWithoutComment + " user@host  "
)

// The real-filesystem loader tests live in sshloader_test.go; this file keeps
// the shared signing test constants and the one scenario those tests lack:
// a directory mixing valid keys, an unparseable key (skipped, not fatal), and
// a non-.pub file (ignored).
func TestLoadSSHKeysFromDirectoryWithMixedContent_MemFS(t *testing.T) {
	t.Parallel()

	fs := afero.NewMemMapFs()
	fileOps := fileops.NewFileOperations(&AferoAdapter{fs: fs})

	keyDir := "/mixed-keys"
	err := fs.MkdirAll(keyDir, 0755)
	check.NoError(t, err)

	// Valid SSH key
	validKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl valid@key"
	err = afero.WriteFile(fs, keyDir+"/valid.pub", []byte(validKey), 0644)
	check.NoError(t, err)

	// Invalid SSH key (will be skipped)
	err = afero.WriteFile(fs, keyDir+"/invalid.pub", []byte("not-a-valid-key"), 0644)
	check.NoError(t, err)

	// Non-.pub file (should be ignored)
	err = afero.WriteFile(fs, keyDir+"/private.key", []byte("private key content"), 0600)
	check.NoError(t, err)

	// Another valid key
	validKey2 := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl another@key"
	err = afero.WriteFile(fs, keyDir+"/another.pub", []byte(validKey2), 0644)
	check.NoError(t, err)

	keys, err := LoadSSHKeysFromDirectory(fileOps, keyDir)
	check.NoError(t, err)
	check.Len(t, keys, 2) // Only the valid .pub files

	// Verify we got the valid keys
	keyNames := []string{keys[0].Name, keys[1].Name}
	check.Contains(t, keyNames, "valid@key")
	check.Contains(t, keyNames, "another@key")
}
