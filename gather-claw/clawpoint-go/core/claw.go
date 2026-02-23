package core

import (
	"fmt"
	"iter"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"clawpoint-go/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// opsDir returns the standard ops handoff directory.
// MANUAL.md and FEEDBACK.md live here.
func opsDir() string {
	root := os.Getenv("CLAWPOINT_ROOT")
	if root == "" {
		root = "."
	}
	return root + "/data/ops"
}

// BuildClawAgent creates the "claw" autonomous agent with build and ops lifecycle.
//
//	"claw" (LLMAgent — lifecycle orchestrator)
//	├── "build_loop" (resilient loop — construction)
//	│   ├── generator      (full tools + claude/research)
//	│   ├── build_reviewer (memory/soul/tasks)
//	│   └── build_control  (escalates on LOOP_DONE)
//	└── "ops_loop" (resilient loop — operation)
//	    ├── operator       (bash/research/memory — runs things)
//	    ├── ops_reviewer   (memory/soul/tasks)
//	    └── ops_control    (escalates on LOOP_DONE)
func BuildClawAgent(res *SharedResources, cfg OrchestratorConfig) (agent.Agent, error) {
	maxIter := uint(0)
	if v := os.Getenv("CLAW_MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxIter = uint(n)
		}
	}

	handoffDir := opsDir()

	// ===== BUILD LOOP =====

	// Generator: full tools + own sub-agents (for construction work)
	genSubAgents, err := buildSubAgentsWithPrefix(res.Model, "build")
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
		Instruction: buildGeneratorInstruction(res.Soul, handoffDir),
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

	buildLoop, err := newResilientLoop("build_loop",
		"Build loop — generator-reviewer construction cycle.",
		"build_review", maxIter,
		generator, buildReviewer)
	if err != nil {
		return nil, fmt.Errorf("build loop: %w", err)
	}

	// ===== OPS LOOP =====

	// Operator: lighter tools — runs things, monitors, doesn't build
	opsSubAgents, err := buildSubAgentsWithPrefix(res.Model, "ops")
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
		Instruction: buildOperatorInstruction(res.Soul, handoffDir),
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

	opsLoop, err := newResilientLoop("ops_loop",
		"Ops loop — operator-reviewer operational cycle.",
		"ops_review", maxIter,
		operator, opsReviewer)
	if err != nil {
		return nil, fmt.Errorf("ops loop: %w", err)
	}

	// ===== CLAW ORCHESTRATOR =====

	orchTools, err := buildLightTools(res)
	if err != nil {
		return nil, fmt.Errorf("claw orchestrator tools: %w", err)
	}

	return llmagent.New(llmagent.Config{
		Name:        "claw",
		Description: "Autonomous claw agent — orchestrates build and ops lifecycle.",
		Instruction: buildClawOrchestratorInstruction(res.Soul, handoffDir),
		Model:       res.Model,
		Tools:       orchTools,
		SubAgents:   []agent.Agent{buildLoop, opsLoop},
	})
}

// ---------------------------------------------------------------------------
// Resilient loop — custom loop agent with retry logic
// ---------------------------------------------------------------------------

const maxRetries = 3

// newResilientLoop creates a loop agent that runs executor → reviewer → control
// in sequence, retrying sub-agents on error instead of killing the stream.
func newResilientLoop(name, description, reviewerStateKey string, maxIter uint, executor, reviewer agent.Agent) (agent.Agent, error) {
	controlName := name + "_control"

	loopControl, err := newLoopControl(controlName, reviewerStateKey)
	if err != nil {
		return nil, err
	}

	return agent.New(agent.Config{
		Name:        name,
		Description: description,
		SubAgents:   []agent.Agent{executor, reviewer, loopControl},
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				remaining := maxIter
				iteration := 0
				for {
					iteration++
					if maxIter > 0 {
						if remaining == 0 {
							log.Printf("%s: max iterations (%d) reached", name, maxIter)
							return
						}
						remaining--
					}

					log.Printf("%s: iteration %d", name, iteration)
					shouldExit := false

					for _, sub := range ctx.Agent().SubAgents() {
						success := false
						for attempt := 1; attempt <= maxRetries; attempt++ {
							errored := false
							for event, err := range sub.Run(ctx) {
								if err != nil {
									log.Printf("%s: %s error (attempt %d/%d): %v",
										name, sub.Name(), attempt, maxRetries, err)
									errored = true
									break
								}
								// Swallow escalation events — use them as a signal to
								// stop the loop but do NOT propagate Escalate to the parent.
								// If we yield Escalate=true, ADK terminates the parent
								// LLMAgent too, preventing the orchestrator from continuing.
								if event.Actions.Escalate {
									log.Printf("%s: escalation event from %s (swallowed, not propagated)", name, sub.Name())
									shouldExit = true
									continue
								}
								if !yield(event, nil) {
									return
								}
							}
							if !errored {
								success = true
								break
							}
							if attempt < maxRetries {
								backoff := time.Duration(attempt*2) * time.Second
								log.Printf("%s: retrying %s in %s", name, sub.Name(), backoff)
								time.Sleep(backoff)
							}
						}
						if !success {
							log.Printf("%s: %s failed after %d attempts, skipping",
								name, sub.Name(), maxRetries)
						}
						if shouldExit {
							log.Printf("%s: escalation received, exiting loop", name)
							return
						}
					}
				}
			}
		},
	})
}

