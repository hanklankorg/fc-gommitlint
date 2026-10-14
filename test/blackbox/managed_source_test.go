//go:build blackbox

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package blackbox_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagedSourceSync_BlackBox(t *testing.T) {
	h := newHarness(t)
	ca := newLocalCA(t)
	caFile := h.trustCA(ca)

	policy := []byte("gommitlint:\n  rules:\n    disabled:\n      - signoff\n")
	otherPolicy := []byte("gommitlint:\n  output: plain\n")
	secret := "bb-managed-source-token-12345"

	var mu sync.Mutex
	hits := map[string]int{}
	authHeaders := []string{}

	mux := http.NewServeMux()
	mux.HandleFunc("/policy.yaml", func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		hits["policy"]++
		authHeaders = append(authHeaders, request.Header.Get("Authorization"))
		writer.Header().Set("ETag", "blackbox-policy")
		serveManagedSourceFixture(writer, request, policy)
	})
	mux.HandleFunc("/other.yaml", func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		hits["other"]++
		serveManagedSourceFixture(writer, request, otherPolicy)
	})

	serverURL := h.startTLSServer(ca, mux)
	workDir := h.caseDir("managed-source-sync")
	configPath := filepath.Join(workDir, ".gommitlint.yaml")
	teamDest := filepath.Join(workDir, ".gommitlint.team.yaml")
	otherDest := filepath.Join(workDir, ".gommitlint.other.yaml")

	config := fmt.Sprintf(`gommitlint:
  managed_sources:
    - id: team-policy
      source: %q
      destination: .gommitlint.team.yaml
      headers:
        Authorization: "Bearer ${GOMMITLINT_POLICY_TOKEN}"
      integrity:
        sha256: %s
    - id: other-policy
      source: %q
      destination: .gommitlint.other.yaml
      integrity:
        sha256: %s
`, serverURL+"/policy.yaml", sha256Hex(policy), serverURL+"/other.yaml", sha256Hex(otherPolicy))
	mustWriteFile(t, configPath, []byte(config), 0o600)

	env := []string{"SSL_CERT_FILE=" + caFile, "GOMMITLINT_POLICY_TOKEN=" + secret}
	beforeConfig := string(mustReadFile(t, configPath))

	dryRun := h.run(workDir, env, "--config", configPath, "config", "sync", "--dry-run")
	assertCode(t, dryRun.code, 0, dryRun)
	assertNotContains(t, dryRun.stdout+dryRun.stderr, secret, "dry-run output")
	if _, err := os.Stat(teamDest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run wrote destination %s", teamDest)
	}
	if got := string(mustReadFile(t, configPath)); got != beforeConfig {
		t.Fatalf("dry-run modified config\nbefore:\n%s\nafter:\n%s", beforeConfig, got)
	}

	syncTeam := h.run(workDir, env, "--config", configPath, "config", "sync", "--source", "team-policy")
	assertCode(t, syncTeam.code, 0, syncTeam)
	assertNotContains(t, syncTeam.stdout+syncTeam.stderr, secret, "sync output")
	if got := string(mustReadFile(t, teamDest)); got != string(policy) {
		t.Fatalf("vendored policy mismatch\nwant:\n%s\ngot:\n%s", policy, got)
	}
	if _, err := os.Stat(otherDest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("--source team-policy wrote unrelated destination %s", otherDest)
	}

	configAfterSync := string(mustReadFile(t, configPath))
	assertNotContains(t, configAfterSync, secret, "synced config")
	if !strings.Contains(configAfterSync, "final_url:") {
		t.Fatalf("synced config missing managed-source resolved metadata:\n%s", configAfterSync)
	}

	mu.Lock()
	policyHits := hits["policy"]
	otherHits := hits["other"]
	seenAuth := append([]string{}, authHeaders...)
	mu.Unlock()
	if policyHits < 2 {
		t.Fatalf("expected policy fixture to be fetched by dry-run and sync, got %d hits", policyHits)
	}
	if otherHits != 1 {
		t.Fatalf("expected other fixture to be fetched only by dry-run, got %d hits", otherHits)
	}
	if len(seenAuth) == 0 || seenAuth[len(seenAuth)-1] != "Bearer "+secret {
		t.Fatalf("managed-source Authorization header not received correctly: %#v", seenAuth)
	}

	checkCurrent := h.run(workDir, env, "--config", configPath, "config", "sync", "--check", "--source", "team-policy")
	assertCode(t, checkCurrent.code, 0, checkCurrent)

	mustWriteFile(t, teamDest, []byte("stale"), 0o600)
	checkStale := h.run(workDir, env, "--config", configPath, "config", "sync", "--check", "--source", "team-policy")
	assertNonZero(t, checkStale)
	assertNotContains(t, checkStale.stdout+checkStale.stderr, secret, "stale check output")

	unknown := h.run(workDir, env, "--config", configPath, "config", "sync", "--source", "missing-policy")
	assertNonZero(t, unknown)
}

