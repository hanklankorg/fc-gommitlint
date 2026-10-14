// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package i18nextract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	check.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}

func TestExtract_CollectsProjectAPICalls(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "consts.go", `package p

const sharedLabel = "Shared label"
`)
	writeFile(t, dir, "main.go", `package p

import "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func render(localizer Localizer, loc Localizer) {
	_ = localizer.Get("Plain message")
	_ = localizer.Get("With value: %s", "x")
	_ = localizer.GetN("%d item", "%d items", 2, 2)
	_ = loc.Get(sharedLabel)
	_ = output.Msg("Qualified help text")
	_ = other.Get("Not a localizer call")
	_ = tag.Get("also skipped")
}

var rules = []Rule{{DisplayName: "Commit body"}}
`)
	writeFile(t, dir, "bare.go", `package p

import . "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func help() { _ = Msg("Help text %s", "y") }
`)
	writeFile(t, dir, "main_test.go", `package p

func testOnly(localizer Localizer) {
	_ = localizer.Get("Test-only message")
}
`)

	catalog, err := Extract(dir, nil)
	check.NoError(t, err)

	var pot strings.Builder

	check.NoError(t, catalog.WritePOT(&pot))
	output := pot.String()

	check.Contains(t, output, `msgid "Plain message"`)
	check.Contains(t, output, `msgid "With value: %s"`)
	check.Contains(t, output, `msgid "%d item"`)
	check.Contains(t, output, `msgid_plural "%d items"`)
	check.Contains(t, output, `msgid "Shared label"`)
	check.Contains(t, output, `msgid "Help text %s"`)
	check.Contains(t, output, `msgid "Qualified help text"`)
	check.Contains(t, output, `msgid "Commit body"`)
	check.NotContains(t, output, "Not a localizer call")
	check.NotContains(t, output, "also skipped")
	check.NotContains(t, output, "Test-only message")
}

func TestExtract_OutputPackageAliasesAndShadows(t *testing.T) {
	t.Parallel()

	catalog, err := Extract(filepath.Join("testdata", "bindings"), nil)
	check.NoError(t, err)

	check.Len(t, catalog.entries, 4)
	check.Contains(t, catalog.entries, "Aliased output message")
	check.Contains(t, catalog.entries, "Default output message")
	check.Contains(t, catalog.entries, "Dot-imported output message")
	check.Contains(t, catalog.entries, "Preserved localizer message")
	check.NotContains(t, catalog.entries, "Shadowed output.Msg")
	check.NotContains(t, catalog.entries, "Shadowed bare Msg")
}

// TestExtract_BareMsgInsideTheOutputPackage covers the arm that makes a bare
// Msg("...") call translatable when the file itself lives in the output
// package (internal/adapters/output declares Msg, so its own files call it
// unqualified). The same bare call anywhere else is not the project's Msg.
func TestExtract_BareMsgInsideTheOutputPackage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputDir := filepath.Join(dir, "internal", "adapters", "output")
	otherDir := filepath.Join(dir, "internal", "other")

	check.NoError(t, os.MkdirAll(outputDir, 0o750))
	check.NoError(t, os.MkdirAll(otherDir, 0o750))
	writeFile(t, outputDir, "help.go", `package output

func help() string { return Msg("Bare output message") }
`)
	writeFile(t, otherDir, "other.go", `package other

func help() string { return Msg("Bare message elsewhere") }
`)

	catalog, err := Extract(dir, nil)
	check.NoError(t, err)

	check.Contains(t, catalog.entries, "Bare output message")
	check.NotContains(t, catalog.entries, "Bare message elsewhere",
		"the control: a bare Msg outside the output package is some other function")
}

func TestExtract_SkipsNestedTestdata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "main.go", `package p

import "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func product() { _ = output.Msg("Product message") }
`)
	check.NoError(t, os.Mkdir(filepath.Join(dir, "testdata"), 0o700))
	writeFile(t, filepath.Join(dir, "testdata"), "fixture.go", `package fixture

import "codefloe.com/itiquette/gommitlint/internal/adapters/output"

func fixture() { _ = output.Msg("Fixture message") }
`)

	catalog, err := Extract(dir, nil)
	check.NoError(t, err)
	check.Contains(t, catalog.entries, "Product message")
	check.NotContains(t, catalog.entries, "Fixture message")
}

func TestExtract_CollectsEveryRuleHelpKey(t *testing.T) {
	t.Parallel()

	root, err := repoRootFromWD()
	check.NoError(t, err)

	catalog, err := Extract(root, []string{"generated"})
	check.NoError(t, err)

	var pot strings.Builder
	check.NoError(t, catalog.WritePOT(&pot))

	for _, ruleID := range rulecatalog.IDs() {
		check.Contains(t, pot.String(), `msgid "rulehelp.`+ruleID+`"`)
	}
}

