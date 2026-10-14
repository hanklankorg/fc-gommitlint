// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const sharedBodyMarker = "# gommitlint:shared-body-begin"

// TestSharedBodyDrift_CommitMsg ensures the commit-msg standalone and plugin
// variants share a byte-identical body after the marker. The variant headers
// can diverge (different docs, different argument-handling order) but the
// validate-and-report logic must stay in sync.
func TestSharedBodyDrift_CommitMsg(t *testing.T) {
	t.Parallel()
	assertSharedBodiesMatch(t, "commit-msg", CommitMsgHook, CommitMsgPlugin)
}

// TestSharedBodyDrift_PrePush mirrors TestSharedBodyDrift_CommitMsg for pre-push.
func TestSharedBodyDrift_PrePush(t *testing.T) {
	t.Parallel()
	assertSharedBodiesMatch(t, "pre-push", PrePushHook, PrePushPlugin)
}

func TestCommitMsgHooks_RenderFailureBeforePreservingExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		script          string
		args            []string
		stdin           string
		exitCode        int
		failureMessage  string
		remediationText string
	}{
		{
			name:            "commit_msg_standalone_classified_error",
			script:          CommitMsgHook,
			args:            []string{"COMMIT_EDITMSG"},
			exitCode:        78,
			failureMessage:  "commit-msg failed",
			remediationText: "Options:",
		},
		{
			name:            "commit_msg_plugin_signal_exit",
			script:          CommitMsgPlugin,
			args:            []string{"COMMIT_EDITMSG"},
			exitCode:        130,
			failureMessage:  "commit-msg failed",
			remediationText: "Options:",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			scriptPath := filepath.Join(tempDir, "hook.sh")
			check.NoError(t, os.WriteFile(scriptPath, []byte(testCase.script), 0o600))
			writeExecutable(t, filepath.Join(tempDir, "gommitlint"), `#!/bin/sh
printf 'captured finding\n'
printf 'captured diagnostic\n' >&2
exit "$GOMMITLINT_TEST_EXIT_CODE"
`)

			commandArgs := append([]string{scriptPath}, testCase.args...)
			cmd := exec.CommandContext(t.Context(), "/bin/sh", commandArgs...)
			cmd.Env = []string{
				"GOMMITLINT_TEST_EXIT_CODE=" + strconv.Itoa(testCase.exitCode),
				"NO_COLOR=1",
				"PATH=" + tempDir,
			}
			cmd.Stdin = strings.NewReader(testCase.stdin)

			var (
				stdout bytes.Buffer
				stderr bytes.Buffer
			)

			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := runCommand(cmd)

			var exitError *exec.ExitError
			check.ErrorAs(t, err, &exitError)
			check.Equal(t, testCase.exitCode, exitError.ExitCode())
			check.Equal(t, "", stdout.String(), "captured validation output must not leak to stdout")
			check.Contains(t, stderr.String(), testCase.failureMessage)
			check.Contains(t, stderr.String(), "captured finding")
			check.Contains(t, stderr.String(), "captured diagnostic")
			check.Contains(t, stderr.String(), testCase.remediationText)
		})
	}
}

func TestCommitMsgHooks_PreserveSuccessfulStderr(t *testing.T) {
	t.Parallel()

	for _, variant := range []struct {
		name   string
		script string
	}{
		{name: "standalone", script: CommitMsgHook},
		{name: "plugin", script: CommitMsgPlugin},
	} {
		t.Run(variant.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			scriptPath := filepath.Join(tempDir, "hook.sh")
			check.NoError(t, os.WriteFile(scriptPath, []byte(variant.script), 0o600))
			writeExecutable(t, filepath.Join(tempDir, "gommitlint"), `#!/bin/sh
printf 'successful report\n'
printf 'configuration warning\n' >&2
`)
			cmd := exec.CommandContext(t.Context(), "/bin/sh", scriptPath, "COMMIT_EDITMSG")
			cmd.Env = []string{"NO_COLOR=1", "PATH=" + tempDir}

			var stdout, stderr bytes.Buffer

			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			check.NoError(t, runCommand(cmd))
			check.Empty(t, stdout.String())
			check.Contains(t, stderr.String(), "configuration warning")
			check.Contains(t, stderr.String(), "commit-msg passed")
			check.NotContains(t, stderr.String(), "successful report")
		})
	}
}

