// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Command schemagen emits the JSON Schema for the gommitlint configuration
// file from the Go types in internal/domain/settings.
//
// Output is written to schemas/gommitlint.schema.json (or the path passed as
// the first positional argument). The resulting file is embedded into the
// shipped binary via internal/adapters/cli/commands so that `gommitlint config
// schema` prints byte-identical content without needing access to source.
//
// Run via `just generate-schema` or `go run ./internal/tools/schemagen`.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/invopop/jsonschema"

	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

const (
	defaultOutputPath = "schemas/gommitlint.schema.json"
	settingsPath      = "./internal/domain/settings"
	schemaID          = "https://codefloe.com/itiquette/gommitlint/raw/branch/main/schemas/gommitlint.schema.json"
	schemaTitle       = "Gommitlint configuration"
	schemaDescription = "Configuration schema for gommitlint — the Git commit message linter. " +
		"Applies to .gommitlint.yaml, .gommitlint.yml, and .gommitlint.toml."
)

func main() {
	outPath := defaultOutputPath
	if len(os.Args) > 1 {
		outPath = os.Args[1]
	}

	schema, err := generate(settingsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "schemagen: %v\n", err)
		os.Exit(1)
	}

	// 0o755 directory + 0o644 file: the schema is a public artifact consumed by
	// editors, CI, and SchemaStore — world-readable is the documented behaviour.
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil { //nolint:gosec
		fmt.Fprintf(os.Stderr, "schemagen: create output directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, schema, 0o644); err != nil { //nolint:gosec
		fmt.Fprintf(os.Stderr, "schemagen: write schema: %v\n", err)
		os.Exit(1)
	}
}

func generate(sourcePath string) ([]byte, error) {
	reflector := &jsonschema.Reflector{
		// Honor `jsonschema:"required"` tags. Pointer fields stay optional so
		// "not set" remains distinct from "explicitly zero" — matches how the
		// loader/merger treats them.
		RequiredFromJSONSchemaTags: true,
		// Keep $defs / $ref structure: smaller schema, lets editors deduplicate
		// hover docs, and survives refactors better than an inlined root.
		DoNotReference: false,
	}

	// Lift Go doc comments into `description` fields. Walks the source tree at
	// schemagen time only — the binary embeds the produced JSON.
	if err := reflector.AddGoComments("codefloe.com/itiquette/gommitlint", sourcePath); err != nil {
		return nil, fmt.Errorf("collect Go doc comments: %w", err)
	}

	schema := reflector.Reflect(&settings.Config{})
	schema.ID = jsonschema.ID(schemaID)
	schema.Title = schemaTitle
	schema.Description = schemaDescription

	buf, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}

	return append(buf, '\n'), nil
}
