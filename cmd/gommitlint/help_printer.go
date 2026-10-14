//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"text/tabwriter"
	"text/template" // nosemgrep: go.lang.security.audit.xss.import-text-template.import-text-template -- CLI terminal output, not HTML.

	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

func printHelpWithWrappedFlagCategories(out io.Writer, templ string, data any, customFuncs map[string]any) error {
	const (
		helpNameTemplateWithWrap    = `{{$v := offset .FullName 6}}{{wrap .FullName 3}}{{if .Usage}} - {{wrap .Usage $v}}{{end}}`
		argsTemplateWithWrap        = `{{if .Arguments}}{{range .Arguments}}{{.Usage}} {{end}}{{end}}`
		usageTemplateWithWrap       = `{{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}}{{if .VisibleFlags}} [options]{{end}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} {{template "argsTemplate" .}}{{end}}{{end}}{{end}}`
		descriptionTemplateWithWrap = `{{wrap .Description 3}}`
		authorsTemplateWithWrap     = `{{with $length := len .Authors}}{{if ne 1 $length}}S{{end}}{{end}}:
   {{range $index, $author := .Authors}}{{if $index}}
   {{end}}{{$author}}{{end}}`
		visibleCommandTemplateWithWrap = `{{ $cv := offsetCommands .VisibleCommands 5}}{{range .VisibleCommands}}
   {{$s := join .Names ", "}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}}{{wrap .Usage $cv}}{{end}}`
		visibleCommandCategoryTemplateWithWrap = `{{range .VisibleCategories}}{{if .Name}}

   {{.Name}}:{{range .VisibleCommands}}
     {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{else}}{{template "visibleCommandTemplate" .}}{{end}}{{end}}`
		visibleFlagCategoryTemplateWithWrap = `{{range .VisibleFlagCategories}}
   {{if .Name}}{{.Name}}

   {{end}}{{$flglen := len .Flags}}{{range $i, $e := .Flags}}{{if eq (subtract $flglen $i) 1}}{{wrap $e.String 6}}
{{else}}{{wrap $e.String 6}}
   {{end}}{{end}}{{end}}`
		visibleFlagTemplateWithWrap = `{{range $i, $e := .VisibleFlags}}
   {{wrap $e.String 6}}{{end}}`
		visiblePersistentFlagTemplateWithWrap = `{{range $i, $e := .VisiblePersistentFlags}}
   {{wrap $e.String 6}}{{end}}`
		versionTemplateWithWrap = `{{if .Version}}{{if not .HideVersion}}

VERSION:
   {{.Version}}{{end}}{{end}}`
		copyrightTemplateWithWrap = `{{wrap .Copyright 3}}`
	)

	if customFuncs == nil {
		customFuncs = map[string]any{}
	}

	const maxLineLength = 10000

	funcMap := template.FuncMap{
		"join":           strings.Join,
		"subtract":       subtract,
		"indent":         indent,
		"nindent":        nindent,
		"trim":           strings.TrimSpace,
		"wrap":           func(input string, offset int) string { return wrap(input, offset, maxLineLength) },
		"offset":         offset,
		"offsetCommands": offsetCommands,
	}

	if wa, ok := customFuncs["wrapAt"]; ok {
		if wrapAtFunc, ok := wa.(func() int); ok {
			wrapAt := wrapAtFunc()
			customFuncs["wrap"] = func(input string, offset int) string {
				return wrap(input, offset, wrapAt)
			}
		}
	}

	maps.Copy(funcMap, customFuncs)

	writer := tabwriter.NewWriter(out, 1, 8, 2, ' ', 0)

	templateRoot, err := template.New("help").Funcs(funcMap).Parse(templ)
	if err != nil {
		return err
	}

	definitions := []struct {
		name string
		body string
	}{
		{name: "helpNameTemplate", body: helpNameTemplateWithWrap},
		{name: "argsTemplate", body: argsTemplateWithWrap},
		{name: "usageTemplate", body: usageTemplateWithWrap},
		{name: "descriptionTemplate", body: descriptionTemplateWithWrap},
		{name: "visibleCommandTemplate", body: visibleCommandTemplateWithWrap},
		{name: "copyrightTemplate", body: copyrightTemplateWithWrap},
		{name: "versionTemplate", body: versionTemplateWithWrap},
		{name: "visibleFlagCategoryTemplate", body: visibleFlagCategoryTemplateWithWrap},
		{name: "visibleFlagTemplate", body: visibleFlagTemplateWithWrap},
		{name: "visiblePersistentFlagTemplate", body: visiblePersistentFlagTemplateWithWrap},
		{name: "visibleGlobalFlagCategoryTemplate", body: strings.ReplaceAll(visibleFlagCategoryTemplateWithWrap, "OPTIONS", "GLOBAL OPTIONS")},
		{name: "authorsTemplate", body: authorsTemplateWithWrap},
		{name: "visibleCommandCategoryTemplate", body: visibleCommandCategoryTemplateWithWrap},
	}

	for _, definition := range definitions {
		if _, err := templateRoot.New(definition.name).Parse(definition.body); err != nil {
			return err
		}
	}

	if err := templateRoot.Execute(writer, data); err != nil {
		return err
	}

	return writer.Flush()
}

