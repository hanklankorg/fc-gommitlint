// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package schemas embeds the JSON Schema for the gommitlint configuration file.
//
// The schema is generated from internal/domain/settings by
// internal/tools/schemagen. The committed JSON in this directory is the
// canonical artifact — `gommitlint config schema` prints these bytes and the
// SchemaStore catalog references the file at its raw URL.
package schemas

import _ "embed"

// Config is the embedded JSON Schema for the gommitlint configuration file.
// Byte-identical to schemas/gommitlint.schema.json shipped at the repo root
// and published via SchemaStore.
//
//go:embed gommitlint.schema.json
var Config []byte
