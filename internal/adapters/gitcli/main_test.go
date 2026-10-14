// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"os"
	"testing"
)

// TestMain cuts these tests off from the developer's git configuration.
//
// The tests in this package drive the real git binary, and runGitCommand keeps
// the configuration variables in its environment -- correctly, since the
// shipped tool must honour a user's config (Environment drops only the
// variables that relocate the repository). That leaves the tests reading
// whatever is in ~/.gitconfig and /etc/gitconfig. The failure that motivated
// this was a global core.hooksPath:
// gommitlint can install its own hooks globally, and when it has, every
// git commit these tests make runs that hook and fails for reasons unrelated
// to the code under test.
//
// Neutralising all config sources covers parallel tests and settings that no
// individual fixture overrides.

// The identity these tests commit under. Named because it is set in two
// places that must agree: the environment below, and the per-repository
// config a signing test writes -- a signature is verified against the
// identity that made it, so a drift between the two fails as a signature
// mismatch rather than as the config mistake it is.
const (
	testUserName  = "Test User"
	testUserEmail = "test@example.com"
)

func TestMain(m *testing.M) {
	// A temporary HOME keeps git from finding the user's config through either
	// ~/.gitconfig or $HOME/.config/git/config.
	home, err := os.MkdirTemp("", "gommitlint-gitcli-home")
	if err != nil {
		panic("create temp HOME: " + err.Error())
	}

	// Cleanup is explicit rather than deferred: os.Exit below skips defers.
	for key, value := range map[string]string{
		"HOME":                home,
		"XDG_CONFIG_HOME":     home,
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_NOSYSTEM": "1",

		// Identity has to come from somewhere once the user's config is gone.
		// Supplying it here lets the per-repository user.name/user.email calls
		// go away with the rest.
		"GIT_AUTHOR_NAME":     testUserName,
		"GIT_AUTHOR_EMAIL":    testUserEmail,
		"GIT_COMMITTER_NAME":  testUserName,
		"GIT_COMMITTER_EMAIL": testUserEmail,
	} {
		if err := os.Setenv(key, value); err != nil {
			panic("set " + key + ": " + err.Error())
		}
	}

	code := m.Run()

	_ = os.RemoveAll(home)

	os.Exit(code)
}
