// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	apphooks "codefloe.com/itiquette/gommitlint/internal/app/hooks"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

type prePushGitRunner func(context.Context, string, ...string) ([]byte, int, error)

const (
	prePushRevListCommand  = "rev-list"
	prePushRevParseCommand = "rev-parse"
	prePushReverseArg      = "--reverse"
	prePushTopoOrderArg    = "--topo-order"
)

var (
	errPrePushGitCommand        = errors.New("pre-push git command failed")
	errPrePushGitOutput         = errors.New("invalid pre-push git output")
	errPrePushLocalObject       = errors.New("pre-push local object cannot be resolved")
	errPrePushLocalRefNotCommit = errors.New("pre-push branch does not resolve to a commit")
	errPrePushRemoteObject      = errors.New("pre-push remote object cannot be resolved")
)

// PrePushResolver resolves only the commits introduced by a push.
type PrePushResolver struct {
	repoPath string
	run      prePushGitRunner
}

// The driven adapter implements the contract owned by its application consumer.
var _ apphooks.PrePushCommitResolver = (*PrePushResolver)(nil)

// NewPrePushResolver creates a resolver for one repository.
func NewPrePushResolver(repoPath string) *PrePushResolver {
	return &PrePushResolver{repoPath: repoPath, run: runPrePushGit}
}

// IntroducedCommits returns commits reachable from pushed tips but not from the
// destination refs' current tips. A zero remote OID contributes no exclusion,
// so a push to an empty remote includes root commits.
func (r PrePushResolver) IntroducedCommits(ctx context.Context, updates []domain.PrePushUpdate) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(updates) == 0 {
		return []string{}, nil
	}

	localOIDs, remoteOIDs, oidWidth, err := r.introducedRevisions(ctx, updates)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if len(localOIDs) == 0 {
		return []string{}, nil
	}

	args := []string{prePushRevListCommand, prePushReverseArg, prePushTopoOrderArg}
	args = append(args, localOIDs...)

	if len(remoteOIDs) > 0 {
		args = append(args, "--not")
		args = append(args, remoteOIDs...)
	}

	output, status, err := r.git(ctx, args...)
	if err != nil {
		return nil, err
	}

	if status != 0 {
		return nil, fmt.Errorf("%w: rev-list exit status %d", errPrePushGitCommand, status)
	}

	commits, err := parsePrePushCommitOutput(output, oidWidth)
	if err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return commits, nil
}

func (r PrePushResolver) introducedRevisions(ctx context.Context, updates []domain.PrePushUpdate) ([]string, []string, int, error) {
	resolved := make(map[string]string, len(updates))
	seenLocal := make(map[string]struct{}, len(updates))
	seenRemote := make(map[string]struct{}, len(updates))
	localOIDs := make([]string, 0, len(updates))
	remoteOIDs := make([]string, 0, len(updates))
	oidWidth := len(updates[0].LocalOID)

	for _, update := range updates {
		commitOID, ok := resolved[update.LocalOID]
		if !ok {
			var err error

			commitOID, err = r.resolveLocalCommit(ctx, update.LocalOID, oidWidth)
			if err != nil {
				return nil, nil, 0, err
			}

			resolved[update.LocalOID] = commitOID
		}

		if commitOID == "" {
			if strings.HasPrefix(update.LocalRef, "refs/heads/") {
				return nil, nil, 0, fmt.Errorf("%w: %s", errPrePushLocalRefNotCommit, update.LocalRef)
			}

			continue
		}

		if _, exists := seenLocal[commitOID]; !exists {
			seenLocal[commitOID] = struct{}{}
			localOIDs = append(localOIDs, commitOID)
		}

		if isZeroPrePushOID(update.RemoteOID) {
			continue
		}

		remoteCommitOID, ok := resolved[update.RemoteOID]
		if !ok {
			var resolveErr error

			remoteCommitOID, resolveErr = r.resolveCommit(ctx, update.RemoteOID, oidWidth, errPrePushRemoteObject)
			if resolveErr != nil {
				return nil, nil, 0, resolveErr
			}

			resolved[update.RemoteOID] = remoteCommitOID
		}

		if remoteCommitOID == "" {
			continue
		}

		if _, exists := seenRemote[remoteCommitOID]; !exists {
			seenRemote[remoteCommitOID] = struct{}{}
			remoteOIDs = append(remoteOIDs, remoteCommitOID)
		}
	}

	return localOIDs, remoteOIDs, oidWidth, nil
}

