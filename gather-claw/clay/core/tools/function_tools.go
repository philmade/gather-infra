package tools

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// --- Arg/Result types ---

type MemoryArgs struct {
	Action  string `json:"action" jsonschema:"Action: store, recall, or search"`
	Content string `json:"content,omitempty" jsonschema:"(store) Content to store"`
	Type    string `json:"type,omitempty" jsonschema:"(store) Memory type: general, continuation, etc"`
	Tags    string `json:"tags,omitempty" jsonschema:"(store) Comma-separated tags"`
	Query   string `json:"query,omitempty" jsonschema:"(search) Search query"`
	Days    int    `json:"days,omitempty" jsonschema:"(recall) Days to look back, default 7"`
}
type MemoryResult struct {
	Message  string   `json:"message,omitempty"`
	Memories []string `json:"memories,omitempty"`
	Count    int      `json:"count,omitempty"`
}

type SoulArgs struct {
	Action   string `json:"action" jsonschema:"Action: read or write"`
	Filename string `json:"filename" jsonschema:"Soul file: SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md"`
	Content  string `json:"content,omitempty" jsonschema:"(write) Content to write"`
}
type SoulResult struct {
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
}

type WebSearchArgs struct {
	Query string `json:"query" jsonschema:"Search query (sent to DuckDuckGo)"`
}
type WebFetchArgs struct {
	URL string `json:"url" jsonschema:"URL to fetch and extract content from"`
}
type ResearchResult struct{ Content string `json:"content"` }

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

type BuildRequestArgs struct {
	Reason string `json:"reason,omitempty" jsonschema:"Reason for build request"`
}
type BuildRequestResult struct {
	Message string `json:"message"`
	Output  string `json:"output,omitempty"`
}

type PlatformSearchArgs struct {
	Query    string `json:"query" jsonschema:"What you want to do (e.g. 'post to social feed', 'check inbox')"`
	Category string `json:"category,omitempty" jsonschema:"Filter: social, msg, skills, platform, claw, peer"`
}
type PlatformSearchResult struct {
	Result string `json:"result"`
}

type PlatformCallArgs struct {
	Tool   string         `json:"tool" jsonschema:"Tool ID from search results (e.g. 'social.create_post')"`
	Params map[string]any `json:"params,omitempty" jsonschema:"Tool parameters as key-value pairs"`
}
type PlatformCallResult struct {
	Result string `json:"result"`
}

type TaskArgs struct {
	Action      string `json:"action" jsonschema:"Action: add, list, start, complete, remove"`
	Title       string `json:"title,omitempty" jsonschema:"(add) Task title"`
	Description string `json:"description,omitempty" jsonschema:"(add) Task description"`
	Priority    int    `json:"priority,omitempty" jsonschema:"(add) Priority 1-5, default 3"`
	ID          int64  `json:"id,omitempty" jsonschema:"(start/complete/remove) Task ID"`
	Status      string `json:"status,omitempty" jsonschema:"(list) Filter: pending, in_progress, completed, or blank for active"`
}
type TaskResult struct {
	Message string     `json:"message,omitempty"`
	Tasks   []TaskInfo `json:"tasks,omitempty"`
}

type ExtensionListArgs struct{}
type ExtensionListResult struct {
	Extensions []string `json:"extensions"`
	Count      int      `json:"count"`
}

type ExtensionRunArgs struct {
	Name string            `json:"name" jsonschema:"Extension script name (e.g. 'hello' or 'hello.star')"`
	Args map[string]string `json:"args,omitempty" jsonschema:"Key-value arguments to pass to the script"`
}
type ExtensionRunResult struct {
	Output string `json:"output"`
}

// Truncate is a shared utility for truncating strings.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ---------------------------------------------------------------------------
// Per-agent tool constructors
// ---------------------------------------------------------------------------

// NewConsolidatedMemoryTool creates a single memory tool with store/recall/search actions.
func NewConsolidatedMemoryTool(mem *MemoryTool) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "memory",
			Description: "Persistent memory — store, recall, or search memories. Use action: 'store' (with content, type, tags), 'recall' (with days), or 'search' (with query).",
		},
		func(ctx tool.Context, args MemoryArgs) (MemoryResult, error) {
			switch args.Action {
			case "store":
				if args.Content == "" {
					return MemoryResult{}, fmt.Errorf("content is required for store")
				}
				memType := args.Type
				if memType == "" {
					memType = "general"
				}
				if err := mem.Store(args.Content, memType, args.Tags); err != nil {
					return MemoryResult{}, err
				}
				return MemoryResult{Message: fmt.Sprintf("Stored: %s", Truncate(args.Content, 50))}, nil

			case "recall":
				days := args.Days
				if days <= 0 {
					days = 7
				}
				memories, err := mem.Recall(days)
				if err != nil {
					return MemoryResult{}, err
				}
				return MemoryResult{Memories: memories, Count: len(memories)}, nil

			case "search":
				if args.Query == "" {
					return MemoryResult{}, fmt.Errorf("query is required for search")
				}
				results, err := mem.Search(args.Query)
				if err != nil {
					return MemoryResult{}, err
				}
				return MemoryResult{Memories: results, Count: len(results)}, nil

			default:
				return MemoryResult{}, fmt.Errorf("unknown action %q — use store, recall, or search", args.Action)
			}
		},
	)
}

