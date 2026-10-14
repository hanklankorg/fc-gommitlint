// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"github.com/urfave/cli/v3"
)

type commandNotFoundState struct {
	localizer output.Localizer
	err       error
}

func newCommandNotFoundState(localizer output.Localizer) *commandNotFoundState {
	return &commandNotFoundState{localizer: output.EnsureLocalizer(localizer)}
}

func (s *commandNotFoundState) handle(ctx context.Context, cmd *cli.Command, command string) {
	if s.err == nil {
		s.err = handleCommandNotFound(ctx, cmd, command, s.localizer)
	}
}

func (s *commandNotFoundState) after(_ context.Context, _ *cli.Command) error {
	return s.take()
}

func (s *commandNotFoundState) take() error {
	err := s.err
	s.err = nil

	return err
}

// handleCommandNotFound provides helpful suggestions for command typos.
// Resolution order: curated alias map → urfave's Jaro-Winkler fuzzy match
// against the current command's subcommands → fall back to listing them.
func handleCommandNotFound(_ context.Context, cmd *cli.Command, command string, localizer output.Localizer) error {
	localizer = output.EnsureLocalizer(localizer)
	commandErr := fmt.Errorf("%w: %q", ErrUnknownCommand, command)
	commands := cmd.VisibleCommands()

	var message strings.Builder

	if suggestion, ok := lookupSuggestion(cmd, command); ok {
		message.WriteString(localizer.Get("Error: Unknown command %q\n\nDid you mean %q?\n\n", command, suggestion))
		message.WriteString(localizer.Get("Run 'gommitlint --help' for available commands\n"))
	} else {
		message.WriteString(localizer.Get("Error: Unknown command %q\n\n", command))
		message.WriteString(localizer.Get("Available commands:\n"))

		if err := printAvailableCommands(&message, commands); err != nil {
			return errors.Join(commandErr, fmt.Errorf("render available commands: %w", err))
		}

		message.WriteString(localizer.Get("\nRun 'gommitlint --help' for more information\n"))
	}

	if _, err := io.WriteString(cmd.ErrWriter, message.String()); err != nil {
		return errors.Join(commandErr, fmt.Errorf("write unknown-command diagnostic: %w", err))
	}

	return commandErr
}

// lookupSuggestion resolves a mistyped command to a full invocation string
// (e.g. "gommitlint config show"). The static alias map wins first; values
// there are absolute paths from the binary, so they get the program name
// prepended. Otherwise a lowercased Jaro-Winkler match against cmd's direct
// visible subcommands is tried; those are leaves under cmd, so they get
// cmd.FullName() prepended. Short and distant matches are rejected because
// urfave always returns the closest candidate, even when it is unrelated.
func lookupSuggestion(cmd *cli.Command, command string) (string, bool) {
	lower := strings.ToLower(command)

	if suggestion, found := commandSuggestions[lower]; found {
		return cmd.Root().Name + " " + suggestion, true
	}

	suggestion := cli.SuggestCommand(cmd.VisibleCommands(), lower)
	if len(suggestion) <= 1 || commandTypoDistance(lower, strings.ToLower(suggestion)) > maximumCommandTypoDistance(suggestion) {
		return "", false
	}

	return cmd.FullName() + " " + suggestion, true
}

func commandTypoDistance(provided string, candidate string) int {
	providedRunes := []rune(provided)
	candidateRunes := []rune(candidate)
	previousPrevious := make([]int, len(candidateRunes)+1)
	previous := make([]int, len(candidateRunes)+1)
	current := make([]int, len(candidateRunes)+1)

	for index := range previous {
		previous[index] = index
	}

	for providedIndex, providedRune := range providedRunes {
		current[0] = providedIndex + 1

		for candidateIndex, candidateRune := range candidateRunes {
			replacementCost := 0
			if providedRune != candidateRune {
				replacementCost = 1
			}

			column := candidateIndex + 1
			current[column] = min(
				previous[column]+1,
				current[column-1]+1,
				previous[column-1]+replacementCost,
			)

			if providedIndex > 0 && candidateIndex > 0 &&
				providedRune == candidateRunes[candidateIndex-1] &&
				providedRunes[providedIndex-1] == candidateRune {
				current[column] = min(current[column], previousPrevious[column-2]+1)
			}
		}

		previousPrevious, previous, current = previous, current, previousPrevious
	}

	return previous[len(candidateRunes)]
}