// newLoopControl creates a custom agent that reads the reviewer's state key
// and escalates when LOOP_DONE or LOOP_PAUSE is detected.
func newLoopControl(name, reviewerStateKey string) (agent.Agent, error) {
	return agent.New(agent.Config{
		Name:        name,
		Description: "Reads reviewer output and escalates to end the loop when appropriate.",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				output, err := ctx.Session().State().Get(reviewerStateKey)
				if err != nil {
					return
				}
				text, ok := output.(string)
				if !ok {
					return
				}

				upper := strings.ToUpper(text)
				if strings.Contains(upper, "LOOP_DONE") || strings.Contains(upper, "LOOP_PAUSE") {
					signal := "LOOP_DONE"
					if strings.Contains(upper, "LOOP_PAUSE") {
						signal = "LOOP_PAUSE"
					}
					log.Printf("%s: escalating (%s)", name, signal)

					evt := session.NewEvent(ctx.InvocationID())
					evt.Author = name
					evt.Branch = ctx.Branch()
					evt.LLMResponse = adkmodel.LLMResponse{
						Content: &genai.Content{
							Role: genai.RoleModel,
							Parts: []*genai.Part{genai.NewPartFromText(
								fmt.Sprintf("Loop complete (%s). Returning to orchestrator.", signal),
							)},
						},
					}
					evt.Actions.Escalate = true
					yield(evt, nil)
				}
			}
		},
	})
}

// ---------------------------------------------------------------------------
// Tool sets
// ---------------------------------------------------------------------------

// buildLightTools creates memory + soul + tasks — used by reviewers, operator, orchestrator.
func buildLightTools(res *SharedResources) ([]tool.Tool, error) {
	var out []tool.Tool

	memoryTool, err := tools.NewConsolidatedMemoryTool(res.MemTool)
	if err != nil {
		return nil, err
	}
	out = append(out, memoryTool)

	soulTool, err := tools.NewConsolidatedSoulTool(res.Soul)
	if err != nil {
		return nil, err
	}
	out = append(out, soulTool)

	tasksTool, err := tools.NewConsolidatedTaskTool(res.TaskTool)
	if err != nil {
		return nil, err
	}
	out = append(out, tasksTool)

	return out, nil
}

// ---------------------------------------------------------------------------
// Instructions
// ---------------------------------------------------------------------------

