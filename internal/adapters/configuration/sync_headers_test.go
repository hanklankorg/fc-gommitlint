// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Test fixture constants. Hoisted to keep golangci's goconst happy and to
// make the intent (this is the env var name we test under / the header we
// exercise) obvious at the call sites. testURLBare is defined alongside
// the other shared URL fixtures in sync_url_strip_test.go.
const (
	testTokenEnvVar        = "TOKEN"
	testAuthHeader         = "Authorization"
	testAutomaticHeaderErr = "set automatically by the HTTP client"
	testSourceID           = "test"
)

// expandManagedSourceTemplate is the env-var substitution applied to header
// values. These tests lock in the small contract: substitute when ${VAR}
// resolves, error when it doesn't, leave literal strings alone. The
// resolved value comes back as []byte so the resolver-owned copy can be
// zeroed after use — tests compare against string equivalents for clarity.
func TestExpandManagedSourceTemplate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		value        string
		env          map[string]string
		want         string
		wantConsumed []string
		wantErr      bool
	}{
		{
			name:         "literal string passes through",
			value:        "Bearer static-value",
			want:         "Bearer static-value",
			wantConsumed: nil,
		},
		{
			name:         "single env var resolves",
			value:        "Bearer ${TOKEN}",
			env:          map[string]string{testTokenEnvVar: "ghp_xxxxxx"},
			want:         "Bearer ghp_xxxxxx",
			wantConsumed: []string{testTokenEnvVar},
		},
		{
			name:         "two env vars in one value",
			value:        "${SCHEME} ${TOKEN}",
			env:          map[string]string{"SCHEME": "Bearer", testTokenEnvVar: "abc123"},
			want:         "Bearer abc123",
			wantConsumed: []string{"SCHEME", testTokenEnvVar},
		},
		{
			name:    "env var unset is an error",
			value:   "Bearer ${MISSING}",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:         "consumed env vars survive a later placeholder error",
			value:        "${TOKEN} ${MISSING}",
			env:          map[string]string{testTokenEnvVar: "abc123"},
			wantConsumed: []string{testTokenEnvVar},
			wantErr:      true,
		},
		{
			name:    "env var with invalid name shape is rejected",
			value:   "Bearer ${not-a-valid-name}",
			env:     map[string]string{"not-a-valid-name": "x"},
			wantErr: true,
		},
		{
			name:  "unterminated placeholder is left as literal",
			value: "Bearer ${TOKEN incomplete",
			env:   map[string]string{},
			want:  "Bearer ${TOKEN incomplete",
		},
		{
			name:  "empty value passes through",
			value: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, consumed, err := expandManagedSourceTemplate(tc.value, testEnvMap(tc.env), "test-source", "TestHeader")
			check.Equal(t, tc.wantConsumed, consumed)

			if tc.wantErr {
				check.Error(t, err)

				return
			}

			check.NoError(t, err)
			check.Equal(t, tc.want, string(got))
		})
	}
}

func TestSyncManagedSources_UnsetsConsumedEnvOnHeaderResolutionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string]string
	}{
		{
			name: "same_template_failure",
			headers: map[string]string{
				"Authorization": "Bearer ${TOKEN} ${MISSING}",
			},
		},
		{
			name: "later_header_failure",
			headers: map[string]string{
				"A-Authorization": "Bearer ${TOKEN}",
				"B-Missing":       "${MISSING}",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := &recordingEnv{values: map[string]string{testTokenEnvVar: "secret-value"}}
			cfg := configTypes.Config{ManagedSources: []configTypes.ManagedSource{{
				ID:      testSourceID,
				Source:  "https://example.test/policy.yaml",
				Headers: tc.headers,
			}}}

			_, err := syncManagedSources(t.Context(), ".gommitlint.yaml", nil, cfg, SyncOptions{Env: env})
			check.Error(t, err)
			check.ErrorIs(t, err, ErrManagedSourceEnvNotSet)

			var envErr *ManagedSourceEnvError
			check.ErrorAs(t, err, &envErr)
			check.Equal(t, "MISSING", envErr.Variable)
			check.Equal(t, []string{testTokenEnvVar}, env.unset)
			check.Equal(t, "", env.GetEnv(testTokenEnvVar))
		})
	}
}

