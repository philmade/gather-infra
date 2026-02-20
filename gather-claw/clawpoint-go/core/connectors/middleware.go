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

	// Paths to SQLite databases (read-only for estimation, write for memories)
	sessionsDBPath string // ADK session events
	messagesDBPath string // Agent memory (messages.db)

	// LLM config for compaction summarization
	llmBaseURL string
	llmAPIKey  string
	llmModel   string
}

// NewMiddleware creates a middleware instance.
// It reads configuration from environment variables.
func NewMiddleware(adkURL string) *Middleware {
	root := os.Getenv("CLAWPOINT_ROOT")
	if root == "" {
		root = "."
	}

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
		sessionsDBPath: root + "/data/sessions.db",
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

// ProcessResult holds the response text and the session ID that was used.
// If compaction occurred, SessionID will differ from the input session ID.
type ProcessResult struct {
	Text      string
	SessionID string // The session ID used (may differ from input if compacted)
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

	// Estimate tokens
	tokens, err := mw.estimateSessionTokens(sessionID)
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

	response, err := mw.sendRunSSE(ctx, userID, sessionID, text)
	if err != nil {
		return nil, err
	}

	return &ProcessResult{Text: response, SessionID: sessionID}, nil
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

// estimateSessionTokens queries sessions.db for all events in a session and
// returns an approximate token count using the chars/4 heuristic.
func (mw *Middleware) estimateSessionTokens(sessionID string) (int, error) {
	db, err := sql.Open("sqlite", mw.sessionsDBPath+"?mode=ro")
	if err != nil {
		return 0, fmt.Errorf("open sessions db: %w", err)
	}
	defer db.Close()

	// Query all content JSON blobs for this session
	rows, err := db.Query(
		`SELECT content FROM events WHERE session_id = ? AND content IS NOT NULL AND content != ''`,
		sessionID,
	)
	if err != nil {
		return 0, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	totalChars := 0
	for rows.Next() {
		var contentJSON string
		if err := rows.Scan(&contentJSON); err != nil {
			continue
		}
		totalChars += countContentChars(contentJSON)
	}

	return totalChars / 4, nil
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
	transcript, err := mw.readSessionTranscript(oldSessionID)
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

	return newSessionID, nil
}

// readSessionTranscript reads events from sessions.db and builds a text transcript.
func (mw *Middleware) readSessionTranscript(sessionID string) (string, error) {
	db, err := sql.Open("sqlite", mw.sessionsDBPath+"?mode=ro")
	if err != nil {
		return "", err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT author, content FROM events
		 WHERE session_id = ? AND content IS NOT NULL AND content != ''
		 ORDER BY timestamp ASC`,
		sessionID,
	)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var author, contentJSON string
		if err := rows.Scan(&author, &contentJSON); err != nil {
			continue
		}

		text := extractTextFromContent(contentJSON)
		if text == "" {
			continue
		}

		if author == "" {
			author = "unknown"
		}
		fmt.Fprintf(&sb, "[%s]: %s\n\n", author, text)
	}

	return sb.String(), nil
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

// sendRunSSE forwards a message to ADK via run_sse and returns the response text.
// This is the same as MatterbridgeConnector.sendRunSSE but on the middleware struct.
func (mw *Middleware) sendRunSSE(ctx context.Context, userID, sessionID, text string) (string, error) {
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
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("run_sse HTTP %d: %s", resp.StatusCode, string(body))
	}

	return parseSSEResponse(resp.Body)
}

// parseSSEResponse reads SSE events and extracts the agent's text response.
func parseSSEResponse(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	var lastText string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "Error while running agent:") {
			return "", fmt.Errorf("%s", line)
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := line[6:]
		var event map[string]any
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

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
			if text, ok := part["text"].(string); ok && text != "" {
				lastText = text
			}
		}
	}

	return lastText, nil
}

func truncSID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
