package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	gogithub "github.com/google/go-github/v57/github"
)

func TestHandleRepo_DefaultRepository(t *testing.T) {
	originalClient := GithubClient
	originalRepo := GithubRepo
	originalOwner := GithubOwner
	defer func() {
		GithubClient = originalClient
		GithubRepo = originalRepo
		GithubOwner = originalOwner
	}()

	GithubOwner = "test-owner"
	GithubRepo = "default-repo"

	expectedURL := "https://github.com/test-owner/default-repo"

	mockClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*gogithub.Repository, error) {
			if owner != "test-owner" || repo != "default-repo" {
				t.Errorf("Expected owner=test-owner and repo=default-repo, got owner=%s, repo=%s", owner, repo)
			}
			return &gogithub.Repository{
				HTMLURL: gogithub.String(expectedURL),
			}, nil
		},
	}
	GithubClient = mockClient

	deferredResponseSeen := false
	editResponseSeen := false
	var finalContent string

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
				Options: []*discordgo.ApplicationCommandInteractionDataOption{},
			},
		},
	}

	handleRepo(s, i)

	if !deferredResponseSeen {
		t.Error("Expected deferred response to be sent")
	}

	if !editResponseSeen {
		t.Error("Expected response edit to be called")
	}

	if finalContent != expectedURL {
		t.Errorf("Expected final content to be %q, got %q", expectedURL, finalContent)
	}
}

func TestHandleRepo_SpecificRepository(t *testing.T) {
	originalClient := GithubClient
	originalOwner := GithubOwner
	defer func() {
		GithubClient = originalClient
		GithubOwner = originalOwner
	}()

	GithubOwner = "test-owner"
	expectedURL := "https://github.com/test-owner/custom-repo"

	mockClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*gogithub.Repository, error) {
			if owner != "test-owner" || repo != "custom-repo" {
				t.Errorf("Expected owner=test-owner and repo=custom-repo, got owner=%s, repo=%s", owner, repo)
			}
			return &gogithub.Repository{
				HTMLURL: gogithub.String(expectedURL),
			}, nil
		},
	}
	GithubClient = mockClient

	var finalContent string

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.Method == "PATCH" {
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
						Name:  "name",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "custom-repo",
					},
				},
			},
		},
	}

	handleRepo(s, i)

	if finalContent != expectedURL {
		t.Errorf("Expected final content to be %q, got %q", expectedURL, finalContent)
	}
}

func TestHandleRepo_RepositoryNotFound(t *testing.T) {
	originalClient := GithubClient
	originalOwner := GithubOwner
	defer func() {
		GithubClient = originalClient
		GithubOwner = originalOwner
	}()

	GithubOwner = "test-owner"

	expectedErr := errors.New("404 Not Found")
	mockClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*gogithub.Repository, error) {
			return nil, expectedErr
		},
	}
	GithubClient = mockClient

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
						Name:  "name",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "nonexistent-repo",
					},
				},
			},
		},
	}

	handleRepo(s, i)

	if !deferredResponseSeen {
		t.Error("Expected deferred response to be sent")
	}

	if !editResponseSeen {
		t.Error("Expected error response edit to be called")
	}

	expectedErrorMsg := "Repository `test-owner/nonexistent-repo` not found in the organization."
	if errorContent != expectedErrorMsg {
		t.Errorf("Expected error message %q, got %q", expectedErrorMsg, errorContent)
	}
}

func TestHandleRepo_EmptyRepositoryName(t *testing.T) {
	originalClient := GithubClient
	originalRepo := GithubRepo
	originalOwner := GithubOwner
	defer func() {
		GithubClient = originalClient
		GithubRepo = originalRepo
		GithubOwner = originalOwner
	}()

	GithubOwner = "test-owner"
	GithubRepo = "default-repo"

	var capturedRepo string

	mockClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*gogithub.Repository, error) {
			capturedRepo = repo
			return &gogithub.Repository{
				HTMLURL: gogithub.String("https://github.com/test-owner/default-repo"),
			}, nil
		},
	}
	GithubClient = mockClient

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
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
						Name:  "name",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "",
					},
				},
			},
		},
	}

	handleRepo(s, i)

	if capturedRepo != "default-repo" {
		t.Errorf("Expected default repo to be used when empty string provided, got %q", capturedRepo)
	}
}

func TestHandleRepo_NoOptions(t *testing.T) {
	originalClient := GithubClient
	originalRepo := GithubRepo
	originalOwner := GithubOwner
	defer func() {
		GithubClient = originalClient
		GithubRepo = originalRepo
		GithubOwner = originalOwner
	}()

	GithubOwner = "test-owner"
	GithubRepo = "default-repo"

	var capturedRepo string

	mockClient := &MockGitHubClient{
		GetRepositoryFunc: func(owner, repo string) (*gogithub.Repository, error) {
			capturedRepo = repo
			return &gogithub.Repository{
				HTMLURL: gogithub.String("https://github.com/test-owner/default-repo"),
			}, nil
		},
	}
	GithubClient = mockClient

	s, _ := discordgo.New("")
	s.Client = &http.Client{
		Transport: &MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
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

	handleRepo(s, i)

	if capturedRepo != "default-repo" {
		t.Errorf("Expected default repo to be used when no options provided, got %q", capturedRepo)
	}
}

func TestResolveRepoAlias(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "apple", input: "apple", expected: "Meshtastic-Apple"},
		{name: "ios", input: "ios", expected: "Meshtastic-Apple"},
		{name: "iphone", input: "iphone", expected: "Meshtastic-Apple"},
		{name: "android", input: "android", expected: "Meshtastic-Android"},
		{name: "uppercase resolves", input: "IOS", expected: "Meshtastic-Apple"},
		{name: "mixed case resolves", input: "Android", expected: "Meshtastic-Android"},
		{name: "surrounding whitespace trimmed", input: "  apple  ", expected: "Meshtastic-Apple"},
		{name: "docs", input: "docs", expected: "meshtastic"},
		{name: "cli", input: "cli", expected: "python"},

		// Names that already resolve on GitHub must pass through untouched, so
		// nothing that worked before this table existed stops working.
		{name: "web passes through", input: "web", expected: "web"},
		{name: "firmware passes through", input: "firmware", expected: "firmware"},
		{name: "design passes through", input: "design", expected: "design"},
		{name: "exact repo name passes through", input: "Meshtastic-Apple", expected: "Meshtastic-Apple"},
		{name: "unknown name passes through", input: "not-a-repo", expected: "not-a-repo"},
		{name: "empty stays empty", input: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveRepoAlias(tt.input); got != tt.expected {
				t.Errorf("resolveRepoAlias(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// An alias pointing at a repository that does not exist would turn a working
// lookup into a confusing failure, so every target must be spelled the way the
// organization spells it.
func TestRepoAliasTargetsAreWellFormed(t *testing.T) {
	known := map[string]bool{
		"Meshtastic-Apple":   true,
		"Meshtastic-Android": true,
		"meshtastic":         true,
		"python":             true,
	}

	for alias, target := range repoAliases {
		if alias != strings.ToLower(alias) {
			t.Errorf("alias %q must be lower case, or lookup will never match it", alias)
		}
		if !known[target] {
			t.Errorf("alias %q points at %q, which is not a known repository", alias, target)
		}
	}
}