// A completed sync scrubs every variable it read, the URL's and the headers',
// and a variable two headers share is unset once.
func TestSyncManagedSources_UnsetsConsumedEnvAfterSuccessfulSync(t *testing.T) {
	t.Parallel()

	remoteContent := []byte("gommitlint:\n  output: json\n")
	sum := sha256.Sum256(remoteContent)
	digest := hex.EncodeToString(sum[:])

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- httptest fake server returning hard-coded test config bytes.
		_, _ = w.Write(remoteContent)
	}))
	defer srv.Close()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, ".gommitlint.yaml")
	configContent := "gommitlint:\n  managed_sources:\n    - id: team-policy\n      source: ${POLICY_URL}\n" +
		"      destination: policy.yaml\n      headers:\n        Authorization: Bearer ${TOKEN}\n" +
		"        X-Backup-Token: ${TOKEN}\n      integrity:\n        sha256: " + digest + "\n"
	check.NoError(t, os.WriteFile(configPath, []byte(configContent), 0o600))

	env := &recordingEnv{values: map[string]string{
		"POLICY_URL":    srv.URL + "/policy.yaml",
		testTokenEnvVar: "secret-value",
	}}

	result, err := SyncManagedSources(t.Context(), configPath, SyncOptions{Env: env, HTTP: srv.Client()})
	check.NoError(t, err)
	check.Len(t, result.Items, 1)
	check.Equal(t, ManagedSourceSyncStatusSynced, result.Items[0].Status)

	check.Equal(t, []string{"POLICY_URL", testTokenEnvVar}, env.unset)
	check.Equal(t, "", env.GetEnv("POLICY_URL"))
	check.Equal(t, "", env.GetEnv(testTokenEnvVar))
}

type testEnvMap map[string]string

func (m testEnvMap) GetEnv(key string) string {
	return m[key]
}

// TestFetchManagedSource_HeadersForwarded confirms the request that reaches
// the server carries the resolved headers (with ${ENV} substituted).
func TestFetchManagedSource_HeadersForwarded(t *testing.T) {
	t.Parallel()

	var receivedAuth string

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get(testAuthHeader)
		_, _ = w.Write([]byte("gommitlint: {}\n"))
	}))
	defer srv.Close()

	client := srv.Client() // TLS-aware client; CheckRedirect unset → use default

	source := configTypes.ManagedSource{
		ID:     testSourceID,
		Source: srv.URL,
		Headers: map[string]string{
			testAuthHeader: "Bearer ${TOKEN}",
		},
	}

	headers, consumed, err := resolveManagedSourceHeaders(source, testEnvMap{testTokenEnvVar: "secret-value"})
	check.NoError(t, err)
	check.Equal(t, []string{testTokenEnvVar}, consumed)

	//nolint:dogsled // intentional: this test asserts a side effect (header arrival, not return values).
	_, _, _, err = fetchManagedSource(t.Context(), client, srv.URL, headers)
	check.NoError(t, err)

	check.Equal(t, "Bearer secret-value", receivedAuth)

	// createManagedSourceRequest zeroes each []byte after Header.Set —
	// confirm the resolver-owned buffer no longer holds the token.
	for _, b := range headers {
		for _, c := range b {
			check.Zero(t, c, "resolver-owned header bytes must be zeroed after fetch")
		}
	}
}

// TestFetchManagedSource_HeadersRequireHTTPS confirms we refuse to send
// any header over plaintext HTTP. Token leakage on the wire is exactly the
// failure mode we're guarding against.
func TestFetchManagedSource_HeadersRequireHTTPS(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("gommitlint: {}\n"))
	}))
	defer srv.Close()

	headers := map[string][]byte{testAuthHeader: []byte("Bearer x")}

	//nolint:dogsled // intentional: this test asserts on the error only.
	_, _, _, err := fetchManagedSource(t.Context(), srv.Client(), srv.URL, headers)
	check.Error(t, err)
	check.Contains(t, err.Error(), "headers require https")
}

// TestFetchManagedSource_CrossHostRedirectRejected simulates an attacker-
// controlled redirect and confirms no request reaches its target.
func TestFetchManagedSource_CrossHostRedirectRejected(t *testing.T) {
	t.Parallel()

	var targetReached bool

	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetReached = true
		_, _ = w.Write([]byte("gommitlint: {}\n"))
	}))
	defer target.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	// Build a client that trusts both servers' self-signed certs and uses
	// our CheckRedirect hook.
	client := &http.Client{
		Transport:     origin.Client().Transport,
		CheckRedirect: validateManagedSourceRedirect,
	}

	headers := map[string][]byte{
		"Private-Token": []byte("private-should-not-leak"),
		"X-Token":       []byte("x-should-not-leak"),
	}

	//nolint:dogsled // intentional: this test asserts the redirect error and target side effect.
	_, _, _, err := fetchManagedSource(t.Context(), client, origin.URL, headers)

	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceUnexpectedHost)
	check.False(t, targetReached, "cross-host redirect target must not receive a request")
}

