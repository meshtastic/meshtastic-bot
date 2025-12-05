package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	gogithub "github.com/google/go-github/v57/github"
	internalgithub "github.com/meshtastic/meshtastic-bot/internal/github"
)

// MockGitHubClient implements internalgithub.Client interface
type MockGitHubClient struct {
	GetReleasesFunc    func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error)
	CompareCommitsFunc func(owner, repo, base, head string) (*gogithub.CommitsComparison, error)
	CreateIssueFunc    func(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error)
	GetRepositoryFunc  func(owner, repo string) (*gogithub.Repository, error)
}

func (m *MockGitHubClient) GetReleases(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
	if m.GetReleasesFunc != nil {
		return m.GetReleasesFunc(owner, repo, limit)
	}
	return nil, nil
}

func (m *MockGitHubClient) CompareCommits(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
	if m.CompareCommitsFunc != nil {
		return m.CompareCommitsFunc(owner, repo, base, head)
	}
	return nil, nil
}

func (m *MockGitHubClient) CreateIssue(owner, repo, title, body string, labels []string) (*internalgithub.IssueResponse, error) {
	if m.CreateIssueFunc != nil {
		return m.CreateIssueFunc(owner, repo, title, body, labels)
	}
	return nil, nil
}

func (m *MockGitHubClient) GetRepository(owner, repo string) (*gogithub.Repository, error) {
	if m.GetRepositoryFunc != nil {
		return m.GetRepositoryFunc(owner, repo)
	}
	return nil, nil
}

type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.RoundTripFunc != nil {
		return m.RoundTripFunc(req)
	}
	return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString("{}")),
		},
		nil
}

