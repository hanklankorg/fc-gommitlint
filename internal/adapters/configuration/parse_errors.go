// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

import (
	"errors"
	"fmt"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	configTypes "codefloe.com/itiquette/gommitlint/internal/domain/settings"
)

// Config parsing error types.
var (
	// ErrConfigUnreadable indicates the config file cannot be read.
	ErrConfigUnreadable = errors.New("cannot read config file")
	// ErrConfigTooLarge indicates the config file exceeds size limits.
	ErrConfigTooLarge = errors.New("config file exceeds size limit")
	// ErrConfigSyntax indicates a syntax error in the config file.
	ErrConfigSyntax = errors.New("config syntax error")
	// ErrConfigFormat indicates an unsupported config file format.
	ErrConfigFormat = errors.New("unsupported config format")
	// ErrConfigStructure indicates invalid config structure.
	ErrConfigStructure = errors.New("invalid config structure")
	// ErrConfigEmpty indicates an empty config path was provided.
	ErrConfigEmpty = errors.New("empty config path")
)

// ParseError represents a configuration parsing error with context.
type ParseError struct {
	Path    string // File path that caused the error
	Phase   string // Phase where error occurred: "validate", "read", "parse", "unmarshal"
	Message string // Human-readable error message
	Cause   error  // Underlying error (may be nil)
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	if e.Message != "" && e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v: %s", e.Path, e.Phase, e.Cause, e.Message)
	}

	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Path, e.Phase, e.Cause)
	}

	return fmt.Sprintf("%s: %s: %s", e.Path, e.Phase, e.Message)
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *ParseError) Unwrap() error {
	return e.Cause
}

// ParseResult holds the result of parsing a config file.
type ParseResult struct {
	// Config is the parsed configuration. Empty if parsing failed.
	Config configTypes.Config
	// Warnings contains non-fatal issues encountered during parsing.
	Warnings []domain.Warning
	// Error is the fatal parse error, if any.
	Error error
}

// HasError returns true if parsing encountered a fatal error.
func (r ParseResult) HasError() bool {
	return r.Error != nil
}

// HasWarnings returns true if parsing encountered non-fatal issues.
func (r ParseResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}
