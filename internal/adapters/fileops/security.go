// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops

import (
	"path/filepath"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// IsValidGitHookType restricts hook names before they are used in paths.
func IsValidGitHookType(hookType string) bool {
	validHooks := map[string]bool{
		"commit-msg":            true,
		"pre-commit":            true,
		"pre-push":              true,
		"pre-receive":           true,
		"update":                true,
		"post-update":           true,
		"post-receive":          true,
		"post-merge":            true,
		"pre-rebase":            true,
		"pre-applypatch":        true,
		"post-applypatch":       true,
		"post-checkout":         true,
		"post-commit":           true,
		"pre-auto-gc":           true,
		"post-rewrite":          true,
		"sendemail-validate":    true,
		"fsmonitor-watchman":    true,
		"p4-pre-submit":         true,
		"p4-prepare-changelist": true,
		"p4-changelist":         true,
		"p4-post-changelist":    true,
		"prepare-commit-msg":    true,
	}

	return validHooks[hookType]
}

// IsWithinPath checks if a relative path stays within a base directory using string logic.
func IsWithinPath(relPath, basePath string) bool {
	if filepath.IsAbs(relPath) {
		return false
	}

	targetPath := filepath.Join(basePath, relPath)

	return domain.DefaultSecurityPolicy().ValidatePathWithinBase(basePath, targetPath) == nil
}
