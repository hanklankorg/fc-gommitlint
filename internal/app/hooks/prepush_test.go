// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package hooks

import (
	"codefloe.com/itiquette/gommitlint/internal/domain"
	"context"
	"errors"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

var errPrePushGraph = errors.New("graph failure")

type prePushResolverStub struct {
	commits []string
	err     error
	updates []domain.PrePushUpdate
}

func (s *prePushResolverStub) IntroducedCommits(_ context.Context, updates []domain.PrePushUpdate) ([]string, error) {
	s.updates = append([]domain.PrePushUpdate(nil), updates...)

	return s.commits, s.err
}

func TestSelectPrePushCommits_SelectsAllNonDeletionUpdatesOnce(t *testing.T) {
	t.Parallel()

	resolver := &prePushResolverStub{commits: []string{"first", "second"}}
	updates := []domain.PrePushUpdate{
		{LocalOID: "local-delete", RemoteOID: "remote-delete", Delete: true},
		{LocalOID: "local-update", RemoteOID: "remote-update"},
		{LocalOID: "local-create", RemoteOID: "0000"},
	}

	commits, err := SelectPrePushCommits(t.Context(), updates, resolver)

	check.NoError(t, err)
	check.Equal(t, []string{"first", "second"}, commits)
	check.Equal(t, updates[1:], resolver.updates)
}

func TestSelectPrePushCommits_DeletionOnlyNeedsNoResolver(t *testing.T) {
	t.Parallel()

	commits, err := SelectPrePushCommits(t.Context(), []domain.PrePushUpdate{{LocalOID: "zero", RemoteOID: "remote", Delete: true}}, nil)

	check.NoError(t, err)
	check.Empty(t, commits)
}

func TestSelectPrePushCommits_RequiresResolverForUpdates(t *testing.T) {
	t.Parallel()

	commits, err := SelectPrePushCommits(t.Context(), []domain.PrePushUpdate{{LocalOID: "local"}}, nil)

	check.ErrorIs(t, err, ErrPrePushResolverRequired)
	check.Empty(t, commits)
}

func TestSelectPrePushCommits_PropagatesGraphErrors(t *testing.T) {
	t.Parallel()

	resolver := &prePushResolverStub{err: errPrePushGraph}

	commits, err := SelectPrePushCommits(t.Context(), []domain.PrePushUpdate{{LocalOID: "local"}}, resolver)

	check.ErrorIs(t, err, errPrePushGraph)
	check.Empty(t, commits)
}
