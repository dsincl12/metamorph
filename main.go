package main

import (
	"context"
	"metamorph/internal/agent"
	"metamorph/internal/config"
	"metamorph/internal/logger"
	"os"
	"time"
)

func main() {
	// Initialize logger
	debug := os.Getenv("DEBUG") == "true"
	logger.Initialize(debug)
	logger.Get().Info().Bool("debug", debug).Msg("Logger initialized")

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Get().Fatal().Err(err).Msg("Error loading configuration")
		os.Exit(1)
	}

	// Apply defaults and validate
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil {
		logger.Get().Fatal().Err(err).Msg("Invalid configuration")
		os.Exit(1)
	}

	// Configure loop protection
	loopProtection := agent.NewLoopProtection()
	loopProtection.MaxConsecutiveToolUses = 100
	loopProtection.MaxToolUsesPerMinute = 20
	loopProtection.MaxSameToolCalls = 100
	loopProtection.MaxSessionDuration = 15 * time.Minute

	// Create and start the agent
	agentConfig := agent.Config{
		Client:         cfg.Client,
		GetUserMessage: cfg.GetUserMessage,
		Tools:          cfg.Tools,
		Model:          cfg.Model,
		MaxTokens:      cfg.MaxTokens,
		LoopProtection: &loopProtection,
	}

	agentInstance := agent.New(agentConfig)

	if err := agentInstance.Run(context.Background()); err != nil {
		logger.Get().Fatal().Err(err).Msg("Agent run failed")
		os.Exit(1)
	}
}
