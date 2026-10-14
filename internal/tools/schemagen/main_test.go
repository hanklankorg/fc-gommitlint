// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// repoRootFromSchemagen is the relative path from this package
// (internal/tools/schemagen) up to the repository root, which is where main
// runs and therefore where settingsPath resolves from.
const repoRootFromSchemagen = "../../.."

// generateSchema produces the schema exactly as main does: from the repository
// root, using the package's own settingsPath.
//
// The working directory is part of the input here, not incidental setup.
// settingsPath is relative, and reflector.AddGoComments derives the import path
// of each package it walks by joining the base module path with that relative
// path. A test that reaches the same directory by another route — say
// "../../domain/settings" from this package — walks the same files but keys the
// comments under a package path matching nothing, and silently produces a
// schema with no descriptions at all. Every structural assertion still passes
// in that state, which is exactly how a green suite can describe a generator
// nobody actually runs.
func generateSchema(t *testing.T) []byte {
	t.Helper()
	t.Chdir(repoRootFromSchemagen)

	data, err := generate(settingsPath)
	check.NoError(t, err)

	return data
}

func decodeSchema(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var schema map[string]any
	check.NoError(t, json.Unmarshal(data, &schema))

	return schema
}

// property walks $defs to one named property, so the tests below read as the
// claim they make rather than as five nested type assertions each.
func property(t *testing.T, schema map[string]any, definition, name string) map[string]any {
	t.Helper()

	defs, ok := schema["$defs"].(map[string]any)
	check.True(t, ok, "schema has no $defs")

	def, ok := defs[definition].(map[string]any)
	check.True(t, ok, "no definition %q", definition)

	properties, ok := def["properties"].(map[string]any)
	check.True(t, ok, "%s has no properties", definition)

	found, ok := properties[name].(map[string]any)
	check.True(t, ok, "%s has no property %q", definition, name)

	return found
}

func TestGenerate_MinSignoffCountRange(t *testing.T) {
	minSignoffCount := property(t, decodeSchema(t, generateSchema(t)), "BodyConfig", "min_signoff_count")

	check.Equal(t, float64(settings.MinSignoffCount), minSignoffCount["minimum"])
	check.Equal(t, float64(settings.MaxSignoffCount), minSignoffCount["maximum"])
}

// TestGenerate_SchemaIdentity pins the fields that make the output a
// publishable artifact rather than an anonymous blob. The range test above
// passes on a schema with no $id and no title at all.
func TestGenerate_SchemaIdentity(t *testing.T) {
	schema := decodeSchema(t, generateSchema(t))

	// The $id is the address the schema is published at (commands.adoc: "The
	// checked-in schema is at ..."), stated here as a literal rather than as
	// the constant main.go sets, so a changed branch or path fails this test.
	check.Equal(t, "https://codefloe.com/itiquette/gommitlint/raw/branch/main/schemas/gommitlint.schema.json", schema["$id"])
	check.Equal(t, schemaTitle, schema["title"])
	check.Equal(t, schemaDescription, schema["description"])
}

// TestGenerate_LiftsGoDocCommentsIntoDescriptions guards the reason
// reflector.AddGoComments is called at all: every configuration key carries its
// Go doc comment into the schema, which is what editors show on hover and what
// makes the published file self-documenting.
//
// Nothing asserted it, and the failure is silent — AddGoComments returns no
// error when it matches nothing, so a changed base path or a moved package
// yields a schema that is structurally perfect and entirely undocumented. Every
// other assertion in this file still passes in that state.
//
// The count is a floor, not an exact number, so adding a configuration field
// does not fail this test; what it catches is the lift collapsing.
func TestGenerate_LiftsGoDocCommentsIntoDescriptions(t *testing.T) {
	data := generateSchema(t)

	descriptions := strings.Count(string(data), `"description"`)
	check.True(t, descriptions > 50,
		"expected a description on most configuration keys, found %d — AddGoComments matched nothing",
		descriptions)

	// Named as well as counted: this text can only have come from the Go
	// source, so the floor above cannot be met by descriptions from elsewhere.
	minSignoffCount := property(t, decodeSchema(t, data), "BodyConfig", "min_signoff_count")

	description, ok := minSignoffCount["description"].(string)
	check.True(t, ok, "min_signoff_count must carry its Go doc comment")
	check.Contains(t, description, "MinSignoffCount")
}

// TestGenerate_RejectsAMissingSourcePath is the negative control for the tests
// above: all of them call generate on a path that exists, so a generate that
// ignored its argument entirely would satisfy them.
func TestGenerate_RejectsAMissingSourcePath(t *testing.T) {
	t.Parallel()

	_, err := generate(filepath.Join(t.TempDir(), "no-such-package"))

	check.Error(t, err)
}
