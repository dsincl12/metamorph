package agent

import (
	"context"
)

// AgentInterface defines the contract for agent implementations
type AgentInterface interface {
	// Run starts the agent's conversation loop
	Run(ctx context.Context) error
}

// Ensure Agent implements AgentInterface
var _ AgentInterface = (*Agent)(nil)
