package handlers

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestTruncatePlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short text unchanged",
			input:    "Short text",
			expected: "Short text",
		},
		{
			name:     "exactly 100 chars unchanged",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 100),
		},
		{
			name:     "long text truncated with ellipsis",
			input:    strings.Repeat("a", 150),
			expected: strings.Repeat("a", 97) + "...",
		},
		{
			name:     "101 chars truncated",
			input:    strings.Repeat("x", 101),
			expected: strings.Repeat("x", 97) + "...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePlaceholder(tt.input)
			if result != tt.expected {
				t.Errorf("truncatePlaceholder(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > 100 {
				t.Errorf("truncatePlaceholder(%q) returned string longer than 100 chars: %d", tt.input, len(result))
			}
		})
	}
}

func TestBuildIssueBody(t *testing.T) {
	tests := []struct {
		name            string
		submittedValues map[string]string
		username        string
		userID          string
		wantContains    []string
	}{
		{
			name: "single field",
			submittedValues: map[string]string{
				"Description": "This is a bug",
			},
			username: "testuser",
			userID:   "123456",
			wantContains: []string{
				"### Description",
				"This is a bug",
				"Submitted via Discord by: testuser (123456)",
			},
		},
		{
			name: "multiple fields",
			submittedValues: map[string]string{
				"Title":       "Bug Title",
				"Description": "Bug description",
				"Steps":       "1. Do this\n2. Do that",
			},
			username: "john_doe",
			userID:   "789",
			wantContains: []string{
				"### Title",
				"Bug Title",
				"### Description",
				"Bug description",
				"### Steps",
				"1. Do this\n2. Do that",
				"Submitted via Discord by: john_doe (789)",
			},
		},
		{
			name:            "empty values",
			submittedValues: map[string]string{},
			username:        "emptyuser",
			userID:          "000",
			wantContains: []string{
				"Submitted via Discord by: emptyuser (000)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildIssueBody(tt.submittedValues, tt.username, tt.userID)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildIssueBody() missing expected content:\nwant substring: %q\ngot: %q", want, result)
				}
			}
		})
	}
}

func TestExtractModalFields(t *testing.T) {
	tests := []struct {
		name       string
		components []discordgo.MessageComponent
		expected   map[string]string
	}{
		{
			name: "single text input",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "title",
							Value:    "My Title",
						},
					},
				},
			},
			expected: map[string]string{
				"title": "My Title",
			},
		},
		{
			name: "multiple text inputs",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "title",
							Value:    "Bug Report",
						},
					},
				},
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "description",
							Value:    "The app crashes",
						},
					},
				},
			},
			expected: map[string]string{
				"title":       "Bug Report",
				"description": "The app crashes",
			},
		},
		{
			name:       "empty components",
			components: []discordgo.MessageComponent{},
			expected:   map[string]string{},
		},
		{
			name: "multiple inputs in same row",
			components: []discordgo.MessageComponent{
				&discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID: "field1",
							Value:    "value1",
						},
						&discordgo.TextInput{
							CustomID: "field2",
							Value:    "value2",
						},
					},
				},
			},
			expected: map[string]string{
				"field1": "value1",
				"field2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractModalFields(tt.components)

			if len(result) != len(tt.expected) {
				t.Errorf("extractModalFields() returned %d fields, want %d", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("extractModalFields() missing key %q", key)
				} else if actualValue != expectedValue {
					t.Errorf("extractModalFields()[%q] = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}
