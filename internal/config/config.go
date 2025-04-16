// Package config provides configuration management for the application
package config

import (
	"bufio"
	"fmt"
	"metamorph/internal/agent/tools"
	"metamorph/internal/logger"
	"os"
	"strconv"

	"github.com/anthropics/anthropic-sdk-go"
)

// Config contains all configuration for the application
type Config struct {
	// API settings
	AnthropicAPIKey string
	Model           string
	MaxTokens       int64

	// User interface settings
	GetUserMessage func() (string, bool)

	// Agent settings
	Client *anthropic.Client
	Tools  []tools.ToolDefinition
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	log := logger.Get()
	log.Debug().Msg("Loading configuration from environment")

	config := &Config{
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		Model:           getEnvOrDefault("CLAUDE_MODEL", anthropic.ModelClaude3_5HaikuLatest),
	}

	log.Debug().Str("model", config.Model).Msg("Loaded model configuration")

	// Parse max tokens
	maxTokensStr := getEnvOrDefault("MAX_TOKENS", "1024")
	maxTokens, err := strconv.ParseInt(maxTokensStr, 10, 64)
	if err != nil {
		log.Error().Err(err).Str("value", maxTokensStr).Msg("Invalid MAX_TOKENS value")
		return nil, fmt.Errorf("invalid MAX_TOKENS value: %w", err)
	}
	log.Debug().Int64("maxTokens", maxTokens).Msg("Loaded max tokens configuration")
	config.MaxTokens = maxTokens

	// Validate required config
	if config.AnthropicAPIKey == "" {
		log.Error().Msg("ANTHROPIC_API_KEY environment variable is not set")
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable is not set")
	}
	log.Debug().Msg("API key loaded successfully")

	return config, nil
}

// WithDefaults sets default values for configuration fields that aren't set
func (c *Config) WithDefaults() *Config {
	log := logger.Get()
	log.Debug().Msg("Applying default configuration values")
	// Set default model if not specified
	if c.Model == "" {
		c.Model = anthropic.ModelClaude3_7SonnetLatest
	}

	// Set default max tokens if not specified
	if c.MaxTokens <= 0 {
		c.MaxTokens = 1024
	}

	// Set default user message function if not specified
	if c.GetUserMessage == nil {
		scanner := bufio.NewScanner(os.Stdin)
		c.GetUserMessage = func() (string, bool) {
			if !scanner.Scan() {
				return "", false
			}
			return scanner.Text(), true
		}
	}

	// Set default client if not specified
	if c.Client == nil {
		// Create a new client with the API key
		client := anthropic.NewClient()
		// The client uses the API key from the ANTHROPIC_API_KEY environment variable
		c.Client = &client
	}

	// Set default tools if not specified
	if c.Tools == nil {
		c.Tools = tools.GetAllTools()
	}

	return c
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	log := logger.Get()
	log.Debug().Msg("Validating configuration")
	if c.Client == nil {
		log.Error().Msg("Claude client is not configured")
		return fmt.Errorf("Claude client is required")
	}

	if c.GetUserMessage == nil {
		log.Error().Msg("GetUserMessage function is not configured")
		return fmt.Errorf("GetUserMessage function is required")
	}

	if len(c.Tools) == 0 {
		logger.Get().Warn().Msg("No tools configured for the agent")
	}

	return nil
}

// getEnvOrDefault gets an environment variable or returns the default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
