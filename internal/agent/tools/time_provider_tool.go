package tools

import (
	"encoding/json"
	"time"
)

// GetTimeDefinition defines the get_time tool
var TimeProviderToolDefinition = ToolDefinition{
	Name:        "time_provider",
	Description: "Get the current system time. Returns the current time in ISO 8601 format.",
	InputSchema: GetTimeInputSchema,
	Function:    GetTime,
}

// GetTimeInput defines the input parameters for the get_time tool
type GetTimeInput struct {
	// We don't need any input parameters for this tool, but we still need the struct for consistency
	Format string `json:"format,omitempty" jsonschema_description:"Optional time format. If not provided, ISO 8601 format will be used."`
}

// GetTimeInputSchema is the JSON schema for the get_time tool
var GetTimeInputSchema = GenerateSchema[GetTimeInput]()

// GetTime implements the get_time tool functionality
func GetTime(input json.RawMessage) (string, error) {
	getTimeInput := GetTimeInput{}
	err := json.Unmarshal(input, &getTimeInput)
	if err != nil {
		return "", err
	}

	currentTime := time.Now()

	// If a format is provided, use it; otherwise, use ISO 8601
	timeFormat := time.RFC3339
	if getTimeInput.Format != "" {
		timeFormat = getTimeInput.Format
	}

	return currentTime.Format(timeFormat), nil
}
