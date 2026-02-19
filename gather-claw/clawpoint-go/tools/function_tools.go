package tools

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Arg/Result types ---

type MemoryStoreArgs struct {
	Content string `json:"content" jsonschema:"Content to store"`
	Type    string `json:"type,omitempty" jsonschema:"Memory type (default: general)"`
	Tags    string `json:"tags,omitempty" jsonschema:"Comma-separated tags"`
}
type MemoryStoreResult struct{ Message string `json:"message"` }

type MemoryRecallArgs struct {
	Days int `json:"days,omitempty" jsonschema:"Days to recall (default 7)"`
}
type MemoryRecallResult struct {
	Memories []string `json:"memories"`
	Count    int      `json:"count"`
}

type MemorySearchArgs struct {
	Query string `json:"query" jsonschema:"Search query"`
}
type MemorySearchResult struct {
	Results []string `json:"results"`
	Count   int      `json:"count"`
}

type FSReadArgs struct {
	Path string `json:"path" jsonschema:"File or directory path to read"`
}
type FSReadResult struct{ Content string `json:"content"` }

type FSWriteArgs struct {
	Path    string `json:"path" jsonschema:"File path to write"`
	Content string `json:"content" jsonschema:"Content to write"`
}
type FSWriteResult struct{ Message string `json:"message"` }

type FSEditArgs struct {
	Path    string `json:"path" jsonschema:"File path to edit"`
	OldText string `json:"old_text" jsonschema:"Text to find and replace"`
	NewText string `json:"new_text" jsonschema:"Replacement text"`
}
type FSEditResult struct{ Message string `json:"message"` }

type FSBashArgs struct {
	Command string `json:"command" jsonschema:"Bash command to run"`
}
type FSBashResult struct{ Output string `json:"output"` }

