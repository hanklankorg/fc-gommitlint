// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"net/http"
	"testing"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestWithSyncOptionDefaults_Timeout(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		options     SyncOptions
		wantTimeout time.Duration
	}{
		{name: "zero timeout selects the default", options: SyncOptions{}, wantTimeout: DefaultSyncTimeout},
		{name: "negative timeout selects the default", options: SyncOptions{Timeout: -time.Second}, wantTimeout: DefaultSyncTimeout},
		{name: "explicit timeout bounds the default client", options: SyncOptions{Timeout: 3 * time.Second}, wantTimeout: 3 * time.Second},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			options := withSyncOptionDefaults(testCase.options)

			check.Equal(t, testCase.wantTimeout, options.Timeout)

			client, ok := options.HTTP.(*http.Client)
			check.True(t, ok, "default HTTP doer should be an *http.Client")
			check.Equal(t, testCase.wantTimeout, client.Timeout)
		})
	}
}

func TestWithSyncOptionDefaults_KeepsInjectedHTTPClient(t *testing.T) {
	t.Parallel()

	injected := &http.Client{Timeout: time.Minute}

	options := withSyncOptionDefaults(SyncOptions{HTTP: injected, Timeout: time.Second})

	check.Equal(t, injected, options.HTTP)
	check.Equal(t, time.Minute, injected.Timeout)
}
