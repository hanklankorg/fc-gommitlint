// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package fileops_test

import (
	"path/filepath"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
	"github.com/spf13/afero"

	"codefloe.com/itiquette/gommitlint/internal/adapters/fileops"
	"codefloe.com/itiquette/gommitlint/internal/adapters/signing"
)

// Repository validation must work against a filesystem it cannot write to.
func TestGitOperations_ValidateGitRepository_NeedsNoWriteAccess(t *testing.T) {
	t.Parallel()

	memFs := afero.NewMemMapFs()

	repoPath := "/test/repo"
	gitDir := filepath.Join(repoPath, ".git")

	check.NoError(t, memFs.MkdirAll(gitDir, 0755))
	check.NoError(t, afero.WriteFile(memFs, filepath.Join(gitDir, "config"), []byte("git config"), 0644))

	adapter := signing.NewAferoAdapter(afero.NewReadOnlyFs(memFs))
	fileOps := fileops.NewFileOperations(adapter)
	gitOps := fileops.NewGitOperations(fileOps)

	result, err := gitOps.ValidateGitRepository(repoPath)
	check.NoError(t, err)
	check.Equal(t, repoPath, result)
}
