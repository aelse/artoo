// Package tool provides tool implementations for the agent.
package tool

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

const (
	maxResponseSize = 5 * 1024 * 1024 // 5MB
	defaultWebTimeout = 30 * time.Second
	maxWebTimeout   = 2 * time.Minute
)

// WebFetchParams defines the parameters for the webfetch tool.
type WebFetchParams struct {
	URL     string  `json:"url"`               // Required: URL to fetch
	Format  string  `json:"format"`            // Required: text, markdown, or html
	Timeout *int    `json:"timeout,omitempty"` // Optional: timeout in seconds
}

// Ensure WebFetchTool implements TypedTool[WebFetchParams]
var _ TypedTool[WebFetchParams] = (*WebFetchTool)(nil)

type WebFetchTool struct{}

// Call implements TypedTool.Call with strongly-typed parameters
func (t *WebFetchTool) Call(params WebFetchParams) (string, error) {
	// Validate URL
	if !strings.HasPrefix(params.URL, "http://") && !strings.HasPrefix(params.URL, "https://") {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}

	// Validate format
	if params.Format != "text" && params.Format != "markdown" && params.Format != "html" {
		return "", fmt.Errorf("format must be one of: text, markdown, html")
	}

	// Determine timeout
	timeout := defaultWebTimeout
	if params.Timeout != nil {
		requestedTimeout := time.Duration(*params.Timeout) * time.Second
		if requestedTimeout > maxWebTimeout {
			timeout = maxWebTimeout
		} else {
			timeout = requestedTimeout
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Build Accept header based on format
	acceptHeader := "*/*"
	switch params.Format {
	case "markdown":
		acceptHeader = "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1"
	case "text":
		acceptHeader = "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1"
	case "html":
		acceptHeader = "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, text/markdown;q=0.7, */*;q=0.1"
	}

	// Create request
	req, err := http.NewRequest("GET", params.URL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request failed with status code: %d", resp.StatusCode)
	}

	// Check content length
	if resp.ContentLength > maxResponseSize {
		return "", fmt.Errorf("response too large (exceeds 5MB limit)")
	}

	// Read body with size limit
	limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if len(body) > maxResponseSize {
		return "", fmt.Errorf("response too large (exceeds 5MB limit)")
	}

	content := string(body)
	contentType := resp.Header.Get("Content-Type")

	// Process based on format and content type
	switch params.Format {
	case "markdown":
		if strings.Contains(contentType, "text/html") {
			// For HTML content, we'd ideally convert to markdown
			// For now, just return HTML with a note
			return "Note: HTML to Markdown conversion not yet implemented\n\n" + content, nil
		}
		return content, nil

	case "text":
		if strings.Contains(contentType, "text/html") {
			// For HTML content, we'd ideally strip tags
			// For now, just return HTML with a note
			return "Note: HTML text extraction not yet implemented\n\n" + content, nil
		}
		return content, nil

	case "html":
		return content, nil

	default:
		return content, nil
	}
}

func (t *WebFetchTool) Param() anthropic.ToolParam {
	return anthropic.ToolParam{
		Name: "webfetch",
		Description: anthropic.String(`- Fetches content from a specified URL
- Takes a URL and a prompt as input
- Fetches the URL content, converts HTML to markdown
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
  - IMPORTANT: if another tool is present that offers better web fetching capabilities, is more targeted to the task, or has fewer restrictions, prefer using that tool instead of this one.
  - The URL must be a fully-formed valid URL
  - HTTP URLs will be automatically upgraded to HTTPS
  - The prompt should describe what information you want to extract from the page
  - This tool is read-only and does not modify any files
  - Results may be summarized if the content is very large
  - Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL`),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "The URL to fetch content from",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "The format to return the content in (text, markdown, or html)",
					"enum":        []string{"text", "markdown", "html"},
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Optional timeout in seconds (max 120)",
				},
			},
			Required: []string{"url", "format"},
		},
	}
}
