// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const gitConfigCommand = "config"

func TestExtractCommitSignatureData_SHA256SSHSignature(t *testing.T) {
	rawCommit := []byte(strings.Join([]string{
		"tree 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"parent abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		"author Test User <test@example.com> 1700000000 +0000",
		"committer Test User <test@example.com> 1700000000 +0000",
		"gpgsig-sha256 -----BEGIN SSH SIGNATURE-----",
		" AAAAB3NzaC1zaWcAAAAHc3NoLXNhAAAAAwEAAQ==",
		" -----END SSH SIGNATURE-----",
		"",
		"feat: signed commit",
		"",
		"Body text.",
		"",
	}, "\n"))

	signature, payload := extractCommitSignatureData(rawCommit)

	check.Equal(t, strings.Join([]string{
		"-----BEGIN SSH SIGNATURE-----",
		"AAAAB3NzaC1zaWcAAAAHc3NoLXNhAAAAAwEAAQ==",
		"-----END SSH SIGNATURE-----",
	}, "\n"), signature)
	check.Equal(t, strings.Join([]string{
		"tree 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"parent abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		"author Test User <test@example.com> 1700000000 +0000",
		"committer Test User <test@example.com> 1700000000 +0000",
		"",
		"feat: signed commit",
		"",
		"Body text.",
		"",
	}, "\n"), string(payload))
}

// Signature headers end at the first empty line. A body line that starts with
// "gpgsig " is message text: it must neither become a signature (an unsigned
// commit would satisfy cryptosignature.required) nor leave the payload.
func TestExtractCommitSignatureData_BodyLineIsNotASignatureHeader(t *testing.T) {
	headers := []string{
		"tree 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"author Test User <test@example.com> 1700000000 +0000",
		"committer Test User <test@example.com> 1700000000 +0000",
	}
	body := []string{
		"",
		"feat: commit whose body imitates a signature header",
		"",
		"gpgsig -----BEGIN PGP SIGNATURE-----",
		" bm90IGEgc2lnbmF0dXJl",
		" -----END PGP SIGNATURE-----",
		"",
	}

	unsigned := []byte(strings.Join(append(append([]string{}, headers...), body...), "\n"))
	signature, payload := extractCommitSignatureData(unsigned)
	check.Equal(t, "", signature)
	check.Empty(t, payload)

	signedHeaders := append(append([]string{}, headers...),
		"gpgsig -----BEGIN SSH SIGNATURE-----",
		" AAAAB3NzaC1zaWcAAAAHc3NoLXNhAAAAAwEAAQ==",
		" -----END SSH SIGNATURE-----",
	)
	signed := []byte(strings.Join(append(signedHeaders, body...), "\n"))
	signature, payload = extractCommitSignatureData(signed)
	check.Equal(t, strings.Join([]string{
		"-----BEGIN SSH SIGNATURE-----",
		"AAAAB3NzaC1zaWcAAAAHc3NoLXNhAAAAAwEAAQ==",
		"-----END SSH SIGNATURE-----",
	}, "\n"), signature)
	check.Equal(t, string(unsigned), string(payload))
}

// A commit signed for both object formats carries "gpgsig" (over the SHA-1
// content) and "gpgsig-sha256". git reads only the header for the repository's
// format and hands the verifier the payload with both headers removed; this
// reader serves SHA-256 repositories, so it must keep the SHA-256 signature
// alone, whichever header comes first.
func TestExtractCommitSignatureData_DualSignedCommitKeepsTheSHA256Signature(t *testing.T) {
	headers := []string{
		"tree 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"author Test User <test@example.com> 1700000000 +0000",
		"committer Test User <test@example.com> 1700000000 +0000",
	}
	body := []string{"", "feat: dual-signed commit", ""}
	sha1Signature := []string{
		"gpgsig -----BEGIN PGP SIGNATURE-----",
		" c2hhMS1zaWduYXR1cmU=",
		" -----END PGP SIGNATURE-----",
	}
	sha256Signature := []string{
		"gpgsig-sha256 -----BEGIN PGP SIGNATURE-----",
		" c2hhMjU2LXNpZ25hdHVyZQ==",
		" -----END PGP SIGNATURE-----",
	}

	tests := []struct {
		name  string
		order [][]string
	}{
		{name: "gpgsig first", order: [][]string{sha1Signature, sha256Signature}},
		{name: "gpgsig-sha256 first", order: [][]string{sha256Signature, sha1Signature}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			lines := append([]string{}, headers...)
			for _, signature := range testCase.order {
				lines = append(lines, signature...)
			}

			signature, payload := extractCommitSignatureData([]byte(strings.Join(append(lines, body...), "\n")))

			check.Equal(t, strings.Join([]string{
				"-----BEGIN PGP SIGNATURE-----",
				"c2hhMjU2LXNpZ25hdHVyZQ==",
				"-----END PGP SIGNATURE-----",
			}, "\n"), signature)
			check.Equal(t, strings.Join(append(append([]string{}, headers...), body...), "\n"), string(payload))
		})
	}
}

