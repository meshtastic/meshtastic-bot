package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type Client struct {
	token      string
	client     *github.Client
	httpClient *http.Client
	ctx        context.Context
}

type IssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels,omitempty"`
}

type IssueResponse struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	ID      int64  `json:"id"`
}

func NewClient(token string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		token:      token,
		client:     github.NewClient(tc),
		httpClient: tc,
		ctx:        ctx,
	}
}

// CreateIssue creates a new issue in the specified repository
func (c *Client) CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error) {
	url := fmt.Sprintf("https://api.github.com/repos/meshtastic/%s/%s/issues", owner, repo)

	issueReq := IssueRequest{
		Title:  title,
		Body:   body,
		Labels: labels,
	}

	jsonData, err := json.Marshal(issueReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("github API returned status %d: %v", resp.StatusCode, errResp)
	}

	var issueResp IssueResponse
	if err := json.NewDecoder(resp.Body).Decode(&issueResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &issueResp, nil
}

// FormatIssueBody formats the issue body with Discord user context
func FormatIssueBody(username, userID, description string) string {
	return fmt.Sprintf(`**Reported by:** %s (ID: %s)

%s

---
*This issue was automatically created from Discord*`, username, userID, description)
}
