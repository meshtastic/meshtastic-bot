package github

import (
	"context"
	"fmt"
	"log"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type Client struct {
	token  string
	client *github.Client
	ctx    context.Context
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

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	return &Client{
		token:  token,
		client: github.NewClient(tc),
		ctx:    ctx,
	}
}

func (c *Client) CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error) {
	log.Printf("[GitHub API] Creating issue in %s/%s", owner, repo)
	log.Printf("[GitHub API] Title: %s", title)
	log.Printf("[GitHub API] Labels: %v", labels)

	req := &github.IssueRequest{
		Title: github.String(title),
		Body:  github.String(body),
	}

	// go-github requires *string slices, so we adapt if labels exist
	if len(labels) > 0 {
		req.Labels = &labels
	}

	issue, resp, err := c.client.Issues.Create(c.ctx, owner, repo, req)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("github API returned %d: %w", resp.StatusCode, err)
		}
		return nil, err
	}

	return &IssueResponse{
		Number:  issue.GetNumber(),
		HTMLURL: issue.GetHTMLURL(),
		ID:      issue.GetID(),
	}, nil
}

func FormatIssueBody(username, userID, description string) string {
	return fmt.Sprintf(`**Reported by:** %s (ID: %s)

%s

---
*This issue was automatically created from Discord*`, username, userID, description)
}
