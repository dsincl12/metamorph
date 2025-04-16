package agent

import (
	"fmt"
)

// ErrLoopProtection indicates that a loop protection limit was reached
type ErrLoopProtection struct {
	Limit     string
	Current   int
	Max       int
	ToolName  string
	TimeFrame string
}

func (e *ErrLoopProtection) Error() string {
	if e.TimeFrame != "" {
		return fmt.Sprintf("loop protection: %s limit reached (%d/%d in %s)",
			e.Limit, e.Current, e.Max, e.TimeFrame)
	}
	return fmt.Sprintf("loop protection: %s limit reached (%d/%d)",
		e.Limit, e.Current, e.Max)
}

// ErrToolExecution indicates an error occurred while executing a tool
type ErrToolExecution struct {
	ToolName string
	Err      error
}

func (e *ErrToolExecution) Error() string {
	return fmt.Sprintf("tool execution error (%s): %v", e.ToolName, e.Err)
}

func (e *ErrToolExecution) Unwrap() error {
	return e.Err
}

// ErrToolNotFound indicates a requested tool does not exist
type ErrToolNotFound struct {
	ToolName string
}

func (e *ErrToolNotFound) Error() string {
	return fmt.Sprintf("tool not found: %s", e.ToolName)
}
