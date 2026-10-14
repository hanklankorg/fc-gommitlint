// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package rules

import (
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/diagnostics"
)

type trailerLineKind int

const (
	trailerLineEmpty trailerLineKind = iota
	trailerLineContent
	trailerLineSignOff
	trailerLineSignOffLike
	trailerLineTrailer
	trailerLineRefs
)

type trailerLine struct {
	index int
	text  string
	kind  trailerLineKind
}

type trailerAnalysis struct {
	lines                 []trailerLine
	firstSignOffIndex     int
	lastSignOffIndex      int
	firstSignOffLikeIndex int
	lastRefsIndex         int
}

func analyzeTrailers(body string) trailerAnalysis {
	analysis := trailerAnalysis{
		firstSignOffIndex:     -1,
		lastSignOffIndex:      -1,
		firstSignOffLikeIndex: -1,
		lastRefsIndex:         -1,
	}

	for index, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		kind := classifyTrailerLine(trimmed)

		analysis.lines = append(analysis.lines, trailerLine{index: index, text: trimmed, kind: kind})

		switch kind {
		case trailerLineSignOff:
			if analysis.firstSignOffIndex == -1 {
				analysis.firstSignOffIndex = index
			}

			if analysis.firstSignOffLikeIndex == -1 {
				analysis.firstSignOffLikeIndex = index
			}

			analysis.lastSignOffIndex = index
		case trailerLineSignOffLike:
			if analysis.firstSignOffLikeIndex == -1 {
				analysis.firstSignOffLikeIndex = index
			}
		case trailerLineRefs:
			analysis.lastRefsIndex = index
		case trailerLineEmpty, trailerLineContent, trailerLineTrailer:
		}
	}

	return analysis
}

func classifyTrailerLine(trimmed string) trailerLineKind {
	switch {
	case trimmed == "":
		return trailerLineEmpty
	case SignOffPattern.MatchString(trimmed):
		return trailerLineSignOff
	case strings.HasPrefix(trimmed, "Signed-off-by:"):
		return trailerLineSignOffLike
	case strings.HasPrefix(trimmed, "Refs"):
		return trailerLineRefs
	case TrailerPattern.MatchString(trimmed):
		return trailerLineTrailer
	default:
		return trailerLineContent
	}
}

func (a trailerAnalysis) hasOnlySignOffLines() bool {
	hasSignOff := false

	for _, line := range a.lines {
		switch line.kind {
		case trailerLineEmpty:
			continue
		case trailerLineSignOff:
			hasSignOff = true
		case trailerLineContent, trailerLineSignOffLike, trailerLineTrailer, trailerLineRefs:
			return false
		}
	}

	return hasSignOff
}

func (a trailerAnalysis) signOffLines() []string {
	signoffs := make([]string, 0, len(a.lines))

	for _, line := range a.lines {
		if line.kind == trailerLineSignOff {
			signoffs = append(signoffs, line.text)
		}
	}

	return signoffs
}

func (a trailerAnalysis) signOffCandidates() []string {
	signoffs := make([]string, 0, len(a.lines))

	for _, line := range a.lines {
		if line.kind == trailerLineSignOff || line.kind == trailerLineSignOffLike {
			signoffs = append(signoffs, line.text)
		}
	}

	return signoffs
}

func (a trailerAnalysis) firstDisallowedLineAfterSignOffBlockStarts() string {
	if a.firstSignOffIndex == -1 {
		return ""
	}

	for _, line := range a.lines {
		if line.index <= a.firstSignOffIndex {
			continue
		}

		switch line.kind {
		case trailerLineEmpty, trailerLineSignOff, trailerLineSignOffLike:
			continue
		case trailerLineContent, trailerLineTrailer, trailerLineRefs:
			return line.text
		}
	}

	return ""
}

func (a trailerAnalysis) refsAfterFirstSignOff() bool {
	return a.firstSignOffLikeIndex >= 0 && a.lastRefsIndex > a.firstSignOffLikeIndex
}

func validateSignOffPlacement(ruleName, body, message, actual, expected string) []domain.ValidationError {
	if strings.TrimSpace(body) == "" {
		return nil
	}

	invalidLine := analyzeTrailers(body).firstDisallowedLineAfterSignOffBlockStarts()
	if invalidLine == "" {
		return nil
	}

	if actual == "" {
		actual = invalidLine
	}

	return []domain.ValidationError{
		domain.New(ruleName, diagnostics.ReasonMisplacedSignoff, message).
			WithActualExpected(actual, expected),
	}
}
