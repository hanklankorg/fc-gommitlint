// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package signing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Static errors for key provider operations.
var (
	errTooManyKeyFiles = errors.New("too many key files")
)

const (
	keyExtASC = ".asc"
	keyExtKey = ".key"
	keyExtGPG = ".gpg"
	keyExtPGP = ".pgp"
	keyExtPub = ".pub"
	keyExtSSH = ".ssh"
)

// FileSystemKeyProvider implements key-provider capabilities using the filesystem.
type FileSystemKeyProvider struct {
	directory   string
	fs          KeyProviderFileSystem
	maxKeyFiles int
	logger      domain.Logger
}

// KeyProviderFileSystem provides the file operations needed to load key files.
type KeyProviderFileSystem interface {
	ReadDir(path string) ([]os.DirEntry, error)
	ReadFile(path string) ([]byte, error)
	Stat(path string) (os.FileInfo, error)
}

// NewFileSystemKeyProviderWithLimit creates a new filesystem-based key provider with a custom limit.
func NewFileSystemKeyProviderWithLimit(directory string, maxKeyFiles int) *FileSystemKeyProvider {
	return NewFileSystemKeyProviderWithFSAndLimit(directory, filesystem.NewOSFileSystem(), maxKeyFiles)
}

// NewFileSystemKeyProviderWithFS creates a new filesystem-based key provider with custom filesystem.
func NewFileSystemKeyProviderWithFS(directory string, fs KeyProviderFileSystem) *FileSystemKeyProvider {
	return NewFileSystemKeyProviderWithFSAndLimit(directory, fs, settings.DefaultMaxKeyFiles)
}

// NewFileSystemKeyProviderWithFSAndLimit creates a new filesystem-based key provider with custom filesystem and limit.
func NewFileSystemKeyProviderWithFSAndLimit(directory string, fs KeyProviderFileSystem, maxKeyFiles int) *FileSystemKeyProvider {
	if maxKeyFiles <= 0 {
		maxKeyFiles = settings.DefaultMaxKeyFiles
	}

	return &FileSystemKeyProvider{
		directory:   directory,
		fs:          fs,
		maxKeyFiles: maxKeyFiles,
	}
}

// WithLogger sets a logger for warning messages.
func (p *FileSystemKeyProvider) WithLogger(logger domain.Logger) *FileSystemKeyProvider {
	p.logger = logger

	return p
}

var _ rules.KeyDirectoryValidator = (*FileSystemKeyProvider)(nil)
var _ rules.GPGKeyLoader = (*FileSystemKeyProvider)(nil)
var _ rules.SSHKeyLoader = (*FileSystemKeyProvider)(nil)

// ValidateDirectory checks if the key directory exists and is accessible.
func (p *FileSystemKeyProvider) ValidateDirectory() error {
	if p.directory == "" {
		return domain.ErrKeyDirectoryNotSpecified
	}

	fileInfo, err := p.fs.Stat(p.directory)
	if err != nil {
		return fmt.Errorf("%w: %w", domain.ErrKeyDirectoryNotAccessible, err)
	}

	if !fileInfo.IsDir() {
		return fmt.Errorf("%w: %s", domain.ErrKeyDirectoryNotDirectory, p.directory)
	}

	return nil
}

// ListKeyFiles returns paths to available key files with specified extensions.
func (p *FileSystemKeyProvider) ListKeyFiles(extensions []string) ([]string, error) {
	if err := p.ValidateDirectory(); err != nil {
		return nil, err
	}

	entries, err := p.fs.ReadDir(p.directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read key directory: %w", err)
	}

	// Use provided extensions or default to common key file extensions
	if len(extensions) == 0 {
		extensions = []string{keyExtGPG, keyExtASC, keyExtPub, keyExtPGP, keyExtKey}
	}

	return p.collectKeyFiles(entries, extensions)
}

// ReadKeyFile reads the contents of a key file.
func (p *FileSystemKeyProvider) ReadKeyFile(path string) ([]byte, error) {
	cleanPath := filepath.Clean(path)
	if err := domain.DefaultSecurityPolicy().ValidatePathWithinBase(p.directory, cleanPath); err != nil {
		return nil, domain.ErrKeyFileOutsideDirectory
	}

	// #nosec G304 - path is validated to be within key directory
	data, err := p.fs.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	return data, nil
}

func (p *FileSystemKeyProvider) collectKeyFiles(entries []os.DirEntry, extensions []string) ([]string, error) {
	keyFiles := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !hasMatchingExtension(fileName, extensions) {
			continue
		}

		fullPath := filepath.Join(p.directory, fileName)
		if p.shouldIgnoreKeyFile(fullPath) {
			continue
		}

		keyFiles = append(keyFiles, fullPath)
		if len(keyFiles) > p.maxKeyFiles {
			return nil, fmt.Errorf("%w: found %d, maximum %d", errTooManyKeyFiles, len(keyFiles), p.maxKeyFiles)
		}
	}

	return keyFiles, nil
}

func hasMatchingExtension(fileName string, extensions []string) bool {
	for _, ext := range extensions {
		if strings.HasSuffix(fileName, ext) {
			return true
		}
	}

	return false
}

func (p *FileSystemKeyProvider) shouldIgnoreKeyFile(path string) bool {
	if !domain.IsPrivateKeyPath(path) {
		return false
	}

	if p.logger != nil {
		p.logger.Warn("Ignoring file whose path resembles a private key", "file", path)
	}

	return true
}
