package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"clawpoint-go/core"
	"clawpoint-go/core/connectors"
	"clawpoint-go/extensions"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/session"
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

	// In-memory sessions — compaction stores durable memories to messages.db,
	// and heartbeat injection restores continuity on restart.
	sessionService := session.InMemoryService()
	log.Printf("Session storage: in-memory")

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Printf("Shutting down...")
		cancel()
	}()

	// Start internal heartbeat goroutine — waits for ADK server to be ready,
	// then sends periodic [HEARTBEAT] messages through the middleware pipeline.
	// Uses the same port as the ADK server (default 8080, or ADK_PORT env var).
	adkPort := os.Getenv("ADK_PORT")
	if adkPort == "" {
		adkPort = "8080"
	}
	hb := connectors.NewInternalHeartbeat("http://127.0.0.1:" + adkPort)
	go hb.Start(ctx)

	// Run with ADK launcher
	config := &launcher.Config{
		AgentLoader:    agent.NewSingleLoader(coordinator),
		SessionService: sessionService,
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
