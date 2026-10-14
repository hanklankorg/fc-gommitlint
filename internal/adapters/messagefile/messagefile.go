// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package messagefile reads commit message files and stdin input.
package messagefile

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"

	"golang.org/x/term"

	"codefloe.com/itiquette/gommitlint/internal/adapters/filesystem"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

var (
	// ErrStdinTerminalMode indicates stdin is interactive and would block.
	ErrStdinTerminalMode = errors.New("stdin is connected to a terminal, but no input was provided")
	// ErrCommitMessageTooLarge indicates stdin commit message content exceeds the safety limit.
	ErrCommitMessageTooLarge = errors.New("commit message file too large")
)

// TerminalDetector reports whether a file descriptor is a terminal.
type TerminalDetector func(fd int) bool

// FileReader reads bounded file content for commit message validation.
type FileReader interface {
	ReadFileWithLimit(path string, maxSize int64) ([]byte, error)
}

// Read reads a commit message from filePath or stdin when filePath is "-".
func Read(filePath string) (string, error) {
	return ReadWithFS(filePath, term.IsTerminal, os.Stdin, nil)
}

// ReadWithFS reads a commit message using an optional filesystem dependency.
func ReadWithFS(filePath string, isTerminal TerminalDetector, stdin io.Reader, fs FileReader) (string, error) {
	if filePath == "-" {
		return readStdin(isTerminal, stdin)
	}

	if fs == nil {
		fs = filesystem.NewOSFileSystem()
	}

	message, err := fs.ReadFileWithLimit(filePath, domain.MaxCommitMessageSize)
	if err != nil {
		return "", fmt.Errorf("failed to read message file: %w", err)
	}

	return string(message), nil
}

func readStdin(isTerminal TerminalDetector, stdin io.Reader) (string, error) {
	if file, ok := stdin.(*os.File); ok {
		fd := file.Fd()
		if fd <= math.MaxInt && isTerminal(int(fd)) {
			return "", fmt.Errorf("%w.\nTo read from stdin, pipe content to gommitlint:\n  echo \"feat: add feature\" | gommitlint validate -f -\nOr redirect from a file:\n  gommitlint validate -f - < commit-msg.txt", ErrStdinTerminalMode)
		}
	}

	message, err := readFromStdinWithLimit(stdin)
	if err != nil {
		return "", err
	}

	return message, nil
}

func readFromStdinWithLimit(stdin io.Reader) (string, error) {
	message, err := filesystem.ReadWithLimit(stdin, int64(domain.MaxCommitMessageSize))
	if err != nil {
		if errors.Is(err, filesystem.ErrFileTooLarge) {
			return "", ErrCommitMessageTooLarge
		}

		return "", fmt.Errorf("failed to read from stdin: %w", err)
	}

	return string(message), nil
}
