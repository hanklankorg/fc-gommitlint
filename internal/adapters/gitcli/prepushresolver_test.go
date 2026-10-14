// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package gitcli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/domain"
	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

var errPrePushGit = errors.New("git unavailable")

func TestPrePushResolver_IntroducedCommitsPeelsUniqueCommitTips(t *testing.T) {
	t.Parallel()

	localHead := strings.Repeat("a", 40)
	annotatedTag := strings.Repeat("b", 40)
	lightweightTag := strings.Repeat("c", 40)
	peeledTag := strings.Repeat("d", 40)
	remoteHead := strings.Repeat("1", 40)
	remoteTag := strings.Repeat("2", 40)
	remoteCustom := strings.Repeat("3", 40)
	zero := strings.Repeat("0", 40)

	var calls [][]string

	resolver := PrePushResolver{
		repoPath: "/repo",
		run: func(_ context.Context, repoPath string, args ...string) ([]byte, int, error) {
			check.Equal(t, "/repo", repoPath)

			calls = append(calls, append([]string(nil), args...))
			switch args[0] {
			case prePushRevParseCommand:
				switch args[3] {
				case localHead + "^{commit}":
					return []byte(localHead + "\n"), 0, nil
				case remoteHead + "^{commit}":
					return []byte(remoteHead + "\n"), 0, nil
				case annotatedTag + "^{commit}":
					return []byte(peeledTag + "\n"), 0, nil
				case remoteTag + "^{commit}":
					return []byte(remoteTag + "\n"), 0, nil
				case lightweightTag + "^{commit}":
					return []byte(lightweightTag + "\n"), 0, nil
				case remoteCustom + "^{commit}":
					return []byte(remoteCustom + "\n"), 0, nil
				}
			case prePushRevListCommand:
				return []byte(localHead + "\n" + peeledTag + "\n" + lightweightTag + "\n"), 0, nil
			}

			t.Fatalf("unexpected git arguments: %v", args)

			return nil, -1, nil
		},
	}

	commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{
		{LocalRef: "refs/heads/main", LocalOID: localHead, RemoteOID: remoteHead},
		{LocalRef: "refs/tags/v1", LocalOID: annotatedTag, RemoteOID: remoteTag},
		{LocalRef: "refs/tags/v2", LocalOID: lightweightTag, RemoteOID: zero},
		{LocalRef: "refs/custom/release", LocalOID: annotatedTag, RemoteOID: remoteCustom},
	})

	check.NoError(t, err)
	check.Equal(t, []string{localHead, peeledTag, lightweightTag}, commits)
	check.Equal(t, [][]string{
		{prePushRevParseCommand, "--verify", gitEndOfOptions, localHead + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteHead + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, annotatedTag + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteTag + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, lightweightTag + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteCustom + "^{commit}"},
		{prePushRevListCommand, prePushReverseArg, prePushTopoOrderArg, localHead, peeledTag, lightweightTag, "--not", remoteHead, remoteTag, remoteCustom},
	}, calls)
}

func TestPrePushResolver_IntroducedCommitsNewRefIncludesRootHistory(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 64)
	zero := strings.Repeat("0", 64)

	var calls [][]string

	resolver := PrePushResolver{
		repoPath: "/repo",
		run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
			calls = append(calls, append([]string(nil), args...))
			if args[0] == prePushRevParseCommand {
				return []byte(local + "\n"), 0, nil
			}

			return nil, 0, nil
		},
	}

	commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
		LocalRef: "refs/heads/main", LocalOID: local, RemoteOID: zero,
	}})

	check.NoError(t, err)
	check.Empty(t, commits)
	check.Equal(t, [][]string{
		{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{commit}"},
		{prePushRevListCommand, prePushReverseArg, prePushTopoOrderArg, local},
	}, calls)
}

func TestPrePushResolver_IntroducedCommitsResolvesRemoteExclusionsToCommits(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	remoteTag := strings.Repeat("b", 40)
	remoteCommit := strings.Repeat("c", 40)

	var calls [][]string

	resolver := PrePushResolver{
		repoPath: "/repo",
		run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
			calls = append(calls, append([]string(nil), args...))

			switch args[0] {
			case prePushRevParseCommand:
				switch args[3] {
				case local + "^{commit}":
					return []byte(local + "\n"), 0, nil
				case remoteTag + "^{commit}":
					return []byte(remoteCommit + "\n"), 0, nil
				}
			case prePushRevListCommand:
				return []byte(local + "\n"), 0, nil
			}

			t.Fatalf("unexpected git arguments: %v", args)

			return nil, -1, nil
		},
	}

	commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
		LocalRef: "refs/tags/v2", LocalOID: local, RemoteOID: remoteTag,
	}})

	check.NoError(t, err)
	check.Equal(t, []string{local}, commits)
	check.Equal(t, [][]string{
		{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteTag + "^{commit}"},
		{prePushRevListCommand, prePushReverseArg, prePushTopoOrderArg, local, "--not", remoteCommit},
	}, calls)
}

