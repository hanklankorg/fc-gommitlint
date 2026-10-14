// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

// Package flags is the single owner of the CLI flag-name string constants
// shared between the cmd/gommitlint entrypoint and the internal command
// adapters. Both packages import these instead of re-declaring the literals,
// so a flag name is spelled exactly once and the two seams cannot drift apart.
package flags

// Flag names. These are the exact strings registered on the root command and
// its subcommands and passed to urfave/cli.
const (
	AllowCoredumps  = "allow-coredumps"
	BaseBranch      = "base-branch"
	Color           = "color"
	Config          = "config"
	Compact         = "compact"
	Count           = "count"
	ExcludeRule     = "exclude-rule"
	Force           = "force"
	Format          = "format"
	Global          = "global"
	IgnoreConfig    = "ignore-config"
	IgnoreRepoRules = "ignore-repo-rules"
	LogLevel        = "log-level"
	MessageFile     = "message-file"
	Output          = "output"
	Preset          = "preset"
	Quiet           = "quiet"
	Range           = "range"
	Ref             = "ref"
	ReportFile      = "report-file"
	RepoPath        = "repo-path"
	Rule            = "rule"
	RuleHelp        = "rule-help"
	Stdout          = "stdout"
	Theme           = "theme"
	Verbose         = "verbose"
)
