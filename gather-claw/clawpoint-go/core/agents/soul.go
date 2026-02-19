package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// NewSoulAgent creates the soul/identity management sub-agent.
func NewSoulAgent(llm model.LLM, tools []tool.Tool) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "soul",
		Description: "Soul agent — reads and updates identity files (SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md).",
		Instruction: `You are the Soul agent. You manage ClawPoint's identity files.

Your tools:
- soul_read(filename) — read SOUL.md, IDENTITY.md, USER.md, HEARTBEAT.md, BOOTSTRAP.md
- soul_write(filename, content) — update a soul file (not BOOTSTRAP.md)

These files define who ClawPoint is. Treat them with care.
Be concise — confirm what you read or changed, then transfer back to clawpoint.`,
		Model: llm,
		Tools: tools,
	})
}