func TestFormatChangelogMessage(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	tests := []struct {
		name       string
		base       string
		head       string
		comparison *gogithub.CommitsComparison
		want       []string // Substrings that should be present
		dontWant   []string // Substrings that should NOT be present
	}{
		{
			name: "basic comparison",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(2),
				HTMLURL:      strPtr("https://github.com/org/repo/compare/v1.0.0...v1.1.0"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abcdef123456"),
						HTMLURL: strPtr("https://github.com/org/repo/commit/abcdef1"),
						Commit: &gogithub.Commit{
							Message: strPtr("feat: cool feature"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("John Doe"),
							},
						},
						Author: &gogithub.User{
							Login: strPtr("johndoe"),
						},
					},
					{
						SHA:     strPtr("123456abcdef"),
						HTMLURL: strPtr("https://github.com/org/repo/commit/123456a"),
						Commit: &gogithub.Commit{
							Message: strPtr("fix: nasty bug\n\nSome details"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("Jane Smith"),
							},
						},
						Author: &gogithub.User{
							Login: strPtr("janesmith"),
						},
					},
				},
			},
			want: []string{
				"## Changes from v1.0.0 to v1.1.0",
				"Total commits: 2",
				"[`abcdef1`](<https://github.com/org/repo/commit/abcdef1>)",
				"feat: cool feature",
				"johndoe",
				"[`123456a`](<https://github.com/org/repo/commit/123456a>)",
				"fix: nasty bug",
				"janesmith",
				"[View Full Comparison](<https://github.com/org/repo/compare/v1.0.0...v1.1.0>)",
			},
			dontWant: []string{
				"Some details",
				"Showing last 10",
			},
		},
		{
			name: "many commits truncated",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(15),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: func() []*gogithub.RepositoryCommit {
					commits := make([]*gogithub.RepositoryCommit, 15)
					for i := 0; i < 15; i++ {
						commits[i] = &gogithub.RepositoryCommit{
							SHA:     strPtr("longhashvalue"),
							HTMLURL: strPtr("url"),
							Commit: &gogithub.Commit{
								Message: strPtr("msg"),
								Author:  &gogithub.CommitAuthor{Name: strPtr("author")},
							},
							Author: &gogithub.User{Login: strPtr("user")},
						}
					}
					return commits
				}(),
			},
			want: []string{
				"Total commits: 15",
				"*Showing last 10 of 15 commits*",
			},
		},
		{
			name: "nil author with fallback to commit author",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("commit with nil author"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("Commit Author"),
							},
						},
						Author: nil, // nil author should trigger fallback
					},
				},
			},
			want: []string{
				"commit with nil author",
				"Commit Author",
			},
		},
		{
			name: "nil commit author with fallback to Unknown",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("def456"),
						HTMLURL: strPtr("https://github.com/commit/def456"),
						Commit: &gogithub.Commit{
							Message: strPtr("commit with all authors nil"),
							Author:  nil, // nil commit author
						},
						Author: nil, // nil author
					},
				},
			},
			want: []string{
				"commit with all authors nil",
				"Unknown",
			},
		},
		{
			name: "empty author login with fallback to commit author",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("ghi789"),
						HTMLURL: strPtr("https://github.com/commit/ghi789"),
						Commit: &gogithub.Commit{
							Message: strPtr("commit with empty login"),
							Author: &gogithub.CommitAuthor{
								Name: strPtr("Fallback Author"),
							},
						},
						Author: &gogithub.User{
							Login: strPtr(""), // empty login
						},
					},
				},
			},
			want: []string{
				"commit with empty login",
				"Fallback Author",
			},
		},
		{
			name: "nil commit object - tests GetCommit() returning nil",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("jkl012"),
						HTMLURL: strPtr("https://github.com/commit/jkl012"),
						Commit:  nil, // nil commit object
						Author: &gogithub.User{
							Login: strPtr("testuser"),
						},
					},
				},
			},
			want: []string{
				"Total commits: 1",
				"testuser",
			},
		},
		{
			name: "nil commit and nil author - complete fallback to Unknown",
			base: "v1.0.0",
			head: "v1.1.0",
			comparison: &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("mno345"),
						HTMLURL: strPtr("https://github.com/commit/mno345"),
						Commit:  nil, // nil commit
						Author:  nil, // nil author
					},
				},
			},
			want: []string{
				"Total commits: 1",
				"Unknown",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatChangelogMessage(tt.base, tt.head, tt.comparison)

			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("formatChangelogMessage() missing %q\nGot:\n%s", w, got)
				}
			}

			for _, dw := range tt.dontWant {
				if strings.Contains(got, dw) {
					t.Errorf("formatChangelogMessage() unexpectedly contains %q", dw)
				}
			}
		})
	}
}

