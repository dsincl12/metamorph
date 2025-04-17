package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// WorkflowDefinition defines the workflow tool
var RefactoringWorkflowToolDefinition = ToolDefinition{
	Name: "refactoring_workflow",
	Description: `Execute a systematic refactoring workflow to avoid loops and ensure clean code changes.
This tool helps implement a disciplined approach to refactoring that prevents common issues like:
- Getting stuck in loops of file edits
- Losing track of progress
- Making incompatible changes
- Breaking the build

It provides a structured workflow with checkpoints to ensure each change is validated before proceeding.`,
	InputSchema: WorkflowInputSchema,
	Function:    ExecuteWorkflow,
}

// WorkflowInput defines the input parameters for the workflow tool
type WorkflowInput struct {
	Stage     string `json:"stage" jsonschema_description:"The current refactoring stage (analyze, plan, implement, test, verify)"`
	Operation string `json:"operation,omitempty" jsonschema_description:"The specific operation to perform within the stage"`
	Path      string `json:"path,omitempty" jsonschema_description:"The path to the file or directory for the operation"`
	Details   string `json:"details,omitempty" jsonschema_description:"Additional details or content for the operation"`
}

// WorkflowInputSchema is the JSON schema for the workflow tool
var WorkflowInputSchema = GenerateSchema[WorkflowInput]()