func maximumCommandTypoDistance(command string) int {
	// Longer names can sustain more edits without making a match ambiguous.
	switch length := len([]rune(command)); {
	case length >= 9:
		return 3
	case length >= 5:
		return 2
	default:
		return 1
	}
}

func flagSuggestionIsRelevant(provided string, suggestion string) bool {
	candidate := strings.TrimLeft(suggestion, "-")
	if candidate == "" {
		return false
	}

	return commandTypoDistance(strings.ToLower(provided), strings.ToLower(candidate)) <=
		maximumCommandTypoDistance(candidate)
}

// applyCommandNotFound recursively installs handleCommandNotFound on every
// command that has subcommands, so typos at any level get suggestions.
func applyCommandNotFound(cmd *cli.Command, handler cli.CommandNotFoundFunc) {
	if cmd == nil || len(cmd.Commands) == 0 {
		return
	}

	cmd.CommandNotFound = handler

	for _, sub := range cmd.Commands {
		applyCommandNotFound(sub, handler)
	}
}

// applySuggest recursively turns on urfave's built-in Suggest flag across the
// command tree. On each command, Suggest: true routes unknown *flag* errors
// through SuggestFlag (Jaro-Winkler). Root already has it set; this propagates
// to subcommands so e.g. `validate --base-brach` suggests `--base-branch`.
func applySuggest(cmd *cli.Command) {
	if cmd == nil {
		return
	}

	cmd.Suggest = true

	for _, sub := range cmd.Commands {
		applySuggest(sub)
	}
}

// commandSuggestions maps common typos and alternative command names to their correct commands.
// This helps users when they mistype or use alternative naming conventions.
//
//nolint:gochecknoglobals // Intentional package-level constant data structure
var commandSuggestions = map[string]string{
	// validate variations (most common)
	"validat":   commandValidate,
	"validae":   commandValidate,
	"validaet":  commandValidate,
	"val":       commandValidate,
	"valid":     commandValidate,
	"valida":    commandValidate,
	"validte":   commandValidate,
	"vaidate":   commandValidate,
	"vaildae":   commandValidate,
	"vvalidate": commandValidate,
	"validato":  commandValidate,
	"validate.": commandValidate,
	"valdiate":  commandValidate,
	"validaate": commandValidate,
	"vlaidate":  commandValidate,

	// config variations
	"conf":          commandConfig,
	"cfg":           commandConfig,
	"confi":         commandConfig,
	"cnofig":        commandConfig,
	"confgi":        commandConfig,
	"configure":     commandConfig,
	"configg":       commandConfig,
	"configuration": commandConfig,

	// hook variations
	"hok":     commandHook,
	"hoko":    commandHook,
	"hoook":   commandHook,
	"hooks":   commandHook,
	"install": "hook install",
	"setup":   "hook install",

	// common mistakes
	"check":     commandValidate,
	"lint":      commandValidate,
	commandTest: commandValidate,
	"verify":    commandValidate,
	"run":       commandValidate,
	"scan":      commandValidate,
	"analyze":   commandValidate,
	"analyse":   commandValidate,
	"audit":     commandValidate,
	"inspect":   commandValidate,
	"review":    commandValidate,
	"commit":    commandValidate,
	"ci":        commandValidate,
	"fix":       commandValidate,
	"correct":   commandValidate,

	// help variations
	"halp":   commandHelp, //nolint:misspell // Intentional typo mapping
	"hlep":   commandHelp,
	"hep":    commandHelp,
	"helpme": commandHelp,
	"?":      commandHelp,
	"info":   commandHelp,
	"usage":  commandHelp,
	"doc":    commandHelp,
	"docs":   commandHelp,
	"man":    commandHelp,
	"manual": commandHelp,
}
