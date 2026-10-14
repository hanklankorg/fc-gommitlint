// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package i18nextract extracts translatable message IDs from gommitlint
// source into a gettext POT catalog.
//
// The project localizes through its own Localizer interface (Get/GetN) and
// the output.Msg constructor, so generic gettext extractors that type-match
// against the gotext library see none of the call sites. This extractor
// matches the project's API syntactically: Get/GetN calls on receivers
// conventionally named localizer or loc, and bare Msg(...) constructors,
// resolving the output package import binding and same-package string
// constants used as message IDs.
package i18nextract

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	projectOutputPackagePath = "codefloe.com/itiquette/gommitlint/internal/adapters/output"
	projectOutputPackageDir  = "internal/adapters/output"
)

// isLocalizerReceiver reports whether name is an identifier the extractor
// treats as a Localizer value. The codebase uses these names consistently; a
// Get call on any other receiver (maps, struct tags, third-party types) is
// not a translation.
func isLocalizerReceiver(name string) bool {
	return name == "localizer" || name == "loc"
}

// Entry is one translatable message with its source references.
type Entry struct {
	MsgID  string
	Plural string
	Refs   []string
}

// Catalog holds extracted entries keyed by message ID.
type Catalog struct {
	entries map[string]*Entry
}

// Extract walks root for Go source files and collects translatable strings.
// Directories whose root-relative path starts with one of excludes are
// skipped, as are _test.go files. Files are grouped per directory so string
// constants declared in one file resolve in sibling files of the package.
func Extract(root string, excludes []string) (*Catalog, error) {
	catalog := &Catalog{entries: map[string]*Entry{}}
	fset := token.NewFileSet()
	dirs := map[string][]string{}

	err := filepath.WalkDir(root, func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}

		if dirEntry.IsDir() {
			if rel != "." && dirEntry.Name() == "testdata" {
				return filepath.SkipDir
			}

			if isExcluded(rel, excludes) {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		dir := filepath.Dir(path)
		dirs[dir] = append(dirs[dir], path)

		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, files := range dirs {
		if err := catalog.extractPackage(fset, root, files); err != nil {
			return nil, err
		}
	}

	return catalog, nil
}

// WritePOT renders the catalog as a gettext POT file with a standard header,
// entries sorted by message ID, and source references sorted per entry.
func (c *Catalog) WritePOT(writer io.Writer) error {
	header := `msgid ""
msgstr ""
"Project-Id-Version: gommitlint\n"
"Report-Msgid-Bugs-To: \n"
"POT-Creation-Date: \n"
"PO-Revision-Date: \n"
"Last-Translator: \n"
"Language-Team: \n"
"Language: \n"
"MIME-Version: 1.0\n"
"Content-Type: text/plain; charset=UTF-8\n"
"Content-Transfer-Encoding: 8bit\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"
`

	if _, err := io.WriteString(writer, header); err != nil {
		return err
	}

	ids := make([]string, 0, len(c.entries))
	for id := range c.entries {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		if err := writeEntry(writer, c.entries[id]); err != nil {
			return err
		}
	}

	return nil
}

// writeEntry renders a single catalog entry: its sorted source references,
// the message ID, and either a plural block or an empty msgstr.
func writeEntry(writer io.Writer, entry *Entry) error {
	slices.Sort(entry.Refs)

	if _, err := io.WriteString(writer, "\n"); err != nil {
		return err
	}

	for _, ref := range entry.Refs {
		if _, err := fmt.Fprintf(writer, "#: %s\n", ref); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(writer, "msgid %s\n", poQuote(entry.MsgID)); err != nil {
		return err
	}

	if entry.Plural != "" {
		_, err := fmt.Fprintf(writer, "msgid_plural %s\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n", poQuote(entry.Plural))

		return err
	}

	_, err := io.WriteString(writer, "msgstr \"\"\n")

	return err
}

func (c *Catalog) extractPackage(fset *token.FileSet, root string, paths []string) error {
	files := make(map[string]*ast.File, len(paths))
	consts := map[string]string{}

	for _, path := range paths {
		src, err := os.ReadFile(path) //nolint:gosec // Extraction walks the project's own source tree.
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		file, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}

		files[rel] = file

		maps.Copy(consts, packageStringConsts(file))
	}

	for rel, file := range files {
		c.extractCalls(fset, rel, file, consts)
	}

	return nil
}

func isExcluded(rel string, excludes []string) bool {
	if rel == "." {
		return false
	}

	for _, exclude := range excludes {
		if rel == exclude || strings.HasPrefix(rel, exclude+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

func (c *Catalog) extractCalls(fset *token.FileSet, rel string, file *ast.File, consts map[string]string) {
	outputImports, dotOutputImport := projectOutputImports(file)

	ast.Inspect(file, func(node ast.Node) bool {
		switch expr := node.(type) {
		case *ast.CallExpr:
			c.extractCall(fset, rel, expr, consts, outputImports, dotOutputImport)
		case *ast.KeyValueExpr:
			c.extractDisplayName(fset, rel, expr)
		}

		return true
	})
}

func projectOutputImports(file *ast.File) (map[string]struct{}, bool) {
	imports := map[string]struct{}{}
	dotImport := false

	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil || importPath != projectOutputPackagePath {
			continue
		}

		switch {
		case spec.Name == nil:
			imports["output"] = struct{}{}
		case spec.Name.Name == ".":
			dotImport = true
		case spec.Name.Name != "_":
			imports[spec.Name.Name] = struct{}{}
		}
	}

	return imports, dotImport
}

// extractDisplayName collects rule display names from the catalog's
// composite literals. They reach the user through Msg(rule.DisplayName),
// which no static extractor can see, so the declaration site is extracted
// instead.
func (c *Catalog) extractDisplayName(fset *token.FileSet, rel string, kv *ast.KeyValueExpr) {
	key, ok := kv.Key.(*ast.Ident)
	if !ok || key.Name != "DisplayName" {
		return
	}

	value, ok := stringLiteral(kv.Value)
	if !ok || value == "" {
		return
	}

	c.add(value, "", fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(kv.Pos()).Line))
}

// packageStringConsts collects file-level string constants so message IDs
// referenced through a named constant still extract.
func packageStringConsts(file *ast.File) map[string]string {
	consts := map[string]string{}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}

		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != len(value.Values) {
				continue
			}

			for i, name := range value.Names {
				if lit, ok := stringLiteral(value.Values[i]); ok {
					consts[name.Name] = lit
				}
			}
		}
	}

	return consts
}

