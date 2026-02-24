package tools

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// TaskTool handles SQLite-based structured task management.
type TaskTool struct {
	db *sql.DB
}

// TaskInfo represents a single task for display.
type TaskInfo struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	CreatedAt   string `json:"created_at"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// NewTaskTool creates a new task tool using an existing DB connection (shared with MemoryTool).
func NewTaskTool(db *sql.DB) (*TaskTool, error) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT,
			status TEXT DEFAULT 'pending',
			priority INTEGER DEFAULT 3,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			started_at TIMESTAMP,
			completed_at TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("create tasks table: %w", err)
	}
	return &TaskTool{db: db}, nil
}

// Add creates a new task and returns its ID.
func (t *TaskTool) Add(title, description string, priority int) (int64, error) {
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}
	if priority < 1 || priority > 5 {
		priority = 3
	}
	result, err := t.db.Exec(
		`INSERT INTO tasks (title, description, priority) VALUES (?, ?, ?)`,
		title, description, priority,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// List returns tasks filtered by status. Empty status returns all active (pending + in_progress).
func (t *TaskTool) List(status string) ([]TaskInfo, error) {
	var query string
	var args []any

	switch status {
	case "pending", "in_progress", "completed":
		query = `SELECT id, title, description, status, priority, created_at,
				COALESCE(started_at, ''), COALESCE(completed_at, '')
				FROM tasks WHERE status = ? ORDER BY priority ASC, id ASC`
		args = []any{status}
	case "":
		// Active tasks: pending + in_progress
		query = `SELECT id, title, description, status, priority, created_at,
				COALESCE(started_at, ''), COALESCE(completed_at, '')
				FROM tasks WHERE status IN ('pending', 'in_progress')
				ORDER BY priority ASC, id ASC`
	default:
		return nil, fmt.Errorf("unknown status filter %q — use pending, in_progress, completed, or blank for active", status)
	}

	rows, err := t.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []TaskInfo
	for rows.Next() {
		var ti TaskInfo
		if err := rows.Scan(&ti.ID, &ti.Title, &ti.Description, &ti.Status,
			&ti.Priority, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt); err != nil {
			continue
		}
		tasks = append(tasks, ti)
	}
	return tasks, nil
}

// Start marks a task as in_progress.
func (t *TaskTool) Start(id int64) error {
	result, err := t.db.Exec(
		`UPDATE tasks SET status = 'in_progress', started_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status = 'pending'`,
		id,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task #%d not found or not in pending status", id)
	}
	return nil
}

// Complete marks a task as completed.
func (t *TaskTool) Complete(id int64) error {
	result, err := t.db.Exec(
		`UPDATE tasks SET status = 'completed', completed_at = CURRENT_TIMESTAMP
		 WHERE id = ? AND status IN ('pending', 'in_progress')`,
		id,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task #%d not found or already completed", id)
	}
	return nil
}

// Remove deletes a task.
func (t *TaskTool) Remove(id int64) error {
	result, err := t.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("task #%d not found", id)
	}
	return nil
}

// FormatTaskList renders the structured task list for heartbeat injection.
func (t *TaskTool) FormatTaskList() string {
	return FormatTaskListFromDB(t.db)
}

// FormatTaskListFromDB renders the structured task list from a DB connection.
// Standalone function so middleware can call it without needing the full TaskTool.
func FormatTaskListFromDB(db *sql.DB) string {
	// Check if tasks table exists
	var tableName string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'`).Scan(&tableName)
	if err != nil {
		return "(no task system initialized)\n"
	}

	var sb strings.Builder
	now := time.Now()

	// In-progress tasks
	rows, err := db.Query(
		`SELECT id, title, priority, started_at FROM tasks
		 WHERE status = 'in_progress' ORDER BY priority ASC, id ASC`)
	if err == nil {
		defer rows.Close()
		var inProgress []string
		for rows.Next() {
			var id int64
			var title string
			var priority int
			var startedAt sql.NullString
			if rows.Scan(&id, &title, &priority, &startedAt) != nil {
				continue
			}
			ago := formatRelativeTime(startedAt.String, now)
			inProgress = append(inProgress, fmt.Sprintf("[#%d P%d] %s (started %s)", id, priority, title, ago))
		}
		rows.Close()
		if len(inProgress) > 0 {
			sb.WriteString("IN PROGRESS:\n")
			for i, t := range inProgress {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t))
			}
			sb.WriteString("\n")
		}
	}

	// Pending tasks
	rows, err = db.Query(
		`SELECT id, title, priority FROM tasks
		 WHERE status = 'pending' ORDER BY priority ASC, id ASC`)
	if err == nil {
		defer rows.Close()
		var pending []string
		for rows.Next() {
			var id int64
			var title string
			var priority int
			if rows.Scan(&id, &title, &priority) != nil {
				continue
			}
			pending = append(pending, fmt.Sprintf("[#%d P%d] %s", id, priority, title))
		}
		rows.Close()
		if len(pending) > 0 {
			sb.WriteString("PENDING:\n")
			for i, t := range pending {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, t))
			}
			sb.WriteString("\n")
		}
	}

	// Recently completed (last 24h)
	rows, err = db.Query(
		`SELECT id, title, completed_at FROM tasks
		 WHERE status = 'completed' AND completed_at > datetime('now', '-1 day')
		 ORDER BY completed_at DESC LIMIT 10`)
	if err == nil {
		defer rows.Close()
		var completed []string
		for rows.Next() {
			var id int64
			var title string
			var completedAt sql.NullString
			if rows.Scan(&id, &title, &completedAt) != nil {
				continue
			}
			ago := formatRelativeTime(completedAt.String, now)
			completed = append(completed, fmt.Sprintf("[#%d] %s (%s)", id, title, ago))
		}
		rows.Close()
		if len(completed) > 0 {
			sb.WriteString("RECENTLY COMPLETED (last 24h):\n")
			for _, t := range completed {
				sb.WriteString(fmt.Sprintf("- done [%s]\n", t))
			}
			sb.WriteString("\n")
		}
	}

	// If nothing active, show default directive
	if sb.Len() == 0 || !strings.Contains(sb.String(), "IN PROGRESS") && !strings.Contains(sb.String(), "PENDING") {
		if sb.Len() == 0 {
			sb.WriteString("(no active tasks)\n\n")
		}
		sb.WriteString("DEFAULT DIRECTIVE: Review your memory, purpose, and recent work. Identify a useful project to work on, plan it out, create yourself tasks, and get started.\n")
	}

	return sb.String()
}

// formatRelativeTime converts a timestamp string to a relative time description.
func formatRelativeTime(ts string, now time.Time) string {
	if ts == "" {
		return "just now"
	}
	// Try parsing SQLite timestamp formats
	var parsed time.Time
	var err error
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	} {
		parsed, err = time.Parse(layout, ts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return "recently"
	}

	diff := now.Sub(parsed)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		d := int(diff.Hours() / 24)
		if d == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", d)
	}
}