// NewConsolidatedSoulTool creates a single soul tool with read/write actions.
func NewConsolidatedSoulTool(soul *SoulTool) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "soul",
			Description: "Read or write soul identity files. Use action: 'read' (with filename) or 'write' (with filename, content). Files: SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md.",
		},
		func(ctx tool.Context, args SoulArgs) (SoulResult, error) {
			switch args.Action {
			case "read":
				if args.Filename == "" {
					return SoulResult{}, fmt.Errorf("filename is required for read")
				}
				content, err := soul.Read(args.Filename)
				if err != nil {
					return SoulResult{}, err
				}
				return SoulResult{Content: content}, nil

			case "write":
				if args.Filename == "" {
					return SoulResult{}, fmt.Errorf("filename is required for write")
				}
				if args.Content == "" {
					return SoulResult{}, fmt.Errorf("content is required for write")
				}
				if err := soul.Write(args.Filename, args.Content); err != nil {
					return SoulResult{}, err
				}
				return SoulResult{Message: fmt.Sprintf("Updated %s", args.Filename)}, nil

			default:
				return SoulResult{}, fmt.Errorf("unknown action %q — use read or write", args.Action)
			}
		},
	)
}

// NewResearchTools creates web_search and webfetch tools for research agents.
func NewResearchTools() ([]tool.Tool, error) {
	research := NewResearchTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "web_search", Description: "Search the web via DuckDuckGo. Returns search results as text."},
		func(ctx tool.Context, args WebSearchArgs) (ResearchResult, error) {
			if args.Query == "" {
				return ResearchResult{}, fmt.Errorf("query is required")
			}
			content, err := research.Research(args.Query)
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

	t, err = functiontool.New(
		functiontool.Config{Name: "webfetch", Description: "Fetch a specific URL and extract its content as text."},
		func(ctx tool.Context, args WebFetchArgs) (ResearchResult, error) {
			if args.URL == "" {
				return ResearchResult{}, fmt.Errorf("url is required")
			}
			content, err := research.Research(args.URL)
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

// NewOrchestratorTools creates read-only filesystem tools for the orchestrator:
// read, search, bash — no write, edit, or build.
func NewOrchestratorTools() ([]tool.Tool, error) {
	fs := NewFSTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "read", Description: "Read a file or list a directory"},
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
		functiontool.Config{Name: "search", Description: "Search for files matching a glob pattern"},
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
		functiontool.Config{Name: "bash", Description: "Run a bash command"},
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

	return out, nil
}

// NewClaudeTools creates the claude sub-agent's tools: filesystem ops + build.
func NewClaudeTools() ([]tool.Tool, error) {
	fs := NewFSTool()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{Name: "read", Description: "Read a file or list a directory"},
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
		functiontool.Config{Name: "write", Description: "Write content to a file (creates directories as needed)"},
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
		functiontool.Config{Name: "edit", Description: "Find and replace text in a file"},
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
		functiontool.Config{Name: "bash", Description: "Run a bash command"},
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
		functiontool.Config{Name: "search", Description: "Search for files matching a glob pattern"},
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

	// Build tool — compile and hot-swap
	buildTools, err := NewBuildTools()
	if err != nil {
		return nil, err
	}
	out = append(out, buildTools...)

	return out, nil
}

// NewExtensionTools creates Starlark extension management tools for the coordinator.
func NewExtensionTools() ([]tool.Tool, error) {
	runner := NewStarlarkRunner()
	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{
			Name:        "extension_list",
			Description: "List available Starlark extension scripts in /app/data/extensions/",
		},
		func(ctx tool.Context, args ExtensionListArgs) (ExtensionListResult, error) {
			exts, err := runner.List()
			if err != nil {
				return ExtensionListResult{}, err
			}
			return ExtensionListResult{Extensions: exts, Count: len(exts)}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{
			Name:        "extension_run",
			Description: "Run a Starlark (.star) extension script by name with optional arguments. Starlark is embedded Python — scripts can call http_get, read_file, write_file, and more.",
		},
		func(ctx tool.Context, args ExtensionRunArgs) (ExtensionRunResult, error) {
			if args.Name == "" {
				return ExtensionRunResult{}, fmt.Errorf("name is required")
			}
			output, err := runner.Run(args.Name, args.Args)
			if err != nil {
				return ExtensionRunResult{Output: err.Error()}, err
			}
			return ExtensionRunResult{Output: output}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}

// NewConsolidatedTaskTool creates a single task tool with add/list/start/complete/remove actions.
func NewConsolidatedTaskTool(tt *TaskTool) (tool.Tool, error) {
	return functiontool.New(
		functiontool.Config{
			Name:        "tasks",
			Description: "Structured task management — add, list, start, complete, or remove tasks. Use action: 'add' (with title, description?, priority?), 'list' (with status?), 'start' (with id), 'complete' (with id), 'remove' (with id).",
		},
		func(ctx tool.Context, args TaskArgs) (TaskResult, error) {
			switch args.Action {
			case "add":
				if args.Title == "" {
					return TaskResult{}, fmt.Errorf("title is required for add")
				}
				priority := args.Priority
				if priority == 0 {
					priority = 3
				}
				id, err := tt.Add(args.Title, args.Description, priority)
				if err != nil {
					return TaskResult{}, err
				}
				return TaskResult{Message: fmt.Sprintf("Created task #%d: %s (P%d)", id, args.Title, priority)}, nil

			case "list":
				tasks, err := tt.List(args.Status)
				if err != nil {
					return TaskResult{}, err
				}
				if len(tasks) == 0 {
					return TaskResult{Message: "No tasks found"}, nil
				}
				return TaskResult{Tasks: tasks, Message: fmt.Sprintf("%d task(s)", len(tasks))}, nil

			case "start":
				if args.ID == 0 {
					return TaskResult{}, fmt.Errorf("id is required for start")
				}
				if err := tt.Start(args.ID); err != nil {
					return TaskResult{}, err
				}
				return TaskResult{Message: fmt.Sprintf("Started task #%d", args.ID)}, nil

			case "complete":
				if args.ID == 0 {
					return TaskResult{}, fmt.Errorf("id is required for complete")
				}
				if err := tt.Complete(args.ID); err != nil {
					return TaskResult{}, err
				}
				return TaskResult{Message: fmt.Sprintf("Completed task #%d", args.ID)}, nil

			case "remove":
				if args.ID == 0 {
					return TaskResult{}, fmt.Errorf("id is required for remove")
				}
				if err := tt.Remove(args.ID); err != nil {
					return TaskResult{}, err
				}
				return TaskResult{Message: fmt.Sprintf("Removed task #%d", args.ID)}, nil

			default:
				return TaskResult{}, fmt.Errorf("unknown action %q — use add, list, start, complete, or remove", args.Action)
			}
		},
	)
}

// NewPlatformTools creates platform_search and platform_call tools for the coordinator.
// Returns nil slice if platform credentials are not configured (GATHER_PRIVATE_KEY not set).
func NewPlatformTools() ([]tool.Tool, error) {
	p := NewPlatformTool()
	if p == nil {
		return nil, nil // platform tools disabled — no credentials
	}

	var out []tool.Tool

	t, err := functiontool.New(
		functiontool.Config{
			Name:        "platform_search",
			Description: "Search for available Gather platform tools (social feed, messaging, skills, inter-claw). Returns matching tools with their parameters. Use this to discover what you can do on the platform.",
		},
		func(ctx tool.Context, args PlatformSearchArgs) (PlatformSearchResult, error) {
			if args.Query == "" {
				return PlatformSearchResult{}, fmt.Errorf("query is required")
			}
			result, err := p.Search(args.Query, args.Category)
			if err != nil {
				return PlatformSearchResult{}, err
			}
			return PlatformSearchResult{Result: result}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	t, err = functiontool.New(
		functiontool.Config{
			Name:        "platform_call",
			Description: "Execute a Gather platform tool by ID. Use platform_search first to find the tool and its parameters.",
		},
		func(ctx tool.Context, args PlatformCallArgs) (PlatformCallResult, error) {
			if args.Tool == "" {
				return PlatformCallResult{}, fmt.Errorf("tool is required")
			}
			if args.Params == nil {
				args.Params = make(map[string]any)
			}
			result, err := p.Call(args.Tool, args.Params)
			if err != nil {
				return PlatformCallResult{}, err
			}
			return PlatformCallResult{Result: result}, nil
		},
	)
	if err != nil {
		return nil, err
	}
	out = append(out, t)

	return out, nil
}
