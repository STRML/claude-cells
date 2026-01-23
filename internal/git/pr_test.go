package git

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestGH_CheckInstalled(t *testing.T) {
	gh := NewGH()
	ctx := context.Background()

	err := gh.CheckInstalled(ctx)
	if err != nil {
		t.Skipf("gh CLI not installed: %v", err)
	}
}

func TestPRRequest(t *testing.T) {
	tests := []struct {
		name  string
		req   *PRRequest
		valid bool
	}{
		{
			name: "valid request with all fields",
			req: &PRRequest{
				Title: "Add feature",
				Body:  "This adds a new feature",
				Base:  "main",
				Draft: true,
			},
			valid: true,
		},
		{
			name: "valid request with minimal fields",
			req: &PRRequest{
				Title: "Fix bug",
				Body:  "Bug fix description",
			},
			valid: true,
		},
		{
			name: "empty title",
			req: &PRRequest{
				Title: "",
				Body:  "Some body",
			},
			valid: false,
		},
		{
			name: "empty body",
			req: &PRRequest{
				Title: "Some title",
				Body:  "",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasTitle := tt.req.Title != ""
			hasBody := tt.req.Body != ""
			isValid := hasTitle && hasBody

			if isValid != tt.valid {
				t.Errorf("PRRequest validation = %v, want %v", isValid, tt.valid)
			}
		})
	}
}

func TestPRResponse(t *testing.T) {
	tests := []struct {
		name   string
		resp   *PRResponse
		number int
		hasURL bool
	}{
		{
			name: "valid response",
			resp: &PRResponse{
				Number: 123,
				URL:    "https://github.com/user/repo/pull/123",
			},
			number: 123,
			hasURL: true,
		},
		{
			name: "response with different number",
			resp: &PRResponse{
				Number: 456,
				URL:    "https://github.com/other/project/pull/456",
			},
			number: 456,
			hasURL: true,
		},
		{
			name: "response with no URL",
			resp: &PRResponse{
				Number: 789,
				URL:    "",
			},
			number: 789,
			hasURL: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.resp.Number != tt.number {
				t.Errorf("Number = %d, want %d", tt.resp.Number, tt.number)
			}
			if (tt.resp.URL != "") != tt.hasURL {
				t.Errorf("URL present = %v, want %v", tt.resp.URL != "", tt.hasURL)
			}
		})
	}
}

func TestNewGH(t *testing.T) {
	gh := NewGH()
	if gh == nil {
		t.Error("NewGH() returned nil")
	}
}

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		number int
	}{
		{
			name:   "standard github URL",
			url:    "https://github.com/user/repo/pull/123",
			number: 123,
		},
		{
			name:   "different repo",
			url:    "https://github.com/org/project/pull/456",
			number: 456,
		},
		{
			name:   "large number",
			url:    "https://github.com/company/app/pull/9999",
			number: 9999,
		},
		{
			name:   "malformed URL",
			url:    "not-a-url",
			number: 0,
		},
		{
			name:   "empty URL",
			url:    "",
			number: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num := extractPRNumber(tt.url)
			if num != tt.number {
				t.Errorf("extractPRNumber(%q) = %d, want %d", tt.url, num, tt.number)
			}
		})
	}
}

