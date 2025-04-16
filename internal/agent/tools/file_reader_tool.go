package tools

import (
	"encoding/json"
	"fmt"
	"os"
)

// FileReaderDefinition defines the read_file tool
var FileReaderToolDefinition = ToolDefinition{
	Name:        "read_file",
	Description: "Read the contents of a given relative file path. Use this when you want to see what's inside a file. Do not use this with directory names.",
	InputSchema: FileReaderInputSchema,
	Function:    ReadFileContent,
}

// FileReaderInput defines the input parameters for the read_file tool
type FileReaderInput struct {
	Path string `json:"path" jsonschema_description:"The relative path of a file in the working directory."`
}

// FileReaderInputSchema is the JSON schema for the read_file tool
var FileReaderInputSchema = GenerateSchema[FileReaderInput]()

// ReadFileContent implements the read_file tool functionality
func ReadFileContent(input json.RawMessage) (string, error) {
	readFileInput := FileReaderInput{}
	err := json.Unmarshal(input, &readFileInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse tool input: %w", err)
	}

	if readFileInput.Path == "" {
		return "", fmt.Errorf("path parameter is required")
	}

	content, err := os.ReadFile(readFileInput.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file '%s': %w", readFileInput.Path, err)
	}

	return string(content), nil
}
