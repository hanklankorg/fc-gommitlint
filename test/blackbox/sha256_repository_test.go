//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// SHA-256 repositories are the one place the product hands history to the
// system git instead of go-git: the object format decides the adapter, and
// the pre-push resolver always asks git. These rows build the repositories
// with the system git and drive the shipped binary over them, so the
// distribution matrix checks gommitlint against each distribution's own git
// on exactly that path. A git that cannot create a SHA-256 repository fails
// the row; it is never skipped.

func isSHA256ObjectID(id string) bool {
	return regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(id)
}

type sha256Repo struct {
	h       *harness
	git     string
	path    string
	commits int
}

func newSHA256Repo(t *testing.T, h *harness, name string, extraConfig ...[]string) *sha256Repo {
	t.Helper()

	repo := &sha256Repo{h: h, git: requireTool(t, "git"), path: h.caseDir(name)}

	initResult := h.runProgram(repo.git, repo.path, nil, "init", "--object-format=sha256", "--initial-branch=main")
	if initResult.code != 0 {
		optionalToolFailure(t, "git cannot create a SHA-256 repository: "+initResult.stderr+initResult.stdout)
	}

	if format := repo.gitOutput(t, "rev-parse", "--show-object-format"); format != "sha256" {
		t.Fatalf("git init --object-format=sha256 produced a %q repository", format)
	}

	config := append([][]string{
		{"user.name", "Black Box"},
		{"user.email", "blackbox@example.invalid"},
		{"commit.gpgsign", "false"},
	}, extraConfig...)
	for _, pair := range config {
		repo.gitOutput(t, append([]string{"config"}, pair...)...)
	}

	mustWriteFile(t, filepath.Join(repo.path, ".gommitlint.yaml"), []byte(signoffDisabledConfig), 0o600)

	return repo
}

// gitOutput runs git in the repository, requires success, and returns its
// trimmed stdout.
func (repo *sha256Repo) gitOutput(t *testing.T, args ...string) string {
	t.Helper()

	result := repo.h.runProgram(repo.git, repo.path, nil, args...)
	assertCode(t, result.code, 0, result)

	return strings.TrimSpace(result.stdout)
}

// commit records one change with the given message and returns its object ID,
// which must be a SHA-256 ID.
func (repo *sha256Repo) commit(t *testing.T, message string, commitArgs ...string) string {
	t.Helper()

	repo.commits++
	file := fmt.Sprintf("change-%d.txt", repo.commits)
	mustWriteFile(t, filepath.Join(repo.path, file), []byte(message+"\n"), 0o600)
	repo.gitOutput(t, "add", file)
	repo.gitOutput(t, append(append([]string{"commit"}, commitArgs...), "-m", message)...)

	id := repo.gitOutput(t, "rev-parse", "HEAD")
	if !isSHA256ObjectID(id) {
		t.Fatalf("commit %q has object ID %q, not a SHA-256 ID", message, id)
	}

	return id
}

type sha256Report struct {
	Summary struct {
		AllPassed     bool `json:"allPassed"`
		TotalCommits  int  `json:"totalCommits"`
		FailedCommits int  `json:"failedCommits"`
	} `json:"summary"`
	Commits []struct {
		Hash    string `json:"hash"`
		Subject string `json:"subject"`
		Passed  bool   `json:"passed"`
	} `json:"commits"`
}

func decodeSHA256Report(t *testing.T, result commandResult) sha256Report {
	t.Helper()

	var report sha256Report
	if err := json.Unmarshal([]byte(result.stdout), &report); err != nil {
		t.Fatalf("output is not a JSON report: %v\nstdout:\n%s\nstderr:\n%s", err, result.stdout, result.stderr)
	}

	return report
}

// TestSHA256Repository_Validate_ReportsCommitsBySHA256ID validates a valid
// HEAD and then a range holding one invalid commit. The report must carry
// the full 64-hex IDs git assigned, so a commit resolved through a truncated
// or SHA-1-shaped ID cannot pass as the right one, and the exit code and
// failure attribution must match the ordinary contract rows.
func TestSHA256Repository_Validate_ReportsCommitsBySHA256ID(t *testing.T) {
	h := newHarness(t)
	repo := newSHA256Repo(t, h, "sha256-validate")

	repo.commit(t, "feat: add sha256 base")
	repo.gitOutput(t, "checkout", "-b", "feature/sha256")
	invalid := repo.commit(t, "invalid sha256 commit message")
	head := repo.commit(t, "docs: document the sha256 feature")

	valid := h.run(repo.path, nil, "validate", "--format", "json")
	assertCode(t, valid.code, 0, valid)

	validReport := decodeSHA256Report(t, valid)
	if !validReport.Summary.AllPassed || len(validReport.Commits) != 1 || validReport.Commits[0].Hash != head {
		t.Fatalf("HEAD validation must pass and report HEAD %s, got:\n%s", head, valid.stdout)
	}

	failing := h.run(repo.path, nil, "validate", "--range", "main..HEAD", "--format", "json")
	assertCode(t, failing.code, 1, failing)

	report := decodeSHA256Report(t, failing)
	if report.Summary.TotalCommits != 2 || report.Summary.FailedCommits != 1 {
		t.Fatalf("range main..HEAD must hold 2 commits with 1 failure, got:\n%s", failing.stdout)
	}

	for _, commit := range report.Commits {
		if !isSHA256ObjectID(commit.Hash) {
			t.Fatalf("report carries %q, not a SHA-256 ID:\n%s", commit.Hash, failing.stdout)
		}

		if want := commit.Hash != invalid; commit.Passed != want {
			t.Fatalf("commit %s (%q) passed=%v, want %v:\n%s", commit.Hash, commit.Subject, commit.Passed, want, failing.stdout)
		}
	}

	text := h.run(repo.path, nil, "validate", "--range", "main..HEAD", "--format", "text")
	assertCode(t, text.code, 1, text)
	if !strings.Contains(text.stdout, "invalid sha256 commit message") {
		t.Fatalf("text report does not name the failing commit\nstdout:\n%s\nstderr:\n%s", text.stdout, text.stderr)
	}
}

