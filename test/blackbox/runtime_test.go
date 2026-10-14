//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const largeHistoryCommitCount = 1000

func TestLocale_BlackBox(t *testing.T) {
	h := newHarness(t)
	msgfmt := requireTool(t, "msgfmt")

	moDir := filepath.Join(envValue(h.env, "XDG_DATA_HOME"), "locale", "sv", "LC_MESSAGES")
	mustMkdir(t, moDir)

	result := h.runProgram(
		msgfmt,
		repoRoot(t),
		nil,
		"--check",
		"-o", filepath.Join(moDir, "gommitlint.mo"),
		filepath.Join(repoRoot(t), "locale", "sv", "gommitlint.po"),
	)
	assertCode(t, result.code, 0, result)

	localeEnv := []string{"LC_ALL=sv_SE.UTF-8", "LANG=sv_SE.UTF-8"}

	// --version is deliberately excluded from localisation: scripts and CI
	// parse that line, so a translated build-date label would break them. See
	// handleVersionFlag and the Version field in createCLIApp, and note that
	// locale/sv/gommitlint.po carries no entry for it. Assert the English
	// wording survives under a Swedish locale rather than reinstating a
	// Swedish assertion that the catalogue cannot satisfy. Everything below
	// proves the catalogue really is in effect.
	version := h.run(h.caseDir("locale"), localeEnv, "--version")
	assertCode(t, version.code, 0, version)
	if !strings.Contains(version.stdout, "Build date:") {
		t.Fatalf("expected --version to stay unlocalised under a Swedish locale, got:\n%s", version.stdout)
	}

	// Each assertion below is backed by a msgstr in locale/sv/gommitlint.po:
	// "ANVÄNDNING" (help heading), "Nästa steg:" (recovery card heading) and
	// "Validera HEAD-incheckningen i stället: ..." (recovery suggestion).
	help := h.run(h.caseDir("locale-help"), localeEnv, "--help")
	assertCode(t, help.code, 0, help)
	if !strings.Contains(help.stdout, "ANVÄNDNING") {
		t.Fatalf("expected Swedish help output to contain ANVÄNDNING, got:\n%s", help.stdout)
	}

	errorResult := h.run(h.caseDir("locale-error"), localeEnv, "validate", "--message-file", "missing.txt")
	assertNonZero(t, errorResult)
	if !strings.Contains(errorResult.stderr, "Nästa steg") || !strings.Contains(errorResult.stderr, "Validera HEAD-incheckning") {
		t.Fatalf("expected Swedish recovery hints, got stdout:\n%s\nstderr:\n%s", errorResult.stdout, errorResult.stderr)
	}

	jsonDir := h.caseDir("locale-json")
	messageFile := filepath.Join(jsonDir, "message.txt")
	mustWriteFile(t, messageFile, []byte("feat: add localized json\n\nSigned-off-by: Black Box <blackbox@example.invalid>\n"), 0o600)
	jsonResult := h.run(jsonDir, localeEnv, "validate", "--message-file", messageFile, "--format", "json")
	assertCode(t, jsonResult.code, 0, jsonResult)
	var payload map[string]any
	if err := json.Unmarshal([]byte(jsonResult.stdout), &payload); err != nil {
		t.Fatalf("localized JSON output is not parseable: %v\n%s", err, jsonResult.stdout)
	}
	if _, ok := payload["summary"]; !ok {
		t.Fatalf("localized JSON output missing stable summary key:\n%s", jsonResult.stdout)
	}
}

func TestShellCompletion_BlackBox(t *testing.T) {
	h := newHarness(t)
	workDir := h.caseDir("completion")

	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			result := h.run(workDir, nil, "completion", "--stdout", shell)
			assertCode(t, result.code, 0, result)
			if !strings.Contains(result.stdout, "gommitlint") {
				t.Fatalf("completion for %s does not mention gommitlint:\n%s", shell, result.stdout)
			}
		})
	}
}