func subtract(a int, b int) int {
	return a - b
}

func indent(spaces int, value string) string {
	padding := strings.Repeat(" ", spaces)

	return padding + strings.ReplaceAll(value, "\n", "\n"+padding)
}

func nindent(spaces int, value string) string {
	return "\n" + indent(spaces, value)
}

func wrap(input string, offset int, wrapAt int) string {
	var lines []string

	padding := strings.Repeat(" ", offset)

	for lineIndex, line := range strings.Split(input, "\n") {
		if line == "" {
			lines = append(lines, line)

			continue
		}

		wrapped := wrapLine(line, offset, wrapAt, padding)
		if lineIndex == 0 {
			lines = append(lines, wrapped)

			continue
		}

		lines = append(lines, padding+wrapped)
	}

	return strings.Join(lines, "\n")
}

func wrapLine(input string, offset int, wrapAt int, padding string) string {
	if wrapAt <= offset || len(input) <= wrapAt-offset {
		return input
	}

	lineWidth := wrapAt - offset

	words := strings.Fields(input)
	if len(words) == 0 {
		return input
	}

	wrapped := words[0]
	spaceLeft := lineWidth - len(wrapped)

	var wrappedBuilder strings.Builder

	for _, word := range words[1:] {
		if len(word)+1 > spaceLeft {
			wrappedBuilder.WriteString("\n" + padding + word)
			spaceLeft = lineWidth - len(word)

			continue
		}

		wrappedBuilder.WriteString(" " + word)
		spaceLeft -= 1 + len(word)
	}

	wrapped += wrappedBuilder.String()

	return wrapped
}

func offset(input string, fixed int) int {
	return len(input) + fixed
}

func offsetCommands(commands []*cli.Command, fixed int) int {
	maxWidth := 0

	for _, command := range commands {
		name := strings.Join(command.Names(), ", ")
		if len(name) > maxWidth {
			maxWidth = len(name)
		}
	}

	return maxWidth + fixed
}

// descriptionExamplesMarker separates prose from the examples block inside
// a command's Description. Splitting on this lets help templates render the
// two parts as dedicated DESCRIPTION and EXAMPLES sections without changing
// any command's data model.
const descriptionExamplesMarker = "Examples:"

// descriptionBeforeExamples returns the prose portion of a Description,
// stripping any trailing whitespace that preceded the examples marker.
// If no marker is present, the whole Description is returned.
func descriptionBeforeExamples(description string, marker string) string {
	idx, _ := descriptionExamplesIndex(description, marker)
	if idx < 0 {
		return description
	}

	return strings.TrimRight(description[:idx], "\n ")
}

