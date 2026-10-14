// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"crypto/sha256"
	"encoding/hex"
	iofs "io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
)

type testEnv map[string]string

func (e testEnv) GetEnv(key string) string {
	return e[key]
}

func TestSyncManagedSources(t *testing.T) {
	t.Parallel()

	remoteContent := []byte(`gommitlint:
  output: json
  message:
    subject:
      max_length: 88
`)
	sum := sha256.Sum256(remoteContent)
	digest := hex.EncodeToString(sum[:])

	t.Run("vendors managed source and updates inline metadata", func(t *testing.T) {
		t.Parallel()

		var receivedQuery string

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedQuery = request.URL.RawQuery
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configuredSource := server.URL + "/policy.yaml?download-token=direct-secret#private-fragment"
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: \"" + configuredSource + "\"\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))

		fixedNow := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
		result, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{
			HTTP: server.Client(),
			Now:  func() time.Time { return fixedNow },
		})
		check.NoError(t, err)
		check.Len(t, result.Items, 1)
		check.Equal(t, configuration.ManagedSourceSyncStatusSynced, result.Items[0].Status)

		vendoredContent, err := iofs.ReadFile(os.DirFS(tempDir), ".gommitlint.team.yaml")
		check.NoError(t, err)
		check.Equal(t, remoteContent, vendoredContent)

		reloaded, err := configuration.LoadConfigFromPathDetailed(configPath)
		check.NoError(t, err)
		check.Len(t, reloaded.Config.ManagedSources, 1)
		check.Equal(t, fixedNow.Format(time.RFC3339), reloaded.Config.ManagedSources[0].Resolved.FetchedAt)
		check.Equal(t, server.URL+"/policy.yaml", reloaded.Config.ManagedSources[0].Resolved.FinalURL)
		check.Equal(t, configuredSource, reloaded.Config.ManagedSources[0].Source)
		check.Equal(t, "download-token=direct-secret", receivedQuery)

		persistedConfig, err := os.ReadFile(configPath)
		check.NoError(t, err)
		check.NotContains(t, string(persistedConfig), "final_url: "+server.URL+"/policy.yaml?")
		check.NotContains(t, string(persistedConfig), "allow_fixup_commits")
	})

	t.Run("check mode detects stale vendored file", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: " + server.URL + "/policy.yaml\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))
		check.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gommitlint.team.yaml"), []byte("stale"), 0o600))

		_, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{CheckOnly: true, HTTP: server.Client()})
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrManagedSourceOutOfDate)
	})

	t.Run("dry run and check only leave a stale destination and the config unchanged", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		for _, testCase := range []struct {
			name       string
			options    configuration.SyncOptions
			wantErr    error
			wantStatus string
			wantWrite  bool
		}{
			{name: "dry run", options: configuration.SyncOptions{DryRun: true}, wantStatus: configuration.ManagedSourceSyncStatusChecked},
			{name: "check only", options: configuration.SyncOptions{CheckOnly: true}, wantErr: configuration.ErrManagedSourceOutOfDate},
			{name: "the control: a real sync rewrites both", wantStatus: configuration.ManagedSourceSyncStatusSynced, wantWrite: true},
		} {
			t.Run(testCase.name, func(t *testing.T) {
				tempDir := t.TempDir()
				configPath := filepath.Join(tempDir, ".gommitlint.yaml")
				destinationPath := filepath.Join(tempDir, ".gommitlint.team.yaml")
				configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: " + server.URL + "/policy.yaml\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
				check.NoError(t, os.WriteFile(configPath, configContent, 0o600))
				check.NoError(t, os.WriteFile(destinationPath, []byte("stale"), 0o600))

				options := testCase.options
				options.HTTP = server.Client()

				result, err := configuration.SyncManagedSources(t.Context(), configPath, options)
				if testCase.wantErr != nil {
					check.ErrorIs(t, err, testCase.wantErr)
				} else {
					check.NoError(t, err)
					check.Len(t, result.Items, 1)
					check.Equal(t, testCase.wantStatus, result.Items[0].Status)
				}

				destinationAfter, err := os.ReadFile(destinationPath)
				check.NoError(t, err)

				configAfter, err := os.ReadFile(configPath)
				check.NoError(t, err)

				if testCase.wantWrite {
					check.Equal(t, remoteContent, destinationAfter)
					check.NotEqual(t, configContent, configAfter)

					return
				}

				check.Equal(t, []byte("stale"), destinationAfter)
				check.Equal(t, configContent, configAfter)
			})
		}
	})

	t.Run("reports verified when vendored file and metadata are unchanged", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: " + server.URL + "/policy.yaml\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n      resolved:\n        final_url: " + server.URL + "/policy.yaml\n        revision: \"\"\n        fetched_at: \"2026-03-09T12:00:00Z\"\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))
		check.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gommitlint.team.yaml"), remoteContent, 0o600))

		result, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{
			HTTP: server.Client(),
			Now:  func() time.Time { return time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC) },
		})
		check.NoError(t, err)
		check.Len(t, result.Items, 1)
		check.Equal(t, configuration.ManagedSourceSyncStatusVerified, result.Items[0].Status)
	})

	t.Run("returns integrity mismatch details", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: " + server.URL + "/policy.yaml\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: '0000000000000000000000000000000000000000000000000000000000000000'\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))

		_, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{HTTP: server.Client()})
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrManagedSourceIntegrity)

		var integrityErr *configuration.ManagedSourceIntegrityError
		check.ErrorAs(t, err, &integrityErr)
		check.Equal(t, "team-policy", integrityErr.ID)
		check.Equal(t, digest, integrityErr.Actual)
	})

	t.Run("resolves managed source URL from environment", func(t *testing.T) {
		t.Parallel()

		var receivedQuery string

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			receivedQuery = request.URL.RawQuery
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: ${GOMMITLINT_POLICY_URL}\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))

		resolvedSource := strings.Replace(server.URL, "https://", "https://env-user:env-secret@", 1) + "/policy.yaml?env-token=private#env-fragment"
		result, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{
			Env:  testEnv{"GOMMITLINT_POLICY_URL": resolvedSource},
			HTTP: server.Client(),
		})
		check.NoError(t, err)
		check.Len(t, result.Items, 1)
		check.Equal(t, configuration.ManagedSourceSyncStatusSynced, result.Items[0].Status)
		check.Equal(t, "env-token=private", receivedQuery)

		reloaded, err := configuration.LoadConfigFromPathDetailed(configPath)
		check.NoError(t, err)
		check.Equal(t, "${GOMMITLINT_POLICY_URL}", reloaded.Config.ManagedSources[0].Source)
		check.Equal(t, server.URL+"/policy.yaml", reloaded.Config.ManagedSources[0].Resolved.FinalURL)

		persistedConfig, err := os.ReadFile(configPath)
		check.NoError(t, err)
		check.NotContains(t, string(persistedConfig), "env-secret")
		check.NotContains(t, string(persistedConfig), "env-token")
		check.NotContains(t, string(persistedConfig), "env-fragment")
	})

	t.Run("fails when managed source env var is missing", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: ${GOMMITLINT_POLICY_URL}\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))

		_, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{Env: testEnv{}})
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrManagedSourceEnvNotSet)

		var envErr *configuration.ManagedSourceEnvError
		check.ErrorAs(t, err, &envErr)
		check.Equal(t, "team-policy", envErr.ID)
		check.Equal(t, "GOMMITLINT_POLICY_URL", envErr.Variable)
	})

	t.Run("fails when managed source env var resolves to non-https URL", func(t *testing.T) {
		t.Parallel()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		configContent := []byte("gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: ${GOMMITLINT_POLICY_URL}\n      destination: .gommitlint.team.yaml\n      integrity:\n        sha256: " + digest + "\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))

		_, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{Env: testEnv{"GOMMITLINT_POLICY_URL": "http://example.test/policy.yaml"}})
		check.Error(t, err)
		check.ErrorIs(t, err, configuration.ErrManagedSourceInvalidURL)

		var urlErr *configuration.ManagedSourceURLError
		check.ErrorAs(t, err, &urlErr)
		check.Equal(t, "team-policy", urlErr.ID)
		check.Equal(t, "${GOMMITLINT_POLICY_URL}", urlErr.ConfiguredSource)
		check.Equal(t, "[REDACTED]", urlErr.ResolvedURL)
	})

	t.Run("later failure leaves every destination unchanged", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
			_, _ = writer.Write(remoteContent)
		}))
		defer server.Close()

		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, ".gommitlint.yaml")
		firstPath := filepath.Join(tempDir, "first.yaml")
		secondPath := filepath.Join(tempDir, "second.yaml")
		firstOriginal := []byte("gommitlint:\n  output: text\n")
		secondOriginal := []byte("gommitlint:\n  output: table\n")
		configContent := []byte("gommitlint:\n  managed_sources:\n" +
			"    - id: first\n      source: " + server.URL + "/first.yaml\n      destination: first.yaml\n      integrity:\n        sha256: " + digest + "\n" +
			"    - id: second\n      source: " + server.URL + "/second.yaml\n      destination: second.yaml\n      integrity:\n        sha256: '0000000000000000000000000000000000000000000000000000000000000000'\n")
		check.NoError(t, os.WriteFile(configPath, configContent, 0o600))
		check.NoError(t, os.WriteFile(firstPath, firstOriginal, 0o600))
		check.NoError(t, os.WriteFile(secondPath, secondOriginal, 0o600))

		_, err := configuration.SyncManagedSources(t.Context(), configPath, configuration.SyncOptions{HTTP: server.Client()})
		check.ErrorIs(t, err, configuration.ErrManagedSourceIntegrity)

		firstAfter, err := os.ReadFile(firstPath)
		check.NoError(t, err)
		check.Equal(t, firstOriginal, firstAfter)

		secondAfter, err := os.ReadFile(secondPath)
		check.NoError(t, err)
		check.Equal(t, secondOriginal, secondAfter)

		configAfter, err := os.ReadFile(configPath)
		check.NoError(t, err)
		check.Equal(t, configContent, configAfter)
	})
}
