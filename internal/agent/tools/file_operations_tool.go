package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileOpsToolDefinition defines the tool for file operations like copy, move, and rename
var FileOperationsToolDefinition = ToolDefinition{
	Name:        "file_operations",
	Description: "Perform file operations such as copying, moving, and renaming files and directories.",
	InputSchema: FileOpsToolInputSchema,
	Function:    FileOpsTool,
}

// FileOpsToolInput defines the input parameters for the file operations tool
type FileOpsToolInput struct {
	Operation   string `json:"operation" jsonschema_description:"The operation to perform: 'copy', 'move', or 'rename'."`
	Source      string `json:"source" jsonschema_description:"Source file or directory path."`
	Destination string `json:"destination" jsonschema_description:"Destination file or directory path."`
	Recursive   bool   `json:"recursive,omitempty" jsonschema_description:"Whether to recursively copy directories (only applicable for 'copy' operation)."`
	CreateDirs  bool   `json:"create_dirs,omitempty" jsonschema_description:"Whether to create parent directories if they don't exist."`
}

// FileOpsToolInputSchema is the JSON schema for the file operations tool
var FileOpsToolInputSchema = GenerateSchema[FileOpsToolInput]()

// FileOpsTool implements file operations functionality
func FileOpsTool(input json.RawMessage) (string, error) {
	fileOpsInput := FileOpsToolInput{}
	err := json.Unmarshal(input, &fileOpsInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse tool input: %w", err)
	}

	// Validate input
	if fileOpsInput.Source == "" {
		return "", fmt.Errorf("source path is required")
	}
	if fileOpsInput.Destination == "" {
		return "", fmt.Errorf("destination path is required")
	}

	// Create parent directories if requested
	if fileOpsInput.CreateDirs {
		destDir := filepath.Dir(fileOpsInput.Destination)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create parent directories: %w", err)
		}
	}

	switch fileOpsInput.Operation {
	case "copy":
		err = copyFileOrDir(fileOpsInput.Source, fileOpsInput.Destination, fileOpsInput.Recursive)
	case "move":
		err = os.Rename(fileOpsInput.Source, fileOpsInput.Destination)
	case "rename":
		err = os.Rename(fileOpsInput.Source, fileOpsInput.Destination)
	default:
		return "", fmt.Errorf("invalid operation: %s. Must be 'copy', 'move', or 'rename'", fileOpsInput.Operation)
	}

	if err != nil {
		return "", fmt.Errorf("file operation failed: %w", err)
	}

	return fmt.Sprintf("Successfully performed %s operation from '%s' to '%s'",
		fileOpsInput.Operation, fileOpsInput.Source, fileOpsInput.Destination), nil
}

// copyFileOrDir copies a file or directory from src to dst
func copyFileOrDir(src, dst string, recursive bool) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("error getting source info: %w", err)
	}

	if srcInfo.IsDir() {
		if !recursive {
			return fmt.Errorf("source is a directory but recursive flag is not set")
		}
		return copyDir(src, dst)
	}

	return copyFile(src, dst)
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("error creating destination file: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("error copying file contents: %w", err)
	}

	// Copy file permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("error getting source file info: %w", err)
	}
	return os.Chmod(dst, srcInfo.Mode())
}

// copyDir recursively copies a directory from src to dst
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("error getting source directory info: %w", err)
	}

	// Create destination directory
	err = os.MkdirAll(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("error creating destination directory: %w", err)
	}

	// Read directory entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("error reading source directory: %w", err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err = copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err = copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}
