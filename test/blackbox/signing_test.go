//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPGSignatures_BlackBox(t *testing.T) {
	h := newHarness(t)
	git := requireTool(t, "git")
	gpg := requireTool(t, "gpg")

	t.Run("primary_signing_key", func(t *testing.T) {
		repo := h.caseDir("gpg-primary-signed-repo")
		keyDir := filepath.Join(h.root, "gpg-primary-signing-keys")
		mustMkdir(t, keyDir)

		fingerprint := generateGPGKey(t, h, gpg, repo, keyDir, gpgKeySpec{
			name:       "Black Box Primary GPG <blackbox-primary-gpg@example.invalid>",
			capability: "sign",
			publicKey:  "blackbox-primary-gpg.asc",
		})
		createGPGSignedCommit(t, h, git, gpg, repo, "Black Box Primary GPG", "blackbox-primary-gpg@example.invalid", fingerprint)
		writeGPGConfig(t, repo, keyDir, "require-gpg.yaml", true, false)

		valid := h.run(repo, nil, "--config", filepath.Join(repo, "require-gpg.yaml"), "validate", "--format", "json")
		assertCode(t, valid.code, 0, valid)
	})

	t.Run("signing_subkey", func(t *testing.T) {
		repo := h.caseDir("gpg-signed-repo")
		keyDir := filepath.Join(h.root, "gpg-signing-keys")
		mustMkdir(t, keyDir)

		fingerprint := generateGPGKey(t, h, gpg, repo, keyDir, gpgKeySpec{
			name:       "Black Box GPG <blackbox-gpg@example.invalid>",
			capability: "cert",
			publicKey:  "blackbox-gpg.asc",
			subkey:     true,
		})
		createGPGSignedCommit(t, h, git, gpg, repo, "Black Box GPG", "blackbox-gpg@example.invalid", fingerprint)
		configPath := writeGPGConfig(t, repo, keyDir, ".gommitlint.yaml", true, false)

		valid := h.run(repo, nil, "--config", configPath, "validate", "--format", "json")
		assertCode(t, valid.code, 0, valid)

		requireSSHPath := writeGPGConfig(t, repo, keyDir, ".gommitlint-require-ssh.yaml", false, true)
		wrongType := h.run(repo, nil, "--config", requireSSHPath, "validate", "--format", "json")
		assertNonZero(t, wrongType)
		if !strings.Contains(wrongType.stdout+wrongType.stderr, "GL502") {
			t.Fatalf("expected GL502 for GPG signature when SSH required\nstdout:\n%s\nstderr:\n%s", wrongType.stdout, wrongType.stderr)
		}
	})
}

type gpgKeySpec struct {
	name       string
	capability string
	publicKey  string
	subkey     bool
}

func generateGPGKey(t *testing.T, h *harness, gpg string, repo string, keyDir string, spec gpgKeySpec) string {
	t.Helper()

	generate := h.runProgram(
		gpg,
		repo,
		nil,
		"--batch",
		"--pinentry-mode", "loopback",
		"--passphrase", "",
		"--quick-generate-key",
		spec.name,
		"rsa2048",
		spec.capability,
		"1d",
	)
	if generate.code != 0 {
		optionalToolFailure(t, "gpg key generation unavailable: "+generate.stderr+generate.stdout)
	}

	list := h.runProgram(gpg, repo, nil, "--batch", "--with-colons", "--list-secret-keys", spec.name)
	assertCode(t, list.code, 0, list)
	fingerprint := gpgFingerprint(list.stdout)
	if fingerprint == "" {
		t.Fatalf("could not extract generated GPG key fingerprint from:\n%s", list.stdout)
	}

	if spec.subkey {
		addSigningKey := h.runProgram(
			gpg,
			repo,
			nil,
			"--batch",
			"--pinentry-mode", "loopback",
			"--passphrase", "",
			"--quick-add-key",
			fingerprint,
			"rsa2048",
			"sign",
			"1d",
		)
		if addSigningKey.code != 0 {
			optionalToolFailure(t, "gpg signing subkey generation unavailable: "+addSigningKey.stderr+addSigningKey.stdout)
		}
	}

	publicKeyPath := filepath.Join(keyDir, spec.publicKey)
	export := h.runProgram(gpg, repo, nil, "--batch", "--armor", "--output", publicKeyPath, "--export", fingerprint)
	assertCode(t, export.code, 0, export)

	return fingerprint
}

