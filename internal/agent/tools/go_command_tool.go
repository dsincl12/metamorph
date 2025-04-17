package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoRunnerDefinition defines the run_go tool
var GoCommandToolDefinition = ToolDefinition{
	Name: "go_command",
	Description: `Execute Go commands like build, run, test, etc. and capture their output.
Use this tool to compile and run Go code, identify errors, and test changes.
Common commands:
- 'build': Compile the package but don't run it
- 'run': Compile and run the package
- 'test': Run tests
- 'vet': Report likely mistakes in packages
- 'fmt': Format Go source code
- 'mod tidy': Add missing and remove unused modules
`,
	InputSchema: RunGoInputSchema,
	Function:    RunGo,
}

// RunGoInput defines the input parameters for the run_go tool
type RunGoInput struct {
	Command    string   `json:"command" jsonschema_description:"Go command to run (build, run, test, fmt, vet, etc.)"`
	Path       string   `json:"path" jsonschema_description:"Path to the Go file or directory to operate on"`
	Args       []string `json:"args,omitempty" jsonschema_description:"Additional arguments to pass to the Go command"`
	WorkingDir string   `json:"working_dir,omitempty" jsonschema_description:"Working directory (defaults to current directory if empty)"`
}

// RunGoInputSchema is the JSON schema for the run_go tool
var RunGoInputSchema = GenerateSchema[RunGoInput]()

// RunGoOutput represents the structured output of the run_go tool
type RunGoOutput struct {
	Success      bool   `json:"success"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ErrorMessage string `json:"error_message,omitempty"`
	Command      string `json:"command"`
}

// RunGo implements the run_go tool functionality
func RunGo(input json.RawMessage) (string, error) {
	runGoInput := RunGoInput{}
	err := json.Unmarshal(input, &runGoInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate input
	if runGoInput.Command == "" {
		return "", fmt.Errorf("command cannot be empty")
	}

	// Handle special case for 'mod' commands
	var args []string
	if strings.HasPrefix(runGoInput.Command, "mod ") {
		// For commands like "mod tidy", split into "mod" and "tidy"
		parts := strings.SplitN(runGoInput.Command, " ", 2)
		args = append([]string{parts[0], parts[1]}, runGoInput.Args...)
	} else {
		args = append([]string{runGoInput.Command}, runGoInput.Args...)
	}

	// Add path if provided and appropriate for the command
	if runGoInput.Path != "" {
		// Don't add path for commands that don't need it
		skipPathCommands := map[string]bool{
			"version": true,
			"env":     true,
			"bug":     true,
			"help":    true,
		}

		// Only add path for commands that operate on packages
		if !skipPathCommands[runGoInput.Command] && !strings.HasPrefix(runGoInput.Command, "mod ") {
			args = append(args, runGoInput.Path)
		}
	}

	// Set working directory
	workingDir := "."
	if runGoInput.WorkingDir != "" {
		workingDir = runGoInput.WorkingDir
		// Create directory if it doesn't exist
		if _, err := os.Stat(workingDir); os.IsNotExist(err) {
			err = os.MkdirAll(workingDir, 0755)
			if err != nil {
				return "", fmt.Errorf("failed to create working directory: %w", err)
			}
		}
	}

	// Run Go command
	cmd := exec.Command("go", args...)
	cmd.Dir = workingDir

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute the command
	cmdErr := cmd.Run()

	// Prepare the output
	output := RunGoOutput{
		Success: cmdErr == nil,
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
		Command: "go " + strings.Join(args, " "),
	}

	if cmdErr != nil {
		output.ErrorMessage = cmdErr.Error()
	}

	// Convert to JSON
	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	return string(jsonOutput), nil
}

// Helper function to be used within other tools to run Go commands
func RunGoCommand(command, path string, args []string, workingDir string) (RunGoOutput, error) {
	input := RunGoInput{
		Command:    command,
		Path:       path,
		Args:       args,
		WorkingDir: workingDir,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return RunGoOutput{}, fmt.Errorf("failed to marshal input: %w", err)
	}

	outputStr, err := RunGo(inputJSON)
	if err != nil {
		return RunGoOutput{}, err
	}

	var output RunGoOutput
	err = json.Unmarshal([]byte(outputStr), &output)
	if err != nil {
		return RunGoOutput{}, fmt.Errorf("failed to unmarshal output: %w", err)
	}

	return output, nil
}

// GetGoPackage attempts to determine the appropriate Go package name from a file path
func GetGoPackage(filePath string) (string, error) {
	// Check if path is a directory
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	// If file is provided, use its directory
	if !fileInfo.IsDir() {
		filePath = filepath.Dir(filePath)
	}

	// Absolute path for compatibility with go list
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Run go list to determine package
	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}}", absPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		// If go list fails, try to infer from go.mod
		return inferPackageFromGoMod(absPath)
	}

	packageName := strings.TrimSpace(stdout.String())
	if packageName == "" {
		return "", fmt.Errorf("could not determine package name")
	}

	return packageName, nil
}

// inferPackageFromGoMod tries to determine package name by reading go.mod
func inferPackageFromGoMod(dir string) (string, error) {
	// Start from the given directory and search upward for go.mod
	currentDir := dir
	for {
		modPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(modPath); err == nil {
			// Found go.mod, extract module name
			content, err := os.ReadFile(modPath)
			if err != nil {
				return "", fmt.Errorf("failed to read go.mod: %w", err)
			}

			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module"))
					// Remove quotes if present
					moduleName = strings.Trim(moduleName, "\"'")

					// Calculate relative path from module root
					relPath, err := filepath.Rel(currentDir, dir)
					if err == nil && relPath != "." {
						return filepath.Join(moduleName, relPath), nil
					}
					return moduleName, nil
				}
			}
			return "", fmt.Errorf("module directive not found in go.mod")
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached root directory without finding go.mod
			break
		}
		currentDir = parentDir
	}

	return "", fmt.Errorf("go.mod not found")
}
