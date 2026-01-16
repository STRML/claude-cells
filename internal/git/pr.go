package git

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// GH wraps the GitHub CLI for PR operations.
type GH struct{}

// NewGH creates a new GitHub CLI wrapper.
func NewGH() *GH {
	return &GH{}
}

// CheckInstalled verifies gh CLI is available.
func (g *GH) CheckInstalled(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "gh", "--version")
	return cmd.Run()
}

// PRRequest contains data for creating a PR.
type PRRequest struct {
	Title string
	Body  string
	Base  string // Optional, defaults to default branch
	Draft bool
}

// PRResponse contains the created PR info.
type PRResponse struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// extractPRNumber extracts the PR number from a GitHub PR URL.
func extractPRNumber(url string) int {
	if url == "" {
		return 0
	}

	// Match URLs like https://github.com/owner/repo/pull/123
	re := regexp.MustCompile(`/pull/(\d+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return 0
	}

	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return num
}

// CreatePR creates a pull request using gh CLI.
func (g *GH) CreatePR(ctx context.Context, repoPath string, req *PRRequest) (*PRResponse, error) {
	args := []string{"pr", "create",
		"--title", req.Title,
		"--body", req.Body,
	}

	if req.Base != "" {
		args = append(args, "--base", req.Base)
	}
	if req.Draft {
		args = append(args, "--draft")
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh pr create failed: %w: %s", err, out)
	}

	// gh pr create outputs the URL on success
	url := strings.TrimSpace(string(out))
	number := extractPRNumber(url)

	return &PRResponse{
		Number: number,
		URL:    url,
	}, nil
}

// GetPR gets info about a PR by number.
func (g *GH) GetPR(ctx context.Context, repoPath string, number int) (*PRResponse, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", fmt.Sprint(number), "--json", "number,url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr view failed: %w", err)
	}

	var resp PRResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse PR response: %w", err)
	}
	return &resp, nil
}

// PRExists checks if a PR exists for the current branch.
func (g *GH) PRExists(ctx context.Context, repoPath string) (bool, *PRResponse, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", "--json", "number,url")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		// No PR for this branch (or other error, but we treat as no PR)
		return false, nil, nil
	}

	var resp PRResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return false, nil, fmt.Errorf("failed to parse PR response: %w", err)
	}
	return true, &resp, nil
}