// Reading a signature off a commit is the product's reason to exist, so this
// test states a tool requirement rather than skipping past it. A skip here is
// indistinguishable from a pass in a run of a thousand tests, which is how
// signature coverage disappears from a machine without anyone being told; the
// external-binary tier already fails loudly for the same reason (see
// test/integration/testkit). ssh-keygen ships with OpenSSH, so
// requiring it costs nothing a git-signing developer does not already have.
func TestCommitReader_GetCommit_SHA256SSHSignature(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Fatalf("ssh-keygen is required to verify SSH signature reading: %v", err)
	}

	repoPath := t.TempDir()
	ctx := t.Context()
	privateKey := filepath.Join(repoPath, "signing_key")

	keygen := exec.CommandContext(ctx, "ssh-keygen", "-t", "ed25519", "-N", "", "-C", testUserEmail, "-f", privateKey)
	if output, err := keygen.CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen cannot create test signing key: %v: %s", err, output)
	}

	for _, args := range [][]string{
		{"init", "--object-format=sha256"},
		{gitConfigCommand, "user.email", testUserEmail},
		{gitConfigCommand, "user.name", testUserName},
		{gitConfigCommand, "gpg.format", "ssh"},
		{gitConfigCommand, "user.signingkey", privateKey},
		{gitConfigCommand, "commit.gpgsign", "true"},
	} {
		_, err := runGitCommand(ctx, repoPath, args...)
		check.NoError(t, err)
	}

	filePath := filepath.Join(repoPath, "file.txt")
	check.NoError(t, os.WriteFile(filePath, []byte("signed\n"), 0600))

	_, err := runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)

	_, err = runGitCommand(ctx, repoPath, "commit", "-S", "-m", "feat: signed")
	if err != nil {
		t.Fatalf("git SSH commit signing is required by this test: %v", err)
	}

	rawCommit, err := runGitCommand(ctx, repoPath, "cat-file", "commit", "HEAD")
	check.NoError(t, err)
	check.Contains(t, string(rawCommit), "gpgsig-sha256 ")

	commit, err := NewCommitReader(repoPath).GetCommit(ctx, "HEAD")
	check.NoError(t, err)
	check.Contains(t, commit.Signature, "-----BEGIN SSH SIGNATURE-----")
	check.Contains(t, commit.Signature, "-----END SSH SIGNATURE-----")

	// The payload is what the signature covers: the raw commit object with
	// the signature header cut out, ending with the message.
	raw := string(rawCommit)
	signatureStart := strings.Index(raw, "gpgsig-sha256 ")
	headersEnd := strings.Index(raw, "\n\n")
	check.Greater(t, headersEnd, signatureStart)

	payload := string(commit.SignaturePayload)
	check.Equal(t, raw[:signatureStart]+raw[headersEnd+1:], payload)
	check.NotContains(t, payload, "gpgsig")
	check.True(t, strings.HasSuffix(payload, "\nfeat: signed\n"), "payload must end with the message: %q", payload)
}

func TestCommitReader_GetHeadCommits_SHA256(t *testing.T) {
	repoPath := t.TempDir()
	ctx := t.Context()

	_, err := runGitCommand(ctx, repoPath, "init", "--object-format=sha256")
	check.NoError(t, err)

	filePath := filepath.Join(repoPath, "file.txt")
	check.NoError(t, os.WriteFile(filePath, []byte("first"), 0600))

	_, err = runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "-m", "feat: first")
	check.NoError(t, err)

	check.NoError(t, os.WriteFile(filePath, []byte("second"), 0600))

	_, err = runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "-m", "feat: second")
	check.NoError(t, err)

	reader := NewCommitReader(repoPath)
	commits, err := reader.GetHeadCommits(ctx, 2)
	check.NoError(t, err)
	check.Len(t, commits, 2)
	check.Equal(t, "feat: second", commits[0].Subject)
	check.Equal(t, "feat: first", commits[1].Subject)
}

