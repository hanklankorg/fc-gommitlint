// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

func printConfigText(effectiveConfig EffectiveConfig, view configTextView, writer io.Writer, localizer output.Localizer) error {
	trackedWriter := &configTextErrorWriter{writer: writer}
	localizer = configTextLocalizer{Localizer: output.EnsureLocalizer(localizer)}

	// Print header with sources
	printHeader(trackedWriter, effectiveConfig.ConfigSource, effectiveConfig.ConfigSources, localizer)

	// Print enabled rules
	printEnabledRules(trackedWriter, effectiveConfig.EnabledRules, localizer)

	if view == configViewCompact {
		printManagedSourceSummaryOnly(trackedWriter, effectiveConfig.ManagedSources, localizer)
		printCompactConfigNextStep(trackedWriter, effectiveConfig.ManagedSources, localizer)

		if err := printFooter(trackedWriter, localizer); err != nil {
			return err
		}

		return nil
	}

	printSourceGraph(trackedWriter, effectiveConfig.SourceGraph, localizer)
	printProvenance(trackedWriter, effectiveConfig.Provenance, localizer)
	printManagedSources(trackedWriter, effectiveConfig.ManagedSources, localizer)

	// Print configuration sections
	printRulesConfig(trackedWriter, effectiveConfig.Config, localizer)
	printGeneralConfig(trackedWriter, effectiveConfig.Config, localizer)
	printMessageConfig(trackedWriter, effectiveConfig.Config, localizer)
	printConventionalConfig(trackedWriter, effectiveConfig.Config, localizer)
	printCryptoSignatureConfig(trackedWriter, effectiveConfig.Config, localizer)
	printIdentityConfig(trackedWriter, effectiveConfig.Config, localizer)
	printRepositoryConfig(trackedWriter, effectiveConfig.Config, localizer)
	printJiraConfig(trackedWriter, effectiveConfig.Config, localizer)
	printSpellConfig(trackedWriter, effectiveConfig.Config, localizer)
	printLimitsConfig(trackedWriter, effectiveConfig.Config, localizer)
	printOutputConfig(trackedWriter, effectiveConfig.Config, localizer)

	// Print footer
	if err := printFooter(trackedWriter, localizer); err != nil {
		return err
	}

	return nil
}

type configTextErrorWriter struct {
	writer io.Writer
	err    error
}

func (writer *configTextErrorWriter) Write(content []byte) (int, error) {
	if writer.err != nil {
		return 0, writer.err
	}

	written, err := writer.writer.Write(content)
	if err == nil && written != len(content) {
		err = io.ErrShortWrite
	}

	if err != nil {
		writer.err = err
	}

	return written, err
}

type configTextLocalizer struct {
	output.Localizer
}

func (localizer configTextLocalizer) Get(message string, args ...any) string {
	return localizer.Localizer.Get(message, sanitizeConfigTextArgs(args)...)
}

func (localizer configTextLocalizer) GetN(singular string, plural string, count int, args ...any) string {
	return localizer.Localizer.GetN(singular, plural, count, sanitizeConfigTextArgs(args)...)
}

func sanitizeConfigTextArgs(args []any) []any {
	sanitized := make([]any, len(args))
	for index, arg := range args {
		switch value := arg.(type) {
		case string:
			sanitized[index] = output.CleanForTerminal(value)
		case []string:
			sanitized[index] = sanitizedConfigTextStrings(value)
		default:
			sanitized[index] = arg
		}
	}

	return sanitized
}

func sanitizedConfigTextStrings(values []string) []string {
	sanitized := make([]string, len(values))
	for index, value := range values {
		sanitized[index] = output.CleanForTerminal(value)
	}

	return sanitized
}

func printManagedSourceSummaryOnly(output io.Writer, managedSources []configuration.ManagedSourceStatus, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Managed Sources:"))

	if len(managedSources) == 0 {
		_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("(none)"))
		_, _ = fmt.Fprintln(output)

		return
	}

	counts := make(map[string]int)

	for _, managedSource := range managedSources {
		status := managedSource.Status
		if managedSource.IntegrityStatus == configuration.ManagedSourceIntegrityMismatch {
			status = configuration.ManagedSourceIntegrityMismatch
		}

		counts[status]++
	}

	_, _ = fmt.Fprintf(output, "  %s\n\n", localizer.Get("Summary: %s", formatManagedSourceCounts(counts, localizer)))
}

func printCompactConfigNextStep(output io.Writer, managedSources []configuration.ManagedSourceStatus, localizer output.Localizer) {
	next := "gommitlint config check"

	for _, managedSource := range managedSources {
		if managedSource.NextStep != "" && managedSourceNeedsAction(managedSource) {
			next = managedSource.NextStep

			break
		}
	}

	_, _ = fmt.Fprintf(output, "%s\n\n", localizer.Get("Next: %s", next))
}

