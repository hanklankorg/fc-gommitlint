// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// The production writer intentionally refuses symlinked state ancestors. Give
// fixtures a physical root, including on macOS where /var aliases /private/var.
func panicTestDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	check.NoError(t, err)

	return path
}

func TestRunWithPanicBoundary_WritesSafePublicAndPrivateBoundedReport(t *testing.T) {
	stateHome := panicTestDirectory(t)
	t.Setenv("XDG_STATE_HOME", stateHome)

	const privateValue = "private panic token"

	var stderr bytes.Buffer

	exitCode := runWithPanicBoundary(func() int {
		panic(privateValue)
	}, &stderr)

	check.Equal(t, int(domain.ExitCodeSoftware), exitCode)
	check.Contains(t, stderr.String(), unexpectedInternalErrorMessage)
	check.Contains(t, stderr.String(), "https://codefloe.com/itiquette/gommitlint/issues/new?")
	check.NotContains(t, stderr.String(), privateValue)

	reportDir := filepath.Join(stateHome, appName, "crash")
	entries, err := os.ReadDir(reportDir)
	check.NoError(t, err)
	check.Len(t, entries, 1)

	reportPath := filepath.Join(reportDir, entries[0].Name())
	info, err := os.Stat(reportPath)
	check.NoError(t, err)
	check.Equal(t, os.FileMode(panicReportMode), info.Mode().Perm())
	check.LessOrEqual(t, info.Size(), int64(maxPanicValueBytes+maxPanicStackBytes+1024))

	content, err := os.ReadFile(reportPath)
	check.NoError(t, err)
	check.Contains(t, string(content), "panic: "+privateValue+"\n")
	// The report is only useful with the goroutine dump behind the stack header.
	check.Contains(t, string(content), "\nstack:\ngoroutine ")
}

// TestBoundedPanicReport_TruncatesValueAndStackAtTheirLimits pins both edges
// of each cap: the last permitted byte survives and the first byte past it is
// cut. The limits are stated as the literal byte counts the constants promise.
func TestBoundedPanicReport_TruncatesValueAndStackAtTheirLimits(t *testing.T) {
	t.Parallel()

	const (
		valueLimit = 4 * 1024
		stackLimit = 64 * 1024
	)

	tests := []struct {
		name     string
		valueLen int
		stackLen int
	}{
		{name: "at the limits", valueLen: valueLimit, stackLen: stackLimit},
		{name: "one byte past the limits", valueLen: valueLimit + 1, stackLen: stackLimit + 1},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			report := string(boundedPanicReport(
				strings.Repeat("v", testCase.valueLen),
				[]byte(strings.Repeat("s", testCase.stackLen)),
			))

			_, afterHeader, found := strings.Cut(report, "\npanic: ")
			check.True(t, found, "report must carry the panic header")

			value, stack, found := strings.Cut(afterHeader, "\n\nstack:\n")
			check.True(t, found, "report must carry the stack header")

			check.Equal(t, strings.Repeat("v", min(testCase.valueLen, valueLimit)), value)
			check.Equal(t, strings.Repeat("s", min(testCase.stackLen, stackLimit)), stack)
		})
	}
}

func TestRunWithPanicBoundary_ReportsUnavailableDiagnostic(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative-state")

	var stderr bytes.Buffer

	exitCode := runWithPanicBoundary(func() int {
		panic("private panic token")
	}, &stderr)

	check.Equal(t, int(domain.ExitCodeSoftware), exitCode)
	check.Contains(t, stderr.String(), "Private diagnostic report unavailable")
	check.NotContains(t, stderr.String(), "Private diagnostic report: \"")

	_, issueURL, found := strings.Cut(stderr.String(), "Report this bug: ")
	check.True(t, found)

	parsed, err := url.Parse(strings.TrimSpace(issueURL))
	check.NoError(t, err)
	check.Contains(t, parsed.Query().Get("body"), "could not write a private diagnostic report")
	check.NotContains(t, parsed.Query().Get("body"), "report path printed by gommitlint")
}

func TestPanicIssueURL_TailorsDiagnosticInstructionsToAvailability(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                string
		reportAvailable     bool
		expectedInstruction string
		unexpectedText      string
	}{
		{
			name:                "report available",
			reportAvailable:     true,
			expectedInstruction: "Attach the private diagnostic report path printed by gommitlint",
			unexpectedText:      "could not write a private diagnostic report",
		},
		{
			name:                "report unavailable",
			reportAvailable:     false,
			expectedInstruction: "could not write a private diagnostic report",
			unexpectedText:      "report path printed by gommitlint",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := url.Parse(panicIssueURL("1.2.3", testCase.reportAvailable))
			check.NoError(t, err)
			check.Equal(t, "codefloe.com", parsed.Host)
			check.Equal(t, "/itiquette/gommitlint/issues/new", parsed.Path)
			check.Contains(t, parsed.Query().Get("title"), "unexpected internal error")
			check.Contains(t, parsed.Query().Get("body"), "1.2.3")
			check.Contains(t, parsed.Query().Get("body"), testCase.expectedInstruction)
			check.NotContains(t, parsed.Query().Get("body"), testCase.unexpectedText)
		})
	}
}

func TestPersistPanicReport_RejectsSymlinkedParent(t *testing.T) {
	stateRoot := panicTestDirectory(t)
	realState := filepath.Join(stateRoot, "real-state")
	check.NoError(t, os.Mkdir(realState, 0o700))

	symlinkedState := filepath.Join(stateRoot, "state")
	check.NoError(t, os.Symlink(realState, symlinkedState))
	t.Setenv("XDG_STATE_HOME", symlinkedState)

	_, err := persistPanicReport("private", []byte("stack"))
	check.Error(t, err)

	entries, readErr := os.ReadDir(realState)
	check.NoError(t, readErr)
	check.Empty(t, entries)
}
