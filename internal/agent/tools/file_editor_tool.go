package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// FileEditorDefinition defines the improved edit_file tool
var FileEditorToolDefinition = ToolDefinition{
	Name: "file_editor",
	Description: `Make sophisticated edits to a text file.
Multiple edit modes available:
1. 'replace': Replace 'old_str' with 'new_str' in the file (requires exact match)
2. 'regex_replace': Replace text matching the regex in 'pattern' with 'new_str'
3. 'create': Create a new file with 'content' (creates parent directories if needed)
4. 'append': Append 'content' to the end of the file
5. 'prepend': Prepend 'content' to the beginning of the file
6. 'insert_at_line': Insert 'content' at line number specified by 'line_number'

If the file doesn't exist and mode is not 'create', it will be created first.`,
	InputSchema: FileEditorInputSchema,
	Function:    EditFileContent,
}

// FileEditorInput defines the enhanced input parameters for the edit_file tool
type FileEditorInput struct {
	Path       string `json:"path" jsonschema_description:"The path to the file"`
	Mode       string `json:"mode" jsonschema_description:"Edit mode: 'replace', 'regex_replace', 'create', 'append', 'prepend', or 'insert_at_line'"`
	OldStr     string `json:"old_str,omitempty" jsonschema_description:"Text to search for when using 'replace' mode - must match exactly"`
	NewStr     string `json:"new_str,omitempty" jsonschema_description:"Text to replace old_str with in 'replace' or 'regex_replace' modes"`
	Pattern    string `json:"pattern,omitempty" jsonschema_description:"Regular expression pattern for 'regex_replace' mode"`
	Content    string `json:"content,omitempty" jsonschema_description:"Content to write in 'create', 'append', 'prepend', or 'insert_at_line' modes"`
	LineNumber int    `json:"line_number,omitempty" jsonschema_description:"Line number for 'insert_at_line' mode (1-based indexing)"`
	Limit      int    `json:"limit,omitempty" jsonschema_description:"Maximum number of replacements to make (0 means replace all occurrences)"`
}

// FileEditorInputSchema is the JSON schema for the edit_file tool
var FileEditorInputSchema = GenerateSchema[FileEditorInput]()

// EditFileContent implements the enhanced edit_file tool functionality
func EditFileContent(input json.RawMessage) (string, error) {
	editFileInput := FileEditorInput{}
	err := json.Unmarshal(input, &editFileInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	// Validate basic inputs
	if editFileInput.Path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Process based on mode
	switch editFileInput.Mode {
	case "create":
		if editFileInput.Content == "" {
			return "", fmt.Errorf("cannot create an empty file, content is required")
		}
		return createFile(editFileInput.Path, editFileInput.Content)
	case "replace":
		return replaceInFile(editFileInput.Path, editFileInput.OldStr, editFileInput.NewStr, editFileInput.Limit)
	case "regex_replace":
		return regexReplaceInFile(editFileInput.Path, editFileInput.Pattern, editFileInput.NewStr, editFileInput.Limit)
	case "append":
		return appendToFile(editFileInput.Path, editFileInput.Content)
	case "prepend":
		return prependToFile(editFileInput.Path, editFileInput.Content)
	case "insert_at_line":
		return insertAtLine(editFileInput.Path, editFileInput.Content, editFileInput.LineNumber)
	default:
		return "", fmt.Errorf("invalid mode: %s", editFileInput.Mode)
	}
}

// createFile creates a new file with the given content, creating parent directories if needed
func createFile(filePath, content string) (string, error) {
	// Check if file already exists
	if _, err := os.Stat(filePath); err == nil {
		// File exists, return a message instead of silently overwriting
		return fmt.Sprintf("File %s already exists. Use append, prepend, or replace modes to modify it.", filePath), nil
	}

	dir := path.Dir(filePath)
	if dir != "." {
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}
	}

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	return fmt.Sprintf("Successfully created file %s", filePath), nil
}

// ensureFileExists creates an empty file if it doesn't exist
func ensureFileExists(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		dir := path.Dir(filePath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		}
		return os.WriteFile(filePath, []byte{}, 0644)
	}
	return nil
}

