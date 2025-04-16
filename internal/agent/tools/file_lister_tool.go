package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// FileListerDefinition defines the list_files tool
var FileListerToolDefinition = ToolDefinition{
	Name:        "list_files",
	Description: "List files and directories at a given path. If no path is provided, lists files in the current directory.",
	InputSchema: ListDirectoryContentsInputSchema,
	Function:    ListDirectoryContents,
}

// ListDirectoryContentsInput defines the input parameters for the list_files tool
type ListDirectoryContentsInput struct {
	Path string `json:"path,omitempty" jsonschema_description:"Optional relative path to list files from. Defaults to current directory if not provided."`
}

// ListDirectoryContentsInputSchema is the JSON schema for the list_files tool
var ListDirectoryContentsInputSchema = GenerateSchema[ListDirectoryContentsInput]()

// ListDirectoryContents implements the list_files tool functionality
func ListDirectoryContents(input json.RawMessage) (string, error) {
	listFilesInput := ListDirectoryContentsInput{}
	err := json.Unmarshal(input, &listFilesInput)
	if err != nil {
		return "", err
	}

	dir := "."
	if listFilesInput.Path != "" {
		dir = listFilesInput.Path
	}

	var files []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if relPath != "." {
			if info.IsDir() {
				files = append(files, relPath+"/")
			} else {
				files = append(files, relPath)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	result, err := json.Marshal(files)
	if err != nil {
		return "", err
	}

	return string(result), nil
}
