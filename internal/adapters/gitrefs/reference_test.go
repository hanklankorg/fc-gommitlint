// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitrefs

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const fullMainRef = "refs/heads/main"

func TestReferenceCandidates_ExpandShorthandsInPriorityOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reference string
		want      []string
	}{
		{
			name:      "fully_qualified_ref",
			reference: fullMainRef,
			want:      []string{fullMainRef},
		},
		{
			name:      "remotes_prefix",
			reference: "remotes/origin/main",
			want:      []string{"refs/remotes/origin/main"},
		},
		{
			name:      "remote_branch_shorthand",
			reference: "origin/main",
			want: []string{
				"refs/remotes/origin/main",
				"refs/heads/origin/main",
				"refs/tags/origin/main",
			},
		},
		{
			// The three namespaces are handled alike. This case used to be
			// asymmetric: only "remotes/" was recognised, "heads/" was merely
			// excluded from the remote-first branch and then prefixed a second
			// time (yielding refs/heads/heads/main and never refs/heads/main),
			// and "tags/" was not recognised at all.
			name:      "heads_namespace_is_qualified_directly",
			reference: "heads/main",
			want:      []string{fullMainRef},
		},
		{
			// The case the old asymmetry hurt most: a tag written this way was
			// probed as a remote first and never as refs/tags/v1.0 at all.
			name:      "tags_namespace_is_qualified_directly",
			reference: "tags/v1.0",
			want:      []string{"refs/tags/v1.0"},
		},
		{
			// The namespace check must anchor at the start. A remote branch
			// literally named "heads/main" under remote "origin" contains a
			// namespace segment but does not begin with one, and matching
			// anywhere would qualify it to refs/origin/heads/main — a ref that
			// cannot exist.
			name:      "a_namespace_appearing_later_in_the_path_is_not_a_prefix",
			reference: "origin/heads/main",
			want: []string{
				"refs/remotes/origin/heads/main",
				"refs/heads/origin/heads/main",
				"refs/tags/origin/heads/main",
			},
		},
		{
			// And it must not swallow a remote that merely starts with the
			// same letters. "tagsmith" is a plausible remote name.
			name:      "a_remote_whose_name_begins_like_a_namespace_is_still_a_shorthand",
			reference: "tagsmith/main",
			want: []string{
				"refs/remotes/tagsmith/main",
				"refs/heads/tagsmith/main",
				"refs/tags/tagsmith/main",
			},
		},
		{
			// The shorthand is prefixed whole: only the presence of a slash
			// decides, and every segment after the first travels along
			// unchanged. Nothing covered a reference with more than one slash.
			name:      "nested_remote_branch_keeps_its_remaining_segments",
			reference: "origin/feature/nested",
			want: []string{
				"refs/remotes/origin/feature/nested",
				"refs/heads/origin/feature/nested",
				"refs/tags/origin/feature/nested",
			},
		},
		{
			name:      "an_already_qualified_prefix_is_returned_untouched",
			reference: "remotes/origin/x/y",
			want:      []string{"refs/remotes/origin/x/y"},
		},
		{
			// The degenerate input. Not useful output, but it is what the
			// function returns and it must not panic or silently produce a
			// bare "refs/" that would match something unintended.
			name:      "empty_reference_expands_to_empty_names",
			reference: "",
			want: []string{
				"refs/heads/",
				"refs/tags/",
				"refs/remotes/",
			},
		},
		{
			name:      "simple_name",
			reference: "main",
			want: []string{
				"refs/heads/main",
				"refs/tags/main",
				"refs/remotes/main",
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want, ReferenceCandidates(testCase.reference))
		})
	}
}
