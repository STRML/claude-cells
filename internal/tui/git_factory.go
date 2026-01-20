package tui

import (
	"github.com/STRML/claude-cells/internal/git"
)

// GitClientFactory is a function that creates a GitClient for a given repo path.
// This can be overridden in tests to return a MockGitClient.
var GitClientFactory = func(repoPath string) git.GitClient {
	return git.New(repoPath)
}

// SetGitClientFactory sets a custom factory for creating git clients.
// This is primarily used in tests to inject mock clients.
// Returns a function to restore the original factory.
func SetGitClientFactory(factory func(string) git.GitClient) func() {
	original := GitClientFactory
	GitClientFactory = factory
	return func() {
		GitClientFactory = original
	}
}