func TestBuildPRPrompt(t *testing.T) {
	tests := []struct {
		name             string
		branchName       string
		workstreamPrompt string
		commitLogs       string
		branchInfo       string
		wantContains     []string
	}{
		{
			name:             "full context",
			branchName:       "feature/add-auth",
			workstreamPrompt: "Add user authentication",
			commitLogs:       "abc123 Add login form\ndef456 Add password validation",
			branchInfo:       "2 commits, 5 files changed",
			wantContains: []string{
				"Branch: feature/add-auth",
				"Original task:\nAdd user authentication",
				"Commits:\nabc123 Add login form",
				"Stats:\n2 commits",
			},
		},
		{
			name:             "minimal context",
			branchName:       "fix-bug",
			workstreamPrompt: "",
			commitLogs:       "",
			branchInfo:       "",
			wantContains: []string{
				"Branch: fix-bug",
				"GitHub PR title and description",
			},
		},
		{
			name:             "only branch and prompt",
			branchName:       "ccells/new-feature",
			workstreamPrompt: "Implement new feature X",
			commitLogs:       "",
			branchInfo:       "",
			wantContains: []string{
				"Branch: ccells/new-feature",
				"Original task:\nImplement new feature X",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPRPrompt(tt.branchName, tt.workstreamPrompt, tt.commitLogs, tt.branchInfo)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildPRPrompt() result does not contain %q\nGot: %s", want, result)
				}
			}
		})
	}
}

func TestPRContentResponse(t *testing.T) {
	tests := []struct {
		name      string
		jsonInput string
		wantTitle string
		wantBody  string
		wantErr   bool
	}{
		{
			name:      "valid response",
			jsonInput: `{"title": "Add user auth", "body": "## Summary\n\n- Added login\n- Added logout"}`,
			wantTitle: "Add user auth",
			wantBody:  "## Summary\n\n- Added login\n- Added logout",
			wantErr:   false,
		},
		{
			name:      "empty title",
			jsonInput: `{"title": "", "body": "Some body"}`,
			wantTitle: "",
			wantBody:  "Some body",
			wantErr:   false,
		},
		{
			name:      "invalid json",
			jsonInput: `not json`,
			wantTitle: "",
			wantBody:  "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp prContentResponse
			err := json.Unmarshal([]byte(tt.jsonInput), &resp)

			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", resp.Title, tt.wantTitle)
				}
				if resp.Body != tt.wantBody {
					t.Errorf("Body = %q, want %q", resp.Body, tt.wantBody)
				}
			}
		})
	}
}

// TestMarkdownStrippingWithJSON tests the stripMarkdownCodeBlock function
// with various JSON formats that may come from Claude CLI responses.
// Note: CLI envelope extraction is now handled internally by the claude package.
func TestMarkdownStrippingWithJSON(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectedTitle string
		expectedBody  string
		shouldParse   bool
	}{
		{
			name:          "markdown-wrapped JSON",
			input:         "```json\n{\"title\":\"Add user authentication\",\"body\":\"## Summary\\n- Added login feature\"}\n```",
			expectedTitle: "Add user authentication",
			expectedBody:  "## Summary\n- Added login feature",
			shouldParse:   true,
		},
		{
			name:          "plain JSON (no markdown)",
			input:         `{"title":"Fix bug","body":"Fixed the bug"}`,
			expectedTitle: "Fix bug",
			expectedBody:  "Fixed the bug",
			shouldParse:   true,
		},
		{
			name:          "explanatory text before JSON code block",
			input:         "Here is the PR content:\n\n```json\n{\"title\":\"Refactor module\",\"body\":\"Cleaned up code\"}\n```",
			expectedTitle: "Refactor module",
			expectedBody:  "Cleaned up code",
			shouldParse:   true,
		},
		{
			name:          "invalid response - not JSON",
			input:         "This is not JSON at all",
			expectedTitle: "",
			expectedBody:  "",
			shouldParse:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMarkdownCodeBlock(tt.input)

			var resp prContentResponse
			err := json.Unmarshal([]byte(result), &resp)

			if tt.shouldParse {
				if err != nil {
					t.Errorf("Expected successful parse, got error: %v\nInput: %s\nAfter strip: %s", err, tt.input, result)
					return
				}
				if resp.Title != tt.expectedTitle {
					t.Errorf("Title = %q, want %q", resp.Title, tt.expectedTitle)
				}
				if resp.Body != tt.expectedBody {
					t.Errorf("Body = %q, want %q", resp.Body, tt.expectedBody)
				}
			} else {
				if err == nil {
					t.Errorf("Expected parse error, but parsing succeeded with title=%q, body=%q", resp.Title, resp.Body)
				}
			}
		})
	}
}

