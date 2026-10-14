// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package filesystem

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestReadWithLimit_EnforcesBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		content   string
		maxSize   int64
		want      string
		wantError bool
	}{
		{name: "below limit", content: "abc", maxSize: 4, want: "abc"},
		{name: "at limit", content: "abcd", maxSize: 4, want: "abcd"},
		{name: "above limit", content: "abcde", maxSize: 4, wantError: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			content, err := ReadWithLimit(strings.NewReader(testCase.content), testCase.maxSize)
			if testCase.wantError {
				check.ErrorIs(t, err, ErrFileTooLarge)
				check.Empty(t, content)

				return
			}

			check.NoError(t, err)
			check.Equal(t, testCase.want, string(content))
		})
	}
}
