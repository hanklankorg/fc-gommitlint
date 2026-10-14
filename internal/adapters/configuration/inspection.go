// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"path/filepath"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Managed source status values used in config inspection output.
const (
	ManagedSourceStatusConfigured = "configured"
	ManagedSourceStatusInvalid    = "invalid"
	ManagedSourceStatusMissing    = "missing"
	ManagedSourceStatusVendored   = "vendored"
)

// Managed source integrity status values used in config inspection output.
const (
	ManagedSourceIntegrityMismatch = "mismatch"
	ManagedSourceIntegrityUnknown  = "unknown"
	ManagedSourceIntegrityVerified = "verified"
)

// ManagedSourceStatus describes the local state of a managed source.
type ManagedSourceStatus struct {
	ConfiguredSource string `json:"configured_source"          yaml:"configured_source"`
	Destination      string `json:"destination"                yaml:"destination"`
	EnvVar           string `json:"env_var,omitempty"          yaml:"env_var,omitempty"`
	FinalURL         string `json:"final_url,omitempty"        yaml:"final_url,omitempty"`
	FetchedAt        string `json:"fetched_at,omitempty"       yaml:"fetched_at,omitempty"`
	ID               string `json:"id"                         yaml:"id"`
	IntegritySHA256  string `json:"integrity_sha256,omitempty" yaml:"integrity_sha256,omitempty"`
	IntegrityStatus  string `json:"integrity_status"           yaml:"integrity_status"`
	NextStep         string `json:"next_step,omitempty"        yaml:"next_step,omitempty"`
	ResolvedFromEnv  bool   `json:"resolved_from_env"          yaml:"resolved_from_env"`
	Revision         string `json:"revision,omitempty"         yaml:"revision,omitempty"`
	Source           string `json:"source"                     yaml:"source"`
	SourceMode       string `json:"source_mode"                yaml:"source_mode"`
	Status           string `json:"status"                     yaml:"status"`
}

// InspectManagedSources reports local vendored state for configured managed sources.
func InspectManagedSources(configPath string, cfg configTypes.Config, fs FileSystem) []ManagedSourceStatus {
	if len(cfg.ManagedSources) == 0 {
		return nil
	}

	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	rootDir := filepath.Dir(filepath.Clean(configPath))
	statuses := make([]ManagedSourceStatus, 0, len(cfg.ManagedSources))

	for _, source := range cfg.ManagedSources {
		statuses = append(statuses, inspectManagedSource(rootDir, source, fs))
	}

	return statuses
}

func inspectManagedSource(rootDir string, source configTypes.ManagedSource, fs FileSystem) ManagedSourceStatus {
	sanitizedSource := configTypes.SanitizeManagedSourceURL(source.Source)
	status := ManagedSourceStatus{
		ConfiguredSource: sanitizedSource,
		Destination:      source.Destination,
		FinalURL:         configTypes.SanitizeManagedSourceURL(source.Resolved.FinalURL),
		FetchedAt:        source.Resolved.FetchedAt,
		ID:               source.ID,
		IntegritySHA256:  source.Integrity.SHA256,
		IntegrityStatus:  ManagedSourceIntegrityUnknown,
		Revision:         source.Resolved.Revision,
		Source:           sanitizedSource,
		SourceMode:       managedSourceMode(source.Source),
		Status:           ManagedSourceStatusConfigured,
		NextStep:         "gommitlint config sync --source " + source.ID,
	}

	if variable, ok := configTypes.ParseManagedSourceEnvReference(source.Source); ok {
		status.EnvVar = variable
		status.ResolvedFromEnv = source.Resolved.FinalURL != ""
	}

	destinationPath, err := resolveManagedSourceDestination(rootDir, source.Destination)
	if err != nil {
		status.Status = ManagedSourceStatusInvalid

		return status
	}

	status.Destination = destinationPath

	if _, err := fs.Stat(destinationPath); err != nil {
		status.Status = ManagedSourceStatusMissing

		return status
	}

	if parseResult := parseConfigFileStrict(destinationPath, fs); parseResult.Error != nil {
		status.Status = ManagedSourceStatusInvalid

		return status
	}

	if integrityStatus, err := inspectManagedSourceIntegrity(destinationPath, source, fs); err == nil {
		status.IntegrityStatus = integrityStatus
	}

	status.Status = ManagedSourceStatusVendored

	return status
}

func managedSourceMode(source string) string {
	if _, ok := configTypes.ParseManagedSourceEnvReference(source); ok {
		return "environment"
	}

	return "direct"
}

func inspectManagedSourceIntegrity(destinationPath string, source configTypes.ManagedSource, fs FileSystem) (string, error) {
	if source.Integrity.SHA256 == "" {
		return ManagedSourceIntegrityUnknown, nil
	}

	content, err := fs.ReadFile(destinationPath)
	if err != nil {
		return ManagedSourceIntegrityUnknown, err
	}

	if _, matches := managedSourceIntegrityMatches(source.Integrity.SHA256, content); !matches {
		return ManagedSourceIntegrityMismatch, nil
	}

	return ManagedSourceIntegrityVerified, nil
}

// FormatConfigSourceType returns a user-facing label for a config source type.
func FormatConfigSourceType(sourceType ConfigSourceType) string {
	switch sourceType {
	case SourceTypeExtended:
		return "extends"
	case SourceTypePreset:
		return "preset"
	case SourceTypeCLI:
		return "explicit"
	default:
		return string(sourceType)
	}
}