func TestPrePushHooks_ForwardArgumentsStdinOutputAndStatus(t *testing.T) {
	t.Parallel()

	const prePushInput = "refs/heads/main 1111111111111111111111111111111111111111 refs/heads/main 2222222222222222222222222222222222222222\n"

	variants := []struct {
		name     string
		script   string
		exitCode int
	}{
		{name: "standalone", script: PrePushHook, exitCode: 77},
		{name: "plugin", script: PrePushPlugin, exitCode: 143},
	}

	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			scriptPath := filepath.Join(tempDir, "hook.sh")
			check.NoError(t, os.WriteFile(scriptPath, []byte(variant.script), 0o600))
			writeExecutable(t, filepath.Join(tempDir, "gommitlint"), `#!/bin/sh
printf 'args:'
for arg do
	printf ' <%s>' "$arg"
done
printf '\n'
while IFS= read -r line; do
	printf 'stdin:%s\n' "$line"
done
printf 'forwarded diagnostic\n' >&2
exit "$GOMMITLINT_TEST_EXIT_CODE"
`)

			cmd := exec.CommandContext(t.Context(), "/bin/sh", scriptPath, "upstream", "ssh://example.invalid/repo")
			cmd.Env = []string{
				"GOMMITLINT_TEST_EXIT_CODE=" + strconv.Itoa(variant.exitCode),
				"PATH=" + tempDir,
			}
			cmd.Stdin = strings.NewReader(prePushInput)

			var stdout, stderr bytes.Buffer

			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := runCommand(cmd)

			var exitError *exec.ExitError
			check.ErrorAs(t, err, &exitError)
			check.Equal(t, variant.exitCode, exitError.ExitCode())
			check.Equal(t, "args: <hook> <run> <pre-push> <upstream> <ssh://example.invalid/repo>\nstdin:"+strings.TrimSuffix(prePushInput, "\n")+"\n", stdout.String())
			check.Equal(t, "forwarded diagnostic\n", stderr.String())
		})
	}
}

func TestDispatcher_ReplaysPrePushInputToEveryPlugin(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dispatcherPath := filepath.Join(tempDir, "pre-push")
	pluginDir := dispatcherPath + ".d"
	check.NoError(t, os.Mkdir(pluginDir, 0o700))
	check.NoError(t, os.WriteFile(dispatcherPath, []byte(strings.ReplaceAll(DispatcherTemplate, "HOOK_TYPE", "pre-push")), 0o700))

	plugin := func(variable string) string {
		return "#!/bin/sh\n" +
			"capture=$" + variable + "\n" +
			"while IFS= read -r line; do printf '%s\\n' \"$line\"; done >\"$capture\"\n"
	}
	writeExecutable(t, filepath.Join(pluginDir, "10-first.sh"), plugin("CAPTURE_FIRST"))
	writeExecutable(t, filepath.Join(pluginDir, "20-second.sh"), plugin("CAPTURE_SECOND"))

	firstCapture := filepath.Join(tempDir, "first.input")
	secondCapture := filepath.Join(tempDir, "second.input")
	input := "first line\nsecond line\n"
	cmd := exec.CommandContext(t.Context(), "/bin/sh", dispatcherPath, "upstream", "ssh://example.invalid/repo")
	cmd.Env = []string{
		"CAPTURE_FIRST=" + firstCapture,
		"CAPTURE_SECOND=" + secondCapture,
		"PATH=" + os.Getenv("PATH"),
		"TMPDIR=" + tempDir,
	}
	cmd.Stdin = strings.NewReader(input)

	check.NoError(t, runCommand(cmd))

	first, err := os.ReadFile(firstCapture)
	check.NoError(t, err)
	second, err := os.ReadFile(secondCapture)
	check.NoError(t, err)
	check.Equal(t, input, string(first))
	check.Equal(t, input, string(second))
	assertNoDispatcherInputFiles(t, tempDir)
}