// replaceInFile replaces oldStr with newStr in the file at filePath
func replaceInFile(filePath, oldStr, newStr string, limit int) (string, error) {
	if oldStr == "" {
		return "", fmt.Errorf("old_str cannot be empty")
	}

	if oldStr == newStr {
		return "No changes needed - old_str and new_str are identical", nil
	}

	// Create file if it doesn't exist
	if err := ensureFileExists(filePath); err != nil {
		return "", err
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	fileContent := string(content)

	// Perform replacements
	count := 0
	newContent := ""

	// If limit is set, replace only up to limit occurrences
	if limit > 0 {
		remaining := fileContent
		parts := []string{}

		for count < limit {
			i := strings.Index(remaining, oldStr)
			if i == -1 {
				break
			}

			parts = append(parts, remaining[:i], newStr)
			remaining = remaining[i+len(oldStr):]
			count++
		}

		if count > 0 {
			parts = append(parts, remaining)
			newContent = strings.Join(parts, "")
		} else {
			newContent = fileContent
		}
	} else {
		// Replace all occurrences
		newContent = strings.ReplaceAll(fileContent, oldStr, newStr)
		count = strings.Count(fileContent, oldStr)
	}

	// Check if any replacements were made
	if fileContent == newContent {
		return "", fmt.Errorf("old_str not found in file")
	}

	// Write the new content
	err = os.WriteFile(filePath, []byte(newContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", count, filePath), nil
}

// regexReplaceInFile replaces text matching pattern with newStr in the file at filePath
func regexReplaceInFile(filePath, pattern, newStr string, limit int) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern cannot be empty")
	}

	// Create file if it doesn't exist
	if err := ensureFileExists(filePath); err != nil {
		return "", err
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Compile regex
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex pattern: %w", err)
	}

	fileContent := string(content)
	var newContent string
	count := 0

	// Handle limited replacements
	if limit > 0 {
		newContent = regex.ReplaceAllStringFunc(fileContent, func(match string) string {
			if count < limit {
				count++
				return regex.ReplaceAllString(match, newStr)
			}
			return match
		})
	} else {
		// Replace all matches and count them
		matches := regex.FindAllString(fileContent, -1)
		count = len(matches)
		newContent = regex.ReplaceAllString(fileContent, newStr)
	}

	// Check if any replacements were made
	if fileContent == newContent {
		return "", fmt.Errorf("pattern not matched in file")
	}

	// Write the new content
	err = os.WriteFile(filePath, []byte(newContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully replaced %d occurrence(s) in %s", count, filePath), nil
}

// appendToFile appends content to the end of the file
func appendToFile(filePath, content string) (string, error) {
	// Create file if it doesn't exist
	if err := ensureFileExists(filePath); err != nil {
		return "", err
	}

	// Open file for appending
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to open file for appending: %w", err)
	}
	defer file.Close()

	// Write content
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("failed to append to file: %w", err)
	}

	return fmt.Sprintf("Successfully appended content to %s", filePath), nil
}

// prependToFile prepends content to the beginning of the file
func prependToFile(filePath, content string) (string, error) {
	// Create file if it doesn't exist
	if err := ensureFileExists(filePath); err != nil {
		return "", err
	}

	// Read existing content
	existingContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Combine content
	newContent := content + string(existingContent)

	// Write back to file
	err = os.WriteFile(filePath, []byte(newContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully prepended content to %s", filePath), nil
}

// insertAtLine inserts content at the specified line number
func insertAtLine(filePath, content string, lineNumber int) (string, error) {
	if lineNumber < 1 {
		return "", fmt.Errorf("line number must be at least 1")
	}

	// Create file if it doesn't exist
	if err := ensureFileExists(filePath); err != nil {
		return "", err
	}

	// Read existing content
	existingContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Split into lines
	lines := strings.Split(string(existingContent), "\n")

	// Check if line number is valid
	if lineNumber > len(lines)+1 {
		return "", fmt.Errorf("line number %d exceeds file length (%d lines)", lineNumber, len(lines))
	}

	// Insert content at specified line
	newLines := make([]string, 0, len(lines)+1)
	if lineNumber == 1 {
		// Insert at the beginning
		newLines = append(newLines, content)
		newLines = append(newLines, lines...)
	} else if lineNumber > len(lines) {
		// Insert at the end
		newLines = append(lines, content)
	} else {
		// Insert in the middle
		newLines = append(newLines, lines[:lineNumber-1]...)
		newLines = append(newLines, content)
		newLines = append(newLines, lines[lineNumber-1:]...)
	}

	// Join lines and write back to file
	newContent := strings.Join(newLines, "\n")
	err = os.WriteFile(filePath, []byte(newContent), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully inserted content at line %d in %s", lineNumber, filePath), nil
}
