package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GitToolDefinition defines the git tool for common Git operations
var GitOperationsToolDefinition = ToolDefinition{
	Name:        "git",
	Description: "Execute common Git operations such as checking status, staging files, committing changes, pulling, pushing, viewing logs, creating branches, and more.",
	InputSchema: GitToolInputSchema,
	Function:    GitTool,
}

// GitToolInput defines the input parameters for the git tool
type GitToolInput struct {
	Command    string   `json:"command" jsonschema_description:"The Git command to execute (status, add, commit, push, pull, log, branch, checkout, etc.)."`
	Args       []string `json:"args,omitempty" jsonschema_description:"Optional additional arguments for the Git command."`
	Message    string   `json:"message,omitempty" jsonschema_description:"Commit message when using the 'commit' command."`
	Files      []string `json:"files,omitempty" jsonschema_description:"Specific files to operate on (for add, checkout, etc.). Use ['.'] for all files."`
	BranchName string   `json:"branch_name,omitempty" jsonschema_description:"Branch name when using branch-related commands."`
}

// GitToolInputSchema is the JSON schema for the git tool
var GitToolInputSchema = GenerateSchema[GitToolInput]()

// GitTool implements Git operations functionality
func GitTool(input json.RawMessage) (string, error) {
	gitInput := GitToolInput{}
	err := json.Unmarshal(input, &gitInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse tool input: %w", err)
	}

	if gitInput.Command == "" {
		return "", fmt.Errorf("Git command is required")
	}

	var cmd *exec.Cmd

	switch strings.ToLower(gitInput.Command) {
	case "status":
		cmd = exec.Command("git", "status")

	case "add":
		if len(gitInput.Files) == 0 {
			// Default to all files if none specified
			cmd = exec.Command("git", "add", ".")
		} else {
			args := append([]string{"add"}, gitInput.Files...)
			cmd = exec.Command("git", args...)
		}

	case "commit":
		if gitInput.Message == "" {
			return "", fmt.Errorf("commit message is required for 'commit' command")
		}
		cmd = exec.Command("git", "commit", "-m", gitInput.Message)

	case "push":
		args := []string{"push"}
		if gitInput.BranchName != "" {
			args = append(args, "origin", gitInput.BranchName)
		}
		if len(gitInput.Args) > 0 {
			args = append(args, gitInput.Args...)
		}
		cmd = exec.Command("git", args...)

	case "pull":
		args := []string{"pull"}
		if len(gitInput.Args) > 0 {
			args = append(args, gitInput.Args...)
		}
		cmd = exec.Command("git", args...)

	case "log":
		args := []string{"log"}
		if len(gitInput.Args) > 0 {
			args = append(args, gitInput.Args...)
		} else {
			// Default to a nicely formatted concise log
			args = append(args, "--oneline", "--graph", "--decorate", "-n", "10")
		}
		cmd = exec.Command("git", args...)

	case "branch":
		args := []string{"branch"}
		if gitInput.BranchName != "" {
			args = append(args, gitInput.BranchName)
		}
		if len(gitInput.Args) > 0 {
			args = append(args, gitInput.Args...)
		}
		cmd = exec.Command("git", args...)

	case "checkout":
		if gitInput.BranchName == "" && len(gitInput.Files) == 0 && len(gitInput.Args) == 0 {
			return "", fmt.Errorf("either branch_name, files, or args are required for 'checkout' command")
		}

		args := []string{"checkout"}
		if gitInput.BranchName != "" {
			args = append(args, gitInput.BranchName)
		}
		if len(gitInput.Files) > 0 {
			args = append(args, gitInput.Files...)
		}
		if len(gitInput.Args) > 0 {
			args = append(args, gitInput.Args...)
		}
		cmd = exec.Command("git", args...)

	case "stage_and_commit":
		// Convenience command to stage all and commit in one step
		if gitInput.Message == "" {
			return "", fmt.Errorf("commit message is required for 'stage_and_commit' command")
		}

		// First stage all changes
		stageCmd := exec.Command("git", "add", ".")
		stageOutput, err := stageCmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("failed to stage changes: %s, %w", string(stageOutput), err)
		}

		// Then commit
		cmd = exec.Command("git", "commit", "-m", gitInput.Message)

	default:
		// For any other Git commands, pass them through
		args := append([]string{gitInput.Command}, gitInput.Args...)
		cmd = exec.Command("git", args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git command failed: %s, %w", string(output), err)
	}

	return string(output), nil
}
