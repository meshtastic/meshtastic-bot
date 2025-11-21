package config

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Environment variable names
const (
	EnvDiscordServerID = "DISCORD_SERVER_ID"
	EnvDiscordToken    = "DISCORD_TOKEN"
	EnvGitHubToken     = "GITHUB_TOKEN"
	EnvConfigPath      = "CONFIG_PATH"
	EnvFAQPath         = "FAQ_PATH"
	EnvHealthCheckPort = "HEALTHCHECK_PORT"
	EnvEnvironment     = "ENV"
)

// Default values
const (
	DefaultHealthCheckPort = "8080"
	DefaultFAQPath         = "faq.yaml"
	DefaultEnvironment     = "dev"
)

// setDefaults initializes the Config with default values
func setDefaults(cfg *Config) {
	cfg.HealthCheckPort = DefaultHealthCheckPort
	cfg.FAQPath = DefaultFAQPath
	cfg.RemoveCommands = false
}

// loadFromEnv loads configuration from environment variables
// First loads from .env.{ENV} file (e.g., .env.dev or .env.prod)
// Then loads from system environment variables (which take precedence)
func loadEnv(cfg *Config) {
	// Load environment-specific .env file
	loadEnvFile()

	envMappings := map[string]*string{
		EnvDiscordServerID: &cfg.ServerID,
		EnvDiscordToken:    &cfg.DiscordToken,
		EnvGitHubToken:     &cfg.GithubToken,
		EnvConfigPath:      &cfg.ConfigPath,
		EnvFAQPath:         &cfg.FAQPath,
		EnvHealthCheckPort: &cfg.HealthCheckPort,
	}

	for envVar, field := range envMappings {
		if val := os.Getenv(envVar); val != "" {
			*field = val
		}
	}
}

// loadEnvFile loads the appropriate .env file based on the ENV variable
// Precedence: .env.{ENV} > .env
func loadEnvFile() {
	env := os.Getenv(EnvEnvironment)
	if env == "" {
		env = DefaultEnvironment
	}

	// Try to load environment-specific file first
	envFile := fmt.Sprintf(".env.%s", env)
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("No %s file found, trying .env", envFile)

		// Fall back to .env
		if err := godotenv.Load(); err != nil {
			log.Printf("No .env file found, using system environment variables only")
		}
	} else {
		log.Printf("Loaded configuration from %s", envFile)
	}
}

// applyFlags overrides configuration with command-line flags
func applyFlags(cfg *Config) {
	flag.StringVar(&cfg.ServerID, "server-id", cfg.ServerID, "Discord server ID")
	flag.StringVar(&cfg.DiscordToken, "discord-token", cfg.DiscordToken, "Discord bot access token")
	flag.StringVar(&cfg.GithubToken, "github-token", cfg.GithubToken, "GitHub access token")
	flag.StringVar(&cfg.ConfigPath, "config-path", cfg.ConfigPath, "Location of modal yaml configuration file")
	flag.StringVar(&cfg.FAQPath, "faq-path", cfg.FAQPath, "Location of FAQ yaml file")
	flag.StringVar(&cfg.HealthCheckPort, "healthcheck-port", cfg.HealthCheckPort, "Health check HTTP server port")
	flag.BoolVar(&cfg.RemoveCommands, "remove-commands", cfg.RemoveCommands, "Remove Discord commands on shutdown")
	flag.Parse()
}

// Validate checks if required configuration values are present
func (c *Config) Validate() error {
	requiredFields := map[string]string{
		EnvDiscordToken:    c.DiscordToken,
		EnvDiscordServerID: c.ServerID,
		EnvGitHubToken:     c.GithubToken,
		EnvConfigPath:      c.ConfigPath,
	}

	for envVar, value := range requiredFields {
		if value == "" {
			return fmt.Errorf("%s is required", envVar)
		}
	}

	// Validate the config path exists and is a file
	if info, err := os.Stat(c.ConfigPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s file does not exist: %s", EnvConfigPath, c.ConfigPath)
		}
		return fmt.Errorf("%s error: %w", EnvConfigPath, err)
	} else if info.IsDir() {
		return fmt.Errorf("%s must be a file, not a directory: %s", EnvConfigPath, c.ConfigPath)
	}

	return nil
}
