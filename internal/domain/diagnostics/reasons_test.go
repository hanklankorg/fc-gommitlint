// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package diagnostics_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

// Go cannot enumerate constants, so the Reason constants in reasons.go are read
// from source: every declared reason must be catalogued (otherwise it has no
// diagnostic ID and no formatter coverage) and every catalogued reason must be a
// declared constant (otherwise it is a typo nothing can emit).
func TestAllValidationReasons_MatchEveryDeclaredReason(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), "reasons.go", nil, 0)
	check.NoError(t, err)

	var declared []diagnostics.Reason

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			typeIdent, ok := valueSpec.Type.(*ast.Ident)
			if !ok || typeIdent.Name != "Reason" {
				continue
			}

			for _, value := range valueSpec.Values {
				literal, ok := value.(*ast.BasicLit)
				check.True(t, ok && literal.Kind == token.STRING, "reason constants must be string literals")

				text, err := strconv.Unquote(literal.Value)
				check.NoError(t, err)

				declared = append(declared, diagnostics.Reason(text))
			}
		}
	}

	check.Contains(t, declared, diagnostics.ReasonSubjectTooLong, "the parser must see the constants")

	listed := diagnostics.AllValidationReasons()

	slices.Sort(declared)
	slices.Sort(listed)

	check.Equal(t, declared, listed)
}
