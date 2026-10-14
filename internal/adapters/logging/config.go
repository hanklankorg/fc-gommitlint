// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Log level string constants.
const (
	logLevelError   = "error"
	logLevelWarn    = "warn"
	logLevelInfo    = "info"
	logLevelDebug   = "debug"
	logLevelTrace   = "trace"
	logLevelWarning = "warning"
)

const (
	logFieldCaller  = "caller"
	logFieldCount   = "count"
	logFieldKey1    = "key1"
	logFieldLevel   = "level"
	logFieldMessage = "message"
	logFieldMsg     = "msg"
	logFieldTime    = "time"
)

// Log level names for CLI usage.
const (
	LogLevelErrorName = logLevelError
	LogLevelWarnName  = logLevelWarn
	LogLevelInfoName  = logLevelInfo
	LogLevelDebugName = logLevelDebug
	LogLevelTraceName = logLevelTrace
)

// LogLevelNames returns the supported log level names in display order.
func LogLevelNames() []string {
	return []string{
		LogLevelErrorName,
		LogLevelWarnName,
		LogLevelInfoName,
		LogLevelDebugName,
		LogLevelTraceName,
	}
}

// IsValidLogLevel reports whether level is an accepted --log-level value. It is
// the single owner of the log-level allowlist: it recognises the canonical
// names returned by LogLevelNames plus the undocumented "warning" / "information"
// aliases that parseLogLevel also tolerates. Matching is case-insensitive and
// ignores surrounding whitespace, mirroring parseLogLevel's normalization.
func IsValidLogLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case logLevelError,
		logLevelWarn, logLevelWarning,
		logLevelInfo, "information",
		logLevelDebug,
		logLevelTrace:
		return true
	default:
		return false
	}
}

// OutputFormat the logging output format.
type OutputFormat int

const (
	// OutputJSON formats logs as JSON.
	OutputJSON OutputFormat = iota
	// OutputConsole formats logs for console output with colors.
	OutputConsole
)

// LogLevel the logging level.
type LogLevel int

const (
	// LogLevelError shows only errors.
	LogLevelError LogLevel = iota
	// LogLevelWarn shows warnings and above.
	LogLevelWarn
	// LogLevelInfo shows info and above (default).
	LogLevelInfo
	// LogLevelDebug shows debug and above.
	LogLevelDebug
	// LogLevelTrace shows all levels including trace.
	LogLevelTrace
)

// Config holds logger configuration.
// Uses value semantics for immutability and functional composition.
type Config struct {
	// Format specifies log output format (JSON or console)
	// Options: OutputJSON, OutputConsole
	// Default: OutputConsole (with colors)
	Format OutputFormat
	// Level controls minimum log level to display
	// Hierarchy: Error < Warn < Info < Debug < Trace
	// Default: LogLevelInfo
	Level LogLevel
	// CallerInfo adds file:line information to log entries
	// Default: false (disabled for performance)
	CallerInfo bool
	// TimeFormat defines timestamp format in logs
	// Uses Go time format constants (time.RFC3339, time.DateTime)
	// Default: time.DateTime ("2006-01-02 15:04:05")
	TimeFormat string
}

// NewConfig creates a default logger configuration.
func NewConfig() Config {
	return Config{
		Format:     OutputConsole,
		Level:      LogLevelInfo,
		CallerInfo: false,
		TimeFormat: time.DateTime,
	}
}

// WithFormat sets the output format.
func (c Config) WithFormat(format OutputFormat) Config {
	c.Format = format

	return c
}

// WithLevel sets the log level.
func (c Config) WithLevel(level LogLevel) Config {
	c.Level = level

	return c
}

// WithCaller enables or disables caller information.
func (c Config) WithCaller(enabled bool) Config {
	c.CallerInfo = enabled

	return c
}

// WithTimeFormat sets the time format.
func (c Config) WithTimeFormat(format string) Config {
	c.TimeFormat = format

	return c
}

// ANSI color constants for log level formatting.
const (
	colorReset  = "\x1b[0m"
	colorGray   = "\x1b[90m"
	colorYellow = "\x1b[33m"
	colorRed    = "\x1b[31m"
	colorWhite  = "\x1b[37m"
	colorCyan   = "\x1b[36m"
)

// parseLogLevel converts string log level to LogLevel enum.
// Validates level hierarchy and ensures consistent parsing across components.
func parseLogLevel(level string) LogLevel {
	// Normalize input by trimming and converting to lowercase
	level = strings.ToLower(strings.TrimSpace(level))

	switch level {
	case logLevelError:
		return LogLevelError
	case logLevelWarn, logLevelWarning:
		return LogLevelWarn
	case logLevelInfo, "information":
		return LogLevelInfo
	case logLevelDebug:
		return LogLevelDebug
	case logLevelTrace:
		return LogLevelTrace
	default:
		// Invalid levels default to info for safety and predictability
		return LogLevelInfo
	}
}

// toZerologLevel converts LogLevel to zerolog.Level.
func (l LogLevel) toZerologLevel() zerolog.Level {
	switch l {
	case LogLevelError:
		return zerolog.ErrorLevel
	case LogLevelWarn:
		return zerolog.WarnLevel
	case LogLevelInfo:
		return zerolog.InfoLevel
	case LogLevelDebug:
		return zerolog.DebugLevel
	case LogLevelTrace:
		return zerolog.TraceLevel
	default:
		// Invalid levels default to info for safety
		return zerolog.InfoLevel
	}
}

// formatLevel formats log level for stderr output.
// Follows the git/go-toolchain idiom: user-facing diagnostics use lowercase
// prose ("warning:", "error:") at default verbosity; info is unlabeled so
// short status lines read as plain prose; debug/trace stay uppercase since
// they're developer-facing and benefit from a clear visual marker.
func formatLevel(levelVal any) string {
	return levelPrefix(levelVal, true)
}

// plainFormatLevel is formatLevel without escape sequences, for a redirected
// stderr or an explicit NO_COLOR / TERM=dumb opt-out. The console writer's own
// NoColor setting cannot reach this token, because the token is produced here.
func plainFormatLevel(levelVal any) string {
	return levelPrefix(levelVal, false)
}

func levelPrefix(levelVal any, useColor bool) string {
	levelStr, ok := levelVal.(string)
	if !ok {
		return fmt.Sprintf("%s", levelVal)
	}

	paint := func(color, text string) string {
		if !useColor {
			return text
		}

		return color + text + colorReset
	}

	switch levelStr {
	case logLevelTrace:
		return paint(colorCyan, "TRACE:")
	case logLevelDebug:
		return paint(colorGray, "DEBUG:")
	case logLevelInfo:
		return ""
	case logLevelWarn:
		return paint(colorYellow, "warning:")
	case logLevelError:
		return paint(colorRed, "error:")
	default:
		return paint(colorWhite, levelStr)
	}
}
