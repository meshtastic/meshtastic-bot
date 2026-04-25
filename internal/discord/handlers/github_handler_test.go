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
