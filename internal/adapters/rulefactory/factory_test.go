// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rulefactory

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	domainrules "codefloe.com/itiquette/gommitlint/internal/domain/rules"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const testCheckIDGL401 = "GL401"

func TestIsRepositoryOnlyRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ruleName string
		expected bool
	}{
		{
			name:     "branchahead_is_repository_only",
			ruleName: rulecatalog.BranchAhead,
			expected: true,
		},
		{
			name:     "subject_is_not_repository_only",
			ruleName: rulecatalog.Subject,
			expected: false,
		},
		{
			name:     "conventional_is_not_repository_only",
			ruleName: rulecatalog.Conventional,
			expected: false,
		},
		{
			name:     "signoff_is_not_repository_only",
			ruleName: rulecatalog.SignOff,
			expected: false,
		},
		{
			name:     "unknown_rule_is_not_repository_only",
			ruleName: "unknownrule",
			expected: false,
		},
		{
			name:     "empty_string_is_not_repository_only",
			ruleName: "",
			expected: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := IsRepositoryOnlyRule(testCase.ruleName)
			check.Equal(t, testCase.expected, result)
		})
	}
}

// mockLogger implements domain.Logger for testing.
type mockLogger struct{}

func (m *mockLogger) Log(_ string, _ string, _ ...any)   {}
func (m *mockLogger) Trace(_ string, _ ...any)           {}
func (m *mockLogger) TraceFunc(_ func() (string, []any)) {}
func (m *mockLogger) Debug(_ string, _ ...any)           {}
func (m *mockLogger) Info(_ string, _ ...any)            {}
func (m *mockLogger) Warn(_ string, _ ...any)            {}
func (m *mockLogger) Error(_ string, _ ...any)           {}

// mockSpellChecker implements rules.SpellChecker for testing.
type mockSpellChecker struct{}

func (m *mockSpellChecker) CheckText(_ string) []domain.Misspelling {
	return nil
}

func ruleNames(rules []domain.CommitRule) []string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		names = append(names, rule.Name())
	}

	return names
}

func TestFactory_CreateCommitRulesFiltered_SkipsRepositoryOnlyRules(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Rules: settings.RulesConfig{
			Enabled: []string{rulecatalog.BranchAhead, rulecatalog.Subject, rulecatalog.Conventional},
		},
	}

	factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
	names := ruleNames(factory.CreateCommitRulesFiltered(nil))

	check.NotContains(t, names, rulecatalog.BranchAhead, "should not contain repository-only rule")
	check.Contains(t, names, rulecatalog.Subject, "should contain subject rule")
	check.Contains(t, names, rulecatalog.Conventional, "should contain conventional rule")
}

func TestFactory_CreateRepositoryRulesFiltered_CreatesBranchAhead(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Rules: settings.RulesConfig{
			Enabled: []string{rulecatalog.BranchAhead},
		},
	}

	factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
	rules := factory.CreateRepositoryRulesFiltered(nil)

	check.Len(t, rules, 1, "should create exactly one repository rule")
	check.Equal(t, rulecatalog.BranchAhead, rules[0].Name(), "should be BranchAhead rule")
}

func TestFactory_CreateCommitRulesFiltered_UsesResolvedConventionalDependency(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Rules: settings.RulesConfig{
			Enabled:  []string{rulecatalog.Subject, rulecatalog.Conventional},
			Disabled: []string{rulecatalog.Conventional},
		},
	}

	factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
	rules := factory.CreateCommitRulesFiltered(nil)

	var subjectRule domain.CommitRule

	for _, rule := range rules {
		if rule.Name() == rulecatalog.Subject {
			subjectRule = rule
		}
	}

	check.NotNil(t, subjectRule)
	check.NotContains(t, ruleNames(rules), rulecatalog.Conventional)

	errs := subjectRule.Validate(domain.ParseCommitMessage("feat: add thing"))
	check.NotEmpty(t, errs)
	check.Equal(t, diagnostics.ReasonWrongCaseSentence, errs[0].Diagnostic.Reason)
}