func TestAggregateCheckStatus(t *testing.T) {
	tests := []struct {
		name           string
		checks         []prCheckContext
		expectedStatus PRCheckStatus
		expectedHas    string // substring that should be in summary
	}{
		{
			name:           "empty rollup",
			checks:         nil,
			expectedStatus: PRCheckStatusUnknown,
			expectedHas:    "No checks",
		},
		{
			name: "all success",
			checks: []prCheckContext{
				{Conclusion: "success"},
				{Conclusion: "success"},
				{Conclusion: "skipped"},
			},
			expectedStatus: PRCheckStatusSuccess,
			expectedHas:    "3/3 passed",
		},
		{
			name: "pending present",
			checks: []prCheckContext{
				{Conclusion: "success"},
				{State: "pending"},
				{Conclusion: "success"},
			},
			expectedStatus: PRCheckStatusPending,
			expectedHas:    "2/3 passed",
		},
		{
			name: "failure present",
			checks: []prCheckContext{
				{Conclusion: "success"},
				{Conclusion: "failure"},
				{State: "pending"},
			},
			expectedStatus: PRCheckStatusFailure,
			expectedHas:    "1/3 passed",
		},
		{
			name: "uses state when conclusion empty",
			checks: []prCheckContext{
				{State: "success"},
				{State: "queued"},
			},
			expectedStatus: PRCheckStatusPending,
			expectedHas:    "1/2 passed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, summary := aggregateCheckStatus(tt.checks)
			if status != tt.expectedStatus {
				t.Errorf("status = %v, want %v", status, tt.expectedStatus)
			}
			if !strings.Contains(summary, tt.expectedHas) {
				t.Errorf("summary = %q, want to contain %q", summary, tt.expectedHas)
			}
		})
	}
}

// TestParseActualGHOutput verifies we can parse real GitHub API JSON output.
// This test uses actual gh pr view output structure to catch API changes.
func TestParseActualGHOutput(t *testing.T) {
	// Actual output from: gh pr view --json number,url,headRefOid,statusCheckRollup,headRefName
	raw := `{
		"headRefName": "feature-branch",
		"headRefOid": "33a62b9a151cf1c051b67de14efe639e73082b4d",
		"number": 28,
		"statusCheckRollup": [
			{
				"__typename": "CheckRun",
				"completedAt": "2026-01-23T17:45:05Z",
				"conclusion": "SUCCESS",
				"detailsUrl": "https://github.com/owner/repo/actions/runs/123",
				"name": "Lint",
				"startedAt": "2026-01-23T17:44:29Z",
				"status": "COMPLETED",
				"workflowName": "CI"
			},
			{
				"__typename": "CheckRun",
				"completedAt": "2026-01-23T17:45:02Z",
				"conclusion": "SUCCESS",
				"detailsUrl": "https://github.com/owner/repo/actions/runs/124",
				"name": "Test",
				"startedAt": "2026-01-23T17:44:29Z",
				"status": "COMPLETED",
				"workflowName": "CI"
			},
			{
				"__typename": "StatusContext",
				"context": "CodeRabbit",
				"startedAt": "2026-01-23T17:44:33Z",
				"state": "SUCCESS",
				"targetUrl": ""
			}
		],
		"url": "https://github.com/owner/repo/pull/28"
	}`

	var resp prViewResponse
	err := json.Unmarshal([]byte(raw), &resp)
	if err != nil {
		t.Fatalf("Failed to parse GitHub JSON: %v", err)
	}

	// Verify basic fields parsed correctly
	if resp.Number != 28 {
		t.Errorf("Number = %d, want 28", resp.Number)
	}
	if resp.HeadRefName != "feature-branch" {
		t.Errorf("HeadRefName = %q, want %q", resp.HeadRefName, "feature-branch")
	}
	if resp.URL != "https://github.com/owner/repo/pull/28" {
		t.Errorf("URL = %q, want %q", resp.URL, "https://github.com/owner/repo/pull/28")
	}

	// Verify statusCheckRollup parsed as array
	if len(resp.StatusCheckRollup) != 3 {
		t.Fatalf("StatusCheckRollup length = %d, want 3", len(resp.StatusCheckRollup))
	}

	// Verify CheckRun parsed (uses Conclusion)
	if resp.StatusCheckRollup[0].Conclusion != "SUCCESS" {
		t.Errorf("CheckRun conclusion = %q, want %q", resp.StatusCheckRollup[0].Conclusion, "SUCCESS")
	}

	// Verify StatusContext parsed (uses State)
	if resp.StatusCheckRollup[2].State != "SUCCESS" {
		t.Errorf("StatusContext state = %q, want %q", resp.StatusCheckRollup[2].State, "SUCCESS")
	}

	// Verify aggregation works with real data
	status, summary := aggregateCheckStatus(resp.StatusCheckRollup)
	if status != PRCheckStatusSuccess {
		t.Errorf("aggregated status = %v, want %v", status, PRCheckStatusSuccess)
	}
	if !strings.Contains(summary, "3/3") {
		t.Errorf("summary = %q, want to contain %q", summary, "3/3")
	}
}