// examplesFromDescription returns the examples block that follows the
// "Examples:" marker in a Description, or "" if no marker is present.
// Leading whitespace on the first line is preserved so the rendered block
// keeps consistent indentation with continuation lines under urfave's wrap.
func examplesFromDescription(description string, marker string) string {
	idx, markerLength := descriptionExamplesIndex(description, marker)
	if idx < 0 {
		return ""
	}

	rest := strings.TrimRight(description[idx+markerLength:], "\n ")

	return strings.TrimLeft(rest, "\n")
}

func descriptionExamplesIndex(description string, marker string) (int, int) {
	if idx := strings.Index(description, marker); idx >= 0 {
		return idx, len(marker)
	}

	if marker != descriptionExamplesMarker {
		return strings.Index(description, descriptionExamplesMarker), len(descriptionExamplesMarker)
	}

	return -1, 0
}

// printAvailableCommands prints the list of available commands dynamically.
func printAvailableCommands(writer io.Writer, commands []*cli.Command) error {
	// Find the longest command name for alignment
	maxLen := 0

	for _, c := range commands {
		if len(c.Name) > maxLen {
			maxLen = len(c.Name)
		}
	}

	// Print each command with aligned usage
	for _, c := range commands {
		if _, err := fmt.Fprintf(writer, "  %-*s  %s\n", maxLen, c.Name, c.Usage); err != nil {
			return err
		}
	}

	return nil
}

// formatHeader formats section headers with terminal-safe bold when enabled.
func formatHeader(text string, useColor bool) string {
	if useColor {
		return fmt.Sprintf("\033[1m%s\033[0m", text)
	}

	return text
}

func rootHelpTemplate(localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	return fmt.Sprintf(`%s:
   {{template "helpNameTemplate" .}}

%s:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}} {{if .VisibleFlags}}[global options]{{end}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Version}}{{if not .HideVersion}}

%s:
   {{.Version}}{{end}}{{end}}{{if .Description}}{{with descriptionBeforeExamples .Description}}

%s:
   {{wrap . 3}}{{end}}{{with examplesFromDescription $.Description}}

%s:
   {{wrap . 3}}{{end}}{{end}}
{{- if len .Authors}}

%s:{{template "authorsTemplate" .}}{{end}}{{if .VisibleCommands}}

%s:{{template "visibleCommandCategoryTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

%s:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s:{{template "visibleFlagTemplate" .}}{{end}}

%s:
   man gommitlint
   https://codefloe.com/itiquette/gommitlint{{if .Copyright}}

%s:
   {{template "copyrightTemplate" .}}{{end}}
`, localizer.Get("NAME"), localizer.Get("USAGE"), localizer.Get("VERSION"), localizer.Get("DESCRIPTION"), localizer.Get("EXAMPLES"), localizer.Get("AUTHOR"), localizer.Get("COMMANDS"), localizer.Get("GLOBAL OPTIONS"), localizer.Get("GLOBAL OPTIONS"), localizer.Get("SEE ALSO"), localizer.Get("COPYRIGHT"))
}

// Documentation links rendered in help SEE ALSO sections. Deep links use
// Codefloe's rendered-browse URLs into the Antora docs tree; anchors are the
// Asciidoctor auto-ids on the rendered page, carrying Forgejo's user-content-
// prefix so they resolve as real element ids even without frontend JS.
const (
	repoURL        = "https://codefloe.com/itiquette/gommitlint"
	docsPagesURL   = repoURL + "/src/branch/main/docs/modules/ROOT/pages"
	commandsDocURL = docsPagesURL + "/reference/commands.adoc"
)

// commandSeeAlsoURLs returns the docs links shown in a command's help SEE ALSO
// section. The switch keeps this immutable lookup out of mutable package state.
func commandSeeAlsoURLs(fullName string) ([]string, bool) {
	switch fullName {
	case appName + " " + commandValidate:
		return []string{commandsDocURL + "#user-content-_gommitlint_validate", docsPagesURL + "/reference/rules.adoc"}, true
	case appName + " " + commandDoctor:
		return []string{commandsDocURL + "#user-content-_gommitlint_doctor", docsPagesURL + "/config-troubleshooting.adoc"}, true
	case appName + " " + commandConfig:
		return []string{commandsDocURL + "#user-content-_gommitlint_config", docsPagesURL + "/reference/configuration.adoc"}, true
	case appName + " " + commandHook:
		return []string{commandsDocURL + "#user-content-_gommitlint_hook", docsPagesURL + "/usecases/hook-lifecycle.adoc"}, true
	case appName + " " + completionCommandName:
		return []string{docsPagesURL + "/shell-completion.adoc"}, true
	default:
		return nil, false
	}
}

