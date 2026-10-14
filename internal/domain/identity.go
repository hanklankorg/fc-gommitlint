// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Identity a committer or author identity with name and email.
type Identity struct {
	Name  string
	Email string
}

// NewIdentity creates a new identity with separate name and email.
func NewIdentity(name, email string) Identity {
	return Identity{
		Name:  strings.TrimSpace(name),
		Email: strings.TrimSpace(email),
	}
}

// NewIdentityFromString parses an identity string in the format "Name <email@example.com>".
func NewIdentityFromString(identity string) Identity {
	name, email := parseIdentityString(identity)

	return NewIdentity(name, email)
}

// String returns the identity in standard "Name <email>" format.
func (i Identity) String() string {
	// If both name and email are empty, return empty string
	if i.Name == "" && i.Email == "" {
		return ""
	}

	// If only name is empty, return just the email
	if i.Name == "" {
		return i.Email
	}

	// Normal case with both name and email
	return fmt.Sprintf("%s <%s>", i.Name, i.Email)
}

// Matches checks if this identity matches another one.
// Two identities match if they have the same email address.
func (i Identity) Matches(other Identity) bool {
	return i.Email != "" && strings.EqualFold(i.Email, other.Email)
}

// MatchesAny checks if this identity matches any in the provided slice.
func (i Identity) MatchesAny(identities []Identity) bool {
	return slices.ContainsFunc(identities, i.Matches)
}

// IsEmpty returns true if both name and email are empty.
func (i Identity) IsEmpty() bool {
	return i.Name == "" && i.Email == ""
}

// parseIdentityString extracts name and email from an identity string.
func parseIdentityString(identity string) (string, string) {
	identity = strings.TrimSpace(identity)

	start := strings.LastIndex(identity, "<")
	end := strings.LastIndex(identity, ">")

	if start != -1 && end != -1 && start < end {
		userName := strings.TrimSpace(identity[:start])
		userEmail := strings.TrimSpace(identity[start+1 : end])

		return userName, userEmail
	}

	// If not in standard format but contains @, treat as email
	if strings.Contains(identity, "@") {
		return "", identity
	}

	// Otherwise, treat as name
	return identity, ""
}