func (c *Catalog) extractCall(
	fset *token.FileSet,
	rel string,
	call *ast.CallExpr,
	consts map[string]string,
	outputImports map[string]struct{},
	dotOutputImport bool,
) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		receiver, ok := fun.X.(*ast.Ident)
		if !ok {
			return
		}

		if fun.Sel.Name == "Msg" && referencesProjectOutputImport(receiver, outputImports) {
			c.addArg(fset, rel, call, "", consts)

			return
		}

		if !isLocalizerReceiver(receiver.Name) {
			return
		}

		switch fun.Sel.Name {
		case "Get":
			c.addArg(fset, rel, call, "", consts)
		case "GetN":
			if plural, ok := resolveArg(call, 1, consts); ok {
				c.addArg(fset, rel, call, plural, consts)
			}
		}
	case *ast.Ident:
		if fun.Name == "Msg" && referencesProjectOutputMsg(fun, rel, dotOutputImport) {
			c.addArg(fset, rel, call, "", consts)
		}
	}
}

func referencesProjectOutputImport(ident *ast.Ident, imports map[string]struct{}) bool {
	_, imported := imports[ident.Name]

	// The parser leaves imports unresolved but binds declarations in lexical
	// scopes, so a non-nil object means a local identifier shadows the import.
	return imported && ident.Obj == nil
}

func referencesProjectOutputMsg(ident *ast.Ident, rel string, dotOutputImport bool) bool {
	if ident.Obj != nil && ident.Obj.Kind != ast.Fun {
		return false
	}

	dir := filepath.ToSlash(filepath.Dir(rel))

	return dotOutputImport || dir == projectOutputPackageDir
}

func (c *Catalog) addArg(fset *token.FileSet, rel string, call *ast.CallExpr, plural string, consts map[string]string) {
	msgID, ok := resolveArg(call, 0, consts)
	if !ok || msgID == "" {
		return
	}

	c.add(msgID, plural, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), fset.Position(call.Pos()).Line))
}

func (c *Catalog) add(msgID, plural, ref string) {
	entry, exists := c.entries[msgID]
	if !exists {
		entry = &Entry{MsgID: msgID}
		c.entries[msgID] = entry
	}

	if entry.Plural == "" {
		entry.Plural = plural
	}

	if !slices.Contains(entry.Refs, ref) {
		entry.Refs = append(entry.Refs, ref)
	}
}

func resolveArg(call *ast.CallExpr, index int, consts map[string]string) (string, bool) {
	if len(call.Args) <= index {
		return "", false
	}

	if lit, ok := stringLiteral(call.Args[index]); ok {
		return lit, true
	}

	if ident, ok := call.Args[index].(*ast.Ident); ok {
		value, found := consts[ident.Name]

		return value, found
	}

	return "", false
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}

	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}

	return value, true
}

// poQuote escapes a message for a single-line PO string.
func poQuote(msg string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\t", `\t`,
	)

	return `"` + replacer.Replace(msg) + `"`
}
