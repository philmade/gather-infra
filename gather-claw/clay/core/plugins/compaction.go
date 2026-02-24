package plugins

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	_ "modernc.org/sqlite"
)

const (
	// compactionThreshold is 90% of 128k tokens. When a session exceeds this,
	// we compact it into a summary + new session.
	compactionThreshold = 115200
)

// CompactionPluginConfig configures the lazy compaction plugin.
type CompactionPluginConfig struct {
	SessionService session.Service
	DBPath         string // Path to messages.db
	LLMBaseURL     string // Anthropic-compatible API base
	LLMAPIKey      string // API key
	LLMModel       string // Model name
}

// NewCompactionPlugin creates a plugin that compacts sessions after each run
// when the session's estimated token count exceeds the threshold.
// Compaction runs synchronously in AfterRunCallback — the user's response
// has already been streamed, so this only delays the HTTP connection close.
func NewCompactionPlugin(cfg CompactionPluginConfig) (*plugin.Plugin, error) {
	// Guard against concurrent compactions on the same session.
	var compacting sync.Map

	return plugin.New(plugin.Config{
		Name: "lazy-compaction",
		AfterRunCallback: func(ctx agent.InvocationContext) {
			sess := ctx.Session()
			if sess == nil {
				return
			}

			events := sess.Events()
			tokens := estimateTokens(events)

			sessionID := sess.ID()
			if tokens <= compactionThreshold {
				return
			}

			log.Printf("  compaction: session %s has ~%d tokens (threshold %d), compacting",
				truncSID(sessionID), tokens, compactionThreshold)

			// Prevent concurrent compactions on the same session
			if _, loaded := compacting.LoadOrStore(sessionID, true); loaded {
				log.Printf("  compaction: session %s already being compacted, skipping", truncSID(sessionID))
				return
			}
			defer compacting.Delete(sessionID)

			appName := sess.AppName()
			userID := sess.UserID()

			// Build transcript from session events (fast — in-memory iteration)
			transcript := buildTranscript(events)
			if transcript == "" {
				log.Printf("  compaction: empty transcript, skipping")
				return
			}

			// Generate summary via LLM
			compactCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()

			summary, err := generateSummary(compactCtx, transcript, cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
			if err != nil {
				log.Printf("  compaction: summary generation failed: %v", err)
				return
			}

			// Store memories to SQLite
			if err := storeMemories(summary, cfg.DBPath); err != nil {
				log.Printf("  compaction: memory storage failed: %v (continuing)", err)
			}

			// Create new session
			createResp, err := cfg.SessionService.Create(compactCtx, &session.CreateRequest{
				AppName: appName,
				UserID:  userID,
			})
			if err != nil {
				log.Printf("  compaction: create session failed: %v", err)
				return
			}
			newSess := createResp.Session

			// Inject summary as a user message in the new session
			contextMsg := fmt.Sprintf(
				"[SYSTEM — Session Compaction]\nYour previous conversation was compacted to stay within context limits. Here is the summary of everything that happened:\n\n%s\n\nContinue from where you left off. The user's next message follows.",
				summary,
			)

			evt := session.NewEvent("compaction")
			evt.Author = "user"
			evt.LLMResponse = model.LLMResponse{
				Content: &genai.Content{
					Role:  "user",
					Parts: []*genai.Part{genai.NewPartFromText(contextMsg)},
				},
			}
			if err := cfg.SessionService.AppendEvent(compactCtx, newSess, evt); err != nil {
				log.Printf("  compaction: append summary event failed: %v", err)
				// Session was created but summary not injected — still usable
			}

			// Delete old session
			if err := cfg.SessionService.Delete(compactCtx, &session.DeleteRequest{
				AppName:   appName,
				UserID:    userID,
				SessionID: sessionID,
			}); err != nil {
				log.Printf("  compaction: delete old session failed: %v", err)
			}

			log.Printf("  compaction: session %s → %s (compacted)", truncSID(sessionID), truncSID(newSess.ID()))
		},
	})
}

// estimateTokens counts characters across all events and divides by 4.
func estimateTokens(events session.Events) int {
	totalChars := 0
	for evt := range events.All() {
		if evt.Content == nil {
			continue
		}
		for _, part := range evt.Content.Parts {
			if part.Text != "" {
				totalChars += len(part.Text)
			} else {
				// Non-text parts (function calls, etc.) — estimate via JSON size
				data, err := json.Marshal(part)
				if err == nil {
					totalChars += len(data)
				}
			}
		}
	}
	return totalChars / 4
}

// buildTranscript extracts text from all session events into a conversation transcript.
func buildTranscript(events session.Events) string {
	var sb strings.Builder
	for evt := range events.All() {
		if evt.Content == nil {
			continue
		}

		var texts []string
		for _, part := range evt.Content.Parts {
			if part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		if len(texts) == 0 {
			continue
		}

		author := evt.Author
		if author == "" {
			author = "unknown"
		}
		fmt.Fprintf(&sb, "[%s]: %s\n\n", author, strings.Join(texts, "\n"))
	}
	return sb.String()
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
func generateSummary(ctx context.Context, transcript, llmBaseURL, llmAPIKey, llmModel string) (string, error) {
	if llmAPIKey == "" {
		return "", fmt.Errorf("no LLM API key configured")
	}

	// Truncate transcript if extremely long (keep last ~100k chars for the LLM)
	maxTranscript := 100000
	if len(transcript) > maxTranscript {
		transcript = transcript[len(transcript)-maxTranscript:]
	}

	prompt := fmt.Sprintf(compactionPrompt, transcript)

	payload := map[string]any{
		"model":      llmModel,
		"max_tokens": 4096,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", llmBaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", llmAPIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
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
func storeMemories(summary, dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("no messages DB path configured")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

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

func truncSID(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