// The --rule filter narrows which rules run, not what the configuration says.
// Subject takes its case default from whether conventional is enabled in the
// resolved configuration, so "--rule subject" on a conventional repository
// must still judge "feat: Add thing" as a conventional subject.
func TestFactory_CreateCommitRulesFiltered_RuleFilterKeepsConventionalAwareness(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		disabled   []string
		wantReason diagnostics.Reason
	}{
		{name: "conventional enabled: the description must be lowercase", wantReason: diagnostics.ReasonWrongCaseLower},
		{name: "conventional disabled: the subject must be sentence case", disabled: []string{rulecatalog.Conventional}, wantReason: diagnostics.ReasonWrongCaseSentence},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Rules: settings.RulesConfig{
					Enabled:  []string{rulecatalog.Subject, rulecatalog.Conventional},
					Disabled: testCase.disabled,
				},
			}

			factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
			rules := factory.CreateCommitRulesFiltered([]string{rulecatalog.Subject})

			check.Equal(t, []string{rulecatalog.Subject}, ruleNames(rules))

			for _, rule := range rules {
				reasons := make([]diagnostics.Reason, 0, 1)
				for _, err := range rule.Validate(domain.ParseCommitMessage("feat: Add thing")) {
					reasons = append(reasons, err.Diagnostic.Reason)
				}

				check.Equal(t, []diagnostics.Reason{testCase.wantReason}, reasons)
			}
		})
	}
}

// TestFactory_CreateCommitRulesFiltered_DisabledCheckFiltersErrors runs the
// same over-long subject twice, once with GL102 disabled and once without.
//
// The disabled row alone could not tell "the check was filtered out" apart from
// "the check never fired here": a subject rule built with no checks at all, or
// one that ignored MaxLength, produces the same empty result and passes. The
// enabled row is what gives the empty one its meaning — it proves this exact
// config and this exact commit do produce GL102 when nothing filters it.
func TestFactory_CreateCommitRulesFiltered_DisabledCheckFiltersErrors(t *testing.T) {
	t.Parallel()

	const overLongSubject = "this subject is very long"

	for _, testCase := range []struct {
		name     string
		disabled []string
		wantIDs  []string
	}{
		{name: "GL102 disabled filters its error", disabled: []string{"GL102"}, wantIDs: nil},
		{name: "the control: GL102 fires when nothing filters it", disabled: nil, wantIDs: []string{"GL102"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Message: settings.MessageConfig{
					Subject: settings.SubjectConfig{
						MaxLength: new(10),
						Case:      new(domainrules.CaseAny),
					},
				},
				Rules:  settings.RulesConfigForEnabled(rulecatalog.Subject),
				Checks: settings.ChecksConfig{Disabled: testCase.disabled},
			}

			factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
			rules := factory.CreateCommitRulesFiltered(nil)

			check.Contains(t, ruleNames(rules), rulecatalog.Subject)

			for _, rule := range rules {
				if rule.Name() != rulecatalog.Subject {
					continue
				}

				errs := rule.Validate(domain.ParseCommitMessage(overLongSubject))
				gotIDs := make([]string, 0, len(errs))

				for _, err := range errs {
					gotIDs = append(gotIDs, string(err.Diagnostic.ID))
				}

				check.Equal(t, testCase.wantIDs, nilIfEmpty(gotIDs))

				return
			}

			check.Failf(t, "subject rule missing", "expected subject rule")
		})
	}
}

// stubCommitRule returns fixed errors under a catalogued rule name, so the check
// wrapper has known checks to filter against.
type stubCommitRule struct {
	name string
	errs []domain.ValidationError
}

func (r stubCommitRule) Name() string                                      { return r.name }
func (r stubCommitRule) Validate(_ domain.Commit) []domain.ValidationError { return r.errs }

