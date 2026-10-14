//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	commandTimeout = 30 * time.Second

	// waitDelay bounds Wait after the child has exited or been killed: a
	// grandchild that escaped the process group can otherwise hold the
	// captured output pipes open indefinitely.
	waitDelay = 5 * time.Second
)

type harness struct {
	bin  string
	env  []string
	root string
	t    *testing.T
}

type commandResult struct {
	code   int
	stdout string
	stderr string
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	root := t.TempDir()
	bin := os.Getenv("GOMMITLINT_BLACKBOX_BIN")
	if bin == "" {
		t.Fatal("GOMMITLINT_BLACKBOX_BIN is required; use the supported external-test launcher")
	}
	if runtime.GOOS != "linux" {
		requireNativeVM(t)
	}

	for _, dir := range []string{"home", "config", "cache", "data", "state", "tmp", "gnupg"} {
		mustMkdir(t, filepath.Join(root, dir))
	}

	gitConfig := filepath.Join(root, "gitconfig")
	mustWriteFile(t, gitConfig, []byte("[user]\n\tname = Black Box\n\temail = blackbox@example.invalid\n"), 0o600)

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(root, "data"),
		"XDG_STATE_HOME=" + filepath.Join(root, "state"),
		"TMPDIR=" + filepath.Join(root, "tmp"),
		"GIT_CONFIG_GLOBAL=" + gitConfig,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_AUTH_SOCK=",
		"SSH_AGENT_PID=",
		"GNUPGHOME=" + filepath.Join(root, "gnupg"),
		"GPG_TTY=",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"NO_COLOR=1",
		"TERM=dumb",
		"GOMMITLINT_NO_CI_DETECT=1",
	}

	return &harness{bin: bin, env: env, root: root, t: t}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repository root from %s", dir)
		}

		dir = parent
	}
}

func (h *harness) run(workDir string, extraEnv []string, args ...string) commandResult {
	h.t.Helper()

	return h.runProgram(h.bin, workDir, extraEnv, args...)
}

func (h *harness) runProgram(program string, workDir string, extraEnv []string, args ...string) commandResult {
	h.t.Helper()

	return h.runProgramWithInput(program, workDir, extraEnv, nil, args...)
}

func (h *harness) runProgramWithInput(program string, workDir string, extraEnv []string, input []byte, args ...string) commandResult {
	h.t.Helper()
	h.requireSafeProgram(program)

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	cmd := boundedCommand(ctx, program, args...)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(h.env, extraEnv)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		h.t.Fatalf("command timed out: %s %s", program, strings.Join(args, " "))
	}

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			h.t.Fatalf("run %s %s: %v", program, strings.Join(args, " "), err)
		}

		code = exitErr.ExitCode()
	}

	return commandResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (h *harness) start(workDir string, extraEnv []string, args ...string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	h.t.Helper()
	h.requireSafeProgram(h.bin)

	// The caller signals and waits for the product itself, so no timeout
	// context here; the process group still lets killProcessGroup reap
	// whatever the product forked if the caller has to give up on it.
	cmd := boundedCommand(context.Background(), h.bin, args...)
	cmd.Dir = workDir
	cmd.Env = mergeEnv(h.env, extraEnv)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		h.t.Fatalf("start %s %s: %v", h.bin, strings.Join(args, " "), err)
	}

	return cmd, &stdout, &stderr
}

// boundedCommand runs program in its own process group so that cancelling ctx
// kills the whole tree (a hook, shell or pager the product forked included),
// and Wait returns within waitDelay even when a survivor keeps a pipe open.
func boundedCommand(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killProcessGroup(cmd) }
	cmd.WaitDelay = waitDelay

	return cmd
}

// killProcessGroup sends SIGKILL to the process group boundedCommand created
// for cmd, so forked descendants die with the direct child.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}

	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func (h *harness) requireSafeProgram(program string) {
	h.t.Helper()

	if program == "" {
		h.t.Fatal("program path cannot be empty")
	}
	if strings.ContainsRune(program, '\x00') || strings.ContainsAny(program, "\r\n") {
		h.t.Fatalf("program path contains control characters: %q", program)
	}
	if strings.HasPrefix(filepath.Base(program), "-") {
		h.t.Fatalf("program path cannot look like an option: %q", program)
	}
}

func mergeEnv(base []string, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range append(append([]string{}, base...), overrides...) {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}

		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}

	return env
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if after, ok := strings.CutPrefix(entry, prefix); ok {
			return after
		}
	}

	return ""
}

func requireTool(t *testing.T, name string) string {
	t.Helper()

	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}

	t.Fatalf("required tool %q is not available", name)

	return ""
}

// optionalToolFailure fails loudly instead of skipping when an environment
// capability (netns, gpg key generation, ...) is unavailable, so lost coverage
// is a visible failure rather than a silently skipped test.
func optionalToolFailure(t *testing.T, message string) {
	t.Helper()

	t.Fatal(message)
}

func exitCodeForSignal(sig syscall.Signal) int {
	return 128 + int(sig)
}

func (h *harness) caseDir(name string) string {
	h.t.Helper()

	dir := filepath.Join(h.root, "cases", name)
	mustMkdir(h.t, dir)

	return dir
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("create directory %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return data
}

func assertCode(t *testing.T, got int, want int, result commandResult) {
	t.Helper()

	if got != want {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", got, want, result.stdout, result.stderr)
	}
}

func assertNonZero(t *testing.T, result commandResult) {
	t.Helper()

	if result.code == 0 {
		t.Fatalf("expected non-zero exit\nstdout:\n%s\nstderr:\n%s", result.stdout, result.stderr)
	}
}

func assertNotContains(t *testing.T, haystack string, needle string, label string) {
	t.Helper()

	if strings.Contains(haystack, needle) {
		t.Fatalf("%s leaked %q in:\n%s", label, needle, haystack)
	}
}

type localCA struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
	pem  []byte
}

func newLocalCA(t *testing.T) localCA {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "gommitlint black-box CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	return localCA{
		cert: cert,
		key:  key,
		pem:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
	}
}

func randomSerial(t *testing.T) *big.Int {
	t.Helper()

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	return serial
}

func (ca localCA) serverCertificate(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(t),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key}
}

func (h *harness) trustCA(ca localCA) string {
	h.t.Helper()

	path := filepath.Join(h.root, "certs", "managed-source-ca.pem")
	mustMkdir(h.t, filepath.Dir(path))
	mustWriteFile(h.t, path, ca.pem, 0o600)
	if runtime.GOOS == "darwin" {
		// Darwin's system verifier does not use SSL_CERT_FILE. The disposable
		// VM exposes only this narrowly scoped, root-owned fixture helper.
		result := h.runProgramWithInput(requireTool(h.t, "sudo"), h.root, nil, ca.pem,
			"-n", "/usr/local/libexec/itiquette-test-trust-ca")
		assertCode(h.t, result.code, 0, result)
	}

	return path
}

func (h *harness) startTLSServer(ca localCA, handler http.Handler) string {
	h.t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		h.t.Fatalf("listen for local TLS fixture: %v", err)
	}

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{ca.serverCertificate(h.t)},
		MinVersion:   tls.VersionTLS12,
	}

	go func() {
		_ = server.Serve(tls.NewListener(listener, tlsConfig))
	}()

	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	return fmt.Sprintf("https://%s", listener.Addr().String())
}