func TestFetchManagedSource_SchemeDowngradeRedirectRejected(t *testing.T) {
	t.Parallel()

	var targetReached bool

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetReached = true
		_, _ = w.Write([]byte("gommitlint: {}\n"))
	}))
	defer target.Close()

	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	client := origin.Client()
	client.CheckRedirect = validateManagedSourceRedirect

	//nolint:dogsled // intentional: this test asserts the redirect error and target side effect.
	_, _, _, err := fetchManagedSource(t.Context(), client, origin.URL, nil)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceFetchFailed)
	check.ErrorIs(t, err, ErrManagedSourceInvalidURL)
	check.False(t, targetReached, "HTTP redirect target must not receive a request")
}

// TestFetchManagedSource_SameHostRedirectKeepsHeaders confirms safe redirects
// retain arbitrary configured headers and complete successfully.
func TestFetchManagedSource_SameHostRedirectKeepsHeaders(t *testing.T) {
	t.Parallel()

	var redirectedHeaders http.Header

	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})

	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		redirectedHeaders = r.Header.Clone()
		_, _ = w.Write([]byte("gommitlint: {}\n"))
	})

	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	client := &http.Client{
		Transport:     srv.Client().Transport,
		CheckRedirect: validateManagedSourceRedirect,
	}

	headers := map[string][]byte{
		"Private-Token": []byte("private-keep-on-same-host"),
		"X-Token":       []byte("x-keep-on-same-host"),
	}

	_, finalURL, _, err := fetchManagedSource(t.Context(), client, srv.URL+"/old?origin-token=private#origin-fragment", headers)
	check.NoError(t, err)
	check.Equal(t, "private-keep-on-same-host", redirectedHeaders.Get("Private-Token"))
	check.Equal(t, "x-keep-on-same-host", redirectedHeaders.Get("X-Token"))
	check.Equal(t, srv.URL+"/new", finalURL)
}

func TestValidateManagedSourceRedirect_PreservesRedirectLimit(t *testing.T) {
	t.Parallel()

	via := make([]*http.Request, 10)
	for index := range via {
		via[index] = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/policy.yaml", nil)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.test/policy.yaml", nil)

	err := validateManagedSourceRedirect(req, via)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceFetchFailed)
	check.Contains(t, err.Error(), "stopped after 10 redirects")
}

// TestResolveManagedSourceHeaders_EmptyReturnsNil verifies the empty-input
// shortcut: no headers configured → nil, not an empty map. Lets callers
// avoid the len() == 0 boilerplate.
func TestResolveManagedSourceHeaders_EmptyReturnsNil(t *testing.T) {
	t.Parallel()

	source := configTypes.ManagedSource{ID: testSourceID}
	got, consumed, err := resolveManagedSourceHeaders(source, testEnvMap{})
	check.NoError(t, err)
	check.Nil(t, got)
	check.Nil(t, consumed)
}

// TestUnsetSecretEnvVars_OptInViaEnvUnsetter verifies that an EnvLookup
// implementing EnvUnsetter has its UnsetEnv called for each name, in order,
// and a plain EnvLookup (no UnsetEnv method) is silently skipped — so
// test injections never mutate the real OS environment.
func TestUnsetSecretEnvVars_OptInViaEnvUnsetter(t *testing.T) {
	t.Parallel()

	t.Run("plain EnvLookup is skipped", func(t *testing.T) {
		t.Parallel()

		// testEnvMap implements GetEnv but NOT UnsetEnv. The function
		// must noop — confirmed by the lack of any panic or test failure.
		unsetSecretEnvVars(testEnvMap{"TOKEN": "x"}, []string{"TOKEN"})
	})

	t.Run("EnvUnsetter receives every name", func(t *testing.T) {
		t.Parallel()

		env := &recordingEnv{values: map[string]string{"A": "1", "B": "2"}}
		unsetSecretEnvVars(env, []string{"A", "B"})
		check.Equal(t, []string{"A", "B"}, env.unset)
	})
}

