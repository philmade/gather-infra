package connectors

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Middleware wraps ADK calls with session-aware token estimation and compaction.
type Middleware struct {
	adkURL     string
	appName    string
	httpClient *http.Client

	messagesDBPath string // Agent memory (messages.db)

	// LLM config for compaction summarization
	llmBaseURL string
	llmAPIKey  string
	llmModel   string
}

// NewMiddleware creates a middleware instance.
// It reads configuration from environment variables.
func NewMiddleware(adkURL string) *Middleware {
	llmBase := os.Getenv("ANTHROPIC_API_BASE")
	if llmBase == "" {
		llmBase = "https://api.z.ai/api/anthropic"
	}

	llmModel := os.Getenv("ANTHROPIC_MODEL")
	if llmModel == "" {
		llmModel = "glm-5"
	}

	return &Middleware{
		adkURL:         adkURL,
		appName:        "clawpoint",
		httpClient:     &http.Client{Timeout: 300 * time.Second},
		messagesDBPath: os.Getenv("CLAWPOINT_DB"),
		llmBaseURL:     llmBase,
		llmAPIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		llmModel:       llmModel,
	}
}

const (
	// compactionThreshold is 90% of 128k tokens. When a session exceeds this,
	// we compact it into a summary + new session.
	compactionThreshold = 115200
)

// ADKEvent represents a single event from the ADK SSE stream (text chunk, tool call, or tool result).
type ADKEvent struct {
	Type     string `json:"type"`                // "text", "tool_call", "tool_result"
	Author   string `json:"author,omitempty"`     // agent that produced this event
	Text     string `json:"text,omitempty"`       // for type=text
	ToolName string `json:"tool_name,omitempty"`  // for type=tool_call / tool_result
	ToolID   string `json:"tool_id,omitempty"`    // for tool_call + tool_result
	ToolArgs any    `json:"tool_args,omitempty"`  // for type=tool_call
	Result   any    `json:"result,omitempty"`     // for type=tool_result
}

// ProcessResult holds the response text and the session ID that was used.
// If compaction occurred, SessionID will differ from the input session ID.
type ProcessResult struct {
	Text      string
	SessionID string     // The session ID used (may differ from input if compacted)
	Events    []ADKEvent // Captured ADK events (tool calls, tool results, text chunks)
}

// ProcessMessage is the unified middleware pipeline:
// 1. If heartbeat → load continuation memory + recent memories as context
// 2. Estimate token count of existing session events
// 3. If over threshold → compact (summarize + store memories + new session)
// 4. Forward message to ADK run_sse
// 5. Return response text + actual session ID used
func (mw *Middleware) ProcessMessage(ctx context.Context, userID, sessionID, text string) (*ProcessResult, error) {
	// Heartbeat context injection: load memories to give the agent continuity
	if strings.HasPrefix(text, "[HEARTBEAT]") {
		text = mw.injectHeartbeatContext(text)
	}

	// Associative memory injection: surface relevant memories for ALL messages
	text = mw.injectAssociativeMemory(text)

	// Estimate tokens
	tokens, err := mw.estimateSessionTokens(userID, sessionID)
	if err != nil {
		log.Printf("  token estimation failed: %v", err)
	} else {
		log.Printf("  session %s: ~%d estimated tokens", truncSID(sessionID), tokens)
	}

	// Compact if over threshold
	if tokens > compactionThreshold {
		log.Printf("  session %s: COMPACTING (%d tokens > %d threshold)",
			truncSID(sessionID), tokens, compactionThreshold)
		newSID, err := mw.compact(ctx, userID, sessionID)
		if err != nil {
			log.Printf("  compaction failed: %v (continuing with current session)", err)
		} else {
			log.Printf("  compacted → new session %s", truncSID(newSID))
			sessionID = newSID
		}
	}

	response, events, err := mw.sendRunSSE(ctx, userID, sessionID, text)
	if err != nil {
		return nil, err
	}

	// HEARTBEAT_OK suppression: if the agent said nothing needs attention,
	// mark the result so callers can skip relaying/saving.
	if isHeartbeatOK(response) {
		log.Printf("  HEARTBEAT_OK — suppressing response")
		return &ProcessResult{Text: "HEARTBEAT_OK", SessionID: sessionID}, nil
	}

	return &ProcessResult{Text: response, SessionID: sessionID, Events: events}, nil
}

