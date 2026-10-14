// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package git

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-git/go-billy/v5/memfs"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"

	"codefloe.com/itiquette/gommitlint/internal/domain"
)

// ErrRepositoryNotFound is returned when an in-memory repository is not found.
var ErrRepositoryNotFound = errors.New("in-memory repository not found")

// RepositoryFactory creates Repository instances from identifiers.
type RepositoryFactory interface {
	CreateRepository(identifier string) (domain.Repository, error)
}

// FileSystemFactory creates repositories from filesystem paths.
type FileSystemFactory struct{}

// CreateRepository creates a repository from a filesystem path.
func (f *FileSystemFactory) CreateRepository(path string) (domain.Repository, error) {
	return NewRepositoryFromPath(path)
}

// InMemoryFactory creates in-memory repositories for testing.
// Repositories are identified by virtual paths like "mem://repo-name".
type InMemoryFactory struct {
	mu    sync.RWMutex
	repos map[string]*gogit.Repository
}

// NewInMemoryFactory creates a new in-memory repository factory.
func NewInMemoryFactory() *InMemoryFactory {
	return &InMemoryFactory{
		repos: make(map[string]*gogit.Repository),
	}
}

// CreateRepository creates or retrieves an in-memory repository.
func (f *InMemoryFactory) CreateRepository(identifier string) (domain.Repository, error) {
	// Extract repo name from virtual path
	repoName := strings.TrimPrefix(identifier, "mem://")

	// First check local repos
	f.mu.RLock()
	goGitRepo, exists := f.repos[repoName]
	f.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrRepositoryNotFound, repoName)
	}

	return &Repository{
		repo: goGitRepo,
	}, nil
}

// RegisterRepository adds a new in-memory repository to the factory.
func (f *InMemoryFactory) RegisterRepository(name string, repo *gogit.Repository) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.repos[name] = repo
}

// CreateInMemoryRepo creates a new in-memory repository and registers it.
func (f *InMemoryFactory) CreateInMemoryRepo(name string) (*gogit.Repository, error) {
	storer := memory.NewStorage()
	fs := memfs.New()

	repo, err := gogit.Init(storer, fs)
	if err != nil {
		return nil, err
	}

	f.RegisterRepository(name, repo)

	return repo, nil
}

// GetVirtualPath returns the virtual path for an in-memory repository.
func (f *InMemoryFactory) GetVirtualPath(name string) string {
	return "mem://" + name
}

// IsVirtualPath checks if a path refers to an in-memory repository.
func IsVirtualPath(path string) bool {
	return strings.HasPrefix(path, "mem://")
}
