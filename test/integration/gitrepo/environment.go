//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitrepo

import (
	"maps"
	"path/filepath"
	"sync"
)

// IsolatedEnvironment provides a thread-safe isolated environment for parallel tests.
type IsolatedEnvironment struct {
	mu   sync.RWMutex
	vars map[string]string
}

// NewIsolatedEnvironment creates a new isolated environment with test defaults.
// The tempDir should be created with t.TempDir() for automatic cleanup.
func NewIsolatedEnvironment(tempDir string) *IsolatedEnvironment {
	isolatedHome := filepath.Join(tempDir, "home")

	return &IsolatedEnvironment{
		vars: map[string]string{
			// Core environment
			"HOME":        isolatedHome,
			"USERPROFILE": isolatedHome, // Windows

			// XDG Base Directory
			"XDG_CONFIG_HOME": filepath.Join(isolatedHome, ".config"),
			"XDG_DATA_HOME":   filepath.Join(isolatedHome, ".local", "share"),
			"XDG_CACHE_HOME":  filepath.Join(isolatedHome, ".cache"),
			"XDG_STATE_HOME":  filepath.Join(isolatedHome, ".local", "state"),

			// Git configuration
			"GIT_CONFIG_NOSYSTEM": "1",
			"GIT_CONFIG_GLOBAL":   filepath.Join(tempDir, "gitconfig"),
			"GIT_AUTHOR_NAME":     "Test Author",
			"GIT_AUTHOR_EMAIL":    "test@example.com",
			"GIT_COMMITTER_NAME":  "Test Committer",
			"GIT_COMMITTER_EMAIL": "test@example.com",

			// Disable interactive prompts and external access
			"GIT_TERMINAL_PROMPT":   "0",
			"GIT_ASKPASS":           "",
			"SSH_ASKPASS":           "",
			"GIT_SSH_COMMAND":       "false",
			"GIT_CREDENTIAL_HELPER": "",
			"GIT_HOOKS_PATH":        "/dev/null",
			"GIT_EDITOR":            "false",
			"GIT_SEQUENCE_EDITOR":   "false",
			"GIT_PAGER":             "cat",

			// GPG isolation
			"GNUPGHOME": filepath.Join(isolatedHome, ".gnupg"),
			"GPG_TTY":   "",

			// SSH isolation
			"SSH_AUTH_SOCK": "",
			"SSH_AGENT_PID": "",

			// Output control
			"NO_COLOR": "1",
			"TERM":     "dumb",

			// Locale (force deterministic output)
			"LC_ALL":      "C",
			"LC_MESSAGES": "C",
			"LANG":        "C",

			// Gommitlint
			"GOMMITLINT_CONFIG":       "",
			"GOMMITLINT_NO_CI_DETECT": "1",

			// Temporary files
			"TMPDIR": tempDir,
			"TMP":    tempDir,
			"TEMP":   tempDir,
		},
	}
}

// ToMap returns a copy of all environment variables as a map.
// Useful for passing to external commands or libraries.
func (e *IsolatedEnvironment) ToMap() map[string]string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]string, len(e.vars))
	maps.Copy(result, e.vars)
	return result
}