// TestDispatcher_RefusesWhenPrivateInputStorageCannotBeCreated is the
// behavioural check on the pre-push input snapshot: with TMPDIR unwritable
// the dispatcher stops before any plugin runs, with exit 1 and the storage
// refusal, instead of falling through to a later failure or replaying an
// empty stream. It needs a user that the read-only directory actually
// refuses, so it does not hold for root.
func TestDispatcher_RefusesWhenPrivateInputStorageCannotBeCreated(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dispatcherPath := filepath.Join(tempDir, "pre-push")
	pluginDir := dispatcherPath + ".d"
	check.NoError(t, os.Mkdir(pluginDir, 0o700))
	check.NoError(t, os.WriteFile(dispatcherPath, []byte(strings.ReplaceAll(DispatcherTemplate, "HOOK_TYPE", "pre-push")), 0o700))

	marker := filepath.Join(tempDir, "plugin-ran")
	writeExecutable(t, filepath.Join(pluginDir, "10-marker.sh"), "#!/bin/sh\n: >\"$MARKER\"\n")

	readOnlyTmp := filepath.Join(tempDir, "tmp")
	check.NoError(t, os.Mkdir(readOnlyTmp, 0o500))
	t.Cleanup(func() { _ = os.Chmod(readOnlyTmp, 0o700) })

	cmd := exec.CommandContext(t.Context(), "/bin/sh", dispatcherPath, "upstream", "location")
	cmd.Env = []string{"MARKER=" + marker, "PATH=" + os.Getenv("PATH"), "TMPDIR=" + readOnlyTmp}
	cmd.Stdin = strings.NewReader("updates\n")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := runCommand(cmd)

	var exitError *exec.ExitError
	check.ErrorAs(t, err, &exitError)
	check.Equal(t, 1, exitError.ExitCode())
	check.Contains(t, stderr.String(), "could not create secure pre-push input storage")
	check.NoFileExists(t, marker, "no plugin may run without the input snapshot")
	assertNoDispatcherInputFiles(t, readOnlyTmp)
}

func TestDispatcher_PreservesPluginFailureStatusAndCleansInput(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dispatcherPath := filepath.Join(tempDir, "pre-push")
	pluginDir := dispatcherPath + ".d"
	check.NoError(t, os.Mkdir(pluginDir, 0o700))
	check.NoError(t, os.WriteFile(dispatcherPath, []byte(strings.ReplaceAll(DispatcherTemplate, "HOOK_TYPE", "pre-push")), 0o700))
	writeExecutable(t, filepath.Join(pluginDir, "10-fails.sh"), "#!/bin/sh\nexit 23\n")

	cmd := exec.CommandContext(t.Context(), "/bin/sh", dispatcherPath, "upstream", "location")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + tempDir}
	cmd.Stdin = strings.NewReader("updates\n")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := runCommand(cmd)

	var exitError *exec.ExitError
	check.ErrorAs(t, err, &exitError)
	check.Equal(t, 23, exitError.ExitCode())
	check.Contains(t, stderr.String(), "10-fails.sh (status 23)")
	assertNoDispatcherInputFiles(t, tempDir)
}

func assertNoDispatcherInputFiles(t *testing.T, tempDir string) {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(tempDir, "gommitlint-pre-push.*"))
	check.NoError(t, err)
	check.Empty(t, matches)
}

func assertSharedBodiesMatch(t *testing.T, hookType, standalone, plugin string) {
	t.Helper()

	standaloneBody := sharedBody(t, hookType+" standalone", standalone)
	pluginBody := sharedBody(t, hookType+" plugin", plugin)

	check.Equal(t, standaloneBody, pluginBody,
		"%s hook variants have drifted below %q. Re-align both files so the post-marker bodies are byte-identical.",
		hookType, sharedBodyMarker)
}

func sharedBody(t *testing.T, label, script string) string {
	t.Helper()

	index := strings.Index(script, sharedBodyMarker)
	check.NotEqual(t, -1, index, "missing %q marker in %s", sharedBodyMarker, label)

	// Skip past the marker line itself; everything after the next newline is the
	// region under the byte-equality guarantee.
	newline := strings.IndexByte(script[index:], '\n')
	check.NotEqual(t, -1, newline, "marker in %s is not followed by a newline", label)

	return script[index+newline+1:]
}

// forkMu keeps every stub write and every child start apart. A child forked
// while another subtest still holds a stub open for writing inherits that
// descriptor until it execs, and a hook running the stub in that window gets
// ETXTBSY (status 126 instead of the stub's exit code); the parallel subtests
// hit it a few times per hundred runs under load.
var forkMu sync.Mutex

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()

	forkMu.Lock()
	defer forkMu.Unlock()

	check.NoError(t, os.WriteFile(path, []byte(content), 0o700))
}

// runCommand is cmd.Run with the fork under forkMu; the wait is outside it.
func runCommand(cmd *exec.Cmd) error {
	forkMu.Lock()
	err := cmd.Start()
	forkMu.Unlock()

	if err != nil {
		return err
	}

	return cmd.Wait()
}

