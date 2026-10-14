// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	logadapter "codefloe.com/itiquette/gommitlint/internal/adapters/logging"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// EffectiveConfig represents the resolved configuration with enabled rules.
type EffectiveConfig struct {
	Config         configTypes.Config                    `json:"config"          yaml:"config"`
	EnabledRules   []string                              `json:"enabled_rules"   yaml:"enabled_rules"`
	ConfigSource   string                                `json:"config_source"   yaml:"config_source"`
	SourceGraph    []configuration.ConfigSourceNode      `json:"source_graph"    yaml:"source_graph"`
	ConfigSources  []configuration.ConfigSource          `json:"config_sources"  yaml:"config_sources"`
	ManagedSources []configuration.ManagedSourceStatus   `json:"managed_sources" yaml:"managed_sources"`
	Provenance     []configuration.ConfigProvenanceEntry `json:"provenance"      yaml:"provenance"`
	Warnings       []string                              `json:"warnings"        yaml:"warnings"`
	SchemaVersion  int                                   `json:"schema_version"  yaml:"schema_version"`
}

// ExecuteConfigShow displays the current configuration.
func ExecuteConfigShow(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()

	if err := validateConfigSubcommandArgs(cmd, localizer, commandNameConfigShow); err != nil {
		return err
	}

	cfgResult, format, view, err := resolveConfigPresentation(cmd, localizer)
	if err != nil {
		return err
	}

	if err := showConfigWarnings(cmd.ErrWriter, false, configWarningDetails(cfgResult.Warnings), localizer); err != nil {
		return err
	}

	effectiveConfig, err := buildEffectiveConfigForDisplay(ctx, cfgResult, localizer)
	if err != nil {
		return err
	}

	switch format {
	case output.FormatJSON:
		err = printConfigJSON(effectiveConfig, cmd.Writer)
	case configTypes.ConfigFormatYAML:
		err = printConfigYAML(effectiveConfig, cmd.Writer, localizer)
	default:
		err = printConfigText(effectiveConfig, view, cmd.Writer, localizer)
	}

	if err != nil {
		if _, ok := errors.AsType[RecoveryError](err); ok {
			return err
		}

		return newRecoveryErrorWithCause(
			err,
			output.Msg("failed to print configuration"),
			helpSuggestion(localizer, "gommitlint config show"),
			localizer.Get("Check output destination"),
		)
	}

	return nil
}

func buildEffectiveConfigForDisplay(ctx context.Context, cfgResult ConfigResult, localizer output.Localizer) (EffectiveConfig, error) {
	cfg := cfgResult.Config
	logger := logadapter.GetDomainLogger(ctx)

	ruleFactory, err := createRuleFactory(cfg, logger, nil, nil, "")
	if err != nil {
		return EffectiveConfig{}, configDependencyError(localizer, err)
	}

	commitRules := ruleFactory.CreateCommitRules()
	repoRules := ruleFactory.CreateRepositoryRules()

	sanitizedCfg := cfg.Sanitized()
	managedSources := configuration.InspectManagedSources(syncConfigPath(cfgResult), sanitizedCfg, nil)
	provenance := configuration.InspectConfigProvenance(cfgResult.Sources, nil)
	cfgResultSanitized := ConfigResult{
		Config:      sanitizedCfg,
		SourceGraph: cfgResult.SourceGraph,
		Sources:     cfgResult.Sources,
		Warnings:    cfgResult.Warnings,
		loadMode:    cfgResult.loadMode,
	}
	effectiveConfig := BuildEffectiveConfig(sanitizedCfg, commitRules, repoRules, cfgResultSanitized, managedSources, localizer)
	effectiveConfig.Provenance = provenance

	return effectiveConfig, nil
}

// BuildEffectiveConfig creates an EffectiveConfig from components.
func BuildEffectiveConfig(cfg configTypes.Config, commitRules []domain.CommitRule, repoRules []domain.RepositoryRule, cfgResult ConfigResult, managedSources []configuration.ManagedSourceStatus, localizer output.Localizer) EffectiveConfig {
	// Collect enabled rule names
	enabledRules := make([]string, 0, len(commitRules)+len(repoRules))

	for _, rule := range commitRules {
		enabledRules = append(enabledRules, rule.Name())
	}

	for _, rule := range repoRules {
		enabledRules = append(enabledRules, rule.Name())
	}

	return EffectiveConfig{
		Config:         cfg,
		EnabledRules:   enabledRules,
		ConfigSource:   cfgResult.DisplaySource(),
		SourceGraph:    cfgResult.SourceGraph,
		ConfigSources:  cfgResult.Sources,
		ManagedSources: managedSources,
		Warnings:       output.FormatWarnings(configWarningDetails(cfgResult.Warnings), localizer),
		SchemaVersion:  1,
	}
}

func printConfigJSON(effectiveConfig EffectiveConfig, writer io.Writer) error {
	return writeIndentedJSON(writer, effectiveConfig, output.Msg("failed to encode config as JSON"))
}

func printConfigYAML(effectiveConfig EffectiveConfig, writer io.Writer, localizer output.Localizer) (returnErr error) {
	localizer = output.EnsureLocalizer(localizer)

	// For YAML output, we output the config wrapped in gommitlint: root key
	// Add header comment to indicate generated config
	if err := writeConfigHeaderLine(writer, "# "+localizer.Get("Gommitlint Configuration")); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, "# "+localizer.Get("Generated by: gommitlint config show --format=yaml")); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, "# "+localizer.Get("Configuration source: %s", output.CleanForTerminal(effectiveConfig.ConfigSource))); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, "#"); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, "# "+localizer.Get("Use this as a starting point for gommitlint.yaml")); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, "# "+localizer.Get("Customize values for your project as needed")); err != nil {
		return err
	}

	if err := writeConfigHeaderLine(writer, ""); err != nil {
		return err
	}

	// Wrap the config in the gommitlint root structure
	configWithRoot := map[string]any{
		gommitlintKey: effectiveConfig.Config,
	}

	encoder := newIndentedYAMLEncoder(writer)

	defer func() {
		if closeErr := encoder.Close(); closeErr != nil && returnErr == nil {
			returnErr = configOutputError(closeErr, output.Msg("failed to close YAML encoder"))
		}
	}()

	if err := encoder.Encode(configWithRoot); err != nil {
		return configOutputError(err, output.Msg("failed to encode config as YAML"))
	}

	return nil
}

func configOutputError(err error, message output.LocaleMsg) error {
	if _, ok := errors.AsType[RecoveryError](err); ok {
		return err
	}

	localizer := commandLocalizer()

	return newRecoveryErrorWithCause(
		errors.Join(ErrOutputWrite, err),
		message,
		localizer.Get("Check output destination"),
	)
}

func validateConfigView(format string, view configTextView, localizer output.Localizer) error {
	if format != output.FormatText {
		return nil
	}

	switch view {
	case configViewCompact, configViewVerbose:
		return nil
	default:
		return unsupportedConfigViewError(string(view), localizer)
	}
}

func unsupportedConfigViewError(view string, localizer output.Localizer) error {
	loc := output.EnsureLocalizer(localizer)

	return newRecoveryErrorWithCause(
		ErrUnsupportedConfigView,
		output.Msg("unsupported view '%s'", view),
		loc.Get("Valid views: compact, verbose"),
		loc.Get("Use --view to choose a supported text view"),
	)
}

func writeConfigHeaderLine(writer io.Writer, line string) error {
	if _, err := fmt.Fprintln(writer, line); err != nil {
		return configOutputError(err, output.Msg("failed to write header"))
	}

	return nil
}
