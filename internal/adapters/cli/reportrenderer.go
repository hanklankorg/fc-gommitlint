// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package cli

import (
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// formatRenderer bundles the two rendering paths for one output format: the
// string-returning render (always present) and an optional writer-backed
// streamTo. A nil streamTo means the format only has the string path.
type formatRenderer struct {
	render   func(OutputOptions, domain.Report) string
	streamTo func(OutputOptions, domain.Report) error
}

// reportRenderers dispatches a format name to its renderer. It is the single
// dispatch table for output rendering; adding a format is one map entry.
// "forgejo" aliases "github" by sharing renderGitHub. Writer-backed formats
// carry a streamTo to avoid first converting output to a string. Text writes
// incrementally; the JSON and SARIF encoders still buffer complete documents.
//
//nolint:gochecknoglobals // Intentional package-level dispatch table
var reportRenderers = map[string]formatRenderer{
	output.FormatJSON:    {render: renderJSON, streamTo: streamJSON},
	output.FormatSARIF:   {render: renderSARIF, streamTo: streamSARIF},
	output.FormatText:    {render: renderText, streamTo: streamText},
	output.FormatGitHub:  {render: renderGitHub},
	output.FormatForgejo: {render: renderGitHub},
	output.FormatGitLab:  {render: renderGitLab},
	output.FormatPlain:   {render: renderPlain},
	output.FormatTable:   {render: renderTable},
}

// formatReport renders report according to o.Format, falling back to the
// text renderer for unknown formats (including the empty format).
func formatReport(o OutputOptions, report domain.Report) string {
	if renderer, ok := reportRenderers[o.Format]; ok {
		return renderer.render(o, report)
	}

	return renderText(o, report)
}

// writeReport formats report and writes it to o.Writer. In quiet mode it
// suppresses output for successful validations. Formats with a streamTo write
// straight to the writer; all others render to a string first.
func writeReport(opts OutputOptions, report domain.Report) error {
	if opts.Quiet && report.Summary.AllPassed {
		return nil
	}

	if renderer, ok := reportRenderers[opts.Format]; ok && renderer.streamTo != nil {
		return renderer.streamTo(opts, report)
	}

	_, err := opts.Writer.Write([]byte(formatReport(opts, report)))

	return err
}

func streamJSON(o OutputOptions, report domain.Report) error {
	return output.JSONTo(o.Writer, report, o.Localizer)
}

func streamSARIF(o OutputOptions, report domain.Report) error {
	return output.SARIFTo(o.Writer, report, sarifOptionsFrom(o))
}

func streamText(o OutputOptions, report domain.Report) error {
	return output.TextTo(o.Writer, report, textOptionsFrom(o))
}

// textOptionsFrom mirrors the conversion done in renderText (the non-streaming
// path). Extracted to keep the streaming and string-returning paths in sync —
// any new TextOptions field added later only needs to be wired here once.
func textOptionsFrom(opts OutputOptions) output.TextOptions {
	return output.TextOptions{
		VerboseLevel:            opts.VerboseLevel,
		ShowHelp:                opts.ShowHelp,
		ShowRuleHelp:            opts.RuleHelp != "",
		ShowRuleName:            true,
		ShowContext:             opts.VerboseLevel >= 1,
		Quiet:                   opts.Quiet,
		RuleHelpName:            opts.GetNormalizedRuleHelp(),
		UseColor:                opts.ShouldUseColor(),
		Theme:                   opts.Theme,
		Localizer:               opts.Localizer,
		ConfigSources:           opts.ConfigSources,
		ConfigSourcesUnexpected: opts.ConfigSourcesUnexpected,
	}
}

func renderJSON(o OutputOptions, report domain.Report) string {
	return output.JSON(report, o.Localizer)
}

func renderSARIF(o OutputOptions, report domain.Report) string {
	return output.SARIF(report, sarifOptionsFrom(o))
}

func sarifOptionsFrom(o OutputOptions) output.SARIFOptions {
	return output.SARIFOptions{ToolVersion: o.ToolVersion}
}

func renderGitHub(o OutputOptions, report domain.Report) string {
	return output.GitHub(report, output.GitHubOptions{
		AnnotationTitle: o.GetAnnotationTitle(),
		Localizer:       o.Localizer,
	})
}

func renderGitLab(o OutputOptions, report domain.Report) string {
	return output.GitLab(report, output.GitLabOptions{
		UseColor:  o.ShouldUseColor(),
		Theme:     o.Theme,
		Localizer: o.Localizer,
	})
}

func renderPlain(_ OutputOptions, report domain.Report) string {
	return output.TextPlain(report)
}

func renderTable(o OutputOptions, report domain.Report) string {
	return output.Table(report, output.TableOptions{
		Colors:    output.GetColorScheme(o.ShouldUseColor(), o.Theme),
		Localizer: o.Localizer,
	})
}

func renderText(opts OutputOptions, report domain.Report) string {
	return output.Text(report, textOptionsFrom(opts))
}
