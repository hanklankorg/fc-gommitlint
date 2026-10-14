// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/parsers/yaml"
	koanf "github.com/knadh/koanf/v2"
	tomlencoder "github.com/pelletier/go-toml/v2"
	yamlv3 "gopkg.in/yaml.v3"
)

// Security constants to prevent attacks.
const (
	// MaxConfigSize limits configuration file size to prevent memory exhaustion.
	MaxConfigSize = 1024 * 1024 // 1MB - more than enough for any reasonable config

	parsePhaseValidate = "validate"
	configExtTOML      = ".toml"
	configExtYAML      = ".yaml"
	configExtYML       = ".yml"
)

// FileReader is a function type for reading file contents with a size limit.
type FileReader func(path string, maxSize int64) ([]byte, error)

// parserFromExtension returns the appropriate parser and tag name for a file extension.
func parserFromExtension(configPath string) (koanf.Parser, string, error) {
	ext := strings.ToLower(filepath.Ext(configPath))

	switch ext {
	case configExtTOML:
		return toml.Parser(), "toml", nil
	case configExtYAML, configExtYML:
		return yaml.Parser(), "yaml", nil
	default:
		return nil, "", ErrConfigFormat
	}
}

// mapProvider is a simple provider for parsed data.
type mapProvider struct {
	data map[string]any
}

func (p *mapProvider) ReadBytes() ([]byte, error) {
	return nil, nil // Not used
}

func (p *mapProvider) Read() (map[string]any, error) {
	return p.data, nil
}

// ParseConfigFileStrict parses a config file and returns detailed errors.
// Use this for explicitly specified config paths (e.g., --config) where
// any error should be fatal rather than silently falling back to defaults.
func ParseConfigFileStrict(configPath string) ParseResult {
	fs := filesystem.NewOSFileSystem()

	return ParseConfigFileStrictWithReader(configPath, fs.ReadFileWithLimit)
}

// ParseConfigFileStrictWithFS parses a config file with custom filesystem and returns errors.
func ParseConfigFileStrictWithFS(configPath string, fs FileSystem) ParseResult {
	return ParseConfigFileStrictWithReader(configPath, fs.ReadFileWithLimit)
}

// ParseConfigFileStrictWithReader parses a config file with custom reader and returns errors.
// This is the core strict parsing function that all other strict parsers delegate to.
func ParseConfigFileStrictWithReader(configPath string, reader FileReader) ParseResult {
	// Validate path is not empty
	if configPath == "" {
		return ParseResult{
			Error: &ParseError{
				Path:  configPath,
				Phase: parsePhaseValidate,
				Cause: ErrConfigEmpty,
			},
		}
	}

	// Read file content
	content, err := reader(configPath, MaxConfigSize)
	if err != nil {
		if errors.Is(err, filesystem.ErrFileTooLarge) {
			return ParseResult{
				Error: &ParseError{
					Path:    configPath,
					Phase:   parsePhaseValidate,
					Message: err.Error(),
					Cause:   ErrConfigTooLarge,
				},
			}
		}

		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "read",
				Message: err.Error(),
				Cause:   ErrConfigUnreadable,
			},
		}
	}

	return ParseConfigBytesStrict(configPath, content)
}

// ParseConfigBytesStrict parses config content from memory using the provided path for format detection.
func ParseConfigBytesStrict(configPath string, content []byte) ParseResult {
	// Empty or whitespace-only content is valid — it means "use defaults".
	if len(bytes.TrimSpace(content)) == 0 {
		return ParseResult{Config: configTypes.Config{}}
	}

	if int64(len(content)) > MaxConfigSize {
		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "validate",
				Message: fmt.Sprintf("file size %d bytes exceeds limit %d bytes", len(content), MaxConfigSize),
				Cause:   ErrConfigTooLarge,
			},
		}
	}

	ext := strings.ToLower(filepath.Ext(configPath))

	parser, tagName, err := parserFromExtension(configPath)
	if err != nil {
		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "validate",
				Message: fmt.Sprintf("unsupported format %q (use .yaml, .yml, or .toml)", ext),
				Cause:   ErrConfigFormat,
			},
		}
	}

	parsedData, err := parser.Unmarshal(content)
	if err != nil {
		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "parse",
				Message: err.Error(),
				Cause:   ErrConfigSyntax,
			},
		}
	}

	koanfConfig := koanf.New(".")
	if err := koanfConfig.Load(&mapProvider{data: parsedData}, nil); err != nil {
		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "load",
				Message: err.Error(),
				Cause:   ErrConfigStructure,
			},
		}
	}

	var cfg configTypes.Config
	if err := koanfConfig.UnmarshalWithConf("gommitlint", &cfg, koanf.UnmarshalConf{Tag: tagName}); err != nil {
		return ParseResult{
			Error: &ParseError{
				Path:    configPath,
				Phase:   "unmarshal",
				Message: err.Error(),
				Cause:   ErrConfigStructure,
			},
		}
	}

	// Schema diagnostics: the parser is forgiving (koanf silently ignores
	// unknown keys and a missing 'gommitlint:' root key loads as empty).
	// Walk the raw parsed map to surface those cases as warnings so users
	// catch typos ('subjct:' for 'subject:') and structural mismatches
	// instead of being mystified when their config has no effect.
	warnings := validateConfigSchema(parsedData, configPath, tagName)

	return ParseResult{Config: cfg, Warnings: warnings}
}

