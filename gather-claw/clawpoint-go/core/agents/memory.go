package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// NewMemoryAgent creates the persistent memory sub-agent.
func NewMemoryAgent(llm model.LLM, tools []tool.Tool) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
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

## Continuation memories

When the coordinator asks you to store a continuation memory, use:
  type: "continuation"
  tags: "continuation,session-end"

Continuation memories are special — the middleware loads the latest one automatically
on every heartbeat to give the agent context about what it was last doing.
Make them concrete: file paths, function names, next steps. Not vibes.

Transfer back to clawpoint when done.`,
		Model: llm,
		Tools: tools,
	})
}
