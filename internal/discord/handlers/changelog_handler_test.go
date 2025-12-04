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
		dontWant   []string // Substrings that should be present
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
	lastCacheUpdate = time.Now().Add(-10 * time.Minute)
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
