// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package hooks

import (
	"context"
	"errors"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// ErrPrePushResolverRequired indicates that commit selection has no Git resolver.
var ErrPrePushResolverRequired = errors.New("pre-push Git resolver required")

// PrePushCommitResolver provides the one graph operation needed by pre-push.
type PrePushCommitResolver interface {
	IntroducedCommits(ctx context.Context, updates []domain.PrePushUpdate) ([]string, error)
}

// SelectPrePushCommits selects commits introduced by all non-deletion updates.
func SelectPrePushCommits(ctx context.Context, refUpdates []domain.PrePushUpdate, resolver PrePushCommitResolver) ([]string, error) {
	updates := make([]domain.PrePushUpdate, 0, len(refUpdates))
	for _, update := range refUpdates {
		if update.Delete {
			continue
		}

		updates = append(updates, update)
	}

	if len(updates) == 0 {
		return []string{}, nil
	}

	if resolver == nil {
		return nil, ErrPrePushResolverRequired
	}

	return resolver.IntroducedCommits(ctx, updates)
}
