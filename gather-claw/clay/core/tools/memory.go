package tools

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	_ "modernc.org/sqlite"
)

// MemoryTool handles SQLite-based persistent memory
type MemoryTool struct {
	db *sql.DB
}

// NewMemoryTool creates a new memory tool
func NewMemoryTool(dbPath string) (*MemoryTool, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Create table if not exists
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
		return nil, err
	}

	mt := &MemoryTool{db: db}

	// Create FTS5 virtual table for associative recall
	if err := mt.initFTS5(); err != nil {
		log.Printf("FTS5 init: %v (associative recall disabled)", err)
	}

	return mt, nil
}

// Store saves a memory
func (m *MemoryTool) Store(content, memType, tags string) error {
	_, err := m.db.Exec(
		`INSERT INTO memories (content, type, tags, importance) VALUES (?, ?, ?, 3)`,
		content, memType, tags,
	)
	return err
}

// Recall retrieves recent memories
func (m *MemoryTool) Recall(days int) ([]string, error) {
	rows, err := m.db.Query(
		`SELECT content FROM memories
		 WHERE created_at > datetime('now', ?)
		 ORDER BY importance DESC, created_at DESC
		 LIMIT 100`,
		fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		memories = append(memories, content)
	}

	return memories, nil
}

// Search finds memories by keyword
func (m *MemoryTool) Search(query string) ([]string, error) {
	rows, err := m.db.Query(
		`SELECT content FROM memories
		 WHERE content LIKE ?
		 ORDER BY importance DESC, created_at DESC
		 LIMIT 50`,
		"%"+query+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			return nil, err
		}
		results = append(results, content)
	}

	return results, nil
}

// LatestContinuation returns the most recent "continuation" memory, or empty string if none.
func (m *MemoryTool) LatestContinuation() string {
	var content string
	err := m.db.QueryRow(
		`SELECT content FROM memories
		 WHERE type = 'continuation'
		 ORDER BY created_at DESC
		 LIMIT 1`,
	).Scan(&content)
	if err != nil {
		return ""
	}
	return content
}

// RecentHighlight returns the N most recent memories (by importance, then recency).
func (m *MemoryTool) RecentHighlight(limit int) []string {
	rows, err := m.db.Query(
		`SELECT content FROM memories
		 WHERE type != 'continuation'
		 ORDER BY importance DESC, created_at DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			continue
		}
		results = append(results, content)
	}
	return results
}

// initFTS5 creates the FTS5 virtual table and sync triggers.
func (m *MemoryTool) initFTS5() error {
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts
		 USING fts5(content, tags, content=memories, content_rowid=id)`,

		`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, content, tags) VALUES (new.id, new.content, new.tags);
		END`,

		`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, tags) VALUES('delete', old.id, old.content, old.tags);
		END`,

		`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, content, tags) VALUES('delete', old.id, old.content, old.tags);
			INSERT INTO memories_fts(rowid, content, tags) VALUES (new.id, new.content, new.tags);
		END`,
	}

	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return fmt.Errorf("FTS5 setup: %w", err)
		}
	}

	// Backfill if FTS5 table is empty but memories exist
	if err := m.rebuildFTSIfNeeded(); err != nil {
		log.Printf("FTS5 backfill: %v", err)
	}

	return nil
}

// rebuildFTSIfNeeded populates the FTS5 index from existing memories if it's empty.
func (m *MemoryTool) rebuildFTSIfNeeded() error {
	var ftsCount int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM memories_fts`).Scan(&ftsCount); err != nil {
		return err
	}
	if ftsCount > 0 {
		return nil // Already populated
	}

	var memCount int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&memCount); err != nil {
		return err
	}
	if memCount == 0 {
		return nil // Nothing to backfill
	}

	_, err := m.db.Exec(`INSERT INTO memories_fts(rowid, content, tags)
		SELECT id, content, tags FROM memories`)
	if err != nil {
		return fmt.Errorf("backfill: %w", err)
	}

	log.Printf("FTS5 index rebuilt: %d memories", memCount)
	return nil
}

// SearchFTS performs a full-text search using FTS5 with BM25 ranking.
// Returns up to limit matching memory contents with relative timestamps.
func (m *MemoryTool) SearchFTS(query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 3
	}

	rows, err := m.db.Query(
		`SELECT m.content,
			CAST((julianday('now') - julianday(m.created_at)) AS INTEGER) AS days_ago
		 FROM memories_fts f
		 JOIN memories m ON m.id = f.rowid
		 WHERE memories_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var content string
		var daysAgo int
		if err := rows.Scan(&content, &daysAgo); err != nil {
			continue
		}
		// Truncate long memories
		if len(content) > 300 {
			content = content[:300] + "..."
		}
		// Format relative time
		var ago string
		switch {
		case daysAgo == 0:
			ago = "today"
		case daysAgo == 1:
			ago = "yesterday"
		default:
			ago = fmt.Sprintf("%d days ago", daysAgo)
		}
		results = append(results, fmt.Sprintf("%s (%s)", content, ago))
	}

	return results, nil
}

// BuildFTSQuery takes keywords and joins them with OR for FTS5 MATCH.
func BuildFTSQuery(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	// Escape double quotes in keywords
	escaped := make([]string, len(keywords))
	for i, kw := range keywords {
		escaped[i] = `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
	}
	return strings.Join(escaped, " OR ")
}

// DB returns the underlying database connection for sharing with other tools.
func (m *MemoryTool) DB() *sql.DB {
	return m.db
}

// Close closes the database connection
func (m *MemoryTool) Close() error {
	return m.db.Close()
}
