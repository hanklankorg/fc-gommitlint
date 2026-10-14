//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestHarness_BoundedCommandKillsForkedChildren is a harness self-test: a shell
// that forks a sleeper and waits for it must return when the bound expires,
// not when the sleeper does, and the sleeper must not survive the bound.
func TestHarness_BoundedCommandKillsForkedChildren(t *testing.T) {
	const bound = time.Second

	sleeperPIDFile := filepath.Join(t.TempDir(), "sleeper.pid")

	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()

	cmd := boundedCommand(ctx, "/bin/sh", "-c", `sleep 6 & printf '%s\n' "$!" > "$1"; wait`, "sh", sleeperPIDFile)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	elapsed := time.Since(started)

	if ctx.Err() == nil {
		t.Fatalf("expected the bound to expire, got err=%v\nstderr:\n%s", err, stderr.String())
	}
	if elapsed > bound+3*time.Second {
		t.Fatalf("Run returned after %s, the %s bound did not cut the forked sleeper short", elapsed, bound)
	}

	sleeperPID := readPID(t, sleeperPIDFile)
	deadline := time.Now().Add(2 * time.Second)
	for {
		terminated, alive := processTerminated(t, sleeperPID)
		if terminated {
			return
		}

		if time.Now().After(deadline) {
			_ = syscall.Kill(sleeperPID, syscall.SIGKILL)
			t.Fatalf("forked sleeper %d survived the bound (%s)", sleeperPID, alive)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// processTerminated reports whether pid has stopped running, and describes what
// it saw when it has not, for the failure message.
//
// kill(pid, 0) cannot tell a live process from a zombie: a dead child nobody
// has wait()ed for still holds its pid table entry, so the signal probe keeps
// succeeding on it. The state check is what separates the two, so that a
// sleeper the bound already killed is not read as a survivor.
//
// Nothing reaches state Z here at present. Commit 4a0f447 gave the harness
// container `podman run --init`, so catatonit is PID 1 and reaps the orphans
// the kernel reparents to it when their parent dies; a bare kill(pid, 0)
// passes this test today. Before that, bash exec-optimised the single-command
// tier, the `go` tool became PID 1 and never wait()ed, and every killed
// grandchild lingered as a zombie — this test then failed although the bound
// had done exactly its job. The check is therefore insurance rather than load
// bearing, and it becomes load bearing again the moment these tiers run under
// a PID 1 that does not reap: --init dropped from the sandbox launcher, or
// the suite run anywhere other than that container.
//
// Process groups are Unix-wide. Linux exposes zombie state through procfs;
// macOS and BSD expose it through ps. An absent procfs is not proof of death.
func processTerminated(t *testing.T, pid int) (bool, string) {
	t.Helper()

	if err := syscall.Kill(pid, 0); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return true, ""
		}

		// EPERM would mean a live process this test may no longer signal;
		// report it rather than spinning until the deadline in silence.
		return false, fmt.Sprintf("kill 0: %v", err)
	}

	if runtime.GOOS != "linux" {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		output, err := boundedCommand(ctx, requireTool(t, "ps"), "-p", strconv.Itoa(pid), "-o", "stat=").Output()
		if err != nil || strings.TrimSpace(string(output)) == "" {
			if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				return true, ""
			}

			return false, fmt.Sprintf("cannot read process state: %v", err)
		}

		state := strings.TrimSpace(string(output))
		return strings.HasPrefix(state, "Z"), "state " + state
	}

	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if errors.Is(err, os.ErrNotExist) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return true, ""
		}

		return false, "process exists but procfs state is unavailable"
	}
	if err != nil {
		t.Fatalf("read /proc/%d/stat: %v", pid, err)
	}

	if state := procState(t, pid, stat); state != "Z" {
		return false, "state " + state
	}

	return true, ""
}

// procState returns field 3 of /proc/<pid>/stat. Field 2 is the executable name
// in parentheses and may itself contain spaces or a ')', so the state is the
// first field after the last ')'.
func procState(t *testing.T, pid int, stat []byte) string {
	t.Helper()

	nameEnd := bytes.LastIndexByte(stat, ')')
	if nameEnd < 0 {
		t.Fatalf("unparseable /proc/%d/stat: %q", pid, stat)
	}

	fields := strings.Fields(string(stat[nameEnd+1:]))
	if len(fields) == 0 {
		t.Fatalf("unparseable /proc/%d/stat: %q", pid, stat)
	}

	return fields[0]
}

func readPID(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse pid %q: %v", data, err)
	}

	return pid
}
