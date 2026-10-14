// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"encoding/base64"
	"fmt"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func TestLoadSSHKey_NonexistentFileFails(t *testing.T) {
	t.Parallel()

	t.Run("handles nonexistent file", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		loader := LoadSSHKey(fileOps)

		keyName, pubKey, err := loader("/nonexistent/key.pub")
		check.Error(t, err)
		check.Empty(t, keyName)
		check.Nil(t, pubKey)
	})
}

func TestLoadSSHKeysFromDirectory_FailsClosedOnInvalidInputs(t *testing.T) {
	t.Parallel()

	t.Run("handles missing directory", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		_, err := LoadSSHKeysFromDirectory(fileOps, "/nonexistent")
		check.Error(t, err)
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		t.Parallel()

		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())
		_, err := LoadSSHKeysFromDirectory(fileOps, "../invalid")
		check.Error(t, err)
	})

	t.Run("handles empty directory", func(t *testing.T) {
		t.Parallel()

		emptyDir := t.TempDir()
		fileOps := fileops.NewFileOperations(filesystem.NewOSFileSystem())

		keys, err := LoadSSHKeysFromDirectory(fileOps, emptyDir)
		check.Error(t, err)
		check.ErrorIs(t, err, ErrNoSSHKeysFound)
		check.Nil(t, keys)
	})

	t.Run("rejects too many key files", func(t *testing.T) {
		t.Parallel()

		keyDir := t.TempDir()
		fs := filesystem.NewOSFileSystem()
		fileOps := fileops.NewFileOperations(fs)

		// Create more than MaxKeyFiles
		for i := 0; i <= settings.DefaultMaxKeyFiles; i++ {
			keyFile := fmt.Sprintf("%s/key%d.pub", keyDir, i)
			err := fs.WriteFile(keyFile, []byte("dummy"), 0644)
			check.NoError(t, err)
		}

		_, err := LoadSSHKeysFromDirectory(fileOps, keyDir)
		check.Error(t, err)
		check.ErrorIs(t, err, ErrTooManySSHKeyFiles)
	})

	t.Run("loads valid SSH keys", func(t *testing.T) {
		t.Parallel()

		// Create temp directory with SSH key files
		keyDir := t.TempDir()
		fs := filesystem.NewOSFileSystem()
		fileOps := fileops.NewFileOperations(fs)

		// Valid Ed25519 SSH public key (example)
		validKey := testSSHKey
		keyFile1 := keyDir + "/test1.pub"
		err := fs.WriteFile(keyFile1, []byte(validKey), 0644)
		check.NoError(t, err)

		// Another valid key without comment
		validKey2 := testSSHKeyWithoutComment
		keyFile2 := keyDir + "/test2.pub"
		err = fs.WriteFile(keyFile2, []byte(validKey2), 0644)
		check.NoError(t, err)

		keys, err := LoadSSHKeysFromDirectory(fileOps, keyDir)
		check.NoError(t, err)
		check.Len(t, keys, 2)

		// Name each key rather than checking the fields are merely populated:
		// the commented key takes its name from the comment, the uncommented
		// one from its filename, so this also pins that both files were read
		// and matched to the right key.
		names := []string{keys[0].Name, keys[1].Name}
		check.ElementsMatch(t, []string{signingTestUserEmail, "test2.pub"}, names)

		sources := []string{keys[0].Source, keys[1].Source}
		check.ElementsMatch(t, []string{keyFile1, keyFile2}, sources)

		check.NotNil(t, keys[0].Key)
		check.NotNil(t, keys[1].Key)
	})
}

func TestLoadSSHKey_ExtractsNameFromCommentOrFilename(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		keyContent   string
		expectedName string
	}{
		{
			name:         "Ed25519 key with comment",
			keyContent:   testSSHKey,
			expectedName: signingTestUserEmail,
		},
		{
			name:         "Ed25519 key without comment",
			keyContent:   testSSHKeyWithoutComment,
			expectedName: "test.pub", // Should use filename as fallback
		},
		{
			name:         "Ed25519 key with whitespace",
			keyContent:   testSSHKeyWithoutCommentWS,
			expectedName: "user@host",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create temp file with SSH key
			keyFile := t.TempDir() + "/test.pub"
			fs := filesystem.NewOSFileSystem()
			fileOps := fileops.NewFileOperations(fs)
			err := fs.WriteFile(keyFile, []byte(testCase.keyContent), 0644)
			check.NoError(t, err)

			loader := LoadSSHKey(fileOps)
			keyName, pubKey, err := loader(keyFile)

			check.NoError(t, err)
			check.Equal(t, testCase.expectedName, keyName)
			check.NotNil(t, pubKey)
		})
	}
}

func TestLoadSSHKey_RejectsMalformedContent(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		keyContent  string
		base64Err   bool
		expectedErr error
	}{
		{
			name:        "invalid format - single field",
			keyContent:  sshKeyTypeEd25519,
			expectedErr: ErrInvalidSSHKeyFormat,
		},
		{
			name:       "invalid base64",
			keyContent: sshKeyTypeEd25519 + " invalid-base64!@# comment",
			base64Err:  true,
		},
		{
			name:        "empty content",
			keyContent:  "",
			expectedErr: ErrInvalidSSHKeyFormat,
		},
		{
			name:        "whitespace only",
			keyContent:  "   \n  ",
			expectedErr: ErrInvalidSSHKeyFormat,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create temp file with invalid key content
			keyFile := t.TempDir() + "/invalid.pub"
			fs := filesystem.NewOSFileSystem()
			fileOps := fileops.NewFileOperations(fs)
			err := fs.WriteFile(keyFile, []byte(testCase.keyContent), 0644)
			check.NoError(t, err)

			loader := LoadSSHKey(fileOps)
			keyName, pubKey, err := loader(keyFile)

			check.Error(t, err)

			if testCase.expectedErr != nil {
				check.ErrorIs(t, err, testCase.expectedErr)
			}

			if testCase.base64Err {
				var corrupt base64.CorruptInputError
				check.ErrorAs(t, err, &corrupt)
			}

			check.Empty(t, keyName)
			check.Nil(t, pubKey)
		})
	}
}
