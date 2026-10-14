// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const testURLBare = "https://example.com/policy.yaml"

type failingHTTPDoer func(*http.Request) (*http.Response, error)

func (do failingHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

type classifiedTransportError struct {
	cause error
	url   string
}

func (e *classifiedTransportError) Error() string {
	return fmt.Sprintf("transport failed for %s: %v", e.url, e.cause)
}

func (e *classifiedTransportError) Unwrap() error {
	return e.cause
}

func TestValidateManagedSourceResolvedURL_SanitizesErrorMetadata(t *testing.T) {
	t.Parallel()

	const secret = "private-token"

	source := configTypes.ManagedSource{
		ID:     "team-policy",
		Source: "https://configured:" + secret + "@example.test/policy.yaml?token=" + secret + "#" + secret,
	}
	err := validateManagedSourceResolvedURL(source, "https://resolved:"+secret+"@example.test/%zz?token="+secret)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceInvalidURL)

	var urlErr *ManagedSourceURLError
	check.True(t, errors.As(err, &urlErr))
	check.Equal(t, "https://example.test/policy.yaml", urlErr.ConfiguredSource)
	check.Equal(t, "[REDACTED]", urlErr.ResolvedURL)
	check.NotContains(t, err.Error(), secret)
}

func TestSyncManagedSources_UnsetsEnvDerivedSourceAfterInvalidURL(t *testing.T) {
	t.Parallel()

	const sourceEnvVar = "GOMMITLINT_POLICY_URL"

	env := &recordingEnv{values: map[string]string{sourceEnvVar: "not-a-url"}}
	httpCalled := false
	client := failingHTTPDoer(func(*http.Request) (*http.Response, error) {
		httpCalled = true

		return nil, errors.New("unexpected HTTP request")
	})
	cfg := configTypes.Config{ManagedSources: []configTypes.ManagedSource{{
		ID:     "team-policy",
		Source: "${" + sourceEnvVar + "}",
	}}}

	_, err := syncManagedSources(t.Context(), ".gommitlint.yaml", nil, cfg, SyncOptions{Env: env, HTTP: client})
	check.ErrorIs(t, err, ErrManagedSourceInvalidURL)
	check.False(t, httpCalled)
	check.Equal(t, []string{sourceEnvVar}, env.unset)
	check.Equal(t, "", env.GetEnv(sourceEnvVar))
}

func TestFetchManagedSource_PreservesURLParseErrorChainWithoutRenderingSecrets(t *testing.T) {
	t.Parallel()

	const secret = "private-token"

	rawURL := "https://user:" + secret + "@example.test/%zz?token=" + secret

	_, revision, finalURL, err := fetchManagedSource(t.Context(), failingHTTPDoer(nil), rawURL, nil)
	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceFetchFailed)
	check.Equal(t, "", revision)
	check.Equal(t, "", finalURL)

	var urlErr *url.Error
	check.ErrorAs(t, err, &urlErr)
	check.NotContains(t, err.Error(), secret)
}

func TestCreateManagedSourceRequest_PreservesConstructionErrorWithoutRenderingSecrets(t *testing.T) {
	t.Parallel()

	const secret = "private-token"

	rawURL := "https://user:" + secret + "@example.test/policy.yaml?token=" + secret

	//lint:ignore SA1012 a nil context deliberately makes request construction fail
	_, err := createManagedSourceRequest(nil, rawURL, nil) //nolint:staticcheck // nil context deliberately exercises request construction failure.
	check.Error(t, err)
	check.ErrorIs(t, err, ErrManagedSourceFetchFailed)

	var fetchErr *managedSourceFetchError
	check.ErrorAs(t, err, &fetchErr)
	check.ErrorIs(t, err, fetchErr.cause)
	check.NotContains(t, err.Error(), secret)
}

func TestFetchManagedSource_PreservesDoErrorChainWithoutRenderingSecrets(t *testing.T) {
	t.Parallel()

	const secret = "private-token"

	rawURL := "https://user:" + secret + "@example.test/policy.yaml?token=" + secret + "#" + secret

	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()

			transportErr := &classifiedTransportError{cause: cause, url: rawURL}
			client := failingHTTPDoer(func(*http.Request) (*http.Response, error) {
				return nil, transportErr
			})

			_, _, _, err := fetchManagedSource(t.Context(), client, rawURL, nil)
			check.Error(t, err)
			check.ErrorIs(t, err, ErrManagedSourceFetchFailed)
			check.ErrorIs(t, err, cause)

			var classifiedErr *classifiedTransportError
			check.ErrorAs(t, err, &classifiedErr)
			check.Equal(t, transportErr, classifiedErr)
			check.Contains(t, err.Error(), "https://example.test/policy.yaml")
			check.NotContains(t, err.Error(), secret)
		})
	}
}