func TestPrePushResolver_IntroducedCommitsOmitsRemoteNonCommitObjects(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	remoteBlob := strings.Repeat("b", 40)

	var calls [][]string

	resolver := PrePushResolver{
		repoPath: "/repo",
		run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
			calls = append(calls, append([]string(nil), args...))

			switch args[0] {
			case prePushRevParseCommand:
				switch args[3] {
				case local + "^{commit}":
					return []byte(local + "\n"), 0, nil
				case remoteBlob + "^{commit}":
					return nil, 128, nil
				case remoteBlob + "^{}":
					return []byte(remoteBlob + "\n"), 0, nil
				}
			case prePushRevListCommand:
				return []byte(local + "\n"), 0, nil
			}

			t.Fatalf("unexpected git arguments: %v", args)

			return nil, -1, nil
		},
	}

	commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
		LocalRef: "refs/tags/v2", LocalOID: local, RemoteOID: remoteBlob,
	}})

	check.NoError(t, err)
	check.Equal(t, []string{local}, commits)
	check.Equal(t, [][]string{
		{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteBlob + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remoteBlob + "^{}"},
		{prePushRevListCommand, prePushReverseArg, prePushTopoOrderArg, local},
	}, calls)
}

func TestPrePushResolver_IntroducedCommitsRejectsMissingRemoteObjects(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	remote := strings.Repeat("b", 40)

	var calls [][]string

	resolver := PrePushResolver{
		repoPath: "/repo",
		run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
			calls = append(calls, append([]string(nil), args...))
			if args[3] == local+"^{commit}" {
				return []byte(local + "\n"), 0, nil
			}

			return nil, 128, nil
		},
	}

	commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
		LocalRef: "refs/tags/v2", LocalOID: local, RemoteOID: remote,
	}})

	check.Error(t, err)
	check.ErrorIs(t, err, errPrePushRemoteObject)
	check.Empty(t, commits)
	check.Equal(t, [][]string{
		{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remote + "^{commit}"},
		{prePushRevParseCommand, "--verify", gitEndOfOptions, remote + "^{}"},
	}, calls)
}

func TestPrePushResolver_IntroducedCommitsPreservesPreCancelledEmptySelection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	commits, err := (PrePushResolver{}).IntroducedCommits(ctx, nil)

	check.ErrorIs(t, err, context.Canceled)
	check.Empty(t, commits)
}

func TestPrePushResolver_IntroducedCommitsHandlesNonCommitAndMissingObjects(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	peeled := strings.Repeat("b", 40)
	remote := strings.Repeat("c", 40)
	tests := []struct {
		name         string
		localRef     string
		objectStatus int
		want         error
	}{
		{name: "tag_blob_is_skipped", localRef: "refs/tags/blob"},
		{name: "custom_tree_is_skipped", localRef: "refs/custom/tree"},
		{name: "branch_blob_is_rejected", localRef: "refs/heads/bad", want: errPrePushLocalRefNotCommit},
		{name: "missing_tag_is_rejected", localRef: "refs/tags/missing", objectStatus: 128, want: errPrePushLocalObject},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var calls [][]string

			resolver := PrePushResolver{
				repoPath: "/repo",
				run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
					calls = append(calls, append([]string(nil), args...))
					if strings.HasSuffix(args[3], "^{commit}") {
						return nil, 128, nil
					}

					return []byte(peeled + "\n"), testCase.objectStatus, nil
				},
			}

			commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
				LocalRef: testCase.localRef, LocalOID: local, RemoteOID: remote,
			}})

			check.Empty(t, commits)
			check.Len(t, calls, 2)
			check.Equal(t, []string{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{commit}"}, calls[0])
			check.Equal(t, []string{prePushRevParseCommand, "--verify", gitEndOfOptions, local + "^{}"}, calls[1])

			if testCase.want == nil {
				check.NoError(t, err)

				return
			}

			check.ErrorIs(t, err, testCase.want)
		})
	}
}

func TestPrePushResolver_IntroducedCommitsPropagatesResolutionFailures(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	tests := []struct {
		name   string
		output []byte
		runErr error
		want   error
	}{
		{name: "runner_failure", runErr: errPrePushGit, want: errPrePushGit},
		{name: "invalid_rev_parse_output", output: []byte("not-an-oid\n"), want: errPrePushGitOutput},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resolver := PrePushResolver{
				repoPath: "/repo",
				run: func(context.Context, string, ...string) ([]byte, int, error) {
					return testCase.output, 0, testCase.runErr
				},
			}

			commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
				LocalRef: "refs/heads/main", LocalOID: local,
			}})

			check.ErrorIs(t, err, testCase.want)
			check.Empty(t, commits)
		})
	}
}

func TestPrePushResolver_IntroducedCommitsRejectsRevListFailures(t *testing.T) {
	t.Parallel()

	local := strings.Repeat("a", 40)
	tests := []struct {
		name   string
		output []byte
		status int
		runErr error
		want   error
	}{
		{name: "runner_failure", status: -1, runErr: errPrePushGit, want: errPrePushGit},
		{name: "nonzero_status", status: 128, want: errPrePushGitCommand},
		{name: "invalid_output", output: []byte("not-an-oid\n"), want: errPrePushGitOutput},
		{name: "mixed_output_format", output: []byte(strings.Repeat("b", 64) + "\n"), want: errPrePushGitOutput},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			resolver := PrePushResolver{
				repoPath: "/repo",
				run: func(_ context.Context, _ string, args ...string) ([]byte, int, error) {
					if args[0] == prePushRevParseCommand {
						return []byte(local + "\n"), 0, nil
					}

					return testCase.output, testCase.status, testCase.runErr
				},
			}

			commits, err := resolver.IntroducedCommits(t.Context(), []domain.PrePushUpdate{{
				LocalRef: "refs/heads/main", LocalOID: local,
			}})

			check.ErrorIs(t, err, testCase.want)
			check.Empty(t, commits)
		})
	}
}