func TestHandleChangelogAutocomplete(t *testing.T) {
	// Save original GithubClient and restore after test
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	tests := []struct {
		name           string
		releases       []*gogithub.RepositoryRelease
		releaseErr     error
		userInput      string
		expectedCount  int
		expectedValues []string
	}{
		{
			name: "Cache Update Success - Matches All",
			releases: []*gogithub.RepositoryRelease{
				{TagName: gogithub.String("v1.0.0")},
				{TagName: gogithub.String("v1.1.0")},
			},
			userInput:      "",
			expectedCount:  2,
			expectedValues: []string{"v1.0.0", "v1.1.0"},
		},
		{
			name: "Filtering",
			releases: []*gogithub.RepositoryRelease{
				{TagName: gogithub.String("v1.0.0")},
				{TagName: gogithub.String("v2.0.0")},
				{TagName: gogithub.String("beta-v3")},
			},
			userInput:      "v1",
			expectedCount:  1,
			expectedValues: []string{"v1.0.0"},
		},
		{
			name:          "Empty Cache Handling",
			releases:      []*gogithub.RepositoryRelease{},
			userInput:     "",
			expectedCount: 0,
		},
		{
			name:          "Error Handling - Cache Update Fails",
			releaseErr:    errors.New("API Error"),
			userInput:     "",
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup Mock GitHub Client
			mockClient := &MockGitHubClient{
				GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
					return tt.releases, tt.releaseErr
				},
			}
			GithubClient = mockClient

			// Reset cache for each test run to ensure updateReleaseCache is called
			releaseCacheMutex.Lock()
			releaseCache = nil
			lastCacheUpdate = time.Time{}
			releaseCacheMutex.Unlock()

			// Setup Mock Discord Session
			s, _ := discordgo.New("")
			s.Client = &http.Client{
				Transport: &MockRoundTripper{
					RoundTripFunc: func(req *http.Request) (*http.Response, error) {
						// Verify request
						if req.Method != "POST" {
							t.Errorf("Expected POST request, got %s", req.Method)
						}

						// Parse body
						var data discordgo.InteractionResponse
						if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
							t.Errorf("Failed to decode request body: %v", err)
						}

						if data.Type != discordgo.InteractionApplicationCommandAutocompleteResult {
							t.Errorf("Expected response type AutocompleteResult, got %v", data.Type)
						}

						choices := data.Data.Choices
						if len(choices) != tt.expectedCount {
							t.Errorf("Expected %d choices, got %d", tt.expectedCount, len(choices))
						}

						for i, val := range tt.expectedValues {
							if i < len(choices) && choices[i].Value != val {
								t.Errorf("Expected choice %d to be %s, got %v", i, val, choices[i].Value)
							}
						}

						return &http.Response{
							StatusCode: 200,
							Body:       io.NopCloser(bytes.NewBufferString("{}")),
							Header:     make(http.Header),
						}, nil
					},
				},
			}

			// Create Interaction
			i := &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					Type: discordgo.InteractionApplicationCommandAutocomplete,
					Data: discordgo.ApplicationCommandInteractionData{
						Options: []*discordgo.ApplicationCommandInteractionDataOption{
							{
								Focused: true,
								Value:   tt.userInput,
								Type:    discordgo.ApplicationCommandOptionString,
								Name:    "option",
							},
						},
					},
				},
			}

			handleChangelogAutocomplete(s, i)
		})
	}
}

func TestHandleChangelogAutocomplete_Limit(t *testing.T) {
	// Save original GithubClient and restore after test
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	// Mock with 30 releases
	releases := make([]*gogithub.RepositoryRelease, 30)
	for i := 0; i < 30; i++ {
		tagName := "v" + strings.Repeat("1", i+1) // Just unique names
		releases[i] = &gogithub.RepositoryRelease{TagName: &tagName}
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	// Reset cache
	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode body: %v", err)
				}
				choices := data.Data.Choices
				if len(choices) != 25 {
					t.Errorf("Expected 25 choices, got %d", len(choices))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}"))}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommandAutocomplete,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Focused: true,
						Value:   "",
						Type:    discordgo.ApplicationCommandOptionString,
					},
				},
			},
		},
	}

	handleChangelogAutocomplete(s, i)
}

func TestHandleChangelogAutocomplete_CaseInsensitive(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("V1.0.0")},
		{TagName: gogithub.String("V1.1.0")},
		{TagName: gogithub.String("v2.0.0")},
		{TagName: gogithub.String("Beta-V3")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode body: %v", err)
				}
				choices := data.Data.Choices
				if len(choices) != 2 {
					t.Errorf("Expected 2 choices (case-insensitive match), got %d", len(choices))
				}
				expectedValues := map[string]bool{"V1.0.0": true, "V1.1.0": true}
				for _, choice := range choices {
					if !expectedValues[choice.Value.(string)] {
						t.Errorf("Unexpected choice value: %v", choice.Value)
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}"))}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommandAutocomplete,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Focused: true,
						Value:   "V1",
						Type:    discordgo.ApplicationCommandOptionString,
					},
				},
			},
		},
	}

	handleChangelogAutocomplete(s, i)
}

func TestHandleChangelogAutocomplete_NoFocusedOption(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
		{TagName: gogithub.String("v2.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode body: %v", err)
				}
				choices := data.Data.Choices
				if len(choices) != 2 {
					t.Errorf("Expected 2 choices (all releases when no focus), got %d", len(choices))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}"))}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommandAutocomplete,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Focused: false,
						Value:   "v1",
						Type:    discordgo.ApplicationCommandOptionString,
					},
				},
			},
		},
	}

	handleChangelogAutocomplete(s, i)
}

