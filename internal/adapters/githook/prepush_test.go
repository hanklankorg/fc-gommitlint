// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package githook

import (
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestDecodePrePush_DecodesSHA1AndSHA256(t *testing.T) {
	t.Parallel()

	for _, width := range []int{40, 64} {
		t.Run(objectFormatName(width), func(t *testing.T) {
			t.Parallel()

			localOID := strings.Repeat("a", width)
			remoteOID := strings.Repeat("b", width)
			zeroOID := strings.Repeat("0", width)
			input := strings.Join([]string{
				"refs/heads/main " + localOID + " refs/heads/main " + remoteOID,
				"refs/tags/v1.0.0 " + localOID + " refs/tags/v1.0.0 " + zeroOID,
				"(delete) " + zeroOID + " refs/heads/old " + remoteOID,
			}, "\n") + "\n"

			updates, err := DecodePrePush([]string{"upstream", "ssh://example.invalid/repo"}, strings.NewReader(input))

			check.NoError(t, err)
			check.Len(t, updates, 3)
			check.Equal(t, "refs/heads/main", updates[0].LocalRef)
			check.Equal(t, localOID, updates[0].LocalOID)
			check.Equal(t, "refs/heads/main", updates[0].RemoteRef)
			check.Equal(t, remoteOID, updates[0].RemoteOID)
			check.False(t, updates[0].Delete)
			check.Equal(t, "refs/tags/v1.0.0", updates[1].LocalRef)
			check.Equal(t, zeroOID, updates[1].RemoteOID)
			check.True(t, updates[2].Delete)
			check.Equal(t, "(delete)", updates[2].LocalRef)
		})
	}
}

// TestDecodePrePush_EnforcesTheLineLimitOnBothSides replaces a one-sided
// oversized_line row that only ever proved too long is refused. A decoder whose
// real limit was half the declared one — or that refused every long line —
// passed that row just as well. A limit is only a limit if the last permitted
// length is also accepted.
//
// Writing it turned up an off-by-one, since fixed: bufio.Scanner treats the
// value given to Buffer as the maximum BUFFER size and needs room for the
// delimiter beside the token, so passing the limit unadjusted refused a line of
// exactly maxPrePushProtocolLine bytes. The old row used max+1, comfortably
// past the edge, so it never showed that max itself was refused too.
func TestDecodePrePush_EnforcesTheLineLimitOnBothSides(t *testing.T) {
	t.Parallel()

	// A well-formed update line of exactly the requested byte length, so that
	// length is the only variable between the two rows.
	buildLine := func(total int) string {
		oid := strings.Repeat("a", 40)
		refBudget := total - (3 + 2*len(oid))
		localRef := "refs/heads/" + strings.Repeat("l", refBudget/2-len("refs/heads/"))
		remoteRef := "refs/heads/" + strings.Repeat("r", refBudget-len(localRef)-len("refs/heads/"))

		return localRef + " " + oid + " " + remoteRef + " " + oid
	}

	t.Run("a line of exactly the limit is accepted", func(t *testing.T) {
		t.Parallel()

		line := buildLine(maxPrePushProtocolLine)
		check.Len(t, line, maxPrePushProtocolLine, "the fixture must sit exactly on the limit")

		updates, err := DecodePrePush([]string{"origin", "location"}, strings.NewReader(line+"\n"))

		check.NoError(t, err)
		check.Len(t, updates, 1)
	})

	t.Run("one byte past the limit is refused", func(t *testing.T) {
		t.Parallel()

		line := buildLine(maxPrePushProtocolLine + 1)
		check.Len(t, line, maxPrePushProtocolLine+1)

		_, err := DecodePrePush([]string{"origin", "location"}, strings.NewReader(line+"\n"))

		check.ErrorIs(t, err, ErrMalformedPrePushProtocol)
	})
}

// TestDecodePrePush_AcceptsAnEmptyUpdateStream covers the case Git actually
// produces when there is nothing to push: valid arguments and no lines at all.
// Nothing distinguished it from a failure, and it is the one input where
// returning zero updates is correct rather than suspicious.
func TestDecodePrePush_AcceptsAnEmptyUpdateStream(t *testing.T) {
	t.Parallel()

	updates, err := DecodePrePush([]string{"origin", "location"}, strings.NewReader(""))

	check.NoError(t, err)
	check.Empty(t, updates)
	check.NotNil(t, updates, "callers range over this; it must be an empty slice, not nil")
}

func TestDecodePrePush_RejectsMalformedProtocol(t *testing.T) {
	t.Parallel()

	sha1 := strings.Repeat("a", 40)
	zero := strings.Repeat("0", 40)
	sha256 := strings.Repeat("b", 64)
	tests := []struct {
		name  string
		args  []string
		input string
	}{
		{name: "missing_arguments", args: []string{"origin"}},
		{name: "empty_remote", args: []string{"", "location"}},
		{name: "blank_location", args: []string{"origin", " "}},
		{name: "wrong_field_count", args: []string{"origin", "location"}, input: "refs/heads/main " + sha1 + " refs/heads/main\n"},
		{name: "invalid_hex", args: []string{"origin", "location"}, input: "refs/heads/main " + strings.Repeat("g", 40) + " refs/heads/main " + sha1 + "\n"},
		{name: "uppercase_hex", args: []string{"origin", "location"}, input: "refs/heads/main " + strings.Repeat("A", 40) + " refs/heads/main " + sha1 + "\n"},
		{name: "mixed_object_formats", args: []string{"origin", "location"}, input: "refs/heads/main " + sha1 + " refs/heads/main " + sha256 + "\n"},
		{name: "mixed_stream_object_formats", args: []string{"origin", "location"}, input: "refs/heads/main " + sha1 + " refs/heads/main " + sha1 + "\nrefs/heads/next " + sha256 + " refs/heads/next " + sha256 + "\n"},
		{name: "invalid_local_ref", args: []string{"origin", "location"}, input: "main " + sha1 + " refs/heads/main " + sha1 + "\n"},
		{name: "invalid_remote_ref", args: []string{"origin", "location"}, input: "refs/heads/main " + sha1 + " main " + sha1 + "\n"},
		// Carries the refs/ prefix, so only Git's reference-name rules refuse it.
		{name: "malformed_local_ref_name", args: []string{"origin", "location"}, input: "refs/heads/a..b " + sha1 + " refs/heads/main " + sha1 + "\n"},
		{name: "zero_local_without_delete_ref", args: []string{"origin", "location"}, input: "refs/heads/main " + zero + " refs/heads/main " + sha1 + "\n"},
		{name: "delete_ref_with_nonzero_local", args: []string{"origin", "location"}, input: "(delete) " + sha1 + " refs/heads/main " + sha1 + "\n"},
		{name: "both_zero_deletion", args: []string{"origin", "location"}, input: "(delete) " + zero + " refs/heads/main " + zero + "\n"},
		{name: "multiple_spaces", args: []string{"origin", "location"}, input: "refs/heads/main  " + sha1 + " refs/heads/main " + sha1 + "\n"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodePrePush(testCase.args, strings.NewReader(testCase.input))

			check.ErrorIs(t, err, ErrMalformedPrePushProtocol)
		})
	}
}

func objectFormatName(width int) string {
	if width == 64 {
		return "sha256"
	}

	return "sha1"
}
