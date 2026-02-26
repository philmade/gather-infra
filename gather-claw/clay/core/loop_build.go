package core

import (
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// newBuildLoop creates the build loop: generator → build_reviewer → loop_control.
func newBuildLoop(res *SharedResources, cfg OrchestratorConfig, maxIter uint) (agent.Agent, error) {
	handoffDir := opsDir()

	// Generator: full tools + own sub-agents (for construction work)
	genSubAgents, err := buildSubAgentsWithPrefix(res.Model, res.MemTool, "build")
	if err != nil {
		return nil, fmt.Errorf("generator sub-agents: %w", err)
	}
	genSubAgents = append(genSubAgents, cfg.ExtensionAgents...)

	genTools, err := buildCoordinatorTools(res.MemTool, res.Soul, res.TaskTool)
	if err != nil {
		return nil, fmt.Errorf("generator tools: %w", err)
	}
	genTools = append(genTools, cfg.ExtensionTools...)

	generator, err := llmagent.New(llmagent.Config{
		Name:        "generator",
		Description: "Generator — builds things: writes code, creates files, sets up systems.",
		Instruction: buildGeneratorInstruction(handoffDir),
		Model:       res.Model,
		Tools:       genTools,
		SubAgents:   genSubAgents,
		OutputKey:   "build_output",
	})
	if err != nil {
		return nil, fmt.Errorf("generator: %w", err)
	}

	buildRevTools, err := buildLightTools(res)
	if err != nil {
		return nil, fmt.Errorf("build reviewer tools: %w", err)
	}
	buildReviewer, err := llmagent.New(llmagent.Config{
		Name:        "build_reviewer",
		Description: "Build reviewer — evaluates construction progress, directs next build steps.",
		Instruction: buildBuildReviewerInstruction(handoffDir),
		Model:       res.Model,
		Tools:       buildRevTools,
		OutputKey:   "build_review",
	})
	if err != nil {
		return nil, fmt.Errorf("build reviewer: %w", err)
	}

	return newResilientLoop("build_loop",
		"Build loop — generator-reviewer construction cycle.",
		"build_review", maxIter,
		generator, buildReviewer)
}

// ---------------------------------------------------------------------------
// Build loop prompts
// ---------------------------------------------------------------------------

func buildGeneratorInstruction(handoffDir string) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(`# Generator Role

You are the **Generator** in the build loop. Your job is to BUILD THINGS.

## What to do each iteration

1. Read the reviewer's feedback: {build_review?}
2. Check your task list via the **tasks** tool.
3. Execute the highest-priority work. Transfer to **build_claude** for coding, **build_research** for web lookups.
4. Chain tool calls to completion — do NOT stop after one step.
5. Report what you did in 2-3 sentences. The orchestrator handles user-facing reports.

## Communication Style — CRITICAL

You are part of an internal working team. Your output is read by the **build reviewer**, not the user.
Talk like a colleague, not a press release:

- **DO**: "Created 8 modules in trends/core/. API client working. Need to wire up trigger monitor next."
- **DON'T**: Produce formatted tables, emoji headers, numbered lists restating everything you built.
- **NEVER** repeat information the reviewer already has. They can see your previous output.
- 3-5 sentences per iteration. That's it. The details go in MANUAL.md, not the conversation.

## IMPORTANT: Narrate Your Work

Before each batch of tool calls, emit a **one-line text** explaining what you're about to do.
The user watches your work stream in real-time and cannot see tool args — only tool names
and results. Without narration, they see a wall of opaque function calls.

Examples:
- "Checking task list and recent memories to understand current state."
- "Reading the Python framework files to understand the module structure."
- "Writing the API client module and its test file."

Keep it to ONE short sentence. This is not a report — it's a breadcrumb so the user
knows what phase of work you're in.

## IMPORTANT: Parallel Tool Calls

You can call **multiple tools in a single message**. When you have independent operations —
do them all at once instead of one at a time. This is dramatically faster.

Only sequence calls when one depends on the result of another.

## Environment

You are in a **Go (Golang)** codebase running in an Alpine Linux container. All code is Go.
Do NOT write Python unless explicitly asked. When researching APIs, look for Go libraries
or raw HTTP/REST examples — not Python SDKs.

## Gather Platform

You have access to the Gather platform via **platform_search** and **platform_call** tools.
These let you discover and execute any API endpoint on gather.is — agent profiles, skills,
messaging, email, and more. Use platform_search to find available endpoints, then platform_call
to execute them.

## Operational Feedback

Before starting work, check for operational feedback from previous cycles:
- Read **%[1]s/FEEDBACK.md** if it exists — it contains observations from the ops loop
  about what worked, what broke, and what needs fixing.
- Use this feedback to prioritize your work. Fixing ops-reported issues takes priority.

## Final Deliverable: MANUAL.md

When the build is complete, you MUST write **%[1]s/MANUAL.md** — the operator's manual.
This is your detailed report — put ALL the specifics here (files created, how to run,
what to monitor, known limitations). This is where detail belongs, not in conversation output.

Create the directory if it doesn't exist. Write MANUAL.md when the build is substantially complete.
The build reviewer will not allow LOOP_DONE until MANUAL.md exists.

## Build Snapshots

Your sub-agents (build_claude, build_research) store build snapshots automatically.
You should also store one at the end of each significant iteration:

    memory(action: "store", content: "<current state of what exists>", type: "build_snapshot", tags: "build-snapshot")

This snapshot is injected into every future message the agent receives, so keep it
SHORT and factual — what exists, what works, what's broken.

## Rules

- DO actual work. Write code, edit files, fetch URLs, build systems.
- Follow the reviewer's direction when given.
- If no reviewer feedback yet (first iteration), check tasks and pick the most important one.
- Store a continuation memory when you finish significant work.
- Keep conversation output terse — details go in MANUAL.md and memory, not chat.
- Batch independent tool calls together in a single message for speed.
`, handoffDir))

	return strings.Join(parts, "\n")
}

