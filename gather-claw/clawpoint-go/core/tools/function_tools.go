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

type ResearchArgs struct {
	Query string `json:"query,omitempty" jsonschema:"Search query or URL to research"`
	URL   string `json:"url,omitempty" jsonschema:"Specific URL to fetch"`
}
type ResearchResult struct{ Content string `json:"content"` }

type ClaudeCodeArgs struct {
	Task       string `json:"task" jsonschema:"Task description for Claude Code"`
	WorkingDir string `json:"working_dir,omitempty" jsonschema:"Working directory (default: current)"`
}
type ClaudeCodeResult struct{ Output string `json:"output"` }

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

// NewClaudeTools creates Claude Code sub-agent tools, including build_and_deploy.
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

	// Also give claude the build tool
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