func TestHandleChangelogAutocomplete_CacheReuse(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			apiCallCount++
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}")), Header: make(http.Header)}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommandAutocomplete,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Focused: true,
						Value:   "",
						Type:    discordgo.ApplicationCommandOptionString,
					},
				},
			},
		},
	}

	handleChangelogAutocomplete(s, i)
	if apiCallCount != 1 {
		t.Errorf("Expected 1 API call on first request, got %d", apiCallCount)
	}

	handleChangelogAutocomplete(s, i)
	if apiCallCount != 1 {
		t.Errorf("Expected cache reuse (still 1 API call), got %d", apiCallCount)
	}

	releaseCacheMutex.Lock()
	lastCacheUpdate = time.Now().Add(-2 * time.Hour)
	releaseCacheMutex.Unlock()

	handleChangelogAutocomplete(s, i)
	if apiCallCount != 2 {
		t.Errorf("Expected cache refresh (2 API calls after expiry), got %d", apiCallCount)
	}
}

func TestHandleChangelogAutocomplete_PartialMatch(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0-beta")},
		{TagName: gogithub.String("v1.0.0-alpha")},
		{TagName: gogithub.String("v2.0.0")},
		{TagName: gogithub.String("beta-release-3")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode body: %v", err)
				}
				choices := data.Data.Choices
				if len(choices) != 2 {
					t.Errorf("Expected 2 choices matching 'beta', got %d", len(choices))
				}
				expectedValues := map[string]bool{"v1.0.0-beta": true, "beta-release-3": true}
				for _, choice := range choices {
					if !expectedValues[choice.Value.(string)] {
						t.Errorf("Unexpected choice value: %v", choice.Value)
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("{}"))}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommandAutocomplete,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Focused: true,
						Value:   "beta",
						Type:    discordgo.ApplicationCommandOptionString,
					},
				},
			},
		},
	}

	handleChangelogAutocomplete(s, i)
}

func TestUpdateReleaseCache_InitialLoad(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
		{TagName: gogithub.String("v2.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error on initial cache load, got %v", err)
	}

	releaseCacheMutex.RLock()
	defer releaseCacheMutex.RUnlock()

	if len(releaseCache) != 2 {
		t.Errorf("Expected 2 releases in cache, got %d", len(releaseCache))
	}

	if lastCacheUpdate.IsZero() {
		t.Error("Expected lastCacheUpdate to be set, but it's zero")
	}

	if time.Since(lastCacheUpdate) > time.Second {
		t.Errorf("Expected lastCacheUpdate to be recent, but it's %v old", time.Since(lastCacheUpdate))
	}
}

func TestUpdateReleaseCache_CacheExpiration(t *testing.T) {
	originalClient := GithubClient
	originalCacheDuration := cacheDuration
	defer func() {
		GithubClient = originalClient
		cacheDuration = originalCacheDuration
	}()

	cacheDuration = 100 * time.Millisecond

	apiCallCount := 0
	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			apiCallCount++
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected 1 API call initially, got %d", apiCallCount)
	}

	err = updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected cache to be reused (still 1 API call), got %d", apiCallCount)
	}

	time.Sleep(150 * time.Millisecond)

	err = updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 2 {
		t.Errorf("Expected cache to expire and refresh (2 API calls), got %d", apiCallCount)
	}
}

func TestUpdateReleaseCache_ErrorHandling(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	expectedErr := errors.New("GitHub API error")

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return nil, expectedErr
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err == nil {
		t.Error("Expected error from failed API call, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error to be %v, got %v", expectedErr, err)
	}

	releaseCacheMutex.RLock()
	cacheIsEmpty := len(releaseCache) == 0
	releaseCacheMutex.RUnlock()

	if !cacheIsEmpty {
		t.Error("Expected cache to remain empty after error")
	}
}

