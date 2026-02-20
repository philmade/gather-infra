package tools

import (
	"database/sql"
	"fmt"

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

	return &MemoryTool{db: db}, nil
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

// Close closes the database connection
func (m *MemoryTool) Close() error {
	return m.db.Close()
}
