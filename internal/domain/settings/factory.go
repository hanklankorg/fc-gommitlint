// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package settings

// NewBaseConfig creates a configuration with sensible field defaults but no rule opinions.
// Use NewDefault for the full product defaults including rule enable/disable lists.
func NewBaseConfig() Config {
	return Config{
		AllowFixupCommits: new(false),
		Message: MessageConfig{
			Subject: SubjectConfig{
				MaxLength:         new(DefaultSubjectMaxLength),
				Case:              new(DefaultSubjectCase),
				RequireImperative: new(false),
				ForbidEndings:     GetDefaultForbiddenEndings(),
			},
			Body: BodyConfig{
				Required:         new(false),
				MinLength:        new(0),
				MaxLineLength:    new(DefaultBodyMaxLineLength),
				AllowSignoffOnly: new(false),
				MinSignoffCount:  new(DefaultMinSignoffCount),
			},
		},
		Conventional: ConventionalConfig{
			AllowMultiScope:      new(false),
			DisallowBreaking:     new(false),
			MaxDescriptionLength: new(DefaultConventionalMaxDescLength),
			RequireScope:         new(false),
			Scopes:               []string{},
			Types:                GetDefaultConventionalTypes(),
		},
		CryptoSignature: CryptoSignatureConfig{
			AllowedSigners: []string{},
			KeyDirectory:   "",
			MinECBits:      new(DefaultMinECBits),
			MinRSABits:     new(DefaultMinRSABits),
			Required:       new(false),
			RequireGPG:     new(false),
			RequireSSH:     new(false),
			VerifyFormat:   new(false),
		},
		Identity: IdentityConfig{
			AllowedAuthors: []string{},
		},
		Repo: RepoConfig{
			MaxCommitsAhead: new(DefaultMaxCommitsAhead),
		},
		Limits: LimitsConfig{
			MaxCommitCount: new(DefaultMaxCommitCount),
			MaxKeyFiles:    new(DefaultMaxKeyFiles),
		},
		Jira: JiraConfig{
			ProjectPrefixes:      []string{},
			RequireInBody:        new(false),
			RequireInSubject:     new(false),
			IgnoreTicketPatterns: []string{},
		},
		Spell: SpellConfig{
			IgnoreWords: []string{},
			Locale:      new(DefaultSpellLocale),
		},
	}
}

// NewDefault returns the product defaults. Rule activation defaults are resolved
// from rule metadata when rules are built, so Rules contains only explicit user
// configuration.
func NewDefault() Config {
	cfg := NewBaseConfig()
	cfg.Message.Subject.RequireImperative = new(true)

	return cfg
}