func createGPGSignedCommit(t *testing.T, h *harness, git string, gpg string, repo string, name string, email string, fingerprint string) {
	t.Helper()

	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", name},
		{"config", "user.email", email},
		{"config", "gpg.program", gpg},
		{"config", "user.signingkey", fingerprint},
		{"config", "commit.gpgsign", "true"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		result := h.runProgram(git, repo, nil, args...)
		assertCode(t, result.code, 0, result)
	}

	mustWriteFile(t, filepath.Join(repo, "signed.txt"), []byte("signed\n"), 0o600)
	add := h.runProgram(git, repo, nil, "add", "signed.txt")
	assertCode(t, add.code, 0, add)
	commit := h.runProgram(git, repo, nil, "commit", "-S", "-m", "feat: add gpg signature")
	if commit.code != 0 {
		optionalToolFailure(t, "git GPG commit signing unavailable: "+commit.stderr+commit.stdout)
	}
}

func writeGPGConfig(t *testing.T, repo string, keyDir string, name string, requireGPG bool, requireSSH bool) string {
	t.Helper()

	configPath := filepath.Join(repo, name)
	requireGPGLine := ""
	if requireGPG {
		requireGPGLine = "    require_gpg: true\n"
	}
	requireSSHLine := ""
	if requireSSH {
		requireSSHLine = "    require_ssh: true\n"
	}
	config := fmt.Sprintf(`gommitlint:
  rules:
    enabled:
      - cryptosignature
    disabled:
      - signoff
  crypto_signature:
    required: true
    verify_format: true
    key_directory: %q
%s%s`, keyDir, requireGPGLine, requireSSHLine)
	mustWriteFile(t, configPath, []byte(config), 0o600)

	return configPath
}

func gpgFingerprint(colonOutput string) string {
	for line := range strings.SplitSeq(colonOutput, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" && fields[9] != "" {
			return fields[9]
		}
	}

	return ""
}

func TestSSHSignatures_BlackBox(t *testing.T) {
	h := newHarness(t)
	git := requireTool(t, "git")
	sshKeygen := requireTool(t, "ssh-keygen")

	repo := h.caseDir("ssh-signed-repo")
	keyDir := filepath.Join(h.root, "ssh-signing-keys")
	mustMkdir(t, keyDir)
	privateKey := filepath.Join(keyDir, "blackbox_ed25519")

	keygen := h.runProgram(sshKeygen, repo, nil, "-t", "ed25519", "-N", "", "-C", "blackbox@example.invalid", "-f", privateKey)
	assertCode(t, keygen.code, 0, keygen)

	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Black Box"},
		{"config", "user.email", "blackbox@example.invalid"},
		{"config", "gpg.format", "ssh"},
		{"config", "user.signingkey", privateKey},
		{"config", "commit.gpgsign", "true"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		result := h.runProgram(git, repo, nil, args...)
		assertCode(t, result.code, 0, result)
	}

	mustWriteFile(t, filepath.Join(repo, "signed.txt"), []byte("signed\n"), 0o600)
	add := h.runProgram(git, repo, nil, "add", "signed.txt")
	assertCode(t, add.code, 0, add)
	commit := h.runProgram(git, repo, nil, "commit", "-S", "-m", "feat: add ssh signature")
	if commit.code != 0 {
		optionalToolFailure(t, "git SSH commit signing unavailable: "+commit.stderr+commit.stdout)
	}

	configPath := filepath.Join(repo, ".gommitlint.yaml")
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
	mustWriteFile(t, configPath, []byte(config), 0o600)

	valid := h.run(repo, nil, "--config", configPath, "validate", "--format", "json")
	assertCode(t, valid.code, 0, valid)

	requireGPGConfig := fmt.Sprintf(`gommitlint:
  rules:
    enabled:
      - cryptosignature
    disabled:
      - signoff
  crypto_signature:
    required: true
    require_gpg: true
    verify_format: true
    key_directory: %q
`, keyDir)
	requireGPGPath := filepath.Join(repo, ".gommitlint-require-gpg.yaml")
	mustWriteFile(t, requireGPGPath, []byte(requireGPGConfig), 0o600)

	wrongType := h.run(repo, nil, "--config", requireGPGPath, "validate", "--format", "json")
	assertNonZero(t, wrongType)
	if !strings.Contains(wrongType.stdout+wrongType.stderr, "GL502") {
		t.Fatalf("expected GL502 for SSH signature when GPG required\nstdout:\n%s\nstderr:\n%s", wrongType.stdout, wrongType.stderr)
	}
}
