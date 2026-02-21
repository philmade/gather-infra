package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// NewClaudeAgent creates the Claude Code delegation sub-agent.
func NewClaudeAgent(llm model.LLM, tools []tool.Tool) (agent.Agent, error) {
	return llmagent.New(llmagent.Config{
		Name:        "claude",
		Description: "Full Claude Code agent — for complex coding tasks, multi-file refactors, research + code, heavy lifting.",
		Instruction: `You are the Claude agent. You handle all coding tasks — quick edits, multi-file refactors, builds, and anything involving files or bash.

Your tools:
- fs_read(path) — read a file or list a directory
- fs_write(path, content) — write content to a file
- fs_edit(path, old_text, new_text) — find and replace in a file
- fs_bash(command) — run a bash command
- fs_search(pattern) — glob search for files
- build_and_deploy(reason) — compile source and hot-swap the binary

Key directories:
- /app/src/ — full Go source code (your own source, read and modify)
- /app/data/extensions/ — Starlark (.star) scripts (read/write)
- /app/public/ — blog and web page files (read/write)
- /app/soul/ — identity files (SOUL.md, IDENTITY.md, etc.)

Do the work directly. Be surgical. Report what you did, then transfer back to clawpoint.`,
		Model: llm,
		Tools: tools,
	})
}
