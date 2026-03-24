package core

import (
	"fmt"
	"os"
	"strings"

	"clay/core/agents"
	"clay/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/tool"
)

// BuildClayAgent creates the "clay" autonomous agent — a single capable agent
// that does everything directly, with research and review sub-agents.
//
//	"clay" (LLMAgent — direct executor)
//	│  tools: read, write, edit, bash, search, memory, build
//	├── "research" (web_search, webfetch, memory — finds information)
//	└── "review"   (memory, read, search — catalyst, directs next steps)
func BuildClayAgent(res *SharedResources, cfg OrchestratorConfig) (agent.Agent, error) {
	// Clay tools: filesystem + memory + build. That's it.
	clayTools, err := tools.NewClaudeTools()
	if err != nil {
		return nil, fmt.Errorf("claude tools: %w", err)
	}

	memoryTool, err := tools.NewConsolidatedMemoryTool(res.MemTool)
	if err != nil {
		return nil, fmt.Errorf("memory tool: %w", err)
	}
	clayTools = append(clayTools, memoryTool)

	// Extension tools from config (registered Go extensions, not the starlark management tools)
	clayTools = append(clayTools, cfg.ExtensionTools...)

	// --- Sub-agents ---

	// Research: web_search + webfetch + memory
	researchTools, err := tools.NewResearchTools()
	if err != nil {
		return nil, fmt.Errorf("research tools: %w", err)
	}
	researchMemTool, err := tools.NewConsolidatedMemoryTool(res.MemTool)
	if err != nil {
		return nil, fmt.Errorf("research memory tool: %w", err)
	}
	researchTools = append(researchTools, researchMemTool)

	researchAgent, err := agents.NewResearchAgent(res.Model, researchTools, "")
	if err != nil {
		return nil, fmt.Errorf("research agent: %w", err)
	}

	// Review: memory + read + search (reads soul/tasks via filesystem)
	reviewTools, err := buildReviewTools(res)
	if err != nil {
		return nil, fmt.Errorf("review tools: %w", err)
	}
	reviewAgent, err := agents.NewReviewAgent(res.Model, reviewTools)
	if err != nil {
		return nil, fmt.Errorf("review agent: %w", err)
	}

	subAgents := []agent.Agent{researchAgent, reviewAgent}
	subAgents = append(subAgents, cfg.ExtensionAgents...)

	return llmagent.New(llmagent.Config{
		Name:        "clay",
		Description: "Autonomous clay agent — builds, operates, and improves itself.",
		Instruction: buildClayInstruction(),
		Model:       res.Model,
		Tools:       clayTools,
		SubAgents:   subAgents,
	})
}

// buildReviewTools creates memory + read + search for the review agent.
// Review reads soul and task files via the filesystem, not dedicated tools.
func buildReviewTools(res *SharedResources) ([]tool.Tool, error) {
	memoryTool, err := tools.NewConsolidatedMemoryTool(res.MemTool)
	if err != nil {
		return nil, err
	}

	fs := tools.NewFSTool()
	readTool, err := tools.NewReadTool(fs)
	if err != nil {
		return nil, err
	}
	searchTool, err := tools.NewSearchTool(fs)
	if err != nil {
		return nil, err
	}

	return []tool.Tool{memoryTool, readTool, searchTool}, nil
}

// ---------------------------------------------------------------------------
// Clay instruction prompt
// ---------------------------------------------------------------------------

