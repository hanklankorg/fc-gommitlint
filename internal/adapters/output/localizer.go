// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package output

import (
	"fmt"
)

// Localizer translates strings for output formatting.
// Implementations provide locale-aware translations; the noopLocalizer
// passes strings through unchanged (used as nil-safe fallback).
type Localizer interface {
	Get(str string, vars ...any) string
	GetN(str, plural string, n int, vars ...any) string
}

// LocaleMsg is a translatable message that bundles a message ID with optional format args.
// Use Msg() to construct one. At render time, call String() for unlocalized output
// or Localize(l) for translated output.
type LocaleMsg struct {
	ID   string
	Args []any
}

// Msg constructs a LocaleMsg from a message ID and optional format arguments.
func Msg(id string, args ...any) LocaleMsg {
	return LocaleMsg{ID: id, Args: args}
}

// String returns the message formatted without translation (suitable for Error() methods).
func (m LocaleMsg) String() string {
	if len(m.Args) == 0 {
		return m.ID
	}

	return fmt.Sprintf(m.ID, m.Args...)
}

// Localize returns the message translated and formatted via the given localizer.
func (m LocaleMsg) Localize(l Localizer) string {
	return EnsureLocalizer(l).Get(m.ID, m.Args...)
}

// noopLocalizer passes strings through with fmt.Sprintf formatting but no translation.
type noopLocalizer struct{}

func (noopLocalizer) Get(str string, vars ...any) string {
	if len(vars) == 0 {
		return str
	}

	return fmt.Sprintf(str, vars...)
}

func (noopLocalizer) GetN(str, plural string, n int, vars ...any) string {
	form := plural
	if n == 1 {
		form = str
	}

	if len(vars) == 0 {
		return form
	}

	return fmt.Sprintf(form, vars...)
}

func ensureLocalizer(localizer Localizer) Localizer {
	if localizer == nil {
		return noopLocalizer{}
	}

	return localizer
}

// EnsureLocalizer returns a non-nil localizer with fallback behavior.
func EnsureLocalizer(localizer Localizer) Localizer {
	return ensureLocalizer(localizer)
}