// validateConfigSchema emits warnings for structural issues in the parsed
// config map that the lenient unmarshal silently ignores.
//
// Catches two classes of misconfiguration:
//
//   - **Missing or wrong-shape root key** — file exists but doesn't have a
//     top-level `gommitlint:` mapping. Without this warning the loader treats
//     the file as empty (uses defaults) and the user can't tell.
//   - **Unknown keys under `gommitlint:`** — typos like `subjct:` or fields
//     from a future gommitlint version. Warning instead of error
//     preserves forward-compat (old binary + new config still works).
func validateConfigSchema(parsedData map[string]any, configPath string, tagName string) []domain.Warning {
	if len(parsedData) == 0 {
		return nil
	}

	raw, present := parsedData["gommitlint"]
	if !present {
		return []domain.Warning{{
			Code:    domain.WarnConfigBadShape,
			Message: configPath + ": top-level 'gommitlint:' key missing; config will be ignored",
			Context: map[string]string{
				domain.WarningContextPath:  configPath,
				domain.WarningContextIssue: "missing_root_key",
			},
		}}
	}

	gommitlintMap, ok := raw.(map[string]any)
	if !ok {
		return []domain.Warning{{
			Code:    domain.WarnConfigBadShape,
			Message: configPath + ": 'gommitlint:' value is not a mapping; config will be ignored",
			Context: map[string]string{
				domain.WarningContextPath:  configPath,
				domain.WarningContextIssue: "root_not_a_map",
			},
		}}
	}

	return unknownConfigKeyWarnings(gommitlintMap, reflect.TypeFor[configTypes.Config](), "", configPath, tagName)
}

func unknownConfigKeyWarnings(raw map[string]any, structType reflect.Type, prefix, configPath, tagName string) []domain.Warning {
	fields := make(map[string]reflect.Type, structType.NumField())

	for field := range structType.Fields() {
		tag := field.Tag.Get(tagName)
		if tag == "" || tag == "-" {
			continue
		}
		// Tags may carry options ("name,omitempty"); take just the name.
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			continue
		}

		fields[name] = field.Type
	}

	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	var warnings []domain.Warning

	for _, key := range keys {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		fieldType, known := fields[key]
		if !known {
			warnings = append(warnings, domain.Warning{
				Code:    domain.WarnConfigUnknownKey,
				Message: fmt.Sprintf("%s: unknown key 'gommitlint.%s' (ignored)", configPath, path),
				Context: map[string]string{
					domain.WarningContextPath:      configPath,
					domain.WarningContextConfigKey: path,
				},
			})

			continue
		}

		warnings = append(warnings, nestedUnknownConfigKeyWarnings(raw[key], fieldType, path, configPath, tagName)...)
	}

	return warnings
}

func nestedUnknownConfigKeyWarnings(raw any, fieldType reflect.Type, path, configPath, tagName string) []domain.Warning {
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}

	switch fieldType.Kind() {
	case reflect.Struct:
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil
		}

		return unknownConfigKeyWarnings(mapping, fieldType, path, configPath, tagName)
	case reflect.Slice, reflect.Array:
		items, ok := raw.([]any)
		if !ok {
			return nil
		}

		var warnings []domain.Warning

		for i, item := range items {
			itemPath := path + "[" + strconv.Itoa(i) + "]"
			warnings = append(warnings, nestedUnknownConfigKeyWarnings(item, fieldType.Elem(), itemPath, configPath, tagName)...)
		}

		return warnings
	case reflect.Map, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64,
		reflect.String, reflect.Interface:
		return nil
	default:
		return nil
	}
}

func encodeConfigWithManagedSources(configPath string, content []byte, sources []configTypes.ManagedSource) ([]byte, error) {
	root := make(map[string]any)

	switch strings.ToLower(filepath.Ext(configPath)) {
	case configExtYAML, configExtYML:
		if err := yamlv3.Unmarshal(content, &root); err != nil {
			return nil, err
		}
	case configExtTOML:
		if err := tomlencoder.Unmarshal(content, &root); err != nil {
			return nil, err
		}
	default:
		return nil, ErrConfigFormat
	}

	config, ok := root["gommitlint"].(map[string]any)
	if !ok {
		return nil, ErrConfigStructure
	}

	config["managed_sources"] = sources

	switch strings.ToLower(filepath.Ext(configPath)) {
	case configExtYAML, configExtYML:
		return yamlv3.Marshal(root)
	case configExtTOML:
		return tomlencoder.Marshal(root)
	default:
		return nil, ErrConfigFormat
	}
}