type FSSearchArgs struct {
	Pattern string `json:"pattern" jsonschema:"Glob pattern to match files"`
}
type FSSearchResult struct {
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

type SkillFindArgs struct {
	Query string `json:"query,omitempty" jsonschema:"Search query to filter skills"`
}
type SkillFindResult struct {
	Skills []string `json:"skills"`
	Count  int      `json:"count"`
}

type SkillRunArgs struct {
	SkillName string         `json:"skill_name" jsonschema:"Name of the skill to run"`
	Args      map[string]any `json:"args,omitempty" jsonschema:"Arguments to pass to the skill"`
}
type SkillRunResult struct{ Output string `json:"output"` }

type ResearchArgs struct {
	Query string `json:"query,omitempty" jsonschema:"Search query or URL to research"`
	URL   string `json:"url,omitempty" jsonschema:"Specific URL to fetch"`
}
type ResearchResult struct{ Content string `json:"content"` }

type SoulReadArgs struct {
	Filename string `json:"filename" jsonschema:"Soul file to read: SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md, BOOTSTRAP.md"`
}
type SoulReadResult struct{ Content string `json:"content"` }

type SoulWriteArgs struct {
	Filename string `json:"filename" jsonschema:"Soul file to write: SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md"`
	Content  string `json:"content" jsonschema:"Content to write"`
}
type SoulWriteResult struct{ Message string `json:"message"` }

type ClaudeCodeArgs struct {
	Task       string `json:"task" jsonschema:"Task description for Claude Code"`
	WorkingDir string `json:"working_dir,omitempty" jsonschema:"Working directory (default: current)"`
}
type ClaudeCodeResult struct{ Output string `json:"output"` }

type SelfBuildArgs struct {
	Reason string `json:"reason,omitempty" jsonschema:"Reason for self-build"`
}
type SelfBuildResult struct {
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

// ---------------------------------------------------------------------------
// Per-agent tool constructors
// ---------------------------------------------------------------------------

// NewMemoryTools creates memory sub-agent tools.
func NewMemoryTools(mem *MemoryTool) ([]tool.Tool, error) {
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "memory_store", Description: "Store a memory in the persistent database"},
		func(ctx tool.Context, args MemoryStoreArgs) (MemoryStoreResult, error) {
			memType := args.Type
			if memType == "" {
				memType = "general"
			}
			if err := mem.Store(args.Content, memType, args.Tags); err != nil {
				return MemoryStoreResult{}, err
			}
			return MemoryStoreResult{Message: fmt.Sprintf("Stored: %s", truncate(args.Content, 50))}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "memory_recall", Description: "Recall recent memories from the database"},
		func(ctx tool.Context, args MemoryRecallArgs) (MemoryRecallResult, error) {
			days := args.Days
			if days <= 0 {
				days = 7
			}
			memories, err := mem.Recall(days)
			if err != nil {
				return MemoryRecallResult{}, err
			}
			return MemoryRecallResult{Memories: memories, Count: len(memories)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "memory_search", Description: "Search memories by keyword"},
		func(ctx tool.Context, args MemorySearchArgs) (MemorySearchResult, error) {
			if args.Query == "" {
				return MemorySearchResult{}, fmt.Errorf("query is required")
			}
			results, err := mem.Search(args.Query)
			if err != nil {
				return MemorySearchResult{}, err
			}
			return MemorySearchResult{Results: results, Count: len(results)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewSoulTools creates soul sub-agent tools.
func NewSoulTools(soul *SoulTool) ([]tool.Tool, error) {
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "soul_read", Description: "Read a soul identity file"},
		func(ctx tool.Context, args SoulReadArgs) (SoulReadResult, error) {
			content, err := soul.Read(args.Filename)
			if err != nil {
				return SoulReadResult{}, err
			}
			return SoulReadResult{Content: content}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "soul_write", Description: "Update a soul identity file"},
		func(ctx tool.Context, args SoulWriteArgs) (SoulWriteResult, error) {
			if err := soul.Write(args.Filename, args.Content); err != nil {
				return SoulWriteResult{}, err
			}
			return SoulWriteResult{Message: fmt.Sprintf("Updated %s", args.Filename)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewCodingTools creates coding sub-agent tools (pi equivalent).
func NewCodingTools() ([]tool.Tool, error) {
	fs := NewFSTool()
	skills := NewSkillTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "fs_read", Description: "Read a file or list a directory"},
		func(ctx tool.Context, args FSReadArgs) (FSReadResult, error) {
			content, err := fs.Read(args.Path)
			if err != nil {
				return FSReadResult{}, err
			}
			return FSReadResult{Content: content}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "fs_write", Description: "Write content to a file"},
		func(ctx tool.Context, args FSWriteArgs) (FSWriteResult, error) {
			if err := fs.Write(args.Path, args.Content); err != nil {
				return FSWriteResult{}, err
			}
			return FSWriteResult{Message: fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), args.Path)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "fs_edit", Description: "Find and replace text in a file"},
		func(ctx tool.Context, args FSEditArgs) (FSEditResult, error) {
			if err := fs.Edit(args.Path, args.OldText, args.NewText); err != nil {
				return FSEditResult{}, err
			}
			return FSEditResult{Message: fmt.Sprintf("Edited %s", args.Path)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "fs_bash", Description: "Run a bash command"},
		func(ctx tool.Context, args FSBashArgs) (FSBashResult, error) {
			output, err := fs.Bash(args.Command)
			if err != nil {
				return FSBashResult{Output: output}, err
			}
			return FSBashResult{Output: output}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "fs_search", Description: "Search for files matching a glob pattern"},
		func(ctx tool.Context, args FSSearchArgs) (FSSearchResult, error) {
			matches, err := fs.Search(args.Pattern)
			if err != nil {
				return FSSearchResult{}, err
			}
			return FSSearchResult{Matches: matches, Count: len(matches)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "skill_find", Description: "Find available dynamic skills"},
		func(ctx tool.Context, args SkillFindArgs) (SkillFindResult, error) {
			found, err := skills.Find(args.Query)
			if err != nil {
				return SkillFindResult{}, err
			}
			return SkillFindResult{Skills: found, Count: len(found)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{Name: "skill_run", Description: "Run a dynamic skill by name"},
		func(ctx tool.Context, args SkillRunArgs) (SkillRunResult, error) {
			output, err := skills.Run(args.SkillName, args.Args)
			if err != nil {
				return SkillRunResult{}, err
			}
			return SkillRunResult{Output: output}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewResearchTools creates research sub-agent tools.
func NewResearchTools() ([]tool.Tool, error) {
	research := NewResearchTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "research", Description: "Search the web or fetch a URL via Chawan browser"},
		func(ctx tool.Context, args ResearchArgs) (ResearchResult, error) {
			query := args.Query
			if args.URL != "" {
				query = args.URL
			}
			if query == "" {
				return ResearchResult{}, fmt.Errorf("query or url is required")
			}
			content, err := research.Research(query)
			if err != nil {
				return ResearchResult{}, err
			}
			return ResearchResult{Content: content}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewClaudeTools creates Claude Code sub-agent tools.
func NewClaudeTools() ([]tool.Tool, error) {
	claude := NewClaudeTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "claude_code", Description: "Run a task via Claude Code CLI. Use for complex coding tasks, multi-file refactors, or anything needing heavy lifting."},
		func(ctx tool.Context, args ClaudeCodeArgs) (ClaudeCodeResult, error) {
			output, err := claude.Run(args.Task, args.WorkingDir)
			if err != nil {
				return ClaudeCodeResult{}, err
			}
			return ClaudeCodeResult{Output: output}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewSelfBuildTools creates self-modification tools (direct on root agent).
func NewSelfBuildTools() ([]tool.Tool, error) {
	sb := NewSelfBuildTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "self_build", Description: "Compile yourself and restart with the new binary"},
		func(ctx tool.Context, args SelfBuildArgs) (SelfBuildResult, error) {
			output, buildErr := sb.Build()
			if buildErr != nil {
				return SelfBuildResult{Message: "Build failed", Output: output}, buildErr
			}
			if err := sb.Reboot(); err != nil {
				return SelfBuildResult{}, err
			}
			return SelfBuildResult{Message: "Build succeeded, rebooting...", Output: output}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}