// TestWrapCommitRuleWithChecks_KeepsErrorsWithoutDiagnosticID pins the check
// filter's exception: an error carrying no diagnostic ID belongs to no check, so
// no checks.disabled entry can name it and it must pass through. The disabled
// GL101 error in the same result is the control showing the filter is active.
func TestWrapCommitRuleWithChecks_KeepsErrorsWithoutDiagnosticID(t *testing.T) {
	t.Parallel()

	withoutID := domain.ValidationError{Rule: rulecatalog.Subject, Message: "no diagnostic id"}
	tooLong := domain.New(rulecatalog.Subject, diagnostics.ReasonSubjectTooLong, "too long")
	emptySubject := domain.New(rulecatalog.Subject, diagnostics.ReasonEmptySubject, "empty")
	check.Equal(t, diagnostics.ID("GL101"), emptySubject.Diagnostic.ID, "the control must be the check that is disabled")

	cfg := settings.Config{
		Rules:  settings.RulesConfigForEnabled(rulecatalog.Subject),
		Checks: settings.ChecksConfig{Disabled: []string{"GL101"}},
	}
	stub := stubCommitRule{name: rulecatalog.Subject, errs: []domain.ValidationError{withoutID, tooLong, emptySubject}}

	wrapped, ok := wrapCommitRuleWithChecks(stub, cfg, true)
	check.True(t, ok, "a rule with checks left must be kept")

	check.Equal(t, []domain.ValidationError{withoutID, tooLong}, wrapped.Validate(domain.Commit{}))
}

func nilIfEmpty(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}

	return ids
}

func TestFactory_CreateCommitRulesFiltered_SignoffChecksAreIndependent(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		disabled   string
		wantReason diagnostics.Reason
	}{
		{name: "missing check disabled", disabled: "GL421", wantReason: diagnostics.ReasonInvalidSignoffFormat},
		{name: "format check disabled", disabled: "GL422", wantReason: diagnostics.ReasonMissingSignoff},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.NewDefault()
			cfg.Rules = settings.RulesConfigForEnabled(rulecatalog.SignOff)
			cfg.Checks.Disabled = []string{testCase.disabled}
			factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")

			for _, rule := range factory.CreateCommitRulesFiltered(nil) {
				if rule.Name() != rulecatalog.SignOff {
					continue
				}

				errs := rule.Validate(domain.ParseCommitMessage("feat: test\n\nSigned-off-by: Invalid"))
				check.Len(t, errs, 1)
				check.Equal(t, testCase.wantReason, errs[0].Diagnostic.Reason)

				return
			}

			check.Failf(t, "signoff rule missing", "expected signoff rule")
		})
	}
}

// TestFactory_AllChecksDisabledRemovesRule pins that a rule whose every check is
// disabled is not built at all, for commit and repository rules alike. Kept
// unwrapped, it would report every check the configuration disabled.
func TestFactory_AllChecksDisabledRemovesRule(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		disabled    []string
		wantPresent bool
	}{
		{
			name:     "every signoff and branchahead check disabled",
			disabled: []string{"GL421", "GL422", "GL423", "GL424", "GL701", "GL702", "GL703", "GL704", "GL705"},
		},
		{
			name:        "the control: one check left keeps each rule",
			disabled:    []string{"GL421", "GL422", "GL423", "GL701", "GL702", "GL703", "GL704"},
			wantPresent: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Rules:  settings.RulesConfigForEnabled(rulecatalog.SignOff, rulecatalog.BranchAhead),
				Checks: settings.ChecksConfig{Disabled: testCase.disabled},
			}

			factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
			commitNames := ruleNames(factory.CreateCommitRulesFiltered(nil))
			repoRules := factory.CreateRepositoryRulesFiltered(nil)

			if testCase.wantPresent {
				check.Contains(t, commitNames, rulecatalog.SignOff)
				check.Len(t, repoRules, 1)

				return
			}

			check.NotContains(t, commitNames, rulecatalog.SignOff)
			check.Empty(t, repoRules)
		})
	}
}

func TestFactory_CreateCommitRulesFiltered_CheckEnabledActivatesParentRule(t *testing.T) {
	t.Parallel()

	cfg := settings.Config{
		Checks: settings.ChecksConfig{Enabled: []string{testCheckIDGL401}},
	}

	factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
	rules := factory.CreateCommitRulesFiltered(nil)

	check.Contains(t, ruleNames(rules), rulecatalog.JiraReference)

	for _, rule := range rules {
		if rule.Name() != rulecatalog.JiraReference {
			continue
		}

		describer, ok := rule.(domain.CheckDescriber)
		check.True(t, ok)
		check.Equal(t, []string{testCheckIDGL401}, describer.ActiveChecks())

		errs := rule.Validate(domain.ParseCommitMessage("feat: add thing"))
		check.Len(t, errs, 1)
		check.Equal(t, testCheckIDGL401, string(errs[0].Diagnostic.ID))

		return
	}

	check.Failf(t, "jira reference rule missing", "expected jira reference rule")
}

