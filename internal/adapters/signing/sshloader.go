// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/crypto/ssh"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
)

// SSH key loading errors.
var (
	ErrInvalidSSHKeyFormat = errors.New("invalid SSH key format")
	ErrNoSSHKeysFound      = errors.New("no SSH key files found")
	ErrTooManySSHKeyFiles  = errors.New("too many SSH key files")
)

// SSHKeyLoader defines functions for loading SSH keys.
type SSHKeyLoader func(path string) (string, ssh.PublicKey, error)

// SSHKeyData a loaded SSH key with its metadata.
type SSHKeyData struct {
	Name   string
	Key    ssh.PublicKey
	Source string
}

type parsedSSHPublicKey struct {
	name    string
	key     ssh.PublicKey
	comment string
}

// LoadSSHKey loads SSH keys from a file using the injected filesystem.
func LoadSSHKey(fileOps fileops.FileOperations) SSHKeyLoader {
	return func(path string) (string, ssh.PublicKey, error) {
		data, err := fileOps.ReadFile(path)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read SSH key file: %w", err)
		}

		parsed, err := parseSSHPublicKeyFile(path, data)
		if err != nil {
			return "", nil, err
		}

		return parsed.name, parsed.key, nil
	}
}

// LoadSSHKeysFromDirectory finds and loads all SSH keys from a directory.
func LoadSSHKeysFromDirectory(fileOps fileops.FileOperations, keyDir string) ([]SSHKeyData, error) {
	provider := newFileSystemKeyProvider(fileOps, keyDir)

	keyFiles, err := requireKeyFiles(provider, sshKeyExtensions(), ErrNoSSHKeysFound, ErrTooManySSHKeyFiles)
	if err != nil {
		return nil, err
	}

	allKeys, _ := loadParsedKeyFiles(keyFiles, func(keyFile string) ([]SSHKeyData, error) {
		data, readErr := provider.ReadKeyFile(keyFile)
		if readErr != nil {
			return nil, readErr
		}

		parsed, parseErr := parseSSHPublicKeyFile(keyFile, data)
		if parseErr != nil {
			return nil, parseErr
		}

		return []SSHKeyData{{
			Name:   parsed.name,
			Key:    parsed.key,
			Source: keyFile,
		}}, nil
	}, nil)

	return allKeys, nil
}

func parseSSHPublicKeyFile(path string, data []byte) (parsedSSHPublicKey, error) {
	trimmed := bytes.TrimSpace(data)
	if len(bytes.Fields(trimmed)) < 2 {
		return parsedSSHPublicKey{}, ErrInvalidSSHKeyFormat
	}

	pubKey, comment, _, _, err := ssh.ParseAuthorizedKey(trimmed)
	if err != nil {
		return parsedSSHPublicKey{}, err
	}

	name := comment
	if name == "" {
		name = filepath.Base(path)
	}

	return parsedSSHPublicKey{
		name:    name,
		key:     pubKey,
		comment: comment,
	}, nil
}
