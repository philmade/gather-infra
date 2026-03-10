package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// ResearchTool handles web research via native HTTP + HTML text extraction
type ResearchTool struct {
	timeout time.Duration
	maxLen  int
	client  *http.Client
}

// NewResearchTool creates a new research tool with optimized settings
func NewResearchTool() *ResearchTool {
	return &ResearchTool{
		timeout: 45 * time.Second,
		maxLen:  50000,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Research performs web research: searches DuckDuckGo for plain queries,
// or fetches and extracts text from direct URLs.
func (r *ResearchTool) Research(queryOrURL string) (string, error) {
	var targetURL string

	if strings.HasPrefix(queryOrURL, "http://") || strings.HasPrefix(queryOrURL, "https://") {
		targetURL = queryOrURL
	} else {
		targetURL = fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(queryOrURL))
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request for %s: %v", targetURL, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ClayAgent/1.0)")

	resp, err := r.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("request timed out after %v", r.timeout)
		}
		return "", fmt.Errorf("error fetching %s: %v", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, targetURL)
	}

	// Limit read to 2MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", fmt.Errorf("error reading response from %s: %v", targetURL, err)
	}

	content := extractText(string(body))
	content = r.cleanContent(content)

	if len(content) > r.maxLen {
		content = content[:r.maxLen] + fmt.Sprintf("\n\n... truncated (%d chars total)", len(content))
	}

	if content == "" {
		return "(no content extracted)", nil
	}

	return content, nil
}

// extractText parses HTML and extracts visible text content,
// skipping script, style, and other non-visible elements.
func extractText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		// If HTML parsing fails, do basic tag stripping
		return stripTags(htmlContent)
	}

	var sb strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "svg", "head":
				return
			case "br", "p", "div", "h1", "h2", "h3", "h4", "h5", "h6",
				"li", "tr", "blockquote", "pre", "hr":
				sb.WriteString("\n")
			}
		}

		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteString(" ")
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}

		// Add newline after block elements
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6",
				"li", "tr", "blockquote", "pre":
				sb.WriteString("\n")
			}
		}
	}
	extract(doc)
	return sb.String()
}

// stripTags is a fallback for when HTML parsing fails
func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(s, "")
}

// cleanContent removes excessive whitespace and HTML entities
func (r *ResearchTool) cleanContent(content string) string {
	re := regexp.MustCompile(`\n{3,}`)
	content = re.ReplaceAllString(content, "\n\n")

	// Collapse runs of spaces
	spaceRe := regexp.MustCompile(`[ \t]{3,}`)
	content = spaceRe.ReplaceAllString(content, "  ")

	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&lt;", "<")
	content = strings.ReplaceAll(content, "&gt;", ">")
	content = strings.ReplaceAll(content, "&quot;", "\"")
	content = strings.ReplaceAll(content, "&#39;", "'")
	content = strings.ReplaceAll(content, "&nbsp;", " ")

	content = strings.TrimSpace(content)

	return content
}
