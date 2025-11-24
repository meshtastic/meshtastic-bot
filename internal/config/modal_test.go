package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetOwnerAndRepo(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		wantOwner  string
		wantRepo   string
	}{
		{
			name: "valid config with template URL",
			configYAML: `config:
  - command: bug
    template_url: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml
    channel_id:
      - "123456789"
`,
			wantOwner: "meshtastic",
			wantRepo:  "web",
		},
		{
			name: "config with different owner/repo",
			configYAML: `config:
  - command: feature
    template_url: https://github.com/danditomaso/meshtastic-web/blob/main/.github/ISSUE_TEMPLATE/feature.yml
    channel_id:
      - "123456789"
`,
			wantOwner: "danditomaso",
			wantRepo:  "meshtastic-web",
		},
		{
			name: "config with multiple modals - returns first",
			configYAML: `config:
  - command: bug
    template_url: https://github.com/owner1/repo1/blob/main/bug.yml
    channel_id:
      - "123456789"
  - command: feature
    template_url: https://github.com/owner2/repo2/blob/main/feature.yml
    channel_id:
      - "987654321"
`,
			wantOwner: "owner1",
			wantRepo:  "repo1",
		},
		{
			name: "config without template URL",
			configYAML: `config:
  - command: bug
    channel_id:
      - "123456789"
    title: Bug Report
    fields:
      - custom_id: bug_title
        label: Title
        style: short
        required: true
`,
			wantOwner: "",
			wantRepo:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0644); err != nil {
				t.Fatalf("Failed to write temp config file: %v", err)
			}

			// Load the config
			if err := LoadModals(configPath); err != nil {
				t.Fatalf("LoadModals() error = %v", err)
			}

			// Test GetOwnerAndRepo
			gotOwner, gotRepo := GetOwnerAndRepo()

			if gotOwner != tt.wantOwner {
				t.Errorf("GetOwnerAndRepo() owner = %q, want %q", gotOwner, tt.wantOwner)
			}

			if gotRepo != tt.wantRepo {
				t.Errorf("GetOwnerAndRepo() repo = %q, want %q", gotRepo, tt.wantRepo)
			}

			// Reset loadedModals for next test
			loadedModals = nil
		})
	}
}

func TestGetOwnerAndRepo_NoModalsLoaded(t *testing.T) {
	// Ensure loadedModals is nil
	loadedModals = nil

	owner, repo := GetOwnerAndRepo()

	if owner != "" {
		t.Errorf("GetOwnerAndRepo() with no modals loaded, owner = %q, want empty string", owner)
	}

	if repo != "" {
		t.Errorf("GetOwnerAndRepo() with no modals loaded, repo = %q, want empty string", repo)
	}
}
