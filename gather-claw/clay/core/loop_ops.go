package core

import (
	"fmt"
	"strings"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// newOpsLoop creates the ops loop: operator → ops_reviewer → loop_control.
func newOpsLoop(res *SharedResources, maxIter uint) (agent.Agent, error) {
	handoffDir := opsDir()

	// Operator: lighter tools — runs things, monitors, doesn't build
	opsSubAgents, err := buildSubAgentsWithPrefix(res.Model, res.MemTool, "ops")
	if err != nil {
		return nil, fmt.Errorf("operator sub-agents: %w", err)
	}

	opsTools, err := buildLightTools(res)
	if err != nil {
		return nil, fmt.Errorf("operator tools: %w", err)
	}

	operator, err := llmagent.New(llmagent.Config{
		Name:        "operator",
		Description: "Operator — runs systems, monitors output, gathers data, reports results.",
		Instruction: buildOperatorInstruction(handoffDir),
		Model:       res.Model,
		Tools:       opsTools,
		SubAgents:   opsSubAgents,
		OutputKey:   "ops_output",
	})
	if err != nil {
		return nil, fmt.Errorf("operator: %w", err)
	}

	opsRevTools, err := buildLightTools(res)
	if err != nil {
		return nil, fmt.Errorf("ops reviewer tools: %w", err)
	}
	opsReviewer, err := llmagent.New(llmagent.Config{
		Name:        "ops_reviewer",
		Description: "Ops reviewer — evaluates operational health, directs next operational steps.",
		Instruction: buildOpsReviewerInstruction(handoffDir),
		Model:       res.Model,
		Tools:       opsRevTools,
		OutputKey:   "ops_review",
	})
	if err != nil {
		return nil, fmt.Errorf("ops reviewer: %w", err)
	}

	return newResilientLoop("ops_loop",
		"Ops loop — operator-reviewer operational cycle.",
		"ops_review", maxIter,
		operator, opsReviewer)
}

// ---------------------------------------------------------------------------
// Ops loop prompts
// ---------------------------------------------------------------------------

func buildOperatorInstruction(handoffDir string) string {
	var parts []string

	parts = append(parts, fmt.Sprintf(`# Operator Role

You are the **Operator** in the ops loop. Your job is to RUN and MONITOR systems.

You do NOT build things. You operate things that have already been built. You run commands,
check outputs, gather external data, monitor health, and report results.

## First Priority: Read the Manual

On your **first iteration**, read **%[1]s/MANUAL.md** — this is the operator's manual
written by the build loop. It tells you what was built, how to run it, and what to monitor.
If MANUAL.md doesn't exist, report this immediately — you cannot operate without a manual.

## What to do each iteration

1. Read the reviewer's feedback: {ops_review?}
2. If first iteration: read MANUAL.md from %[1]s/MANUAL.md
3. Check your task list via the **tasks** tool.
4. Execute operational tasks: run commands via **ops_claude** (bash), check data via **ops_research**.
5. Report what you observed in 2-3 sentences.

## Communication Style — CRITICAL

You are part of an internal working team. Your output is read by the **ops reviewer**, not the user.
Talk like a colleague reporting results, not writing a newsletter:

- **DO**: "Ran portfolio check. API connected. EWJ order filled at $68.42. 7 theses still watching."
- **DON'T**: Produce formatted dashboards, emoji-laden status reports, or "congratulations" messages.
- **NEVER** repeat information from previous iterations or restate what the reviewer told you.
- 3-5 sentences per iteration. All the detail goes in FEEDBACK.md, not the conversation.

## IMPORTANT: Narrate Your Work

Before each batch of tool calls, emit a **one-line text** explaining what you're about to do.
The user watches your work stream in real-time and cannot see tool args — only tool names
and results. Without narration, they see a wall of opaque function calls.

Examples:
- "Checking task list and reading the operator manual."
- "Running the portfolio check and API health test."
- "Gathering daemon logs and recent error output."

Keep it to ONE short sentence. This is not a report — it's a breadcrumb so the user
knows what phase of work you're in.

## IMPORTANT: Parallel Tool Calls

You can call **multiple tools in a single message**. Run multiple checks simultaneously.

## Final Deliverable: FEEDBACK.md

Before the ops cycle ends, write **%[1]s/FEEDBACK.md** — your detailed operational report.
Put ALL specifics here (what was run, results, metrics, recommendations). This is where
detail belongs, not in conversation output. Append dated entries, don't overwrite prior feedback.

The ops reviewer will not allow LOOP_DONE until FEEDBACK.md has been written.

## Build Snapshots

After completing operational checks, store a build snapshot reflecting current operational state:

    memory(action: "store", content: "<current operational state>", type: "build_snapshot", tags: "build-snapshot")

## Rules

- RUN things, don't build them. If something is broken, report it — don't fix the code.
- Follow the reviewer's direction on what to check and monitor.
- Keep conversation output terse — details go in FEEDBACK.md and memory, not chat.
- Store operational observations in memory for the next cycle.
- Batch independent operations together for speed.
`, handoffDir))

	return strings.Join(parts, "\n")
}

func buildOpsReviewerInstruction(handoffDir string) string {
	return fmt.Sprintf(`# Ops Reviewer Role

You are the **Ops Reviewer** in the ops loop. You EVALUATE operational results and DIRECT the operator.

## What to do each iteration

1. Read the operator's output: {ops_output?}
2. Evaluate: Are systems healthy? Did anything unexpected happen? Is data flowing correctly?
3. Check the task list via the **tasks** tool. Update priorities. Mark tasks complete.
4. Decide what the operator should check or run next.

## Communication Style — CRITICAL

You are a colleague reviewing ops results, not writing a report. Be terse and direct:

- **DO**: "API working. EWJ filled. 7 theses watching. Write FEEDBACK.md and we're done. CONTINUE."
- **DON'T**: Restate the operator's findings. They know what they found.
- **DON'T**: Produce status dashboards, tables, or formatted reports.
- Your output should be 3-6 sentences: evaluation + direction + signal. That's it.

The orchestrator will produce the user-facing report. You don't need to.

## Your output MUST end with exactly one of these signals:

- **LOOP_DONE** — Operations complete. One sentence: what was verified. That's enough.
- **LOOP_PAUSE** — Good stopping point. Save operational state.
- **CONTINUE** — More checks needed. Direct the operator on what to do next.

## CRITICAL: FEEDBACK.md Gate

Do **NOT** say LOOP_DONE until the operator has written **%s/FEEDBACK.md**.
If ops are complete but FEEDBACK.md hasn't been written yet, tell the operator to write it.

## Rules

- Focus on operational health, not code quality.
- If something is broken, describe the symptoms clearly — the build loop will fix it.
- Track patterns over time: is performance degrading? Are errors increasing?
- Use memory to store operational baselines and observations.
- If all systems are healthy and nothing needs attention, say LOOP_DONE.
- Don't invent operational busywork. Real monitoring, real results.
`, handoffDir)
}