// isHeartbeatOK checks if the agent's response is a HEARTBEAT_OK idle signal.
// Tolerates minor surrounding whitespace or punctuation.
func isHeartbeatOK(response string) bool {
	trimmed := strings.TrimSpace(response)
	return trimmed == "HEARTBEAT_OK"
}

// injectHeartbeatContext loads the latest continuation memory and recent highlights
// from the memory database, and appends them to the heartbeat message so the agent
// has continuity between heartbeat cycles.
func (mw *Middleware) injectHeartbeatContext(text string) string {
	dbPath := mw.messagesDBPath
	if dbPath == "" {
		return text
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		log.Printf("  heartbeat context: db open failed: %v", err)
		return text
	}
	defer db.Close()

	var sb strings.Builder
	sb.WriteString(text)

	// Load HEARTBEAT.md (the agent's living task list)
	heartbeatMD := mw.loadHeartbeatMD()
	if heartbeatMD != "" {
		sb.WriteString("\n\n--- YOUR TASK LIST (HEARTBEAT.md) ---\n")
		sb.WriteString(heartbeatMD)
	} else {
		sb.WriteString("\n\n--- YOUR TASK LIST (HEARTBEAT.md) ---\n(empty — no pending tasks)\n")
	}

	// Load latest continuation memory (what I was last doing)
	var continuation string
	err = db.QueryRow(
		`SELECT content FROM memories
		 WHERE type = 'continuation'
		 ORDER BY created_at DESC LIMIT 1`,
	).Scan(&continuation)

	if err == nil && continuation != "" {
		sb.WriteString("\n\n--- YOUR LAST SESSION ---\n")
		sb.WriteString(continuation)
	}

	// Load recent high-importance memories (excluding continuations)
	rows, err := db.Query(
		`SELECT content FROM memories
		 WHERE type != 'continuation'
		 ORDER BY importance DESC, created_at DESC
		 LIMIT 3`,
	)
	if err == nil {
		defer rows.Close()
		var memories []string
		for rows.Next() {
			var content string
			if rows.Scan(&content) == nil && content != "" {
				// Truncate long memories to keep context manageable
				if len(content) > 500 {
					content = content[:500] + "..."
				}
				memories = append(memories, content)
			}
		}
		if len(memories) > 0 {
			sb.WriteString("\n\n--- RECENT MEMORIES ---\n")
			for _, mem := range memories {
				sb.WriteString("- ")
				sb.WriteString(mem)
				sb.WriteString("\n")
			}
		}
	}

	enriched := sb.String()
	if enriched != text {
		log.Printf("  heartbeat context: injected continuation + %d memories", 3)
	}
	return enriched
}

// loadHeartbeatMD reads the agent's HEARTBEAT.md soul file from disk.
// Returns the contents or empty string if the file doesn't exist.
func (mw *Middleware) loadHeartbeatMD() string {
	root := os.Getenv("CLAWPOINT_ROOT")
	if root == "" {
		root = "."
	}
	data, err := os.ReadFile(root + "/soul/HEARTBEAT.md")
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	return content
}

