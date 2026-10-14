// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

const (
	maxPanicValueBytes = 4 * 1024
	maxPanicStackBytes = 64 * 1024
	panicReportDirMode = 0o700
	panicReportMode    = 0o600
)

var errStateHomeNotAbsolute = errors.New("XDG_STATE_HOME is not absolute")

// runWithPanicBoundary turns an otherwise-unhandled panic into EX_SOFTWARE.
// Detailed panic data is written to a bounded private state file; stderr gets
// only stable public text, the report location, and a prefilled Codefloe URL.
//
//nolint:nonamedreturns // Deferred panic recovery must replace the return status.
func runWithPanicBoundary(runFunc func() int, stderr io.Writer) (exitCode int) {
	exitCode = int(domain.ExitCodeSoftware)

	defer func() {
		panicValue := recover()
		if panicValue == nil {
			return
		}

		_, _ = fmt.Fprintln(stderr, unexpectedInternalErrorMessage)

		reportAvailable := false

		if reportPath, err := persistPanicReport(panicValue, debug.Stack()); err == nil {
			_, _ = fmt.Fprintf(stderr, "Private diagnostic report: %q\n", reportPath)
			reportAvailable = true
		} else {
			_, _ = fmt.Fprintln(stderr, "Private diagnostic report unavailable")
		}

		_, _ = fmt.Fprintf(stderr, "Report this bug: %s\n", panicIssueURL(version, reportAvailable))
		exitCode = int(domain.ExitCodeSoftware)
	}()

	return runFunc()
}

func persistPanicReport(panicValue any, stack []byte) (string, error) {
	stateHome, err := panicStateHome()
	if err != nil {
		return "", err
	}

	reportDir := filepath.Join(stateHome, appName, "crash")
	reportName := fmt.Sprintf("panic-%s-%d.log", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())

	return writePrivatePanicReport(reportDir, reportName, boundedPanicReport(panicValue, stack))
}

func panicStateHome() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		if !filepath.IsAbs(stateHome) {
			return "", errStateHomeNotAbsolute
		}

		return stateHome, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".local", "state"), nil
}

func boundedPanicReport(panicValue any, stack []byte) []byte {
	var report bytes.Buffer

	_, _ = fmt.Fprintf(&report, "gommitlint panic report\nversion: %s\ngo: %s\nos/arch: %s/%s\npanic: %s\n\nstack:\n",
		version, runtime.Version(), runtime.GOOS, runtime.GOARCH, boundedPanicValue(panicValue))
	report.Write(stack[:min(len(stack), maxPanicStackBytes)])

	return report.Bytes()
}

//nolint:nonamedreturns // Deferred recovery sanitizes the value produced below.
func boundedPanicValue(value any) (text string) {
	text = fmt.Sprintf("<%T>", value)

	defer func() {
		if recover() != nil {
			text = fmt.Sprintf("<%T>", value)
		}

		if len(text) > maxPanicValueBytes {
			text = text[:maxPanicValueBytes]
		}

		text = strings.ToValidUTF8(text, "?")
	}()

	switch typed := value.(type) {
	case string:
		text = typed
	case error:
		text = typed.Error()
	default:
		text = fmt.Sprint(typed)
	}

	return text
}

func panicIssueURL(buildVersion string, reportAvailable bool) string {
	query := url.Values{}
	query.Set("title", "bug: unexpected internal error")

	body := fmt.Sprintf(`## What happened

Describe the command and what happened.

## Environment

- Gommitlint version: %s
- OS/architecture: %s/%s`, buildVersion, runtime.GOOS, runtime.GOARCH)

	if reportAvailable {
		body += `

## Diagnostic report

Attach the private diagnostic report path printed by gommitlint after reviewing it for sensitive data.`
	} else {
		body += `

## Diagnostic report

Gommitlint could not write a private diagnostic report. Include any relevant non-sensitive details manually.`
	}

	query.Set("body", body)

	return "https://codefloe.com/itiquette/gommitlint/issues/new?" + query.Encode()
}
