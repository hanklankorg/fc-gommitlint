// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestMergeConfig_CoversEveryConfigLeaf reflectively walks Config, builds an overlay where every
// leaf field is populated with a non-zero overlay-marker value, merges it onto a
// zero-valued base, and asserts every leaf field in the result reflects the
// overlay. Any Config field MergeConfig forgets to handle will leave its
// corresponding leaf at the base's zero value and fail the test.
//
// Skips: rules.{enabled,disabled} (NormalizeRulesConfig may rewrite values);
// managed_sources (slice of structs, compared as a whole instead).
func TestMergeConfig_CoversEveryConfigLeaf(t *testing.T) {
	t.Parallel()

	overlay := buildOverlayConfig(t)
	base := configTypes.Config{}
	merged := MergeConfig(base, overlay)

	var missing []string

	walkConfigLeaves(reflect.ValueOf(merged), reflect.ValueOf(overlay), "", &missing)

	slices.Sort(missing)
	check.Empty(t, missing,
		"MergeConfig did not overlay these fields — add the missing overlay call in merger.go")
}

const (
	overlayMarkString = "overlay-mark"
	overlayMarkInt    = 42
	overlayMarkBool   = true
)

func buildOverlayConfig(t *testing.T) configTypes.Config {
	t.Helper()

	var cfg configTypes.Config

	populateValue(reflect.ValueOf(&cfg).Elem())

	// NormalizeRulesConfig runs inside MergeConfig and lowercases/dedupes rule
	// lists. Use values that survive normalization unchanged.
	cfg.Rules.Enabled = []string{"overlay-rule-enabled"}
	cfg.Rules.Disabled = []string{"overlay-rule-disabled"}
	// NormalizeChecksConfig runs inside MergeConfig and uppercases/dedupes check
	// lists. Use values that survive normalization unchanged.
	cfg.Checks.Enabled = []string{"GL102"}
	cfg.Checks.Disabled = []string{"GL901"}

	return cfg
}

func populateValue(value reflect.Value) {
	switch value.Kind() { //nolint:exhaustive // Only the kinds reachable from Config leaves are handled; others left at zero on purpose.
	case reflect.Struct:
		for _, field := range value.Fields() {
			populateValue(field)
		}
	case reflect.Pointer:
		elem := reflect.New(value.Type().Elem())
		populateValue(elem.Elem())
		value.Set(elem)
	case reflect.String:
		value.SetString(overlayMarkString)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(overlayMarkInt)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value.SetUint(overlayMarkInt)
	case reflect.Bool:
		value.SetBool(overlayMarkBool)
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.String {
			value.Set(reflect.ValueOf([]string{overlayMarkString}))
		}
		// slice-of-struct (ManagedSources): leave nil; checked separately.
	}
}

// mergerUntracked names config paths intentionally excluded from leaf-by-leaf
// comparison. Both entries are post-processed by NormalizeRulesConfig inside
// MergeConfig, so their values may legitimately differ from the overlay.
var mergerUntracked = map[string]struct{}{
	"rules.enabled":  {},
	"rules.disabled": {},
}

func walkConfigLeaves(merged, overlay reflect.Value, prefix string, missing *[]string) {
	mergedType := merged.Type()

	if merged.Kind() == reflect.Struct {
		for fieldIdx := range merged.NumField() {
			name := yamlNameFromField(mergedType.Field(fieldIdx))
			if name == "" || name == "-" {
				continue
			}

			path := name
			if prefix != "" {
				path = prefix + "." + name
			}

			if _, skip := mergerUntracked[path]; skip {
				continue
			}

			walkConfigLeaves(merged.Field(fieldIdx), overlay.Field(fieldIdx), path, missing)
		}

		return
	}

	if !reflect.DeepEqual(merged.Interface(), overlay.Interface()) {
		*missing = append(*missing, prefix)
	}
}

func yamlNameFromField(field reflect.StructField) string {
	tag := field.Tag.Get("yaml")
	if tag == "" {
		return ""
	}

	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}

	return tag
}
