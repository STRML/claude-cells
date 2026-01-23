package git

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/STRML/claude-cells/internal/claude"
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
	Head  string // Branch to create PR from (required for worktrees)
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

	// --head is required for worktrees since gh may not detect the branch correctly
	if req.Head != "" {
		args = append(args, "--head", req.Head)
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

// prContentResponse is the expected JSON response from Claude for PR content generation.
type prContentResponse struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// claudeCLIEnvelope is the JSON envelope returned by claude CLI with --output-format json.
type claudeCLIEnvelope struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// extractCLIResult extracts the actual result from Claude CLI's JSON envelope.
// When using --output-format json, the CLI wraps responses in an envelope like:
// {"type":"result","result":"actual content here",...}
func extractCLIResult(output string) string {
	var envelope claudeCLIEnvelope
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		// Not a CLI envelope, return as-is
		return output
	}
	if envelope.Type == "result" && envelope.Result != "" {
		return envelope.Result
	}
	return output
}

// stripMarkdownCodeBlock removes markdown code block fencing from a string.
// Handles patterns like: ```json\n{...}\n``` or ```\n{...}\n```
// Also handles cases where there's text before the code block.
func stripMarkdownCodeBlock(s string) string {
	s = strings.TrimSpace(s)

	// Find opening fence - it might not be at the start
	openingFence := strings.Index(s, "```")
	if openingFence == -1 {
		return s
	}

	// Find end of first line (the opening fence with optional language)
	firstNewline := strings.Index(s[openingFence:], "\n")
	if firstNewline == -1 {
		return s
	}
	firstNewline += openingFence // Adjust to absolute position

	// Find closing fence (must be after the opening fence line)
	closingFence := strings.LastIndex(s, "```")
	if closingFence <= firstNewline {
		return s
	}

	// Extract content between fences
	content := s[firstNewline+1 : closingFence]
	return strings.TrimSpace(content)
}

// GeneratePRContent uses Claude to generate a PR title and description based on
// the branch commits and workstream context. Returns sensible defaults on failure.
func GeneratePRContent(ctx context.Context, gitClient GitClient, branchName, workstreamPrompt string) (title, body string) {
	// Default fallbacks
	defaultTitle := branchNameToTitle(branchName)
	defaultBody := fmt.Sprintf("## Summary\n\n%s\n\n## Changes\n\nCreated by [claude-cells](https://github.com/STRML/claude-cells).", workstreamPrompt)

	// Get commit logs for context
	commitLogs, err := gitClient.GetBranchCommitLogs(ctx, branchName)
	if err != nil {
		log.Printf("GeneratePRContent: failed to get commit logs: %v", err)
		return defaultTitle, defaultBody
	}

	// Get diff stats for context
	branchInfo, _ := gitClient.GetBranchInfo(ctx, branchName)

	// Build the prompt
	prompt := buildPRPrompt(branchName, workstreamPrompt, commitLogs, branchInfo)

	// Query Claude
	result, err := claude.Query(ctx, prompt, &claude.QueryOptions{
		OutputFormat: "json",
		Timeout:      claude.DefaultTimeout,
	})
	if err != nil {
		log.Printf("GeneratePRContent: Claude query failed: %v", err)
		return defaultTitle, defaultBody
	}

	// Extract result from CLI envelope (--output-format json wraps in envelope)
	result = extractCLIResult(result)

	// Strip markdown code blocks if present (Claude often wraps JSON in ```json...```)
	result = stripMarkdownCodeBlock(result)

	// Parse the JSON response
	var resp prContentResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		log.Printf("GeneratePRContent: failed to parse JSON response: %v (result after extraction: %q)", err, result)
		return defaultTitle, defaultBody
	}

	// Validate response
	if resp.Title == "" {
		resp.Title = defaultTitle
	}
	if resp.Body == "" {
		resp.Body = defaultBody
	}

	// Enforce title length limit
	if len(resp.Title) > 72 {
		resp.Title = resp.Title[:72]
	}

	return resp.Title, resp.Body
}

// buildPRPrompt constructs the prompt for PR content generation.
func buildPRPrompt(branchName, workstreamPrompt, commitLogs, branchInfo string) string {
	var sb strings.Builder

	sb.WriteString(`Generate a GitHub PR title and description. Output valid JSON only.

Format:
{"title": "concise title under 72 chars", "body": "markdown description"}

Rules for title:
- Concise, imperative mood (e.g., "Add user authentication")
- Under 72 characters
- No period at the end

Rules for body:
- Start with "## Summary" section with 2-3 bullet points
- Include "## Changes" section listing key modifications
- Keep it scannable and concise
- Use markdown formatting

`)

	sb.WriteString(fmt.Sprintf("Branch: %s\n\n", branchName))

	if workstreamPrompt != "" {
		sb.WriteString(fmt.Sprintf("Original task:\n%s\n\n", workstreamPrompt))
	}

	if commitLogs != "" {
		sb.WriteString(fmt.Sprintf("Commits:\n%s\n\n", commitLogs))
	}

	if branchInfo != "" {
		sb.WriteString(fmt.Sprintf("Stats:\n%s\n", branchInfo))
	}

	return sb.String()
}
