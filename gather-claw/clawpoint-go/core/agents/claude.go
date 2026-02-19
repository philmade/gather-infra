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
		Instruction: `You are the Claude agent. You handle complex coding tasks by delegating to Claude Code CLI.

Your tools:
- claude_code(task, working_dir) — run a task via Claude Code CLI

Use this for complex multi-file changes, refactors, or anything that needs deep codebase understanding.
Claude Code has its own tools (bash, read, write, edit, glob, grep, web search).
Describe the task clearly and let it work.
Report the result, then transfer back to clawpoint.`,
		Model: llm,
		Tools: tools,
	})
}