// TestExtract_RespectsExcludes needs the kept file as much as the skipped one.
// With only the excluded directory present, an Extract that walked nothing at
// all — or that failed to find any call sites — produces an empty catalog and
// satisfies the absence assertion perfectly. The sibling message proves the
// walk ran and found calls, so the one that is missing is missing because it
// was excluded.
func TestExtract_RespectsExcludes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	check.NoError(t, os.MkdirAll(filepath.Join(dir, "skipme"), 0o750))
	check.NoError(t, os.MkdirAll(filepath.Join(dir, "keepme"), 0o750))
	writeFile(t, dir, filepath.Join("skipme", "a.go"), `package p

func f(localizer Localizer) { _ = localizer.Get("Excluded message") }
`)
	writeFile(t, dir, filepath.Join("keepme", "b.go"), `package p

func g(localizer Localizer) { _ = localizer.Get("Kept message") }
`)

	catalog, err := Extract(dir, []string{"skipme"})
	check.NoError(t, err)

	var pot strings.Builder

	check.NoError(t, catalog.WritePOT(&pot))
	output := pot.String()

	check.NotContains(t, output, "Excluded message")
	check.Contains(t, output, `msgid "Kept message"`,
		"the control: a directory that was not excluded must still be walked")
}

func TestWritePOT_EscapesSpecialCharacters(t *testing.T) {
	t.Parallel()

	// Tab is escaped by poQuote alongside the other three and was the one
	// character the fixture left out; an unescaped tab in a PO file breaks the
	// entry for any tool that reads it.
	const raw = "line\nbreak \"quoted\" back\\slash tab\there"

	catalog := &Catalog{entries: map[string]*Entry{
		raw: {MsgID: raw, Refs: []string{"a.go:1"}},
	}}

	var pot strings.Builder

	check.NoError(t, catalog.WritePOT(&pot))
	check.Contains(t, pot.String(), `msgid "line\nbreak \"quoted\" back\\slash tab\there"`)
}

// TestLocaleCatalogsFullyTranslated guards against untranslated or fuzzy
// entries sneaking into the shipped catalogs. New msgids surface here the
// moment `just update-translations` runs, so the failure names exactly the
// strings that still need a translation.
func TestLocaleCatalogsFullyTranslated(t *testing.T) {
	t.Parallel()

	root, err := repoRootFromWD()
	check.NoError(t, err)

	matches, err := filepath.Glob(filepath.Join(root, "locale", "*", "gommitlint.po"))
	check.NoError(t, err)
	check.NotEmpty(t, matches, "no locale catalogs found")

	for _, path := range matches {
		data, err := os.ReadFile(path)
		check.NoError(t, err)

		for _, missing := range untranslatedMsgIDs(string(data)) {
			t.Errorf("%s: untranslated msgid %q", filepath.Base(filepath.Dir(path)), missing)
		}

		// A fuzzy flag can share its line with other flags (`#, fuzzy, c-format`)
		// and translator tools put `#| msgid "..."` lines between it and the
		// msgid, so the check is any flag line carrying fuzzy, not one literal shape.
		if fuzzyFlagRE.Match(data) {
			t.Errorf("%s: catalog contains fuzzy entries — review and definalize them", path)
		}
	}
}

var (
	fuzzyFlagRE = regexp.MustCompile(`(?m)^#,.*\bfuzzy\b`)
	msgIDRE     = regexp.MustCompile(`msgid ((?:"(?:[^"\\]|\\.)*"\n?)+)`)
	msgStrRE    = regexp.MustCompile(`msgstr(?:\[0\])? ((?:"(?:[^"\\]|\\.)*"\n?)+)`)
	quotedRE    = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
)

func untranslatedMsgIDs(po string) []string {
	var missing []string

	for block := range strings.SplitSeq(po, "\n\n") {
		if !strings.Contains(block, "msgid") || strings.HasPrefix(strings.TrimSpace(block), "#~") {
			continue
		}

		id := joinedQuoted(msgIDRE, block)
		if id == "" { // header block
			continue
		}

		if joinedQuoted(msgStrRE, block) == "" {
			missing = append(missing, id)
		}
	}

	return missing
}

func joinedQuoted(re *regexp.Regexp, block string) string {
	match := re.FindStringSubmatch(block)
	if match == nil {
		return ""
	}

	var joined strings.Builder
	for _, part := range quotedRE.FindAllStringSubmatch(match[1], -1) {
		joined.WriteString(part[1])
	}

	return joined.String()
}

func repoRootFromWD() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}

		dir = parent
	}
}
