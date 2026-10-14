// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// TestValidateCommitRules_UsesTheProvidedLogger tests that validation functions properly use logger.
// This tests actual behavior rather than mock implementation.
func TestValidateCommitRules_UsesTheProvidedLogger(t *testing.T) {
	t.Parallel()

	// Logger that tracks calls.
	loggerCalled := false
	logger := &trackingLogger{onLog: func() { loggerCalled = true }}

	// Use logger in actual validation scenario
	commit := Commit{Hash: testHashABC123, Subject: testString, Message: testString}
	rules := []CommitRule{} // Empty rules to keep test focused

	_ = ValidateCommitRules(commit, rules, logger)

	// Verify logger was actually used during validation
	check.True(t, loggerCalled, "logger should be called during validation")
}

// trackingLogger helps verify logger is actually used, not just passed around.
type trackingLogger struct {
	onLog func()
}

func (t *trackingLogger) Log(level string, msg string, args ...any) { t.doLog() }
func (t *trackingLogger) Trace(msg string, args ...any)             { t.doLog() }
func (t *trackingLogger) TraceFunc(fn func() (string, []any)) {
	fn()
	t.doLog()
}
func (t *trackingLogger) Debug(msg string, args ...any) { t.doLog() }
func (t *trackingLogger) Info(msg string, args ...any)  { t.doLog() }
func (t *trackingLogger) Warn(msg string, args ...any)  { t.doLog() }
func (t *trackingLogger) Error(msg string, args ...any) { t.doLog() }

func (t *trackingLogger) doLog() {
	if t.onLog != nil {
		t.onLog()
	}
}