// git exports the variables that located its own repository to a hook: GIT_DIR
// whenever it did not find the repository as ./.git (a subdirectory, git -C, a
// worktree), GIT_WORK_TREE, and GIT_INDEX_FILE for a partial commit. Every
// product git call must read the repository it was given, not the hook's.
func TestGitCommands_IgnoreInheritedRepositoryEnvironment(t *testing.T) {
	ctx := t.Context()
	decoy := t.TempDir()
	repo := t.TempDir()

	for _, fixture := range []struct {
		path, format, subject string
	}{
		{path: decoy, format: "sha1", subject: "feat: decoy"},
		{path: repo, format: "sha256", subject: "feat: target"},
	} {
		_, err := runGitCommand(ctx, fixture.path, "init", "--object-format="+fixture.format)
		check.NoError(t, err)
		_, err = runGitCommand(ctx, fixture.path, "commit", "--allow-empty", "-m", fixture.subject)
		check.NoError(t, err)
	}

	t.Setenv("GIT_DIR", filepath.Join(decoy, ".git"))
	t.Setenv("GIT_WORK_TREE", decoy)

	format, err := DetectObjectFormat(ctx, repo)
	check.NoError(t, err)
	check.Equal(t, "sha256", format)

	commit, err := NewCommitReader(repo).GetCommit(ctx, "HEAD")
	check.NoError(t, err)
	check.Equal(t, "feat: target", commit.Subject)

	output, code, err := runPrePushGit(ctx, repo, "rev-parse", "HEAD")
	check.NoError(t, err)
	check.Equal(t, 0, code)
	check.Equal(t, commit.Hash, strings.TrimSpace(string(output)))
}

func TestCommitReader_GetCommit_MissingRefReturnsSentinel(t *testing.T) {
	repoPath := t.TempDir()
	ctx := t.Context()

	_, err := runGitCommand(ctx, repoPath, "init")
	check.NoError(t, err)

	reader := NewCommitReader(repoPath)
	for _, ref := range []string{"missing-ref", "--all"} {
		_, err = reader.GetCommit(ctx, ref)
		check.ErrorIs(t, err, domain.ErrReferenceNotFound, "ref %q", ref)
	}
}

func TestCommitReader_GetCommitRange_OrderAndEmpty(t *testing.T) {
	repoPath := t.TempDir()
	ctx := t.Context()

	_, err := runGitCommand(ctx, repoPath, "init", "--object-format=sha256")
	check.NoError(t, err)

	filePath := filepath.Join(repoPath, "file.txt")
	check.NoError(t, os.WriteFile(filePath, []byte("first"), 0600))

	_, err = runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "-m", "feat: first")
	check.NoError(t, err)

	check.NoError(t, os.WriteFile(filePath, []byte("second"), 0600))

	_, err = runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "-m", "feat: second")
	check.NoError(t, err)

	check.NoError(t, os.WriteFile(filePath, []byte("third"), 0600))

	_, err = runGitCommand(ctx, repoPath, "add", "file.txt")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "-m", "feat: third")
	check.NoError(t, err)

	reader := NewCommitReader(repoPath)
	headCommits, err := reader.GetHeadCommits(ctx, 3)
	check.NoError(t, err)
	check.Len(t, headCommits, 3)

	oldest := headCommits[len(headCommits)-1].Hash
	rangeCommits, err := reader.GetCommitRange(ctx, oldest, "HEAD")
	check.NoError(t, err)
	check.GreaterOrEqual(t, len(rangeCommits), 2)
	check.Equal(t, "feat: second", rangeCommits[0].Subject)
	check.Equal(t, "feat: third", rangeCommits[len(rangeCommits)-1].Subject)

	emptyCommits, err := reader.GetCommitRange(ctx, "HEAD", "HEAD")
	check.NoError(t, err)
	check.Empty(t, emptyCommits)
}

func TestCommitReader_GetCommitRange_SHA256MissingRefsReturnSentinel(t *testing.T) {
	repoPath := t.TempDir()
	ctx := t.Context()

	_, err := runGitCommand(ctx, repoPath, "init", "--object-format=sha256")
	check.NoError(t, err)
	_, err = runGitCommand(ctx, repoPath, "commit", "--allow-empty", "-m", "feat: initial")
	check.NoError(t, err)

	reader := NewCommitReader(repoPath)
	testCases := []struct {
		name    string
		fromRef string
		toRef   string
	}{
		{name: "missing_from", fromRef: "missing-from", toRef: "HEAD"},
		{name: "missing_to", fromRef: "HEAD", toRef: "missing-to"},
		{name: "option_like_from", fromRef: "--all", toRef: "HEAD"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			commits, err := reader.GetCommitRange(ctx, testCase.fromRef, testCase.toRef)
			check.ErrorIs(t, err, domain.ErrReferenceNotFound)
			check.Nil(t, commits)
		})
	}
}

func TestDetectObjectFormat_SHA256(t *testing.T) {
	repoPath := t.TempDir()
	ctx := t.Context()

	_, err := runGitCommand(ctx, repoPath, "init", "--object-format=sha256")
	check.NoError(t, err)

	format, err := DetectObjectFormat(ctx, repoPath)
	check.NoError(t, err)
	check.Equal(t, "sha256", format)
}