// TestSHA256Repository_PrePush_ValidatesOnlyPushedSignatures pushes a
// SHA-256 repository to a SHA-256 bare remote through the installed pre-push
// hook, so git itself feeds the hook 64-hex object IDs and the product
// resolves them with the system git. Signatures in a SHA-256 repository sit
// in git's SHA-256 signature header, which the signature rule has to find.
//
// An unsigned commit is pushed first with --no-verify. A signed commit on top
// must then push, which it only can if the hook excluded what the remote
// already holds; a second unsigned commit must be refused, and the remote
// must keep its previous tip.
func TestSHA256Repository_PrePush_ValidatesOnlyPushedSignatures(t *testing.T) {
	h := newHarness(t)
	sshKeygen := requireTool(t, "ssh-keygen")

	keyDir := filepath.Join(h.root, "sha256-ssh-keys")
	mustMkdir(t, keyDir)
	privateKey := filepath.Join(keyDir, "blackbox_ed25519")
	keygen := h.runProgram(sshKeygen, keyDir, nil, "-t", "ed25519", "-N", "", "-C", "blackbox@example.invalid", "-f", privateKey)
	assertCode(t, keygen.code, 0, keygen)

	remote := h.caseDir("sha256-remote.git")
	bare := h.runProgram(requireTool(t, "git"), remote, nil, "init", "--bare", "--object-format=sha256", "--initial-branch=main")
	if bare.code != 0 {
		optionalToolFailure(t, "git cannot create a SHA-256 bare repository: "+bare.stderr+bare.stdout)
	}

	repo := newSHA256Repo(t, h, "sha256-push",
		[]string{"gpg.format", "ssh"},
		[]string{"user.signingkey", privateKey},
	)
	repo.gitOutput(t, "remote", "add", "origin", remote)

	config := fmt.Sprintf(`gommitlint:
  rules:
    enabled:
      - cryptosignature
    disabled:
      - signoff
  crypto_signature:
    required: true
    require_ssh: true
    verify_format: true
    key_directory: %q
`, keyDir)
	mustWriteFile(t, filepath.Join(repo.path, ".gommitlint.yaml"), []byte(config), 0o600)

	remoteTip := func(t *testing.T) string {
		t.Helper()

		result := h.runProgram(repo.git, remote, nil, "rev-parse", "refs/heads/main")
		assertCode(t, result.code, 0, result)

		return strings.TrimSpace(result.stdout)
	}

	unsignedBase := repo.commit(t, "feat: add unsigned base", "--no-gpg-sign")
	bypass := h.runProgram(repo.git, repo.path, nil, "push", "--no-verify", "origin", "main")
	assertCode(t, bypass.code, 0, bypass)

	install := h.run(repo.path, nil, "hook", "install", "--repo-path", repo.path)
	assertCode(t, install.code, 0, install)

	signed := repo.commit(t, "feat: add signed change", "-S")
	push := h.runProgram(repo.git, repo.path, nil, "push", "origin", "main")
	assertCode(t, push.code, 0, push)
	if !strings.Contains(push.stderr, "All checks passed") {
		t.Fatalf("pre-push hook did not validate the pushed commit\nstdout:\n%s\nstderr:\n%s", push.stdout, push.stderr)
	}
	if tip := remoteTip(t); tip != signed {
		t.Fatalf("remote main = %s, want the signed commit %s (unsigned base %s)", tip, signed, unsignedBase)
	}

	repo.commit(t, "feat: add unsigned change", "--no-gpg-sign")
	refused := h.runProgram(repo.git, repo.path, nil, "push", "origin", "main")
	assertNonZero(t, refused)
	output := refused.stdout + refused.stderr
	if !strings.Contains(output, "feat: add unsigned change") {
		t.Fatalf("refused push does not name the unsigned commit\nstdout:\n%s\nstderr:\n%s", refused.stdout, refused.stderr)
	}

	assertNotContains(t, output, "feat: add unsigned base", "refused push report")
	if tip := remoteTip(t); tip != signed {
		t.Fatalf("remote main moved to %s after a refused push, want %s", tip, signed)
	}
}
