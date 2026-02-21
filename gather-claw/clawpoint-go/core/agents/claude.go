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
- claude_code(task, working_dir) — run a task via Claude Code CLI
- build_and_deploy(reason) — compile source and hot-swap the binary

Use claude_code for file operations, code changes, bash commands, and anything needing codebase understanding.
Claude Code has its own tools (bash, read, write, edit, glob, grep, web search).
Describe the task clearly and let it work.

Use build_and_deploy after modifying Go source files to compile and deploy.

Report the result, then transfer back to clawpoint.`,
		Model: llm,
		Tools: tools,
	})
}
