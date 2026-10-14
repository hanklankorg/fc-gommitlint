//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFlagCoherence_GlobalFlagsApplyAndHelpStaysPerCommand(t *testing.T) {
	h := newHarness(t)
	repo := createGitRepoWithCommit(t, h, "flag-coherence", "feat: add flag coherence")
	workDir := h.caseDir("flag-coherence-outside")
	configPath := filepath.Join(workDir, "gommitlint.yaml")
	mustWriteFile(t, configPath, []byte(`gommitlint:
  rules:
    disabled:
      - signoff
      - spell
      - cryptosignature
      - branchahead
`), 0o600)

	for _, testCase := range []struct {
		name string
		args []string
	}{
		{name: "validate", args: []string{"--repo-path", repo, "--config", configPath, "validate", "--format", "json"}},
		{name: "config_show", args: []string{"--repo-path", repo, "--config", configPath, "config", "show", "--format", "json"}},
		{name: "config_check", args: []string{"--repo-path", repo, "--config", configPath, "config", "check", "--format", "json"}},
		{name: "hook_status", args: []string{"--repo-path", repo, "--config", configPath, "hook", "status"}},
		{name: "doctor", args: []string{"--repo-path", repo, "--config", configPath, "doctor"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := h.run(workDir, nil, testCase.args...)
			assertCode(t, result.code, 0, result)
		})
	}

	for _, testCase := range []struct {
		name     string
		args     []string
		want     []string
		wantNone []string
	}{
		{name: "validate_help", args: []string{"validate", "--help"}, want: []string{"--message-file", "--count", "--rule", "docs/modules/ROOT/pages/reference/commands.adoc#user-content-_gommitlint_validate"}, wantNone: []string{"--preset"}},
		{name: "config_show_help", args: []string{"config", "show", "--help"}, want: []string{"--format"}, wantNone: []string{"--message-file", "--force"}},
		{name: "config_check_help", args: []string{"config", "check", "--help"}, want: []string{"--format"}, wantNone: []string{"--message-file", "--force"}},
		{name: "hook_status_help", args: []string{"hook", "status", "--help"}, want: []string{"status"}, wantNone: []string{"--message-file", "--force"}},
		{name: "doctor_help", args: []string{"doctor", "--help"}, want: []string{"doctor"}, wantNone: []string{"--message-file", "--format"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			result := h.run(workDir, nil, testCase.args...)
			assertCode(t, result.code, 0, result)
			for _, want := range testCase.want {
				if !strings.Contains(result.stdout, want) {
					t.Fatalf("help output missing %q:\n%s", want, result.stdout)
				}
			}
			for _, unwanted := range testCase.wantNone {
				if strings.Contains(result.stdout, unwanted) {
					t.Fatalf("help output unexpectedly contains %q:\n%s", unwanted, result.stdout)
				}
			}
		})
	}
}
