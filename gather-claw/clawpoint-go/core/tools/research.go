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
		maxLen:  50000,
	}
}

// Research performs web research via Chawan browser
func (r *ResearchTool) Research(queryOrURL string) (string, error) {
	var targetURL string

	if strings.HasPrefix(queryOrURL, "http://") || strings.HasPrefix(queryOrURL, "https://") {
		targetURL = queryOrURL
	} else {
		targetURL = fmt.Sprintf("https://duckduckgo.com/html/?q=%s", url.QueryEscape(queryOrURL))
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cha", "-d", targetURL)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("request timed out after %v", r.timeout)
		}
		return "", fmt.Errorf("error fetching %s: %v", targetURL, err)
	}

	content := string(output)
	content = r.cleanContent(content)

	if len(content) > r.maxLen {
		content = content[:r.maxLen] + fmt.Sprintf("\n\n... truncated (%d chars total)", len(content))
	}

	if content == "" {
		return "(no output)", nil
	}

	return content, nil
}

// cleanContent removes excessive whitespace and HTML artifacts
func (r *ResearchTool) cleanContent(content string) string {
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&lt;", "<")
	content = strings.ReplaceAll(content, "&gt;", ">")
	content = strings.ReplaceAll(content, "&quot;", "\"")
	content = strings.ReplaceAll(content, "&#39;", "'")

	content = strings.TrimSpace(content)

	return content
}