func TestPRStatusInfo(t *testing.T) {
	// Test PRStatusInfo struct creation
	status := &PRStatusInfo{
		Number:        123,
		URL:           "https://github.com/user/repo/pull/123",
		HeadSHA:       "abc123",
		CheckStatus:   PRCheckStatusSuccess,
		ChecksSummary: "3/3 passed",
		UnpushedCount: 2,
		DivergedCount: 1,
		IsDiverged:    true,
	}

	if status.Number != 123 {
		t.Errorf("Number = %d, want 123", status.Number)
	}
	if status.IsDiverged != true {
		t.Errorf("IsDiverged = %v, want true", status.IsDiverged)
	}
	if status.CheckStatus != PRCheckStatusSuccess {
		t.Errorf("CheckStatus = %v, want %v", status.CheckStatus, PRCheckStatusSuccess)
	}
}

// MockGitClientForPRStatus is a mock implementation for testing GetPRStatus
type MockGitClientForPRStatus struct {
	unpushedCount int
	unpushedErr   error
	divergedCount int
	divergedErr   error
}

func (m *MockGitClientForPRStatus) GetUnpushedCommitCount(ctx context.Context, branch string) (int, error) {
	if m.unpushedErr != nil {
		return 0, m.unpushedErr
	}
	return m.unpushedCount, nil
}

func (m *MockGitClientForPRStatus) GetDivergedCommitCount(ctx context.Context, branch string) (int, error) {
	if m.divergedErr != nil {
		return 0, m.divergedErr
	}
	return m.divergedCount, nil
}

