package github

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

type Client interface {
	GetReleases(owner, repo string, limit int) ([]*github.RepositoryRelease, error)
	CompareCommits(owner, repo, base, head string) (*github.CommitsComparison, error)
	CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error)
	GetRepository(owner, repo string) (*github.Repository, error)
}

type LiveGitHubClient struct {
	token     string
	client    *github.Client
	ctx       context.Context
	repoCache map[string]*github.Repository
	cacheMux  sync.RWMutex
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

func NewClient(token string) Client {
	ctx := context.Background()

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	return &LiveGitHubClient{
		token:     token,
		client:    github.NewClient(tc),
		ctx:       ctx,
		repoCache: make(map[string]*github.Repository),
	}
}

func (c *LiveGitHubClient) GetReleases(owner, repo string, limit int) ([]*github.RepositoryRelease, error) {
	opts := &github.ListOptions{
		PerPage: limit,
	}
	releases, _, err := c.client.Repositories.ListReleases(c.ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}
	return releases, nil
}

func (c *LiveGitHubClient) CompareCommits(owner, repo, base, head string) (*github.CommitsComparison, error) {
	comparison, _, err := c.client.Repositories.CompareCommits(c.ctx, owner, repo, base, head, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}
	return comparison, nil
}

func (c *LiveGitHubClient) CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error) {
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

func (c *LiveGitHubClient) GetRepository(owner, repo string) (*github.Repository, error) {
	cacheKey := fmt.Sprintf("%s/%s", owner, repo)

	// Check cache first
	c.cacheMux.RLock()
	if cached, exists := c.repoCache[cacheKey]; exists {
		c.cacheMux.RUnlock()
		return cached, nil
	}
	c.cacheMux.RUnlock()

	// Fetch from GitHub API
	repository, _, err := c.client.Repositories.Get(c.ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// Store in cache
	c.cacheMux.Lock()
	c.repoCache[cacheKey] = repository
	c.cacheMux.Unlock()

	return repository, nil
}

func FormatIssueBody(username, userID, description string) string {
	return fmt.Sprintf(`**Reported by:** %s (ID: %s)

%s

---
*This issue was automatically created from Discord*`, username, userID, description)
}