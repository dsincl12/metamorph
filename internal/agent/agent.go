package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"metamorph/internal/agent/tools"
	"metamorph/internal/logger"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// LoopProtection holds settings for preventing infinite loops
type LoopProtection struct {
	MaxConsecutiveToolUses int           // Maximum number of consecutive tool uses without user input
	MaxToolUsesPerMinute   int           // Maximum tool uses per minute
	MaxSessionDuration     time.Duration // Maximum total session time
	MaxSameToolCalls       int           // Maximum calls to the same tool in sequence

	// Internal tracking
	ConsecutiveToolUses int
	LastToolName        string
	SameToolCallCount   int
	ToolUseStartTime    time.Time
	ToolUseCount        int
	SessionStartTime    time.Time
}

// NewLoopProtection creates LoopProtection with default settings
func NewLoopProtection() LoopProtection {
	return LoopProtection{
		MaxConsecutiveToolUses: 15,               // Stop after 15 consecutive tool uses
		MaxToolUsesPerMinute:   30,               // No more than 30 tool uses per minute
		MaxSessionDuration:     30 * time.Minute, // Max 30 minutes per session
		MaxSameToolCalls:       5,                // No more than 5 consecutive calls to same tool

		// Initialize tracking
		ConsecutiveToolUses: 0,
		LastToolName:        "",
		SameToolCallCount:   0,
		ToolUseStartTime:    time.Now(),
		ToolUseCount:        0,
		SessionStartTime:    time.Now(),
	}
}

// Agent represents a Claude-powered conversational agent with tool usage
type Agent struct {
	client         *anthropic.Client
	getUserMessage func() (string, bool)
	tools          []tools.ToolDefinition
	model          string
	maxTokens      int64
	loopProtection LoopProtection
}

// Config holds configuration options for creating a new Agent
type Config struct {
	Client         *anthropic.Client
	GetUserMessage func() (string, bool)
	Tools          []tools.ToolDefinition
	Model          string
	MaxTokens      int64
	LoopProtection *LoopProtection // Optional custom loop protection settings
}

// New creates a new Agent with the provided configuration
func New(config Config) *Agent {
	log := logger.Get()
	log.Debug().
		Str("model", config.Model).
		Int64("maxTokens", config.MaxTokens).
		Int("numTools", len(config.Tools)).
		Msg("Creating new agent")

	loopProtection := NewLoopProtection()
	if config.LoopProtection != nil {
		loopProtection = *config.LoopProtection
		log.Debug().
			Int("maxConsecutiveToolUses", loopProtection.MaxConsecutiveToolUses).
			Int("maxToolUsesPerMinute", loopProtection.MaxToolUsesPerMinute).
			Int("maxSameToolCalls", loopProtection.MaxSameToolCalls).
			Dur("maxSessionDuration", loopProtection.MaxSessionDuration).
			Msg("Using custom loop protection settings")
	}

	return &Agent{
		client:         config.Client,
		getUserMessage: config.GetUserMessage,
		tools:          config.Tools,
		model:          config.Model,
		maxTokens:      config.MaxTokens,
		loopProtection: loopProtection,
	}
}

// Run starts the agent's conversation loop
func (a *Agent) Run(ctx context.Context) error {
	conversation := []anthropic.MessageParam{}
	logger.Get().Info().Msg("Starting chat with Claude (use 'ctrl-c' to quit)")

	a.loopProtection.SessionStartTime = time.Now()

	readUserInput := true
	for {
		// Check session time limit
		if time.Since(a.loopProtection.SessionStartTime) > a.loopProtection.MaxSessionDuration {
			logger.Get().Warn().
				Dur("sessionDuration", time.Since(a.loopProtection.SessionStartTime)).
				Dur("limit", a.loopProtection.MaxSessionDuration).
				Msg("Session time limit reached. Please restart the agent if needed.")
			break
		}

		if readUserInput {
			a.loopProtection.ConsecutiveToolUses = 0
			a.loopProtection.LastToolName = ""
			a.loopProtection.SameToolCallCount = 0

			if !a.readUserInputToConversation(&conversation) {
				break
			}
		}

		message, err := a.generateResponse(ctx, conversation)
		if err != nil {
			return err
		}

		conversation = append(conversation, message.ToParam())

		// Process any tool uses and add results to conversation
		readUserInput, err = a.processToolUsages(message, &conversation)
		if err != nil {
			logger.Get().Error().Err(err).Msg("Error processing tool usage")
			readUserInput = true
		}
	}

	return nil
}

// readUserInputToConversation prompts for and adds user input to the conversation
// Returns false if input reading fails
func (a *Agent) readUserInputToConversation(conversation *[]anthropic.MessageParam) bool {
	fmt.Print("\u001b[94mYou\u001b[0m: ") // Keep this as fmt.Print for better UX
	userInput, ok := a.getUserMessage()
	if !ok {
		return false
	}

	userMessage := anthropic.NewUserMessage(anthropic.NewTextBlock(userInput))
	*conversation = append(*conversation, userMessage)
	return true
}