// TestFactory_CreateCommitRulesFiltered_RulesDisabledVetoesCheckEnabledRule
// pins the precedence: enabling a check normally activates its parent rule, and
// an explicit rules.disabled entry must override that.
//
// The rule under test has to be one the check is genuinely responsible for. The
// original form used CommitBody with GL303, and CommitBody is DefaultEnabled in
// the catalog — so it was present regardless of the check, its absence when
// disabled was ordinary rule-disabling, and the checks.enabled entry did
// nothing observable at all. The test could not fail for the reason it names. I
// found this by mutating appendCheckEnabledRuleNames into a no-op and watching
// the test stay green.
//
// JiraReference is not DefaultEnabled, so GL401 is the only thing that can pull
// it in. That makes both rows mean what they say: the control proves the check
// activates the rule, and the veto row proves rules.disabled outranks it.
func TestFactory_CreateCommitRulesFiltered_RulesDisabledVetoesCheckEnabledRule(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name        string
		disabled    []string
		wantPresent bool
	}{
		{name: "rules.disabled vetoes the check-enabled rule", disabled: []string{rulecatalog.JiraReference}},
		{name: "the control: the check activates the rule when not vetoed", disabled: nil, wantPresent: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := settings.Config{
				Rules:  settings.RulesConfig{Disabled: testCase.disabled},
				Checks: settings.ChecksConfig{Enabled: []string{testCheckIDGL401}},
			}

			factory := NewFactory(cfg, &mockSpellChecker{}, &mockLogger{}, "")
			names := ruleNames(factory.CreateCommitRulesFiltered(nil))

			if testCase.wantPresent {
				check.Contains(t, names, rulecatalog.JiraReference)

				return
			}

			check.NotContains(t, names, rulecatalog.JiraReference)
		})
	}
}

func TestResolveEnabledRules_PriorityLogic(t *testing.T) {
	t.Parallel()

	// want is exact, not containment. The previous form asserted only that
	// certain names were present and certain names absent, so a resolution
	// that returned the WHOLE catalog satisfied every row whose expectMissing
	// was empty — "you get exactly the defaults when there is no config" was
	// never actually tested. The sibling KeepsEnabledFirstInOrder already
	// compares the exact slice, so exactness is the house style here and this
	// table was the outlier.
	tests := []struct {
		name           string
		defaultEnabled []string
		rulesConfig    settings.RulesConfig
		want           []string
	}{
		{
			name:           "default_rules_when_no_config",
			defaultEnabled: []string{rulecatalog.Subject, rulecatalog.Conventional},
			rulesConfig:    settings.RulesConfig{},
			want:           []string{rulecatalog.Subject, rulecatalog.Conventional},
		},
		{
			name:           "disabled_takes_precedence_over_enabled",
			defaultEnabled: []string{rulecatalog.Subject},
			rulesConfig: settings.RulesConfig{
				Enabled:  []string{rulecatalog.SignOff},
				Disabled: []string{rulecatalog.SignOff}, // disabled always wins
			},
			want: []string{rulecatalog.Subject},
		},
		{
			name:           "no_overlap",
			defaultEnabled: []string{rulecatalog.Subject, rulecatalog.Conventional},
			rulesConfig: settings.RulesConfig{
				Enabled:  []string{rulecatalog.Spell, rulecatalog.Subject},
				Disabled: []string{rulecatalog.Conventional},
			},
			want: []string{rulecatalog.Spell, rulecatalog.Subject},
		},
		{
			name:           "disabled_overrides_default",
			defaultEnabled: []string{rulecatalog.Subject, rulecatalog.Conventional},
			rulesConfig: settings.RulesConfig{
				Disabled: []string{rulecatalog.Conventional},
			},
			want: []string{rulecatalog.Subject},
		},
		{
			// Rule names fold case, like the whitespace and quotes around
			// them. These two rows were the reverse claim until
			// rulecatalog.Normalize was taught to lower-case: rules.enabled
			// and rules.disabled matched case-sensitively while the --rules
			// filter did not, so a config saying enabled: ["JiraReference"]
			// was rejected as an unknown rule.
			name:           "uppercase_disabled_rule_disables_the_canonical_id",
			defaultEnabled: []string{rulecatalog.Subject},
			rulesConfig: settings.RulesConfig{
				Disabled: []string{"SUBJECT"},
			},
			want: []string{},
		},
		{
			name:           "uppercase_enabled_rule_resolves_to_the_canonical_id",
			defaultEnabled: []string{rulecatalog.Subject},
			rulesConfig: settings.RulesConfig{
				Enabled: []string{"SPELL"},
			},
			want: []string{rulecatalog.Spell, rulecatalog.Subject},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			check.Equal(t, testCase.want,
				settings.ResolveEnabledRules(testCase.defaultEnabled, testCase.rulesConfig))
		})
	}
}

