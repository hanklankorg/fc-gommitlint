// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

import (
	"reflect"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// FuzzConfigComposition verifies overlay precedence for strings, pointers,
// slices, and nested fields, plus idempotence and input immutability.
func FuzzConfigComposition(f *testing.F) {
	f.Add("text", "", "subject", "spell", 72, 0, true, false, false)
	f.Add("text", "json", "subject", "spell", 72, 0, true, true, false)
	f.Add("", "sarif", "", "conventional", -1, 100, false, true, true)

	f.Fuzz(func(
		t *testing.T,
		baseOutput, overlayOutput, baseRule, overlayRule string,
		baseMax, overlayMax int,
		required, setRules, clearRules bool,
	) {
		if len(baseOutput)+len(overlayOutput)+len(baseRule)+len(overlayRule) > 64*1024 {
			t.Skip()
		}

		base := settings.Config{
			Output: baseOutput,
			Message: settings.MessageConfig{
				Subject: settings.SubjectConfig{MaxLength: new(baseMax)},
				Body:    settings.BodyConfig{Required: new(!required)},
			},
			Rules: settings.RulesConfig{Enabled: []string{baseRule}},
		}
		overlay := settings.Config{
			Output: overlayOutput,
			Message: settings.MessageConfig{
				Subject: settings.SubjectConfig{MaxLength: new(overlayMax)},
				Body:    settings.BodyConfig{Required: new(required)},
			},
		}

		if setRules {
			if clearRules {
				overlay.Rules.Enabled = []string{}
			} else {
				overlay.Rules.Enabled = []string{overlayRule, overlayRule}
			}
		}

		baseBefore := cloneFuzzConfig(base)
		overlayBefore := cloneFuzzConfig(overlay)

		merged := configuration.MergeConfig(base, overlay)

		wantOutput := baseOutput
		if overlayOutput != "" {
			wantOutput = overlayOutput
		}

		if merged.Output != wantOutput {
			t.Fatalf("string precedence changed: got %q want %q", merged.Output, wantOutput)
		}

		if got := settings.DerefInt(merged.Message.Subject.MaxLength, baseMax); got != overlayMax {
			t.Fatalf("pointer precedence changed: got %d want %d", got, overlayMax)
		}

		if got := settings.DerefBool(merged.Message.Body.Required, !required); got != required {
			t.Fatalf("explicit boolean precedence changed: got %t want %t", got, required)
		}

		wantRules := settings.NormalizeRulesConfig(base.Rules).Enabled
		if setRules {
			wantRules = settings.NormalizeRulesConfig(overlay.Rules).Enabled
		}

		if !reflect.DeepEqual(merged.Rules.Enabled, wantRules) {
			t.Fatalf("slice precedence changed: got %#v want %#v", merged.Rules.Enabled, wantRules)
		}

		if composed := configuration.MergeConfigs(base, overlay); !reflect.DeepEqual(merged, composed) {
			t.Fatalf("variadic and pairwise composition disagree:\npairwise=%#v\nvariadic=%#v", merged, composed)
		}

		if again := configuration.MergeConfig(merged, overlay); !reflect.DeepEqual(merged, again) {
			t.Fatalf("merging the same overlay is not idempotent:\nfirst=%#v\nsecond=%#v", merged, again)
		}

		if !reflect.DeepEqual(base, baseBefore) || !reflect.DeepEqual(overlay, overlayBefore) {
			t.Fatal("MergeConfig mutated an input configuration")
		}

		// The snapshots share the inputs' pointers, so a write through one
		// would leave them equal; compare the pointees with the values set.
		if *base.Message.Subject.MaxLength != baseMax || *base.Message.Body.Required != !required {
			t.Fatalf("MergeConfig wrote through a base pointer: max_length=%d required=%t",
				*base.Message.Subject.MaxLength, *base.Message.Body.Required)
		}

		if *overlay.Message.Subject.MaxLength != overlayMax || *overlay.Message.Body.Required != required {
			t.Fatalf("MergeConfig wrote through an overlay pointer: max_length=%d required=%t",
				*overlay.Message.Subject.MaxLength, *overlay.Message.Body.Required)
		}
	})
}

func cloneFuzzConfig(config settings.Config) settings.Config {
	clone := config
	if config.Rules.Enabled != nil {
		clone.Rules.Enabled = append([]string{}, config.Rules.Enabled...)
	}

	if config.Rules.Disabled != nil {
		clone.Rules.Disabled = append([]string{}, config.Rules.Disabled...)
	}

	return clone
}