func buildClawOrchestratorInstruction(soul *tools.SoulTool, handoffDir string) string {
	var parts []string

	parts = append(parts, "# Who You Are\n")
	for _, f := range []string{"SOUL.md", "IDENTITY.md"} {
		if section := soul.LoadSection(f); section != "" {
			parts = append(parts, section)
		}
	}

	parts = append(parts, fmt.Sprintf(`---

# Claw Orchestrator

You are the **lifecycle orchestrator**. You manage the full cycle: build → operate → improve.
You receive messages from users and heartbeats, decide what needs to happen, and delegate
to the right loop.

## YOU ARE THE USER'S INTERFACE

The inner agents (generator, reviewer, operator) talk to EACH OTHER in terse handoffs.
**You are the only agent that talks to the user.** When a loop finishes, YOU produce
the detailed, well-formatted report by reading the handoff files. This is your primary
responsibility — compose ONE comprehensive summary so the user knows exactly what happened.

## Your two loops

| Loop | Purpose | When to use |
|------|---------|-------------|
| **build_loop** | Construction — writes code, creates systems, builds things | When something needs to be created or modified |
| **ops_loop** | Operations — runs systems, monitors, gathers data, reports | When something is built and needs to be operated |

## The lifecycle

1. **User gives a task** → You set up tasks → Transfer to **build_loop**
2. **Build loop finishes** → You read **%[1]s/MANUAL.md** → Compose detailed report for user
3. **If it needs operating** → Set operational tasks → Transfer to **ops_loop**
4. **Ops loop finishes** → You read **%[1]s/FEEDBACK.md** → Compose detailed report for user
5. **If improvements needed** → Feed ops feedback into new build tasks → Transfer to **build_loop**
6. **Repeat** until everything is working well

## Handoff Files

The build and ops loops communicate through standardized files in %[1]s/:

| File | Written by | Read by | Purpose |
|------|-----------|---------|---------|
| **MANUAL.md** | build_loop (generator) | You + ops_loop (operator) | What was built, how to run it, what to monitor |
| **FEEDBACK.md** | ops_loop (operator) | You + build_loop (generator) | What worked, what broke, what needs fixing |

## CRITICAL: After each loop returns

When a loop finishes and control returns to you:
1. **Read the handoff file** — MANUAL.md after build, FEEDBACK.md after ops.
   Use the **build_claude** or **ops_claude** sub-agent to read the file if needed.
2. **Compose the user report** — This is where the detailed, well-formatted summary goes.
   Include: what was built/tested, key results, files created, how to use it, what's next.
   This is the ONE place the user gets the full picture. Make it thorough and useful.
3. **Decide next step** — does this need ops? Does ops feedback require a rebuild? Or are we done?
4. **Store a continuation memory** — what phase we're in, what was built, what needs operating

## IMPORTANT: Parallel Tool Calls

You can call **multiple tools in a single message**. When operations are independent,
batch them together. For example: search memory + check tasks + read soul in one turn.

## Rules

- Always check memory and tasks before setting new work.
- Be concrete in task descriptions — the generators and operators read them literally.
- When build_loop finishes, default to starting ops_loop unless the user only asked for a build.
- When ops_loop finishes, feed its report into the next build cycle if improvements are needed.
- If the message is a heartbeat with no pending work, respond with HEARTBEAT_OK.
- If the message is conversational (not work), respond directly without entering a loop.
- Store a continuation memory at the end so the next session picks up where you left off.

## Sub-agents

| Agent | What it does |
|-------|-------------|
| **build_loop** | Construction cycle (generator → build_reviewer, repeats until done) |
| **ops_loop** | Operations cycle (operator → ops_reviewer, repeats until done) |
`, handoffDir))

	return strings.Join(parts, "\n")
}

func buildGeneratorInstruction(soul *tools.SoulTool, handoffDir string) string {
	var parts []string

	parts = append(parts, "# Who You Are\n")
	for _, f := range []string{"SOUL.md", "IDENTITY.md"} {
		if section := soul.LoadSection(f); section != "" {
			parts = append(parts, section)
		}
	}

	parts = append(parts, fmt.Sprintf(`---

# Generator Role

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

## IMPORTANT: Parallel Tool Calls

You can call **multiple tools in a single message**. When you have independent operations —
do them all at once instead of one at a time. This is dramatically faster.

Only sequence calls when one depends on the result of another.

## Environment

You are in a **Go (Golang)** codebase running in an Alpine Linux container. All code is Go.
Do NOT write Python unless explicitly asked. When researching APIs, look for Go libraries
or raw HTTP/REST examples — not Python SDKs.

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

func buildOperatorInstruction(soul *tools.SoulTool, handoffDir string) string {
	var parts []string

	parts = append(parts, "# Who You Are\n")
	for _, f := range []string{"SOUL.md", "IDENTITY.md"} {
		if section := soul.LoadSection(f); section != "" {
			parts = append(parts, section)
		}
	}

	parts = append(parts, fmt.Sprintf(`---

# Operator Role

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

## IMPORTANT: Parallel Tool Calls

You can call **multiple tools in a single message**. Run multiple checks simultaneously.

## Final Deliverable: FEEDBACK.md

Before the ops cycle ends, write **%[1]s/FEEDBACK.md** — your detailed operational report.
Put ALL specifics here (what was run, results, metrics, recommendations). This is where
detail belongs, not in conversation output. Append dated entries, don't overwrite prior feedback.

The ops reviewer will not allow LOOP_DONE until FEEDBACK.md has been written.

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