func TestUpdateReleaseCache_ConcurrentAccess(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	var apiCallMutex sync.Mutex

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			apiCallMutex.Lock()
			apiCallCount++
			apiCallMutex.Unlock()
			time.Sleep(50 * time.Millisecond)
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if err := updateReleaseCache(); err != nil {
				errChan <- err
			}
		}()
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Unexpected error from goroutine: %v", err)
	}

	apiCallMutex.Lock()
	finalCallCount := apiCallCount
	apiCallMutex.Unlock()

	if finalCallCount != 1 {
		t.Errorf("Expected exactly 1 API call despite concurrent access, got %d", finalCallCount)
	}

	releaseCacheMutex.RLock()
	cacheLen := len(releaseCache)
	releaseCacheMutex.RUnlock()

	if cacheLen != 1 {
		t.Errorf("Expected 1 release in cache, got %d", cacheLen)
	}
}

func TestUpdateReleaseCache_DoubleCheckedLocking(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	var apiCallMutex sync.Mutex

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			apiCallMutex.Lock()
			apiCallCount++
			apiCallMutex.Unlock()
			time.Sleep(100 * time.Millisecond)
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	const numGoroutines = 5
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			updateReleaseCache()
		}()
	}

	close(startBarrier)
	wg.Wait()

	apiCallMutex.Lock()
	finalCallCount := apiCallCount
	apiCallMutex.Unlock()

	if finalCallCount != 1 {
		t.Errorf("Double-checked locking failed: expected 1 API call, got %d", finalCallCount)
	}
}

func TestUpdateReleaseCache_EmptyCacheWithExpiredTime(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	releases := []*gogithub.RepositoryRelease{
		{TagName: gogithub.String("v1.0.0")},
	}

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return releases, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Now().Add(-10 * time.Minute)
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	releaseCacheMutex.RLock()
	cacheLen := len(releaseCache)
	releaseCacheMutex.RUnlock()

	if cacheLen != 1 {
		t.Errorf("Expected cache to be populated, got %d releases", cacheLen)
	}
}

func TestUpdateReleaseCache_EmptyReleasesFromAPI(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			return []*gogithub.RepositoryRelease{}, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error when API returns empty releases, got %v", err)
	}

	releaseCacheMutex.RLock()
	defer releaseCacheMutex.RUnlock()

	if releaseCache == nil {
		t.Error("Expected releaseCache to be initialized (empty slice), not nil")
	}

	if len(releaseCache) != 0 {
		t.Errorf("Expected empty cache, got %d releases", len(releaseCache))
	}

	if lastCacheUpdate.IsZero() {
		t.Error("Expected lastCacheUpdate to be set even with empty releases")
	}
}

func TestUpdateReleaseCache_ParametersPassedCorrectly(t *testing.T) {
	originalClient := GithubClient
	originalOwner := GithubOwner
	originalRepo := GithubRepo
	defer func() {
		GithubClient = originalClient
		GithubOwner = originalOwner
		GithubRepo = originalRepo
	}()

	GithubOwner = "test-owner"
	GithubRepo = "test-repo"

	var capturedOwner, capturedRepo string
	var capturedLimit int

	mockClient := &MockGitHubClient{
		GetReleasesFunc: func(owner, repo string, limit int) ([]*gogithub.RepositoryRelease, error) {
			capturedOwner = owner
			capturedRepo = repo
			capturedLimit = limit
			return []*gogithub.RepositoryRelease{
				{TagName: gogithub.String("v1.0.0")},
			}, nil
		},
	}
	GithubClient = mockClient

	releaseCacheMutex.Lock()
	releaseCache = nil
	lastCacheUpdate = time.Time{}
	releaseCacheMutex.Unlock()

	err := updateReleaseCache()
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if capturedOwner != "test-owner" {
		t.Errorf("Expected owner 'test-owner', got %q", capturedOwner)
	}

	if capturedRepo != "test-repo" {
		t.Errorf("Expected repo 'test-repo', got %q", capturedRepo)
	}

	if capturedLimit != 100 {
		t.Errorf("Expected limit 100, got %d", capturedLimit)
	}
}