// seeAlsoLinks resolves the docs links for a command help screen. Unmatched
// full names walk up to their parent command so nested subcommands inherit
// the parent's links; the final fallback is the project home page.
func seeAlsoLinks(fullName string) []string {
	name := strings.TrimSpace(fullName)

	for name != "" {
		if links, ok := commandSeeAlsoURLs(name); ok {
			return links
		}

		idx := strings.LastIndex(name, " ")
		if idx < 0 {
			break
		}

		name = name[:idx]
	}

	return []string{repoURL}
}

// seeAlsoSection renders the SEE ALSO body for a command help screen: the
// in-CLI rule docs hint for validate, then the resolved docs links. Lines
// after the first carry the 3-space indent of a help section body; the
// template supplies the first line's indent.
func seeAlsoSection(fullName string, localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	var lines []string

	if strings.TrimSpace(fullName) == appName+" "+commandValidate {
		lines = append(lines, localizer.Get("Show docs for a rule: gommitlint validate --rule-help <name>"))
	}

	lines = append(lines, seeAlsoLinks(fullName)...)

	return strings.Join(lines, "\n   ")
}

func commandHelpTemplate(localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	return fmt.Sprintf(`%s:
   {{template "helpNameTemplate" .}}

%s:
   {{template "usageTemplate" .}}{{if .Category}}

%s:
   {{.Category}}{{end}}{{if .Description}}{{with descriptionBeforeExamples .Description}}

%s:
   {{wrap . 3}}{{end}}{{with examplesFromDescription $.Description}}

%s:
   {{wrap . 3}}{{end}}{{end}}{{if .VisibleFlagCategories}}

%s:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s:{{template "visibleFlagTemplate" .}}{{end}}{{if .VisiblePersistentFlags}}

%s:{{template "visiblePersistentFlagTemplate" .}}{{end}}

%s:
   {{seeAlso .FullName}}
`, localizer.Get("NAME"), localizer.Get("USAGE"), localizer.Get("CATEGORY"), localizer.Get("DESCRIPTION"), localizer.Get("EXAMPLES"), localizer.Get("OPTIONS"), localizer.Get("OPTIONS"), localizer.Get("GLOBAL OPTIONS"), localizer.Get("SEE ALSO"))
}

func subcommandHelpTemplate(localizer output.Localizer) string {
	localizer = output.EnsureLocalizer(localizer)

	return fmt.Sprintf(`%s:
   {{template "helpNameTemplate" .}}

%s:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.FullName}}{{if .VisibleCommands}} [command [command options]]{{end}}{{if .ArgsUsage}} {{.ArgsUsage}}{{else}}{{if .Arguments}} [arguments...]{{end}}{{end}}{{end}}{{if .Category}}

%s:
   {{.Category}}{{end}}{{if .Description}}{{with descriptionBeforeExamples .Description}}

%s:
   {{wrap . 3}}{{end}}{{with examplesFromDescription $.Description}}

%s:
   {{wrap . 3}}{{end}}{{end}}{{if .VisibleCommands}}

%s:{{template "visibleCommandTemplate" .}}{{end}}{{if .VisibleFlagCategories}}

%s:{{template "visibleFlagCategoryTemplate" .}}{{else if .VisibleFlags}}

%s:{{template "visibleFlagTemplate" .}}{{end}}

%s:
   {{seeAlso .FullName}}
`, localizer.Get("NAME"), localizer.Get("USAGE"), localizer.Get("CATEGORY"), localizer.Get("DESCRIPTION"), localizer.Get("EXAMPLES"), localizer.Get("COMMANDS"), localizer.Get("OPTIONS"), localizer.Get("OPTIONS"), localizer.Get("SEE ALSO"))
}