func TestPTYAndColor_BlackBox(t *testing.T) {
	h := newHarness(t)
	script := requireTool(t, "script")
	repo := createGitRepoWithCommit(t, h, "pty-color", "bad message")
	colorEnv := []string{"NO_COLOR=", "TERM=xterm-256color"}

	pipe := h.run(repo, colorEnv, "validate", "--color", "auto")
	assertNonZero(t, pipe)
	assertNotContains(t, pipe.stdout, "\x1b[", "piped auto-color output")

	ttyCommand := fmt.Sprintf("%q validate --color auto", h.bin)
	tty := h.runProgram(script, repo, colorEnv, ptyCommandArgs(ttyCommand)...)
	if tty.stdout == "" {
		t.Fatalf("pty validation produced empty stdout\nstderr:\n%s", tty.stderr)
	}
	if !strings.Contains(tty.stdout, "\x1b[") {
		t.Fatalf("expected TTY auto-color output to contain ANSI color, got:\n%s", tty.stdout)
	}

	always := h.run(repo, colorEnv, "validate", "--color", "always")
	assertNonZero(t, always)
	if !strings.Contains(always.stdout, "\x1b[") {
		t.Fatalf("expected --color always output to contain ANSI color, got:\n%s", always.stdout)
	}

	noColor := h.run(repo, []string{"NO_COLOR=1", "TERM=xterm-256color"}, "validate", "--color", "always")
	assertNonZero(t, noColor)
	assertNotContains(t, noColor.stdout, "\x1b[", "NO_COLOR output")

	dumb := h.run(repo, []string{"NO_COLOR=", "TERM=dumb"}, "validate", "--color", "always")
	assertNonZero(t, dumb)
	assertNotContains(t, dumb.stdout, "\x1b[", "TERM=dumb output")

	for _, width := range []string{"40", "160"} {
		t.Run("table_width_"+width, func(t *testing.T) {
			command := fmt.Sprintf("stty cols %s; %q validate --format table", width, h.bin)
			result := h.runProgram(script, repo, colorEnv, ptyCommandArgs(command)...)
			if result.stdout == "" || !strings.Contains(result.stdout, "SUMMARY") {
				t.Fatalf("table output for width %s missing summary\nstdout:\n%s\nstderr:\n%s", width, result.stdout, result.stderr)
			}
		})
	}
}

func ptyCommandArgs(command string) []string {
	switch runtime.GOOS {
	case "openbsd":
		// OpenBSD accepts -c but has no quiet (-q) option.
		return []string{"-c", command, "/dev/null"}
	case "darwin", "freebsd":
		// Darwin/FreeBSD script(1) takes the output file before the argv.
		return []string{"-q", "/dev/null", "/bin/sh", "-c", command}
	default:
		return []string{"-q", "-c", command, "/dev/null"}
	}
}

