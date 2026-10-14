// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/adapters/configuration"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/urfave/cli/v3"
)

// flagNameTimeout bounds each managed-source download during config sync.
const flagNameTimeout = "timeout"

// ExecuteConfigSync fetches and vendors managed configuration sources.
func ExecuteConfigSync(ctx context.Context, cmd *cli.Command) error {
	localizer := commandLocalizer()

	if err := validateConfigSubcommandArgs(cmd, localizer, commandNameConfigSync); err != nil {
		return err
	}

	cfgResult, err := LoadConfigFromCommand(cmd.Root())
	if err != nil {
		return configLoadError(err, localizer)
	}

	configPath, err := requireSyncConfigPath(cfgResult, localizer)
	if err != nil {
		return err
	}

	result, err := configuration.SyncManagedSources(ctx, configPath, configuration.SyncOptions{
		CheckOnly: cmd.Bool("check"),
		DryRun:    cmd.Bool("dry-run"),
		SourceID:  cmd.String("source"),
		Timeout:   cmd.Duration(flagNameTimeout),
	})
	if err != nil {
		return configSyncError(localizer, err)
	}

	return writeConfigSyncOutput(cmd, localizer, result)
}

func requireSyncConfigPath(cfgResult ConfigResult, localizer output.Localizer) (string, error) {
	configPath := syncConfigPath(cfgResult)
	if configPath != "" {
		return configPath, nil
	}

	return "", newRecoveryErrorWithCause(
		configuration.ErrManagedSourcePath,
		output.Msg("no writable config file found for sync"),
		localizer.Get("Create a project config file: gommitlint config init"),
		localizer.Get("Select a specific config: gommitlint --config <path> config sync"),
		helpSuggestion(localizer, "gommitlint config sync"),
	)
}

func configSyncError(localizer output.Localizer, err error) error {
	details := configSyncErrorDetails(localizer, err)

	return newRecoveryErrorWithCause(err, details.message, details.suggestions...)
}

type configSyncRecoveryDetails struct {
	message     output.LocaleMsg
	suggestions []string
}

func configSyncErrorDetails(localizer output.Localizer, err error) configSyncRecoveryDetails {
	if integrityErr, ok := errors.AsType[*configuration.ManagedSourceIntegrityError](err); ok {
		return configSyncRecoveryDetails{
			message:     output.Msg("integrity mismatch for managed source %s", integrityErr.ID),
			suggestions: configSyncDescriptorSuggestions(localizer, configSyncErrorDescriptorFor(err)),
		}
	}

	if envErr, ok := errors.AsType[*configuration.ManagedSourceEnvError](err); ok {
		return configSyncRecoveryDetails{
			message: output.Msg("managed source environment variable is not set for %s (%s)", envErr.ID, envErr.Variable),
			suggestions: []string{
				localizer.Get("Set the referenced environment variable before running 'gommitlint config sync'"),
				localizer.Get("Source id: %s", envErr.ID),
				localizer.Get("Env var: %s", envErr.Variable),
				localizer.Get("Configured source: %s", envErr.ConfiguredSource),
				helpSuggestion(localizer, "gommitlint config sync --dry-run"),
			},
		}
	}

	if urlErr, ok := errors.AsType[*configuration.ManagedSourceURLError](err); ok {
		return configSyncRecoveryDetails{
			message: output.Msg("managed source URL is invalid for %s", urlErr.ID),
			suggestions: []string{
				localizer.Get("Use a direct HTTPS source URL or resolve the environment variable to one"),
				localizer.Get("Source id: %s", urlErr.ID),
				localizer.Get("Configured source: %s", configTypes.SanitizeManagedSourceURL(urlErr.ConfiguredSource)),
				localizer.Get("Resolved URL: %s", configTypes.SanitizeManagedSourceURL(urlErr.ResolvedURL)),
				helpSuggestion(localizer, "gommitlint config sync --dry-run"),
			},
		}
	}

	descriptor := configSyncErrorDescriptorFor(err)

	return configSyncRecoveryDetails{
		message:     descriptor.message,
		suggestions: configSyncDescriptorSuggestions(localizer, descriptor),
	}
}