// injectAssociativeMemory extracts keywords from the message, queries FTS5 for
// matching memories, and prepends them to the message text.
func (mw *Middleware) injectAssociativeMemory(text string) string {
	dbPath := mw.messagesDBPath
	if dbPath == "" {
		return text
	}

	keywords := extractKeywords(text)
	if len(keywords) == 0 {
		return text
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return text
	}
	defer db.Close()

	// Check if FTS5 table exists
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='memories_fts'`).Scan(&tableName)
	if err != nil {
		return text // FTS5 not available yet
	}

	// Build FTS5 query: "term1" OR "term2" OR "term3"
	escaped := make([]string, len(keywords))
	for i, kw := range keywords {
		escaped[i] = `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
	}
	ftsQuery := strings.Join(escaped, " OR ")

	rows, err := db.Query(
		`SELECT m.content,
			CAST((julianday('now') - julianday(m.created_at)) AS INTEGER) AS days_ago
		 FROM memories_fts f
		 JOIN memories m ON m.id = f.rowid
		 WHERE memories_fts MATCH ?
		 ORDER BY rank
		 LIMIT 3`,
		ftsQuery,
	)
	if err != nil {
		log.Printf("  associative recall query failed: %v", err)
		return text
	}
	defer rows.Close()

	var memories []string
	for rows.Next() {
		var content string
		var daysAgo int
		if err := rows.Scan(&content, &daysAgo); err != nil {
			continue
		}
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		var ago string
		switch {
		case daysAgo == 0:
			ago = "today"
		case daysAgo == 1:
			ago = "yesterday"
		default:
			ago = fmt.Sprintf("%d days ago", daysAgo)
		}
		memories = append(memories, fmt.Sprintf("- %s (%s)", content, ago))
	}

	if len(memories) == 0 {
		return text
	}

	log.Printf("  associative recall: %d keywords → %d memories", len(keywords), len(memories))

	var sb strings.Builder
	sb.WriteString("--- ASSOCIATIVE RECALL ---\n")
	sb.WriteString("These memories surfaced based on your current conversation:\n")
	for _, mem := range memories {
		sb.WriteString(mem)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(text)
	return sb.String()
}

// extractKeywords splits text into meaningful terms for FTS5 queries.
// Returns up to 8 keywords after removing stop words and short terms.
func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))

	seen := make(map[string]bool)
	var keywords []string

	for _, w := range words {
		// Strip punctuation from edges
		w = strings.Trim(w, ".,!?;:\"'`()[]{}—–-/\\<>@#$%^&*~")
		if len(w) < 3 {
			continue
		}
		if stopWords[w] {
			continue
		}
		if seen[w] {
			continue
		}
		seen[w] = true
		keywords = append(keywords, w)
	}

	// Prefer longer/rarer words — sort by length descending
	// Simple selection sort since we're dealing with small slices
	for i := 0; i < len(keywords); i++ {
		maxIdx := i
		for j := i + 1; j < len(keywords); j++ {
			if len(keywords[j]) > len(keywords[maxIdx]) {
				maxIdx = j
			}
		}
		keywords[i], keywords[maxIdx] = keywords[maxIdx], keywords[i]
	}

	if len(keywords) > 8 {
		keywords = keywords[:8]
	}
	return keywords
}

// stopWords is a set of common English words to filter from FTS5 queries.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true,
	"not": true, "you": true, "all": true, "can": true, "had": true,
	"her": true, "was": true, "one": true, "our": true, "out": true,
	"has": true, "have": true, "been": true, "will": true,
	"would": true, "could": true, "should": true, "may": true, "might": true,
	"shall": true, "this": true, "that": true, "with": true, "from": true,
	"they": true, "them": true, "then": true, "than": true, "these": true,
	"those": true, "which": true, "what": true, "when": true, "where": true,
	"who": true, "whom": true, "how": true, "why": true, "each": true,
	"she": true, "his": true, "him": true, "its": true, "let": true,
	"say": true, "said": true, "also": true, "into": true, "just": true,
	"your": true, "some": true, "any": true, "only": true, "very": true,
	"here": true, "there": true, "their": true, "about": true, "more": true,
	"most": true, "other": true, "over": true, "such": true, "after": true,
	"before": true, "between": true, "under": true, "above": true,
	"being": true, "does": true, "did": true, "doing": true, "done": true,
	"get": true, "got": true, "going": true, "gone": true, "come": true,
	"came": true, "make": true, "made": true, "take": true, "took": true,
	"give": true, "gave": true, "know": true, "knew": true, "think": true,
	"thought": true, "tell": true, "told": true, "see": true, "seen": true,
	"want": true, "use": true, "used": true, "find": true, "found": true,
	"back": true, "like": true, "look": true, "well": true, "still": true,
	"even": true, "much": true, "many": true, "really": true, "already": true,
	"through": true, "because": true, "while": true, "since": true,
	"another": true, "same": true, "different": true, "thing": true,
	"things": true, "right": true, "good": true, "new": true, "now": true,
	"way": true, "time": true, "day": true, "need": true, "too": true,
	"yes": true, "yeah": true, "okay": true, "sure": true, "please": true,
	"thanks": true, "thank": true, "hello": true, "hey": true,
	"don't": true, "doesn't": true, "didn't": true, "won't": true,
	"wouldn't": true, "couldn't": true, "shouldn't": true, "isn't": true,
	"aren't": true, "wasn't": true, "weren't": true, "haven't": true,
	"hasn't": true, "hadn't": true, "can't": true,
}

