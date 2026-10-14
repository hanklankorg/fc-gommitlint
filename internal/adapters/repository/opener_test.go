// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package repository

import (
	"context"
	"errors"
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"

	"codefloe.com/itiquette/gommitlint/internal/adapters/git"
	"codefloe.com/itiquette/gommitlint/internal/domain"
)

var (
	errDetectObjectFormat = errors.New("detect object format")
	errVirtualFactory     = errors.New("virtual factory failed")
)

func TestOpener_Open_VirtualRepositoryRequiresFactory(t *testing.T) {
	t.Parallel()

	_, err := Opener{}.Open(t.Context(), "mem://repo")

	check.ErrorIs(t, err, ErrVirtualFactoryRequired)
}

func TestOpener_Open_UsesVirtualFactory(t *testing.T) {
	t.Parallel()

	factory := &testRepositoryFactory{}
	opener := Opener{VirtualFactory: factory}

	repo, err := opener.Open(t.Context(), "mem://repo")

	check.NoError(t, err)
	check.NotNil(t, repo)
	check.Equal(t, "mem://repo", factory.identifier)
}

// TestOpener_Open_SelectsTheAdapterForTheDetectedFormat replaces two
// near-identical tests that between them covered the SHA-256 branch and the
// detection-failure branch, and left the ordinary case uncovered: detection
// SUCCEEDS and reports something that is not SHA-256, which is every SHA-1
// repository in existence and so the path almost every real run takes.
//
// EqualFold is the product's own choice here, so the casing rows are not
// decoration — a change to == would silently route an upper-case "SHA256" to
// the SHA-1 adapter, and nothing would have noticed.
//
// Each row proves which opener ran AND which did not: the unused one fails the
// test if it is called, so "the right adapter opened it" cannot be satisfied by
// an Open that calls both.
func TestOpener_Open_SelectsTheAdapterForTheDetectedFormat(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name       string
		format     string
		detectErr  error
		wantSHA256 bool
	}{
		{name: "sha256 selects the git CLI adapter", format: ObjectFormatSHA256, wantSHA256: true},
		{name: "sha256 is matched case-insensitively", format: "SHA256", wantSHA256: true},
		{name: "sha256 is matched in mixed case", format: "Sha256", wantSHA256: true},
		{name: "sha1 selects the go-git adapter", format: ObjectFormatSHA1},
		{name: "an unrecognised format falls back to go-git", format: "sha3-512"},
		{name: "an empty format falls back to go-git", format: ""},
		{name: "a detection failure falls back to go-git", detectErr: errDetectObjectFormat},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var openedBy, openedPath string

			record := func(name string) OpenFunc {
				return func(path string) (domain.Repository, error) {
					openedBy, openedPath = name, path

					return testRepository{}, nil
				}
			}

			opener := Opener{
				DetectObjectFormat: func(context.Context, string) (string, error) {
					return testCase.format, testCase.detectErr
				},
				OpenSHA1:   record("sha1"),
				OpenSHA256: record("sha256"),
			}

			repo, err := opener.Open(t.Context(), "/repo")

			check.NoError(t, err)
			check.NotNil(t, repo)
			check.Equal(t, "/repo", openedPath, "the adapter must be given the requested path")

			want := "sha1"
			if testCase.wantSHA256 {
				want = "sha256"
			}

			check.Equal(t, want, openedBy, "wrong adapter selected")
		})
	}
}

// TestOpener_Open_PropagatesVirtualFactoryFailure covers the one virtual-path
// outcome the other two tests do not: the factory itself failing. Both existing
// virtual tests use a factory that always succeeds, so an Open that swallowed
// the error and returned a nil repository would pass them.
func TestOpener_Open_PropagatesVirtualFactoryFailure(t *testing.T) {
	t.Parallel()

	repo, err := Opener{
		VirtualFactory: failingRepositoryFactory{},
	}.Open(t.Context(), "mem://repo")

	check.ErrorIs(t, err, errVirtualFactory)
	check.Nil(t, repo)
}

type testRepository struct{}

type testRepositoryFactory struct {
	identifier string
}

func (f *testRepositoryFactory) CreateRepository(identifier string) (domain.Repository, error) {
	f.identifier = identifier

	return testRepository{}, nil
}

func (testRepository) GetCommit(context.Context, string) (domain.Commit, error) {
	return domain.Commit{}, nil
}

func (testRepository) GetCommitRange(context.Context, string, string) ([]domain.Commit, error) {
	return nil, nil
}

func (testRepository) GetHeadCommits(context.Context, int) ([]domain.Commit, error) {
	return nil, nil
}

func (testRepository) GetCommitsAheadCount(context.Context, string) (int, error) {
	return 0, nil
}

func (testRepository) GetCurrentBranch(context.Context) (string, error) {
	return "", nil
}

func (testRepository) BranchExists(context.Context, string) (bool, error) {
	return false, nil
}

type failingRepositoryFactory struct{}

func (failingRepositoryFactory) CreateRepository(string) (domain.Repository, error) {
	return nil, errVirtualFactory
}

// TestOpener_Open_ReportsWhyGitFailedForUnsupportedFormats pins the one case
// where the fallback's own error misleads: a repository the embedded reader
// cannot open (SHA-256) after git could not be used. go-git reports a
// format-extension name; the cause is whatever stopped git, so the error must
// carry it. The second row proves an unrelated fallback failure is returned
// as it is.
func TestOpener_Open_ReportsWhyGitFailedForUnsupportedFormats(t *testing.T) {
	t.Parallel()

	errOther := errors.New("permission denied")

	for _, testCase := range []struct {
		name       string
		openErr    error
		wantSystem bool
	}{
		{name: "unsupported format", openErr: git.ErrUnsupportedRepositoryFormat, wantSystem: true},
		{name: "another failure", openErr: errOther},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			opener := Opener{
				DetectObjectFormat: func(context.Context, string) (string, error) {
					return "", errDetectObjectFormat
				},
				OpenSHA1: func(string) (domain.Repository, error) {
					return nil, testCase.openErr
				},
			}

			repo, err := opener.Open(t.Context(), "/repo")

			check.Nil(t, repo)
			check.ErrorIs(t, err, testCase.openErr)
			check.Equal(t, testCase.wantSystem, errors.Is(err, ErrSystemGitRequired))
			check.Equal(t, testCase.wantSystem, errors.Is(err, errDetectObjectFormat), "the git failure must be reported exactly when it is the cause")
		})
	}
}
