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

// TestProvenanceCoverage walks the Config struct via reflection and verifies that
// every leaf field is either tracked by getProvenanceFields or listed in
// provenanceUntracked. New Config fields fail this test until they appear in
// one of those two places.
func TestProvenanceCoverage(t *testing.T) {
	t.Parallel()

	leaves := collectConfigLeaves(reflect.TypeFor[configTypes.Config](), "")

	tracked := make(map[string]struct{}, len(getProvenanceFields()))
	for _, field := range getProvenanceFields() {
		tracked[field.path] = struct{}{}
	}

	var missing []string

	for _, path := range leaves {
		if _, ok := tracked[path]; ok {
			continue
		}

		if _, ok := provenanceUntracked[path]; ok {
			continue
		}

		missing = append(missing, path)
	}

	slices.Sort(missing)
	check.Empty(t, missing,
		"Config fields without provenance coverage. Add to getProvenanceFields() in provenance.go, or to provenanceUntracked if intentional.")

	stray := make([]string, 0)

	for path := range tracked {
		if !pathExists(leaves, path) {
			stray = append(stray, path)
		}
	}

	slices.Sort(stray)
	check.Empty(t, stray, "Provenance entries refer to paths that no longer exist on Config")
}

// collectConfigLeaves returns the yaml-tagged dotted paths of every leaf field
// reachable from t. Struct fields recurse; everything else is a leaf.
func collectConfigLeaves(t reflect.Type, prefix string) []string {
	var paths []string

	for field := range t.Fields() {
		name := yamlName(field)

		if name == "" || name == "-" {
			continue
		}

		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		fieldType := field.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct {
			paths = append(paths, collectConfigLeaves(fieldType, path)...)

			continue
		}

		paths = append(paths, path)
	}

	return paths
}

func yamlName(field reflect.StructField) string {
	tag := field.Tag.Get("yaml")
	if tag == "" {
		return ""
	}

	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}

	return tag
}

func pathExists(leaves []string, path string) bool {
	return slices.Contains(leaves, path)
}
