package tools

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ActionLimiterDefinition defines the action_limiter tool
var ActionLimiterToolDefinition = ToolDefinition{
	Name: "action_limiter",
	Description: `Control and limit agent actions to prevent infinite loops and excessive operations.
This tool tracks agent actions and can enforce limits on:
- Total number of actions per session
- Number of similar actions (e.g., editing the same file)
- Rate of actions (e.g., not too many per minute)
- Duration of the session

It helps prevent the agent from getting stuck in loops or making too many rapid changes.`,
	InputSchema: ActionLimiterInputSchema,
	Function:    ActionLimiter,
}

// ActionLimiterInput defines the input parameters for the action_limiter tool
type ActionLimiterInput struct {
	Action     string `json:"action" jsonschema_description:"The action being performed (e.g., 'edit_file', 'create_file')"`
	Target     string `json:"target,omitempty" jsonschema_description:"The target of the action (e.g., file path)"`
	CheckOnly  bool   `json:"check_only,omitempty" jsonschema_description:"If true, only check limits without recording the action"`
	ResetState bool   `json:"reset_state,omitempty" jsonschema_description:"If true, reset all counters and state"`
}

// ActionLimiterInputSchema is the JSON schema for the action_limiter tool
var ActionLimiterInputSchema = GenerateSchema[ActionLimiterInput]()

// ActionStats tracks statistics about agent actions
type ActionStats struct {
	TotalActions      int                       `json:"total_actions"`
	ActionsByType     map[string]int            `json:"actions_by_type"`
	ActionsByTarget   map[string]int            `json:"actions_by_target"`
	ActionsByTypePath map[string]map[string]int `json:"actions_by_type_path"`
	StartTime         time.Time                 `json:"start_time"`
	LastActionTime    time.Time                 `json:"last_action_time"`
	ConsecutiveSame   int                       `json:"consecutive_same"`
	LastAction        string                    `json:"last_action"`
	LastTarget        string                    `json:"last_target"`
}

var (
	stats         ActionStats
	statsMutex    sync.Mutex
	isInitialized bool
)

// initializeStats initializes the stats if not already done
func initializeStats() {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	if !isInitialized {
		stats = ActionStats{
			ActionsByType:     make(map[string]int),
			ActionsByTarget:   make(map[string]int),
			ActionsByTypePath: make(map[string]map[string]int),
			StartTime:         time.Now(),
			LastActionTime:    time.Now(),
		}
		isInitialized = true
	}
}

// checkLimits checks if any limits have been exceeded
func checkLimits() (bool, string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	if !isInitialized {
		return false, ""
	}

	// Check total actions limit (e.g., 50 actions per session)
	if stats.TotalActions >= 50 {
		return true, "Total action limit exceeded (50 actions)"
	}

	// Check for too many actions on the same target (e.g., 10 edits to the same file)
	for target, count := range stats.ActionsByTarget {
		if count >= 10 {
			return true, fmt.Sprintf("Too many actions (%d) on the same target: %s", count, target)
		}
	}

	// Check for too many consecutive identical actions
	if stats.ConsecutiveSame >= 5 {
		return true, fmt.Sprintf("Too many consecutive identical actions (%d): %s on %s",
			stats.ConsecutiveSame, stats.LastAction, stats.LastTarget)
	}

	// Check for rapid actions (more than 10 actions in 5 seconds)
	timeSinceStart := time.Since(stats.StartTime)
	if stats.TotalActions > 10 && timeSinceStart.Seconds() < 5 {
		return true, fmt.Sprintf("Too many actions (%d) in a short time (%0.1f seconds)",
			stats.TotalActions, timeSinceStart.Seconds())
	}

	// Check for excessive time (e.g., 5 minutes)
	if timeSinceStart.Minutes() > 5 {
		return true, fmt.Sprintf("Session time limit exceeded (%0.1f minutes)",
			timeSinceStart.Minutes())
	}

	return false, ""
}

// recordAction records an action in the stats
func recordAction(action, target string) {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	if !isInitialized {
		stats = ActionStats{
			ActionsByType:     make(map[string]int),
			ActionsByTarget:   make(map[string]int),
			ActionsByTypePath: make(map[string]map[string]int),
			StartTime:         time.Now(),
			LastActionTime:    time.Now(),
		}
		isInitialized = true
	}

	stats.TotalActions++
	stats.ActionsByType[action]++
	stats.ActionsByTarget[target]++

	// Track actions by type and path
	if stats.ActionsByTypePath[action] == nil {
		stats.ActionsByTypePath[action] = make(map[string]int)
	}
	stats.ActionsByTypePath[action][target]++

	// Track consecutive same actions
	if action == stats.LastAction && target == stats.LastTarget {
		stats.ConsecutiveSame++
	} else {
		stats.ConsecutiveSame = 1
	}

	stats.LastAction = action
	stats.LastTarget = target
	stats.LastActionTime = time.Now()
}

// resetStats resets all stats
func resetStats() {
	statsMutex.Lock()
	defer statsMutex.Unlock()

	stats = ActionStats{
		ActionsByType:     make(map[string]int),
		ActionsByTarget:   make(map[string]int),
		ActionsByTypePath: make(map[string]map[string]int),
		StartTime:         time.Now(),
		LastActionTime:    time.Now(),
	}
}

// ActionLimiter implements the action_limiter tool functionality
func ActionLimiter(input json.RawMessage) (string, error) {
	actionLimiterInput := ActionLimiterInput{}
	err := json.Unmarshal(input, &actionLimiterInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Make sure stats are initialized
	initializeStats()

	// Reset state if requested
	if actionLimiterInput.ResetState {
		resetStats()
		return "Action stats reset successfully", nil
	}

	// Check limits
	exceeded, reason := checkLimits()
	if exceeded {
		return fmt.Sprintf("Action limit exceeded: %s", reason), nil
	}

	// Record the action if not just checking
	if !actionLimiterInput.CheckOnly && actionLimiterInput.Action != "" {
		recordAction(actionLimiterInput.Action, actionLimiterInput.Target)
	}

	// Return the current stats
	statsJSON, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal stats: %w", err)
	}

	return string(statsJSON), nil
}
