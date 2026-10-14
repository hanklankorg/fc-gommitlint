// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package spell implements spell checking using the golangci/misspell library.
//
// Supported locales (case-insensitive):
//   - "both"      - Accept both US and UK spellings (default; recommended for
//     global teams). Catches genuine misspellings without flagging
//     cross-dialect words.
//   - "US"        - American English (color, analyze, organize). Flags British forms.
//   - "UK" / "GB" - British English (colour, analyse, organise). Flags American forms.
//   - ""          - Backward-compatible alias for "both".
package spell
