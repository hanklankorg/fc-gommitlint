// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"bytes"
	"context"
	"io"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"
)

// createTestConfigCommand creates a properly initialized CLI command for testing.
func createTestConfigCommand(name string, writer io.Writer) *cli.Command {
	cmd := &cli.Command{
		Name:   name,
		Writer: writer,
	}
	// parsedArgs will be nil in tests, but our Execute functions now handle that gracefully

	return cmd
}

func TestConfigCommand_HasCorrectStructure(t *testing.T) {
	t.Parallel()

	cmd := NewConfigCommand(nil)

	check.Equal(t, "config", cmd.Name)
	check.Equal(t, "configuration operations", cmd.Usage)
	check.NotEmpty(t, cmd.Description)
	check.Len(t, cmd.Commands, 5)

	// Check subcommands
	showCmd := cmd.Commands[0]
	check.Equal(t, "show", showCmd.Name)
	check.Equal(t, "show effective configuration", showCmd.Usage)
	check.NotNil(t, showCmd.Action)

	for _, description := range []string{cmd.Description, showCmd.Description} {
		check.Contains(t, description, "Explicit file (--config PATH)")
		check.Contains(t, description, "discovered user and project configs are not loaded")
		check.Contains(t, description, "Defaults only (--ignore-config)")
	}

	// Verify show command has format flag with correct options
	showFlagNames := make(map[string]bool)
	for _, flag := range showCmd.Flags {
		showFlagNames[flag.Names()[0]] = true
	}

	check.True(t, showFlagNames["format"], "config show should have --format flag")
	check.True(t, showFlagNames["view"], "config show should have --view flag")

	checkCmd := cmd.Commands[1]
	check.Equal(t, "check", checkCmd.Name)
	check.Equal(t, "check configuration health", checkCmd.Usage)
	check.NotNil(t, checkCmd.Action)

	checkFlagNames := make(map[string]bool)
	for _, flag := range checkCmd.Flags {
		checkFlagNames[flag.Names()[0]] = true
	}

	check.True(t, checkFlagNames["format"])
	check.True(t, checkFlagNames["view"])

	initCmd := cmd.Commands[2]
	check.Equal(t, "init", initCmd.Name)
	check.Equal(t, "create .gommitlint.yaml in the current directory", initCmd.Usage)
	check.NotNil(t, initCmd.Action)

	schemaCmd := cmd.Commands[3]
	check.Equal(t, "schema", schemaCmd.Name)
	check.Equal(t, "print the JSON Schema for the config file", schemaCmd.Usage)
	check.NotNil(t, schemaCmd.Action)

	schemaFlagNames := make(map[string]bool)
	for _, flag := range schemaCmd.Flags {
		schemaFlagNames[flag.Names()[0]] = true
	}

	check.True(t, schemaFlagNames["compact"], "config schema should have --compact flag")

	syncCmd := cmd.Commands[4]
	check.Equal(t, "sync", syncCmd.Name)
	check.Equal(t, "sync managed configuration sources", syncCmd.Usage)
	check.NotNil(t, syncCmd.Action)

	syncFlagNames := make(map[string]bool)
	for _, flag := range syncCmd.Flags {
		syncFlagNames[flag.Names()[0]] = true
	}

	check.True(t, syncFlagNames["source"])
	check.True(t, syncFlagNames["dry-run"])
	check.True(t, syncFlagNames["check"])
}

func TestConfigCommand_SyncHelpUsesSourcePlaceholder(t *testing.T) {
	var stdout bytes.Buffer

	cmd := &cli.Command{
		Name:     "gommitlint",
		Writer:   &stdout,
		Commands: []*cli.Command{NewConfigCommand(nil)},
	}

	err := cmd.Run(t.Context(), []string{"gommitlint", "config", "sync", "--help"})
	check.NoError(t, err)
	check.Contains(t, stdout.String(), "--source ID")
}

// Mock implementations for testing

type mockCommitRule struct {
	name string
}

func (m mockCommitRule) Name() string {
	return m.name
}

func (m mockCommitRule) Validate(_ domain.Commit) []domain.ValidationError {
	return nil
}

type mockRepoRule struct {
	name string
}

func (m mockRepoRule) Name() string {
	return m.name
}

func (m mockRepoRule) Validate(_ context.Context, _ domain.BranchAnalyzer) (string, []domain.ValidationError) {
	return "", nil
}

// Ensure mocks implement the interfaces.
var (
	_ domain.CommitRule     = (*mockCommitRule)(nil)
	_ domain.RepositoryRule = (*mockRepoRule)(nil)
)