func printHeader(output io.Writer, configSource string, sources []configuration.ConfigSource, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Gommitlint Configuration"))
	_, _ = fmt.Fprintln(output, "========================")
	_, _ = fmt.Fprintln(output)

	// Show detailed sources if multiple config files were merged
	nonDefaultSources := countNonDefaultSources(sources)
	if nonDefaultSources > 1 {
		_, _ = fmt.Fprintln(output, localizer.Get("Configuration Sources (in precedence order):"))

		for idx, src := range sources {
			marker := ""
			if idx == len(sources)-1 && src.Type != configuration.SourceTypeDefaults {
				marker = " " + localizer.Get("← active")
			}

			_, _ = fmt.Fprintf(output, "  %d. %s (%s)%s\n", idx+1, safeConfigText(src.Path), localizedConfigSourceType(src.Type, localizer), marker)
		}
	} else {
		_, _ = fmt.Fprintf(output, "%s\n", localizer.Get("Configuration Source: %s", configSource))
	}

	_, _ = fmt.Fprintln(output)
}

func printManagedSources(output io.Writer, managedSources []configuration.ManagedSourceStatus, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Managed Sources:"))

	if len(managedSources) == 0 {
		_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("(none)"))
		_, _ = fmt.Fprintln(output)

		return
	}

	for _, managedSource := range managedSources {
		_, _ = fmt.Fprintf(output, "  - %s [%s]\n", safeConfigText(managedSource.ID), safeConfigText(managedSource.Status))
		_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Source: %s", managedSource.Source))
		_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Source mode: %s", managedSource.SourceMode))

		if managedSource.EnvVar != "" {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Env var: %s", managedSource.EnvVar))
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Configured source: %s", managedSource.ConfiguredSource))
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Resolved from env during last sync: %t", managedSource.ResolvedFromEnv))
		}

		_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Destination: %s", managedSource.Destination))

		if managedSource.FinalURL != "" {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Final URL: %s", managedSource.FinalURL))
		}

		if managedSource.Revision != "" {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Revision: %s", managedSource.Revision))
		}

		if managedSource.FetchedAt != "" {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Fetched at: %s", managedSource.FetchedAt))
		}

		_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Integrity: %s", managedSource.IntegrityStatus))
	}

	_, _ = fmt.Fprintln(output)
}

func printSourceGraph(output io.Writer, sourceGraph []configuration.ConfigSourceNode, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Configuration Graph:"))

	if len(sourceGraph) == 0 {
		_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("(none)"))
		_, _ = fmt.Fprintln(output)

		return
	}

	for _, node := range sourceGraph {
		printSourceGraphNode(output, node, 1, localizer)
	}

	_, _ = fmt.Fprintln(output)
}

func printSourceGraphNode(output io.Writer, node configuration.ConfigSourceNode, depth int, localizer output.Localizer) {
	indent := strings.Repeat("  ", depth)
	_, _ = fmt.Fprintf(output, "%s- %s (%s)\n", indent, safeConfigText(node.Path), localizedConfigSourceType(node.Type, localizer))

	for _, child := range node.Children {
		printSourceGraphNode(output, child, depth+1, localizer)
	}
}

func localizedConfigSourceType(sourceType configuration.ConfigSourceType, localizer output.Localizer) string {
	switch sourceType {
	case configuration.SourceTypeDefaults:
		return localizer.Get("defaults")
	case configuration.SourceTypeExtended:
		return localizer.Get("extends")
	case configuration.SourceTypePreset:
		return localizer.Get("preset")
	case configuration.SourceTypeGlobal:
		return localizer.Get("global")
	case configuration.SourceTypeProject:
		return localizer.Get("project")
	case configuration.SourceTypeCLI:
		return localizer.Get("explicit")
	default:
		return safeConfigText(string(sourceType))
	}
}

func printProvenance(output io.Writer, entries []configuration.ConfigProvenanceEntry, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Configuration Provenance:"))

	if len(entries) == 0 {
		_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("(none)"))
		_, _ = fmt.Fprintln(output)

		return
	}

	for _, entry := range entries {
		_, _ = fmt.Fprintf(output, "  %s = %s\n", safeConfigText(entry.Path), safeConfigText(entry.Value))
		_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Source: %s", entry.Source))

		if entry.Reason != "" {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Reason: %s", entry.Reason))
		}

		if len(entry.Overrides) > 0 {
			_, _ = fmt.Fprintf(output, "    %s\n", localizer.Get("Overrides:"))
			for _, override := range entry.Overrides {
				_, _ = fmt.Fprintf(output, "      - %s = %s\n", safeConfigText(override.Source), safeConfigText(override.Value))
			}
		}
	}

	_, _ = fmt.Fprintln(output)
}

