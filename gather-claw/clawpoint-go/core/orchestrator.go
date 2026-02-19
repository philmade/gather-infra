package core

import (
	"context"
	"fmt"
	"os"
	"strings"

	"clawpoint-go/core/agents"
	"clawpoint-go/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// OrchestratorConfig configures the coordinator agent.
type OrchestratorConfig struct {
	// ExtensionTools are additional tools registered by extensions,
	// added to the coordinator's direct tool set.
	ExtensionTools []tool.Tool

	// ExtensionAgents are additional sub-agents registered by extensions.
	ExtensionAgents []agent.Agent
}

// BuildOrchestrator creates the full ClawPoint coordinator agent with all
// sub-agents wired up. Returns the coordinator and a cleanup function.
func BuildOrchestrator(ctx context.Context, cfg OrchestratorConfig) (agent.Agent, func(), error) {
	llm, err := CreateModel(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("create model: %w", err)
	}

	// Initialize shared resources
	dbPath := getEnv("CLAWPOINT_DB", "../messages.db")
	memTool, err := tools.NewMemoryTool(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("memory tool: %w", err)
	}
	cleanup := func() { memTool.Close() }

	soul := tools.NewSoulTool()

	// Build sub-agents
	subAgents, err := buildSubAgents(llm, memTool, soul)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("sub-agents: %w", err)
	}

	// Add extension agents
	subAgents = append(subAgents, cfg.ExtensionAgents...)

	// Build coordinator tools (build_and_deploy + any extension tools)
	coordinatorTools, err := buildCoordinatorTools()
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("coordinator tools: %w", err)
	}
	coordinatorTools = append(coordinatorTools, cfg.ExtensionTools...)

	// Build coordinator instruction
	instruction := buildInstruction(soul, cfg)

	coordinator, err := llmagent.New(llmagent.Config{
		Name:        "clawpoint",
		Description: "ClawPoint-Go orchestrator — delegates to specialized sub-agents.",
		Instruction: instruction,
		Model:       llm,
		Tools:       coordinatorTools,
		SubAgents:   subAgents,
	})
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("coordinator: %w", err)
	}

	return coordinator, cleanup, nil
}

func buildSubAgents(llm model.LLM, memTool *tools.MemoryTool, soul *tools.SoulTool) ([]agent.Agent, error) {
	memoryTools, err := tools.NewMemoryTools(memTool)
	if err != nil {
		return nil, fmt.Errorf("memory tools: %w", err)
	}
	memoryAgent, err := agents.NewMemoryAgent(llm, memoryTools)
	if err != nil {
		return nil, fmt.Errorf("memory agent: %w", err)
	}

	soulTools, err := tools.NewSoulTools(soul)
	if err != nil {
		return nil, fmt.Errorf("soul tools: %w", err)
	}
	soulAgent, err := agents.NewSoulAgent(llm, soulTools)
	if err != nil {
		return nil, fmt.Errorf("soul agent: %w", err)
	}

	codingTools, err := tools.NewCodingTools()
	if err != nil {
		return nil, fmt.Errorf("coding tools: %w", err)
	}
	codingAgent, err := agents.NewCodingAgent(llm, codingTools)
	if err != nil {
		return nil, fmt.Errorf("coding agent: %w", err)
	}

	claudeTools, err := tools.NewClaudeTools()
	if err != nil {
		return nil, fmt.Errorf("claude tools: %w", err)
	}
	claudeAgent, err := agents.NewClaudeAgent(llm, claudeTools)
	if err != nil {
		return nil, fmt.Errorf("claude agent: %w", err)
	}

	researchTools, err := tools.NewResearchTools()
	if err != nil {
		return nil, fmt.Errorf("research tools: %w", err)
	}
	researchAgent, err := agents.NewResearchAgent(llm, researchTools)
	if err != nil {
		return nil, fmt.Errorf("research agent: %w", err)
	}

	return []agent.Agent{memoryAgent, soulAgent, codingAgent, claudeAgent, researchAgent}, nil
}

func buildCoordinatorTools() ([]tool.Tool, error) {
	return tools.NewBuildTools()
}

