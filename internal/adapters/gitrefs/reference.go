// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitrefs

import "strings"

// ReferenceCandidates returns fully qualified refs to probe for a user-provided ref.
func ReferenceCandidates(reference string) []string {
	if strings.HasPrefix(reference, "refs/") {
		return []string{reference}
	}

	// A reference that already names one of Git's namespaces is qualified
	// directly. All three are treated alike: "heads/main", "tags/v1.0" and
	// "remotes/origin/main" each become that ref and nothing else.
	//
	// Only "remotes/" used to be handled here. "heads/" was merely excluded
	// from the remote-first branch below and then prefixed a second time, so
	// "heads/main" probed refs/heads/heads/main and never refs/heads/main;
	// "tags/" was not recognised at all, so "tags/v1.0" was probed as a remote
	// first and never as refs/tags/v1.0.
	for _, namespace := range []string{"heads/", "tags/", "remotes/"} {
		if strings.HasPrefix(reference, namespace) {
			return []string{"refs/" + reference}
		}
	}

	// Anything else containing a slash is a remote-branch shorthand such as
	// "origin/main", where the first segment names a remote.
	if strings.Contains(reference, "/") {
		return []string{
			"refs/remotes/" + reference,
			"refs/heads/" + reference,
			"refs/tags/" + reference,
		}
	}

	return []string{
		"refs/heads/" + reference,
		"refs/tags/" + reference,
		"refs/remotes/" + reference,
	}
}
