// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package domain_test

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

const (
	identityJohnDoe        = "John Doe"
	identityJohnEmail      = "john@example.com"
	identityJohnFormatted  = "John Doe <john@example.com>"
	identityTrimWhitespace = "Trim whitespace"
)

func TestIdentity_FormatsNameAndEmailCorrectly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		inputName  string
		inputEmail string
		wantName   string
		wantEmail  string
		wantString string
		wantEmpty  bool
	}{
		{
			name:       "Both name and email",
			inputName:  identityJohnDoe,
			inputEmail: identityJohnEmail,
			wantName:   identityJohnDoe,
			wantEmail:  identityJohnEmail,
			wantString: identityJohnFormatted,
			wantEmpty:  false,
		},
		{
			name:       "Email only",
			inputName:  "",
			inputEmail: identityJohnEmail,
			wantName:   "",
			wantEmail:  identityJohnEmail,
			wantString: identityJohnEmail,
			wantEmpty:  false,
		},
		{
			name:       "Name only",
			inputName:  identityJohnDoe,
			inputEmail: "",
			wantName:   identityJohnDoe,
			wantEmail:  "",
			wantString: "John Doe <>",
			wantEmpty:  false,
		},
		{
			name:       "Empty identity",
			inputName:  "",
			inputEmail: "",
			wantName:   "",
			wantEmail:  "",
			wantString: "",
			wantEmpty:  true,
		},
		{
			name:       identityTrimWhitespace,
			inputName:  "  John Doe  ",
			inputEmail: "  john@example.com  ",
			wantName:   identityJohnDoe,
			wantEmail:  identityJohnEmail,
			wantString: identityJohnFormatted,
			wantEmpty:  false,
		},
		{
			name:       "Whitespace-only input is empty after trimming",
			inputName:  "   ",
			inputEmail: "  \t ",
			wantName:   "",
			wantEmail:  "",
			wantString: "",
			wantEmpty:  true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			identity := domain.NewIdentity(testCase.inputName, testCase.inputEmail)

			check.Equal(t, testCase.wantName, identity.Name)
			check.Equal(t, testCase.wantEmail, identity.Email)
			check.Equal(t, testCase.wantString, identity.String())
			check.Equal(t, testCase.wantEmpty, identity.IsEmpty())
		})
	}
}

func TestNewIdentityFromString_ParsesEachForm(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantName  string
		wantEmail string
	}{
		{
			name:      "Standard format",
			input:     identityJohnFormatted,
			wantName:  identityJohnDoe,
			wantEmail: identityJohnEmail,
		},
		{
			name:      "Email only with angle brackets",
			input:     "<" + identityJohnEmail + ">",
			wantName:  "",
			wantEmail: identityJohnEmail,
		},
		{
			name:      "Email only without brackets",
			input:     identityJohnEmail,
			wantName:  "",
			wantEmail: identityJohnEmail,
		},
		{
			name:      "Name only",
			input:     identityJohnDoe,
			wantName:  identityJohnDoe,
			wantEmail: "",
		},
		{
			name:      "With extra whitespace",
			input:     "  John Doe   <  john@example.com  >  ",
			wantName:  identityJohnDoe,
			wantEmail: identityJohnEmail,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			identity := domain.NewIdentityFromString(testCase.input)

			check.Equal(t, testCase.wantName, identity.Name)
			check.Equal(t, testCase.wantEmail, identity.Email)
		})
	}
}

func TestIdentity_MatchesByEmailOnly(t *testing.T) {
	t.Parallel()

	identity1 := domain.NewIdentity(identityJohnDoe, identityJohnEmail)
	identity2 := domain.NewIdentity("John Smith", identityJohnEmail)
	identity3 := domain.NewIdentity("Jane Doe", "jane@example.com")
	emptyIdentity := domain.NewIdentity("", "")

	check.True(t, identity1.Matches(identity2), "Identities with same email should match")
	check.False(t, identity1.Matches(identity3), "Identities with different emails should not match")
	check.False(t, identity1.Matches(emptyIdentity), "Identity should not match empty identity")
	check.False(t, emptyIdentity.Matches(identity1), "Empty identity should not match any identity")
}

func TestIdentity_MatchesAnyScansList(t *testing.T) {
	t.Parallel()

	identity1 := domain.NewIdentity(identityJohnDoe, identityJohnEmail)
	identities := []domain.Identity{
		domain.NewIdentity("Alice", "alice@example.com"),
		domain.NewIdentity("Bob", "bob@example.com"),
		domain.NewIdentity("John Smith", identityJohnEmail),
	}

	check.True(t, identity1.MatchesAny(identities), "Should match one of the identities")

	noMatchIdentities := []domain.Identity{
		domain.NewIdentity("Alice", "alice@example.com"),
		domain.NewIdentity("Bob", "bob@example.com"),
	}

	check.False(t, identity1.MatchesAny(noMatchIdentities), "Should not match any identity")
}
