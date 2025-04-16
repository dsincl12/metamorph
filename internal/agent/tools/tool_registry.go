// Package tools provides functionality for Claude to interact with the system through defined tools.
// Each tool represents a specific capability that the AI can use, such as reading files,
// listing directories, editing files, or getting the current time.
package tools

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"
)

// ToolDefinition defines a tool that can be used by the agent.
// Each tool has a name, description, input schema, and an implementation function.
type ToolDefinition struct {
	// Name is the identifier of the tool used by Claude to invoke it
	Name string `json:"name"`

	// Description explains what the tool does and when to use it
	Description string `json:"description"`

	// InputSchema defines the expected parameters and their types
	InputSchema anthropic.ToolInputSchemaParam `json:"input_schema"`

	// Function is the actual implementation that will be executed when the tool is used
	Function func(input json.RawMessage) (string, error)
}

// GenerateSchema creates a JSON schema for the given type
func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}

	var v T

	schema := reflector.Reflect(v)

	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

// GetAllTools returns all available tools
func GetAllTools() []ToolDefinition {
	return []ToolDefinition{
		FileReaderToolDefinition,
		FileListerToolDefinition,
		FileEditorToolDefinition,
		TimeProviderToolDefinition,
		GoCommandToolDefinition,
		GoErrorFixToolDefinition,
		RefactoringWorkflowToolDefinition,
		ActionLimiterToolDefinition,
		GitOperationsToolDefinition,
		FileOperationsToolDefinition,
	}
}