func TestManagedSourceSecurity_BlackBox(t *testing.T) {
	h := newHarness(t)
	ca := newLocalCA(t)
	caFile := h.trustCA(ca)
	policy := []byte("gommitlint:\n  rules:\n    disabled:\n      - signoff\n")
	secret := "bb-secret-redaction-token-67890"
	env := []string{"SSL_CERT_FILE=" + caFile, "GOMMITLINT_POLICY_TOKEN=" + secret}

	t.Run("integrity_mismatch_does_not_write", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/policy.yaml", func(writer http.ResponseWriter, request *http.Request) {
			serveManagedSourceFixture(writer, request, policy)
		})
		serverURL := h.startTLSServer(ca, mux)
		workDir := h.caseDir("managed-source-integrity")
		configPath := filepath.Join(workDir, ".gommitlint.yaml")
		dest := filepath.Join(workDir, ".gommitlint.team.yaml")

		config := fmt.Sprintf(`gommitlint:
  managed_sources:
    - id: team-policy
      source: %q
      destination: .gommitlint.team.yaml
      integrity:
        sha256: 0000000000000000000000000000000000000000000000000000000000000000
`, serverURL+"/policy.yaml")
		mustWriteFile(t, configPath, []byte(config), 0o600)

		result := h.run(workDir, env, "--config", configPath, "config", "sync")
		assertNonZero(t, result)
		assertNotContains(t, result.stdout+result.stderr, secret, "integrity failure output")
		if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("integrity mismatch wrote destination %s", dest)
		}
	})

	t.Run("cross_host_redirect_strips_authorization", func(t *testing.T) {
		var mu sync.Mutex
		redirectedAuth := ""

		targetMux := http.NewServeMux()
		targetMux.HandleFunc("/policy.yaml", func(writer http.ResponseWriter, request *http.Request) {
			mu.Lock()
			redirectedAuth = request.Header.Get("Authorization")
			mu.Unlock()
			serveManagedSourceFixture(writer, request, policy)
		})
		targetURL := h.startTLSServer(ca, targetMux)

		originMux := http.NewServeMux()
		originMux.HandleFunc("/redirect.yaml", func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, targetURL+"/policy.yaml", http.StatusFound)
		})
		originURL := h.startTLSServer(ca, originMux)

		workDir := h.caseDir("managed-source-redirect")
		configPath := filepath.Join(workDir, ".gommitlint.yaml")
		config := fmt.Sprintf(`gommitlint:
  managed_sources:
    - id: team-policy
      source: %q
      destination: .gommitlint.team.yaml
      headers:
        Authorization: "Bearer ${GOMMITLINT_POLICY_TOKEN}"
      integrity:
        sha256: %s
`, originURL+"/redirect.yaml", sha256Hex(policy))
		mustWriteFile(t, configPath, []byte(config), 0o600)

		result := h.run(workDir, env, "--config", configPath, "config", "sync")
		assertNonZero(t, result)
		assertNotContains(t, result.stdout+result.stderr, secret, "redirect failure output")

		mu.Lock()
		gotAuth := redirectedAuth
		mu.Unlock()
		if gotAuth != "" {
			t.Fatalf("Authorization leaked across host redirect: %q", gotAuth)
		}
	})

	t.Run("http_source_with_headers_rejected_before_fetch", func(t *testing.T) {
		workDir := h.caseDir("managed-source-http-headers")
		configPath := filepath.Join(workDir, ".gommitlint.yaml")
		config := fmt.Sprintf(`gommitlint:
  managed_sources:
    - id: team-policy
      source: ${GOMMITLINT_POLICY_URL}
      destination: .gommitlint.team.yaml
      headers:
        Authorization: "Bearer ${GOMMITLINT_POLICY_TOKEN}"
      integrity:
        sha256: %s
`, sha256Hex(policy))
		mustWriteFile(t, configPath, []byte(config), 0o600)

		result := h.run(workDir, append(env, "GOMMITLINT_POLICY_URL=http://127.0.0.1:1/policy.yaml"), "--config", configPath, "config", "sync")
		assertNonZero(t, result)
		assertNotContains(t, result.stdout+result.stderr, secret, "http rejection output")
	})
}

func serveManagedSourceFixture(writer http.ResponseWriter, request *http.Request, content []byte) {
	writer.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	http.ServeContent(writer, request, "policy.yaml", time.Unix(0, 0), bytes.NewReader(content))
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:])
}