func TestGetPRStatus_WithMockClient(t *testing.T) {
	tests := []struct {
		name               string
		mockClient         *MockGitClientForPRStatus
		expectedUnpushed   int
		expectedDiverged   int
		expectedIsDiverged bool
	}{
		{
			name: "with unpushed and diverged commits",
			mockClient: &MockGitClientForPRStatus{
				unpushedCount: 3,
				divergedCount: 2,
			},
			expectedUnpushed:   3,
			expectedDiverged:   2,
			expectedIsDiverged: true,
		},
		{
			name: "no divergence",
			mockClient: &MockGitClientForPRStatus{
				unpushedCount: 1,
				divergedCount: 0,
			},
			expectedUnpushed:   1,
			expectedDiverged:   0,
			expectedIsDiverged: false,
		},
		{
			name: "error in unpushed count",
			mockClient: &MockGitClientForPRStatus{
				unpushedErr:   context.DeadlineExceeded,
				divergedCount: 1,
			},
			expectedUnpushed:   0,
			expectedDiverged:   1,
			expectedIsDiverged: true,
		},
		{
			name: "error in diverged count",
			mockClient: &MockGitClientForPRStatus{
				unpushedCount: 2,
				divergedErr:   context.DeadlineExceeded,
			},
			expectedUnpushed:   2,
			expectedDiverged:   0,
			expectedIsDiverged: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the full GetPRStatus without mocking exec.Command,
			// but we can verify the interface contract
			ctx := context.Background()

			unpushed, _ := tt.mockClient.GetUnpushedCommitCount(ctx, "test-branch")
			diverged, _ := tt.mockClient.GetDivergedCommitCount(ctx, "test-branch")

			if unpushed != tt.expectedUnpushed {
				t.Errorf("unpushed = %d, want %d", unpushed, tt.expectedUnpushed)
			}
			if diverged != tt.expectedDiverged {
				t.Errorf("diverged = %d, want %d", diverged, tt.expectedDiverged)
			}
			if (diverged > 0) != tt.expectedIsDiverged {
				t.Errorf("isDiverged = %v, want %v", diverged > 0, tt.expectedIsDiverged)
			}
		})
	}
}

func TestPRCheckStatusConstants(t *testing.T) {
	// Verify constant values are as expected
	if PRCheckStatusSuccess != "success" {
		t.Errorf("PRCheckStatusSuccess = %q, want %q", PRCheckStatusSuccess, "success")
	}
	if PRCheckStatusPending != "pending" {
		t.Errorf("PRCheckStatusPending = %q, want %q", PRCheckStatusPending, "pending")
	}
	if PRCheckStatusFailure != "failure" {
		t.Errorf("PRCheckStatusFailure = %q, want %q", PRCheckStatusFailure, "failure")
	}
	if PRCheckStatusUnknown != "unknown" {
		t.Errorf("PRCheckStatusUnknown = %q, want %q", PRCheckStatusUnknown, "unknown")
	}
}

func TestStripMarkdownCodeBlock(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "json code block",
			input:    "```json\n{\"title\":\"Test\"}\n```",
			expected: `{"title":"Test"}`,
		},
		{
			name:     "plain code block",
			input:    "```\n{\"title\":\"Test\"}\n```",
			expected: `{"title":"Test"}`,
		},
		{
			name:     "code block with whitespace",
			input:    "  ```json\n{\"title\":\"Test\"}\n```  ",
			expected: `{"title":"Test"}`,
		},
		{
			name:     "multiline content in code block",
			input:    "```json\n{\n  \"title\": \"Test\",\n  \"body\": \"Content\"\n}\n```",
			expected: "{\n  \"title\": \"Test\",\n  \"body\": \"Content\"\n}",
		},
		{
			name:     "no code block - plain JSON",
			input:    `{"title":"Test"}`,
			expected: `{"title":"Test"}`,
		},
		{
			name:     "no code block - plain text",
			input:    "just plain text",
			expected: "just plain text",
		},
		{
			name:     "unclosed code block",
			input:    "```json\n{\"title\":\"Test\"}",
			expected: "```json\n{\"title\":\"Test\"}",
		},
		{
			name:     "code block on single line",
			input:    "```json```",
			expected: "```json```",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "text before code block",
			input:    "Here is the JSON:\n\n```json\n{\"title\":\"Test\"}\n```",
			expected: `{"title":"Test"}`,
		},
		{
			name:     "explanation text before code block",
			input:    "I've generated the PR content as requested:\n```json\n{\"title\":\"Add feature\",\"body\":\"Description\"}\n```\nLet me know if you need changes.",
			expected: `{"title":"Add feature","body":"Description"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMarkdownCodeBlock(tt.input)
			if result != tt.expected {
				t.Errorf("stripMarkdownCodeBlock(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