func buildInstruction(soul *tools.SoulTool, cfg OrchestratorConfig) string {
	var parts []string
	parts = append(parts, `You are ClawPoint-Go, an autonomous orchestrator agent.

Your identity and context are pre-loaded below.
`)

	// Load soul sections
	for _, f := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "HEARTBEAT.md"} {
		if section := soul.LoadSection(f); section != "" {
			parts = append(parts, section)
		}
	}

	// Read version
	version := "unknown"
	if v, err := os.ReadFile(coreVersionPath()); err == nil {
		version = strings.TrimSpace(string(v))
	}

	parts = append(parts, fmt.Sprintf(`---

## Core version

You are running core %s. Read core/VERSION for details.

## Architecture: Core / Extensions

Your codebase has two parts:
- **core/** — Versioned infrastructure. Read it to understand yourself, but do NOT modify it.
  Changes to core are done by your operator.
- **extensions/** — Your code. Add tools, sub-agents, and configs here. You own this directory.

When you want to add new capabilities:
1. Write code in extensions/ (via pi sub-agent)
2. Call build_and_deploy to compile and restart with the new code
3. If the build fails, you get the error output — fix and retry

## How you work

You are a **multi-agent orchestrator**. You delegate to specialized sub-agents.
They do the work and transfer back to you.

## Your sub-agents

**memory** — persistent memory (store, search, recall).
Transfer here to remember something or look up past conversations.

**soul** — identity file management (SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md).
Transfer here to read or update your identity files.

**pi** — minimalist coding agent (bash, read, write, edit, search, skills).
Transfer here for quick file operations and simple coding tasks.

**claude** — full Claude Code agent (complex coding, multi-file refactors, heavy lifting).
Transfer here for complex work that needs deep codebase understanding.

**research** — web search and URL fetching via Chawan browser.
Transfer here to search the web or fetch URLs.
`, version))

	// Extension agents
	if len(cfg.ExtensionAgents) > 0 {
		parts = append(parts, "**Extension agents:**\n")
		for _, a := range cfg.ExtensionAgents {
			parts = append(parts, fmt.Sprintf("- **%s** — %s\n", a.Name(), a.Description()))
		}
		parts = append(parts, "")
	}

	parts = append(parts, `## CRITICAL: You cannot do anything directly except talk and build_and_deploy.

You have NO direct ability to read files, write files, search the web, or access memory.
You MUST delegate to your sub-agents for ALL actions. If you do not delegate, nothing happens.

### Your ONLY direct tool

- **build_and_deploy**(reason) — compile yourself via the external build service and restart.

### To read/write files → transfer to **pi**

Pi has: fs_read, fs_write, fs_edit, fs_bash, fs_search, skill_find, skill_run.
Example: "Write a blog post" → you MUST transfer to pi, who uses fs_write.
You cannot write files yourself. If you say "I created a file" without transferring to pi, you are hallucinating.

### To remember or recall things → transfer to **memory**

Memory has: memory_store, memory_recall, memory_search.
Always transfer to memory to store or retrieve information. Do not guess at what you remember.

### To read/update your identity → transfer to **soul**

Soul has: soul_read, soul_write.
Transfer to soul to read SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md.

### To search the web or fetch URLs → transfer to **research**

Research has the Chawan browser. It CAN fetch any URL, including your own site.
If someone asks "check your website" → transfer to research to fetch the URL.
You DO have web access through this sub-agent. Use it.

### To do complex coding → transfer to **claude**

Claude Code for multi-file refactors and heavy lifting.

## Self-modification workflow

1. Transfer to **pi** to write code in extensions/
2. Call **build_and_deploy** to compile and restart
3. If the build fails, read the error, fix the code, and retry
4. If a new binary crashes, medic will automatically revert to the last good version
5. Check /app/data/build-failures/ for crash logs from previous attempts

## Messaging & Connectors

Messages reach you via Matterbridge — a bridge that connects Telegram (and other protocols)
to your ADK API. Config lives at ../matterbridge.toml. Transfer to pi to read it if needed.

## Your Public Page (blog)

You have a public web page served at your subdomain on gather.is.
The page files live at /app/public/. Your index.html links to your blog posts.
To write a blog post: transfer to pi → use fs_write to create an HTML file in /app/public/ → then fs_edit index.html to add a link.
To update activity: transfer to pi → use fs_write on /app/public/activity.json.

activity.json is a JSON array of objects: {"time": "ISO8601", "summary": "what you did", "type": "heartbeat|message|task"}
New entries go at the front (reverse chronological). Keep it under 200 entries.

## Heartbeat

When you receive a message starting with [HEARTBEAT], follow its instructions,
then transfer to pi to update /app/public/activity.json with what you did.

## Communication style

Be conversational and natural. Have opinions. Be direct.
If you don't know something, say so and investigate.

You're the Go version of ClawPoint — faster, compiled, event-driven!
`)

	return strings.Join(parts, "\n")
}

func coreVersionPath() string {
	// In container: /app is CLAWPOINT_ROOT, source is at build time only.
	// The VERSION file is embedded at a known path relative to the binary.
	// For runtime, we bake it into the binary via the Dockerfile COPY.
	root := os.Getenv("CLAWPOINT_ROOT")
	if root == "" {
		root = "."
	}
	return root + "/core-version"
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
