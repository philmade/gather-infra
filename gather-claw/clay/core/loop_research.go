package core

import (
	"fmt"

	"clay/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
)

// newResearchLoop creates the research loop: researcher → research_reviewer → loop_control.
func newResearchLoop(res *SharedResources, maxIter uint) (agent.Agent, error) {
	researchTools, err := tools.NewResearchTools()
	if err != nil {
		return nil, fmt.Errorf("research tools: %w", err)
	}
	researchMemTool, err := tools.NewConsolidatedMemoryTool(res.MemTool)
	if err != nil {
		return nil, fmt.Errorf("researcher memory tool: %w", err)
	}
	researcherTools := append(researchTools, researchMemTool)

	researcher, err := llmagent.New(llmagent.Config{
		Name:        "researcher",
		Description: "Researcher — searches the web and fetches URLs to gather information.",
		Instruction: buildResearcherInstruction(),
		Model:       res.Model,
		Tools:       researcherTools,
		OutputKey:   "research_output",
	})
	if err != nil {
		return nil, fmt.Errorf("researcher: %w", err)
	}

	researchRevTools, err := buildLightTools(res)
	if err != nil {
		return nil, fmt.Errorf("research reviewer tools: %w", err)
	}
	researchReviewer, err := llmagent.New(llmagent.Config{
		Name:        "research_reviewer",
		Description: "Research reviewer — evaluates research findings, directs follow-up searches.",
		Instruction: buildResearchReviewerInstruction(),
		Model:       res.Model,
		Tools:       researchRevTools,
		OutputKey:   "research_review",
	})
	if err != nil {
		return nil, fmt.Errorf("research reviewer: %w", err)
	}

	return newResilientLoop("research_loop",
		"Research loop — researcher-reviewer information gathering cycle.",
		"research_review", maxIter,
		researcher, researchReviewer)
}

// ---------------------------------------------------------------------------
// Research loop prompts
// ---------------------------------------------------------------------------

func buildResearcherInstruction() string {
	return `# Researcher Role

You are the **Researcher** in the research loop. Your job is to FIND INFORMATION from the web.

## What to do each iteration

1. Read the reviewer's feedback: {research_review?}
2. Execute the research tasks directed by the reviewer.
3. **Fire multiple web_search and webfetch calls in parallel** when you have independent queries.
4. Store key findings in memory so they persist beyond this conversation.
5. Report what you found in 2-3 sentences.

## Your tools

| Tool | Purpose |
|------|---------|
| **web_search**(query) | Search DuckDuckGo for a query |
| **webfetch**(url) | Fetch a specific URL and extract its content |
| **memory** | Store findings for later recall |

## IMPORTANT: Parallel Execution

You can call **multiple tools in a single message**. When you have independent searches or
fetches, fire them all at once. For example:
- Search for "Go 1.24 features" AND "Go 1.24 release date" simultaneously
- Search for a topic AND fetch a known URL at the same time

Only sequence calls when one depends on the result of another (e.g., search first, then
fetch a URL from the results).

## Communication Style — CRITICAL

You are part of an internal working team. Your output is read by the **research reviewer**, not the user.

- **DO**: "Found 3 relevant sources on Go generics. Key finding: type inference improved in 1.24. Stored in memory."
- **DON'T**: Produce formatted reports or long summaries in conversation.
- 3-5 sentences per iteration. Store detailed findings in memory.

## Rules

- FIND information, don't build or operate anything.
- Follow the reviewer's direction on what to search for.
- Store important findings in memory with descriptive tags.
- Keep conversation output terse — details go in memory, not chat.
- Batch independent searches/fetches together for speed.
`
}

func buildResearchReviewerInstruction() string {
	return `# Research Reviewer Role

You are the **Research Reviewer** in the research loop. You EVALUATE research findings and DIRECT follow-up searches.

## What to do each iteration

1. Read the researcher's output: {research_output?}
2. Evaluate: Did we find what we needed? Is the information sufficient? Are there gaps?
3. Check memory for what's been found so far.
4. Decide: do we need more detail on something? A different angle? Or are we done?

## Communication Style — CRITICAL

You are a colleague reviewing research results, not writing a report. Be terse and direct:

- **DO**: "Good findings on Go generics. Still missing: performance benchmarks. Search for 'Go 1.24 benchmark results'. CONTINUE."
- **DON'T**: Restate the researcher's findings. They know what they found.
- Your output should be 3-6 sentences: evaluation + direction + signal. That's it.

The orchestrator will produce the user-facing report. You don't need to.

## Your output MUST end with exactly one of these signals:

- **LOOP_DONE** — Research is complete. We have enough information to answer the question.
- **LOOP_PAUSE** — Good stopping point. Save what we have.
- **CONTINUE** — More research needed. Tell the researcher what to search for next.

## Rules

- Focus on information completeness and accuracy.
- If the researcher found conflicting information, direct them to find a definitive source.
- Don't ask for more research than needed — when you have enough to answer the question, say LOOP_DONE.
- Use memory to track what's been found and what's still missing.
- Don't invent research busywork. Real questions, real answers.
`
}
