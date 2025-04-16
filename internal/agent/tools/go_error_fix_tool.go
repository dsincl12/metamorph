package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// FixGoErrorsDefinition defines the fix_go_errors tool
var GoErrorFixToolDefinition = ToolDefinition{
	Name: "fix_go_errors",
	Description: `Analyze Go compiler errors and suggest fixes.
Given the error output from a Go command, this tool will analyze common errors
and suggest how to fix them. It can help identify and fix issues like:
- Import cycles
- Undefined variables or functions
- Missing imports
- Type mismatches
- Syntax errors

The tool returns a structured analysis with suggested fixes that can be applied.
`,
	InputSchema: FixGoErrorsInputSchema,
	Function:    FixGoErrors,
}

// FixGoErrorsInput defines the input parameters for the fix_go_errors tool
type FixGoErrorsInput struct {
	ErrorOutput string `json:"error_output" jsonschema_description:"The stderr output from a Go command containing error messages"`
}

// FixGoErrorsInputSchema is the JSON schema for the fix_go_errors tool
var FixGoErrorsInputSchema = GenerateSchema[FixGoErrorsInput]()

// GoError represents a parsed Go error
type GoError struct {
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Column      int    `json:"column,omitempty"`
	Message     string `json:"message"`
	ErrorType   string `json:"error_type"`
	Suggestion  string `json:"suggestion"`
	CodeSnippet string `json:"code_snippet,omitempty"`
}

// FixGoErrorsOutput represents the structured output of the fix_go_errors tool
type FixGoErrorsOutput struct {
	TotalErrors    int       `json:"total_errors"`
	ParsedErrors   []GoError `json:"parsed_errors"`
	OverallSummary string    `json:"overall_summary"`
}

// FixGoErrors implements the fix_go_errors tool functionality
func FixGoErrors(input json.RawMessage) (string, error) {
	fixGoErrorsInput := FixGoErrorsInput{}
	err := json.Unmarshal(input, &fixGoErrorsInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse input: %w", err)
	}

	if fixGoErrorsInput.ErrorOutput == "" {
		return "", fmt.Errorf("error_output cannot be empty")
	}

	// Parse the errors
	parsedErrors := parseGoErrors(fixGoErrorsInput.ErrorOutput)

	// Generate overall summary
	summary := generateErrorSummary(parsedErrors)

	// Prepare output
	output := FixGoErrorsOutput{
		TotalErrors:    len(parsedErrors),
		ParsedErrors:   parsedErrors,
		OverallSummary: summary,
	}

	// Convert to JSON
	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	return string(jsonOutput), nil
}