func TestGetChangelogMessage_CacheMiss(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			apiCallCount++
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	message, err := getChangelogMessage("v1.0.0", "v2.0.0")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected 1 API call on cache miss, got %d", apiCallCount)
	}

	if !strings.Contains(message, "v1.0.0") || !strings.Contains(message, "v2.0.0") {
		t.Errorf("Expected message to contain version info, got: %s", message)
	}
}

func TestGetChangelogMessage_CacheHit(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			apiCallCount++
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	message1, err := getChangelogMessage("v1.0.0", "v2.0.0")
	if err != nil {
		t.Errorf("Expected no error on first call, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected 1 API call on first request, got %d", apiCallCount)
	}

	message2, err := getChangelogMessage("v1.0.0", "v2.0.0")
	if err != nil {
		t.Errorf("Expected no error on second call, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected cache hit (still 1 API call), got %d", apiCallCount)
	}

	if message1 != message2 {
		t.Error("Expected cached message to match original")
	}
}

func TestGetChangelogMessage_CacheExpiration(t *testing.T) {
	originalClient := GithubClient
	originalTTL := comparisonCacheTTL
	defer func() {
		GithubClient = originalClient
		comparisonCacheTTL = originalTTL
	}()

	comparisonCacheTTL = 100 * time.Millisecond

	apiCallCount := 0
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			apiCallCount++
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	_, err := getChangelogMessage("v1.0.0", "v2.0.0")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 1 {
		t.Errorf("Expected 1 API call initially, got %d", apiCallCount)
	}

	time.Sleep(150 * time.Millisecond)

	_, err = getChangelogMessage("v1.0.0", "v2.0.0")
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if apiCallCount != 2 {
		t.Errorf("Expected cache expiration (2 API calls), got %d", apiCallCount)
	}
}

func TestGetChangelogMessage_ErrorHandling(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	expectedErr := errors.New("GitHub API error")

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			return nil, expectedErr
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	_, err := getChangelogMessage("v1.0.0", "v2.0.0")
	if err == nil {
		t.Error("Expected error from failed API call, got nil")
	}

	if !errors.Is(err, expectedErr) {
		t.Errorf("Expected error to be %v, got %v", expectedErr, err)
	}

	comparisonCacheMutex.RLock()
	cacheLen := len(comparisonCache)
	comparisonCacheMutex.RUnlock()

	if cacheLen != 0 {
		t.Errorf("Expected cache to remain empty after error, got %d entries", cacheLen)
	}
}

func TestGetChangelogMessage_ConcurrentAccess(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	var apiCallMutex sync.Mutex
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			apiCallMutex.Lock()
			apiCallCount++
			apiCallMutex.Unlock()
			time.Sleep(50 * time.Millisecond)
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	messages := make([]string, numGoroutines)
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			msg, err := getChangelogMessage("v1.0.0", "v2.0.0")
			if err != nil {
				errChan <- err
				return
			}
			messages[index] = msg
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Unexpected error from goroutine: %v", err)
	}

	apiCallMutex.Lock()
	finalCallCount := apiCallCount
	apiCallMutex.Unlock()

	if finalCallCount > 3 {
		t.Errorf("Expected at most 3 API calls with concurrent access, got %d", finalCallCount)
	}

	firstMessage := messages[0]
	for i, msg := range messages {
		if msg != firstMessage {
			t.Errorf("Message %d differs from first message", i)
		}
	}
}