// TestHookScripts_MissingBinaryFailsClosed covers the three resolutions when
// gommitlint is not on PATH: the installed-binary fallback runs it, the explicit
// opt-out lets the operation through, and otherwise the hook refuses.
func TestHookScripts_MissingBinaryFailsClosed(t *testing.T) {
	t.Parallel()

	const prePushInput = "refs/heads/main 1111111111111111111111111111111111111111 refs/heads/main 2222222222222222222222222222222222222222\n"

	scripts := []struct {
		name       string
		script     string
		args       []string
		stdin      string
		printsPass bool
	}{
		{name: "commit_msg", script: CommitMsgHook, args: []string{"COMMIT_EDITMSG"}, printsPass: true},
		{name: "commit_msg_plugin", script: CommitMsgPlugin, args: []string{"COMMIT_EDITMSG"}, printsPass: true},
		{name: "pre_push", script: PrePushHook, stdin: prePushInput},
		{name: "pre_push_plugin", script: PrePushPlugin, stdin: prePushInput},
	}

	for _, variant := range scripts {
		t.Run(variant.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			// PATH holds only the commit-msg parsing tools and no gommitlint,
			// regardless of what the host has installed.
			gitDir := filepath.Join(tempDir, "tool-bin")
			check.NoError(t, os.Mkdir(gitDir, 0o700))

			for _, tool := range []string{"grep", "sed", "head"} {
				toolPath, err := exec.LookPath(tool)
				check.NoError(t, err)
				check.NoError(t, os.Symlink(toolPath, filepath.Join(gitDir, tool)))
			}

			run := func(script string, env ...string) (int, string) {
				scriptPath := filepath.Join(tempDir, "hook-"+strconv.Itoa(len(script))+".sh")
				check.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o600))
				cmd := exec.CommandContext(t.Context(), "/bin/sh", append([]string{scriptPath}, variant.args...)...)
				cmd.Env = append([]string{"PATH=" + gitDir, "NO_COLOR=1"}, env...)
				cmd.Stdin = strings.NewReader(variant.stdin)

				var stderr bytes.Buffer

				cmd.Stderr = &stderr

				err := runCommand(cmd)
				if err == nil {
					return 0, stderr.String()
				}

				var exitError *exec.ExitError
				check.ErrorAs(t, err, &exitError)

				return exitError.ExitCode(), stderr.String()
			}

			// 1. No gommitlint anywhere: refuse with guidance.
			code, stderr := run(variant.script)
			check.Equal(t, 1, code)
			check.Contains(t, stderr, "gommitlint not found in PATH")
			check.Contains(t, stderr, "GOMMITLINT_HOOK_MISSING=skip")

			// 2. Explicit opt-out lets the operation through.
			code, stderr = run(variant.script, "GOMMITLINT_HOOK_MISSING=skip")
			check.Equal(t, 0, code)
			check.Contains(t, stderr, "skipping")

			// 3. The installed binary recorded at install time is used when PATH lacks one.
			// The directory name carries every character that is special inside
			// a double-quoted shell string; an unescaped backtick would run `y`.
			installedDir := filepath.Join(tempDir, "installed dir \"q\" $x `y`")
			check.NoError(t, os.MkdirAll(installedDir, 0o700))
			// A script, not a symlink to /bin/true: on a BusyBox system true is
			// a multi-call applet and refuses to run under another name.
			installed := filepath.Join(installedDir, "gommitlint")
			check.NoError(t, os.WriteFile(installed, []byte("#!/bin/sh\nexit 0\n"), 0o700))
			code, stderr = run(WithInstalledBinary(variant.script, installed))
			check.Equal(t, 0, code, stderr)

			if variant.printsPass {
				check.Contains(t, stderr, "passed")
			}
		})
	}
}

func TestWithInstalledBinary_EscapesShellSpecials(t *testing.T) {
	t.Parallel()

	script := `BIN="` + InstalledBinaryPlaceholder + `"`
	check.Equal(t, "BIN=\"/opt/we\\\"ird\\\\\\$dir/\\`x\\`/gommitlint\"", WithInstalledBinary(script, "/opt/we\"ird\\$dir/`x`/gommitlint"))
	check.Equal(t, `BIN=""`, WithInstalledBinary(script, ""))
}