type configSyncErrorDescriptor struct {
	target     error
	message    output.LocaleMsg
	suggestion output.LocaleMsg
}

func configSyncErrorDescriptorFor(err error) configSyncErrorDescriptor {
	descriptors := []configSyncErrorDescriptor{
		{target: configuration.ErrManagedSourceNotFound, message: output.Msg("managed source not found"), suggestion: output.Msg("Use 'gommitlint config show --format=yaml' to inspect configured managed source ids")},
		{target: configuration.ErrManagedSourceIntegrity, message: output.Msg("managed source integrity check failed"), suggestion: output.Msg("Review fetched content and update integrity.sha256 only if the change is trusted")},
		{target: configuration.ErrManagedSourceEnvNotSet, message: output.Msg("managed source environment variable is not set"), suggestion: output.Msg("Set the referenced environment variable before running 'gommitlint config sync'")},
		{target: configuration.ErrManagedSourceInvalidURL, message: output.Msg("managed source URL is invalid"), suggestion: output.Msg("Use a direct HTTPS source URL or resolve the environment variable to one")},
		{target: configuration.ErrManagedSourceOutOfDate, message: output.Msg("managed source is out of date"), suggestion: output.Msg("Run 'gommitlint config sync' to refresh vendored managed source files")},
		{target: configuration.ErrManagedSourcePath, message: output.Msg("managed source destination is invalid"), suggestion: output.Msg("Use a relative destination path inside the config directory")},
		{target: configuration.ErrManagedSourceUnexpectedHost, message: output.Msg("managed source redirect changed host"), suggestion: output.Msg("Use a direct HTTPS source URL or keep redirects on the same host")},
		{target: configuration.ErrManagedSourceFetchFailed, message: output.Msg("failed to fetch managed source")},
	}

	for _, descriptor := range descriptors {
		if errors.Is(err, descriptor.target) {
			return descriptor
		}
	}

	return configSyncErrorDescriptor{message: output.Msg("failed to sync managed sources"), suggestion: output.Msg("Check managed_sources configuration, URL reachability, and integrity.sha256 values")}
}

func configSyncDescriptorSuggestions(localizer output.Localizer, descriptor configSyncErrorDescriptor) []string {
	base := []string{helpSuggestion(localizer, "gommitlint config sync --dry-run")}
	if descriptor.suggestion.ID == "" {
		return base
	}

	return append([]string{descriptor.suggestion.Localize(localizer)}, base...)
}
func writeConfigSyncOutput(cmd *cli.Command, localizer output.Localizer, result configuration.ManagedSourceSyncResult) error {
	if len(result.Items) == 0 {
		if _, err := fmt.Fprintln(cmd.Writer, localizer.Get("No managed sources configured")); err != nil {
			return configOutputError(err, output.Msg("failed to write sync output"))
		}

		return nil
	}

	counts := make(map[string]int)
	for _, item := range result.Items {
		counts[item.Status]++
		if _, err := fmt.Fprintf(
			cmd.Writer,
			"- %s\n  %s\n  %s\n\n",
			output.CleanForTerminal(item.ID),
			localizer.Get("status: %s", output.CleanForTerminal(item.Status)),
			localizer.Get("destination: %s", output.CleanForTerminal(item.Destination)),
		); err != nil {
			return configOutputError(err, output.Msg("failed to write sync output"))
		}
	}

	if _, err := fmt.Fprintln(cmd.Writer, localizer.Get("Overall: %s", formatManagedSourceCounts(counts, localizer))); err != nil {
		return configOutputError(err, output.Msg("failed to write sync output"))
	}

	return nil
}
