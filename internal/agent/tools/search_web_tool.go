package tools

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// SearchWebToolDefinition defines the web search tool, currently implemented using Brave's Search API
var SearchWebToolDefinition = ToolDefinition{
	Name:        "search_web",
	Description: "Search the web using Brave Search API. Requires BRAVE_API_KEY environment variable. Returns search results as a JSON string with title, URL, and description.",
	InputSchema: WebSearchInputSchema,
	Function:    SearchWeb,
}

// WebSearchInput defines the input parameters for the search_web tool
type WebSearchInput struct {
	Query      string `json:"query" jsonschema_description:"Search query."`
	NumResults int    `json:"num_results,omitempty" jsonschema_description:"Optional number of results to return. Default is 5, maximum is 20."`
}

// WebSearchInputSchema is the JSON schema for the search_web tool
var WebSearchInputSchema = GenerateSchema[WebSearchInput]()

// SearchResult represents a single search result
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
}

// SearchResponse represents the full response from a search
type SearchResponse struct {
	Results      []SearchResult `json:"results"`
	TotalResults int            `json:"total_results"`
	Query        string         `json:"query"`
	Error        string         `json:"error,omitempty"`
}

// SearchWeb implements the search_web tool functionality using Brave Search API
func SearchWeb(input json.RawMessage) (string, error) {
	// Parse input
	searchInput := WebSearchInput{}
	err := json.Unmarshal(input, &searchInput)
	if err != nil {
		return "", fmt.Errorf("failed to parse search input: %v", err)
	}

	// Validate input
	if searchInput.Query == "" {
		return "", errors.New("search query cannot be empty")
	}

	// Set default number of results if not specified or if it exceeds maximum
	if searchInput.NumResults <= 0 {
		searchInput.NumResults = 5
	}
	if searchInput.NumResults > 20 {
		searchInput.NumResults = 20
	}

	// Get API key from environment variable
	apiKey := os.Getenv("BRAVE_API_KEY")
	if apiKey == "" {
		return createErrorResponse(searchInput.Query, "BRAVE_API_KEY environment variable not set"), nil
	}

	// Build request URL for Brave Search API
	baseURL := "https://api.search.brave.com/res/v1/web/search"
	params := url.Values{}
	params.Add("q", searchInput.Query)
	// Add additional parameters that might be required
	params.Add("count", fmt.Sprintf("%d", searchInput.NumResults))
	params.Add("country", "us") // Default to US results

	requestURL := baseURL + "?" + params.Encode()

	// Create a new request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Failed to create request: %v", err)), nil
	}

	// Add required headers
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Accept-Encoding", "gzip") // We explicitly request gzip encoding
	req.Header.Add("X-Subscription-Token", apiKey)

	// Create HTTP client and send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Search request failed: %v", err)), nil
	}
	defer resp.Body.Close()

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Search API returned error code %d: %s", resp.StatusCode, string(bodyBytes))), nil
	}

	// Handle compressed responses
	var reader io.ReadCloser
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return createErrorResponse(searchInput.Query, fmt.Sprintf("Failed to decompress gzipped response: %v", err)), nil
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	// Read response body
	body, err := io.ReadAll(reader)
	if err != nil {
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Failed to read search response: %v", err)), nil
	}

	// Prepare our response
	searchResponse := SearchResponse{
		Query:   searchInput.Query,
		Results: []SearchResult{},
	}

	// Parse the response
	var braveResponse map[string]interface{}
	err = json.Unmarshal(body, &braveResponse)
	if err != nil {
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Failed to parse search results: %v. Response: %s", err, truncateString(string(body), 100))), nil
	}

	// Extract web results if available
	if webSection, ok := braveResponse["web"].(map[string]interface{}); ok {
		if results, ok := webSection["results"].([]interface{}); ok {
			for _, result := range results {
				if len(searchResponse.Results) >= searchInput.NumResults {
					break
				}

				if resultMap, ok := result.(map[string]interface{}); ok {
					title, _ := resultMap["title"].(string)
					url, _ := resultMap["url"].(string)
					description, _ := resultMap["description"].(string)

					// Only add if we have at least title and URL
					if title != "" && url != "" {
						searchResponse.Results = append(searchResponse.Results, SearchResult{
							Title:       title,
							URL:         url,
							Description: description,
						})
					}
				}
			}
		}
	}

	// Extract news results if needed
	if len(searchResponse.Results) < searchInput.NumResults {
		if newsSection, ok := braveResponse["news"].(map[string]interface{}); ok {
			if results, ok := newsSection["results"].([]interface{}); ok {
				for _, result := range results {
					if len(searchResponse.Results) >= searchInput.NumResults {
						break
					}

					if resultMap, ok := result.(map[string]interface{}); ok {
						title, _ := resultMap["title"].(string)
						url, _ := resultMap["url"].(string)
						description, _ := resultMap["description"].(string)
						source, _ := resultMap["source"].(string)

						// Only add if we have at least title and URL
						if title != "" && url != "" {
							searchResponse.Results = append(searchResponse.Results, SearchResult{
								Title:       title,
								URL:         url,
								Description: description,
								Source:      source,
							})
						}
					}
				}
			}
		}
	}

	// If we have no results, try a more generic approach
	if len(searchResponse.Results) == 0 {
		// Try to find any result arrays in the response
		for _, sectionData := range braveResponse {
			if len(searchResponse.Results) >= searchInput.NumResults {
				break
			}

			if sectionMap, ok := sectionData.(map[string]interface{}); ok {
				if results, ok := sectionMap["results"].([]interface{}); ok {
					for _, result := range results {
						if len(searchResponse.Results) >= searchInput.NumResults {
							break
						}

						if resultMap, ok := result.(map[string]interface{}); ok {
							// Try to extract common fields
							title := extractString(resultMap, "title")
							url := extractString(resultMap, "url")
							description := extractString(resultMap, "description")
							if description == "" {
								description = extractString(resultMap, "snippet")
							}
							source := extractString(resultMap, "source")

							// Only add if we have at least title and URL
							if title != "" && url != "" {
								searchResponse.Results = append(searchResponse.Results, SearchResult{
									Title:       title,
									URL:         url,
									Description: description,
									Source:      source,
								})
							}
						}
					}
				}
			}
		}
	}

	// Set total results
	searchResponse.TotalResults = len(searchResponse.Results)

	// If we still have no results, add an error
	if searchResponse.TotalResults == 0 {
		searchResponse.Error = "No results found in the API response"
	}

	// Convert response to JSON
	resultJSON, err := json.Marshal(searchResponse)
	if err != nil {
		return createErrorResponse(searchInput.Query, fmt.Sprintf("Failed to format results: %v", err)), nil
	}

	return string(resultJSON), nil
}

// Helper function to create error responses
func createErrorResponse(query string, errorMsg string) string {
	errorResponse := SearchResponse{
		Query:        query,
		Results:      []SearchResult{},
		TotalResults: 0,
		Error:        errorMsg,
	}
	resultJSON, _ := json.Marshal(errorResponse)
	return string(resultJSON)
}

// Helper function to extract string values from a map
func extractString(data map[string]interface{}, key string) string {
	if value, ok := data[key].(string); ok {
		return value
	}
	return ""
}

// Helper function to truncate strings for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