func TestGetChangelogMessage_DifferentComparisons(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	apiCallCount := 0
	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			apiCallCount++
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	msg1, _ := getChangelogMessage("v1.0.0", "v2.0.0")
	msg2, _ := getChangelogMessage("v2.0.0", "v3.0.0")

	if apiCallCount != 2 {
		t.Errorf("Expected 2 API calls for different comparisons, got %d", apiCallCount)
	}

	if !strings.Contains(msg1, "v1.0.0") {
		t.Error("First message should contain v1.0.0")
	}

	if !strings.Contains(msg2, "v2.0.0") && !strings.Contains(msg2, "v3.0.0") {
		t.Error("Second message should contain v2.0.0 or v3.0.0")
	}

	comparisonCacheMutex.RLock()
	cacheLen := len(comparisonCache)
	comparisonCacheMutex.RUnlock()

	if cacheLen != 2 {
		t.Errorf("Expected 2 cache entries, got %d", cacheLen)
	}
}

func TestHandleChangelog_MissingBaseParameter(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	mockClient := &MockGitHubClient{}
	GithubClient = mockClient

	respondCalled := false
	var capturedResponse *discordgo.InteractionResponse

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				respondCalled = true
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				capturedResponse = &data
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "head",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v2.0.0",
					},
				},
			},
		},
	}

	handleChangelog(s, i)

	if !respondCalled {
		t.Error("Expected InteractionRespond to be called")
	}

	if capturedResponse.Type != discordgo.InteractionResponseChannelMessageWithSource {
		t.Errorf("Expected response type ChannelMessageWithSource, got %v", capturedResponse.Type)
	}

	if capturedResponse.Data.Content != "Please provide both base and head versions." {
		t.Errorf("Expected validation error message, got: %s", capturedResponse.Data.Content)
	}

	if capturedResponse.Data.Flags != discordgo.MessageFlagsEphemeral {
		t.Error("Expected ephemeral flag to be set")
	}
}

func TestHandleChangelog_MissingHeadParameter(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	mockClient := &MockGitHubClient{}
	GithubClient = mockClient

	respondCalled := false
	var capturedResponse *discordgo.InteractionResponse

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				respondCalled = true
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				capturedResponse = &data
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "base",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v1.0.0",
					},
				},
			},
		},
	}

	handleChangelog(s, i)

	if !respondCalled {
		t.Error("Expected InteractionRespond to be called")
	}

	if capturedResponse.Data.Content != "Please provide both base and head versions." {
		t.Errorf("Expected validation error message, got: %s", capturedResponse.Data.Content)
	}
}

func TestHandleChangelog_EmptyParameters(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	mockClient := &MockGitHubClient{}
	GithubClient = mockClient

	respondCalled := false
	var capturedResponse *discordgo.InteractionResponse

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				respondCalled = true
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				capturedResponse = &data
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "base",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "",
					},
					{
						Name:  "head",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "",
					},
				},
			},
		},
	}

	handleChangelog(s, i)

	if !respondCalled {
		t.Error("Expected InteractionRespond to be called")
	}

	if capturedResponse.Data.Content != "Please provide both base and head versions." {
		t.Errorf("Expected validation error message, got: %s", capturedResponse.Data.Content)
	}
}

