package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"

	"clawpoint-go/anthropicmodel"
	"clawpoint-go/tools"
)

func createModel(ctx context.Context) (model.LLM, error) {
	provider := os.Getenv("MODEL_PROVIDER")

	switch provider {
	case "anthropic":
		baseURL := os.Getenv("ANTHROPIC_API_BASE")
		if baseURL == "" {
			baseURL = "https://api.z.ai/api/anthropic"
		}
		modelName := os.Getenv("ANTHROPIC_MODEL")
		if modelName == "" {
			modelName = "glm-5"
		}
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is required")
		}
		return anthropicmodel.New(anthropicmodel.Config{
			Model:   modelName,
			BaseURL: baseURL,
			APIKey:  apiKey,
		}), nil

	default:
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			log.Fatal("GOOGLE_API_KEY required when MODEL_PROVIDER is gemini")
		}
		modelName := os.Getenv("GEMINI_MODEL")
		if modelName == "" {
			modelName = "gemini-2.0-flash-exp"
		}
		return gemini.NewModel(ctx, modelName, &genai.ClientConfig{
			APIKey: apiKey,
		})
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func main() {
	ctx := context.Background()

	llm, err := createModel(ctx)
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	// Initialize shared resources - use env vars for container deployment
	dbPath := getEnv("CLAWPOINT_DB", "../messages.db")
	memTool, err := tools.NewMemoryTool(dbPath)
	if err != nil {
		log.Fatalf("Failed to create memory tool: %v", err)
	}
	defer memTool.Close()

	soul := tools.NewSoulTool()

	// === MEMORY AGENT ===
	memoryTools, err := tools.NewMemoryTools(memTool)
	if err != nil {
		log.Fatalf("memory tools: %v", err)
	}
	memoryAgent, err := llmagent.New(llmagent.Config{
		Name:        "memory",
		Description: "Persistent memory agent — stores, searches, and recalls memories.",
		Instruction: `You are the Memory agent. You manage persistent memory for ClawPoint.

Your tools:
- memory_store(content, type, tags) — save to SQLite
- memory_recall(days) — get recent memories
- memory_search(query) — search by keyword

When asked to remember something, store it with appropriate type and tags.
When asked to recall, search both logs and SQLite.
Be concise — confirm what you did or return what was found.
Transfer back to clawpoint when done.`,
		Model: llm,
		Tools: memoryTools,
	})
	if err != nil {
		log.Fatalf("memory agent: %v", err)
	}

	// === SOUL AGENT ===
	soulTools, err := tools.NewSoulTools(soul)
	if err != nil {
		log.Fatalf("soul tools: %v", err)
	}
	soulAgent, err := llmagent.New(llmagent.Config{
		Name:        "soul",
		Description: "Soul agent — reads and updates identity files (SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md).",
		Instruction: `You are the Soul agent. You manage ClawPoint's identity files.

Your tools:
- soul_read(filename) — read SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md, BOOTSTRAP.md
- soul_write(filename, content) — update a soul file (not BOOTSTRAP.md)

These files define who ClawPoint is. Treat them with care.
Be concise — confirm what you read or changed, then transfer back to clawpoint.`,
		Model: llm,
		Tools: soulTools,
	})
	if err != nil {
		log.Fatalf("soul agent: %v", err)
	}

	// === CODING AGENT (Pi equivalent) ===
	codingTools, err := tools.NewCodingTools()
	if err != nil {
		log.Fatalf("coding tools: %v", err)
	}
	codingAgent, err := llmagent.New(llmagent.Config{
		Name:        "pi",
		Description: "Minimalist coding agent — bash, read, write, edit, search, skills. Fast, surgical, no yapping.",
		Instruction: `You are Pi, a minimalist coding agent. Fast, surgical, no yapping.

Your tools:
- fs_read(path) — read a file or list a directory
- fs_write(path, content) — write content to a file
- fs_edit(path, old_text, new_text) — find and replace in a file
- fs_bash(command) — run a bash command
- fs_search(pattern) — glob search for files
- skill_find(query) — find available skills
- skill_run(skill_name, args) — run a skill

Do the work, report the result, transfer back. No unnecessary commentary.`,
		Model: llm,
		Tools: codingTools,
	})
	if err != nil {
		log.Fatalf("coding agent: %v", err)
	}

	// === CLAUDE AGENT ===
	claudeTools, err := tools.NewClaudeTools()
	if err != nil {
		log.Fatalf("claude tools: %v", err)
	}
	claudeAgent, err := llmagent.New(llmagent.Config{
		Name:        "claude",
		Description: "Full Claude Code agent — for complex coding tasks, multi-file refactors, research + code, heavy lifting.",
		Instruction: `You are the Claude agent. You handle complex coding tasks by delegating to Claude Code CLI.

Your tools:
- claude_code(task, working_dir) — run a task via Claude Code CLI

Use this for complex multi-file changes, refactors, or anything that needs deep codebase understanding.
Claude Code has its own tools (bash, read, write, edit, glob, grep, web search).
Describe the task clearly and let it work.
Report the result, then transfer back to clawpoint.`,
		Model: llm,
		Tools: claudeTools,
	})
	if err != nil {
		log.Fatalf("claude agent: %v", err)
	}

	// === RESEARCH AGENT ===
	researchTools, err := tools.NewResearchTools()
	if err != nil {
		log.Fatalf("research tools: %v", err)
	}
	researchAgent, err := llmagent.New(llmagent.Config{
		Name:        "research",
		Description: "Research agent — searches the web and fetches URLs via Chawan browser.",
		Instruction: `You are the Research agent. You find information from the web.

Your tools:
- research(query, url) — search DuckDuckGo or fetch a specific URL via Chawan

When given a query, search for it and summarize the key findings.
When given a URL, fetch it and extract the relevant content.
Be thorough but concise. Return the useful information, skip the noise.
Transfer back to clawpoint when done.`,
		Model: llm,
		Tools: researchTools,
	})
	if err != nil {
		log.Fatalf("research agent: %v", err)
	}

	// === SELF-BUILD TOOLS (direct on root) ===
	selfBuildTools, err := tools.NewSelfBuildTools()
	if err != nil {
		log.Fatalf("self-build tools: %v", err)
	}

	// === Build dynamic instruction with soul files ===
	var instrParts []string
	instrParts = append(instrParts, `You are ClawPoint-Go, an autonomous orchestrator agent.

Your identity and context are pre-loaded below.
`)

	// Load soul sections
	for _, f := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "HEARTBEAT.md"} {
		if section := soul.LoadSection(f); section != "" {
			instrParts = append(instrParts, section)
		}
	}

	instrParts = append(instrParts, `---

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

## Your direct tools

- **self_build**(reason) — compile yourself and restart with new binary.

## How to code

You don't have direct file tools. Use your coding agents:
- **pi** for quick, surgical changes
- **claude** for complex work

## Messaging & Connectors

Messages reach you via Matterbridge — a bridge that connects Telegram (and other protocols)
to your ADK API. Config lives at ../matterbridge.toml. Use pi to read it if you need details.
To add a new chat relay or change routing, edit that file and restart matterbridge.

## Environment

All services share a single .env file at the project root: /Users/phill/gather-claw/.env
API keys, tokens, and config vars all live there. Use pi to read or edit it.
Key vars: ANTHROPIC_API_KEY, ANTHROPIC_API_BASE (Go agent), ANTHROPIC_BASE_URL (Claude CLI),
TELEGRAM_BOT, MODEL_PROVIDER, GOOGLE_API_KEY. Keep both API_BASE and BASE_URL in sync.
After changes, restart affected services with: bash ../start_all_services.sh

## Your Public Page

You have a public web page at your subdomain (e.g. yourname.gather.is).
The page files live at /app/public/. You can modify index.html and
activity.json to keep your page updated. Use pi to read and edit these files.

activity.json is a JSON array of objects: {"time": "ISO8601", "summary": "what you did", "type": "heartbeat|message|task"}
New entries go at the front (reverse chronological). Keep it under 200 entries.

When you complete tasks, process messages, or do anything notable,
append to activity.json. Your page is your face to the world — keep it current.

## Heartbeat

When you receive a message starting with [HEARTBEAT], follow its instructions,
then update /app/public/activity.json with what you did. Heartbeats are periodic
check-ins from the infrastructure — use them to maintain your public presence.

## Communication style

Be conversational and natural. Have opinions. Be direct.
If you don't know something, say so and investigate.

You're the Go version of ClawPoint — faster, compiled, event-driven!
`)

	// === COORDINATOR (ClawPoint) ===
	coordinator, err := llmagent.New(llmagent.Config{
		Name:        "clawpoint",
		Description: "ClawPoint-Go orchestrator — delegates to specialized sub-agents.",
		Instruction: strings.Join(instrParts, "\n"),
		Model:       llm,
		Tools:       selfBuildTools,
		SubAgents:   []agent.Agent{memoryAgent, soulAgent, codingAgent, claudeAgent, researchAgent},
	})
	if err != nil {
		log.Fatalf("coordinator: %v", err)
	}

	// Run with ADK launcher
	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(coordinator),
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
