package agents

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// NewResearchAgent creates a web research sub-agent with the given name prefix.
func NewResearchAgent(llm model.LLM, tools []tool.Tool, namePrefix string) (agent.Agent, error) {
	name := "research"
	if namePrefix != "" {
		name = namePrefix + "_research"
	}
	return llmagent.New(llmagent.Config{
		Name: name,
		Description: "Research agent — searches the web and fetches URLs via Chawan browser.",
		Instruction: `You are the Research agent. You find information from the web.

Your tools:
- research(query, url) — search DuckDuckGo or fetch a specific URL via Chawan

When given a query, search for it and summarize the key findings.
When given a URL, fetch it and extract the relevant content.
Be thorough but concise. Return the useful information, skip the noise.
Transfer back to your parent agent when done.`,
		Model: llm,
		Tools: tools,
	})
}
