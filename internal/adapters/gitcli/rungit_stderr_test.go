// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"os"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestRunGitCommand_KeepsStderrOutOfTheParsedOutput controls the use of Output
// rather than CombinedOutput.
//
// git can warn on stderr and still exit 0 -- an ambiguous refname, a broken
// ref it chose to ignore, an advice hint. What this function returns is parsed
// as git data: log records split on 0x1e, and the raw commit whose bytes
// become SignaturePayload. A warning spliced in front corrupts the first
// record, and makes a signature check run against bytes git never produced.
//
// Driving real git into writing to stderr while succeeding is version- and
// config-dependent, so the guarantee is pinned against a stub that always does.
func TestRunGitCommand_KeepsStderrOutOfTheParsedOutput(t *testing.T) {
	const hash = "0123456789abcdef0123456789abcdef01234567"

	env := testhelpers.NewRealFSTestEnv(t)

	env.FS.WriteExecutable("git", []byte("#!/bin/sh\n"+
		"echo 'warning: refname \"dup\" is ambiguous.' >&2\n"+
		"printf '"+hash+"\\037\\037Test Bot\\037bot@example.invalid\\0372024-01-01T00:00:00Z\\037feat: parsed stdout\\036\\n'\n"+
		"exit 0\n"))

	t.Setenv("PATH", env.FS.Path("")+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := runGitCommand(t.Context(), env.RepoPath, "show", "HEAD")
	check.NoError(t, err, "the stub exits 0, so this must succeed")

	commits, err := parseGitLogOutput(out)
	check.NoError(t, err, "valid stdout must remain parseable when git warns")
	check.Len(t, commits, 1)
	check.Equal(t, hash, commits[0].Hash)
	check.Equal(t, "feat: parsed stdout", commits[0].Subject)
	check.Equal(t, "Test Bot", commits[0].Author)
	check.Equal(t, "bot@example.invalid", commits[0].AuthorEmail)
}

func TestRunPrePushGit_KeepsStderrOutOfTheParsedOutput(t *testing.T) {
	const oid = "abcdef0123456789abcdef0123456789abcdef01"

	env := testhelpers.NewRealFSTestEnv(t)

	env.FS.WriteExecutable("git", []byte("#!/bin/sh\n"+
		"echo 'warning: refname \"dup\" is ambiguous.' >&2\n"+
		"printf '"+oid+"\\n'\n"+
		"exit 0\n"))

	t.Setenv("PATH", env.FS.Path("")+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, status, err := runPrePushGit(t.Context(), env.RepoPath, prePushRevParseCommand, "HEAD")
	check.NoError(t, err, "the stub exits 0, so this must succeed")
	check.Equal(t, 0, status)

	parsedOID, err := parseSinglePrePushOID(out, len(oid))
	check.NoError(t, err, "valid stdout must remain parseable when git warns")
	check.Equal(t, oid, parsedOID)
}
