// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Security validation errors.
var (
	ErrPathEmpty         = errors.New("path cannot be empty")
	ErrPathTraversal     = errors.New("path contains directory traversal")
	ErrPathDangerous     = errors.New("path contains dangerous characters")
	ErrPathOutsideBase   = errors.New("path is outside base directory")
	ErrGitRefEmpty       = errors.New("git reference cannot be empty")
	ErrGitRefDangerous   = errors.New("git reference contains dangerous characters")
	ErrCommitRangeEmpty  = errors.New("commit range cannot be empty")
	ErrCommitRangeFormat = errors.New("invalid commit range format")
	ErrCommitRangeParts  = errors.New("commit range must have exactly two parts")
	ErrParameterTooLong  = errors.New("parameter exceeds maximum length")
)

// SecurityPolicy defines security validation rules.
type SecurityPolicy struct{}

// DefaultSecurityPolicy returns the default security policy.
func DefaultSecurityPolicy() SecurityPolicy {
	return SecurityPolicy{}
}

// ValidatePath performs essential path validation.
func (p SecurityPolicy) ValidatePath(path string) error {
	if path == "" {
		return ErrPathEmpty
	}

	if len(path) > MaxPathLength {
		return ErrParameterTooLong
	}

	if HasPathTraversal(path) {
		return ErrPathTraversal
	}

	if strings.Contains(path, "\x00") {
		return ErrPathDangerous
	}

	return nil
}

// ValidatePathWithinBase ensures targetPath is contained by baseDir after path normalization.
func (p SecurityPolicy) ValidatePathWithinBase(baseDir string, targetPath string) error {
	if baseDir == "" || targetPath == "" {
		return ErrPathEmpty
	}

	absBase, err := filepath.Abs(filepath.Clean(baseDir))
	if err != nil {
		return fmt.Errorf("resolve base path: %w", err)
	}

	absTarget, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		return fmt.Errorf("resolve target path: %w", err)
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return fmt.Errorf("resolve relative path: %w", err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrPathOutsideBase
	}

	return nil
}

// ValidateGitReference applies the shared input-safety subset used by both Git
// adapters. Git remains responsible for complete revision-format validation.
func (p SecurityPolicy) ValidateGitReference(ref string) error {
	if ref == "" {
		return ErrGitRefEmpty
	}

	if len(ref) > MaxRefLength {
		return ErrParameterTooLong
	}

	// These metacharacters are not interpreted by exec.Command, but rejecting
	// them keeps references inert across adapters, diagnostics, and logs.
	if strings.ContainsAny(ref, "$`;&|\n\r\x00") || strings.Contains(ref, "$(") {
		return ErrGitRefDangerous
	}

	return nil
}

// ValidateCommitRange checks commit range security.
func (p SecurityPolicy) ValidateCommitRange(commitRange string) error {
	_, err := p.ParseSecureCommitRange(commitRange)

	return err
}

// ParseSecureCommitRange parses a supported commit range and validates both refs.
func (p SecurityPolicy) ParseSecureCommitRange(commitRange string) (CommitRange, error) {
	parsed, err := ParseCommitRange(commitRange)
	if err != nil {
		return CommitRange{}, err
	}

	for _, ref := range []string{parsed.From, parsed.To} {
		if err := p.ValidateGitReference(ref); err != nil {
			return CommitRange{}, fmt.Errorf("invalid git reference in range '%s': %w", ref, err)
		}
	}

	return parsed, nil
}

// HasPathTraversal conservatively reports paths that start with two dots after
// cleaning or contain raw parent-directory separators. It may reject paths
// that would remain contained after cleaning.
func HasPathTraversal(input string) bool {
	// Catch relative paths that still escape after cleaning, plus raw parent
	// segments so callers do not silently accept and normalise them.
	cleaned := filepath.Clean(input)

	return strings.HasPrefix(cleaned, "..") ||
		strings.Contains(input, "../") || // Unix traversal
		strings.Contains(input, "..\\") // Windows traversal
}

// RequiresSecurePermissions reports whether path resembles a private-key path
// according to the filename and location heuristics in IsPrivateKeyPath.
func RequiresSecurePermissions(path string) bool {
	return IsPrivateKeyPath(path)
}

// IsPrivateKeyPath returns true when a path appears to reference a private key.
func IsPrivateKeyPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	lowerPath := strings.ToLower(path)

	return isKnownSSHPrivateKeyBase(base) ||
		isLikelySSHPrivateKeyBase(base) ||
		hasPrivateKeyExtension(base) ||
		isPrivateKeyLocation(lowerPath, base)
}

func isKnownSSHPrivateKeyBase(base string) bool {
	switch base {
	case "id_rsa", "id_ed25519", "id_ecdsa", "id_dsa":
		return true
	default:
		return false
	}
}

func isLikelySSHPrivateKeyBase(base string) bool {
	return strings.HasPrefix(base, "id_") && !strings.HasSuffix(base, ".pub")
}

func hasPrivateKeyExtension(base string) bool {
	return strings.HasSuffix(base, ".pem") ||
		(strings.HasSuffix(base, ".key") && strings.Contains(base, "private"))
}

func isPrivateKeyLocation(path, base string) bool {
	return strings.Contains(path, "/.ssh/") && !strings.HasSuffix(base, ".pub")
}

// IsSecureFileMode checks if file permissions are secure enough.
func IsSecureFileMode(mode fs.FileMode) bool {
	// Ensure not world-writable or group-writable (0o022 mask)
	return mode&0o022 == 0
}