// processToolUsages handles any tool uses in the message
// Returns whether user input should be read next (true) or not (false) and any errors
func (a *Agent) processToolUsages(message *anthropic.Message, conversation *[]anthropic.MessageParam) (bool, error) {
	toolResults := []anthropic.ContentBlockParamUnion{}

	hasToolUses := false
	for _, content := range message.Content {
		switch content.Type {
		case "text":
			fmt.Printf("\u001b[95mClaude\u001b[0m: %s\n", content.Text)
		case "tool_use":
			hasToolUses = true

			// Check loop protection limits
			a.loopProtection.ConsecutiveToolUses++
			a.loopProtection.ToolUseCount++

			// Check consecutive tool use limit
			if a.loopProtection.ConsecutiveToolUses > a.loopProtection.MaxConsecutiveToolUses {
				err := fmt.Errorf("too many consecutive tool uses (%d) without user input",
					a.loopProtection.ConsecutiveToolUses)
				logger.Get().Error().
					Int("consecutiveUses", a.loopProtection.ConsecutiveToolUses).
					Int("limit", a.loopProtection.MaxConsecutiveToolUses).
					Msg("Consecutive tool use limit exceeded")
				return true, err
			}

			// Check rate limit
			elapsed := time.Since(a.loopProtection.ToolUseStartTime).Minutes()
			if elapsed <= 1 && a.loopProtection.ToolUseCount >= a.loopProtection.MaxToolUsesPerMinute {
				err := fmt.Errorf("tool use rate limit exceeded (%d uses in %.1f seconds)",
					a.loopProtection.ToolUseCount, elapsed*60)
				logger.Get().Error().
					Int("useCount", a.loopProtection.ToolUseCount).
					Float64("elapsedMinutes", elapsed).
					Int("limit", a.loopProtection.MaxToolUsesPerMinute).
					Msg("Tool use rate limit exceeded")
				return true, err
			}
			if elapsed > 1 {
				// Reset rate limiting after 1 minute
				a.loopProtection.ToolUseStartTime = time.Now()
				a.loopProtection.ToolUseCount = 1
			}

			// Check same tool call limit
			if a.loopProtection.LastToolName == content.Name {
				a.loopProtection.SameToolCallCount++
				if a.loopProtection.SameToolCallCount >= a.loopProtection.MaxSameToolCalls {
					err := fmt.Errorf("too many consecutive calls to the same tool: %s (%d calls)",
						content.Name, a.loopProtection.SameToolCallCount)
					logger.Get().Error().
						Str("tool", content.Name).
						Int("callCount", a.loopProtection.SameToolCallCount).
						Int("limit", a.loopProtection.MaxSameToolCalls).
						Msg("Same tool call limit exceeded")
					return true, err
				}
			} else {
				a.loopProtection.LastToolName = content.Name
				a.loopProtection.SameToolCallCount = 1
			}

			result := a.executeTool(content.ID, content.Name, content.Input)
			toolResults = append(toolResults, result)
		}
	}

	if !hasToolUses {
		return true, nil // Read user input next
	}

	// Add tool results to conversation and continue without user input
	*conversation = append(*conversation, anthropic.NewUserMessage(toolResults...))
	return false, nil
}

// executeTool runs the specified tool and returns its result
func (a *Agent) executeTool(id, name string, input json.RawMessage) anthropic.ContentBlockParamUnion {
	toolDef, found := a.findTool(name)
	if !found {
		logger.Get().Error().
			Str("tool", name).
			Msg("Tool not found")
		return anthropic.NewToolResultBlock(id, "tool not found", true)
	}

	log := logger.Get()
	log.Info().
		Str("tool", name).
		RawJSON("input", input).
		Msg("Executing tool")
	response, err := toolDef.Function(input)
	if err != nil {
		return anthropic.NewToolResultBlock(id, err.Error(), true)
	}

	return anthropic.NewToolResultBlock(id, response, false)
}

// findTool searches for a tool by name
func (a *Agent) findTool(name string) (tools.ToolDefinition, bool) {
	for _, tool := range a.tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return tools.ToolDefinition{}, false
}

// generateResponse sends the conversation to Claude and gets a response
func (a *Agent) generateResponse(ctx context.Context, conversation []anthropic.MessageParam) (*anthropic.Message, error) {
	anthropicTools := a.prepareToolDefinitions()

	return a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.model,
		MaxTokens: a.maxTokens,
		Messages:  conversation,
		Tools:     anthropicTools,
	})
}

// prepareToolDefinitions converts local tool definitions to Anthropic format
func (a *Agent) prepareToolDefinitions() []anthropic.ToolUnionParam {
	anthropicTools := make([]anthropic.ToolUnionParam, len(a.tools))

	for i, tool := range a.tools {
		anthropicTools[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: tool.InputSchema,
			},
		}
	}

	return anthropicTools
}
