package tools

import (
	"context"
	"fmt"

	"google.golang.org/adk/tool"
)

// MemoryADKTool wraps MemoryTool as an ADK tool
type MemoryADKTool struct {
	mem *MemoryTool
}

func NewMemoryADKTool(mem *MemoryTool) *MemoryADKTool {
	return &MemoryADKTool{mem: mem}
}

func (t *MemoryADKTool) Name() string {
	return "memory"
}

func (t *MemoryADKTool) Description() string {
	return "Store and retrieve memories from SQLite database"
}

func (t *MemoryADKTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"store", "recall", "search"},
				"description": "Action to perform",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content to store (for store action)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for search action)",
			},
			"days": map[string]interface{}{
				"type":        "integer",
				"description": "Days to recall (for recall action, default 7)",
			},
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Memory type (default: general)",
			},
			"tags": map[string]interface{}{
				"type":        "string",
				"description": "Comma-separated tags",
			},
		},
		"required": []string{"action"},
	}
}

func (t *MemoryADKTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)

	switch action {
	case "store":
		content, _ := args["content"].(string)
		memType, _ := args["type"].(string)
		if memType == "" {
			memType = "general"
		}
		tags, _ := args["tags"].(string)
		
		if err := t.mem.Store(content, memType, tags); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ Stored memory: %s", truncate(content, 50)), nil

	case "recall":
		days := 7
		if d, ok := args["days"].(float64); ok {
			days = int(d)
		}
		
		memories, err := t.mem.Recall(days)
		if err != nil {
			return "", err
		}
		
		if len(memories) == 0 {
			return "No recent memories found", nil
		}
		
		result := "Recent memories:\n"
		for i, m := range memories {
			result += fmt.Sprintf("%d. %s\n", i+1, m)
		}
		return result, nil

	case "search":
		query, _ := args["query"].(string)
		if query == "" {
			return "", fmt.Errorf("query required for search")
		}
		
		results, err := t.mem.Search(query)
		if err != nil {
			return "", err
		}
		
		if len(results) == 0 {
			return fmt.Sprintf("No memories found matching: %s", query), nil
		}
		
		result := fmt.Sprintf("Found %d memories:\n", len(results))
		for i, r := range results {
			result += fmt.Sprintf("%d. %s\n", i+1, r)
		}
		return result, nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// FSADKTool wraps FSTool as an ADK tool
type FSADKTool struct {
	fs *FSTool
}

func NewFSADKTool() *FSADKTool {
	return &FSADKTool{fs: NewFSTool()}
}

func (t *FSADKTool) Name() string {
	return "fs"
}

func (t *FSADKTool) Description() string {
	return "File system operations: read, write, edit, bash, search"
}

func (t *FSADKTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"read", "write", "edit", "bash", "search"},
				"description": "Action to perform",
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path (for read/write/edit)",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content (for write)",
			},
			"old_text": map[string]interface{}{
				"type":        "string",
				"description": "Text to replace (for edit)",
			},
			"new_text": map[string]interface{}{
				"type":        "string",
				"description": "Replacement text (for edit)",
			},
			"command": map[string]interface{}{
				"type":        "string",
				"description": "Bash command to run",
			},
			"pattern": map[string]interface{}{
				"type":        "string",
				"description": "Glob pattern (for search)",
			},
		},
		"required": []string{"action"},
	}
}

func (t *FSADKTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)

	switch action {
	case "read":
		path, _ := args["path"].(string)
		if path == "" {
			return "", fmt.Errorf("path required")
		}
		return t.fs.Read(path)

	case "write":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" || content == "" {
			return "", fmt.Errorf("path and content required")
		}
		if err := t.fs.Write(path, content); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ Wrote %d bytes to %s", len(content), path), nil

	case "edit":
		path, _ := args["path"].(string)
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		if path == "" || oldText == "" || newText == "" {
			return "", fmt.Errorf("path, old_text, and new_text required")
		}
		if err := t.fs.Edit(path, oldText, newText); err != nil {
			return "", err
		}
		return fmt.Sprintf("✓ Edited %s", path), nil

	case "bash":
		command, _ := args["command"].(string)
		if command == "" {
			return "", fmt.Errorf("command required")
		}
		return t.fs.Bash(command)

	case "search":
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return "", fmt.Errorf("pattern required")
		}
		matches, err := t.fs.Search(pattern)
		if err != nil {
			return "", err
		}
		result := fmt.Sprintf("Found %d files:\n", len(matches))
		for i, m := range matches {
			result += fmt.Sprintf("%d. %s\n", i+1, m)
		}
		return result, nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// SkillADKTool wraps SkillTool as an ADK tool
type SkillADKTool struct {
	skills *SkillTool
}

func NewSkillADKTool() *SkillADKTool {
	return &SkillADKTool{skills: NewSkillTool()}
}

func (t *SkillADKTool) Name() string {
	return "skill"
}

func (t *SkillADKTool) Description() string {
	return "Find and run dynamic skills"
}

func (t *SkillADKTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"find", "run"},
				"description": "Action to perform",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Search query (for find)",
			},
			"skill_name": map[string]interface{}{
				"type":        "string",
				"description": "Skill name (for run)",
			},
			"args": map[string]interface{}{
				"type":        "object",
				"description": "Skill arguments",
			},
		},
		"required": []string{"action"},
	}
}

func (t *SkillADKTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	action, _ := args["action"].(string)

	switch action {
	case "find":
		query, _ := args["query"].(string)
		skills, err := t.skills.Find(query)
		if err != nil {
			return "", err
		}
		if len(skills) == 0 {
			return "No skills found", nil
		}
		result := fmt.Sprintf("Found %d skills:\n", len(skills))
		for i, s := range skills {
			result += fmt.Sprintf("%d. %s\n", i+1, s)
		}
		return result, nil

	case "run":
		name, _ := args["skill_name"].(string)
		if name == "" {
			return "", fmt.Errorf("skill_name required")
		}
		
		skillArgs, _ := args["args"].(map[string]interface{})
		result, err := t.skills.Run(name, skillArgs)
		if err != nil {
			return "", err
		}
		return result, nil

	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

// IsLongRunning indicates these are not long-running operations
func (t *MemoryADKTool) IsLongRunning() bool { return false }
func (t *FSADKTool) IsLongRunning() bool { return false }
func (t *SkillADKTool) IsLongRunning() bool { return false }

// Verify interfaces are satisfied
var _ tool.Tool = (*MemoryADKTool)(nil)
var _ tool.Tool = (*FSADKTool)(nil)
var _ tool.Tool = (*SkillADKTool)(nil)

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