func TestHandleChangelog_SuccessfulComparison(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	strPtr := func(s string) *string { return &s }
	intPtr := func(i int) *int { return &i }

	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			return &gogithub.CommitsComparison{
				TotalCommits: intPtr(1),
				HTMLURL:      strPtr("https://github.com/compare"),
				Commits: []*gogithub.RepositoryCommit{
					{
						SHA:     strPtr("abc123"),
						HTMLURL: strPtr("https://github.com/commit/abc123"),
						Commit: &gogithub.Commit{
							Message: strPtr("test commit"),
							Author:  &gogithub.CommitAuthor{Name: strPtr("Test Author")},
						},
						Author: &gogithub.User{Login: strPtr("testuser")},
					},
				},
			}, nil
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	callSequence := []string{}
	deferredResponseSeen := false
	editResponseSeen := false
	var finalContent string

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/callback") {
					callSequence = append(callSequence, "respond")
					var data discordgo.InteractionResponse
					if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
						t.Errorf("Failed to decode request body: %v", err)
					}
					if data.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
						deferredResponseSeen = true
					}
				} else if req.Method == "PATCH" {
					callSequence = append(callSequence, "edit")
					editResponseSeen = true
					var edit discordgo.WebhookEdit
					if err := json.NewDecoder(req.Body).Decode(&edit); err != nil {
						t.Errorf("Failed to decode edit body: %v", err)
					}
					if edit.Content != nil {
						finalContent = *edit.Content
					}
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "base",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v1.0.0",
					},
					{
						Name:  "head",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v2.0.0",
					},
				},
			},
		},
	}

	handleChangelog(s, i)

	if !deferredResponseSeen {
		t.Error("Expected deferred response to be sent")
	}

	if !editResponseSeen {
		t.Error("Expected response edit to be called")
	}

	if len(callSequence) != 2 || callSequence[0] != "respond" || callSequence[1] != "edit" {
		t.Errorf("Expected call sequence [respond, edit], got %v", callSequence)
	}

	if !strings.Contains(finalContent, "v1.0.0") || !strings.Contains(finalContent, "v2.0.0") {
		t.Errorf("Expected final content to contain version info, got: %s", finalContent)
	}

	if !strings.Contains(finalContent, "test commit") {
		t.Errorf("Expected final content to contain commit message, got: %s", finalContent)
	}
}

func TestHandleChangelog_GitHubAPIError(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	expectedErr := errors.New("GitHub API error")
	mockClient := &MockGitHubClient{
		CompareCommitsFunc: func(owner, repo, base, head string) (*gogithub.CommitsComparison, error) {
			return nil, expectedErr
		},
	}
	GithubClient = mockClient

	comparisonCacheMutex.Lock()
	comparisonCache = make(map[string]*CachedComparison)
	comparisonCacheMutex.Unlock()

	deferredResponseSeen := false
	editResponseSeen := false
	var errorContent string

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/callback") {
					var data discordgo.InteractionResponse
					if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
						t.Errorf("Failed to decode request body: %v", err)
					}
					if data.Type == discordgo.InteractionResponseDeferredChannelMessageWithSource {
						deferredResponseSeen = true
					}
				} else if req.Method == "PATCH" {
					editResponseSeen = true
					var edit discordgo.WebhookEdit
					if err := json.NewDecoder(req.Body).Decode(&edit); err != nil {
						t.Errorf("Failed to decode edit body: %v", err)
					}
					if edit.Content != nil {
						errorContent = *edit.Content
					}
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "base",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v1.0.0",
					},
					{
						Name:  "head",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "v2.0.0",
					},
				},
			},
		},
	}

	handleChangelog(s, i)

	if !deferredResponseSeen {
		t.Error("Expected deferred response to be sent")
	}

	if !editResponseSeen {
		t.Error("Expected error response edit to be called")
	}

	expectedErrorMsg := "Failed to compare versions: v1.0.0...v2.0.0"
	if errorContent != expectedErrorMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrorMsg, errorContent)
	}
}

func TestHandleChangelog_NoOptions(t *testing.T) {
	originalClient := GithubClient
	defer func() { GithubClient = originalClient }()

	mockClient := &MockGitHubClient{}
	GithubClient = mockClient

	respondCalled := false
	var capturedResponse *discordgo.InteractionResponse

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				respondCalled = true
				var data discordgo.InteractionResponse
				if err := json.NewDecoder(req.Body).Decode(&data); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				capturedResponse = &data
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString("{}")),
					Header:     make(http.Header),
				}, nil
			},
		},
	}

	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Options: []*discordgo.ApplicationCommandInteractionDataOption{},
			},
		},
	}

	handleChangelog(s, i)

	if !respondCalled {
		t.Error("Expected InteractionRespond to be called")
	}

	if capturedResponse.Data.Content != "Please provide both base and head versions." {
		t.Errorf("Expected validation error message, got: %s", capturedResponse.Data.Content)
	}
}
