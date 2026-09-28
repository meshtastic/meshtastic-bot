package github

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

const (
	// RepositoryCacheTTL defines how long repository metadata is cached
	RepositoryCacheTTL = 4 * time.Hour

	// apiTimeout bounds each call. Handlers hold cache locks across them, so an
	// unbounded hang would stall every /changelog and /repo behind it.
	apiTimeout = 10 * time.Second
)

type Client interface {
	GetReleases(owner, repo string, limit int) ([]*github.RepositoryRelease, error)
	CompareCommits(owner, repo, base, head string) (*github.CommitsComparison, error)
	CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error)
	GetRepository(owner, repo string) (*github.Repository, error)
	FindSubmission(owner, repo, marker string, since time.Time) (*IssueResponse, error)
}

type CachedRepository struct {
	Repository *github.Repository
	Timestamp  time.Time
}

type LiveGitHubClient struct {
	token     string
	client    *github.Client
	ctx       context.Context
	repoCache map[string]*CachedRepository
	cacheMux  sync.RWMutex

	loginMu sync.Mutex
	login   string
}

// FindSubmission returns the issue this token filed since `since` whose body
// holds marker, or nil. It reads the issue list rather than search, which
// lags, so an attempt whose response was lost is found at once.
func (c *LiveGitHubClient) FindSubmission(owner, repo, marker string, since time.Time) (*IssueResponse, error) {
	ctx, cancel := context.WithTimeout(c.ctx, apiTimeout)
	defer cancel()
	login, err := c.tokenLogin(ctx)
	if err != nil {
		return nil, err
	}
	opts := &github.IssueListByRepoOptions{
		Creator: login, State: "all", Since: since, Sort: "created", Direction: "desc",
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for range findSubmissionPages {
		issues, resp, err := c.client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list recent issues: %w", err)
		}
		for _, issue := range issues {
			if strings.Contains(issue.GetBody(), marker) {
				return &IssueResponse{Number: issue.GetNumber(), HTMLURL: issue.GetHTMLURL(), ID: issue.GetID()}, nil
			}
			// Newest first: everything after this predates the first attempt.
			if issue.GetCreatedAt().Before(since) {
				return nil, nil
			}
		}
		if resp.NextPage == 0 {
			return nil, nil
		}
		opts.Page = resp.NextPage
	}
	// Not found but not ruled out either; the caller files nothing on error.
	return nil, fmt.Errorf("more than %d pages of recent issues to check", findSubmissionPages)
}

// findSubmissionPages bounds FindSubmission's walk through the issue list.
const findSubmissionPages = 5

func (c *LiveGitHubClient) tokenLogin(ctx context.Context) (string, error) {
	c.loginMu.Lock()
	defer c.loginMu.Unlock()
	if c.login != "" {
		return c.login, nil
	}
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return "", fmt.Errorf("failed to read the token's user: %w", err)
	}
	c.login = user.GetLogin()
	return c.login, nil
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
		repoCache: make(map[string]*CachedRepository),
	}
}

func (c *LiveGitHubClient) GetReleases(owner, repo string, limit int) ([]*github.RepositoryRelease, error) {
	opts := &github.ListOptions{
		PerPage: limit,
	}
	ctx, cancel := context.WithTimeout(c.ctx, apiTimeout)
	defer cancel()
	releases, _, err := c.client.Repositories.ListReleases(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}
	return releases, nil
}

func (c *LiveGitHubClient) CompareCommits(owner, repo, base, head string) (*github.CommitsComparison, error) {
	ctx, cancel := context.WithTimeout(c.ctx, apiTimeout)
	defer cancel()
	comparison, resp, err := c.client.Repositories.CompareCommits(ctx, owner, repo, base, head, nil)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("github API returned %d: failed to compare commits: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("failed to compare commits: %w", err)
	}
	return comparison, nil
}

func (c *LiveGitHubClient) CreateIssue(owner, repo, title, body string, labels []string) (*IssueResponse, error) {
	// The title is the reporter's own text; it stays out of the host log.
	log.Printf("[GitHub API] Creating issue in %s/%s with labels %v", owner, repo, labels)

	req := &github.IssueRequest{
		Title: github.String(title),
		Body:  github.String(body),
	}

	// go-github requires *string slices, so we adapt if labels exist
	if len(labels) > 0 {
		req.Labels = &labels
	}

	ctx, cancel := context.WithTimeout(c.ctx, apiTimeout)
	defer cancel()
	issue, resp, err := c.client.Issues.Create(ctx, owner, repo, req)
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

	// First check with read lock
	c.cacheMux.RLock()
	if cached, exists := c.repoCache[cacheKey]; exists {
		if time.Since(cached.Timestamp) < RepositoryCacheTTL {
			c.cacheMux.RUnlock()
			return cached.Repository, nil
		}
	}
	c.cacheMux.RUnlock()

	// Cache miss or expired - acquire write lock
	c.cacheMux.Lock()
	defer c.cacheMux.Unlock()

	// Double-check after acquiring write lock
	if cached, exists := c.repoCache[cacheKey]; exists {
		if time.Since(cached.Timestamp) < RepositoryCacheTTL {
			return cached.Repository, nil
		}
	}

	// Fetch from GitHub API
	ctx, cancel := context.WithTimeout(c.ctx, apiTimeout)
	defer cancel()
	repository, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}

	// Store in cache with timestamp
	c.repoCache[cacheKey] = &CachedRepository{
		Repository: repository,
		Timestamp:  time.Now(),
	}

	return repository, nil
}

func FormatIssueBody(username, userID, description string) string {
	return fmt.Sprintf(`**Reported by:** %s (ID: %s)

%s

---
*This issue was automatically created from Discord*`, username, userID, description)
}