type helpOutputState struct {
	err error
}

func (s *helpOutputState) Err() error {
	if s == nil {
		return nil
	}

	return s.err
}

func (s *helpOutputState) record(err error) {
	if s.err == nil {
		s.err = err
	}
}

func configureHelpLocalization(localizer output.Localizer) *helpOutputState {
	localizer = output.EnsureLocalizer(localizer)
	examplesMarker := localizer.Get(descriptionExamplesMarker)
	state := &helpOutputState{}
	cli.HelpFlag = &cli.BoolFlag{
		Name:    commandHelp,
		Aliases: []string{"h"},
		Usage:   localizer.Get("show help"),
	}
	cli.UsageCommandHelp = localizer.Get("Shows a list of commands or help for one command")
	cli.RootCommandHelpTemplate = rootHelpTemplate(localizer)
	cli.CommandHelpTemplate = commandHelpTemplate(localizer)
	cli.SubcommandHelpTemplate = subcommandHelpTemplate(localizer)
	cli.HelpPrinterCustom = func(out io.Writer, templ string, data any, customFuncs map[string]any) {
		if customFuncs == nil {
			customFuncs = map[string]any{}
		}

		customFuncs["wrapAt"] = func() int {
			return 80
		}
		customFuncs["descriptionBeforeExamples"] = func(description string) string {
			return descriptionBeforeExamples(description, examplesMarker)
		}
		customFuncs["examplesFromDescription"] = func(description string) string {
			return examplesFromDescription(description, examplesMarker)
		}
		customFuncs["seeAlso"] = func(fullName string) string {
			return seeAlsoSection(fullName, localizer)
		}

		if err := printHelpWithWrappedFlagCategories(out, templ, data, customFuncs); err != nil {
			state.record(err)

			if os.Getenv("CLI_TEMPLATE_ERROR_DEBUG") != "" {
				_, _ = fmt.Fprintf(cli.ErrWriter, "CLI TEMPLATE ERROR: %#v\n", err)
			}
		}
	}

	return state
}

// showConciseHelp displays essential usage patterns for quick reference.
func showConciseHelp(writer io.Writer, localizer output.Localizer, useColor bool) {
	localizer = output.EnsureLocalizer(localizer)
	formatList := strings.Join(output.SupportedFormats(), ", ")
	logLevelList := strings.Join(logadapter.LogLevelNames(), ", ")
	helpText := localizer.Get(`gommitlint - git commit linter

%s
  gommitlint validate [--format FORMAT] [--count N] [options]

%s
  --format FORMAT       output format: %s
  --count N             validate last N commits
  --base-branch BRANCH  validate from branch to HEAD
  --log-level LEVEL     logging level: %s

%s
  # Validate current commit
  $ gommitlint validate
  ✓ abc1234 feat: add new feature
  ✓ All checks passed

  # Validate multiple commits
  $ gommitlint validate --count 3
  #1 ✓ abc1234 feat: add feature
  #2 ✓ def5678 fix: resolve bug
  #3 ✗ bad7890 bad commit message
  ✗ 1 issue found

  # CI/CD automation with JSON output
  $ gommitlint validate --format json | jq '.summary.allPassed'
  true

  # Validate feature branch commits
  $ gommitlint validate --base-branch main
  #1 ✓ abc1234 feat: new feature
  ✓ All checks passed

%s
  validate    validate commit messages (primary command)
  doctor      inspect configuration, hooks, and effective policy
  config      configuration operations  
  hook        manage Git hooks
  --version   print version information

Docs: https://docs.itiquette.org/gommitlint/
Issues: https://codefloe.com/itiquette/gommitlint/issues
Manual: man gommitlint

Full options: gommitlint --help
`, formatHeader(localizer.Get("USAGE"), useColor), formatHeader(localizer.Get("COMMON FLAGS"), useColor), formatList, logLevelList, formatHeader(localizer.Get("EXAMPLES"), useColor), formatHeader(localizer.Get("COMMANDS"), useColor))

	_, _ = fmt.Fprint(writer, helpText)
}
