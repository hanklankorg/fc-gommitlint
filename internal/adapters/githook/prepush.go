// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package githook adapts Git hook protocols to application requests.
package githook

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"github.com/go-git/go-git/v5/plumbing"
)

const maxPrePushProtocolLine = 4096

// ErrMalformedPrePushProtocol indicates invalid pre-push argv or stdin.
var ErrMalformedPrePushProtocol = errors.New("malformed pre-push protocol")

// DecodePrePush validates Git's two arguments and decodes its ref-update stream.
func DecodePrePush(args []string, input io.Reader) ([]domain.PrePushUpdate, error) {
	if len(args) != 2 {
		return nil, malformedPrePushError("expected 2 arguments, got %d", len(args))
	}

	if strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		return nil, malformedPrePushError("remote name and location must not be empty")
	}

	updates := make([]domain.PrePushUpdate, 0)
	scanner := bufio.NewScanner(input)
	// +1 so the constant means what it is named: bufio.Scanner treats this as
	// the maximum BUFFER size and needs room for the delimiter alongside the
	// token, so passing the limit unadjusted refuses a line of exactly
	// maxPrePushProtocolLine bytes.
	scanner.Buffer(make([]byte, 256), maxPrePushProtocolLine+1)

	objectIDWidth := 0

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		update, width, err := decodePrePushUpdate(scanner.Text(), lineNumber, objectIDWidth)
		if err != nil {
			return nil, err
		}

		objectIDWidth = width

		updates = append(updates, update)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: read updates: %w", ErrMalformedPrePushProtocol, err)
	}

	return updates, nil
}

func decodePrePushUpdate(line string, lineNumber int, objectIDWidth int) (domain.PrePushUpdate, int, error) {
	fields := strings.Split(line, " ")
	if len(fields) != 4 {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: expected 4 fields, got %d", lineNumber, len(fields))
	}

	localRef, localOID, remoteRef, remoteOID := fields[0], fields[1], fields[2], fields[3]

	if !isObjectID(localOID) || !isObjectID(remoteOID) || len(localOID) != len(remoteOID) {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: invalid object IDs", lineNumber)
	}

	if objectIDWidth == 0 {
		objectIDWidth = len(localOID)
	} else if len(localOID) != objectIDWidth {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: inconsistent object format", lineNumber)
	}

	deletion := isZeroObjectID(localOID)
	if deletion != (localRef == "(delete)") {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: deletion ref and object ID disagree", lineNumber)
	}

	if deletion && isZeroObjectID(remoteOID) {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: deletion has no remote object", lineNumber)
	}

	if !deletion && !isFullReference(localRef) {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: invalid local reference", lineNumber)
	}

	if !isFullReference(remoteRef) {
		return domain.PrePushUpdate{}, objectIDWidth, malformedPrePushError("line %d: invalid remote reference", lineNumber)
	}

	return domain.PrePushUpdate{
		LocalRef:  localRef,
		LocalOID:  localOID,
		RemoteRef: remoteRef,
		RemoteOID: remoteOID,
		Delete:    deletion,
	}, objectIDWidth, nil
}

func malformedPrePushError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformedPrePushProtocol, fmt.Sprintf(format, args...))
}

func isObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}

	for _, char := range value {
		if !isHexDigit(char) {
			return false
		}
	}

	return true
}

func isHexDigit(char rune) bool {
	return (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')
}

func isZeroObjectID(value string) bool {
	if !isObjectID(value) {
		return false
	}

	return strings.Trim(value, "0") == ""
}

func isFullReference(value string) bool {
	return strings.HasPrefix(value, "refs/") && plumbing.ReferenceName(value).Validate() == nil
}
