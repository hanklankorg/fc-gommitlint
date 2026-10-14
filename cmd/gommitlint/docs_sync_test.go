// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	"codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// TestDocsSync_ManpageCoversAllFlags walks the live CLI tree and asserts
// every visible flag on every non-hidden command is mentioned in the man
// page. Catches drift when a new flag lands without a corresponding doc
// update.
func TestDocsSync_ManpageCoversAllFlags(t *testing.T) {
	t.Parallel()

	manpages, err := filepath.Glob(filepath.Join("..", "..", "docs", "manpages", "gommitlint.1.md"))
	check.NoError(t, err)

	locales, err := filepath.Glob(filepath.Join("..", "..", "docs", "manpages", "locale", "*", "gommitlint.1.md"))
	check.NoError(t, err)

	manpages = append(manpages, locales...)
	check.NotEmpty(t, manpages, "no man page sources found under docs/")

	app := createCLIApp(nil)

	for _, path := range manpages {
		data, err := os.ReadFile(path)
		check.NoError(t, err)

		content := string(data)

		var missing []string

		walkCommands(app, app.Name, func(cmd *cli.Command, cmdPath string) {
			for _, flag := range cmd.Flags {
				if vf, ok := flag.(cli.VisibleFlag); ok && !vf.IsVisible() {
					continue
				}

				name := flag.Names()[0]
				if !strings.Contains(content, "--"+name) {
					missing = append(missing, cmdPath+" --"+name)
				}
			}
		})

		check.Empty(t, missing,
			"flags not documented in %s — add them there or mark them Hidden: %v",
			path, missing)
	}
}

// TestDocsSync_DocsCoverAllRules asserts every rule name from the domain
// constants is mentioned in both the man page and the reference document.
// Rule-name drift would break user configuration silently.
func TestDocsSync_DocsCoverAllRules(t *testing.T) {
	t.Parallel()

	for _, relpath := range []string{"docs/manpages/gommitlint.1.md", "docs/modules/ROOT/pages/reference/rules.adoc"} {
		content := readRepoFile(t, relpath)
		for _, rule := range settings.GetValidRuleNames() {
			check.Contains(t, content, rule,
				"rule %q missing from %s", rule, relpath)
		}
	}
}

func TestDocsSync_DocsCoverAllChecks(t *testing.T) {
	t.Parallel()

	content := readRepoFile(t, "docs/modules/ROOT/pages/reference/rules.adoc")
	for _, definition := range diagnostics.Definitions() {
		check.Contains(t, content, definition.ID,
			"check %q missing from validation rule reference", definition.ID)
	}
}

func TestDocsSync_StructuredDefaultsMatchMetadata(t *testing.T) {
	t.Parallel()

	readme := readRepoFile(t, "README.adoc")
	check.Contains(t, readme, fmt.Sprintf("default limit: %d) | ✓", settings.DefaultMaxCommitsAhead))
	check.Contains(t, readme, "| `"+rulecatalog.CommitBody+"` | Commit body | Commit body requirements (presence, length, line width) | ✓")

	configuration := readRepoFile(t, "docs/modules/ROOT/pages/reference/configuration.adoc")
	check.Contains(t, configuration, "|`"+settings.DefaultSpellLocale+"`")
	check.Contains(t, configuration, "Empty lists do not mean that no rules run")

	rules := readRepoFile(t, "docs/modules/ROOT/pages/reference/rules.adoc")

	for _, rule := range rulecatalog.Rules() {
		defaultStatus := "Disabled"
		if rule.DefaultEnabled {
			defaultStatus = "Enabled"
		}

		check.Contains(t, rules, "|`"+rule.ID+"`\n|"+rule.DisplayName+"\n|"+defaultStatus)
	}
}

func TestDocsSync_ManpageSynopsisCoversSubcommands(t *testing.T) {
	t.Parallel()

	manpages, err := filepath.Glob(filepath.Join("..", "..", "docs", "manpages", "gommitlint.1.md"))
	check.NoError(t, err)

	locales, err := filepath.Glob(filepath.Join("..", "..", "docs", "manpages", "locale", "*", "gommitlint.1.md"))
	check.NoError(t, err)

	manpages = append(manpages, locales...)
	check.NotEmpty(t, manpages, "no man page sources found under docs/")

	app := createCLIApp(nil)

	for _, path := range manpages {
		content := readRepoFile(t, filepath.ToSlash(pathFromRepoRoot(t, path)))
		synopsis := markdownSection(content, "## SYNOPSIS")
		check.NotEmpty(t, synopsis, "missing synopsis section in %s", path)

		var missing []string

		walkCommands(app, app.Name, func(cmd *cli.Command, cmdPath string) {
			if cmd == app || len(cmd.Commands) == 0 {
				return
			}

			line := synopsisLineForCommand(synopsis, cmdPath)
			if line == "" {
				missing = append(missing, cmdPath)

				return
			}

			for _, sub := range cmd.Commands {
				if sub.Hidden {
					continue
				}

				if !strings.Contains(line, "**"+sub.Name+"**") {
					missing = append(missing, cmdPath+" "+sub.Name)
				}
			}
		})

		check.Empty(t, missing, "commands missing from synopsis in %s: %v", path, missing)
	}
}

func walkCommands(cmd *cli.Command, path string, fn func(*cli.Command, string)) {
	fn(cmd, path)

	for _, sub := range cmd.Commands {
		if sub.Hidden {
			continue
		}

		walkCommands(sub, path+" "+sub.Name, fn)
	}
}

func readRepoFile(t *testing.T, relpath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "..", relpath))
	check.NoError(t, err)

	return string(data)
}

func pathFromRepoRoot(t *testing.T, path string) string {
	t.Helper()

	relpath, err := filepath.Rel(filepath.Join("..", ".."), path)
	check.NoError(t, err)

	return relpath
}

func markdownSection(content, heading string) string {
	_, after, ok := strings.Cut(content, heading)
	if !ok {
		return ""
	}

	afterHeading := after

	before0, _, ok0 := strings.Cut(afterHeading, "\n## ")
	if !ok0 {
		return afterHeading
	}

	return before0
}

func synopsisLineForCommand(synopsis, cmdPath string) string {
	parts := strings.Fields(cmdPath)
	if len(parts) <= 1 {
		return ""
	}

	for line := range strings.SplitSeq(synopsis, "\n") {
		matched := true

		for _, part := range parts[1:] {
			if !strings.Contains(line, "**"+part+"**") {
				matched = false

				break
			}
		}

		if matched {
			return line
		}
	}

	return ""
}
