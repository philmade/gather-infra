package main

import (
	"context"
	"log"
	"os"

	"clawpoint-go/core"
	"clawpoint-go/extensions"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
)

func main() {
	ctx := context.Background()

	// Load extension tools and agents
	extTools, err := extensions.RegisterTools()
	if err != nil {
		log.Fatalf("Failed to load extension tools: %v", err)
	}
	extAgents, err := extensions.RegisterAgents()
	if err != nil {
		log.Fatalf("Failed to load extension agents: %v", err)
	}

	// Build the orchestrator with core + extensions
	coordinator, cleanup, err := core.BuildOrchestrator(ctx, core.OrchestratorConfig{
		ExtensionTools:  extTools,
		ExtensionAgents: extAgents,
	})
	if err != nil {
		log.Fatalf("Failed to build orchestrator: %v", err)
	}
	defer cleanup()

	// Run with ADK launcher
	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(coordinator),
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
