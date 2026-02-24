package plugins

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"clay/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/plugin"
	"google.golang.org/genai"

	_ "modernc.org/sqlite"
)

// MemoryPluginConfig configures the memory injection plugin.
type MemoryPluginConfig struct {
	DBPath   string  // Path to messages.db (from CLAY_DB)
	SoulRoot string  // Root path for soul files (from CLAY_ROOT)
	TaskDB   *sql.DB // Shared DB for FormatTaskListFromDB
}

// NewMemoryPlugin creates a plugin that injects identity, build snapshot,
// associative memory, and heartbeat context into user messages before the
// agent sees them.
//
// Injection order for EVERY message:
//
//	--- IDENTITY ---           (cached SOUL.md + IDENTITY.md)
//	--- BUILD SNAPSHOT ---     (latest build_snapshot memory)
//	--- ASSOCIATIVE RECALL --- (FTS5 keyword matches)
//	[original message text]
//
// For HEARTBEAT messages, additional context is appended after the message:
//
//	--- YOUR TASKS ---
//	--- HEARTBEAT NOTES ---
//	--- YOUR LAST SESSION ---
//	--- RECENT MEMORIES ---
func NewMemoryPlugin(cfg MemoryPluginConfig) (*plugin.Plugin, error) {
	// Cache soul files at creation time — rarely change, avoids disk I/O per message
	cachedIdentity := loadIdentityBlock(cfg.SoulRoot)

	return plugin.New(plugin.Config{
		Name: "memory-injection",
		OnUserMessageCallback: func(_ agent.InvocationContext, content *genai.Content) (*genai.Content, error) {
			if content == nil || len(content.Parts) == 0 {
				return content, nil
			}

			text := content.Parts[0].Text
			if text == "" {
				return content, nil
			}

			isHeartbeat := strings.HasPrefix(text, "[HEARTBEAT]")

			// Build the enriched message with all injections
			var sb strings.Builder

			// 1. Identity block (every message)
			if cachedIdentity != "" {
				sb.WriteString(cachedIdentity)
				sb.WriteString("\n")
			}

			// 2. Build snapshot (every message)
			snapshot := loadBuildSnapshot(cfg.DBPath)
			sb.WriteString("--- BUILD SNAPSHOT ---\n")
			if snapshot != "" {
				sb.WriteString(snapshot)
			} else {
				sb.WriteString("No build snapshot yet.")
			}
			sb.WriteString("\n\n")

			// 3. Associative recall (every message)
			recall := buildAssociativeRecall(text, cfg.DBPath)
			if recall != "" {
				sb.WriteString(recall)
				sb.WriteString("\n")
			}

			// 4. Original message text
			sb.WriteString(text)

			// 5. Heartbeat extras (appended after message)
			if isHeartbeat {
				appendHeartbeatContext(&sb, cfg.DBPath, cfg.SoulRoot, cfg.TaskDB)
			}

			content.Parts[0].Text = sb.String()
			return content, nil
		},
	})
}

// loadIdentityBlock reads SOUL.md and IDENTITY.md from disk and formats
// them as a single identity injection block. Called once at plugin creation.
func loadIdentityBlock(soulRoot string) string {
	if soulRoot == "" {
		soulRoot = "."
	}
	var sb strings.Builder
	sb.WriteString("--- IDENTITY ---\n")
	loaded := false
	for _, filename := range []string{"SOUL.md", "IDENTITY.md"} {
		data, err := os.ReadFile(soulRoot + "/soul/" + filename)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			sb.WriteString(content)
			sb.WriteString("\n\n")
			loaded = true
		}
	}
	sb.WriteString("---")
	if !loaded {
		return "" // No soul files found — skip the block entirely
	}
	return sb.String()
}

// loadBuildSnapshot queries the latest build_snapshot memory from the database.
func loadBuildSnapshot(dbPath string) string {
	if dbPath == "" {
		return ""
	}
	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return ""
	}
	defer db.Close()

	var content string
	err = db.QueryRow(
		`SELECT content FROM memories
		 WHERE type = 'build_snapshot'
		 ORDER BY created_at DESC LIMIT 1`,
	).Scan(&content)
	if err != nil || content == "" {
		return ""
	}
	if len(content) > 1000 {
		content = content[:1000] + "..."
	}
	return content
}

// appendHeartbeatContext appends task list, HEARTBEAT.md notes, continuation
// memory, and recent highlights to the string builder for heartbeat messages.
func appendHeartbeatContext(sb *strings.Builder, dbPath, soulRoot string, taskDB *sql.DB) {
	// Load structured task list from SQLite (authoritative task tracking)
	taskList := tools.FormatTaskListFromDB(taskDB)
	sb.WriteString("\n\n--- YOUR TASKS ---\n")
	sb.WriteString(taskList)

	// Load HEARTBEAT.md as supplementary notes
	heartbeatMD := loadHeartbeatMD(soulRoot)
	if heartbeatMD != "" {
		sb.WriteString("\n--- HEARTBEAT NOTES (HEARTBEAT.md) ---\n")
		sb.WriteString(heartbeatMD)
	}

	if dbPath == "" {
		return
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		log.Printf("  heartbeat context: db open failed: %v", err)
		return
	}
	defer db.Close()

	// Load latest continuation memory
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

	log.Printf("  heartbeat context: injected tasks + continuation + memories")
}

// loadHeartbeatMD reads the agent's HEARTBEAT.md soul file from disk.
func loadHeartbeatMD(soulRoot string) string {
	if soulRoot == "" {
		soulRoot = "."
	}
	data, err := os.ReadFile(soulRoot + "/soul/HEARTBEAT.md")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// buildAssociativeRecall extracts keywords from the message, queries FTS5 for
// matching memories, and returns the recall block as a string (or "" if none).
func buildAssociativeRecall(text, dbPath string) string {
	if dbPath == "" {
		return ""
	}

	keywords := extractKeywords(text)
	if len(keywords) == 0 {
		return ""
	}

	db, err := sql.Open("sqlite", dbPath+"?mode=ro")
	if err != nil {
		return ""
	}
	defer db.Close()

	// Check if FTS5 table exists
	var tableName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='memories_fts'`).Scan(&tableName)
	if err != nil {
		return "" // FTS5 not available yet
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
		return ""
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
		return ""
	}

	log.Printf("  associative recall: %d keywords → %d memories", len(keywords), len(memories))

	var sb strings.Builder
	sb.WriteString("--- ASSOCIATIVE RECALL ---\n")
	sb.WriteString("These memories surfaced based on your current conversation:\n")
	for _, mem := range memories {
		sb.WriteString(mem)
		sb.WriteString("\n")
	}
	sb.WriteString("---")
	return sb.String()
}

// extractKeywords splits text into meaningful terms for FTS5 queries.
// Returns up to 8 keywords after removing stop words and short terms.
func extractKeywords(text string) []string {
	words := strings.Fields(strings.ToLower(text))

	seen := make(map[string]bool)
	var keywords []string

	for _, w := range words {
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

	// Prefer longer/rarer words — selection sort by length descending
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
