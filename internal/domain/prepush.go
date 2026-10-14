// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

// PrePushUpdate is one decoded ref update from Git's pre-push protocol. It
// is a domain value because two adapters meet on it: the hook adapter
// decodes it from Git's stdin and the git adapter resolves the commits it
// introduces, while the pre-push use case only selects among them.
type PrePushUpdate struct {
	LocalRef  string
	LocalOID  string
	RemoteRef string
	RemoteOID string
	Delete    bool
}