// parseGoErrors extracts structured error information from Go error output
func parseGoErrors(errorOutput string) []GoError {
	var errors []GoError

	// Split error output into lines
	lines := strings.Split(errorOutput, "\n")

	// Common error patterns
	fileLineColPattern := regexp.MustCompile(`(.+):(\d+):(\d+):\s+(.+)`)
	fileLinePattern := regexp.MustCompile(`(.+):(\d+):\s+(.+)`)
	packageErrorPattern := regexp.MustCompile(`package\s+(.+):\s+(.+)`)
	importCyclePattern := regexp.MustCompile(`import cycle not allowed`)
	undefinedPattern := regexp.MustCompile(`undefined:\s+([^\s]+)`)
	missingImportPattern := regexp.MustCompile(`(?i)could not import ([^\s]+)`)
	unexpectedPattern := regexp.MustCompile(`unexpected (.+)`)
	typeErrorPattern := regexp.MustCompile(`cannot use (.+) \((?:type |variable of type )?(.+)\) as (.+)`)

	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		var goError GoError

		// Try to match file:line:col pattern
		if matches := fileLineColPattern.FindStringSubmatch(line); matches != nil {
			goError.File = matches[1]
			if _, err := fmt.Sscanf(matches[2], "%d", &goError.Line); err != nil {
				goError.Line = 0
			}
			if _, err := fmt.Sscanf(matches[3], "%d", &goError.Column); err != nil {
				goError.Column = 0
			}
			goError.Message = matches[4]

			// Look ahead to get more context
			if i+1 < len(lines) {
				goError.CodeSnippet = strings.TrimSpace(lines[i+1])
			}
		} else if matches := fileLinePattern.FindStringSubmatch(line); matches != nil {
			// Try to match file:line pattern
			goError.File = matches[1]
			if _, err := fmt.Sscanf(matches[2], "%d", &goError.Line); err != nil {
				goError.Line = 0
			}
			goError.Message = matches[3]

			// Look ahead to get more context
			if i+1 < len(lines) {
				goError.CodeSnippet = strings.TrimSpace(lines[i+1])
			}
		} else if matches := packageErrorPattern.FindStringSubmatch(line); matches != nil {
			// Try to match package error pattern
			goError.File = matches[1]
			goError.Message = matches[2]
		} else if strings.Contains(line, "error:") {
			// Generic error message
			parts := strings.SplitN(line, "error:", 2)
			if len(parts) > 1 {
				goError.Message = strings.TrimSpace(parts[1])
			} else {
				goError.Message = line
			}
		} else {
			// If we couldn't match any specific pattern, just use the line as a message
			goError.Message = line
		}

		// Determine error type and suggestion
		if goError.Message != "" {
			if importCyclePattern.MatchString(goError.Message) {
				goError.ErrorType = "Import Cycle"
				goError.Suggestion = "Restructure your packages to avoid circular dependencies. Consider creating a new package to break the cycle or using interfaces to reduce direct dependencies."
			} else if matches := undefinedPattern.FindStringSubmatch(goError.Message); matches != nil {
				goError.ErrorType = "Undefined Symbol"
				goError.Suggestion = fmt.Sprintf("Check the spelling of '%s'. Make sure it's defined and in scope. You might need to add an import, or the symbol might be unexported (lowercase).", matches[1])
			} else if matches := missingImportPattern.FindStringSubmatch(goError.Message); matches != nil {
				goError.ErrorType = "Missing Import"
				goError.Suggestion = fmt.Sprintf("Add the import for '%s' to your file, or check that the package name is correct.", matches[1])
			} else if matches := unexpectedPattern.FindStringSubmatch(goError.Message); matches != nil {
				goError.ErrorType = "Syntax Error"
				goError.Suggestion = fmt.Sprintf("Fix the syntax error. Unexpected '%s' indicates a problem with your code structure or a missing element before this point.", matches[1])
			} else if matches := typeErrorPattern.FindStringSubmatch(goError.Message); matches != nil {
				goError.ErrorType = "Type Error"
				goError.Suggestion = fmt.Sprintf("Type mismatch: cannot use '%s' (type %s) as %s. Make sure you're using the correct types or add appropriate type conversions.", matches[1], matches[2], matches[3])
			} else if strings.Contains(goError.Message, "declared and not used") {
				goError.ErrorType = "Unused Declaration"
				goError.Suggestion = "Remove the unused variable or import, or use it in your code. You can prefix the variable name with _ to explicitly ignore it."
			} else if strings.Contains(goError.Message, "no required module") {
				goError.ErrorType = "Module Error"
				goError.Suggestion = "Run 'go mod tidy' to add missing modules or fix your import statements to use the correct module paths."
			} else if strings.Contains(goError.Message, "multiple-value") && strings.Contains(goError.Message, "in single-value context") {
				goError.ErrorType = "Multiple Return Values"
				goError.Suggestion = "Function returns multiple values, but you're not handling all of them. Use multiple variable assignment: v1, v2 := function()"
			} else if strings.Contains(goError.Message, "missing return") {
				goError.ErrorType = "Missing Return"
				goError.Suggestion = "Function must return a value for all code paths. Add a return statement at the end of the function or in any missing branches."
			} else {
				goError.ErrorType = "General Error"
				goError.Suggestion = "Review the error message carefully and check the relevant code section."
			}

			errors = append(errors, goError)
		}

		i++
	}

	return errors
}

// generateErrorSummary creates an overall summary of the errors and suggestions
func generateErrorSummary(errors []GoError) string {
	if len(errors) == 0 {
		return "No errors were found in the provided output."
	}

	// Count error types
	errorTypeCounts := make(map[string]int)
	for _, err := range errors {
		errorTypeCounts[err.ErrorType]++
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Found %d error(s) of %d different type(s):\n", len(errors), len(errorTypeCounts)))

	// List error types and counts
	for errType, count := range errorTypeCounts {
		summary.WriteString(fmt.Sprintf("- %s: %d occurrence(s)\n", errType, count))
	}

	// Add general recommendation
	summary.WriteString("\nRecommended action plan:\n")

	// Add specific recommendations based on error types
	if count, found := errorTypeCounts["Import Cycle"]; found && count > 0 {
		summary.WriteString("1. Resolve import cycles by restructuring package dependencies\n")
	}
	if count, found := errorTypeCounts["Undefined Symbol"]; found && count > 0 {
		summary.WriteString("2. Fix undefined symbols by adding proper imports or correcting variable/function names\n")
	}
	if count, found := errorTypeCounts["Type Error"]; found && count > 0 {
		summary.WriteString("3. Address type mismatches with proper type conversions or correcting variable types\n")
	}
	if count, found := errorTypeCounts["Syntax Error"]; found && count > 0 {
		summary.WriteString("4. Correct syntax errors by fixing code structure and ensuring proper formatting\n")
	}
	if count, found := errorTypeCounts["Module Error"]; found && count > 0 {
		summary.WriteString("5. Run 'go mod tidy' to fix module-related issues\n")
	}
	if count, found := errorTypeCounts["Unused Declaration"]; found && count > 0 {
		summary.WriteString("6. Remove or use declared variables and imports\n")
	}

	return summary.String()
}
