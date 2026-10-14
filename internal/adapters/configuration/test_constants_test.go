// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package configuration_test

const (
	testEnvXDGConfigHome = "XDG_CONFIG_HOME"
	testStrictConfigPath = "/test.yaml"
	testPhaseRead        = "read"
	testYAMLRoot         = `gommitlint:`

	testThemeDefault = "default"
	testThemeDracula = "dracula"
	testOutputText   = "text"

	testManagedTeamFile = ".gommitlint.team.yaml"
	testManagedTeamID   = "team-policy"
	testManagedTeamURL  = "https://example.test/policy.yaml"

	testConfigSourceDefaults = "defaults"
	testRepoBaseConfig       = "/repo/base.yaml"
	testRepoProjectConfig    = "/repo/.gommitlint.yaml"
	testReasonExplicitConfig = "explicitly set in config"
	testValueEnabled         = "enabled"
	testValueFalse           = "false"
)
