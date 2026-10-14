// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const presetPrefix = "preset:"

var presetNames = []string{"minimal", "conventional", "ambitious", "secure"} //nolint:gochecknoglobals // immutable registry

// ErrUnknownPreset is returned when a preset reference cannot be resolved.
var ErrUnknownPreset = errors.New("unknown config preset")

// IsPresetReference reports whether an extends entry refers to a built-in preset.
func IsPresetReference(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), presetPrefix)
}

// ParsePresetReference extracts the preset name from a preset reference.
func ParsePresetReference(ref string) (string, bool) {
	trimmed := strings.TrimSpace(ref)
	if !strings.HasPrefix(trimmed, presetPrefix) {
		return "", false
	}

	name := strings.TrimSpace(strings.TrimPrefix(trimmed, presetPrefix))
	if name == "" {
		return "", false
	}

	return name, true
}

// PresetNames returns the built-in preset names in stable display order.
func PresetNames() []string {
	result := make([]string, len(presetNames))
	copy(result, presetNames)

	return result
}

// IsKnownPreset reports whether name identifies a built-in preset.
func IsKnownPreset(name string) bool {
	return slices.Contains(presetNames, strings.TrimSpace(name))
}

// ResolvePreset returns the configuration associated with a built-in preset.
func ResolvePreset(name string) (configTypes.Config, error) {
	switch name {
	case "minimal":
		return presetMinimal(), nil
	case "conventional":
		return presetConventional(), nil
	case "ambitious":
		return presetAmbitious(), nil
	case "secure":
		return presetSecure(), nil
	default:
		return configTypes.Config{}, fmt.Errorf("%w: %s", ErrUnknownPreset, name)
	}
}

func presetConventional() configTypes.Config {
	return configTypes.NewDefault()
}

func presetMinimal() configTypes.Config {
	return configTypes.Config{
		Rules: configTypes.RulesConfigForEnabled(
			rulecatalog.Conventional,
			rulecatalog.Subject,
		),
	}
}

func presetAmbitious() configTypes.Config {
	return configTypes.Config{
		Conventional: configTypes.ConventionalConfig{
			RequireScope: new(true),
		},
		Message: configTypes.MessageConfig{
			Body: configTypes.BodyConfig{
				Required: new(true),
			},
		},
		Rules: configTypes.RulesConfigForEnabled(
			rulecatalog.BranchAhead,
			rulecatalog.CommitBody,
			rulecatalog.Conventional,
			rulecatalog.SignOff,
			rulecatalog.Spell,
			rulecatalog.Subject,
		),
	}
}

func presetSecure() configTypes.Config {
	return configTypes.Config{
		Rules: configTypes.RulesConfigForEnabled(
			rulecatalog.Conventional,
			rulecatalog.CryptoSignature,
			rulecatalog.Identity,
			rulecatalog.SignOff,
			rulecatalog.Subject,
		),
	}
}