func TestSignalCancellation_BlackBox(t *testing.T) {
	h := newHarness(t)
	ca := newLocalCA(t)
	caFile := h.trustCA(ca)
	policy := []byte("gommitlint:\n  rules:\n    disabled:\n      - signoff\n")

	for _, testCase := range []struct {
		name string
		sig  syscall.Signal
	}{
		{name: "sigint", sig: syscall.SIGINT},
		{name: "sigquit", sig: syscall.SIGQUIT},
		{name: "sigterm", sig: syscall.SIGTERM},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			requested := make(chan struct{}, 1)
			release := make(chan struct{})
			mux := http.NewServeMux()
			mux.HandleFunc("/policy.yaml", func(_ http.ResponseWriter, request *http.Request) {
				select {
				case requested <- struct{}{}:
				default:
				}

				select {
				case <-release:
				case <-request.Context().Done():
				}
			})

			serverURL := h.startTLSServer(ca, mux)
			workDir := h.caseDir("signal-" + testCase.name)
			configPath := filepath.Join(workDir, ".gommitlint.yaml")
			config := fmt.Sprintf(`gommitlint:
  managed_sources:
    - id: team-policy
      source: %q
      destination: .gommitlint.team.yaml
      integrity:
        sha256: %s
`, serverURL+"/policy.yaml", sha256Hex(policy))
			mustWriteFile(t, configPath, []byte(config), 0o600)

			cmd, stdout, stderr := h.start(workDir, []string{"SSL_CERT_FILE=" + caFile}, "--config", configPath, "config", "sync")
			defer close(release)

			select {
			case <-requested:
			case <-time.After(10 * time.Second):
				_ = killProcessGroup(cmd)
				t.Fatalf("config sync did not reach local TLS fixture\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
			}

			if err := cmd.Process.Signal(testCase.sig); err != nil {
				_ = killProcessGroup(cmd)
				t.Fatalf("send %s: %v", testCase.sig, err)
			}

			err := waitWithTimeout(cmd, 10*time.Second)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected signal exit error, got %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
			}

			want := exitCodeForSignal(testCase.sig)
			if exitErr.ExitCode() != want {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", exitErr.ExitCode(), want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestResourceLimitSmoke_BlackBox(t *testing.T) {
	h := newHarness(t)

	workDir := h.caseDir("resource-limit")
	var version commandResult
	if runtime.GOOS == "linux" {
		version = h.runProgram(requireTool(t, "prlimit"), workDir, nil, "--nofile=32:32", "--", h.bin, "--version")
	} else {
		version = h.runProgram(requireTool(t, "sh"), workDir, nil, "-c", `ulimit -n 32; exec "$@"`, "limited", h.bin, "--version")
	}
	assertCode(t, version.code, 0, version)

	messageFile := filepath.Join(workDir, "message.txt")
	mustWriteFile(t, messageFile, []byte("feat: add constrained validation\n\nSigned-off-by: Black Box <blackbox@example.invalid>\n"), 0o600)
	var validation commandResult
	if runtime.GOOS == "linux" {
		validation = h.runProgram(requireTool(t, "prlimit"), workDir, nil, "--as=2147483648:2147483648", "--", h.bin, "validate", "--message-file", messageFile, "--format", "json")
	} else {
		// Darwin does not implement Linux's per-process address-space limit.
		// Verify the real guest RAM bound instead, rather than pretending an
		// ignored ulimit is equivalent. The VM controller allocates 4 GiB.
		key := "hw.physmem64"
		switch runtime.GOOS {
		case "darwin":
			key = "hw.memsize"
		case "freebsd", "openbsd":
			key = "hw.physmem"
		}
		memory := h.runProgram(requireTool(t, "sysctl"), workDir, nil, "-n", key)
		assertCode(t, memory.code, 0, memory)
		bytes, err := strconv.ParseUint(strings.TrimSpace(memory.stdout), 10, 64)
		if err != nil || bytes == 0 {
			t.Fatalf("cannot read guest memory from sysctl %s: stdout=%q stderr=%q (%v)", key, memory.stdout, memory.stderr, err)
		}
		if bytes > 4*1024*1024*1024 {
			t.Fatalf("guest exceeds the 4 GiB VM memory bound: %d bytes", bytes)
		}
		validation = h.run(workDir, nil, "validate", "--message-file", messageFile, "--format", "json")
	}
	assertCode(t, validation.code, 0, validation)
}

func TestZeroEgressNamespaceSmoke_BlackBox(t *testing.T) {
	h := newHarness(t)
	workDir := h.caseDir("zero-egress")
	messageFile := filepath.Join(workDir, "message.txt")
	mustWriteFile(t, messageFile, []byte("feat: add offline validation\n\nSigned-off-by: Black Box <blackbox@example.invalid>\n"), 0o600)

	var result commandResult
	if runtime.GOOS == "linux" {
		result = h.runProgram(requireTool(t, "unshare"), workDir, nil, "-rn", "--", h.bin, "validate", "--message-file", messageFile, "--format", "json")
	} else {
		requireNativeVM(t)
		interfaces, err := net.Interfaces()
		if err != nil {
			t.Fatal(err)
		}
		for _, device := range interfaces {
			if device.Flags&net.FlagUp != 0 && device.Flags&net.FlagLoopback == 0 {
				t.Fatalf("VM still has an active non-loopback interface: %s", device.Name)
			}
		}
		result = h.run(workDir, nil, "validate", "--message-file", messageFile, "--format", "json")
	}
	if result.code != 0 && strings.Contains(strings.ToLower(result.stderr+result.stdout), "operation not permitted") {
		optionalToolFailure(t, "unprivileged network namespaces are unavailable: "+result.stderr+result.stdout)
	}

	assertCode(t, result.code, 0, result)
}

func TestNoNetworkProxySmoke_BlackBox(t *testing.T) {
	h := newHarness(t)
	workDir := h.caseDir("no-network-proxy")
	messageFile := filepath.Join(workDir, "message.txt")
	mustWriteFile(t, messageFile, []byte("feat: add offline validation\n\nSigned-off-by: Black Box <blackbox@example.invalid>\n"), 0o600)

	deadProxyEnv := []string{
		"HTTP_PROXY=http://127.0.0.1:1",
		"HTTPS_PROXY=http://127.0.0.1:1",
		"ALL_PROXY=http://127.0.0.1:1",
		"NO_PROXY=",
	}
	result := h.run(workDir, deadProxyEnv, "validate", "--message-file", messageFile, "--format", "json")
	assertCode(t, result.code, 0, result)
}

func TestLargeHistory_BlackBox(t *testing.T) {
	h := newHarness(t)
	repo := createLargeHistoryRepo(t, h, "large-history", largeHistoryCommitCount)
	configPath := filepath.Join(repo, ".gommitlint.yaml")
	config := fmt.Sprintf("gommitlint:\n  limits:\n    max_commit_count: %d\n", largeHistoryCommitCount)
	mustWriteFile(t, configPath, []byte(config), 0o600)

	jsonStart := time.Now()
	jsonResult := h.run(repo, nil, "--config", configPath, "validate", "--count", fmt.Sprint(largeHistoryCommitCount), "--format", "json")
	jsonElapsed := time.Since(jsonStart)
	assertCode(t, jsonResult.code, 0, jsonResult)
	assertLargeHistoryJSON(t, jsonResult.stdout, largeHistoryCommitCount)

	textStart := time.Now()
	textResult := h.run(repo, nil, "--config", configPath, "validate", "--count", fmt.Sprint(largeHistoryCommitCount), "--format", "text")
	textElapsed := time.Since(textStart)
	assertCode(t, textResult.code, 0, textResult)
	if !strings.Contains(strings.ToLower(textResult.stdout), "passed") {
		t.Fatalf("large-history text output did not report passing validation:\n%s", textResult.stdout)
	}

	exceeds := h.run(repo, nil, "--config", configPath, "validate", "--count", fmt.Sprint(largeHistoryCommitCount+1), "--format", "json")
	assertCode(t, exceeds.code, 65, exceeds)
	if !strings.Contains(exceeds.stdout+exceeds.stderr, "max_commit_count") {
		t.Fatalf("max-count rejection did not mention max_commit_count\nstdout:\n%s\nstderr:\n%s", exceeds.stdout, exceeds.stderr)
	}

	repoSize := dirSizeBytes(t, repo)
	totalElapsed := jsonElapsed + textElapsed
	t.Logf("large-history: commits=%d rules=default json_elapsed=%s text_elapsed=%s repo_size_bytes=%d machine=%s/%s",
		largeHistoryCommitCount, jsonElapsed, textElapsed, repoSize, runtime.GOOS, runtime.GOARCH)
	assertOptionalPerformanceBudget(t, totalElapsed)
}

func createGitRepoWithCommit(t *testing.T, h *harness, name string, message string) string {
	t.Helper()

	git := requireTool(t, "git")
	repo := h.caseDir(name)

	for _, args := range [][]string{
		{"init"},
		{"config", "user.name", "Black Box"},
		{"config", "user.email", "blackbox@example.invalid"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		result := h.runProgram(git, repo, nil, args...)
		assertCode(t, result.code, 0, result)
	}

	mustWriteFile(t, filepath.Join(repo, "file.txt"), []byte(message+"\n"), 0o600)
	add := h.runProgram(git, repo, nil, "add", "file.txt")
	assertCode(t, add.code, 0, add)
	commit := h.runProgram(git, repo, nil, "commit", "--no-gpg-sign", "-m", message)
	assertCode(t, commit.code, 0, commit)

	return repo
}

func createLargeHistoryRepo(t *testing.T, h *harness, name string, count int) string {
	t.Helper()
	git := requireTool(t, "git")
	repo := h.caseDir(name)

	for _, args := range [][]string{
		{"init"},
		{"symbolic-ref", "HEAD", "refs/heads/main"},
		{"config", "user.name", "Black Box"},
		{"config", "user.email", "blackbox@example.invalid"},
		{"config", "commit.gpgsign", "false"},
		{"config", "core.hooksPath", "/dev/null"},
	} {
		result := h.runProgram(git, repo, nil, args...)
		assertCode(t, result.code, 0, result)
	}

	var stream strings.Builder
	const baseTimestamp = int64(1_700_000_000)
	for i := 1; i <= count; i++ {
		mark := 100000 + i
		message := fmt.Sprintf("feat: add large history %04d\n\nSigned-off-by: Black Box <blackbox@example.invalid>\n", i)
		fileData := fmt.Sprintf("commit %04d\n", i)

		_, _ = fmt.Fprintf(&stream, "commit refs/heads/main\n")
		_, _ = fmt.Fprintf(&stream, "mark :%d\n", mark)
		_, _ = fmt.Fprintf(&stream, "author Black Box <blackbox@example.invalid> %d +0000\n", baseTimestamp+int64(i))
		_, _ = fmt.Fprintf(&stream, "committer Black Box <blackbox@example.invalid> %d +0000\n", baseTimestamp+int64(i))
		_, _ = fmt.Fprintf(&stream, "data %d\n%s", len(message), message)
		if i > 1 {
			_, _ = fmt.Fprintf(&stream, "from :%d\n", mark-1)
		}
		_, _ = fmt.Fprintf(&stream, "M 100644 inline file.txt\n")
		_, _ = fmt.Fprintf(&stream, "data %d\n%s", len(fileData), fileData)
	}

	importResult := h.runProgramWithInput(git, repo, nil, []byte(stream.String()), "fast-import", "--quiet")
	assertCode(t, importResult.code, 0, importResult)

	return repo
}

func assertLargeHistoryJSON(t *testing.T, output string, wantCommits int) {
	t.Helper()

	var payload struct {
		Summary struct {
			AllPassed        bool `json:"allPassed"`
			TotalCommits     int  `json:"totalCommits"`
			ValidatedCommits int  `json:"validatedCommits"`
			FailedCommits    int  `json:"failedCommits"`
		} `json:"summary"`
		Commits []struct {
			Passed bool `json:"passed"`
		} `json:"commits"`
	}
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		t.Fatalf("large-history JSON output is not parseable: %v\n%s", err, output)
	}

	if !payload.Summary.AllPassed || payload.Summary.FailedCommits != 0 {
		t.Fatalf("large-history JSON did not report all commits passing: %+v", payload.Summary)
	}
	if payload.Summary.TotalCommits != wantCommits || payload.Summary.ValidatedCommits != wantCommits || len(payload.Commits) != wantCommits {
		t.Fatalf("large-history JSON commit counts = total %d validated %d entries %d, want %d",
			payload.Summary.TotalCommits, payload.Summary.ValidatedCommits, len(payload.Commits), wantCommits)
	}
}

func dirSizeBytes(t *testing.T, root string) int64 {
	t.Helper()

	var size int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()

		return nil
	})
	if err != nil {
		t.Fatalf("measure directory size %s: %v", root, err)
	}

	return size
}

func assertOptionalPerformanceBudget(t *testing.T, elapsed time.Duration) {
	t.Helper()

	budgetValue := strings.TrimSpace(os.Getenv("GOMMITLINT_BLACKBOX_LARGE_HISTORY_BUDGET"))
	if budgetValue == "" {
		return
	}

	budget, err := time.ParseDuration(budgetValue)
	if err != nil {
		t.Fatalf("parse GOMMITLINT_BLACKBOX_LARGE_HISTORY_BUDGET=%q: %v", budgetValue, err)
	}
	if elapsed > budget {
		t.Fatalf("large-history elapsed time %s exceeds budget %s", elapsed, budget)
	}
}

func waitWithTimeout(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = killProcessGroup(cmd)
		return fmt.Errorf("process did not exit within %s", timeout)
	}
}
