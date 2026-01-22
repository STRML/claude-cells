package git

import (
	"context"
	"encoding/json"
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
				if !contains(result, want) {
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

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