// TestDedupeEnvNames covers the small helper that keeps the unset call
// once per variable even when multiple sources reference the same token.
func TestDedupeEnvNames(t *testing.T) {
	t.Parallel()

	check.Nil(t, dedupeEnvNames(nil))
	check.Equal(t, []string{"X"}, dedupeEnvNames([]string{"X"}))
	check.Equal(t, []string{"A", "B"}, dedupeEnvNames([]string{"A", "B", "A", "B"}))
	check.Equal(t, []string{"A", "B", "C"}, dedupeEnvNames([]string{"A", "B", "A", "C", "B"}))
}

// recordingEnv captures UnsetEnv calls without changing the process environment.
type recordingEnv struct {
	values map[string]string
	unset  []string
}

func (r *recordingEnv) GetEnv(key string) string {
	return r.values[key]
}

func (r *recordingEnv) UnsetEnv(key string) {
	r.unset = append(r.unset, key)
	delete(r.values, key)
}

// TestValidateManagedSourceHeaders_Forbidden checks the validation layer
// rejects automatically-set headers and HTTP-smuggling header names.
func TestValidateManagedSourceHeaders_Forbidden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		headers map[string]string
		wantErr string
	}{
		{
			name:    "Host header rejected",
			headers: map[string]string{"Host": "attacker.example"},
			wantErr: testAutomaticHeaderErr,
		},
		{
			name:    "Content-Length header rejected (case-insensitive)",
			headers: map[string]string{"content-length": "0"},
			wantErr: testAutomaticHeaderErr,
		},
		{
			name:    "Transfer-Encoding rejected",
			headers: map[string]string{"Transfer-Encoding": "chunked"},
			wantErr: testAutomaticHeaderErr,
		},
		{
			name:    "Connection rejected",
			headers: map[string]string{"Connection": "close"},
			wantErr: testAutomaticHeaderErr,
		},
		{
			name:    "empty name rejected",
			headers: map[string]string{"": "value"},
			wantErr: "empty header name",
		},
		{
			name:    "CR in name rejected",
			headers: map[string]string{"X-Custom\rInjection": "v"},
			wantErr: "contains CR or LF",
		},
		{
			name:    "LF in name rejected",
			headers: map[string]string{"X-Custom\nInjection": "v"},
			wantErr: "contains CR or LF",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			source := configTypes.ManagedSource{
				ID:      testSourceID,
				Source:  testURLBare,
				Headers: tc.headers,
			}

			errs := configTypes.ValidateManagedSources([]configTypes.ManagedSource{source})

			var found bool

			for _, msg := range errs {
				if strings.Contains(msg, tc.wantErr) {
					found = true

					break
				}
			}

			check.True(t, found, "expected validation error containing %q, got %v", tc.wantErr, errs)
		})
	}
}

// TestValidateManagedSourceHeaders_HTTPSRequired confirms that adding
// headers to a non-HTTPS literal source URL is a validation error.
func TestValidateManagedSourceHeaders_HTTPSRequired(t *testing.T) {
	t.Parallel()

	source := configTypes.ManagedSource{
		ID:      testSourceID,
		Source:  "http://example.com/policy.yaml",
		Headers: map[string]string{testAuthHeader: "Bearer x"},
	}

	errs := configTypes.ValidateManagedSources([]configTypes.ManagedSource{source})

	var found bool

	for _, msg := range errs {
		if strings.Contains(msg, "not https") {
			found = true

			break
		}
	}

	check.True(t, found, "expected HTTPS-required error, got %v", errs)
}

// TestValidateManagedSourceHeaders_AllowsEnvVarSource confirms that
// ${ENV_VAR} sources skip the literal HTTPS check at validation time;
// the resolved URL is checked at fetch time instead.
func TestValidateManagedSourceHeaders_AllowsEnvVarSource(t *testing.T) {
	t.Parallel()

	source := configTypes.ManagedSource{
		ID:      testSourceID,
		Source:  "${POLICY_URL}",
		Headers: map[string]string{"Authorization": "Bearer x"},
	}

	errs := configTypes.ValidateManagedSources([]configTypes.ManagedSource{source})

	for _, msg := range errs {
		check.NotContains(t, msg, "not https",
			"env-var sources defer the HTTPS check to fetch time")
	}
}
