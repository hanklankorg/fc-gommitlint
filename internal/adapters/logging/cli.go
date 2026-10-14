// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package logging

import (
	"context"
	"io"
	"math"
	"os"
	"time"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/rs/zerolog"
	"golang.org/x/term"
)

// InitLogger creates a configured zerolog instance from string parameters.
func InitLogger(ctx context.Context, outputFormat string, logLevel string) context.Context {
	parsedLevel := parseLogLevel(logLevel)
	config := NewConfig().
		WithLevel(parsedLevel).
		WithCaller(parsedLevel == LogLevelDebug)

	if outputFormat == "json" {
		config = config.WithFormat(OutputJSON).WithTimeFormat(time.RFC3339)
	} else {
		config = config.WithFormat(OutputConsole).WithTimeFormat(time.DateTime)
	}

	return InitLoggerWithConfig(ctx, config)
}

// InitLoggerWithConfig creates a configured zerolog instance from Config.
func InitLoggerWithConfig(ctx context.Context, config Config) context.Context {
	writerConfig := createWriterConfig(config)
	writer := applyWriterConfig(writerConfig)
	logger := createZerologger(writer, config)

	return logger.WithContext(ctx)
}

// GetLogger retrieves zerolog from context for adapter layer use.
func GetLogger(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// GetDomainLogger returns domain.Logger interface per hexagonal architecture pattern.
// GetDomainLogger retrieves the domain logger from the given context.
// It provides logging abstraction for the domain layer per hexagonal architecture.
func GetDomainLogger(ctx context.Context) domain.Logger {
	zerologInstance := GetLogger(ctx)

	return New(*zerologInstance)
}

// WriterType the type of writer to create.
type WriterType int

const (
	// WriterTypeStdout creates a writer that outputs to stdout.
	WriterTypeStdout WriterType = iota
	// WriterTypeConsole creates a console writer with formatting.
	WriterTypeConsole
)

// WriterConfig describes how to create a writer without actually creating it.
type WriterConfig struct {
	Type         WriterType
	Output       *os.File
	TimeFormat   string
	FormatLevel  func(any) string
	PartsExclude []string
	NoColor      bool
}

// termDumb marks a terminal that cannot render escape sequences.
const termDumb = "dumb"

// logColorEnabled mirrors the CLI colour policy for log output. Logs have no
// --color flag of their own, so they follow the environment: NO_COLOR and
// TERM=dumb win outright (no-color.org), and otherwise colour is used only
// when the sink really is a terminal. Without this a redirected stderr
// collected any escape the console writer felt like emitting.
func logColorEnabled(out *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	if os.Getenv("TERM") == termDumb {
		return false
	}

	fd := out.Fd()
	if fd > math.MaxInt {
		return false
	}

	return term.IsTerminal(int(fd))
}

// createWriterConfig returns configuration data for writer creation.
// At default verbosity (info and below) timestamps are suppressed so stderr
// reads as plain diagnostics; at debug/trace the full structured shape is
// kept since developers want it.
func createWriterConfig(config Config) WriterConfig {
	if config.Format == OutputJSON {
		return WriterConfig{
			Type:   WriterTypeStdout,
			Output: os.Stderr,
		}
	}

	partsExclude := []string{zerolog.CallerFieldName}
	if config.Level < LogLevelDebug {
		partsExclude = append(partsExclude, zerolog.TimestampFieldName)
	}

	useColor := logColorEnabled(os.Stderr)
	levelFormatter := plainFormatLevel

	if useColor {
		levelFormatter = formatLevel
	}

	return WriterConfig{
		Type:         WriterTypeConsole,
		Output:       os.Stderr,
		TimeFormat:   config.TimeFormat,
		FormatLevel:  levelFormatter,
		PartsExclude: partsExclude,
		NoColor:      !useColor,
	}
}

// applyWriterConfig applies the configuration and creates the actual writer.
func applyWriterConfig(cfg WriterConfig) io.Writer {
	switch cfg.Type {
	case WriterTypeStdout:
		return cfg.Output
	case WriterTypeConsole:
		return zerolog.ConsoleWriter{
			Out:          cfg.Output,
			TimeFormat:   cfg.TimeFormat,
			FormatLevel:  cfg.FormatLevel,
			NoColor:      cfg.NoColor,
			PartsExclude: cfg.PartsExclude,
		}
	default:
		return os.Stdout
	}
}

// createZerologger creates configured zerolog instance.
func createZerologger(writer io.Writer, config Config) zerolog.Logger {
	loggerContext := zerolog.New(writer).
		Level(config.Level.toZerologLevel()).
		With()

	// For JSON output, the hook below writes the timestamp in the configured
	// format; adding Context.Timestamp() as well would emit a second "time"
	// key in every line (the global zerolog format next to the custom one).
	if config.Format == OutputJSON && config.TimeFormat != "" {
		logger := loggerContext.Logger()
		logger = logger.Hook(zerolog.HookFunc(func(e *zerolog.Event, _ zerolog.Level, _ string) {
			if config.TimeFormat == time.RFC3339 {
				e.Time(zerolog.TimestampFieldName, time.Now().UTC())
			} else {
				e.Str(zerolog.TimestampFieldName, time.Now().Format(config.TimeFormat))
			}
		}))

		// Add caller info if requested
		if config.CallerInfo {
			logger = logger.With().Caller().Logger()
		}

		return logger
	}

	// For console output, timestamp is handled by ConsoleWriter
	loggerContext = loggerContext.Timestamp()

	// Add caller info if requested
	if config.CallerInfo {
		loggerContext = loggerContext.Caller()
	}

	return loggerContext.Logger()
}