func TestResolveEnabledRules_KeepsEnabledFirstInOrder(t *testing.T) {
	t.Parallel()

	result := settings.ResolveEnabledRules(
		[]string{rulecatalog.Subject, rulecatalog.Conventional, rulecatalog.SignOff},
		settings.RulesConfig{
			Enabled:  []string{rulecatalog.Spell, rulecatalog.Subject},
			Disabled: []string{rulecatalog.SignOff},
		},
	)

	check.Equal(t, []string{rulecatalog.Spell, rulecatalog.Subject, rulecatalog.Conventional}, result)
}

func TestFactory_FilterRules_CaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rules    []string
		filter   []string
		expected []string
	}{
		{
			name:     "exact_match",
			rules:    []string{rulecatalog.Subject, rulecatalog.Conventional, rulecatalog.SignOff},
			filter:   []string{rulecatalog.Subject},
			expected: []string{rulecatalog.Subject},
		},
		{
			name:     "case_insensitive_match",
			rules:    []string{rulecatalog.Subject, rulecatalog.Conventional},
			filter:   []string{"SUBJECT", "Conventional"},
			expected: []string{rulecatalog.Subject, rulecatalog.Conventional},
		},
		{
			name:     "empty_filter_returns_all",
			rules:    []string{rulecatalog.Subject, rulecatalog.Conventional},
			filter:   []string{},
			expected: []string{rulecatalog.Subject, rulecatalog.Conventional},
		},
		{
			name:     "no_matches_returns_empty",
			rules:    []string{rulecatalog.Subject},
			filter:   []string{"nonexistent"},
			expected: []string{},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			result := filterRuleNames(testCase.rules, testCase.filter)
			check.ElementsMatch(t, testCase.expected, result)
		})
	}
}

// TestRuleFactories_CoverCatalogExhaustively is the drift guard for rule
// registration coherence: the rulecatalog is the single source of truth for
// which rules exist, and every catalog entry must have exactly one constructor
// wired here, keyed by its Kind. Adding a rule to the catalog without a factory
// (or removing a factory without the catalog) fails this test at build time
// rather than silently producing a rule that can never be constructed.
func TestRuleFactories_CoverCatalogExhaustively(t *testing.T) {
	t.Parallel()

	commitFactories := commitRuleFactories()
	repoFactories := repositoryRuleFactories()

	// Forward: every catalog rule has a constructor keyed by its Kind.
	for _, rule := range rulecatalog.Rules() {
		switch rule.Kind {
		case rulecatalog.KindCommit:
			_, ok := commitFactories[rule.ID]
			check.True(t, ok, "commit rule %q in catalog must have a commit-rule factory", rule.ID)
		case rulecatalog.KindRepository:
			_, ok := repoFactories[rule.ID]
			check.True(t, ok, "repository rule %q in catalog must have a repository-rule factory", rule.ID)
		default:
			t.Errorf("rule %q has unknown kind %q", rule.ID, rule.Kind)
		}
	}

	// Reverse: no orphan factories for rules the catalog no longer knows.
	for id := range commitFactories {
		rule, ok := rulecatalog.ByID(id)
		check.True(t, ok, "commit-rule factory %q has no catalog entry", id)
		check.Equal(t, rulecatalog.KindCommit, rule.Kind, "factory %q is registered as commit but catalog says otherwise", id)
	}

	for id := range repoFactories {
		rule, ok := rulecatalog.ByID(id)
		check.True(t, ok, "repository-rule factory %q has no catalog entry", id)
		check.Equal(t, rulecatalog.KindRepository, rule.Kind, "factory %q is registered as repository but catalog says otherwise", id)
	}
}
