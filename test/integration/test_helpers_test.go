//go:build integration

// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package integration

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/urfave/cli/v3"

	cliadapter "codefloe.com/itiquette/gommitlint/internal/adapters/cli"
	"codefloe.com/itiquette/gommitlint/internal/adapters/cli/commands"
	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/adapters/output"
	"codefloe.com/itiquette/gommitlint/internal/adapters/rulefactory"
	"codefloe.com/itiquette/gommitlint/internal/adapters/spell"
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/domain/rulecatalog"
	settings "codefloe.com/itiquette/gommitlint/internal/domain/settings"
	"codefloe.com/itiquette/gommitlint/test/integration/gitrepo"
	"codefloe.com/itiquette/gommitlint/test/integration/testkit"
)

func TestMain(m *testing.M) {
	type previousEnv struct {
		key    string
		value  string
		wasSet bool
	}

	// The in-process CLI runs read the user's configuration through HOME and
	// XDG_CONFIG_HOME, and git and gpg read GIT_CONFIG_GLOBAL and GNUPGHOME, so
	// outside the sandbox launcher a host settings.yaml or ~/.gitconfig would
	// otherwise reach the scenarios that expect defaults. Cleanup is explicit
	// rather than deferred: os.Exit below skips defers.
	home, err := os.MkdirTemp("", "gommitlint-integration-home")
	if err != nil {
		panic("create temp HOME: " + err.Error())
	}

	isolated := map[string]string{
		"LC_ALL":              "C",
		"LANG":                "C",
		"LC_MESSAGES":         "C",
		"HOME":                home,
		"XDG_CONFIG_HOME":     filepath.Join(home, ".config"),
		"GIT_CONFIG_GLOBAL":   filepath.Join(home, ".gitconfig"),
		"GIT_CONFIG_NOSYSTEM": "1",
		"GNUPGHOME":           filepath.Join(home, ".gnupg"),
	}

	previous := make([]previousEnv, 0, len(isolated))
	for key, value := range isolated {
		old, wasSet := os.LookupEnv(key)
		previous = append(previous, previousEnv{key: key, value: old, wasSet: wasSet})
		_ = os.Setenv(key, value)
	}

	code := m.Run()

	for _, env := range previous {
		if env.wasSet {
			_ = os.Setenv(env.key, env.value)
		} else {
			_ = os.Unsetenv(env.key)
		}
	}

	_ = os.RemoveAll(home)

	os.Exit(code)
}

// mockLogger for integration tests.
type mockLogger struct{}

func (m *mockLogger) Log(_ string, _ string, _ ...any)   {}
func (m *mockLogger) Trace(_ string, _ ...any)           {}
func (m *mockLogger) TraceFunc(_ func() (string, []any)) {}
func (m *mockLogger) Debug(_ string, _ ...any)           {}
func (m *mockLogger) Info(_ string, _ ...any)            {}
func (m *mockLogger) Warn(_ string, _ ...any)            {}
func (m *mockLogger) Error(_ string, _ ...any)           {}

// ValidationResult represents the result of a validation operation.
type ValidationResult struct {
	Valid  bool
	Errors []domain.ValidationError
}

// ValidateCommitHelper for testing validation.
func ValidateCommitHelper(t *testing.T, repoPath string, config settings.Config) ValidationResult {
	t.Helper()

	ctx := context.Background()

	gitRepo, err := git.NewRepositoryFromPath(repoPath)
	check.NoError(t, err, "Failed to create git repository")

	spellChecker, err := spell.NewMisspellAdapter(settings.DerefString(config.Spell.Locale, settings.DefaultSpellLocale))
	check.NoError(t, err)

	mockLog := &mockLogger{}
	ruleFactory := rulefactory.NewFactory(config, spellChecker, mockLog, "")
	commitRules := ruleFactory.CreateCommitRules()

	commits, err := gitRepo.GetHeadCommits(ctx, 1)
	check.NoError(t, err, "Failed to get latest commit")
	check.Len(t, commits, 1, "Expected exactly one commit")

	commit := commits[0]

	// Validate commit
	logger := &mockLogger{}
	result := domain.ValidateCommit(commit, commitRules, config, logger)

	// Use errors directly from ValidationResult
	allErrors := result.Errors

	// Convert to simple result
	return ValidationResult{
		Valid:  len(allErrors) == 0,
		Errors: allErrors,
	}
}

// ValidateMessageHelper validates a commit message directly without requiring a git repository.
func ValidateMessageHelper(t *testing.T, message string, config settings.Config) ValidationResult {
	t.Helper()

	repo := gitrepo.NewRepoWithCommits(t, []gitrepo.CommitSpec{
		{Message: message},
	})

	return ValidateCommitHelper(t, repo.Path, config)
}

