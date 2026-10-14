// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration

// MapEnvironment implements EnvLookup using a map for testing.
// This type is exported for use in tests across packages.
type MapEnvironment struct {
	vars map[string]string
}

// NewMapEnvironment creates a new MapEnvironment with the given variables.
func NewMapEnvironment(vars map[string]string) *MapEnvironment {
	return &MapEnvironment{vars: vars}
}

// GetEnv returns the value of the environment variable from the map.
func (m *MapEnvironment) GetEnv(key string) string {
	return m.vars[key]
}