func (r PrePushResolver) resolveLocalCommit(ctx context.Context, oid string, oidWidth int) (string, error) {
	return r.resolveCommit(ctx, oid, oidWidth, errPrePushLocalObject)
}

func (r PrePushResolver) resolveCommit(ctx context.Context, oid string, oidWidth int, unresolvedErr error) (string, error) {
	output, status, err := r.git(ctx, prePushRevParseCommand, "--verify", gitEndOfOptions, oid+"^{commit}")
	if err != nil {
		return "", err
	}

	if status == 0 {
		return parseSinglePrePushOID(output, oidWidth)
	}

	output, status, err = r.git(ctx, prePushRevParseCommand, "--verify", gitEndOfOptions, oid+"^{}")
	if err != nil {
		return "", err
	}

	if status != 0 {
		return "", fmt.Errorf("%w: %s", unresolvedErr, oid)
	}

	if _, err := parseSinglePrePushOID(output, oidWidth); err != nil {
		return "", err
	}

	return "", nil
}

func parseSinglePrePushOID(output []byte, oidWidth int) (string, error) {
	oids, err := parsePrePushCommitOutput(output, oidWidth)
	if err != nil {
		return "", err
	}

	if len(oids) != 1 {
		return "", fmt.Errorf("%w: rev-parse returned %d object IDs", errPrePushGitOutput, len(oids))
	}

	return oids[0], nil
}

func parsePrePushCommitOutput(output []byte, oidWidth int) ([]string, error) {
	if len(output) == 0 {
		return []string{}, nil
	}

	text := strings.TrimSuffix(string(output), "\n")
	if text == "" {
		return []string{}, nil
	}

	commits := strings.Split(text, "\n")
	for _, oid := range commits {
		if !isPrePushOID(oid, oidWidth) {
			return nil, fmt.Errorf("%w: Git returned an invalid object ID", errPrePushGitOutput)
		}
	}

	return commits, nil
}

func isPrePushOID(oid string, width int) bool {
	if (width != 40 && width != 64) || len(oid) != width {
		return false
	}

	for _, char := range oid {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}

	return true
}

func isZeroPrePushOID(oid string) bool {
	return isPrePushOID(oid, len(oid)) && strings.Trim(oid, "0") == ""
}

func (r PrePushResolver) git(ctx context.Context, args ...string) ([]byte, int, error) {
	runner := r.run
	if runner == nil {
		runner = runPrePushGit
	}

	return runner(ctx, r.repoPath, args...)
}

func runPrePushGit(ctx context.Context, repoPath string, args ...string) ([]byte, int, error) {
	if repoPath == "" {
		return nil, -1, domain.ErrNotGitRepository
	}

	cmdArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...) //nolint:gosec // fixed executable; validated refs are separate argv values
	cmd.Env = Environment()

	// Output, not CombinedOutput: both callers parse this as git data when the
	// status is 0 -- a commit list, or a single resolved OID -- and git can
	// warn on stderr while exiting 0. Neither caller reads the output on a
	// non-zero status, so nothing is lost by leaving stderr on the ExitError.
	output, err := cmd.Output()
	if err == nil {
		return output, 0, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, -1, ctxErr
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return output, exitErr.ExitCode(), nil
	}

	return nil, -1, fmt.Errorf("run git: %w", err)
}
