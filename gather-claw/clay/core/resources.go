package core

import (
	"context"
	"fmt"
	"os"

	"clay/core/agents"
	"clay/core/tools"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/tool"
)

// OrchestratorConfig configures the Clay agent.
type OrchestratorConfig struct {
	// ExtensionTools are additional tools registered by extensions,
	// added to the coordinator's direct tool set.
	ExtensionTools []tool.Tool

	// ExtensionAgents are additional sub-agents registered by extensions.
	ExtensionAgents []agent.Agent
}

// SharedResources holds the shared model, tools, and cleanup function
// used by both the clay coordinator and the clay loop agent.
type SharedResources struct {
	Model    model.LLM
	MemTool  *tools.MemoryTool
	Soul     *tools.SoulTool
	TaskTool *tools.TaskTool
	Cleanup  func()
}

// BuildSharedResources initializes the LLM, memory, soul, and task tools
// that are shared across all ADK apps in the process.
func BuildSharedResources(ctx context.Context) (*SharedResources, error) {
	llm, err := CreateModel(ctx)
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}

	dbPath := getEnv("CLAY_DB", "../messages.db")
	memTool, err := tools.NewMemoryTool(dbPath)
	if err != nil {
		return nil, fmt.Errorf("memory tool: %w", err)
	}
	cleanup := func() { memTool.Close() }

	soul := tools.NewSoulTool()

	taskTool, err := tools.NewTaskTool(memTool.DB())
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("task tool: %w", err)
	}

	return &SharedResources{
		Model:    llm,
		MemTool:  memTool,
		Soul:     soul,
		TaskTool: taskTool,
		Cleanup:  cleanup,
	}, nil
}

// opsDir returns the standard ops handoff directory.
// MANUAL.md and FEEDBACK.md live here.
func opsDir() string {
	root := os.Getenv("CLAY_ROOT")
	if root == "" {
		root = "."
	}
	return root + "/data/ops"
}

// buildSubAgentsWithPrefix creates claude + research sub-agents with unique name prefixes.
// Each caller gets its own instances (ADK sets parent pointers, so sharing
// sub-agents between agent trees causes conflicts).
func buildSubAgentsWithPrefix(llm model.LLM, memTool *tools.MemoryTool, prefix string) ([]agent.Agent, error) {
	claudeTools, err := tools.NewClaudeTools()
	if err != nil {
		return nil, fmt.Errorf("claude tools: %w", err)
	}
	// Give claude the memory tool so it can store a summary before returning to parent
	memoryTool, err := tools.NewConsolidatedMemoryTool(memTool)
	if err != nil {
		return nil, fmt.Errorf("claude memory tool: %w", err)
	}
	claudeTools = append(claudeTools, memoryTool)

	claudeAgent, err := agents.NewClaudeAgent(llm, claudeTools, prefix)
	if err != nil {
		return nil, fmt.Errorf("claude agent: %w", err)
	}

	researchTools, err := tools.NewResearchTools()
	if err != nil {
		return nil, fmt.Errorf("research tools: %w", err)
	}
	researchAgent, err := agents.NewResearchAgent(llm, researchTools, prefix)
	if err != nil {
		return nil, fmt.Errorf("research agent: %w", err)
	}

	return []agent.Agent{claudeAgent, researchAgent}, nil
}

// buildCoordinatorTools builds the full tool set for coordinator-level agents.
func buildCoordinatorTools(memTool *tools.MemoryTool, soul *tools.SoulTool, taskTool *tools.TaskTool) ([]tool.Tool, error) {
	var out []tool.Tool

	memoryTool, err := tools.NewConsolidatedMemoryTool(memTool)
	if err != nil {
		return nil, fmt.Errorf("memory tool: %w", err)
	}
	out = append(out, memoryTool)

	soulTool, err := tools.NewConsolidatedSoulTool(soul)
	if err != nil {
		return nil, fmt.Errorf("soul tool: %w", err)
	}
	out = append(out, soulTool)

	tasksTool, err := tools.NewConsolidatedTaskTool(taskTool)
	if err != nil {
		return nil, fmt.Errorf("tasks tool: %w", err)
	}
	out = append(out, tasksTool)

	buildTools, err := tools.NewBuildTools()
	if err != nil {
		return nil, err
	}
	out = append(out, buildTools...)

	extTools, err := tools.NewExtensionTools()
	if err != nil {
		return nil, err
	}
	out = append(out, extTools...)

	platformTools, err := tools.NewPlatformTools()
	if err != nil {
		return nil, fmt.Errorf("platform tools: %w", err)
	}
	if platformTools != nil {
		out = append(out, platformTools...)
	}

	return out, nil
}

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

func coreVersionPath() string {
	root := os.Getenv("CLAY_ROOT")
	if root == "" {
		root = "."
	}
	return root + "/core-version"
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
