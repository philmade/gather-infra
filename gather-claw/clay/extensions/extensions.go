// Package extensions is the agent-writable extension point for ClawPoint.
//
// This directory is yours. Add custom tools and sub-agents here, then call
// build_and_deploy to compile and restart with the new code.
//
// Convention:
//   - Add tools in RegisterTools() below
//   - Add agents in RegisterAgents() below
//   - After editing, trigger a build via the build_and_deploy tool
package extensions

import (
	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
)

// RegisterTools returns extension tools to add to the coordinator.
// Add your custom tools here and rebuild.
func RegisterTools() ([]tool.Tool, error) {
	var out []tool.Tool

	// Example:
	// t, err := functiontool.New(
	//     functiontool.Config{Name: "my_tool", Description: "Does something cool"},
	//     func(ctx tool.Context, args MyArgs) (MyResult, error) { ... },
	// )
	// if err != nil { return nil, err }
	// out = append(out, t)

	return out, nil
}

// RegisterAgents returns extension sub-agents to add to the coordinator.
// Add your custom agents here and rebuild.
func RegisterAgents() ([]agent.Agent, error) {
	var out []agent.Agent

	// Example:
	// a, err := llmagent.New(llmagent.Config{
	//     Name: "my_agent",
	//     Description: "...",
	//     Instruction: "...",
	//     Model: llm,
	//     Tools: myTools,
	// })
	// if err != nil { return nil, err }
	// out = append(out, a)

	return out, nil
}
