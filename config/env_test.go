package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_Validate(t *testing.T) {
	// Create a temporary config file for testing
	tmpDir := t.TempDir()
	validConfigFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(validConfigFile, []byte("test: config"), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid configuration",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "123456",
				GithubToken:  "gh-token",
				ConfigPath:   validConfigFile,
			},
			wantErr: false,
		},
		{
			name: "missing discord token",
			config: &Config{
				DiscordToken: "",
				ServerID:     "123456",
				GithubToken:  "gh-token",
				ConfigPath:   validConfigFile,
			},
			wantErr: true,
			errMsg:  "DISCORD_TOKEN is required",
		},
		{
			name: "missing server ID",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "",
				GithubToken:  "gh-token",
				ConfigPath:   validConfigFile,
			},
			wantErr: true,
			errMsg:  "DISCORD_SERVER_ID is required",
		},
		{
			name: "missing github token",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "123456",
				GithubToken:  "",
				ConfigPath:   validConfigFile,
			},
			wantErr: true,
			errMsg:  "GITHUB_TOKEN is required",
		},
		{
			name: "missing config path",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "123456",
				GithubToken:  "gh-token",
				ConfigPath:   "",
			},
			wantErr: true,
			errMsg:  "CONFIG_PATH is required",
		},
		{
			name: "config file does not exist",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "123456",
				GithubToken:  "gh-token",
				ConfigPath:   "/nonexistent/path/config.yaml",
			},
			wantErr: true,
			errMsg:  "file does not exist",
		},
		{
			name: "config path is directory",
			config: &Config{
				DiscordToken: "test-token",
				ServerID:     "123456",
				GithubToken:  "gh-token",
				ConfigPath:   tmpDir,
			},
			wantErr: true,
			errMsg:  "must be a file, not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{
			name:     "HealthCheckPort default",
			got:      cfg.HealthCheckPort,
			expected: DefaultHealthCheckPort,
		},
		{
			name:     "FAQPath default",
			got:      cfg.FAQPath,
			expected: DefaultFAQPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("setDefaults() %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}

	if cfg.RemoveCommands != false {
		t.Errorf("setDefaults() RemoveCommands = %v, want false", cfg.RemoveCommands)
	}
}

func TestLoadEnv(t *testing.T) {
	// Save original env vars and restore after test
	origVars := map[string]string{
		EnvDiscordServerID: os.Getenv(EnvDiscordServerID),
		EnvDiscordToken:    os.Getenv(EnvDiscordToken),
		EnvGitHubToken:     os.Getenv(EnvGitHubToken),
		EnvConfigPath:      os.Getenv(EnvConfigPath),
		EnvFAQPath:         os.Getenv(EnvFAQPath),
		EnvHealthCheckPort: os.Getenv(EnvHealthCheckPort),
	}
	defer func() {
		for key, val := range origVars {
			if val == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, val)
			}
		}
	}()

	tests := []struct {
		name    string
		envVars map[string]string
		check   func(*testing.T, *Config)
	}{
		{
			name: "loads environment variables",
			envVars: map[string]string{
				EnvDiscordServerID: "test-server-123",
				EnvDiscordToken:    "test-token-456",
				EnvGitHubToken:     "gh-token-789",
				EnvConfigPath:      "/test/config.yaml",
				EnvFAQPath:         "/test/faq.yaml",
				EnvHealthCheckPort: "9090",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.ServerID != "test-server-123" {
					t.Errorf("ServerID = %q, want %q", cfg.ServerID, "test-server-123")
				}
				if cfg.DiscordToken != "test-token-456" {
					t.Errorf("DiscordToken = %q, want %q", cfg.DiscordToken, "test-token-456")
				}
				if cfg.GithubToken != "gh-token-789" {
					t.Errorf("GithubToken = %q, want %q", cfg.GithubToken, "gh-token-789")
				}
				if cfg.ConfigPath != "/test/config.yaml" {
					t.Errorf("ConfigPath = %q, want %q", cfg.ConfigPath, "/test/config.yaml")
				}
				if cfg.FAQPath != "/test/faq.yaml" {
					t.Errorf("FAQPath = %q, want %q", cfg.FAQPath, "/test/faq.yaml")
				}
				if cfg.HealthCheckPort != "9090" {
					t.Errorf("HealthCheckPort = %q, want %q", cfg.HealthCheckPort, "9090")
				}
			},
		},
		{
			name:    "handles missing environment variables",
			envVars: map[string]string{},
			check: func(t *testing.T, cfg *Config) {
				// All fields should be empty strings when no env vars are set
				if cfg.ServerID != "" {
					t.Errorf("ServerID = %q, want empty string", cfg.ServerID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all env vars
			for key := range origVars {
				os.Unsetenv(key)
			}

			// Set test env vars
			for key, val := range tt.envVars {
				os.Setenv(key, val)
			}

			cfg := &Config{}
			loadEnv(cfg)

			tt.check(t, cfg)
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