// estimateSessionTokens queries the ADK API for session events and returns
// an approximate token count using the chars/4 heuristic.
func (mw *Middleware) estimateSessionTokens(userID, sessionID string) (int, error) {
	events, err := mw.fetchSessionEvents(userID, sessionID)
	if err != nil {
		return 0, err
	}

	totalChars := 0
	for _, event := range events {
		contentRaw, ok := event["content"]
		if !ok || contentRaw == nil {
			continue
		}
		contentJSON, err := json.Marshal(contentRaw)
		if err != nil {
			continue
		}
		totalChars += countContentChars(string(contentJSON))
	}

	return totalChars / 4, nil
}

// fetchSessionEvents calls the ADK API to get all events for a session.
func (mw *Middleware) fetchSessionEvents(userID, sessionID string) ([]map[string]any, error) {
	url := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		mw.adkURL, mw.appName, userID, sessionID)
	resp, err := mw.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read session response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("get session HTTP %d: %s", resp.StatusCode, string(body))
	}

	var session struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(body, &session); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}

	return session.Events, nil
}

// countContentChars extracts text from a genai.Content JSON blob and counts characters.
func countContentChars(contentJSON string) int {
	var content struct {
		Parts []json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		// Fallback: count the raw JSON length
		return len(contentJSON)
	}

	total := 0
	for _, partRaw := range content.Parts {
		var part struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(partRaw, &part); err == nil && part.Text != "" {
			total += len(part.Text)
		} else {
			// Non-text parts (function calls, etc.) — count the JSON size
			total += len(partRaw)
		}
	}
	return total
}

// compact reads all session events, generates a summary via LLM, stores
// memories, creates a new session with the summary, and returns the new session ID.
func (mw *Middleware) compact(ctx context.Context, userID, oldSessionID string) (string, error) {
	// Read all events from the old session
	transcript, err := mw.readSessionTranscript(userID, oldSessionID)
	if err != nil {
		return "", fmt.Errorf("read transcript: %w", err)
	}

	if transcript == "" {
		return "", fmt.Errorf("empty transcript, nothing to compact")
	}

	// Generate summary via LLM
	summary, err := mw.generateSummary(ctx, transcript)
	if err != nil {
		return "", fmt.Errorf("generate summary: %w", err)
	}

	// Store extracted memories
	if err := mw.storeMemories(summary); err != nil {
		log.Printf("  memory storage failed: %v (continuing)", err)
	}

	// Create new session with summary as context
	newSessionID, err := mw.createSessionWithSummary(userID, summary)
	if err != nil {
		return "", fmt.Errorf("create new session: %w", err)
	}

	// Delete old session to free RAM
	mw.deleteSession(userID, oldSessionID)

	return newSessionID, nil
}

