// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package commands

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"codefloe.com/itiquette/gommitlint/internal/adapters/gitcli"
)

type globalGitConfig struct{}

func getGlobalHooksPath(ctx context.Context) string {
	path, _ := readGlobalHooksPath(ctx)

	return path
}

func readGlobalHooksPath(ctx context.Context) (string, error) {
	return globalGitConfig{}.HooksPath(ctx)
}

func setGlobalHooksPath(ctx context.Context, path string) error {
	return globalGitConfig{}.SetHooksPath(ctx, path)
}

func unsetGlobalHooksPath(ctx context.Context) error {
	return globalGitConfig{}.UnsetHooksPath(ctx)
}

func (globalGitConfig) HooksPath(ctx context.Context) (string, error) {
	// --type=path expands a leading ~ the way git itself does when it reads the key.
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--type=path", "core.hooksPath")
	cmd.Env = gitcli.Environment()

	output, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(output) == 0 && len(exitErr.Stderr) == 0 {
			return "", nil
		}

		return "", fmt.Errorf("read global core.hooksPath: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (globalGitConfig) SetHooksPath(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "core.hooksPath", path) //nolint:gosec
	cmd.Env = gitcli.Environment()

	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return err
}

func (globalGitConfig) UnsetHooksPath(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "--unset", "core.hooksPath")
	cmd.Env = gitcli.Environment()

	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}

	return err
}