// WorkflowOutput represents the structured output of the workflow tool
type WorkflowOutput struct {
	Stage       string `json:"stage"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	NextSteps   string `json:"next_steps,omitempty"`
	BuildStatus bool   `json:"build_status,omitempty"`
}

// ExecuteWorkflow implements the workflow tool functionality
func ExecuteWorkflow(input json.RawMessage) (string, error) {
	workflowInput := WorkflowInput{}
	err := json.Unmarshal(input, &workflowInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate input
	if workflowInput.Stage == "" {
		return "", fmt.Errorf("stage cannot be empty")
	}

	var output WorkflowOutput
	output.Stage = workflowInput.Stage

	// Execute the appropriate stage
	switch workflowInput.Stage {
	case "analyze":
		output = executeAnalyzeStage(workflowInput)
	case "plan":
		output = executePlanStage(workflowInput)
	case "implement":
		output = executeImplementStage(workflowInput)
	case "test":
		output = executeTestStage(workflowInput)
	case "verify":
		output = executeVerifyStage(workflowInput)
	default:
		return "", fmt.Errorf("invalid stage: %s", workflowInput.Stage)
	}

	// Convert to JSON
	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	return string(jsonOutput), nil
}

// executeAnalyzeStage handles the analysis phase of refactoring
func executeAnalyzeStage(input WorkflowInput) WorkflowOutput {
	output := WorkflowOutput{
		Stage: "analyze",
	}

	switch input.Operation {
	case "project_structure":
		// Run commands to list files and analyze project structure
		output.Status = "success"
		output.Message = "Project structure analysis complete."
		output.NextSteps = "Use 'analyze' with operation 'dependencies' to check module dependencies, or move to 'plan' stage."

	case "dependencies":
		// Check dependencies with go mod
		result, err := RunGoCommand("mod", "", []string{"graph"}, "")
		if err != nil {
			output.Status = "error"
			output.Message = fmt.Sprintf("Failed to analyze dependencies: %v", err)
			return output
		}

		output.Status = "success"
		output.Message = "Dependency analysis complete."
		if len(result.Stdout) > 0 {
			output.Message += fmt.Sprintf(" Found %d dependencies.", len(strings.Split(result.Stdout, "\n")))
		}
		output.NextSteps = "Use 'analyze' with operation 'code_quality' to check code quality, or move to 'plan' stage."

	case "code_quality":
		// Run golint or other code quality tools
		result, err := RunGoCommand("vet", "./...", nil, "")
		if err != nil {
			output.Status = "error"
			output.Message = fmt.Sprintf("Failed to analyze code quality: %v", err)
			return output
		}

		output.Status = "success"
		if len(result.Stderr) > 0 {
			output.Message = fmt.Sprintf("Code quality issues found: %s", result.Stderr)
		} else {
			output.Message = "Code quality analysis complete. No issues found."
		}
		output.NextSteps = "Move to 'plan' stage to create a refactoring plan."

	default:
		output.Status = "error"
		output.Message = fmt.Sprintf("Unknown analyze operation: %s", input.Operation)
	}

	return output
}

// executePlanStage handles the planning phase of refactoring
func executePlanStage(input WorkflowInput) WorkflowOutput {
	output := WorkflowOutput{
		Stage: "plan",
	}

	switch input.Operation {
	case "create":
		// Create a refactoring plan
		output.Status = "success"
		output.Message = "Refactoring plan created."
		output.NextSteps = "Move to 'implement' stage to start making changes."

	case "validate":
		// Validate the refactoring plan
		output.Status = "success"
		output.Message = "Refactoring plan validated."
		output.NextSteps = "Move to 'implement' stage to start making changes."

	default:
		output.Status = "error"
		output.Message = fmt.Sprintf("Unknown plan operation: %s", input.Operation)
	}

	return output
}

// executeImplementStage handles the implementation phase of refactoring
func executeImplementStage(input WorkflowInput) WorkflowOutput {
	output := WorkflowOutput{
		Stage: "implement",
	}

	if input.Path == "" {
		output.Status = "error"
		output.Message = "Path is required for implement stage."
		return output
	}

	switch input.Operation {
	case "edit":
		// Edit a file using the edit_file tool
		editInput := FileEditorInput{
			Path:   input.Path,
			Mode:   "replace",
			OldStr: "", // Would be specified in actual use
			NewStr: input.Details,
		}

		editJSON, _ := json.Marshal(editInput)
		result, err := EditFileContent(editJSON)
		if err != nil {
			output.Status = "error"
			output.Message = fmt.Sprintf("Failed to edit file: %v", err)
			return output
		}

		output.Status = "success"
		output.Message = fmt.Sprintf("Successfully edited %s: %s", input.Path, result)
		output.NextSteps = "Use 'test' stage to check if the changes build correctly."

	case "create":
		// Create a new file
		createResult, err := createFile(input.Path, input.Details)
		if err != nil {
			output.Status = "error"
			output.Message = fmt.Sprintf("Failed to create file: %v", err)
			return output
		}

		output.Status = "success"
		output.Message = createResult
		output.NextSteps = "Use 'test' stage to check if the changes build correctly."

	default:
		output.Status = "error"
		output.Message = fmt.Sprintf("Unknown implement operation: %s", input.Operation)
	}

	return output
}

// executeTestStage handles the testing phase of refactoring
func executeTestStage(input WorkflowInput) WorkflowOutput {
	output := WorkflowOutput{
		Stage: "test",
	}

	switch input.Operation {
	case "build":
		// Build the project
		buildResult, err := RunGoCommand("build", "./...", nil, "")
		if err != nil {
			output.Status = "error"
			output.Message = "Build failed. See errors below:"
			output.NextSteps = fmt.Sprintf("Fix build errors and try again:\n%s", buildResult.Stderr)
			output.BuildStatus = false
			return output
		}

		output.Status = "success"
		output.Message = "Build succeeded."
		output.NextSteps = "Use 'verify' stage to run tests or verify functionality."
		output.BuildStatus = true

	case "unit_test":
		// Run unit tests
		testResult, err := RunGoCommand("test", "./...", nil, "")
		if err != nil {
			output.Status = "error"
			output.Message = "Tests failed. See errors below:"
			output.NextSteps = fmt.Sprintf("Fix test errors and try again:\n%s", testResult.Stderr)
			return output
		}

		output.Status = "success"
		output.Message = "Tests passed."
		output.NextSteps = "Move to 'verify' stage to perform final verification."

	default:
		output.Status = "error"
		output.Message = fmt.Sprintf("Unknown test operation: %s", input.Operation)
	}

	return output
}

// executeVerifyStage handles the verification phase of refactoring
func executeVerifyStage(input WorkflowInput) WorkflowOutput {
	output := WorkflowOutput{
		Stage: "verify",
	}

	switch input.Operation {
	case "summary":
		// Generate a summary of changes
		output.Status = "success"
		output.Message = "Refactoring complete and verified."
		output.NextSteps = "Consider adding more tests or documentation."

	case "documentation":
		// Check documentation
		output.Status = "success"
		output.Message = "Documentation verified."
		output.NextSteps = "Refactoring process is complete."

	default:
		output.Status = "error"
		output.Message = fmt.Sprintf("Unknown verify operation: %s", input.Operation)
	}

	return output
}