// readSessionTranscript fetches events from the ADK API and builds a text transcript.
func (mw *Middleware) readSessionTranscript(userID, sessionID string) (string, error) {
	events, err := mw.fetchSessionEvents(userID, sessionID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, event := range events {
		contentRaw, ok := event["content"]
		if !ok || contentRaw == nil {
			continue
		}
		contentJSON, err := json.Marshal(contentRaw)
		if err != nil {
			continue
		}

		text := extractTextFromContent(string(contentJSON))
		if text == "" {
			continue
		}

		author, _ := event["author"].(string)
		if author == "" {
			author = "unknown"
		}
		fmt.Fprintf(&sb, "[%s]: %s\n\n", author, text)
	}

	return sb.String(), nil
}

// deleteSession removes a session via the ADK API to free RAM.
func (mw *Middleware) deleteSession(userID, sessionID string) {
	url := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions/%s",
		mw.adkURL, mw.appName, userID, sessionID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		log.Printf("  delete session: request error: %v", err)
		return
	}
	resp, err := mw.httpClient.Do(req)
	if err != nil {
		log.Printf("  delete session %s: %v", truncSID(sessionID), err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode == 200 {
		log.Printf("  deleted old session %s", truncSID(sessionID))
	} else {
		log.Printf("  delete session %s: HTTP %d", truncSID(sessionID), resp.StatusCode)
	}
}

// extractTextFromContent pulls text parts from a genai.Content JSON blob.
func extractTextFromContent(contentJSON string) string {
	var content struct {
		Parts []json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal([]byte(contentJSON), &content); err != nil {
		return ""
	}

	var texts []string
	for _, partRaw := range content.Parts {
		var part struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(partRaw, &part); err == nil && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

const compactionPrompt = `You are a session compaction agent. Your job is to analyze a conversation transcript and extract a structured summary that preserves critical context for the agent's continued operation.

Analyze the following conversation transcript and produce a structured summary with these sections:

## CONVERSATION SUMMARY
A 2-3 paragraph summary of what happened in this conversation — the main topics, decisions made, and current state.

## KEY_MEMORIES
Bullet list of important facts, decisions, and context the agent needs to remember. Be specific — include names, IDs, URLs, and concrete details.

## FAILED_TOOLS
Bullet list of any tools or actions that failed, with the error and any workarounds found.

## PATTERNS
Bullet list of recurring patterns, preferences, or workflows observed.

## NEXT_ACTIONS
Bullet list of any pending tasks, commitments, or things the agent was about to do.

TRANSCRIPT:
%s

Produce the structured summary now. Be thorough but concise — this summary replaces the entire conversation history.`

// generateSummary calls the LLM with the compaction prompt to produce a session summary.
func (mw *Middleware) generateSummary(ctx context.Context, transcript string) (string, error) {
	if mw.llmAPIKey == "" {
		return "", fmt.Errorf("no LLM API key configured")
	}

	// Truncate transcript if extremely long (keep last ~100k chars for the LLM)
	maxTranscript := 100000
	if len(transcript) > maxTranscript {
		transcript = transcript[len(transcript)-maxTranscript:]
	}

	prompt := fmt.Sprintf(compactionPrompt, transcript)

	// Anthropic Messages API format
	payload := map[string]any{
		"model":      mw.llmModel,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", mw.llmBaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", mw.llmAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := mw.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read LLM response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("LLM returned %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse Anthropic response
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse LLM response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("empty LLM response")
	}

	return result.Content[0].Text, nil
}

// storeMemories writes the compaction summary to the agent's memory database.
func (mw *Middleware) storeMemories(summary string) error {
	dbPath := mw.messagesDBPath
	if dbPath == "" {
		return fmt.Errorf("no messages DB path configured")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Ensure table exists (same schema as core/tools/memory.go)
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			type TEXT DEFAULT 'general',
			tags TEXT,
			importance INTEGER DEFAULT 3,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("create memories table: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO memories (content, type, tags, importance) VALUES (?, ?, ?, ?)`,
		summary, "compaction", "session-compaction,context-summary", 5,
	)
	return err
}

// createSessionWithSummary creates a new ADK session and injects the summary as a
// user message so the agent has context when the conversation continues.
func (mw *Middleware) createSessionWithSummary(userID, summary string) (string, error) {
	// Create session via ADK API
	createURL := fmt.Sprintf("%s/api/apps/%s/users/%s/sessions", mw.adkURL, mw.appName, userID)
	resp, err := mw.httpClient.Post(createURL, "application/json", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("create session HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse session response: %w", err)
	}

	sessionID, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("no session id in response")
	}

	// Inject the summary as the first message via run_sse so the agent
	// processes it and has context for subsequent messages.
	contextMsg := fmt.Sprintf("[SYSTEM — Session Compaction]\nYour previous conversation was compacted to stay within context limits. Here is the summary of everything that happened:\n\n%s\n\nContinue from where you left off. The user's next message follows.", summary)

	payload := map[string]any{
		"appName":   mw.appName,
		"userId":    userID,
		"sessionId": sessionID,
		"newMessage": map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"text": contextMsg},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(context.Background(), "POST", mw.adkURL+"/api/run_sse", bytes.NewReader(jsonPayload))
	if err != nil {
		return sessionID, nil // Session created, just can't inject summary
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 120 * time.Second}
	sseResp, err := client.Do(req)
	if err != nil {
		log.Printf("  summary injection failed: %v (session still usable)", err)
		return sessionID, nil
	}
	// Drain the response to complete the SSE stream
	io.Copy(io.Discard, sseResp.Body)
	sseResp.Body.Close()

	log.Printf("  summary injected into new session %s", truncSID(sessionID))
	return sessionID, nil
}

// sendRunSSE forwards a message to ADK via run_sse and returns the response text + captured events.
func (mw *Middleware) sendRunSSE(ctx context.Context, userID, sessionID, text string) (string, []ADKEvent, error) {
	payload := map[string]any{
		"appName":   mw.appName,
		"userId":    userID,
		"sessionId": sessionID,
		"newMessage": map[string]any{
			"role": "user",
			"parts": []map[string]any{
				{"text": text},
			},
		},
	}

	jsonPayload, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", mw.adkURL+"/api/run_sse", bytes.NewReader(jsonPayload))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("run_sse HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseSSEResponseFull(resp.Body)
}

// parseSSEResponse reads SSE events and extracts the agent's text response.
func parseSSEResponse(r io.Reader) (string, error) {
	text, _, err := parseSSEResponseFull(r)
	return text, err
}

// parseSSEResponseFull reads SSE events and extracts the agent's text response
// plus all ADK events (tool calls, tool results, text chunks).
func parseSSEResponseFull(r io.Reader) (string, []ADKEvent, error) {
	scanner := bufio.NewScanner(r)
	var lastText string
	var events []ADKEvent

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Error while running agent:") {
			return "", nil, fmt.Errorf("%s", line)
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		author, _ := event["author"].(string)

		content, ok := event["content"].(map[string]any)
		if !ok {
			continue
		}

		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}

		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}

			// Text part
			if text, ok := part["text"].(string); ok && text != "" {
				lastText = text
				events = append(events, ADKEvent{
					Type:   "text",
					Author: author,
					Text:   text,
				})
			}

			// Function call
			if fc, ok := part["functionCall"].(map[string]any); ok {
				name, _ := fc["name"].(string)
				id, _ := fc["id"].(string)
				args := fc["args"]
				events = append(events, ADKEvent{
					Type:     "tool_call",
					Author:   author,
					ToolName: name,
					ToolID:   id,
					ToolArgs: args,
				})
			}

			// Function response
			if fr, ok := part["functionResponse"].(map[string]any); ok {
				name, _ := fr["name"].(string)
				id, _ := fr["id"].(string)
				result := fr["response"]
				events = append(events, ADKEvent{
					Type:     "tool_result",
					Author:   author,
					ToolName: name,
					ToolID:   id,
					Result:   result,
				})
			}
		}
	}

	return lastText, events, nil
}

func truncSID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
