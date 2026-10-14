// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package check provides the project's test assertion vocabulary. Equality
// failures use go-cmp to render reviewable expected/actual diffs, and callers
// may supply cmp options for type-specific comparisons.
package check
