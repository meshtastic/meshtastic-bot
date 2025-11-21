package config

import (
	"testing"
)

func TestParseTemplateURL(t *testing.T) {
	tests := []struct {
		name        string
		templateURL string
		wantOwner   string
		wantRepo    string
		wantRawURL  string
		wantAPIURL  string
		wantErr     bool
	}{
		{
			name:        "valid github URL with https",
			templateURL: "https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml",
			wantOwner:   "meshtastic",
			wantRepo:    "web",
			wantRawURL:  "https://raw.githubusercontent.com/meshtastic/web/main/.github/ISSUE_TEMPLATE/bug.yml",
			wantAPIURL:  "https://api.github.com/repos/meshtastic/web/issues",
			wantErr:     false,
		},
		{
			name:        "valid github URL without https",
			templateURL: "github.com/owner/repo/blob/main/template.yml",
			wantOwner:   "owner",
			wantRepo:    "repo",
			wantRawURL:  "https://raw.githubusercontent.com/owner/repo/main/template.yml",
			wantAPIURL:  "https://api.github.com/repos/owner/repo/issues",
			wantErr:     false,
		},
		{
			name:        "valid github URL with http",
			templateURL: "http://github.com/test/project/blob/dev/issue.yml",
			wantOwner:   "test",
			wantRepo:    "project",
			wantRawURL:  "https://raw.githubusercontent.com/test/project/dev/issue.yml",
			wantAPIURL:  "https://api.github.com/repos/test/project/issues",
			wantErr:     false,
		},
		{
			name:        "empty URL",
			templateURL: "",
			wantErr:     true,
		},
		{
			name:        "invalid URL - missing repo",
			templateURL: "https://github.com/meshtastic",
			wantErr:     true,
		},
		{
			name:        "invalid URL - only owner",
			templateURL: "github.com/owner",
			wantErr:     true,
		},
		{
			name:        "URL without blob path",
			templateURL: "https://github.com/meshtastic/firmware",
			wantOwner:   "meshtastic",
			wantRepo:    "firmware",
			wantRawURL:  "https://raw.githubusercontent.com/meshtastic/firmware/",
			wantAPIURL:  "https://api.github.com/repos/meshtastic/firmware/issues",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTemplateURL(tt.templateURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTemplateURL(%q) expected error, got nil", tt.templateURL)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTemplateURL(%q) unexpected error: %v", tt.templateURL, err)
				return
			}

			if result.Owner() != tt.wantOwner {
				t.Errorf("ParseTemplateURL(%q).Owner() = %q, want %q", tt.templateURL, result.Owner(), tt.wantOwner)
			}

			if result.Repo() != tt.wantRepo {
				t.Errorf("ParseTemplateURL(%q).Repo() = %q, want %q", tt.templateURL, result.Repo(), tt.wantRepo)
			}

			if result.RawURL() != tt.wantRawURL {
				t.Errorf("ParseTemplateURL(%q).RawURL() = %q, want %q", tt.templateURL, result.RawURL(), tt.wantRawURL)
			}

			if result.IssueAPIURL() != tt.wantAPIURL {
				t.Errorf("ParseTemplateURL(%q).IssueAPIURL() = %q, want %q", tt.templateURL, result.IssueAPIURL(), tt.wantAPIURL)
			}

			if result.String() != tt.templateURL {
				t.Errorf("ParseTemplateURL(%q).String() = %q, want %q", tt.templateURL, result.String(), tt.templateURL)
			}
		})
	}
}