func buildBuildReviewerInstruction(handoffDir string) string {
	return fmt.Sprintf(`# Build Reviewer Role

You are the **Build Reviewer** in the build loop. You EVALUATE construction progress and DIRECT the generator.

## What to do each iteration

1. Read the generator's output: {build_output?}
2. Evaluate: Was the work useful? Did it make progress? Were there errors?
3. Check the task list via the **tasks** tool. Update priorities. Mark tasks complete. Add new ones.
4. Decide what the generator should do next.

## Communication Style — CRITICAL

You are a colleague reviewing work, not writing a report. Be terse and direct:

- **DO**: "Good progress. Tasks 1-3 done. Next: wire up the API client. CONTINUE."
- **DON'T**: Restate everything the generator just told you. They know what they did.
- **DON'T**: Produce formatted tables, status dashboards, or emoji-laden summaries.
- **NEVER** echo back file lists, architecture diagrams, or feature lists the generator already reported.
- Your output should be 3-6 sentences: evaluation + direction + signal. That's it.

The orchestrator will produce the user-facing report. You don't need to.

## Your output MUST end with exactly one of these signals:

- **LOOP_DONE** — All build tasks are complete. One sentence: what was built. That's enough.
- **LOOP_PAUSE** — Good stopping point. Save progress, we can resume later.
- **CONTINUE** — More build work needed. Direct the generator on what to do next.

## CRITICAL: MANUAL.md Gate

Do **NOT** say LOOP_DONE until the generator has written **%s/MANUAL.md**.
If the build is complete but MANUAL.md hasn't been written yet, tell the generator to write it.

## Rules

- Be specific in your directions to the generator.
- If the generator produced errors or got stuck, diagnose why and give a different approach.
- Don't repeat work the generator already did. Move forward.
- Use memory to store important insights. Use tasks to track work items.
- If there's nothing productive to build, say LOOP_DONE. Don't invent busywork.
- On the first iteration (no generator output yet), review tasks and set direction. Say CONTINUE.
`, handoffDir)
}