// DefaultConfig returns a sensible default configuration for testing.
func DefaultConfig() settings.Config {
	return settings.Config{
		Message: settings.MessageConfig{
			Subject: settings.SubjectConfig{
				Case:              new("ignore"),
				MaxLength:         new(72),
				RequireImperative: new(false),
				ForbidEndings:     []string{"."},
			},

			Body: settings.BodyConfig{
				Required:         new(true),
				MinLength:        new(10),
				AllowSignoffOnly: new(false),
				MinSignoffCount:  new(0),
			},
		},
		Conventional: settings.ConventionalConfig{
			DisallowBreaking:     new(false),
			MaxDescriptionLength: new(72),
			RequireScope:         new(false),
			Types:                []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"},
		},
		Rules: settings.RulesConfig{
			Enabled: []string{
				rulecatalog.Subject,
				rulecatalog.Conventional,
			},
			Disabled: []string{
				rulecatalog.CommitBody,
				rulecatalog.CryptoSignature,
				rulecatalog.SignOff,
				rulecatalog.Identity,
				rulecatalog.JiraReference,
				rulecatalog.Spell,
			},
		},
		CryptoSignature: settings.CryptoSignatureConfig{
			Required:       new(false),
			VerifyFormat:   new(false),
			KeyDirectory:   "",
			AllowedSigners: []string{},
		},
		Identity: settings.IdentityConfig{
			AllowedAuthors: []string{},
		},
		Repo: settings.RepoConfig{
			MaxCommitsAhead: new(10),
		},
		Output: output.FormatText,
		Spell: settings.SpellConfig{
			Locale:      new("US"),
			IgnoreWords: []string{},
		},
		Jira: settings.JiraConfig{
			ProjectPrefixes:      []string{},
			RequireInBody:        new(false),
			RequireInSubject:     new(false),
			IgnoreTicketPatterns: []string{},
		},
	}
}

// ComposeConfigs applies multiple configuration functions in sequence.
func ComposeConfigs(fns ...func(settings.Config) settings.Config) settings.Config {
	cfg := DefaultConfig()
	for _, fn := range fns {
		cfg = fn(cfg)
	}

	return cfg
}

// urfave/cli mutates command defaults and help state during app setup and run.
// RunCLICommand serializes that complete operation for all integration tests.
var cliExecutionMutex sync.Mutex

func lockDirectCLIExecution(t *testing.T) {
	t.Helper()
	cliExecutionMutex.Lock()
	t.Cleanup(cliExecutionMutex.Unlock)
}

// CreateFreshCLIApp assembles the commands under test from the same
// constructors main uses, so their subcommands and flags cannot drift from the
// product.
func CreateFreshCLIApp(repoPath string, outputBuffer *bytes.Buffer) *cli.Command {
	app := &cli.Command{
		Name: "gommitlint",
		Commands: []*cli.Command{
			commands.NewConfigCommand(nil),
			commands.NewValidateCommand(nil),
			commands.NewHookCommand(nil),
		},
		Flags:  cliadapter.NewGlobalFlags(nil, repoPath),
		Writer: outputBuffer,
	}

	setWriterRecursively(app, outputBuffer)
	attachInMemoryRepositoryFactory(app, repoPath)

	return app
}

// setWriterRecursively points every command in the tree at the test buffer.
func setWriterRecursively(cmd *cli.Command, writer *bytes.Buffer) {
	cmd.Writer = writer

	for _, sub := range cmd.Commands {
		setWriterRecursively(sub, writer)
	}
}

func attachInMemoryRepositoryFactory(app *cli.Command, repoPath string) {
	if strings.HasPrefix(repoPath, "mem://") {
		commands.SetVirtualRepositoryFactory(app, gitrepo.InMemoryFactory())
	}
}

// RunCLICommand runs an in-process CLI command under the package serialization lock.
// For in-memory repos (mem://), auto-injects --config to disable sign-off.
func RunCLICommand(t *testing.T, args []string, repoPath string) (string, error) {
	t.Helper()

	outputBuffer := &bytes.Buffer{}

	// For in-memory repos, inject --config so the config loader finds a
	// config with signoff disabled (since there's no real filesystem to search).
	if strings.HasPrefix(repoPath, "mem://") && !sliceContains(args, "--config") {
		configPath := testkit.InMemoryConfigPath(t)
		args = append(args, "--config", configPath)
	}

	ctx := context.Background()

	cliExecutionMutex.Lock()
	defer cliExecutionMutex.Unlock()

	app := CreateFreshCLIApp(repoPath, outputBuffer)
	err := app.Run(ctx, args)

	result := outputBuffer.String()

	return result, err
}

// sliceContains checks if a string slice contains a specific value.
func sliceContains(slice []string, val string) bool {
	return slices.Contains(slice, val)
}
