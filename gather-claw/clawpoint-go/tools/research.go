package tools

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ResearchTool handles web research using Chawan browser
type ResearchTool struct {
	timeout time.Duration
	maxLen  int
}

// NewResearchTool creates a new research tool with optimized settings
func NewResearchTool() *ResearchTool {
	return &ResearchTool{
		timeout: 45 * time.Second,
		maxLen:  50000, // Increased from 30k for better results
	}
}

// Research performs web research via Chawan browser
func (r *ResearchTool) Research(queryOrURL string) (string, error) {
	var targetURL string

	// Check if it's already a URL
	if strings.HasPrefix(queryOrURL, "http://") || strings.HasPrefix(queryOrURL, "https://") {
		targetURL = queryOrURL
	} else {
		// Search DuckDuckGo HTML version (no JS, works great with Chawan)
		targetURL = fmt.Sprintf("https://duckduckgo.com/html/?q=%s", url.QueryEscape(queryOrURL))
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Use cha -d to dump page as text
	cmd := exec.CommandContext(ctx, "cha", "-d", targetURL)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("request timed out after %v", r.timeout)
		}
		return "", fmt.Errorf("error fetching %s: %v", targetURL, err)
	}

	content := string(output)

	// Clean up the content
	content = r.cleanContent(content)

	// Truncate if needed
	if len(content) > r.maxLen {
		content = content[:r.maxLen] + fmt.Sprintf("\n\n... truncated (%d chars total)", len(content))
	}

	if content == "" {
		return "(no output)", nil
	}

	return content, nil
}

// FetchURL fetches a specific URL with better formatting
func (r *ResearchTool) FetchURL(urlStr string) (string, error) {
	content, err := r.Research(urlStr)
	if err != nil {
		return "", err
	}
	
	// Add header for clarity
	header := fmt.Sprintf("=== Fetched: %s ===\n\n", urlStr)
	return header + content, nil
}

// Search performs a DuckDuckGo search with result parsing
func (r *ResearchTool) Search(query string) (string, error) {
	content, err := r.Research(query)
	if err != nil {
		return "", err
	}
	
	// Add header for clarity
	header := fmt.Sprintf("=== Search Results for: %s ===\n\n", query)
	return header + content, nil
}

// SearchWithOptions performs a search with advanced options
func (r *ResearchTool) SearchWithOptions(query string, opts SearchOptions) (string, error) {
	// Build advanced DuckDuckGo query
	advancedQuery := query
	
	if opts.Site != "" {
		advancedQuery = fmt.Sprintf("site:%s %s", opts.Site, query)
	}
	if opts.FileType != "" {
		advancedQuery = fmt.Sprintf("%s filetype:%s", advancedQuery, opts.FileType)
	}
	if opts.ExcludeTerms != "" {
		terms := strings.Split(opts.ExcludeTerms, ",")
		for _, term := range terms {
			advancedQuery = fmt.Sprintf("%s -%s", advancedQuery, strings.TrimSpace(term))
		}
	}
	if opts.DateRange != "" {
		// DuckDuckGo date syntax
		advancedQuery = fmt.Sprintf("%s %s", advancedQuery, opts.DateRange)
	}
	
	return r.Search(advancedQuery)
}

// SearchOptions provides advanced search parameters
type SearchOptions struct {
	Site         string // Limit to specific site
	FileType     string // File type filter (pdf, doc, etc)
	ExcludeTerms string // Terms to exclude (comma-separated)
	DateRange    string // Date range filter
}

// cleanContent removes excessive whitespace and HTML artifacts
func (r *ResearchTool) cleanContent(content string) string {
	// Remove excessive newlines
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")
	
	// Remove common HTML artifacts that might slip through
	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&lt;", "<")
	content = strings.ReplaceAll(content, "&gt;", ">")
	content = strings.ReplaceAll(content, "&quot;", "\"")
	content = strings.ReplaceAll(content, "&#39;", "'")
	
	// Trim leading/trailing whitespace
	content = strings.TrimSpace(content)
	
	return content
}

// FetchMultiple fetches multiple URLs concurrently
func (r *ResearchTool) FetchMultiple(urls []string) (map[string]string, error) {
	results := make(map[string]string)
	errors := make(map[string]error)
	
	// For now, do sequentially (TODO: could use goroutines for parallel fetching)
	for _, u := range urls {
		content, err := r.FetchURL(u)
		if err != nil {
			errors[u] = err
			results[u] = fmt.Sprintf("Error: %v", err)
		} else {
			results[u] = content
		}
	}
	
	// If all failed, return error
	if len(errors) == len(urls) {
		return nil, fmt.Errorf("all URL fetches failed")
	}
	
	return results, nil
}

// ExtractLinks extracts all links from a webpage
func (r *ResearchTool) ExtractLinks(urlStr string) ([]string, error) {
	content, err := r.FetchURL(urlStr)
	if err != nil {
		return nil, err
	}
	
	// Simple link extraction (looks for http/https URLs in text)
	linkRegex := regexp.MustCompile("https?://[^\\s<>\"{}|\\\\^`\\[\\]]+")
	links := linkRegex.FindAllString(content, -1)
	
	// Deduplicate
	seen := make(map[string]bool)
	unique := []string{}
	for _, link := range links {
		if !seen[link] {
			seen[link] = true
			unique = append(unique, link)
		}
	}
	
	return unique, nil
}

// QuickFact performs a quick search optimized for simple factual queries
func (r *ResearchTool) QuickFact(query string) (string, error) {
	// Add "what is" or similar if query seems like a fact question
	if !strings.Contains(strings.ToLower(query), "what is") &&
		!strings.Contains(strings.ToLower(query), "who is") &&
		!strings.Contains(strings.ToLower(query), "how to") {
		// Try to detect if it's a question
		if strings.HasSuffix(query, "?") || 
			strings.Contains(strings.ToLower(query), " is ") ||
			strings.Contains(strings.ToLower(query), " are ") {
			// Already looks like a question, search as-is
		} else {
			// Make it a "what is" query
			query = "what is " + query
		}
	}
	
	return r.Search(query)
}

// SetTimeout allows customizing the timeout for slow connections
func (r *ResearchTool) SetTimeout(d time.Duration) {
	r.timeout = d
}

// SetMaxLength allows customizing max content length
func (r *ResearchTool) SetMaxLength(maxLen int) {
	r.maxLen = maxLen
}
