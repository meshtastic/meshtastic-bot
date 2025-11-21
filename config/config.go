package config

import (
	"fmt"
	"strings"
)

type Config struct {
	ServerID        string
	DiscordToken    string
	GithubToken     string
	RemoveCommands  bool
	ConfigPath      string
	FAQPath         string
	HealthCheckPort string
}

// TemplateURL represents a parsed GitHub issue template URL
type TemplateURL struct {
	original string
	owner    string
	repo     string
	path     string
}

// Load initializes configuration with proper precedence:
// 1. Defaults
// 2. Environment variables
// 3. Command-line flags
// 4. Validation
func Load() (*Config, error) {
	cfg := &Config{}
	setDefaults(cfg)
	loadEnv(cfg)
	applyFlags(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// ParseTemplateURL parses and validates a GitHub template URL
// Example: https://github.com/meshtastic/web/blob/main/.github/ISSUE_TEMPLATE/bug.yml
func ParseTemplateURL(templateURL string) (*TemplateURL, error) {
	if templateURL == "" {
		return nil, fmt.Errorf("template URL cannot be empty")
	}

	url := strings.TrimPrefix(templateURL, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "github.com/")

	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL format: %s", templateURL)
	}

	return &TemplateURL{
		original: templateURL,
		owner:    parts[0],
		repo:     parts[1],
		path:     strings.Join(parts[2:], "/"),
	}, nil
}

func (t *TemplateURL) Owner() string {
	return t.owner
}

func (t *TemplateURL) Repo() string {
	return t.repo
}

// IssueAPIURL returns the GitHub API endpoint for creating issues
// Example: https://api.github.com/repos/meshtastic/web/issues
func (t *TemplateURL) IssueAPIURL() string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", t.owner, t.repo)
}

// RawURL returns the raw content URL for fetching the template YAML
// Example: https://raw.githubusercontent.com/meshtastic/web/main/.github/ISSUE_TEMPLATE/bug.yml
func (t *TemplateURL) RawURL() string {
	// Remove /blob/ from path if present
	path := strings.Replace(t.path, "blob/", "", 1)
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", t.owner, t.repo, path)
}

// Returns the original URL
func (t *TemplateURL) String() string {
	return t.original
}