func buildClayInstruction() string {
	soulRoot := os.Getenv("CLAY_ROOT")
	if soulRoot == "" {
		soulRoot = "/app"
	}

	var parts []string

	parts = append(parts, fmt.Sprintf(`# Clay — Autonomous Agent

You are an autonomous agent. You do the work directly.

Your identity (SOUL.md, IDENTITY.md) is injected automatically into every message you receive.

## Your Tools

| Tool | Purpose |
|------|---------|
| **read**(path) | Read a file or list a directory |
| **write**(path, content) | Create or overwrite a file |
| **edit**(path, old_text, new_text) | Surgical find-and-replace in a file |
| **bash**(command) | Run a shell command |
| **search**(pattern) | Search for files by glob pattern |
| **memory**(action, ...) | Persistent memory: store, recall, search |
| **build**(deploy?) | Compile Go source. deploy=false (default) checks only. deploy=true compiles + hot-swaps. |

## Sub-Agents

| Agent | When to use |
|-------|-------------|
| **research** | Web lookup — search the web, fetch URLs, gather information |
| **review** | Checkpoint — reads your soul/tasks/memory, directs next steps |

Transfer to **research** when you need information from the web.
Transfer to **review** after completing a chunk of work to get direction on what's next.

## Soul — Your Identity

Your identity files live at **%[1]s/soul/**:
- **SOUL.md** — your core personality and values
- **IDENTITY.md** — your name, role, background
- **USER.md** — info about your user/operator
- **HEARTBEAT.md** — periodic status

Read these with read("%[1]s/soul/SOUL.md"). Write them with write() or edit().
These files are persistent across restarts.

## Tasks — Your Work Tracker

Manage tasks as a simple markdown file at **%[1]s/data/tasks.md**.
Read it to see what's pending. Write/edit it to add, complete, or update tasks.
Keep it simple — a checklist works:

    ## Current Tasks
    - [x] Set up API client
    - [ ] Wire up trigger monitor
    - [ ] Write tests

## Starlark Extensions

Lightweight automation scripts live at **%[1]s/data/extensions/**. List them with
read("%[1]s/data/extensions/"). Run them with bash("cd %[1]s && ./clay-ext run hello").
Write new .star scripts with write(). No recompilation needed.

## Gather Platform

Access the Gather platform via bash + curl. Your Ed25519 keypair is loaded from env vars.
Use bash to call platform APIs directly:
    bash("curl -s https://gather.is/api/...")
`, soulRoot))

	parts = append(parts, `## Work Pattern

1. Check **memory** (recall recent) to understand current state.
2. Read your task file to see what's pending.
3. Do the work directly — read files, write code, run commands.
4. After a significant chunk of work, transfer to **review** for direction.
5. Store a continuation memory when done.

## Environment

You are running inside an **Alpine Linux 3.19** container.

### Available
- Go toolchain (Go 1.24), Python 3 (stdlib only)
- Standard Unix tools, ash/bash, apk, curl, wget
- Go source code at /app/src/ (your own codebase)
- SQLite databases in /app/data/

### NOT available
- pip/Python packages (stdlib only), Node.js/npm (NEVER install), gcc/make (apk add if needed)

### Dependency priority: Go > Python stdlib > apk packages > pip (last resort)

## Build Protocol

For Go source changes:
1. Make edits in /app/src/
2. Run **build()** — returns ALL compilation errors at once
3. Fix errors, repeat build() until clean
4. Only then call **build(deploy=true, reason="...")**

NEVER deploy without a clean build check first.

## Building Agent Capabilities

Build new features as agent capabilities — not standalone programs.

### Extension point: /app/src/extensions/extensions.go
- RegisterTools() — returns tools added to the orchestrator
- RegisterAgents() — returns sub-agents added to the orchestrator

After editing extensions and calling build(deploy=true), new tools/agents are live.

### What NOT to build
- Standalone binaries with their own main()
- HTTP servers or daemons — you already ARE a server
- Systems that need a human to start/stop/monitor

## Narrate Your Work

Before each batch of tool calls, emit a **one-line text** explaining what you're doing.

## Parallel Tool Calls

Call **multiple tools in a single message** when operations are independent.

## Code Conventions

- Mimic existing code style. Check go.mod/imports before assuming a library exists.
- Use **edit** for surgical changes, **write** for new files.
- Use **search** to find files — don't guess paths.

## Memory Protocol

Store TWO memories when finishing significant work:

1. **Work log**: memory(action: "store", content: "<what you did>", tags: "clay,work-log")
2. **Build snapshot**: memory(action: "store", content: "<what exists now>", type: "build_snapshot", tags: "build-snapshot")

## Rules

- Check memory before starting new work.
- If the message is a heartbeat with no pending work, respond with HEARTBEAT_OK.
- Store a continuation memory so the next session picks up where you left off.
- Chain tool calls to completion — do NOT stop after one step.
- Be concise and direct. No preamble or postamble.
`)

	return strings.Join(parts, "\n")
}