// sortedCopy returns a sorted copy of items, leaving the input unchanged.
// Use at call sites that need to print a deterministically-ordered list so the
// localizer's printf template stays a constant literal (vet-checked).
func sortedCopy(items []string) []string {
	out := make([]string, len(items))
	copy(out, items)
	slices.Sort(out)

	return out
}

func safeConfigText(value string) string {
	return output.CleanForTerminal(value)
}

// fprintIndented writes "  <line>\n" to w. Use for any indented config line.
func fprintIndented(w io.Writer, line string) {
	_, _ = fmt.Fprintf(w, "  %s\n", line)
}

func countNonDefaultSources(sources []configuration.ConfigSource) int {
	count := 0

	for _, s := range sources {
		if s.Type != configuration.SourceTypeDefaults {
			count++
		}
	}

	return count
}

func printEnabledRules(out io.Writer, rules []string, localizer output.Localizer) {
	// Sort rules for consistent output
	rules = sortedCopy(rules)

	// Build an enabled-name set keyed by canonical rule ID.
	enabledNorm := make(map[string]bool, len(rules))
	for _, name := range rules {
		enabledNorm[configTypes.NormalizeRuleName(name)] = true
	}

	catalogRules := rulecatalog.Rules()

	var disabled []string

	for _, rule := range catalogRules {
		if enabledNorm[rule.ID] {
			continue
		}

		disabled = append(disabled, output.Msg(rule.DisplayName).Localize(localizer))
	}

	slices.Sort(disabled)

	_, _ = fmt.Fprintf(out, "%s\n", localizer.Get("Enabled Rules (%d of %d):", len(rules), len(catalogRules)))

	if len(rules) == 0 {
		_, _ = fmt.Fprintf(out, "  %s\n", localizer.Get("(none)"))
	} else {
		for _, rule := range rules {
			_, _ = fmt.Fprintf(out, "  - %s\n", safeConfigText(rule))
		}
	}

	if len(disabled) > 0 {
		_, _ = fmt.Fprintln(out)

		_, _ = fmt.Fprintf(out, "%s\n", localizer.Get("Disabled (not in active config):"))
		for _, rule := range disabled {
			_, _ = fmt.Fprintf(out, "  - %s\n", rule)
		}
	}

	_, _ = fmt.Fprintln(out)
}

func printRulesConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Rules Configuration:"))

	if len(cfg.Rules.Enabled) > 0 {
		fprintIndented(output, localizer.Get("Enabled: %v", sortedCopy(cfg.Rules.Enabled)))
	} else {
		fprintIndented(output, localizer.Get("Enabled: (none)"))
	}

	if len(cfg.Rules.Disabled) > 0 {
		fprintIndented(output, localizer.Get("Disabled: %v", sortedCopy(cfg.Rules.Disabled)))
	} else {
		fprintIndented(output, localizer.Get("Disabled: (none)"))
	}

	_, _ = fmt.Fprintln(output)
}

func printMessageConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Message Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Subject max length: %d", configTypes.DerefInt(cfg.Message.Subject.MaxLength, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Subject case: %s", configTypes.DerefString(cfg.Message.Subject.Case, configTypes.DefaultSubjectCase)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require imperative: %t", configTypes.DerefBool(cfg.Message.Subject.RequireImperative, false)))

	if len(cfg.Message.Subject.ForbidEndings) > 0 {
		fprintIndented(output, localizer.Get("Forbid endings: %v", sortedCopy(cfg.Message.Subject.ForbidEndings)))
	}

	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Body required: %t", configTypes.DerefBool(cfg.Message.Body.Required, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Body min length: %d", configTypes.DerefInt(cfg.Message.Body.MinLength, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Body max line length: %d", configTypes.DerefInt(cfg.Message.Body.MaxLineLength, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Allow signoff only: %t", configTypes.DerefBool(cfg.Message.Body.AllowSignoffOnly, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Min signoff count: %d", configTypes.DerefInt(cfg.Message.Body.MinSignoffCount, configTypes.DefaultMinSignoffCount)))
	_, _ = fmt.Fprintln(output)
}

func printConventionalConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Conventional Commit Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Allow multi-scope: %t", configTypes.DerefBool(cfg.Conventional.AllowMultiScope, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Disallow breaking changes: %t", configTypes.DerefBool(cfg.Conventional.DisallowBreaking, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Max description length: %d", configTypes.DerefInt(cfg.Conventional.MaxDescriptionLength, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require scope: %t", configTypes.DerefBool(cfg.Conventional.RequireScope, false)))

	if len(cfg.Conventional.Scopes) > 0 {
		fprintIndented(output, localizer.Get("Scopes: %v", sortedCopy(cfg.Conventional.Scopes)))
	}

	if len(cfg.Conventional.Types) > 0 {
		fprintIndented(output, localizer.Get("Types: %v", sortedCopy(cfg.Conventional.Types)))
	}

	_, _ = fmt.Fprintln(output)
}

func printCryptoSignatureConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Crypto Signature Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Required: %t", configTypes.DerefBool(cfg.CryptoSignature.Required, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Verify format: %t", configTypes.DerefBool(cfg.CryptoSignature.VerifyFormat, false)))

	if len(cfg.CryptoSignature.AllowedSigners) > 0 {
		fprintIndented(output, localizer.Get("Allowed signers: %v", sortedCopy(cfg.CryptoSignature.AllowedSigners)))
	}

	if cfg.CryptoSignature.KeyDirectory != "" {
		_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Key directory: %s", cfg.CryptoSignature.KeyDirectory))
	}

	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Min EC bits: %d", configTypes.DerefInt(cfg.CryptoSignature.MinECBits, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Min RSA bits: %d", configTypes.DerefInt(cfg.CryptoSignature.MinRSABits, 0)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require GPG: %t", configTypes.DerefBool(cfg.CryptoSignature.RequireGPG, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require SSH: %t", configTypes.DerefBool(cfg.CryptoSignature.RequireSSH, false)))

	_, _ = fmt.Fprintln(output)
}

func printIdentityConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Identity Configuration:"))

	if len(cfg.Identity.AllowedAuthors) > 0 {
		fprintIndented(output, localizer.Get("Allowed authors: %v", sortedCopy(cfg.Identity.AllowedAuthors)))
	} else {
		fprintIndented(output, localizer.Get("Allowed authors: (any)"))
	}

	_, _ = fmt.Fprintln(output)
}

func printRepositoryConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Repository Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Max commits ahead: %d", configTypes.DerefInt(cfg.Repo.MaxCommitsAhead, 0)))
	_, _ = fmt.Fprintln(output)
}

func printJiraConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("JIRA Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require in subject: %t", configTypes.DerefBool(cfg.Jira.RequireInSubject, false)))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Require in body: %t", configTypes.DerefBool(cfg.Jira.RequireInBody, false)))

	if len(cfg.Jira.ProjectPrefixes) > 0 {
		fprintIndented(output, localizer.Get("Project prefixes: %v", sortedCopy(cfg.Jira.ProjectPrefixes)))
	}

	if len(cfg.Jira.IgnoreTicketPatterns) > 0 {
		fprintIndented(output, localizer.Get("Ignore ticket patterns: %v", sortedCopy(cfg.Jira.IgnoreTicketPatterns)))
	}

	_, _ = fmt.Fprintln(output)
}

func printSpellConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Spell Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Locale: %s", configTypes.DerefString(cfg.Spell.Locale, configTypes.DefaultSpellLocale)))

	if len(cfg.Spell.IgnoreWords) > 0 {
		fprintIndented(output, localizer.Get("Ignore words: %v", sortedCopy(cfg.Spell.IgnoreWords)))
	}

	_, _ = fmt.Fprintln(output)
}

func printOutputConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Output Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Format: %s", cfg.Output))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Theme: %s", cfg.Theme))
	_, _ = fmt.Fprintln(output)
}

func printLimitsConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("Limits Configuration:"))
	maxCommitCount := configTypes.DerefInt(cfg.Limits.MaxCommitCount, configTypes.DefaultMaxCommitCount)
	maxKeyFiles := configTypes.DerefInt(cfg.Limits.MaxKeyFiles, configTypes.DefaultMaxKeyFiles)
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Max commit count: %d", maxCommitCount))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Max key files: %d", maxKeyFiles))
	_, _ = fmt.Fprintln(output)
}

func printGeneralConfig(output io.Writer, cfg configTypes.Config, localizer output.Localizer) {
	_, _ = fmt.Fprintln(output, localizer.Get("General Configuration:"))
	_, _ = fmt.Fprintf(output, "  %s\n", localizer.Get("Allow fixup commits: %t", configTypes.DerefBool(cfg.AllowFixupCommits, false)))
	_, _ = fmt.Fprintln(output)
}

func printFooter(writer io.Writer, localizer output.Localizer) error {
	if _, err := fmt.Fprintln(writer, localizer.Get("Use --format=json for machine-readable output")); err != nil {
		return configOutputError(err, output.Msg("failed to write output"))
	}

	if _, err := fmt.Fprintln(writer, localizer.Get("Use --format=yaml to generate a config file output")); err != nil {
		return configOutputError(err, output.Msg("failed to write output"))
	}

	return nil
}
